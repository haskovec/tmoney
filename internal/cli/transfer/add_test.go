package transfer_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestTransferAdd_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transfer", "add", "--from", "Checking", "--to", "Savings", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestTransferAdd_MissingFrom(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transfer", "add", "--file", dbPath, "--to", "Savings", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) without --from should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "from") {
		t.Errorf("expected Cobra required-flag error mentioning from, got: %v", err)
	}
}

func TestTransferAdd_MissingTo(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transfer", "add", "--file", dbPath, "--from", "Checking", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) without --to should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "to") {
		t.Errorf("expected Cobra required-flag error mentioning to, got: %v", err)
	}
}

func TestTransferAdd_MissingAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transfer", "add", "--file", dbPath, "--from", "Checking", "--to", "Savings"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) without --amount should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "amount") {
		t.Errorf("expected Cobra required-flag error mentioning amount, got: %v", err)
	}
}

func TestTransferAdd_InvalidAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transfer", "add", "--file", dbPath, "--from", "Checking", "--to", "Savings", "--amount", "not-a-number"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) with invalid amount should return error")
	}
	if !strings.Contains(err.Error(), "invalid --amount") {
		t.Errorf("expected 'invalid --amount', got: %v", err)
	}
}

func TestTransferAdd_NegativeAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transfer", "add", "--file", dbPath, "--from", "Checking", "--to", "Savings", "--amount", "-100"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) with negative amount should return error")
	}
	if !strings.Contains(err.Error(), "must be positive") {
		t.Errorf("expected 'must be positive', got: %v", err)
	}
}

func TestTransferAdd_SourceAccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transfer", "add", "--file", dbPath, "--from", "Nonexistent", "--to", "Savings", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) with nonexistent source account should return error")
	}
	if !strings.Contains(err.Error(), "source account") || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'source account ... not found', got: %v", err)
	}
}

func TestTransferAdd_DestAccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	repo := account.NewRepository(database)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := repo.Create(checking); err != nil {
		t.Fatalf("setup: create checking: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"transfer", "add", "--file", dbPath, "--from", "Checking", "--to", "Nonexistent", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) with nonexistent destination account should return error")
	}
	if !strings.Contains(err.Error(), "destination account") || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'destination account ... not found', got: %v", err)
	}
}

// assertTransferLegsExist opens the DB and asserts that exactly one
// transaction exists in the regular `transactions` table for the regular
// account (if non-nil), one investment row exists in the
// `investment_transactions` table for each investment account in invAccts,
// and that all legs share the same transfer_id.
func assertTransferLegsExist(t *testing.T, dbPath string, regAcct *account.Account, invAccts []*account.Account, expectAmount string) {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("post: db.Open: %v", err)
	}
	defer database.Close()

	var transferID types.ID
	txnRepo := transaction.NewRepository(database)
	if regAcct != nil {
		regs, err := txnRepo.ListByAccount(regAcct.ID)
		if err != nil {
			t.Fatalf("list reg txns: %v", err)
		}
		if len(regs) != 1 {
			t.Fatalf("expected 1 regular leg in %s, got %d", regAcct.Name, len(regs))
		}
		if !regs[0].IsTransfer() {
			t.Errorf("regular leg in %s is not a transfer", regAcct.Name)
		}
		transferID = regs[0].TransferID.ID
	}

	invRepo := investment.NewRepository(database)
	for _, acct := range invAccts {
		rows, err := invRepo.ListByAccount(acct.ID, investment.TransactionFilter{})
		if err != nil {
			t.Fatalf("list inv txns for %s: %v", acct.Name, err)
		}
		var legs []*investment.Transaction
		for _, r := range rows {
			if r.IsTransfer() {
				legs = append(legs, r)
			}
		}
		if len(legs) != 1 {
			t.Fatalf("expected 1 transfer-cash leg in %s, got %d (rows=%d)", acct.Name, len(legs), len(rows))
		}
		if transferID == (types.ID{}) {
			transferID = legs[0].TransferID.ID
		} else if legs[0].TransferID.ID != transferID {
			t.Errorf("transfer_id mismatch on %s leg: got %s, want %s", acct.Name, legs[0].TransferID.ID, transferID)
		}
	}

	if expectAmount != "" {
		want := types.MustNewMoney(expectAmount)
		if regAcct != nil {
			regs, _ := txnRepo.ListByAccount(regAcct.ID)
			if !regs[0].Amount.Abs().Equal(want.Abs()) {
				t.Errorf("regular leg amount = %s, want abs %s", regs[0].Amount, want.Abs())
			}
		}
		for _, acct := range invAccts {
			rows, _ := invRepo.ListByAccount(acct.ID, investment.TransactionFilter{})
			for _, r := range rows {
				if r.IsTransfer() && !r.TotalAmount.Abs().Equal(want.Abs()) {
					t.Errorf("inv leg amount = %s, want abs %s", r.TotalAmount, want.Abs())
				}
			}
		}
	}
}

func TestTransferAdd_DispatchRegToReg_CreatesPair(t *testing.T) {
	dbPath, _, _ := clitest.SetupTransferAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transfer", "add", "--file", dbPath, "--from", "Checking", "--to", "Savings", "--amount", "75.00"}, stdout, stderr)
	if err != nil {
		t.Fatalf("transfer add reg→reg: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "Transfer created successfully") {
		t.Errorf("expected success line, got: %s", stdout.String())
	}
	// Open and verify both regular-side legs exist with same transfer_id.
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()
	acctRepo := account.NewRepository(database)
	checking, _ := acctRepo.GetByName("Checking")
	savings, _ := acctRepo.GetByName("Savings")
	txnRepo := transaction.NewRepository(database)
	src, _ := txnRepo.ListByAccount(checking.ID)
	dst, _ := txnRepo.ListByAccount(savings.ID)
	if len(src) != 1 || len(dst) != 1 {
		t.Fatalf("expected one leg per account, got src=%d dst=%d", len(src), len(dst))
	}
	if src[0].TransferID.ID != dst[0].TransferID.ID {
		t.Errorf("transfer_id mismatch reg→reg")
	}
}

func TestTransferAdd_DispatchRegToInv_CreatesPair(t *testing.T) {
	dbPath, checking, brokerage, _, _ := clitest.SetupTransferDispatchAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transfer", "add", "--file", dbPath, "--from", "Checking", "--to", "Brokerage", "--amount", "500.00"}, stdout, stderr)
	if err != nil {
		t.Fatalf("transfer add reg→inv: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "Transfer created successfully") {
		t.Errorf("expected success line, got: %s", stdout.String())
	}
	assertTransferLegsExist(t, dbPath, checking, []*account.Account{brokerage}, "500.00")
}

func TestTransferAdd_DispatchInvToReg_CreatesPair(t *testing.T) {
	dbPath, checking, brokerage, _, _ := clitest.SetupTransferDispatchAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transfer", "add", "--file", dbPath, "--from", "Brokerage", "--to", "Checking", "--amount", "250.00"}, stdout, stderr)
	if err != nil {
		t.Fatalf("transfer add inv→reg: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "Transfer created successfully") {
		t.Errorf("expected success line, got: %s", stdout.String())
	}
	assertTransferLegsExist(t, dbPath, checking, []*account.Account{brokerage}, "250.00")
}

func TestTransferAdd_DispatchInvToInv_CreatesPair(t *testing.T) {
	dbPath, _, brokerage, ira, _ := clitest.SetupTransferDispatchAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transfer", "add", "--file", dbPath, "--from", "Brokerage", "--to", "Rollover IRA", "--amount", "1000.00"}, stdout, stderr)
	if err != nil {
		t.Fatalf("transfer add inv→inv: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "Transfer created successfully") {
		t.Errorf("expected success line, got: %s", stdout.String())
	}
	assertTransferLegsExist(t, dbPath, nil, []*account.Account{brokerage, ira}, "1000.00")
}

// HSA accounts satisfy IsInvestmentType, so HSA on either leg routes
// via the investment-side dispatch paths.
func TestTransferAdd_HSACountsAsInvestment(t *testing.T) {
	cases := []struct {
		name string
		from string
		to   string
	}{
		{"reg→hsa", "Checking", "HSA"},
		{"hsa→reg", "HSA", "Checking"},
		{"hsa→inv", "HSA", "Brokerage"},
		{"inv→hsa", "Brokerage", "HSA"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dbPath, checking, brokerage, _, hsa := clitest.SetupTransferDispatchAccounts(t)

			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			err := cli.ExecuteWith([]string{"transfer", "add", "--file", dbPath, "--from", tc.from, "--to", tc.to, "--amount", "100.00"}, stdout, stderr)
			if err != nil {
				t.Fatalf("transfer add %s: %v\nstderr=%s", tc.name, err, stderr)
			}
			// Verify legs landed in the right tables.
			var regAcct *account.Account
			invAccts := []*account.Account{}
			for _, side := range []string{tc.from, tc.to} {
				switch side {
				case "Checking":
					regAcct = checking
				case "HSA":
					invAccts = append(invAccts, hsa)
				case "Brokerage":
					invAccts = append(invAccts, brokerage)
				}
			}
			assertTransferLegsExist(t, dbPath, regAcct, invAccts, "100.00")
		})
	}
}

func TestTransferAdd_PrintsIDsInConfirmation(t *testing.T) {
	dbPath, _, _ := clitest.SetupTransferAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transfer", "add", "--file", dbPath, "--from", "Checking", "--to", "Savings", "--amount", "42.00"}, stdout, stderr)
	if err != nil {
		t.Fatalf("transfer add: %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	for _, want := range []string{"Transfer ID:", "From transaction ID:", "To transaction ID:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got: %s", want, out)
		}
	}
}

func TestTransferAdd_InvalidDate(t *testing.T) {
	dbPath, _, _ := clitest.SetupTransferAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"transfer", "add",
		"--file", dbPath,
		"--from", "Checking",
		"--to", "Savings",
		"--amount", "100",
		"--date", "not-a-date",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) with invalid date should return error")
	}
	if !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected 'invalid --date', got: %v", err)
	}
}

func TestTransferAdd_Basic(t *testing.T) {
	dbPath, _, _ := clitest.SetupTransferAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transfer", "add", "--file", dbPath, "--from", "Checking", "--to", "Savings", "--amount", "100.00"}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(transfer add): %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	for _, want := range []string{"Transfer created successfully", "Checking", "Savings", "$100.00"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got: %s", want, out)
		}
	}
}

func TestTransferAdd_WithDateAndMemo(t *testing.T) {
	dbPath, _, _ := clitest.SetupTransferAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"transfer", "add",
		"--file", dbPath,
		"--from", "Checking",
		"--to", "Savings",
		"--amount", "250.50",
		"--date", "2024-06-15",
		"--memo", "Monthly savings",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(transfer add): %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	for _, want := range []string{"Transfer created successfully", "2024-06-15", "Monthly savings", "$250.50"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got: %s", want, out)
		}
	}
}

func TestTransferAdd_VerifyTransactions(t *testing.T) {
	dbPath, checking, savings := clitest.SetupTransferAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"transfer", "add",
		"--file", dbPath,
		"--from", "Checking",
		"--to", "Savings",
		"--amount", "100.00",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(transfer add): %v\nstderr=%s", err, stderr)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("post: db.Open: %v", err)
	}
	defer database.Close()

	txnRepo := transaction.NewRepository(database)

	checkingTxns, err := txnRepo.ListByAccount(checking.ID)
	if err != nil {
		t.Fatalf("list checking transactions: %v", err)
	}
	if len(checkingTxns) != 1 {
		t.Fatalf("expected 1 checking transaction, got %d", len(checkingTxns))
	}
	if !checkingTxns[0].Amount.Equal(types.MustNewMoney("-100.00")) {
		t.Errorf("checking amount = %s, want -100.00", checkingTxns[0].Amount.String())
	}
	if !checkingTxns[0].IsTransfer() {
		t.Error("checking transaction should be a transfer")
	}

	savingsTxns, err := txnRepo.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("list savings transactions: %v", err)
	}
	if len(savingsTxns) != 1 {
		t.Fatalf("expected 1 savings transaction, got %d", len(savingsTxns))
	}
	if !savingsTxns[0].Amount.Equal(types.MustNewMoney("100.00")) {
		t.Errorf("savings amount = %s, want 100.00", savingsTxns[0].Amount.String())
	}
	if !savingsTxns[0].IsTransfer() {
		t.Error("savings transaction should be a transfer")
	}
	if checkingTxns[0].TransferID.ID != savingsTxns[0].TransferID.ID {
		t.Error("both transactions should share the same transfer ID")
	}
}

func TestTransferCmd_HelpListsAdd(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transfer", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(transfer --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "add") {
		t.Errorf("expected `transfer --help` to list `add`; got:\n%s", stdout.String())
	}
}

func TestTransferAdd_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transfer", "add", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(transfer add --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "add") {
		t.Errorf("expected `transfer add --help` to describe the command; got:\n%s", stdout.String())
	}
}
