package account_test

import (
	"bytes"
	"strings"
	"testing"

	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// seedAccount creates an account in a fresh test database and returns the db
// path. The caller mutates the returned account before it is created.
func seedAccount(t *testing.T, acct *accountdom.Account) (dbPath string) {
	t.Helper()
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	repo := accountdom.NewRepository(database)
	if err := repo.Create(acct); err != nil {
		t.Fatalf("setup account %q: %v", acct.Name, err)
	}
	database.Close()
	return dbPath
}

// reloadAccount reopens the database and fetches the account by name.
func reloadAccount(t *testing.T, dbPath, name string) *accountdom.Account {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer database.Close()
	acct, err := accountdom.NewRepository(database).GetByName(name)
	if err != nil {
		t.Fatalf("reload account %q: %v", name, err)
	}
	return acct
}

func runEdit(t *testing.T, args ...string) (string, error) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith(append([]string{"account", "edit"}, args...), stdout, stderr)
	return stdout.String(), err
}

func newChecking(name string) *accountdom.Account {
	return accountdom.NewAccount(name, accountdom.TypeChecking, "USD",
		types.MustNewMoney("0"), types.MustParseDate("2020-01-01"))
}

func TestAccountEdit_MissingName(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()
	_, err := runEdit(t, "--file", dbPath, "--new-name", "X")
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("expected required --name error, got %v", err)
	}
}

func TestAccountEdit_UnknownAccount(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()
	_, err := runEdit(t, "--file", dbPath, "--name", "Nope", "--new-name", "X")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

func TestAccountEdit_NoEditableFlag(t *testing.T) {
	dbPath := seedAccount(t, newChecking("Checking"))
	_, err := runEdit(t, "--file", dbPath, "--name", "Checking")
	if err == nil || !strings.Contains(err.Error(), "at least one editable flag") {
		t.Fatalf("expected no-editable-flag error, got %v", err)
	}
}

func TestAccountEdit_RenamePersists(t *testing.T) {
	dbPath := seedAccount(t, newChecking("Checking"))
	out, err := runEdit(t, "--file", dbPath, "--name", "Checking", "--new-name", "Main Checking")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if !strings.Contains(out, "Main Checking") {
		t.Errorf("expected new name in output, got %s", out)
	}
	acct := reloadAccount(t, dbPath, "Main Checking")
	if acct.Name != "Main Checking" {
		t.Errorf("expected renamed account, got %q", acct.Name)
	}
	// Old name should be gone.
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer database.Close()
	if _, err := accountdom.NewRepository(database).GetByName("Checking"); err == nil {
		t.Errorf("expected old name to be gone")
	}
}

func TestAccountEdit_EmptyNewNameErrors(t *testing.T) {
	dbPath := seedAccount(t, newChecking("Checking"))
	_, err := runEdit(t, "--file", dbPath, "--name", "Checking", "--new-name", "")
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty-name error, got %v", err)
	}
}

func TestAccountEdit_RenameToExistingErrors(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	repo := accountdom.NewRepository(database)
	if err := repo.Create(newChecking("Checking")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := repo.Create(newChecking("Savings")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	database.Close()

	_, err := runEdit(t, "--file", dbPath, "--name", "Checking", "--new-name", "Savings")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate-name error, got %v", err)
	}
}

func TestAccountEdit_CurrencyPersists(t *testing.T) {
	dbPath := seedAccount(t, newChecking("Checking"))
	if _, err := runEdit(t, "--file", dbPath, "--name", "Checking", "--currency", "eur"); err != nil {
		t.Fatalf("edit currency: %v", err)
	}
	if got := reloadAccount(t, dbPath, "Checking").Currency; got != "EUR" {
		t.Errorf("expected EUR, got %q", got)
	}
}

func TestAccountEdit_OpeningBalancePersists(t *testing.T) {
	dbPath := seedAccount(t, newChecking("Checking"))
	if _, err := runEdit(t, "--file", dbPath, "--name", "Checking", "--opening-balance", "250.50"); err != nil {
		t.Fatalf("edit opening balance: %v", err)
	}
	if got := reloadAccount(t, dbPath, "Checking").OpeningBalance; got.Float64() != 250.50 {
		t.Errorf("expected 250.50, got %q", got.String())
	}
}

func TestAccountEdit_OpeningDatePersists(t *testing.T) {
	dbPath := seedAccount(t, newChecking("Checking"))
	if _, err := runEdit(t, "--file", dbPath, "--name", "Checking", "--opening-date", "2019-05-05"); err != nil {
		t.Fatalf("edit opening date: %v", err)
	}
	if got := reloadAccount(t, dbPath, "Checking").OpeningDate.String(); got != "2019-05-05" {
		t.Errorf("expected 2019-05-05, got %q", got)
	}
}

func TestAccountEdit_InstitutionSetThenClear(t *testing.T) {
	dbPath := seedAccount(t, newChecking("Checking"))
	if _, err := runEdit(t, "--file", dbPath, "--name", "Checking", "--institution", "Acme Bank"); err != nil {
		t.Fatalf("set institution: %v", err)
	}
	acct := reloadAccount(t, dbPath, "Checking")
	if !acct.Institution.Valid || acct.Institution.String != "Acme Bank" {
		t.Fatalf("expected institution set, got %+v", acct.Institution)
	}
	if _, err := runEdit(t, "--file", dbPath, "--name", "Checking", "--institution", ""); err != nil {
		t.Fatalf("clear institution: %v", err)
	}
	if reloadAccount(t, dbPath, "Checking").Institution.Valid {
		t.Errorf("expected institution cleared")
	}
}

func TestAccountEdit_NotesSetThenClear(t *testing.T) {
	dbPath := seedAccount(t, newChecking("Checking"))
	if _, err := runEdit(t, "--file", dbPath, "--name", "Checking", "--notes", "rainy day fund"); err != nil {
		t.Fatalf("set notes: %v", err)
	}
	if !reloadAccount(t, dbPath, "Checking").Notes.Valid {
		t.Fatalf("expected notes set")
	}
	if _, err := runEdit(t, "--file", dbPath, "--name", "Checking", "--notes", ""); err != nil {
		t.Fatalf("clear notes: %v", err)
	}
	if reloadAccount(t, dbPath, "Checking").Notes.Valid {
		t.Errorf("expected notes cleared")
	}
}

func TestAccountEdit_CreditLimitOnCreditCard(t *testing.T) {
	cc := accountdom.NewAccount("Card", accountdom.TypeCreditCard, "USD",
		types.MustNewMoney("0"), types.MustParseDate("2020-01-01"))
	dbPath := seedAccount(t, cc)
	if _, err := runEdit(t, "--file", dbPath, "--name", "Card", "--credit-limit", "5000"); err != nil {
		t.Fatalf("set credit limit: %v", err)
	}
	acct := reloadAccount(t, dbPath, "Card")
	if !acct.CreditLimit.Valid || acct.CreditLimit.Money.Float64() != 5000 {
		t.Errorf("expected credit limit 5000, got %+v", acct.CreditLimit)
	}
}

func TestAccountEdit_InterestRateOnLoan(t *testing.T) {
	loan := accountdom.NewAccount("Loan", accountdom.TypeLoan, "USD",
		types.MustNewMoney("0"), types.MustParseDate("2020-01-01"))
	dbPath := seedAccount(t, loan)
	if _, err := runEdit(t, "--file", dbPath, "--name", "Loan", "--interest-rate", "5.25"); err != nil {
		t.Fatalf("set interest rate: %v", err)
	}
	acct := reloadAccount(t, dbPath, "Loan")
	if !acct.InterestRate.Valid || acct.InterestRate.Money.String() != "5.25" {
		t.Errorf("expected interest rate 5.25, got %+v", acct.InterestRate)
	}
}

func TestAccountEdit_CreditLimitRefusedOnChecking(t *testing.T) {
	dbPath := seedAccount(t, newChecking("Checking"))
	_, err := runEdit(t, "--file", dbPath, "--name", "Checking", "--credit-limit", "5000")
	if err == nil || !strings.Contains(err.Error(), "credit_card") {
		t.Fatalf("expected credit-limit-gating error, got %v", err)
	}
}

func TestAccountEdit_OpeningBalanceRefusedWhenClosed(t *testing.T) {
	acct := newChecking("Checking")
	acct.Close(types.MustParseDate("2021-01-01"))
	dbPath := seedAccount(t, acct)
	_, err := runEdit(t, "--file", dbPath, "--name", "Checking", "--opening-balance", "100")
	if err == nil || !strings.Contains(err.Error(), "locked while the account is closed") {
		t.Fatalf("expected closed-lock error, got %v", err)
	}
}

func TestAccountEdit_OpeningDateRefusedWhenClosed(t *testing.T) {
	acct := newChecking("Checking")
	acct.Close(types.MustParseDate("2021-01-01"))
	dbPath := seedAccount(t, acct)
	_, err := runEdit(t, "--file", dbPath, "--name", "Checking", "--opening-date", "2019-01-01")
	if err == nil || !strings.Contains(err.Error(), "locked while the account is closed") {
		t.Fatalf("expected closed-lock error, got %v", err)
	}
}

func TestAccountEdit_MetadataAllowedWhenClosed(t *testing.T) {
	acct := newChecking("Checking")
	acct.Close(types.MustParseDate("2021-01-01"))
	dbPath := seedAccount(t, acct)
	if _, err := runEdit(t, "--file", dbPath, "--name", "Checking", "--notes", "archived"); err != nil {
		t.Fatalf("metadata edit on closed account should be allowed, got %v", err)
	}
	if !reloadAccount(t, dbPath, "Checking").Notes.Valid {
		t.Errorf("expected notes set on closed account")
	}
}

func TestAccountEdit_TypeChangePersists(t *testing.T) {
	dbPath := seedAccount(t, newChecking("Checking"))
	if _, err := runEdit(t, "--file", dbPath, "--name", "Checking", "--type", "savings"); err != nil {
		t.Fatalf("type change: %v", err)
	}
	if got := reloadAccount(t, dbPath, "Checking").Type; got != accountdom.TypeSavings {
		t.Errorf("expected savings, got %q", got)
	}
}

func TestAccountEdit_TypeChangeClearsTrackLots(t *testing.T) {
	inv := accountdom.NewAccount("Brokerage", accountdom.TypeInvestment, "USD",
		types.MustNewMoney("0"), types.MustParseDate("2020-01-01"))
	inv.TrackLots = true
	dbPath := seedAccount(t, inv)
	if _, err := runEdit(t, "--file", dbPath, "--name", "Brokerage", "--type", "checking"); err != nil {
		t.Fatalf("type change: %v", err)
	}
	acct := reloadAccount(t, dbPath, "Brokerage")
	if acct.Type != accountdom.TypeChecking {
		t.Errorf("expected checking, got %q", acct.Type)
	}
	if acct.TrackLots {
		t.Errorf("expected TrackLots cleared after type change away from investment")
	}
}

func TestAccountEdit_TypeChangeClearsCreditLimit(t *testing.T) {
	cc := accountdom.NewAccount("Card", accountdom.TypeCreditCard, "USD",
		types.MustNewMoney("0"), types.MustParseDate("2020-01-01"))
	cc.SetCreditLimit(types.MustNewMoney("5000"))
	dbPath := seedAccount(t, cc)
	if _, err := runEdit(t, "--file", dbPath, "--name", "Card", "--type", "checking"); err != nil {
		t.Fatalf("type change: %v", err)
	}
	acct := reloadAccount(t, dbPath, "Card")
	if acct.Type != accountdom.TypeChecking {
		t.Errorf("expected checking, got %q", acct.Type)
	}
	if acct.CreditLimit.Valid {
		t.Errorf("expected credit limit cleared after type change away from credit_card")
	}
}

// seedInvestedHSA creates an hsa_investment account with the given
// investment rows and returns the db path plus the account.
func seedInvestedHSA(t *testing.T, rows ...*investment.Transaction) (string, *accountdom.Account) {
	t.Helper()
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acct := accountdom.NewAccount("Cedar Bank HSA", accountdom.TypeHSAInvestment, "USD",
		types.MustNewMoney("0"), types.MustParseDate("2020-01-01"))
	if err := accountdom.NewRepository(database).Create(acct); err != nil {
		t.Fatalf("setup account: %v", err)
	}
	invRepo := investment.NewRepository(database)
	for _, r := range rows {
		r.AccountID = acct.ID
		if err := invRepo.Create(r); err != nil {
			t.Fatalf("setup row: %v", err)
		}
	}
	database.Close()
	return dbPath, acct
}

func cashRow(kind investment.TransactionType, amount string) *investment.Transaction {
	return investment.NewTransaction(types.NilID, types.MustParseDate("2024-03-01"), kind, types.MustNewMoney(amount))
}

func countRows(t *testing.T, dbPath string, acctID types.ID) (reg, inv int) {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer database.Close()
	reg, err = transaction.NewRepository(database).CountByAccount(acctID)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := investment.NewRepository(database).ListByAccount(acctID, investment.TransactionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	return reg, len(rows)
}

func TestAccountEdit_LedgerMove_PrintsPlanWithoutConfirm(t *testing.T) {
	dbPath, acct := seedInvestedHSA(t,
		cashRow(investment.TransactionTypeDeposit, "250.00"),
		cashRow(investment.TransactionTypeDeposit, "250.00"),
		cashRow(investment.TransactionTypeWithdrawal, "-180.25"),
	)
	out, err := runEdit(t, "--file", dbPath, "--name", "Cedar Bank HSA", "--type", "hsa")
	if err != nil {
		t.Fatalf("plan run: %v", err)
	}
	for _, want := range []string{
		"Cedar Bank HSA: HSA Investment -> HSA",
		"This moves 3 row(s)",
		"deposit        2",
		"withdrawal     1",
		"Re-run with --confirm",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plan output missing %q:\n%s", want, out)
		}
	}
	if got := reloadAccount(t, dbPath, "Cedar Bank HSA").Type; got != accountdom.TypeHSAInvestment {
		t.Errorf("type changed without --confirm: %q", got)
	}
	if reg, inv := countRows(t, dbPath, acct.ID); reg != 0 || inv != 3 {
		t.Errorf("rows moved without --confirm: reg %d inv %d", reg, inv)
	}
}

func TestAccountEdit_LedgerMove_ConfirmMovesRowsAndAppliesEdits(t *testing.T) {
	dbPath, acct := seedInvestedHSA(t,
		cashRow(investment.TransactionTypeDeposit, "250.00"),
		cashRow(investment.TransactionTypeInterest, "0.12"),
	)
	out, err := runEdit(t, "--file", dbPath, "--name", "Cedar Bank HSA", "--type", "hsa",
		"--institution", "Cedar Bank", "--confirm")
	if err != nil {
		t.Fatalf("confirm run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Moved 2 row(s) to the register.") || !strings.Contains(out, "Account updated.") {
		t.Errorf("output:\n%s", out)
	}
	got := reloadAccount(t, dbPath, "Cedar Bank HSA")
	if got.Type != accountdom.TypeHSA || got.Institution.String != "Cedar Bank" || got.TrackLots {
		t.Errorf("account after move = type %s institution %q trackLots %v", got.Type, got.Institution.String, got.TrackLots)
	}
	if reg, inv := countRows(t, dbPath, acct.ID); reg != 2 || inv != 0 {
		t.Errorf("rows after move: reg %d inv %d", reg, inv)
	}
	// The pre-write backup exists next to the file.
	backups, err := backup.ListBackups(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) == 0 {
		t.Error("no backup written before the move")
	}
}

func TestAccountEdit_LedgerMove_RefusesSecurityRows(t *testing.T) {
	buy := cashRow(investment.TransactionTypeBuy, "-250.00")
	buy.SecurityID = types.NullableID{ID: types.NewID(), Valid: true}
	buy.Shares = types.NullableQuantity{Quantity: types.MustNewQuantity("10"), Valid: true}
	buy.PricePerShare = types.NullableMoney{Money: types.MustNewMoney("25.00"), Valid: true}
	dbPath, acct := seedInvestedHSA(t, cashRow(investment.TransactionTypeDeposit, "250.00"), buy)

	_, err := runEdit(t, "--file", dbPath, "--name", "Cedar Bank HSA", "--type", "hsa", "--confirm")
	if err == nil || !strings.Contains(err.Error(), "security rows") {
		t.Fatalf("expected refusal naming security rows, got %v", err)
	}
	if got := reloadAccount(t, dbPath, "Cedar Bank HSA").Type; got != accountdom.TypeHSAInvestment {
		t.Errorf("type changed on refusal: %q", got)
	}
	if reg, inv := countRows(t, dbPath, acct.ID); reg != 0 || inv != 2 {
		t.Errorf("rows changed on refusal: reg %d inv %d", reg, inv)
	}
}

func TestAccountEdit_LedgerMove_RefusesRegularWithRows(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acct := newChecking("Checking")
	if err := accountdom.NewRepository(database).Create(acct); err != nil {
		t.Fatal(err)
	}
	txn := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-01-05"), types.MustNewMoney("-20.00"))
	if err := transaction.NewRepository(database).Create(txn); err != nil {
		t.Fatal(err)
	}
	database.Close()

	_, err := runEdit(t, "--file", dbPath, "--name", "Checking", "--type", "investment", "--confirm")
	if err == nil || !strings.Contains(err.Error(), "register rows") {
		t.Fatalf("expected refusal, got %v", err)
	}
	if got := reloadAccount(t, dbPath, "Checking").Type; got != accountdom.TypeChecking {
		t.Errorf("type changed on refusal: %q", got)
	}
}
