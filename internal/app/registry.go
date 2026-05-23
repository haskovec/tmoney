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
	"github.com/haskovec/tmoney/internal/transferlink"
	"github.com/haskovec/tmoney/internal/types"
)

// Services is the central registry for all application services and repositories.
// This is the single source of truth for wiring up the application layer.
type Services struct {
	// Services
	Account         *account.Service
	Transaction     *transaction.Service
	Category        *category.Service
	Payee           *payee.Service
	Scheduled       *scheduled.Service
	Report          *report.Service
	Reconciliation  *reconciliation.Service
	Security        *security.Service
	Price           *price.Service
	Investment      *investment.Service
	CorporateAction *investment.CorporateActionService
	TransferLink    *transferlink.Service

	// Repositories (exposed for direct use by CLI/TUI when needed)
	AccountRepo         *account.Repository
	TransactionRepo     *transaction.Repository
	SplitRepo           *transaction.SplitRepository
	TransferRepo        *transaction.TransferRepository
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
	transferRepo := transaction.NewTransferRepository(database, txnRepo)

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
	payeeSvc := payee.NewService(payeeRepo, database)
	securitySvc := security.NewService(securityRepo, database,
		security.WithLotChecker(lotRepo),
		security.WithPositionChecker(positionRepo),
	)

	txnSvc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, database)
	scheduledSvc := scheduled.NewService(scheduledRepo, txnRepo, txnSvc, database)
	// Heal any schedule rows poisoned by older binaries that updated
	// StartDate without syncing NextDate. Best-effort, mirrors the
	// HealAllAccounts precedent below.
	_, _ = scheduledSvc.HealNextDates()
	reconciliationSvc := reconciliation.NewService(reconciliationRepo, txnRepo, accountRepo, database)
	priceSvc := price.NewService(priceRepo, securityRepo, database)
	investmentSvc := investment.NewService(investmentRepo, accountRepo, positionRepo, lotRepo, transactionLotRepo, priceRepo, txnRepo, corporateActionRepo, database)
	// Silently heal any desynced positions/lots so the user doesn't have to
	// run rebuild-positions manually after upgrading. This is a no-op on
	// databases that contain corporate-action records.
	_, _ = investmentSvc.HealAllAccounts()
	reportSvc := report.NewService(accountRepo, database, report.WithInvestmentValuer(&investmentValuerAdapter{svc: investmentSvc}))
	corporateActionSvc := investment.NewCorporateActionService(corporateActionRepo, lotRepo, positionRepo, priceRepo, investmentRepo, securityRepo, database)
	transferLinkSvc := transferlink.NewService(txnRepo, transferRepo, splitRepo, accountRepo)

	return &Services{
		Account:         accountSvc,
		Transaction:     txnSvc,
		Category:        categorySvc,
		Payee:           payeeSvc,
		Scheduled:       scheduledSvc,
		Report:          reportSvc,
		Reconciliation:  reconciliationSvc,
		Security:        securitySvc,
		Price:           priceSvc,
		Investment:      investmentSvc,
		CorporateAction: corporateActionSvc,
		TransferLink:    transferLinkSvc,

		AccountRepo:         accountRepo,
		TransactionRepo:     txnRepo,
		SplitRepo:           splitRepo,
		TransferRepo:        transferRepo,
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
	}
}

// investmentValuerAdapter adapts *investment.Service to report.InvestmentValuer.
type investmentValuerAdapter struct {
	svc *investment.Service
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
