package tui

import (
	"path/filepath"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// paycheckWizardFixture returns a representative category list +
// account list used across wizard tests.
type paycheckWizardFixture struct {
	categoryOptions []string
	categoryIDs     []types.ID
	salaryID        types.ID
	healthID        types.ID
	federalID       types.ID
	ssID            types.ID
	medicareID      types.ID

	accounts     []*account.Account
	checkingID   types.ID
	savingsID    types.ID
	retire401kID types.ID
}

func newPaycheckWizardFixture() *paycheckWizardFixture {
	salaryID := types.NewID()
	healthID := types.NewID()
	federalID := types.NewID()
	ssID := types.NewID()
	medicareID := types.NewID()
	checkingID := types.NewID()
	savingsID := types.NewID()
	retire401kID := types.NewID()

	return &paycheckWizardFixture{
		categoryOptions: []string{
			"(None)",
			"Income > Salary",
			"Insurance > Health",
			"Tax > Federal",
			"Tax > Social Security",
			"Tax > Medicare",
		},
		categoryIDs: []types.ID{
			types.NilID,
			salaryID,
			healthID,
			federalID,
			ssID,
			medicareID,
		},
		salaryID:   salaryID,
		healthID:   healthID,
		federalID:  federalID,
		ssID:       ssID,
		medicareID: medicareID,
		accounts: []*account.Account{
			{BaseModel: types.BaseModel{ID: checkingID}, Name: "Checking", Active: true, Type: account.TypeChecking},
			{BaseModel: types.BaseModel{ID: savingsID}, Name: "Savings", Active: true, Type: account.TypeSavings},
			{BaseModel: types.BaseModel{ID: retire401kID}, Name: "401k", Active: true, Type: account.TypeInvestment},
		},
		checkingID:   checkingID,
		savingsID:    savingsID,
		retire401kID: retire401kID,
	}
}

// indexOf is a small helper used in several tests.
func indexOf(haystack []string, needle string) int {
	for i, s := range haystack {
		if s == needle {
			return i
		}
	}
	return -1
}

// TestPaycheckWizard_V2Layout_OpensWithSpecPrePopulation asserts the v2
// wizard opens with five sections pre-populated per
// specs/multiline-splits-and-paycheck.md "Pre-populated rows" table:
// Earnings has one Income:Salary row, Pre-tax is empty, Taxes has
// three rows (Federal/Social Security/Medicare), Post-tax is empty,
// and Net Pay Destinations has no additional transfers (the primary
// deposit picker lives in the header).
func TestPaycheckWizard_V2Layout_OpensWithSpecPrePopulation(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)
	if w == nil {
		t.Fatal("NewPaycheckWizard returned nil")
	}
	if !w.IsVisible() {
		t.Error("wizard should be visible after construction")
	}

	// Header field defaults (carried over from v1).
	if got := w.Employer().Value; got != "" {
		t.Errorf("employer should start empty, got %q", got)
	}
	if w.Employer().Type != FieldText {
		t.Errorf("employer should be FieldText, got %v", w.Employer().Type)
	}
	if got, want := w.Frequency().SelectedIndex, defaultPaycheckFrequencyIndex; got != want {
		t.Errorf("frequency default = %d, want %d (Fortnightly)", got, want)
	}
	if opt := paycheckFrequencyForIndex(w.Frequency().SelectedIndex); opt.frequency != scheduled.FrequencyFortnightly {
		t.Errorf("default frequency option = %v, want fortnightly", opt.frequency)
	}
	if w.NextPayday().Type != FieldDate {
		t.Errorf("next payday should be FieldDate, got %v", w.NextPayday().Type)
	}
	if w.NextPayday().Value == "" {
		t.Error("next payday should be seeded with today's date")
	}
	if w.DepositAccount().Type != FieldSelect {
		t.Errorf("deposit account should be FieldSelect, got %v", w.DepositAccount().Type)
	}
	if got := w.DepositAccount().Options[w.DepositAccount().SelectedIndex]; got != "Checking" {
		t.Errorf("deposit default = %q, want Checking", got)
	}

	// Earnings: 1 row pre-populated with Income > Salary, empty amount.
	earnings := w.EarningsLines()
	if got := len(earnings); got != 1 {
		t.Fatalf("EarningsLines count = %d, want 1", got)
	}
	if got, want := selectedLineOption(earnings[0]), "Income > Salary"; got != want {
		t.Errorf("Earnings row[0] category = %q, want %q", got, want)
	}
	if earnings[0].AmountField().Value != "" {
		t.Errorf("Earnings row[0] amount = %q, want empty",
			earnings[0].AmountField().Value)
	}
	if earnings[0].Section != PaycheckEarnings {
		t.Errorf("Earnings row[0] Section = %v, want PaycheckEarnings",
			earnings[0].Section)
	}

	// Pre-tax: 0 rows (added via [+ Add pre-tax line]).
	if got := len(w.PreTaxLines()); got != 0 {
		t.Errorf("PreTaxLines count = %d, want 0", got)
	}

	// Taxes: 3 rows pre-populated in order Federal, Social Security,
	// Medicare; all with empty amounts.
	tax := w.TaxLines()
	if got := len(tax); got != 3 {
		t.Fatalf("TaxLines count = %d, want 3", got)
	}
	wantTaxCats := []string{"Tax > Federal", "Tax > Social Security", "Tax > Medicare"}
	for i, want := range wantTaxCats {
		if got := selectedLineOption(tax[i]); got != want {
			t.Errorf("Tax row[%d] category = %q, want %q", i, got, want)
		}
		if tax[i].AmountField().Value != "" {
			t.Errorf("Tax row[%d] amount = %q, want empty",
				i, tax[i].AmountField().Value)
		}
		if tax[i].Section != PaycheckTax {
			t.Errorf("Tax row[%d] Section = %v, want PaycheckTax",
				i, tax[i].Section)
		}
	}

	// Post-tax: 0 rows (added via [+ Add post-tax line]).
	if got := len(w.PostTaxLines()); got != 0 {
		t.Errorf("PostTaxLines count = %d, want 0", got)
	}

	// Net Pay Destinations: 0 additional transfers. The primary
	// deposit picker is in the header (w.DepositAccount()).
	if got := len(w.AdditionalTransfers()); got != 0 {
		t.Errorf("AdditionalTransfers count = %d, want 0", got)
	}
}

// selectedLineOption returns the display string currently selected in
// a paycheck line's category-or-transfer picker, or "" if unset.
func selectedLineOption(line *PaycheckLine) string {
	f := line.SelectField()
	if f == nil || f.SelectedIndex < 0 || f.SelectedIndex >= len(f.Options) {
		return ""
	}
	return f.Options[f.SelectedIndex]
}

// TestPaycheckWizard_AddRow_AppendsToSection asserts AddRow appends an
// empty row to the requested section (on top of any v2-pre-populated
// rows) and that the line knows its section.
func TestPaycheckWizard_AddRow_AppendsToSection(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	earningsBefore := len(w.EarningsLines())
	preBefore := len(w.PreTaxLines())
	taxBefore := len(w.TaxLines())
	postBefore := len(w.PostTaxLines())

	earn := w.AddRow(PaycheckEarnings)
	pre := w.AddRow(PaycheckPreTax)
	tax := w.AddRow(PaycheckTax)
	post := w.AddRow(PaycheckPostTax)

	if earn.Section != PaycheckEarnings {
		t.Errorf("earn.Section = %v, want PaycheckEarnings", earn.Section)
	}
	if pre.Section != PaycheckPreTax {
		t.Errorf("pre.Section = %v, want PaycheckPreTax", pre.Section)
	}
	if tax.Section != PaycheckTax {
		t.Errorf("tax.Section = %v, want PaycheckTax", tax.Section)
	}
	if post.Section != PaycheckPostTax {
		t.Errorf("post.Section = %v, want PaycheckPostTax", post.Section)
	}

	if got, want := len(w.EarningsLines()), earningsBefore+1; got != want {
		t.Errorf("earnings count = %d, want %d", got, want)
	}
	if got, want := len(w.PreTaxLines()), preBefore+1; got != want {
		t.Errorf("pre-tax count = %d, want %d", got, want)
	}
	if got, want := len(w.TaxLines()), taxBefore+1; got != want {
		t.Errorf("tax count = %d, want %d", got, want)
	}
	if got, want := len(w.PostTaxLines()), postBefore+1; got != want {
		t.Errorf("post-tax count = %d, want %d", got, want)
	}

	// Newly-added rows default to empty amount and the (None) category.
	if pre.AmountField().Value != "" {
		t.Errorf("new row amount should be empty, got %q", pre.AmountField().Value)
	}
	if pre.SelectField().SelectedIndex != 0 {
		t.Errorf("new row select should default to (None), got %d", pre.SelectField().SelectedIndex)
	}
	if pre.IsTransfer() {
		t.Error("new row should not be a transfer-line by default")
	}
}

// TestPaycheckWizard_AddLine_AppendsRowToSection asserts that each
// section's `[+ Add …]` helper appends one row to that section with
// an empty amount field and the section's appropriate default:
//
//   - Earnings        → categorized, defaulted to Income > Salary
//   - Pre-tax / Tax / Post-tax → categorized, defaulted to (None)
//   - Net Pay Destinations    → transfer-line, defaulted to a
//     non-deposit account
//
// The five helpers (`AddEarningsLine`, `AddPreTaxLine`, `AddTaxLine`,
// `AddPostTaxLine`, `AddAdditionalTransfer`) are also the dispatch
// targets used when the user activates a `[+ Add …]` button in the
// rendered wizard.
func TestPaycheckWizard_AddLine_AppendsRowToSection(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	earnBefore := len(w.EarningsLines())
	preBefore := len(w.PreTaxLines())
	taxBefore := len(w.TaxLines())
	postBefore := len(w.PostTaxLines())
	xferBefore := len(w.AdditionalTransfers())

	earn := w.AddEarningsLine()
	pre := w.AddPreTaxLine()
	tax := w.AddTaxLine()
	post := w.AddPostTaxLine()
	xfer := w.AddAdditionalTransfer()

	if earn == nil || pre == nil || tax == nil || post == nil || xfer == nil {
		t.Fatalf("Add* helper returned nil: earn=%v pre=%v tax=%v post=%v xfer=%v",
			earn, pre, tax, post, xfer)
	}

	// Each section's row count grows by exactly one.
	if got, want := len(w.EarningsLines()), earnBefore+1; got != want {
		t.Errorf("EarningsLines count = %d, want %d", got, want)
	}
	if got, want := len(w.PreTaxLines()), preBefore+1; got != want {
		t.Errorf("PreTaxLines count = %d, want %d", got, want)
	}
	if got, want := len(w.TaxLines()), taxBefore+1; got != want {
		t.Errorf("TaxLines count = %d, want %d", got, want)
	}
	if got, want := len(w.PostTaxLines()), postBefore+1; got != want {
		t.Errorf("PostTaxLines count = %d, want %d", got, want)
	}
	if got, want := len(w.AdditionalTransfers()), xferBefore+1; got != want {
		t.Errorf("AdditionalTransfers count = %d, want %d", got, want)
	}

	// Section assignment on each new line.
	if earn.Section != PaycheckEarnings {
		t.Errorf("earn.Section = %v, want PaycheckEarnings", earn.Section)
	}
	if pre.Section != PaycheckPreTax {
		t.Errorf("pre.Section = %v, want PaycheckPreTax", pre.Section)
	}
	if tax.Section != PaycheckTax {
		t.Errorf("tax.Section = %v, want PaycheckTax", tax.Section)
	}
	if post.Section != PaycheckPostTax {
		t.Errorf("post.Section = %v, want PaycheckPostTax", post.Section)
	}
	if xfer.Section != PaycheckNetPayDestination {
		t.Errorf("xfer.Section = %v, want PaycheckNetPayDestination", xfer.Section)
	}

	// All five rows start with an empty amount.
	for name, line := range map[string]*PaycheckLine{
		"earnings": earn, "pre-tax": pre, "tax": tax, "post-tax": post, "transfer": xfer,
	} {
		if line.AmountField().Value != "" {
			t.Errorf("%s row amount = %q, want empty", name, line.AmountField().Value)
		}
	}

	// Earnings line is categorized and pre-selected with Income > Salary.
	if earn.IsTransfer() {
		t.Error("AddEarningsLine should produce a categorized row, not a transfer-line")
	}
	if got, want := selectedLineOption(earn), "Income > Salary"; got != want {
		t.Errorf("AddEarningsLine default category = %q, want %q", got, want)
	}

	// Pre-tax / Tax / Post-tax start categorized at (None) so the user picks.
	for name, line := range map[string]*PaycheckLine{
		"pre-tax": pre, "tax": tax, "post-tax": post,
	} {
		if line.IsTransfer() {
			t.Errorf("%s row should default to categorized, not transfer-line", name)
		}
		if got := line.SelectField().SelectedIndex; got != 0 {
			t.Errorf("%s row default category index = %d, want 0 ((None))", name, got)
		}
	}

	// Net Pay Destinations row defaults to a transfer targeting some
	// account other than the deposit account (which is the schedule's
	// parent — a self-transfer would be rejected on save).
	if !xfer.IsTransfer() {
		t.Error("AddAdditionalTransfer should produce a transfer-line")
	}
	depositIdx := w.DepositAccount().SelectedIndex
	if xfer.AccountIndex() == depositIdx {
		t.Errorf("AddAdditionalTransfer should not default to the deposit account (idx %d)", depositIdx)
	}
}

// TestPaycheckWizard_RemoveRow_RemovesByPointer asserts RemoveRow
// drops the row from its section. Uses Pre-tax which starts empty in
// v2 so the assertion is independent of pre-populated row counts.
func TestPaycheckWizard_RemoveRow_RemovesByPointer(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	a := w.AddRow(PaycheckPreTax)
	b := w.AddRow(PaycheckPreTax)

	w.RemoveRow(a)
	if got := len(w.PreTaxLines()); got != 1 {
		t.Fatalf("pre-tax count after remove = %d, want 1", got)
	}
	if w.PreTaxLines()[0] != b {
		t.Error("remaining row should be b")
	}
}

// TestPaycheckWizard_SetAccountIndex_FlagsTransfer asserts that
// pointing a row's select at an account flips it to transfer-line
// mode.
func TestPaycheckWizard_SetAccountIndex_FlagsTransfer(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	line := w.AddRow(PaycheckPostTax)
	if line.IsTransfer() {
		t.Fatal("new row should start categorized")
	}

	// 401k is at account index 2 (Checking 0, Savings 1, 401k 2).
	line.SetAccountIndex(2)
	if !line.IsTransfer() {
		t.Error("after SetAccountIndex line should be a transfer-line")
	}
	if got := line.AccountIndex(); got != 2 {
		t.Errorf("AccountIndex = %d, want 2", got)
	}

	// Switching back to a category index flips it back.
	line.SetCategoryIndex(1)
	if line.IsTransfer() {
		t.Error("after SetCategoryIndex line should be categorized again")
	}
	if got := line.CategoryIndex(); got != 1 {
		t.Errorf("CategoryIndex = %d, want 1", got)
	}
}

// TestPaycheckWizard_BuildSplits_PreservesSignedAmounts asserts the
// signed sum equals the parent amount and each row is persisted
// with the user's typed sign.
func TestPaycheckWizard_BuildSplits_PreservesSignedAmounts(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	gross := w.AddRow(PaycheckPreTax)
	gross.SetCategoryIndex(indexOf(w.PreTaxLines()[0].SelectField().Options, "Income > Salary"))
	gross.AmountField().Value = "5000"

	fed := w.AddRow(PaycheckTax)
	fed.SetCategoryIndex(indexOf(fed.SelectField().Options, "Tax > Federal"))
	fed.AmountField().Value = "-800"

	ss := w.AddRow(PaycheckTax)
	ss.SetCategoryIndex(indexOf(ss.SelectField().Options, "Tax > Social Security"))
	ss.AmountField().Value = "-310"

	transfer := w.AddRow(PaycheckPostTax)
	// 401k is account index 2.
	transfer.SetAccountIndex(2)
	transfer.AmountField().Value = "-500"

	parent, splits, err := w.BuildSplits()
	if err != nil {
		t.Fatalf("BuildSplits: %v", err)
	}
	if got, want := len(splits), 4; got != want {
		t.Fatalf("split count = %d, want %d", got, want)
	}
	wantParent := types.MustNewMoney("3390")
	if !parent.Equal(wantParent) {
		t.Errorf("parent = %s, want %s", parent.String(), wantParent.String())
	}

	wantBySource := map[string]string{
		"salary":   "5000",
		"federal":  "-800",
		"ss":       "-310",
		"transfer": "-500",
	}
	got := map[string]string{}
	for _, sp := range splits {
		switch {
		case sp.CategoryID.Valid && sp.CategoryID.ID == fx.salaryID:
			got["salary"] = sp.Amount.String()
		case sp.CategoryID.Valid && sp.CategoryID.ID == fx.federalID:
			got["federal"] = sp.Amount.String()
		case sp.CategoryID.Valid && sp.CategoryID.ID == fx.ssID:
			got["ss"] = sp.Amount.String()
		case sp.TransferAccountID.Valid && sp.TransferAccountID.ID == fx.retire401kID:
			got["transfer"] = sp.Amount.String()
		}
	}
	for k, want := range wantBySource {
		if g := got[k]; g != want {
			t.Errorf("%s amount = %q, want %q", k, g, want)
		}
	}
}

// TestPaycheckWizard_BuildSplits_PersistsNotesAsMemo asserts the
// per-row Notes field is written to Split.Memo (and that empty notes
// stay NULL).
func TestPaycheckWizard_BuildSplits_PersistsNotesAsMemo(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	gross := w.AddRow(PaycheckPreTax)
	gross.SetCategoryIndex(indexOf(gross.SelectField().Options, "Income > Salary"))
	gross.AmountField().Value = "5000"
	gross.NotesField().Value = "Base pay"

	fed := w.AddRow(PaycheckTax)
	fed.SetCategoryIndex(indexOf(fed.SelectField().Options, "Tax > Federal"))
	fed.AmountField().Value = "-800"
	// Note: no notes set on this row — Split.Memo should stay NULL.

	_, splits, err := w.BuildSplits()
	if err != nil {
		t.Fatalf("BuildSplits: %v", err)
	}
	if len(splits) != 2 {
		t.Fatalf("split count = %d, want 2", len(splits))
	}

	var sawSalary, sawFederal bool
	for _, sp := range splits {
		if sp.CategoryID.Valid && sp.CategoryID.ID == fx.salaryID {
			sawSalary = true
			if !sp.Memo.Valid || sp.Memo.String != "Base pay" {
				t.Errorf("salary row Memo = %+v, want valid=true value=%q", sp.Memo, "Base pay")
			}
		}
		if sp.CategoryID.Valid && sp.CategoryID.ID == fx.federalID {
			sawFederal = true
			if sp.Memo.Valid {
				t.Errorf("federal row Memo unexpectedly set: %q", sp.Memo.String)
			}
		}
	}
	if !sawSalary || !sawFederal {
		t.Fatalf("missing expected split rows: salary=%v federal=%v", sawSalary, sawFederal)
	}
}

// TestPaycheckWizard_BuildSplits_SkipsEmptyRows asserts an empty
// amount row is silently skipped instead of producing a zero-amount
// split.
func TestPaycheckWizard_BuildSplits_SkipsEmptyRows(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	a := w.AddRow(PaycheckPreTax)
	a.SetCategoryIndex(indexOf(a.SelectField().Options, "Income > Salary"))
	a.AmountField().Value = "5000"

	_ = w.AddRow(PaycheckTax) // empty row, no category picked

	_, splits, err := w.BuildSplits()
	if err != nil {
		t.Fatalf("BuildSplits: %v", err)
	}
	if got := len(splits); got != 1 {
		t.Errorf("split count = %d, want 1 (the empty row should be skipped)", got)
	}
}

// TestPaycheckWizard_Save_CreatesMultiLineSchedule drives the
// wizard end-to-end against a real DB and confirms the saved
// schedule mirrors the user's input.
func TestPaycheckWizard_Save_CreatesMultiLineSchedule(t *testing.T) {
	tempDir := t.TempDir()
	database, err := db.Create(filepath.Join(tempDir, "test.tdb"))
	if err != nil {
		t.Fatalf("db.Create: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	payeeRepo := payee.NewRepository(database)
	schedRepo := scheduled.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitTxnRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)

	txnSvc := transaction.NewService(txnRepo, splitTxnRepo, transferRepo, payeeRepo, database)
	schedSvc := scheduled.NewService(schedRepo, txnRepo, txnSvc, database)
	accountSvc := account.NewService(accountRepo, database)
	payeeSvc := payee.NewService(payeeRepo, database)
	categorySvc := category.NewService(categoryRepo, database)

	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(checking); err != nil {
		t.Fatalf("create checking: %v", err)
	}
	retire := account.NewAccount("401k", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(retire); err != nil {
		t.Fatalf("create 401k: %v", err)
	}
	if err := categorySvc.EnsurePaycheckCategories(); err != nil {
		t.Fatalf("EnsurePaycheckCategories: %v", err)
	}

	accounts, err := accountSvc.List(true)
	if err != nil {
		t.Fatalf("List accounts: %v", err)
	}
	cats, err := categorySvc.List()
	if err != nil {
		t.Fatalf("List categories: %v", err)
	}
	categoryOptions, categoryIDs := buildCategoryOptions(cats)

	app := &App{
		currentView:     ViewDashboard,
		keys:            defaultKeyMap(),
		menubar:         NewMenuBar(),
		statusbar:       NewStatusBar(),
		sidebar:         NewSidebar(),
		accountSvc:      accountSvc,
		payeeSvc:        payeeSvc,
		categorySvc:     categorySvc,
		scheduledTxnSvc: schedSvc,
		transactionSvc:  txnSvc,
		undoManager:     undo.NewManager(),
	}
	app.paycheckWizard = NewPaycheckWizard(categoryOptions, categoryIDs, accounts)
	w := app.paycheckWizard

	// Set header fields.
	w.Employer().Value = "Acme Corp"
	w.NextPayday().Value = "05/15/2026"
	// Pick semi-monthly (15th & last day) — option index 3.
	w.Frequency().SelectedIndex = 3
	// Resolve deposit account by name to be robust to List ordering.
	checkingAcctIdx := indexOf(w.DepositAccount().Options, "Checking")
	if checkingAcctIdx < 0 {
		t.Fatalf("Checking missing from account picker: %v", w.DepositAccount().Options)
	}
	w.DepositAccount().SelectedIndex = checkingAcctIdx

	salaryIdx := indexOf(categoryOptions, "Income > Salary")
	federalIdx := indexOf(categoryOptions, "Tax > Federal")
	healthIdx := indexOf(categoryOptions, "Insurance > Health")
	retireAcctIdx := indexOf(w.DepositAccount().Options, "401k")
	if salaryIdx <= 0 || federalIdx <= 0 || healthIdx <= 0 || retireAcctIdx < 0 {
		t.Fatalf("category/account indices unresolved: salary=%d federal=%d health=%d 401k=%d",
			salaryIdx, federalIdx, healthIdx, retireAcctIdx)
	}

	gross := w.AddRow(PaycheckPreTax)
	gross.SetCategoryIndex(salaryIdx)
	gross.AmountField().Value = "5000"

	fed := w.AddRow(PaycheckTax)
	fed.SetCategoryIndex(federalIdx)
	fed.AmountField().Value = "-800"

	health := w.AddRow(PaycheckPostTax)
	health.SetCategoryIndex(healthIdx)
	health.AmountField().Value = "-150"

	transfer := w.AddRow(PaycheckPostTax)
	transfer.SetAccountIndex(retireAcctIdx)
	transfer.AmountField().Value = "-500"

	model, cmd := app.submitPaycheckWizard()
	app2 := model.(*App)
	if app2.paycheckWizard != nil {
		t.Errorf("wizard should be cleared after a successful save; errorMsg=%q", app2.paycheckWizard.errorMsg)
	}
	if cmd == nil {
		t.Fatal("submitPaycheckWizard should return a non-nil command")
	}
	if msg := cmd(); msg != nil {
		if e, ok := msg.(errMsg); ok {
			t.Fatalf("save command returned error: %v", e.err)
		}
		if _, ok := msg.(scheduledDialogSavedMsg); !ok {
			t.Fatalf("save command returned unexpected message type: %T", msg)
		}
	}

	schedules, err := schedSvc.List()
	if err != nil {
		t.Fatalf("List schedules: %v", err)
	}
	if len(schedules) != 1 {
		t.Fatalf("got %d schedules, want 1", len(schedules))
	}
	sched := schedules[0]

	if sched.AccountID != checking.ID {
		t.Errorf("AccountID = %v, want Checking %v", sched.AccountID, checking.ID)
	}
	if sched.Frequency != scheduled.FrequencySemiMonthly {
		t.Errorf("Frequency = %s, want semimonthly", sched.Frequency)
	}
	if !sched.DayOfMonth.Valid || sched.DayOfMonth.Int64 != 15 {
		t.Errorf("DayOfMonth = %+v, want 15", sched.DayOfMonth)
	}
	if !sched.SecondaryDayOfMonth.Valid || sched.SecondaryDayOfMonth.Int64 != -1 {
		t.Errorf("SecondaryDayOfMonth = %+v, want -1", sched.SecondaryDayOfMonth)
	}
	if sched.HasCategory() {
		t.Error("multi-line schedule should clear the scalar category")
	}
	wantNet := types.MustNewMoney("3550") // 5000 - 800 - 150 - 500
	if !sched.Amount.Money.Equal(wantNet) {
		t.Errorf("parent amount = %s, want %s", sched.Amount.Money.String(), wantNet.String())
	}
	if len(sched.Splits) != 4 {
		t.Fatalf("got %d splits, want 4", len(sched.Splits))
	}
	if !sched.Splits.Total().Equal(wantNet) {
		t.Errorf("signed sum of children = %s, want %s",
			sched.Splits.Total().String(), wantNet.String())
	}

	// Verify each saved split mirrors the wizard input.
	var sawSalary, sawFederal, sawHealth, sawTransfer bool
	for _, sp := range sched.Splits {
		switch {
		case sp.CategoryID.Valid && categoryByID(cats, sp.CategoryID.ID) == "Salary":
			sawSalary = true
			if got, want := sp.Amount, types.MustNewMoney("5000"); !got.Equal(want) {
				t.Errorf("salary amount = %s, want %s", got, want)
			}
		case sp.CategoryID.Valid && categoryByID(cats, sp.CategoryID.ID) == "Federal":
			sawFederal = true
			if got, want := sp.Amount, types.MustNewMoney("-800"); !got.Equal(want) {
				t.Errorf("federal amount = %s, want %s", got, want)
			}
		case sp.CategoryID.Valid && categoryByID(cats, sp.CategoryID.ID) == "Health":
			sawHealth = true
			if got, want := sp.Amount, types.MustNewMoney("-150"); !got.Equal(want) {
				t.Errorf("health amount = %s, want %s", got, want)
			}
		case sp.TransferAccountID.Valid && sp.TransferAccountID.ID == retire.ID:
			sawTransfer = true
			if got, want := sp.Amount, types.MustNewMoney("-500"); !got.Equal(want) {
				t.Errorf("transfer amount = %s, want %s", got, want)
			}
		}
	}
	if !sawSalary || !sawFederal || !sawHealth || !sawTransfer {
		t.Errorf("missing splits: salary=%v fed=%v health=%v transfer=%v",
			sawSalary, sawFederal, sawHealth, sawTransfer)
	}
}

// categoryByID returns the leaf category name (subcategory) for a given ID,
// or empty string when not found. Helper for paycheck-save tests.
func categoryByID(cats []*category.Category, id types.ID) string {
	for _, c := range cats {
		if c.ID == id {
			return c.Name
		}
	}
	return ""
}
