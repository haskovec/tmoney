package app

import (
	"errors"
	"fmt"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/reconciliation"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/transferlink"
	"github.com/haskovec/tmoney/internal/types"
)

// Services is the central registry for all application services and repositories.
// This is the single source of truth for wiring up the application layer.
type Services struct {
	// Services
	Account        *account.Service
	Transaction    *transaction.Service
	Category       *category.Service
	Payee          *payee.Service
	Scheduled      *scheduled.Service
	Report         *report.Service
	Reconciliation *reconciliation.Service
	Security       *security.Service
	Price          *price.Service
	Investment     *investment.Service
	// InvestmentValuation is the read-only half: holdings, valuation and total
	// return. It writes nothing and opens no transaction.
	InvestmentValuation *investment.ValuationService
	// InvestmentEdit owns the ten edit entry points. It holds the core service
	// and rebinds it, so an edit joins a caller's transaction rather than
	// opening a second one.
	InvestmentEdit  *investment.EditService
	CorporateAction *investment.CorporateActionService
	TransferLink    *transferlink.Service

	// Transfer owns whole-transaction cash transfers across both ledgers —
	// bank↔bank, bank↔investment and investment↔investment alike. It is the
	// single door for creating, editing, voiding and deleting them; callers
	// must not reach past it into transaction.Service or investment.Service
	// for transfer work.
	Transfer *transfer.Service

	// Repositories (exposed for direct use by CLI/TUI when needed)
	AccountRepo     *account.Repository
	TransactionRepo *transaction.Repository
	SplitRepo       *transaction.SplitRepository
	CategoryRepo    *category.Repository
	PayeeRepo       *payee.Repository
	InvestmentRepo  *investment.Repository
	LotRepo         *investment.LotRepository
	PositionRepo    *investment.PositionRepository

	// ValueAdjustmentUserCollision is true when a *user* (non-system)
	// category named "Value Adjustment" already exists, so the system
	// category could not be seeded. Prepare sets it. The TUI surfaces a one-time
	// notice; the CLI ignores it.
	ValueAdjustmentUserCollision bool
}

// NewServices creates all repositories and services with proper dependency
// wiring. It writes nothing to the file; call Prepare after it.
func NewServices(database *db.DB) *Services {
	// Create repositories (leaf dependencies first)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	payeeRepo := payee.NewRepository(database)
	securityRepo := security.NewRepository(database)
	priceRepo := price.NewRepository(database)

	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)

	scheduledRepo := scheduled.NewRepository(database)
	reconciliationRepo := reconciliation.NewRepository(database)

	investmentRepo := investment.NewRepository(database)
	lotRepo := investment.NewLotRepository(database)
	positionRepo := investment.NewPositionRepository(database)
	transactionLotRepo := investment.NewTransactionLotRepository(database)
	corporateActionRepo := investment.NewCorporateActionRepository(database)

	// Create services (inject cross-slice repo dependencies)
	categorySvc := category.NewService(categoryRepo, database)
	payeeSvc := payee.NewService(payeeRepo, database)
	securitySvc := security.NewService(securityRepo, database,
		security.WithLotChecker(lotRepo),
		security.WithPositionChecker(positionRepo),
	)

	// Construction order now runs investment FIRST, then transaction.
	//
	// It used to be the other way round, with txnSvc.SetInvestmentCounterpart
	// patching the dependency in afterwards, because investment.NewService needed
	// a *transaction.Repository. It no longer does — the whole-transfer surface
	// that wanted it moved to internal/transfer — so investment.Service can be
	// built first and passed to transaction.NewService as its counterpart port.
	// The post-construction setter is gone, and with it the window in which a
	// transaction service existed with a nil counterpart.
	priceSvc := price.NewService(priceRepo, securityRepo, database)
	investmentSvc := investment.NewService(investmentRepo, accountRepo, positionRepo, lotRepo, transactionLotRepo, priceRepo, corporateActionRepo, database)
	// The read model is its own type: it holds the same eight repositories but no
	// database handle, so it can only read committed state. Views, reports and CLI
	// commands take this rather than the full service.
	investmentValuationSvc := investment.NewValuationService(investmentRepo, accountRepo, positionRepo, lotRepo, transactionLotRepo, priceRepo, corporateActionRepo, database)
	// Close judges an investment account by its investment ledger: its
	// register balance is always zero. The read model supplies that ledger
	// state through account's own port, since account cannot import
	// investment. Built here, after the read model, so no setter is needed.
	accountSvc := account.NewService(accountRepo, database, account.WithInvestmentLedger(investmentValuationSvc))
	investmentEditSvc := investment.NewEditService(investmentSvc)

	// The counterpart port mints and cleans up the investment-side row of a
	// transfer LINE inside a split (e.g. a paycheck → 401k contribution line).
	// Whole-transaction transfers do not go through it — internal/transfer owns
	// those.
	//
	// It is its own small type rather than investmentSvc, which used to satisfy
	// this interface. CounterpartService holds only the two repositories these
	// four methods need, and holds no *db.DB at all, so it cannot open a
	// transaction — it can only join the one transaction.Service hands it.
	txnSvc := transaction.NewService(txnRepo, splitRepo, payeeRepo, accountRepo,
		investment.NewCounterpartService(investmentRepo, accountRepo), database)
	scheduledSvc := scheduled.NewService(scheduledRepo, txnRepo, txnSvc, database, accountRepo)
	reconciliationSvc := reconciliation.NewService(reconciliationRepo, txnRepo, accountRepo, database)
	reportSvc := report.NewService(accountRepo, database, report.WithInvestmentValuer(&investmentValuerAdapter{svc: investmentValuationSvc}))
	corporateActionSvc := investment.NewCorporateActionService(corporateActionRepo, lotRepo, positionRepo, priceRepo, investmentRepo, securityRepo, database)
	transferSvc := transfer.NewService(txnRepo, investmentRepo, splitRepo, accountRepo, categoryRepo, database)
	// transferlink decides what to link; the transfer owner performs the link,
	// so there is one place that stamps a transfer_id and mutual
	// transfer_account_ids.
	transferLinkSvc := transferlink.NewService(txnRepo, transferSvc, splitRepo, accountRepo, database)
	// Scheduled posting routes transfer occurrences through the transfer owner.
	// Injected after construction because a direct scheduled → transfer import is
	// an "import cycle not allowed in test" (see scheduled/transfer_port.go).
	scheduledSvc.SetTransferPort(transferSvc)

	return &Services{
		Account:             accountSvc,
		Transaction:         txnSvc,
		Category:            categorySvc,
		Payee:               payeeSvc,
		Scheduled:           scheduledSvc,
		Report:              reportSvc,
		Reconciliation:      reconciliationSvc,
		Security:            securitySvc,
		Price:               priceSvc,
		Investment:          investmentSvc,
		InvestmentValuation: investmentValuationSvc,
		InvestmentEdit:      investmentEditSvc,
		CorporateAction:     corporateActionSvc,
		TransferLink:        transferLinkSvc,
		Transfer:            transferSvc,

		AccountRepo:     accountRepo,
		TransactionRepo: txnRepo,
		SplitRepo:       splitRepo,
		CategoryRepo:    categoryRepo,
		PayeeRepo:       payeeRepo,
		InvestmentRepo:  investmentRepo,
		LotRepo:         lotRepo,
		PositionRepo:    positionRepo,
	}
}

// Prepare runs the repairs an older file may need, once per open. NewServices
// only wires the graph and writes nothing; every opener calls Prepare after it
// (cmdutil.OpenServices, the TUI's newTUIServices).
//
// The steps, in order:
//
//   - seed the paycheck-wizard categories, so existing files gain them;
//   - seed the system Value Adjustment category, recording in
//     ValueAdjustmentUserCollision when a user category already holds the
//     name (the TUI shows a one-time notice);
//   - heal desynced positions and lots, so the user need not run
//     rebuild-positions after an upgrade;
//   - heal schedule rows whose NextDate an older binary left behind StartDate;
//   - clear categories from transfer schedules whose pair cannot store one:
//     older binaries let that be created, and the transfer owner refuses to
//     post it. This needs the transfer port, which NewServices wires.
//
// Every step runs even after one fails, and Prepare returns errors.Join of
// every failure. The steps do not depend on each other's data, so one failure
// should not stop the other repairs, and a joined error hides none of them.
// Callers show the error and carry on: writes do not depend on these repairs,
// because each share operation heals its own account first.
func (s *Services) Prepare() error {
	var errs []error
	if err := s.Category.EnsurePaycheckCategories(); err != nil {
		errs = append(errs, fmt.Errorf("seed paycheck categories: %w", err))
	}
	collision, err := s.Category.EnsureValueAdjustmentCategory()
	s.ValueAdjustmentUserCollision = collision
	if err != nil {
		errs = append(errs, fmt.Errorf("seed the Value Adjustment category: %w", err))
	}
	if _, err := s.Investment.HealAllAccounts(); err != nil {
		errs = append(errs, fmt.Errorf("heal investment positions: %w", err))
	}
	if _, err := s.Scheduled.HealNextDates(); err != nil {
		errs = append(errs, fmt.Errorf("heal schedule dates: %w", err))
	}
	if _, err := s.Scheduled.HealTransferCategories(); err != nil {
		errs = append(errs, fmt.Errorf("heal transfer schedule categories: %w", err))
	}
	return errors.Join(errs...)
}

// investmentValuerAdapter adapts *investment.ValuationService to
// report.InvestmentValuer.
type investmentValuerAdapter struct {
	svc *investment.ValuationService
}

func (a *investmentValuerAdapter) GetAccountValuation(accountID types.ID, asOf types.Date) (*report.ValuationResult, error) {
	val, err := a.svc.GetAccountValuation(accountID, asOf, investment.ValuationOptions{})
	if err != nil {
		return nil, err
	}

	// Check if any holdings lack pricing data (using cost basis as estimate)
	hasMissingPrices := false
	for _, h := range val.Holdings {
		if !h.HasPricing {
			hasMissingPrices = true
			break
		}
	}

	return &report.ValuationResult{
		TotalValue:       val.TotalValue,
		CashBalance:      val.CashBalance,
		HasMissingPrices: hasMissingPrices,
	}, nil
}
