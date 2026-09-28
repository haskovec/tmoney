package clitest

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
)

// SetupTransferAccounts creates a temp DB with a Checking and a Savings account
// (the reg↔reg transfer fixture), closes it, and returns the path plus the two
// account records.
//
// It is shared by the transfer noun's internal white-box test (package
// transfer) and its external black-box tests (package transfer_test), which is
// why it lives in clitest rather than a single test file.
func SetupTransferAccounts(t *testing.T) (string, *account.Account, *account.Account) {
	t.Helper()
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	repo := account.NewRepository(database)

	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.NewDate(2000, time.January, 1))
	if err := repo.Create(checking); err != nil {
		t.Fatalf("setup: create checking: %v", err)
	}
	savings := account.NewAccount("Savings", account.TypeSavings, "USD", types.MustNewMoney("500.00"), types.NewDate(2000, time.January, 1))
	if err := repo.Create(savings); err != nil {
		t.Fatalf("setup: create savings: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("setup: db.Close: %v", err)
	}
	return dbPath, checking, savings
}

// SetupTransferDispatchAccounts seeds a temp DB with one account of every type
// that the four transfer-dispatch paths exercise: a Checking (reg), two
// investment accounts (Brokerage, Rollover IRA), and an invested HSA. It closes the DB
// and returns the path plus the four account references.
func SetupTransferDispatchAccounts(t *testing.T) (string, *account.Account, *account.Account, *account.Account, *account.Account) {
	t.Helper()
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	repo := account.NewRepository(database)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("10000.00"), types.Today())
	if err := repo.Create(checking); err != nil {
		t.Fatalf("setup: create checking: %v", err)
	}
	brokerage := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := repo.Create(brokerage); err != nil {
		t.Fatalf("setup: create brokerage: %v", err)
	}
	ira := account.NewAccount("Rollover IRA", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := repo.Create(ira); err != nil {
		t.Fatalf("setup: create ira: %v", err)
	}
	hsa := account.NewAccount("HSA", account.TypeHSAInvestment, "USD", types.ZeroMoney, types.Today())
	if err := repo.Create(hsa); err != nil {
		t.Fatalf("setup: create hsa: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("setup: db.Close: %v", err)
	}
	return dbPath, checking, brokerage, ira, hsa
}

// OpenSvc opens the application services for a database file and registers a
// cleanup that closes the underlying connection.
//
// It opens the DB and builds services directly (db.Open + app.NewServices +
// Prepare) rather than routing through cmdutil.OpenServices, because clitest
// must stay free of internal/cli (D5/R2). The returned *app.Services is
// identical; only cmdutil.OpenServices' incidental side effects (recent-files
// config write, auto-post of due scheduled transactions) are skipped — both
// are no-ops for these fixtures, which never seed scheduled transactions.
func OpenSvc(t *testing.T, dbPath string) *app.Services {
	t.Helper()
	svc, _ := OpenSvcDB(t, dbPath)
	return svc
}

// OpenSvcDB is OpenSvc that also returns the open database, for a test that
// checks stored state through a repository it builds itself. app.Services
// exports no repositories.
func OpenSvcDB(t *testing.T, dbPath string) (*app.Services, *db.DB) {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("OpenSvc: db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	// Open the way the CLI does (cmdutil.OpenServices): wire, then run the
	// open-time repairs, which seed the system categories and heal stored
	// positions.
	svc := app.NewServices(database)
	if err := svc.Prepare(); err != nil {
		t.Fatalf("OpenSvc: Prepare: %v", err)
	}
	return svc, database
}

// FindInvestmentLegForTest returns the ID of the single investment transaction
// in the given investment account.
func FindInvestmentLegForTest(t *testing.T, svc *app.Services, invAcctID types.ID) types.ID {
	t.Helper()
	rows, err := svc.InvestmentRepo.ListByAccount(invAcctID, investment.TransactionFilter{})
	if err != nil {
		t.Fatalf("list investment txns: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 investment row, got %d", len(rows))
	}
	return rows[0].ID
}
