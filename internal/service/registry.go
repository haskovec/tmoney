package service

import (
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/repository"
)

// Services holds all application services, providing a single point of
// initialization shared between the CLI and TUI.
type Services struct {
	Account        *AccountService
	Transaction    *TransactionService
	Category       *CategoryService
	Payee          *PayeeService
	ScheduledTxn   *ScheduledTransactionService
	Report         *ReportService
	Reconciliation *ReconciliationService

	// Repositories exposed for cases where direct repo access is needed
	// (e.g. search criteria, category lookups in CLI).
	AccountRepo          *repository.AccountRepository
	TransactionRepo      *repository.TransactionRepository
	SplitRepo            *repository.SplitRepository
	TransferRepo         *repository.TransferRepository
	CategoryRepo         *repository.CategoryRepository
	PayeeRepo            *repository.PayeeRepository
	ScheduledTxnRepo     *repository.ScheduledTransactionRepository
	ReconciliationRepo   *repository.ReconciliationRepository
}

// NewServices creates all repositories and services from a database connection.
// This is the single source of truth for wiring up the application layer.
func NewServices(database *db.DB) *Services {
	// Create repositories
	accountRepo := repository.NewAccountRepository(database)
	transactionRepo := repository.NewTransactionRepository(database)
	splitRepo := repository.NewSplitRepository(database)
	transferRepo := repository.NewTransferRepository(database)
	categoryRepo := repository.NewCategoryRepository(database)
	payeeRepo := repository.NewPayeeRepository(database)
	scheduledRepo := repository.NewScheduledTransactionRepository(database)
	reconciliationRepo := repository.NewReconciliationRepository(database)

	// Create services
	accountSvc := NewAccountService(accountRepo, database)
	transactionSvc := NewTransactionService(transactionRepo, splitRepo, transferRepo, payeeRepo, database)
	categorySvc := NewCategoryService(categoryRepo, database)
	payeeSvc := NewPayeeService(payeeRepo, database)
	scheduledTxnSvc := NewScheduledTransactionService(scheduledRepo, transactionRepo, database)
	reportSvc := NewReportService(accountRepo, database)
	reconciliationSvc := NewReconciliationService(reconciliationRepo, transactionRepo, accountRepo, database)

	return &Services{
		Account:        accountSvc,
		Transaction:    transactionSvc,
		Category:       categorySvc,
		Payee:          payeeSvc,
		ScheduledTxn:   scheduledTxnSvc,
		Report:         reportSvc,
		Reconciliation: reconciliationSvc,

		AccountRepo:        accountRepo,
		TransactionRepo:    transactionRepo,
		SplitRepo:          splitRepo,
		TransferRepo:       transferRepo,
		CategoryRepo:       categoryRepo,
		PayeeRepo:          payeeRepo,
		ScheduledTxnRepo:   scheduledRepo,
		ReconciliationRepo: reconciliationRepo,
	}
}
