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
		},
		categoryIDs: []types.ID{
			types.NilID,
			salaryID,
			healthID,
			federalID,
			ssID,
		},
		salaryID:  salaryID,
		healthID:  healthID,
		federalID: federalID,
		ssID:      ssID,
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

// TestPaycheckWizard_OpensWithEmptyForm asserts the new wizard opens
// with all three sections empty (no preset rows) and the header
// fields seeded to sensible defaults.
func TestPaycheckWizard_OpensWithEmptyForm(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)
	if w == nil {
		t.Fatal("NewPaycheckWizard returned nil")
	}
	if !w.IsVisible() {
		t.Error("wizard should be visible after construction")
	}

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

	// Next payday is a masked MM/DD/YYYY date field, matching the
	// transaction dialogs' Date input behavior.
	if w.NextPayday().Type != FieldDate {
		t.Errorf("next payday should be FieldDate, got %v", w.NextPayday().Type)
	}
	if w.NextPayday().Value == "" {
		t.Error("next payday should be seeded with today's date")
	}

	// Deposit account: select with all accounts, default first.
	if w.DepositAccount().Type != FieldSelect {
		t.Errorf("deposit account should be FieldSelect, got %v", w.DepositAccount().Type)
	}
	if got := w.DepositAccount().Options[w.DepositAccount().SelectedIndex]; got != "Checking" {
		t.Errorf("deposit default = %q, want Checking", got)
	}

	// Sections all empty.
	if got := len(w.PreTaxLines()); got != 0 {
		t.Errorf("pre-tax should start empty, got %d rows", got)
	}
	if got := len(w.TaxLines()); got != 0 {
		t.Errorf("tax should start empty, got %d rows", got)
	}
	if got := len(w.PostTaxLines()); got != 0 {
		t.Errorf("post-tax should start empty, got %d rows", got)
	}
}

// TestPaycheckWizard_AddRow_AppendsToSection asserts AddRow creates
// an empty row in the requested section and that the line knows its
// section.
func TestPaycheckWizard_AddRow_AppendsToSection(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	pre := w.AddRow(PaycheckPreTax)
	tax := w.AddRow(PaycheckTax)
	post := w.AddRow(PaycheckPostTax)

	if pre.Section != PaycheckPreTax {
		t.Errorf("pre.Section = %v, want PaycheckPreTax", pre.Section)
	}
	if tax.Section != PaycheckTax {
		t.Errorf("tax.Section = %v, want PaycheckTax", tax.Section)
	}
	if post.Section != PaycheckPostTax {
		t.Errorf("post.Section = %v, want PaycheckPostTax", post.Section)
	}

	if got := len(w.PreTaxLines()); got != 1 {
		t.Errorf("pre-tax count = %d, want 1", got)
	}
	if got := len(w.TaxLines()); got != 1 {
		t.Errorf("tax count = %d, want 1", got)
	}
	if got := len(w.PostTaxLines()); got != 1 {
		t.Errorf("post-tax count = %d, want 1", got)
	}

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

// TestPaycheckWizard_RemoveRow_RemovesByPointer asserts RemoveRow
// drops the row from its section.
func TestPaycheckWizard_RemoveRow_RemovesByPointer(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	a := w.AddRow(PaycheckTax)
	b := w.AddRow(PaycheckTax)

	w.RemoveRow(a)
	if got := len(w.TaxLines()); got != 1 {
		t.Fatalf("tax count after remove = %d, want 1", got)
	}
	if w.TaxLines()[0] != b {
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
