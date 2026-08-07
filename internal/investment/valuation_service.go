package investment

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

// GetAccountValuation returns the total valuation of an investment account.
// It computes cash balance + market value of all holdings. Securities with
// no price as of the given date use cost basis as the estimated value.
//
// The returned struct also carries the total-return breakdown
// (RealizedGain, DividendsReceived, InterestReceived, FeesPaid,
// TotalCostDeployed, TotalReturn, TotalReturnPct). InterestReceived,
// FeesPaid (per-security commissions + account-level fee transactions),
// and TotalCostDeployed are pulled from authoritative account-level
// helpers. RealizedGain and DividendsReceived are summed across the
// per-holding values produced by enrichHoldingTotalReturn, **including
// synthesized rows for closed positions** — so an account whose positions
// have all been sold still reports the realized gains and dividends those
// positions produced. The legacy TotalGainLoss / TotalGainPct fields
// retain their unrealized-only meaning.
//
// When opts.IncludeClosed is true, the returned Holdings slice contains
// synthesized rows for securities the account has ever held but no longer
// holds (Shares == 0, MarketValue / CostBasis zero, IsClosed = true) with
// total-return components populated from the ledger. When false, those
// rows are filtered out of the returned slice, but their realized gain
// and dividends are still counted in the account-level totals.
//
// HasClosedPositions is set whenever the account has at least one
// fully-sold security, regardless of opts.IncludeClosed — it advises the
// caller that there are closed positions to display.
// ValuationService is the read model for an investment account: what is held,
// what it is worth, and what it has returned.
//
// It is the second type extracted out of investment.Service, and the property
// that makes it worth having is not its size — it is what it CANNOT do. It holds
// eight repositories and no *db.DB and no bound-tx field, exactly like
// CounterpartService, but for a different reason: CounterpartService cannot open
// a transaction because it always joins the caller's, while ValuationService
// cannot open one because it never writes at all. Every method here reads.
//
// It also has no InTx, and that is deliberate rather than an omission. The
// cluster is a leaf: nothing on the write side of this package calls into it,
// and every production caller is a view, a CLI command or a report
// (registry.go's report adapter, portfolio/dashboard/register views,
// cli/investment). None of them is inside a transaction, so there is no
// caller's transaction to join and reads correctly see committed state. If a
// future caller does need these figures mid-transaction, it needs an InTx that
// rebinds all eight fields — and a binding test, per design section 8.
type ValuationService struct {
	repo                *Repository
	accountRepo         *account.Repository
	positionRepo        *PositionRepository
	lotRepo             *LotRepository
	transactionLotRepo  *TransactionLotRepository
	priceRepo           *price.Repository
	corporateActionRepo *CorporateActionRepository
	holdingsRepo        *HoldingsRepository
}

// NewValuationService creates the investment read model.
func NewValuationService(
	repo *Repository,
	accountRepo *account.Repository,
	positionRepo *PositionRepository,
	lotRepo *LotRepository,
	transactionLotRepo *TransactionLotRepository,
	priceRepo *price.Repository,
	corporateActionRepo *CorporateActionRepository,
	database *db.DB,
) *ValuationService {
	return &ValuationService{
		repo:                repo,
		accountRepo:         accountRepo,
		positionRepo:        positionRepo,
		lotRepo:             lotRepo,
		transactionLotRepo:  transactionLotRepo,
		priceRepo:           priceRepo,
		corporateActionRepo: corporateActionRepo,
		holdingsRepo:        NewHoldingsRepository(database),
	}
}

func (s *ValuationService) GetAccountValuation(accountID types.ID, asOf types.Date, opts ValuationOptions) (*AccountValuation, error) {
	acct, err := loadInvestmentAccount(s.accountRepo, accountID)
	if err != nil {
		return nil, err
	}

	cashBalance, err := cashBalanceOf(s.repo, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get cash balance: %w", err)
	}

	// Always pull the full holdings list (open + closed) for accumulation
	// so closed positions contribute their realized gain and dividends to
	// the account-level totals. Filter to open-only for the returned slice
	// when the caller didn't ask for closed.
	holdingsAll, err := s.getHoldings(acct, asOf, ValuationOptions{IncludeClosed: true})
	if err != nil {
		return nil, fmt.Errorf("failed to get holdings: %w", err)
	}

	marketValue := types.ZeroMoney
	totalCostBasis := types.ZeroMoney
	realizedGain := types.ZeroMoney
	dividendsReceived := types.ZeroMoney
	anyRealizedUnavailable := false
	for _, h := range holdingsAll {
		marketValue = marketValue.Add(h.MarketValue)
		totalCostBasis = totalCostBasis.Add(h.CostBasis)
		realizedGain = realizedGain.Add(h.RealizedGain)
		dividendsReceived = dividendsReceived.Add(h.DividendsReceived)
		if h.RealizedGainUnavailable {
			anyRealizedUnavailable = true
		}
	}

	holdings := holdingsAll
	if !opts.IncludeClosed {
		holdings = make([]Holding, 0, len(holdingsAll))
		for _, h := range holdingsAll {
			if !h.IsClosed {
				holdings = append(holdings, h)
			}
		}
	}

	totalValue := cashBalance.Add(marketValue)
	totalGainLoss := marketValue.Sub(totalCostBasis)
	totalGainPct := computeGainPct(marketValue, totalCostBasis)

	interestReceived, err := s.sumInterestForAccount(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to sum interest for account: %w", err)
	}
	feesPaid, err := s.sumFeesForAccount(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to sum fees for account: %w", err)
	}
	totalCostDeployed, err := s.totalCostDeployedForAccount(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to sum total cost deployed for account: %w", err)
	}

	// HasClosedPositions advises callers (CLI footer, TUI affordance) that
	// the account has at least one fully-sold security. It is set
	// independently of opts.IncludeClosed: it describes the account's
	// history, not the shape of the returned holdings slice. We count the
	// distinct securities ever held in the ledger and compare against the
	// number of open positions in `holdings` (those without IsClosed).
	openCount := 0
	for _, h := range holdings {
		if !h.IsClosed {
			openCount++
		}
	}
	everHeld, err := s.listEverHeldSecurities(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to list ever-held securities: %w", err)
	}
	closedPositionCount := max(len(everHeld)-openCount, 0)
	hasClosedPositions := closedPositionCount > 0

	totalReturn := totalGainLoss.
		Add(realizedGain).
		Add(dividendsReceived).
		Add(interestReceived).
		Sub(feesPaid)
	var totalReturnPct *float64
	if !totalCostDeployed.IsZero() {
		pct := (totalReturn.Float64() / totalCostDeployed.Float64()) * 100
		totalReturnPct = &pct
	}

	return &AccountValuation{
		AccountID:              accountID,
		CashBalance:            cashBalance,
		MarketValue:            marketValue,
		TotalValue:             totalValue,
		TotalCostBasis:         totalCostBasis,
		TotalGainLoss:          totalGainLoss,
		TotalGainPct:           totalGainPct,
		Holdings:               holdings,
		RealizedGain:           realizedGain,
		DividendsReceived:      dividendsReceived,
		InterestReceived:       interestReceived,
		FeesPaid:               feesPaid,
		TotalCostDeployed:      totalCostDeployed,
		TotalReturn:            totalReturn,
		TotalReturnPct:         totalReturnPct,
		HasClosedPositions:     hasClosedPositions,
		ClosedPositionCount:    closedPositionCount,
		AnyRealizedUnavailable: anyRealizedUnavailable,
	}, nil
}

// GetHoldings returns a list of holdings for an investment account, rolled up by security.
// For lot-tracking accounts, lots are aggregated into a single holding per security.
//
// When opts.IncludeClosed is true, the returned slice also contains
// synthesized rows (Shares == 0, IsClosed = true) for securities the
// account has ever held but no longer holds, with total-return components
// populated from the ledger. Otherwise only open positions are returned.
func (s *ValuationService) GetHoldings(accountID types.ID, asOf types.Date, opts ValuationOptions) ([]Holding, error) {
	acct, err := loadInvestmentAccount(s.accountRepo, accountID)
	if err != nil {
		return nil, err
	}

	return s.getHoldings(acct, asOf, opts)
}

// GetLotDetail returns lot-level detail for a specific security in a lot-tracking account.
func (s *ValuationService) GetLotDetail(accountID types.ID, securityID types.ID, asOf types.Date) ([]LotDetail, error) {
	acct, err := loadInvestmentAccount(s.accountRepo, accountID)
	if err != nil {
		return nil, err
	}

	if !acct.TrackLots {
		return nil, fmt.Errorf("account %s does not use lot tracking", accountID)
	}

	lots, err := s.lotRepo.ListByAccountAndSecurity(accountID, securityID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list lots: %w", err)
	}

	currentPrice, hasPricing := s.getCurrentPrice(securityID, asOf)

	details := make([]LotDetail, 0, len(lots))
	for _, lot := range lots {
		costBasis := lot.CostBasis()

		var currentValue types.Money
		if hasPricing {
			currentValue = currentPrice.Mul(lot.Shares.Decimal())
		} else {
			currentValue = costBasis
		}

		gainLoss := currentValue.Sub(costBasis)

		details = append(details, LotDetail{
			LotID:        lot.ID,
			PurchaseDate: lot.PurchaseDate,
			Shares:       lot.Shares,
			CostPerShare: lot.CostPerShare,
			CostBasis:    costBasis,
			CurrentValue: currentValue,
			GainLoss:     gainLoss,
			GainPct:      computeGainPct(currentValue, costBasis),
		})
	}

	return details, nil
}
