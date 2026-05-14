package investment

import (
	"sort"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

// sortTxnsForReplay sorts transactions by date ascending, then created_at
// ascending — the same canonical chronological order used by replayPosition
// and the TR-008 service wrapper.
func sortTxnsForReplay(txns []*Transaction) {
	sort.SliceStable(txns, func(i, j int) bool {
		if txns[i].Date.Time().Equal(txns[j].Date.Time()) {
			return txns[i].CreatedAt.Time().Before(txns[j].CreatedAt.Time())
		}
		return txns[i].Date.Time().Before(txns[j].Date.Time())
	})
}

// loadAndSortTxnsForSecurity returns this (account, security)'s investment
// transactions in canonical chronological order for replayRealizedGain.
func loadAndSortTxnsForSecurity(t *testing.T, env *testServiceEnv, accountID, securityID types.ID) []*Transaction {
	t.Helper()
	secFilter := securityID
	txns, err := env.invRepo.ListByAccount(accountID, TransactionFilter{SecurityID: &secFilter})
	if err != nil {
		t.Fatalf("ListByAccount(secFilter) error = %v", err)
	}
	sortTxnsForReplay(txns)
	return txns
}

// TR-001: New total-return fields default to zero values when callers pass
// ValuationOptions{}. Existing valuation semantics (CashBalance, MarketValue,
// TotalValue, TotalCostBasis, TotalGainLoss, TotalGainPct, per-holding values)
// remain unchanged.
func TestValuation_NewFieldsZeroValue_BackCompat(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("5000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	total := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	p := price.NewPrice(sec.ID, types.NewDate(2024, time.March, 20), types.MustNewMoney("120"), price.SourceManual)
	if err := env.priceRepo.Create(p); err != nil {
		t.Fatalf("Create price error = %v", err)
	}

	asOf := types.NewDate(2024, time.March, 20)

	t.Run("GetAccountValuation new fields zero", func(t *testing.T) {
		val, err := env.svc.GetAccountValuation(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetAccountValuation() error = %v", err)
		}

		if !val.RealizedGain.IsZero() {
			t.Errorf("Expected RealizedGain to be zero, got %s", val.RealizedGain.String())
		}
		if !val.DividendsReceived.IsZero() {
			t.Errorf("Expected DividendsReceived to be zero, got %s", val.DividendsReceived.String())
		}
		if !val.InterestReceived.IsZero() {
			t.Errorf("Expected InterestReceived to be zero, got %s", val.InterestReceived.String())
		}
		if !val.FeesPaid.IsZero() {
			t.Errorf("Expected FeesPaid to be zero, got %s", val.FeesPaid.String())
		}
		if !val.TotalCostDeployed.IsZero() {
			t.Errorf("Expected TotalCostDeployed to be zero, got %s", val.TotalCostDeployed.String())
		}
		if !val.TotalReturn.IsZero() {
			t.Errorf("Expected TotalReturn to be zero, got %s", val.TotalReturn.String())
		}
		if val.TotalReturnPct != nil {
			t.Errorf("Expected TotalReturnPct to be nil, got %v", *val.TotalReturnPct)
		}
		if val.HasClosedPositions {
			t.Errorf("Expected HasClosedPositions to be false, got true")
		}

		// Existing behavior must be unchanged.
		if val.CashBalance.String() != "4000" {
			t.Errorf("Expected CashBalance '4000', got %q", val.CashBalance.String())
		}
		if val.MarketValue.String() != "1200" {
			t.Errorf("Expected MarketValue '1200', got %q", val.MarketValue.String())
		}
		if val.TotalCostBasis.String() != "1000" {
			t.Errorf("Expected TotalCostBasis '1000', got %q", val.TotalCostBasis.String())
		}
		if val.TotalGainLoss.String() != "200" {
			t.Errorf("Expected TotalGainLoss '200', got %q", val.TotalGainLoss.String())
		}
	})

	t.Run("GetHoldings new fields zero", func(t *testing.T) {
		holdings, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetHoldings() error = %v", err)
		}
		if len(holdings) != 1 {
			t.Fatalf("Expected 1 holding, got %d", len(holdings))
		}
		h := holdings[0]

		if !h.RealizedGain.IsZero() {
			t.Errorf("Expected RealizedGain to be zero, got %s", h.RealizedGain.String())
		}
		if !h.DividendsReceived.IsZero() {
			t.Errorf("Expected DividendsReceived to be zero, got %s", h.DividendsReceived.String())
		}
		if !h.FeesPaid.IsZero() {
			t.Errorf("Expected FeesPaid to be zero, got %s", h.FeesPaid.String())
		}
		if !h.TotalCostDeployed.IsZero() {
			t.Errorf("Expected TotalCostDeployed to be zero, got %s", h.TotalCostDeployed.String())
		}
		if !h.TotalReturn.IsZero() {
			t.Errorf("Expected TotalReturn to be zero, got %s", h.TotalReturn.String())
		}
		if h.TotalReturnPct != nil {
			t.Errorf("Expected TotalReturnPct to be nil, got %v", *h.TotalReturnPct)
		}
		if h.IsClosed {
			t.Errorf("Expected IsClosed to be false, got true")
		}
		if h.RealizedGainUnavailable {
			t.Errorf("Expected RealizedGainUnavailable to be false, got true")
		}

		// Existing per-holding values unchanged.
		if h.Shares.String() != "10" {
			t.Errorf("Expected Shares '10', got %q", h.Shares.String())
		}
		if h.CostBasis.String() != "1000" {
			t.Errorf("Expected CostBasis '1000', got %q", h.CostBasis.String())
		}
		if h.MarketValue.String() != "1200" {
			t.Errorf("Expected MarketValue '1200', got %q", h.MarketValue.String())
		}
		if h.GainLoss.String() != "200" {
			t.Errorf("Expected GainLoss '200', got %q", h.GainLoss.String())
		}
	})
}

// TR-002: sumDividendsForSecurity sums `dividend` transactions for a single
// (account, security) pair. Reinvested dividends and other securities must
// not contribute.

func TestSumDividendsForSecurity_HappyPath(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("100"), ""); err != nil {
		t.Fatalf("Dividend() error = %v", err)
	}
	if _, err := env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("75"), ""); err != nil {
		t.Fatalf("Dividend() error = %v", err)
	}

	got, err := env.svc.sumDividendsForSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("sumDividendsForSecurity() error = %v", err)
	}
	if got.String() != "175" {
		t.Errorf("Expected total dividends '175', got %q", got.String())
	}

	other := createSec(t, env.secRepo, "MSFT")
	got, err = env.svc.sumDividendsForSecurity(acct.ID, other.ID)
	if err != nil {
		t.Fatalf("sumDividendsForSecurity() error = %v", err)
	}
	if !got.IsZero() {
		t.Errorf("Expected zero for security with no dividends, got %q", got.String())
	}
}

func TestSumDividendsForSecurity_IgnoresReinvest(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("5000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buyTotal := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	if _, err := env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("50"), ""); err != nil {
		t.Fatalf("Dividend() error = %v", err)
	}
	reinvestTotal := types.MustNewMoney("110")
	if _, err := env.svc.ReinvestDividend(acct.ID, sec.ID, date, types.MustNewQuantity("1"), &reinvestTotal, nil, ""); err != nil {
		t.Fatalf("ReinvestDividend() error = %v", err)
	}

	got, err := env.svc.sumDividendsForSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("sumDividendsForSecurity() error = %v", err)
	}
	if got.String() != "50" {
		t.Errorf("Expected only cash dividend '50' (reinvest excluded), got %q", got.String())
	}
}

func TestSumDividendsForSecurity_IgnoresOtherSecurities(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	aapl := createSec(t, env.secRepo, "AAPL")
	msft := createSec(t, env.secRepo, "MSFT")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Dividend(acct.ID, aapl.ID, date, types.MustNewMoney("80"), ""); err != nil {
		t.Fatalf("Dividend() error = %v", err)
	}
	if _, err := env.svc.Dividend(acct.ID, msft.ID, date, types.MustNewMoney("40"), ""); err != nil {
		t.Fatalf("Dividend() error = %v", err)
	}

	got, err := env.svc.sumDividendsForSecurity(acct.ID, aapl.ID)
	if err != nil {
		t.Fatalf("sumDividendsForSecurity() error = %v", err)
	}
	if got.String() != "80" {
		t.Errorf("Expected AAPL-only dividends '80', got %q", got.String())
	}
}

// TR-003: sumInterestForAccount sums `interest` transactions for an investment
// account. No security filter — interest is paid on the cash sweep, not a
// holding. Interest in sibling accounts must not leak in.

func TestSumInterestForAccount_HappyPath(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Interest(acct.ID, date, types.MustNewMoney("12.50"), ""); err != nil {
		t.Fatalf("Interest() error = %v", err)
	}
	if _, err := env.svc.Interest(acct.ID, date, types.MustNewMoney("7.25"), ""); err != nil {
		t.Fatalf("Interest() error = %v", err)
	}
	if _, err := env.svc.Interest(acct.ID, date, types.MustNewMoney("30.25"), ""); err != nil {
		t.Fatalf("Interest() error = %v", err)
	}

	got, err := env.svc.sumInterestForAccount(acct.ID)
	if err != nil {
		t.Fatalf("sumInterestForAccount() error = %v", err)
	}
	if got.String() != "50" {
		t.Errorf("Expected total interest '50', got %q", got.String())
	}

	empty := createInvAccount(t, env.accountRepo, "Empty")
	got, err = env.svc.sumInterestForAccount(empty.ID)
	if err != nil {
		t.Fatalf("sumInterestForAccount() error = %v", err)
	}
	if !got.IsZero() {
		t.Errorf("Expected zero for account with no interest, got %q", got.String())
	}
}

func TestSumInterestForAccount_OtherAccountIgnored(t *testing.T) {
	env := createFullTestService(t)
	a := createInvAccount(t, env.accountRepo, "Brokerage A")
	b := createInvAccount(t, env.accountRepo, "Brokerage B")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Interest(a.ID, date, types.MustNewMoney("20"), ""); err != nil {
		t.Fatalf("Interest() error = %v", err)
	}
	if _, err := env.svc.Interest(b.ID, date, types.MustNewMoney("99"), ""); err != nil {
		t.Fatalf("Interest() error = %v", err)
	}

	got, err := env.svc.sumInterestForAccount(a.ID)
	if err != nil {
		t.Fatalf("sumInterestForAccount() error = %v", err)
	}
	if got.String() != "20" {
		t.Errorf("Expected account A interest '20', got %q", got.String())
	}
}

// TR-004: sumFeesForSecurity sums commissions on buy/sell/reinvest_dividend
// transactions plus the full total_amount of any fee_liquidation transactions
// for a single (account, security) pair. Account-level `fee` transactions
// (no security_id) are deliberately excluded — they are summed separately at
// the account level. The result is a positive magnitude.

func TestSumFeesForSecurity_HappyPath(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}

	buyTotal := types.MustNewMoney("1005")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"),
		&buyTotal, nil, types.MustNewMoney("5"), ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	sellTotal := types.MustNewMoney("800")
	if _, err := env.svc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("5"),
		&sellTotal, nil, types.MustNewMoney("10"), "", nil); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	// Service-level ReinvestDividend hardcodes zero commission; persist a $1
	// commission on the resulting row so the helper exercises the
	// reinvest_dividend.commission branch per the spec.
	reinvestTotal := types.MustNewMoney("100")
	rtxn, err := env.svc.ReinvestDividend(acct.ID, sec.ID, date,
		types.MustNewQuantity("1"), &reinvestTotal, nil, "")
	if err != nil {
		t.Fatalf("ReinvestDividend() error = %v", err)
	}
	rtxn.SetCommission(types.MustNewMoney("1"))
	if err := env.invRepo.Update(rtxn); err != nil {
		t.Fatalf("Update(reinvest commission) error = %v", err)
	}

	feeTotal := types.MustNewMoney("25")
	if _, err := env.svc.FeeLiquidation(acct.ID, sec.ID, date,
		types.MustNewQuantity("0.1"), &feeTotal, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("FeeLiquidation() error = %v", err)
	}

	got, err := env.svc.sumFeesForSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("sumFeesForSecurity() error = %v", err)
	}
	if got.String() != "41" {
		t.Errorf("Expected total fees '41' (5 + 10 + 1 + 25), got %q", got.String())
	}
}

func TestSumFeesForSecurity_NoCommissionField(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("5000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}

	buyTotal := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"),
		&buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	sellTotal := types.MustNewMoney("600")
	if _, err := env.svc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("5"),
		&sellTotal, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	got, err := env.svc.sumFeesForSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("sumFeesForSecurity() error = %v", err)
	}
	if !got.IsZero() {
		t.Errorf("Expected zero (no commissions set), got %q", got.String())
	}
}

func TestSumFeesForSecurity_AccountLevelFeeIgnored(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("5000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	if _, err := env.svc.Fee(acct.ID, date, types.MustNewMoney("50"), ""); err != nil {
		t.Fatalf("Fee() error = %v", err)
	}
	buyTotal := types.MustNewMoney("105")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("1"),
		&buyTotal, nil, types.MustNewMoney("5"), ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	got, err := env.svc.sumFeesForSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("sumFeesForSecurity() error = %v", err)
	}
	if got.String() != "5" {
		t.Errorf("Expected only the per-security commission '5' (account fee excluded), got %q", got.String())
	}
}

// TR-005: sumFeesForAccount aggregates fees across every security in the
// account plus any account-level `fee` transactions (which have no
// security_id). The result is a positive magnitude.

func TestSumFeesForAccount_AccumulatesPerSecurityAndAccount(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	aapl := createSec(t, env.secRepo, "AAPL")
	msft := createSec(t, env.secRepo, "MSFT")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("20000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}

	// AAPL: buy commission $5, sell commission $10, fee_liquidation $25 = $40.
	buyA := types.MustNewMoney("1005")
	if _, err := env.svc.Buy(acct.ID, aapl.ID, date, types.MustNewQuantity("10"),
		&buyA, nil, types.MustNewMoney("5"), ""); err != nil {
		t.Fatalf("Buy(AAPL) error = %v", err)
	}
	sellA := types.MustNewMoney("800")
	if _, err := env.svc.Sell(acct.ID, aapl.ID, date, types.MustNewQuantity("5"),
		&sellA, nil, types.MustNewMoney("10"), "", nil); err != nil {
		t.Fatalf("Sell(AAPL) error = %v", err)
	}
	feeA := types.MustNewMoney("25")
	if _, err := env.svc.FeeLiquidation(acct.ID, aapl.ID, date,
		types.MustNewQuantity("0.1"), &feeA, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("FeeLiquidation(AAPL) error = %v", err)
	}

	// MSFT: buy commission $7, sell commission $3 = $10.
	buyM := types.MustNewMoney("507")
	if _, err := env.svc.Buy(acct.ID, msft.ID, date, types.MustNewQuantity("5"),
		&buyM, nil, types.MustNewMoney("7"), ""); err != nil {
		t.Fatalf("Buy(MSFT) error = %v", err)
	}
	sellM := types.MustNewMoney("300")
	if _, err := env.svc.Sell(acct.ID, msft.ID, date, types.MustNewQuantity("2"),
		&sellM, nil, types.MustNewMoney("3"), "", nil); err != nil {
		t.Fatalf("Sell(MSFT) error = %v", err)
	}

	// Account-level fees (no security_id): $15 + $20 = $35.
	if _, err := env.svc.Fee(acct.ID, date, types.MustNewMoney("15"), ""); err != nil {
		t.Fatalf("Fee() error = %v", err)
	}
	if _, err := env.svc.Fee(acct.ID, date, types.MustNewMoney("20"), ""); err != nil {
		t.Fatalf("Fee() error = %v", err)
	}

	got, err := env.svc.sumFeesForAccount(acct.ID)
	if err != nil {
		t.Fatalf("sumFeesForAccount() error = %v", err)
	}
	// AAPL $40 + MSFT $10 + account-level $35 = $85.
	if got.String() != "85" {
		t.Errorf("Expected total fees '85' (AAPL 40 + MSFT 10 + acct 35), got %q", got.String())
	}
}

func TestSumFeesForAccount_OnlyAccountLevelFees(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("5000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	if _, err := env.svc.Fee(acct.ID, date, types.MustNewMoney("12"), ""); err != nil {
		t.Fatalf("Fee() error = %v", err)
	}
	if _, err := env.svc.Fee(acct.ID, date, types.MustNewMoney("8"), ""); err != nil {
		t.Fatalf("Fee() error = %v", err)
	}

	got, err := env.svc.sumFeesForAccount(acct.ID)
	if err != nil {
		t.Fatalf("sumFeesForAccount() error = %v", err)
	}
	if got.String() != "20" {
		t.Errorf("Expected total fees '20' (account-level only), got %q", got.String())
	}

	empty := createInvAccount(t, env.accountRepo, "Empty")
	got, err = env.svc.sumFeesForAccount(empty.ID)
	if err != nil {
		t.Fatalf("sumFeesForAccount(empty) error = %v", err)
	}
	if !got.IsZero() {
		t.Errorf("Expected zero for account with no fees, got %q", got.String())
	}
}

// TR-006: realizedGainLotTracked walks the transaction_lots junction for sell
// and fee_liquidation transactions and sums
// (txn.price_per_share − lot.cost_per_share) × junction.shares. Commission is
// already netted into txn.price_per_share by ComputePricePerShare; it is
// counted separately as a fee, not subtracted twice.

func TestRealizedGainLotTracked_SingleSell_AtGain(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buyTotal := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"),
		&buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
	if err != nil {
		t.Fatalf("ListByAccountAndSecurity() error = %v", err)
	}
	if len(lots) != 1 {
		t.Fatalf("Expected 1 lot, got %d", len(lots))
	}

	sellTotal := types.MustNewMoney("750")
	allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("5")}}
	if _, err := env.svc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("5"),
		&sellTotal, nil, types.ZeroMoney, "", allocs); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	got, err := env.svc.realizedGainLotTracked(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("realizedGainLotTracked() error = %v", err)
	}
	// (150 − 100) × 5 = 250
	if got.String() != "250" {
		t.Errorf("Expected realized gain '250', got %q", got.String())
	}
}

func TestRealizedGainLotTracked_SingleSell_AtLoss(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buyTotal := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"),
		&buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
	if err != nil {
		t.Fatalf("ListByAccountAndSecurity() error = %v", err)
	}

	sellTotal := types.MustNewMoney("400")
	allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("5")}}
	if _, err := env.svc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("5"),
		&sellTotal, nil, types.ZeroMoney, "", allocs); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	got, err := env.svc.realizedGainLotTracked(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("realizedGainLotTracked() error = %v", err)
	}
	// (80 − 100) × 5 = −100
	if got.String() != "-100" {
		t.Errorf("Expected realized gain '-100', got %q", got.String())
	}
}

func TestRealizedGainLotTracked_MultipleSells_AcrossLots(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date1 := types.NewDate(2024, time.March, 1)
	date2 := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date1, types.MustNewMoney("20000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	// Lot 1: 10 @ $100
	buy1 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date1, types.MustNewQuantity("10"),
		&buy1, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(lot1) error = %v", err)
	}
	// Lot 2: 5 @ $200
	buy2 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date2, types.MustNewQuantity("5"),
		&buy2, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(lot2) error = %v", err)
	}

	lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
	if err != nil {
		t.Fatalf("ListByAccountAndSecurity() error = %v", err)
	}
	if len(lots) != 2 {
		t.Fatalf("Expected 2 lots, got %d", len(lots))
	}

	// Sell 8 @ $250 — 5 from lot 1, 3 from lot 2.
	sellTotal := types.MustNewMoney("2000")
	allocs := []SellLotAllocation{
		{LotID: lots[0].ID, Shares: types.MustNewQuantity("5")},
		{LotID: lots[1].ID, Shares: types.MustNewQuantity("3")},
	}
	if _, err := env.svc.Sell(acct.ID, sec.ID, date2, types.MustNewQuantity("8"),
		&sellTotal, nil, types.ZeroMoney, "", allocs); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	got, err := env.svc.realizedGainLotTracked(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("realizedGainLotTracked() error = %v", err)
	}
	// (250 − 100) × 5 + (250 − 200) × 3 = 750 + 150 = 900
	if got.String() != "900" {
		t.Errorf("Expected realized gain '900', got %q", got.String())
	}
}

func TestRealizedGainLotTracked_FeeLiquidation(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buyTotal := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"),
		&buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
	if err != nil {
		t.Fatalf("ListByAccountAndSecurity() error = %v", err)
	}

	// fee_liquidation: 0.1 shares at $200 from a lot bought at $100.
	feeTotal := types.MustNewMoney("20")
	allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("0.1")}}
	if _, err := env.svc.FeeLiquidation(acct.ID, sec.ID, date, types.MustNewQuantity("0.1"),
		&feeTotal, nil, types.ZeroMoney, "", allocs); err != nil {
		t.Fatalf("FeeLiquidation() error = %v", err)
	}

	got, err := env.svc.realizedGainLotTracked(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("realizedGainLotTracked() error = %v", err)
	}
	// (200 − 100) × 0.1 = 10
	if got.String() != "10" {
		t.Errorf("Expected realized gain '10', got %q", got.String())
	}
}

func TestRealizedGainLotTracked_NoSells_ReturnsZero(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buyTotal := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"),
		&buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	got, err := env.svc.realizedGainLotTracked(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("realizedGainLotTracked() error = %v", err)
	}
	if !got.IsZero() {
		t.Errorf("Expected zero (no sells), got %q", got.String())
	}
}

// TR-007: replayRealizedGain walks a non-lot (account, security) ledger in
// chronological order, accumulating
// (sell.price_per_share − running_avg_cost_per_share) × sell.shares on each
// sell. Buys and reinvest_dividend transactions update the running average
// cost via the same weighted-average that Position.AddShares uses, so the
// avg cost moves with new acquisitions.

func TestReplayRealizedGain_HappyPath(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date1 := types.NewDate(2024, time.March, 1)
	date2 := types.NewDate(2024, time.March, 15)
	date3 := types.NewDate(2024, time.April, 1)
	date4 := types.NewDate(2024, time.April, 15)

	if _, err := env.svc.Deposit(acct.ID, date1, types.MustNewMoney("50000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	// Buy 10 @ $100 → avg cost $100
	buy1 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date1, types.MustNewQuantity("10"),
		&buy1, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(1) error = %v", err)
	}
	// Buy 10 @ $120 → avg cost (1000+1200)/20 = $110
	buy2 := types.MustNewMoney("1200")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date2, types.MustNewQuantity("10"),
		&buy2, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(2) error = %v", err)
	}
	// Sell 5 @ $150 → realized = (150−110)×5 = 200
	sell1 := types.MustNewMoney("750")
	if _, err := env.svc.Sell(acct.ID, sec.ID, date3, types.MustNewQuantity("5"),
		&sell1, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(1) error = %v", err)
	}
	// Sell 5 @ $130 → realized = (130−110)×5 = 100 (RemoveShares leaves avg unchanged)
	sell2 := types.MustNewMoney("650")
	if _, err := env.svc.Sell(acct.ID, sec.ID, date4, types.MustNewQuantity("5"),
		&sell2, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(2) error = %v", err)
	}

	txns := loadAndSortTxnsForSecurity(t, env, acct.ID, sec.ID)
	got, err := env.svc.replayRealizedGain(acct.ID, sec.ID, txns)
	if err != nil {
		t.Fatalf("replayRealizedGain() error = %v", err)
	}
	if got.String() != "300" {
		t.Errorf("Expected realized gain '300' (200 + 100), got %q", got.String())
	}
}

func TestReplayRealizedGain_LossThenGain(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date1 := types.NewDate(2024, time.January, 1)
	date2 := types.NewDate(2024, time.February, 1)
	date3 := types.NewDate(2024, time.March, 1)
	date4 := types.NewDate(2024, time.April, 1)

	if _, err := env.svc.Deposit(acct.ID, date1, types.MustNewMoney("50000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	// Buy 10 @ $100 → avg cost $100
	buy1 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date1, types.MustNewQuantity("10"),
		&buy1, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(1) error = %v", err)
	}
	// Sell 5 @ $80 (loss) → realized = (80−100)×5 = −100. 5 shares remain @ $100.
	sell1 := types.MustNewMoney("400")
	if _, err := env.svc.Sell(acct.ID, sec.ID, date2, types.MustNewQuantity("5"),
		&sell1, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(1) error = %v", err)
	}
	// Buy 5 @ $200 → avg cost = (5×100 + 5×200)/10 = $150. Confirms a subsequent
	// buy does not "reset" the running avg cost — it accumulates through the
	// existing Position.AddShares weighted average.
	buy2 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date3, types.MustNewQuantity("5"),
		&buy2, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(2) error = %v", err)
	}
	// Sell 10 @ $200 → realized = (200−150)×10 = +500.
	sell2 := types.MustNewMoney("2000")
	if _, err := env.svc.Sell(acct.ID, sec.ID, date4, types.MustNewQuantity("10"),
		&sell2, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(2) error = %v", err)
	}

	txns := loadAndSortTxnsForSecurity(t, env, acct.ID, sec.ID)
	got, err := env.svc.replayRealizedGain(acct.ID, sec.ID, txns)
	if err != nil {
		t.Fatalf("replayRealizedGain() error = %v", err)
	}
	// −100 + 500 = 400
	if got.String() != "400" {
		t.Errorf("Expected realized gain '400' (−100 + 500), got %q", got.String())
	}
}

func TestReplayRealizedGain_SameDateOrdering(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	earlier := types.NewDate(2024, time.March, 1)
	same := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, earlier, types.MustNewMoney("50000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	// All four share the SAME date, but distinct CreatedAt timestamps from
	// monotonic insertion order. Sorting by (Date asc, CreatedAt asc) must
	// preserve insertion order so the realized gain is deterministic.
	buy1 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, same, types.MustNewQuantity("10"),
		&buy1, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(1) error = %v", err)
	}
	sell1 := types.MustNewMoney("750")
	if _, err := env.svc.Sell(acct.ID, sec.ID, same, types.MustNewQuantity("5"),
		&sell1, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(1) error = %v", err)
	}
	buy2 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, same, types.MustNewQuantity("5"),
		&buy2, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(2) error = %v", err)
	}
	sell2 := types.MustNewMoney("1000")
	if _, err := env.svc.Sell(acct.ID, sec.ID, same, types.MustNewQuantity("5"),
		&sell2, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(2) error = %v", err)
	}

	txns := loadAndSortTxnsForSecurity(t, env, acct.ID, sec.ID)
	got, err := env.svc.replayRealizedGain(acct.ID, sec.ID, txns)
	if err != nil {
		t.Fatalf("replayRealizedGain() error = %v", err)
	}
	// Buy 10 @ 100 → avg 100, 10 sh.
	// Sell 5 @ 150 → realized (150−100)×5 = 250; 5 sh @ 100.
	// Buy 5 @ 200 → avg (5×100 + 5×200)/10 = 150; 10 sh.
	// Sell 5 @ 200 → realized (200−150)×5 = 250.
	// Total = 500.
	//
	// Reordered (e.g. both buys before both sells) the result is 250 + 166.67
	// = 416.67 — a different number. Asserting 500 confirms CreatedAt-order
	// determinism.
	if got.String() != "500" {
		t.Errorf("Expected realized gain '500' (same-date insertion order), got %q", got.String())
	}
}

func TestReplayRealizedGain_ReinvestRaisesAvgCost(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date1 := types.NewDate(2024, time.March, 1)
	date2 := types.NewDate(2024, time.March, 15)
	date3 := types.NewDate(2024, time.April, 1)

	if _, err := env.svc.Deposit(acct.ID, date1, types.MustNewMoney("50000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	// Buy 10 @ $100 → avg cost $100.
	buy1 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date1, types.MustNewQuantity("10"),
		&buy1, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	// Reinvest 10 @ $200 (total $2000) → avg cost (1000 + 2000)/20 = $150.
	reinvest := types.MustNewMoney("2000")
	if _, err := env.svc.ReinvestDividend(acct.ID, sec.ID, date2,
		types.MustNewQuantity("10"), &reinvest, nil, ""); err != nil {
		t.Fatalf("ReinvestDividend() error = %v", err)
	}
	// Sell 5 @ $200 → realized (200−150)×5 = 250.
	// Without the reinvest the avg cost would still be $100 and realized
	// would be (200−100)×5 = 500. Asserting 250 confirms the reinvest
	// raised the avg cost.
	sell := types.MustNewMoney("1000")
	if _, err := env.svc.Sell(acct.ID, sec.ID, date3, types.MustNewQuantity("5"),
		&sell, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	txns := loadAndSortTxnsForSecurity(t, env, acct.ID, sec.ID)
	got, err := env.svc.replayRealizedGain(acct.ID, sec.ID, txns)
	if err != nil {
		t.Fatalf("replayRealizedGain() error = %v", err)
	}
	if got.String() != "250" {
		t.Errorf("Expected realized gain '250' (reinvest raised avg cost to $150), got %q", got.String())
	}
}
