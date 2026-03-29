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

	// Repositories (exposed for direct use by CLI/TUI when needed)
	AccountRepo        *account.Repository
	TransactionRepo    *transaction.Repository
	SplitRepo          *transaction.SplitRepository
	TransferRepo       *transaction.TransferRepository
	CategoryRepo       *category.Repository
	PayeeRepo          *payee.Repository
	ScheduledTxnRepo   *scheduled.Repository
	ReconciliationRepo *reconciliation.Repository
	SecurityRepo       *security.Repository
	PriceRepo          *price.Repository
	InvestmentRepo     *investment.Repository
	LotRepo            *investment.LotRepository
	PositionRepo       *investment.PositionRepository
	TransactionLotRepo *investment.TransactionLotRepository
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

	// Create services (inject cross-slice repo dependencies)
	accountSvc := account.NewService(accountRepo, database)
	categorySvc := category.NewService(categoryRepo, database)
	payeeSvc := payee.NewService(payeeRepo, database)
	securitySvc := security.NewService(securityRepo, database)

	txnSvc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, database)
	scheduledSvc := scheduled.NewService(scheduledRepo, txnRepo, database)
	reconciliationSvc := reconciliation.NewService(reconciliationRepo, txnRepo, accountRepo, database)
	reportSvc := report.NewService(accountRepo, database)
	priceSvc := price.NewService(priceRepo, securityRepo, database)
	investmentSvc := investment.NewService(investmentRepo, accountRepo, database)

	return &Services{
		Account:        accountSvc,
		Transaction:    txnSvc,
		Category:       categorySvc,
		Payee:          payeeSvc,
		Scheduled:      scheduledSvc,
		Report:         reportSvc,
		Reconciliation: reconciliationSvc,
		Security:       securitySvc,
		Price:          priceSvc,
		Investment:     investmentSvc,

		AccountRepo:        accountRepo,
		TransactionRepo:    txnRepo,
		SplitRepo:          splitRepo,
		TransferRepo:       transferRepo,
		CategoryRepo:       categoryRepo,
		PayeeRepo:          payeeRepo,
		ScheduledTxnRepo:   scheduledRepo,
		ReconciliationRepo: reconciliationRepo,
		SecurityRepo:       securityRepo,
		PriceRepo:          priceRepo,
		InvestmentRepo:     investmentRepo,
		LotRepo:            lotRepo,
		PositionRepo:       positionRepo,
		TransactionLotRepo: transactionLotRepo,
	}
}
