package account_test

import (
	"bytes"
	"strings"
	"testing"

	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/types"
)

func TestAccountAdd_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "add", "--name", "Checking", "--type", "checking"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(account add) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestAccountAdd_MissingName(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "add", "--file", dbPath, "--type", "checking"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(account add) without --name should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "name") {
		t.Errorf("expected Cobra required-flag error mentioning name, got: %v", err)
	}
}

func TestAccountAdd_MissingType(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "add", "--file", dbPath, "--name", "Checking"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(account add) without --type should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "type") {
		t.Errorf("expected Cobra required-flag error mentioning type, got: %v", err)
	}
}

func TestAccountAdd_InvalidType(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "add", "--file", dbPath, "--name", "Checking", "--type", "invalid_type"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(account add) with invalid type should return error")
	}
	if !strings.Contains(err.Error(), "invalid --type") {
		t.Errorf("expected handler error 'invalid --type', got: %v", err)
	}
}

func TestAccountAdd_InvalidOpeningBalance(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "add", "--file", dbPath, "--name", "Checking", "--type", "checking", "--opening-balance", "invalid"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(account add) with invalid opening-balance should return error")
	}
	if !strings.Contains(err.Error(), "invalid --opening-balance") {
		t.Errorf("expected 'invalid --opening-balance', got: %v", err)
	}
}

func TestAccountAdd_InvalidOpeningDate(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "add", "--file", dbPath, "--name", "Checking", "--type", "checking", "--opening-date", "invalid-date"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(account add) with invalid opening-date should return error")
	}
	if !strings.Contains(err.Error(), "invalid --opening-date") {
		t.Errorf("expected 'invalid --opening-date', got: %v", err)
	}
}

func TestAccountAdd_DuplicateName(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := accountdom.NewRepository(database)
	acct := accountdom.NewAccount("Checking", accountdom.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "add", "--file", dbPath, "--name", "Checking", "--type", "checking"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(account add) with duplicate name should return error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists', got: %v", err)
	}
}

func TestAccountAdd_CreditLimitOnNonCreditCard(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "add", "--file", dbPath, "--name", "Checking", "--type", "checking", "--credit-limit", "5000"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(account add) with --credit-limit on non-credit-card should return error")
	}
	if !strings.Contains(err.Error(), "only valid for credit_card") {
		t.Errorf("expected credit_card-only error, got: %v", err)
	}
}

func TestAccountAdd_InterestRateOnNonLoan(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "add", "--file", dbPath, "--name", "Checking", "--type", "checking", "--interest-rate", "5.5"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(account add) with --interest-rate on non-loan should return error")
	}
	if !strings.Contains(err.Error(), "only valid for loan") {
		t.Errorf("expected loan-only error, got: %v", err)
	}
}

func TestAccountAdd_Basic(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "add", "--file", dbPath, "--name", "My Checking", "--type", "checking"}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(account add): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, "Account created successfully") {
		t.Errorf("expected creation confirmation, got: %s", out)
	}
	if !strings.Contains(out, "My Checking") {
		t.Errorf("expected account name in output, got: %s", out)
	}
	if !strings.Contains(out, "USD") {
		t.Errorf("expected default currency, got: %s", out)
	}
	if !strings.Contains(out, "$0.00") {
		t.Errorf("expected default opening balance, got: %s", out)
	}

	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("post: db.Open: %v", err)
	}
	defer database.Close()
	acctRepo := accountdom.NewRepository(database)
	acct, err := acctRepo.GetByName("My Checking")
	if err != nil {
		t.Fatalf("expected account to be created: %v", err)
	}
	if acct.Name != "My Checking" {
		t.Errorf("name = %q, want %q", acct.Name, "My Checking")
	}
	if acct.Type != accountdom.TypeChecking {
		t.Errorf("type = %v, want %v", acct.Type, accountdom.TypeChecking)
	}
}

func TestAccountAdd_WithAllOptions(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"account", "add",
		"--file", dbPath,
		"--name", "Primary Checking",
		"--type", "checking",
		"--currency", "EUR",
		"--opening-balance", "1000.50",
		"--opening-date", "2024-01-15",
		"--institution", "Chase Bank",
		"--account-number", "1234567890",
		"--notes", "Primary account",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(account add): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"Primary Checking", "EUR", "1000.50", "2024-01-15", "Chase Bank", "1234567890", "Primary account"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got: %s", want, out)
		}
	}

	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("post: db.Open: %v", err)
	}
	defer database.Close()
	acctRepo := accountdom.NewRepository(database)
	acct, err := acctRepo.GetByName("Primary Checking")
	if err != nil {
		t.Fatalf("expected account to be created: %v", err)
	}
	if acct.Currency != "EUR" {
		t.Errorf("currency = %q, want EUR", acct.Currency)
	}
	if acct.OpeningDate.String() != "2024-01-15" {
		t.Errorf("opening date = %s, want 2024-01-15", acct.OpeningDate.String())
	}
	if !acct.Institution.Valid || acct.Institution.String != "Chase Bank" {
		t.Errorf("institution = %v, want Chase Bank", acct.Institution)
	}
	if !acct.AccountNumber.Valid || acct.AccountNumber.String != "1234567890" {
		t.Errorf("account number = %v, want 1234567890", acct.AccountNumber)
	}
	if !acct.Notes.Valid || acct.Notes.String != "Primary account" {
		t.Errorf("notes = %v, want 'Primary account'", acct.Notes)
	}
}

func TestAccountAdd_CreditCard(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"account", "add",
		"--file", dbPath,
		"--name", "Visa Card",
		"--type", "credit_card",
		"--credit-limit", "5000.00",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(account add): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, "Credit Limit") {
		t.Errorf("expected 'Credit Limit' label, got: %s", out)
	}
	if !strings.Contains(out, "$5000.00") {
		t.Errorf("expected '$5000.00', got: %s", out)
	}

	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("post: db.Open: %v", err)
	}
	defer database.Close()
	acctRepo := accountdom.NewRepository(database)
	acct, err := acctRepo.GetByName("Visa Card")
	if err != nil {
		t.Fatalf("expected account to be created: %v", err)
	}
	if !acct.CreditLimit.Valid {
		t.Error("credit limit should be set")
	}
	if acct.CreditLimit.Money.String() != "5000" {
		t.Errorf("credit limit = %s, want 5000", acct.CreditLimit.Money.String())
	}
}

func TestAccountAdd_Loan(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"account", "add",
		"--file", dbPath,
		"--name", "Car Loan",
		"--type", "loan",
		"--opening-balance", "-15000.00",
		"--interest-rate", "5.5",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(account add): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, "Interest Rate") {
		t.Errorf("expected 'Interest Rate' label, got: %s", out)
	}
	if !strings.Contains(out, "5.5%") {
		t.Errorf("expected '5.5%%', got: %s", out)
	}

	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("post: db.Open: %v", err)
	}
	defer database.Close()
	acctRepo := accountdom.NewRepository(database)
	acct, err := acctRepo.GetByName("Car Loan")
	if err != nil {
		t.Fatalf("expected account to be created: %v", err)
	}
	if !acct.InterestRate.Valid {
		t.Error("interest rate should be set")
	}
	if acct.InterestRate.Money.String() != "5.5" {
		t.Errorf("interest rate = %s, want 5.5", acct.InterestRate.Money.String())
	}
}

func TestAccountAdd_AllTypes(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	for _, acctType := range []string{"checking", "savings", "credit_card", "investment", "cash", "loan", "asset"} {
		t.Run(acctType, func(t *testing.T) {
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			err := cli.ExecuteWith([]string{
				"account", "add",
				"--file", dbPath,
				"--name", "Test " + acctType,
				"--type", acctType,
			}, stdout, stderr)
			if err != nil {
				t.Fatalf("cli.ExecuteWith(account add --type %s): %v\nstderr=%s", acctType, err, stderr)
			}
			if !strings.Contains(stdout.String(), "Account created successfully") {
				t.Errorf("expected creation confirmation for %s, got: %s", acctType, stdout.String())
			}
		})
	}
}

func TestAccountCmd_HelpListsAdd(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(account --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "add") {
		t.Errorf("expected `account --help` to list `add`; got:\n%s", stdout.String())
	}
}

func TestAccountAdd_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "add", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(account add --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "add") {
		t.Errorf("expected `account add --help` to describe the command; got:\n%s", stdout.String())
	}
}
