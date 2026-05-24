package undo_test

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// =============================================================================
// Test helpers for investment-transfer undo commands
// =============================================================================

type invUndoEnv struct {
	invSvc      *investment.Service
	txnSvc      *transaction.Service
	invRepo     *investment.Repository
	txnRepo     *transaction.Repository
	accountRepo *account.Repository
}

func createInvUndoEnv(t *testing.T) *invUndoEnv {
	t.Helper()
	database := createTestDB(t)

	invRepo := investment.NewRepository(database)
	accountRepo := account.NewRepository(database)
	positionRepo := investment.NewPositionRepository(database)
	lotRepo := investment.NewLotRepository(database)
	txnLotRepo := investment.NewTransactionLotRepository(database)
	priceRepo := price.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	caRepo := investment.NewCorporateActionRepository(database)

	invSvc := investment.NewService(invRepo, accountRepo, positionRepo, lotRepo, txnLotRepo, priceRepo, txnRepo, caRepo, database)

	// transaction.Service is only needed for environment parity; tests below
	// drive the investment service directly and inspect via repos.
	splitRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	txnSvc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

	return &invUndoEnv{
		invSvc:      invSvc,
		txnSvc:      txnSvc,
		invRepo:     invRepo,
		txnRepo:     txnRepo,
		accountRepo: accountRepo,
	}
}

func createInvAccountForUndo(t *testing.T, repo *account.Repository, name string) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("create investment account: %v", err)
	}
	return acct
}

func createCheckingAccountForUndo(t *testing.T, repo *account.Repository, name string) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("create checking account: %v", err)
	}
	return acct
}

// =============================================================================
// CreateInvestmentTransferCashCommand — inv→reg (cash leaves the investment account)
// =============================================================================

func TestCreateInvestmentTransferCashCommand_Roundtrip(t *testing.T) {
	env := createInvUndoEnv(t)
	inv := createInvAccountForUndo(t, env.accountRepo, "Brokerage")
	checking := createCheckingAccountForUndo(t, env.accountRepo, "Checking")
	amount := types.MustNewMoney("250.00")
	date := types.Today()

	cmd := undo.NewCreateInvestmentTransferCashCommand(env.invSvc, inv.ID, checking.ID, date, amount, "draw")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	result := cmd.Result()
	if result == nil {
		t.Fatal("Result() should not be nil after Execute")
	}
	if result.InvestmentTransaction == nil || result.RegularTransaction == nil {
		t.Fatal("Result should carry both investment and regular transactions")
	}
	if !result.InvestmentTransaction.TotalAmount.Equal(amount.Neg()) {
		t.Errorf("inv leg amount = %s, want %s", result.InvestmentTransaction.TotalAmount.String(), amount.Neg().String())
	}
	if !result.RegularTransaction.Amount.Equal(amount) {
		t.Errorf("reg leg amount = %s, want %s", result.RegularTransaction.Amount.String(), amount.String())
	}

	// Sanity: both rows exist.
	if _, err := env.invRepo.GetByID(result.InvestmentTransaction.ID); err != nil {
		t.Fatalf("inv leg should exist: %v", err)
	}
	if _, err := env.txnRepo.GetByID(result.RegularTransaction.ID); err != nil {
		t.Fatalf("reg leg should exist: %v", err)
	}

	if err := cmd.Undo(); err != nil {
		t.Fatalf("Undo() error = %v", err)
	}

	// Both rows gone.
	if _, err := env.invRepo.GetByID(result.InvestmentTransaction.ID); err == nil {
		t.Error("inv leg should be gone after undo")
	}
	if _, err := env.txnRepo.GetByID(result.RegularTransaction.ID); err == nil {
		t.Error("reg leg should be gone after undo")
	}
}

func TestCreateInvestmentTransferCashCommand_Description(t *testing.T) {
	cmd := undo.NewCreateInvestmentTransferCashCommand(nil, types.NewID(), types.NewID(), types.Today(), types.ZeroMoney, "")
	if cmd.Description() != "Create transfer" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Create transfer")
	}
}

// =============================================================================
// CreateInvestmentDepositCommand — reg→inv (cash arrives at the investment account)
// =============================================================================

func TestCreateInvestmentDepositCommand_Roundtrip(t *testing.T) {
	env := createInvUndoEnv(t)
	inv := createInvAccountForUndo(t, env.accountRepo, "Brokerage")
	checking := createCheckingAccountForUndo(t, env.accountRepo, "Checking")
	amount := types.MustNewMoney("750.00")
	date := types.Today()

	cmd := undo.NewCreateInvestmentDepositCommand(env.invSvc, inv.ID, checking.ID, date, amount, "fund")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	result := cmd.Result()
	if result == nil {
		t.Fatal("Result() should not be nil after Execute")
	}
	if !result.InvestmentTransaction.TotalAmount.Equal(amount) {
		t.Errorf("inv leg amount = %s, want %s", result.InvestmentTransaction.TotalAmount.String(), amount.String())
	}
	if !result.RegularTransaction.Amount.Equal(amount.Neg()) {
		t.Errorf("reg leg amount = %s, want %s", result.RegularTransaction.Amount.String(), amount.Neg().String())
	}

	if err := cmd.Undo(); err != nil {
		t.Fatalf("Undo() error = %v", err)
	}
	if _, err := env.invRepo.GetByID(result.InvestmentTransaction.ID); err == nil {
		t.Error("inv leg should be gone after undo")
	}
	if _, err := env.txnRepo.GetByID(result.RegularTransaction.ID); err == nil {
		t.Error("reg leg should be gone after undo")
	}
}

func TestCreateInvestmentDepositCommand_Description(t *testing.T) {
	cmd := undo.NewCreateInvestmentDepositCommand(nil, types.NewID(), types.NewID(), types.Today(), types.ZeroMoney, "")
	if cmd.Description() != "Create transfer" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Create transfer")
	}
}

// =============================================================================
// CreateInvestmentToInvestmentTransferCommand — inv↔inv (e.g. IRA→IRA rollover)
// =============================================================================

func TestCreateInvestmentToInvestmentTransferCommand_Roundtrip(t *testing.T) {
	env := createInvUndoEnv(t)
	src := createInvAccountForUndo(t, env.accountRepo, "E*Trade IRA")
	dst := createInvAccountForUndo(t, env.accountRepo, "Wealthfront IRA")
	amount := types.MustNewMoney("1500.00")
	date := types.Today()

	cmd := undo.NewCreateInvestmentToInvestmentTransferCommand(env.invSvc, src.ID, dst.ID, date, amount, "rollover")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	result := cmd.Result()
	if result == nil {
		t.Fatal("Result() should not be nil after Execute")
	}
	if !result.SourceTransaction.TotalAmount.Equal(amount.Neg()) {
		t.Errorf("source amount = %s, want %s", result.SourceTransaction.TotalAmount.String(), amount.Neg().String())
	}
	if !result.DestinationTransaction.TotalAmount.Equal(amount) {
		t.Errorf("dest amount = %s, want %s", result.DestinationTransaction.TotalAmount.String(), amount.String())
	}

	// Both legs must be present in the investment repo.
	if _, err := env.invRepo.GetByID(result.SourceTransaction.ID); err != nil {
		t.Fatalf("source leg should exist: %v", err)
	}
	if _, err := env.invRepo.GetByID(result.DestinationTransaction.ID); err != nil {
		t.Fatalf("destination leg should exist: %v", err)
	}

	if err := cmd.Undo(); err != nil {
		t.Fatalf("Undo() error = %v", err)
	}
	if _, err := env.invRepo.GetByID(result.SourceTransaction.ID); err == nil {
		t.Error("source leg should be gone after undo")
	}
	if _, err := env.invRepo.GetByID(result.DestinationTransaction.ID); err == nil {
		t.Error("destination leg should be gone after undo (no orphan)")
	}
}

func TestCreateInvestmentToInvestmentTransferCommand_Description(t *testing.T) {
	cmd := undo.NewCreateInvestmentToInvestmentTransferCommand(nil, types.NewID(), types.NewID(), types.Today(), types.ZeroMoney, "")
	if cmd.Description() != "Create transfer" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Create transfer")
	}
}
