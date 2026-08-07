package app

import (
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
	CorporateAction     *investment.CorporateActionService
	TransferLink        *transferlink.Service

	// Transfer owns whole-transaction cash transfers across both ledgers —
	// bank↔bank, bank↔investment and investment↔investment alike. It is the
	// single door for creating, editing, voiding and deleting them; callers
	// must not reach past it into transaction.Service or investment.Service
	// for transfer work.
	Transfer *transfer.Service

	// Repositories (exposed for direct use by CLI/TUI when needed)
	AccountRepo         *account.Repository
	TransactionRepo     *transaction.Repository
	SplitRepo           *transaction.SplitRepository
	CategoryRepo        *category.Repository
	PayeeRepo           *payee.Repository
	ScheduledTxnRepo    *scheduled.Repository
	ReconciliationRepo  *reconciliation.Repository
	SecurityRepo        *security.Repository
	PriceRepo           *price.Repository
	InvestmentRepo      *investment.Repository
	LotRepo             *investment.LotRepository
	PositionRepo        *investment.PositionRepository
	TransactionLotRepo  *investment.TransactionLotRepository
	CorporateActionRepo *investment.CorporateActionRepository

	// ValueAdjustmentUserCollision is true when a *user* (non-system)
	// category named "Value Adjustment" already exists, so the system
	// category could not be seeded on open. The TUI surfaces a one-time
	// notice; the CLI ignores it.
	ValueAdjustmentUserCollision bool
}

// NewServices creates all repositories and services with proper dependency wiring.
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
	accountSvc := account.NewService(accountRepo, database)
	categorySvc := category.NewService(categoryRepo, database)
	// Seed paycheck-wizard categories on every open so existing
	// databases gain them automatically; best-effort, matches the
	// HealAllAccounts precedent below.
	_ = categorySvc.EnsurePaycheckCategories()
	// Seed the system Value Adjustment category on every open (same
	// best-effort rationale). A collision with a pre-existing user
	// category is surfaced to the TUI as a one-time notice.
	valueAdjustmentCollision, _ := categorySvc.EnsureValueAdjustmentCategory()
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
	// Silently heal any desynced positions/lots so the user doesn't have to
	// run rebuild-positions manually after upgrading. This is a no-op on
	// databases that contain corporate-action records.
	_, _ = investmentSvc.HealAllAccounts()

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
	// Heal any schedule rows poisoned by older binaries that updated
	// StartDate without syncing NextDate. Best-effort, mirrors the
	// HealAllAccounts precedent above.
	_, _ = scheduledSvc.HealNextDates()
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
		CorporateAction:     corporateActionSvc,
		TransferLink:        transferLinkSvc,
		Transfer:            transferSvc,

		AccountRepo:         accountRepo,
		TransactionRepo:     txnRepo,
		SplitRepo:           splitRepo,
		CategoryRepo:        categoryRepo,
		PayeeRepo:           payeeRepo,
		ScheduledTxnRepo:    scheduledRepo,
		ReconciliationRepo:  reconciliationRepo,
		SecurityRepo:        securityRepo,
		PriceRepo:           priceRepo,
		InvestmentRepo:      investmentRepo,
		LotRepo:             lotRepo,
		PositionRepo:        positionRepo,
		TransactionLotRepo:  transactionLotRepo,
		CorporateActionRepo: corporateActionRepo,

		ValueAdjustmentUserCollision: valueAdjustmentCollision,
	}
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
		HasMissingPrices: hasMissingPrices,
	}, nil
}
