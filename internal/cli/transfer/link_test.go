package transfer_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// setupLinkScenario creates a temp database with two accounts and the
// given transactions, then closes the database so the CLI can re-open
// it. The returned tuple is (dbPath, checking, savings).
func setupLinkScenario(t *testing.T, txns ...*transaction.Transaction) (string, *account.Account, *account.Account) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}

	acctRepo := account.NewRepository(database)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("0.00"), types.Today())
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("setup: create checking: %v", err)
	}
	savings := account.NewAccount("Savings", account.TypeSavings, "USD", types.MustNewMoney("0.00"), types.Today())
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("setup: create savings: %v", err)
	}

	txnRepo := transaction.NewRepository(database)
	for _, txn := range txns {
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("setup: create txn: %v", err)
		}
	}

	database.Close()
	return dbPath, checking, savings
}

func TestTransferLink_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transfer", "link"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer link) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestTransferLink_PreviewEmpty(t *testing.T) {
	dbPath, _, _ := setupLinkScenario(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transfer", "link", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(transfer link): %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	for _, want := range []string{"LINK TRANSFERS PREVIEW", "Scanned:   0", "Clean:     0", "Nothing to link"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestTransferLink_PreviewWithCleanPair(t *testing.T) {
	date := types.MustParseDate("2024-06-01")
	dbPath, checking, savings := setupLinkScenario(t)

	// Reopen, write a matching pair, close.
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("post: db.Open: %v", err)
	}
	txnRepo := transaction.NewRepository(database)
	out := transaction.NewTransaction(checking.ID, date, types.MustNewMoney("-100.00"))
	in := transaction.NewTransaction(savings.ID, date, types.MustNewMoney("100.00"))
	if err := txnRepo.Create(out); err != nil {
		t.Fatalf("create out txn: %v", err)
	}
	if err := txnRepo.Create(in); err != nil {
		t.Fatalf("create in txn: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transfer", "link", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(transfer link): %v\nstderr=%s", err, stderr)
	}
	o := stdout.String()
	for _, want := range []string{"LINK TRANSFERS PREVIEW", "Clean:     1", "Checking", "Savings", "Run with --confirm"} {
		if !strings.Contains(o, want) {
			t.Errorf("expected %q in output, got:\n%s", want, o)
		}
	}
}

func TestTransferLink_ConfirmExecutes(t *testing.T) {
	date := types.MustParseDate("2024-06-01")
	dbPath, checking, savings := setupLinkScenario(t)

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("post: db.Open: %v", err)
	}
	txnRepo := transaction.NewRepository(database)
	out := transaction.NewTransaction(checking.ID, date, types.MustNewMoney("-100.00"))
	in := transaction.NewTransaction(savings.ID, date, types.MustNewMoney("100.00"))
	if err := txnRepo.Create(out); err != nil {
		t.Fatalf("create out txn: %v", err)
	}
	if err := txnRepo.Create(in); err != nil {
		t.Fatalf("create in txn: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transfer", "link", "--file", dbPath, "--confirm"}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(transfer link --confirm): %v\nstderr=%s", err, stderr)
	}
	o := stdout.String()
	for _, want := range []string{"LINK TRANSFERS COMPLETE", "Linked:    1 pairs", "Ambiguous: 0 pairs"} {
		if !strings.Contains(o, want) {
			t.Errorf("expected %q in output, got:\n%s", want, o)
		}
	}

	// Verify both transactions are now linked transfers.
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("verify: db.Open: %v", err)
	}
	defer database.Close()
	verifyRepo := transaction.NewRepository(database)
	checkingTxns, err := verifyRepo.ListByAccount(checking.ID)
	if err != nil {
		t.Fatalf("list checking: %v", err)
	}
	savingsTxns, err := verifyRepo.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("list savings: %v", err)
	}
	if len(checkingTxns) != 1 || len(savingsTxns) != 1 {
		t.Fatalf("expected 1 transaction per account, got checking=%d savings=%d",
			len(checkingTxns), len(savingsTxns))
	}
	if !checkingTxns[0].IsTransfer() {
		t.Error("checking txn should now be a transfer after link")
	}
	if !savingsTxns[0].IsTransfer() {
		t.Error("savings txn should now be a transfer after link")
	}
	if checkingTxns[0].TransferID.ID != savingsTxns[0].TransferID.ID {
		t.Error("transfer IDs should match after link")
	}
}

func TestTransferLink_MaxDaysWindowExcludes(t *testing.T) {
	dbPath, checking, savings := setupLinkScenario(t)

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("post: db.Open: %v", err)
	}
	txnRepo := transaction.NewRepository(database)
	// Pair posted 4 days apart — within default 5 but outside --max-days 2.
	out := transaction.NewTransaction(checking.ID, types.MustParseDate("2024-06-01"), types.MustNewMoney("-100.00"))
	in := transaction.NewTransaction(savings.ID, types.MustParseDate("2024-06-05"), types.MustNewMoney("100.00"))
	if err := txnRepo.Create(out); err != nil {
		t.Fatalf("create out: %v", err)
	}
	if err := txnRepo.Create(in); err != nil {
		t.Fatalf("create in: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transfer", "link", "--file", dbPath, "--max-days", "2"}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(transfer link --max-days=2): %v\nstderr=%s", err, stderr)
	}
	o := stdout.String()
	if !strings.Contains(o, "window: 2 days") {
		t.Errorf("expected window: 2 days in output, got:\n%s", o)
	}
	if !strings.Contains(o, "Clean:     0") {
		t.Errorf("expected zero clean pairs with narrow window, got:\n%s", o)
	}
	if !strings.Contains(o, "Nothing to link") {
		t.Errorf("expected 'Nothing to link', got:\n%s", o)
	}
}

func TestTransferLink_AmbiguousPairs(t *testing.T) {
	dbPath, checking, savings := setupLinkScenario(t)

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("post: db.Open: %v", err)
	}
	txnRepo := transaction.NewRepository(database)
	// One outgoing $100 from checking matched by two incoming $100 deposits
	// in savings on the same day — ambiguous.
	date := types.MustParseDate("2024-06-01")
	out := transaction.NewTransaction(checking.ID, date, types.MustNewMoney("-100.00"))
	in1 := transaction.NewTransaction(savings.ID, date, types.MustNewMoney("100.00"))
	in2 := transaction.NewTransaction(savings.ID, date, types.MustNewMoney("100.00"))
	if err := txnRepo.Create(out); err != nil {
		t.Fatalf("create out: %v", err)
	}
	if err := txnRepo.Create(in1); err != nil {
		t.Fatalf("create in1: %v", err)
	}
	if err := txnRepo.Create(in2); err != nil {
		t.Fatalf("create in2: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transfer", "link", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(transfer link): %v\nstderr=%s", err, stderr)
	}
	o := stdout.String()
	for _, want := range []string{"Clean:     0", "Ambiguous: 2", "Ambiguous pairs:", "Nothing to link"} {
		if !strings.Contains(o, want) {
			t.Errorf("expected %q in output, got:\n%s", want, o)
		}
	}

	// Confirm pass leaves the database untouched: nothing should be linked
	// because there are no clean pairs to act on.
	stdout.Reset()
	stderr.Reset()
	err = cli.ExecuteWith([]string{"transfer", "link", "--file", dbPath, "--confirm"}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(transfer link --confirm): %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "Linked:    0 pairs") {
		t.Errorf("expected zero linked when ambiguous, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Ambiguous: 2 pairs") {
		t.Errorf("expected 2 ambiguous pairs reported, got:\n%s", stdout.String())
	}
}

func TestTransferLink_InvalidMaxDays(t *testing.T) {
	dbPath, _, _ := setupLinkScenario(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transfer", "link", "--file", dbPath, "--max-days", "-1"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer link --max-days=-1) should return error")
	}
	if !strings.Contains(err.Error(), "max-days") {
		t.Errorf("expected error to mention max-days, got: %v", err)
	}
}

func TestTransferCmd_HelpListsLink(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transfer", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(transfer --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "link") {
		t.Errorf("expected `transfer --help` to list `link`; got:\n%s", stdout.String())
	}
}

func TestTransferLink_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transfer", "link", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(transfer link --help): %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"link", "max-days", "confirm"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in `transfer link --help` output; got:\n%s", want, out)
		}
	}
}
