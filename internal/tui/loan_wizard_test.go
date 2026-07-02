package tui

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// loanWizardEnv is a DB-backed harness for the loan wizard (Phase 7). It builds
// an App wired with real services plus a funding "Checking" account and a
// couple of escrow categories, then opens the wizard.
type loanWizardEnv struct {
	app         *App
	database    *db.DB
	accountSvc  *account.Service
	schedSvc    *scheduled.Service
	categorySvc *category.Service
	funding     *account.Account
	taxCat      *category.Category
	insCat      *category.Category
}

func newLoanWizardEnv(t *testing.T) *loanWizardEnv {
	t.Helper()
	database := dbtest.New(t)

	accountRepo := account.NewRepository(database)
	payeeRepo := payee.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	schedRepo := scheduled.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitTxnRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)

	txnSvc := transaction.NewService(txnRepo, splitTxnRepo, transferRepo, payeeRepo, accountRepo, database)
	schedSvc := scheduled.NewService(schedRepo, txnRepo, txnSvc, database, accountRepo)
	accountSvc := account.NewService(accountRepo, database)
	payeeSvc := payee.NewService(payeeRepo, database)
	categorySvc := category.NewService(categoryRepo, database)

	opened := types.NewDate(2020, time.January, 1)
	funding := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, opened)
	if err := accountRepo.Create(funding); err != nil {
		t.Fatalf("create funding: %v", err)
	}

	// Escrow categories to pick from.
	housing := category.NewCategory("Housing", category.TypeExpense)
	if err := categoryRepo.Create(housing); err != nil {
		t.Fatalf("create housing: %v", err)
	}
	taxCat := category.NewSubcategory("Property Tax", housing.ID, category.TypeExpense)
	if err := categoryRepo.Create(taxCat); err != nil {
		t.Fatalf("create tax cat: %v", err)
	}
	insCat := category.NewSubcategory("Home Insurance", housing.ID, category.TypeExpense)
	if err := categoryRepo.Create(insCat); err != nil {
		t.Fatalf("create ins cat: %v", err)
	}

	app := &App{
		currentView:     ViewDashboard,
		width:           120,
		height:          40,
		keys:            defaultKeyMap(),
		menubar:         widget.NewMenuBar(),
		statusbar:       widget.NewStatusBar(),
		sidebar:         NewSidebar(),
		styles:          widget.NewStyles(),
		accountSvc:      accountSvc,
		payeeSvc:        payeeSvc,
		categorySvc:     categorySvc,
		scheduledTxnSvc: schedSvc,
		transactionSvc:  txnSvc,
		undoManager:     undo.NewManager(),
	}

	accounts, err := accountSvc.List(true)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	cats, err := categorySvc.List()
	if err != nil {
		t.Fatalf("list categories: %v", err)
	}
	app.loanWizard, app.loanWizardState = buildNewLoanWizard(accounts, cats)

	return &loanWizardEnv{
		app:         app,
		database:    database,
		accountSvc:  accountSvc,
		schedSvc:    schedSvc,
		categorySvc: categorySvc,
		funding:     funding,
		taxCat:      taxCat,
		insCat:      insCat,
	}
}

// set writes a text/date field's value.
func (env *loanWizardEnv) set(idx int, value string) {
	env.app.loanWizard.Fields()[idx].Value = value
}

// selectOption sets a select field to the option matching name.
func (env *loanWizardEnv) selectOption(t *testing.T, idx int, name string) {
	t.Helper()
	f := env.app.loanWizard.Fields()[idx]
	for i, opt := range f.Options {
		if opt == name {
			f.SelectedIndex = i
			return
		}
	}
	t.Fatalf("option %q not found in field %d options %v", name, idx, f.Options)
}

// submit drives submitLoanWizard and, when it produced an async command, runs
// it to completion, returning the resulting message (nil for a validation
// failure that leaves the wizard open).
func (env *loanWizardEnv) submit(t *testing.T) any {
	t.Helper()
	env.app.refreshLoanWizardDerived()
	_, cmd := env.app.submitLoanWizard()
	if cmd == nil {
		return nil
	}
	return cmd()
}

// findLoanSchedule returns the single loan-shaped schedule, failing if none.
func (env *loanWizardEnv) findLoanSchedule(t *testing.T) *scheduled.Transaction {
	t.Helper()
	all, err := env.schedSvc.List()
	if err != nil {
		t.Fatalf("list schedules: %v", err)
	}
	for _, st := range all {
		if env.schedSvc.IsLoanShaped(st) {
			return st
		}
	}
	t.Fatal("no loan-shaped schedule found")
	return nil
}

func TestLoanWizard_CreatesLoanAccountAndSchedule(t *testing.T) {
	env := newLoanWizardEnv(t)

	env.set(loanFieldName, "Mortgage")
	env.set(loanFieldInstitution, "Wells Fargo")
	env.set(loanFieldCurrentBalance, "380000")
	env.set(loanFieldAPR, "6.5")
	env.set(loanFieldPayment, "2401.86")
	env.set(loanFieldNextPaymentDate, "08/01/2026")
	env.selectOption(t, loanFieldFromAccount, "Checking")
	env.set(loanFieldPayee, "Wells Fargo")
	// Interest category left at the default (Loan > Interest, get-or-created).
	// One escrow line.
	env.selectOption(t, loanFieldEscrowStart, "Housing > Property Tax")
	env.set(loanEscrowAmtIndex(0), "650")

	msg := env.submit(t)
	if _, ok := msg.(loanWizardSavedMsg); !ok {
		t.Fatalf("submit msg = %T (%v), want loanWizardSavedMsg", msg, msg)
	}

	// Loan account created with negated opening balance + APR + institution.
	loanAcct, err := env.accountSvc.GetByName("Mortgage")
	if err != nil {
		t.Fatalf("loan account not created: %v", err)
	}
	if loanAcct.Type != account.TypeLoan {
		t.Errorf("type = %v, want loan", loanAcct.Type)
	}
	if want := types.MustNewMoney("-380000"); !loanAcct.OpeningBalance.Equal(want) {
		t.Errorf("opening balance = %s, want %s (negated)", loanAcct.OpeningBalance, want)
	}
	if !loanAcct.InterestRate.Valid || !loanAcct.InterestRate.Money.Equal(types.MustNewMoney("6.5")) {
		t.Errorf("interest rate = %v, want 6.5", loanAcct.InterestRate)
	}
	if !loanAcct.Institution.Valid || loanAcct.Institution.String != "Wells Fargo" {
		t.Errorf("institution = %v, want Wells Fargo", loanAcct.Institution)
	}

	// Schedule is loan-shaped, monthly, indefinite, on the funding account.
	st := env.findLoanSchedule(t)
	if st.Frequency != scheduled.FrequencyMonthly {
		t.Errorf("frequency = %v, want monthly", st.Frequency)
	}
	if st.AccountID != env.funding.ID {
		t.Errorf("schedule account = %v, want funding %v", st.AccountID, env.funding.ID)
	}
	if st.EndDate.Valid || st.Occurrences.Valid {
		t.Errorf("schedule should be indefinite (no end date / occurrences): end=%v occ=%v", st.EndDate, st.Occurrences)
	}
	if !st.DayOfMonth.Valid || st.DayOfMonth.Int64 != 1 {
		t.Errorf("day of month = %v, want 1", st.DayOfMonth)
	}

	// Splits: one interest (categorized), one principal (transfer→loan), one escrow.
	var interest, principal, escrow int
	for _, sp := range st.Splits {
		if !sp.LoanSection.Valid {
			t.Errorf("split has no loan_section tag: %+v", sp)
			continue
		}
		switch sp.LoanSection.String {
		case scheduled.LoanSectionInterest:
			interest++
			if !sp.CategoryID.Valid {
				t.Error("interest line must be categorized")
			}
		case scheduled.LoanSectionPrincipal:
			principal++
			if !sp.TransferAccountID.Valid || sp.TransferAccountID.ID != loanAcct.ID {
				t.Errorf("principal transfer target = %v, want loan %v", sp.TransferAccountID, loanAcct.ID)
			}
		case scheduled.LoanSectionEscrow:
			escrow++
		}
		if sp.Amount.IsPositive() {
			t.Errorf("split %s should be negative (funding-account outflow)", sp.Amount)
		}
	}
	if interest != 1 || principal != 1 || escrow != 1 {
		t.Errorf("tag counts: interest=%d principal=%d escrow=%d, want 1/1/1", interest, principal, escrow)
	}

	// Default interest category was get-or-created.
	if _, err := env.categorySvc.GetOrCreateLoanInterestCategory(); err != nil {
		t.Errorf("interest category lookup: %v", err)
	}

	// One atomic undo step that removes both the account and the schedule.
	if env.app.undoManager.UndoLen() != 1 {
		t.Fatalf("undo stack = %d, want 1 (single atomic step)", env.app.undoManager.UndoLen())
	}
	if _, err := env.app.undoManager.Undo(); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if _, err := env.accountSvc.GetByName("Mortgage"); err == nil {
		t.Error("undo did not remove the loan account")
	}
	all, _ := env.schedSvc.List()
	if len(all) != 0 {
		t.Errorf("undo left %d schedules, want 0", len(all))
	}
}

func TestLoanWizard_WithAssetAccount(t *testing.T) {
	env := newLoanWizardEnv(t)

	env.set(loanFieldName, "Mortgage")
	env.set(loanFieldCurrentBalance, "300000")
	env.set(loanFieldAPR, "5")
	env.set(loanFieldPayment, "1800")
	env.set(loanFieldNextPaymentDate, "09/15/2026")
	env.selectOption(t, loanFieldFromAccount, "Checking")
	env.app.loanWizard.Fields()[loanFieldTrackAsset].Checked = true
	env.set(loanFieldAssetName, "123 Main St")
	env.set(loanFieldAssetValue, "450000")

	msg := env.submit(t)
	if _, ok := msg.(loanWizardSavedMsg); !ok {
		t.Fatalf("submit msg = %T, want loanWizardSavedMsg", msg)
	}

	asset, err := env.accountSvc.GetByName("123 Main St")
	if err != nil {
		t.Fatalf("asset account not created: %v", err)
	}
	if asset.Type != account.TypeAsset {
		t.Errorf("asset type = %v, want asset", asset.Type)
	}
	if want := types.MustNewMoney("450000"); !asset.OpeningBalance.Equal(want) {
		t.Errorf("asset opening balance = %s, want %s", asset.OpeningBalance, want)
	}

	// One atomic step covering loan account + asset account + schedule.
	if env.app.undoManager.UndoLen() != 1 {
		t.Fatalf("undo stack = %d, want 1", env.app.undoManager.UndoLen())
	}
	if _, err := env.app.undoManager.Undo(); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if _, err := env.accountSvc.GetByName("123 Main St"); err == nil {
		t.Error("undo did not remove the asset account")
	}
	if _, err := env.accountSvc.GetByName("Mortgage"); err == nil {
		t.Error("undo did not remove the loan account")
	}
}

func TestLoanWizard_ZeroRateOmitsInterestLine(t *testing.T) {
	env := newLoanWizardEnv(t)

	env.set(loanFieldName, "Car Loan")
	env.set(loanFieldCurrentBalance, "32000")
	env.set(loanFieldAPR, "0")
	env.set(loanFieldPayment, "533.34")
	env.set(loanFieldNextPaymentDate, "08/01/2026")
	env.selectOption(t, loanFieldFromAccount, "Checking")

	// Interest category field is hidden at 0% APR.
	env.app.refreshLoanWizardDerived()
	if !env.app.loanWizard.Fields()[loanFieldInterestCategory].Hidden {
		t.Error("interest category field should be hidden at 0% APR")
	}

	msg := env.submit(t)
	if _, ok := msg.(loanWizardSavedMsg); !ok {
		t.Fatalf("submit msg = %T (%v), want loanWizardSavedMsg", msg, msg)
	}

	st := env.findLoanSchedule(t)
	for _, sp := range st.Splits {
		if sp.LoanSection.Valid && sp.LoanSection.String == scheduled.LoanSectionInterest {
			t.Error("0% loan should have no interest line")
		}
	}
	if len(st.Splits) != 1 {
		t.Errorf("0%% loan schedule has %d splits, want 1 (principal only)", len(st.Splits))
	}
}

func TestLoanWizard_PaymentPrefill(t *testing.T) {
	env := newLoanWizardEnv(t)

	// With original principal + APR + term present, the payment prefills.
	env.set(loanFieldOrigPrincipal, "380000")
	env.set(loanFieldAPR, "6.5")
	env.set(loanFieldTermMonths, "360")
	env.app.refreshLoanWizardDerived()

	payment := env.app.loanWizard.Fields()[loanFieldPayment]
	if payment.Value != "2401.86" {
		t.Errorf("prefilled payment = %q, want 2401.86", payment.Value)
	}

	// A user edit sticks: further prefill recomputes do not overwrite it.
	payment.Value = "2400.00"
	env.set(loanFieldTermMonths, "180") // would recompute to a different value
	env.app.refreshLoanWizardDerived()
	if payment.Value != "2400.00" {
		t.Errorf("payment overwrote a user edit: %q, want 2400.00", payment.Value)
	}
}

func TestLoanWizard_ValidationErrorsBlockSave(t *testing.T) {
	env := newLoanWizardEnv(t)

	// Missing name, balance, payment, date.
	env.set(loanFieldAPR, "6.5")
	msg := env.submit(t)
	if msg != nil {
		t.Fatalf("submit returned a command despite validation errors: %T", msg)
	}
	if env.app.loanWizard == nil {
		t.Fatal("wizard should stay open on validation failure")
	}
	if env.app.loanWizard.Fields()[loanFieldName].Error == "" {
		t.Error("expected a name-required error")
	}
	if env.app.undoManager.UndoLen() != 0 {
		t.Errorf("nothing should have been created: undo len = %d", env.app.undoManager.UndoLen())
	}
	if all, _ := env.schedSvc.List(); len(all) != 0 {
		t.Errorf("no schedule should exist, got %d", len(all))
	}
}

func TestLoanWizard_NegativeAmortizationBlocked(t *testing.T) {
	env := newLoanWizardEnv(t)
	env.set(loanFieldName, "Mortgage")
	env.set(loanFieldCurrentBalance, "380000")
	env.set(loanFieldAPR, "6.5")
	env.set(loanFieldPayment, "100") // < month-one interest
	env.set(loanFieldNextPaymentDate, "08/01/2026")
	env.selectOption(t, loanFieldFromAccount, "Checking")

	msg := env.submit(t)
	if msg != nil {
		t.Fatalf("submit should fail validation, got %T", msg)
	}
	if env.app.loanWizard.Fields()[loanFieldPayment].Error == "" {
		t.Error("expected a payment error for negative amortization")
	}
}

func TestLoanWizard_MidLifeOpeningDateIsToday(t *testing.T) {
	env := newLoanWizardEnv(t)
	env.set(loanFieldName, "Mortgage")
	env.set(loanFieldCurrentBalance, "312450.22") // mid-life: balance != original principal
	env.set(loanFieldAPR, "6.5")
	env.set(loanFieldOrigPrincipal, "380000")
	env.set(loanFieldOpenDate, "01/01/2020")
	env.set(loanFieldTermMonths, "360")
	env.set(loanFieldNextPaymentDate, "08/01/2026")
	env.selectOption(t, loanFieldFromAccount, "Checking")
	// clear the prefilled payment to a known editable value
	env.app.loanWizard.Fields()[loanFieldPayment].Value = "2401.86"

	if _, ok := env.submit(t).(loanWizardSavedMsg); !ok {
		t.Fatal("expected save")
	}
	loanAcct, err := env.accountSvc.GetByName("Mortgage")
	if err != nil {
		t.Fatalf("loan account: %v", err)
	}
	if !loanAcct.OpeningDate.Equal(types.Today()) {
		t.Errorf("mid-life opening date = %v, want today %v", loanAcct.OpeningDate, types.Today())
	}
}

func TestLoanWizard_OriginationOpeningDateUsesOpenDate(t *testing.T) {
	env := newLoanWizardEnv(t)
	openDate := types.NewDate(2026, time.July, 1)
	env.set(loanFieldName, "Car Loan")
	env.set(loanFieldCurrentBalance, "32000") // == original principal → origination
	env.set(loanFieldAPR, "5.9")
	env.set(loanFieldOrigPrincipal, "32000")
	env.set(loanFieldOpenDate, "07/01/2026")
	env.set(loanFieldTermMonths, "60")
	env.set(loanFieldNextPaymentDate, "08/01/2026")
	env.selectOption(t, loanFieldFromAccount, "Checking")

	if _, ok := env.submit(t).(loanWizardSavedMsg); !ok {
		t.Fatal("expected save")
	}
	loanAcct, err := env.accountSvc.GetByName("Car Loan")
	if err != nil {
		t.Fatalf("loan account: %v", err)
	}
	if !loanAcct.OpeningDate.Equal(openDate) {
		t.Errorf("origination opening date = %v, want %v", loanAcct.OpeningDate, openDate)
	}
}

func TestLoanWizard_AtomicRollbackOnAssetCollision(t *testing.T) {
	env := newLoanWizardEnv(t)

	// Pre-create an account whose name collides with the asset the wizard will
	// try to create second — forcing the second sub-command to fail after the
	// loan account (first sub-command) already succeeded.
	dup := account.NewAccount("123 Main St", account.TypeAsset, "USD", types.MustNewMoney("400000"), types.Today())
	if err := env.accountSvc.Create(dup); err != nil {
		t.Fatalf("seed dup asset: %v", err)
	}

	env.set(loanFieldName, "Mortgage")
	env.set(loanFieldCurrentBalance, "300000")
	env.set(loanFieldAPR, "5")
	env.set(loanFieldPayment, "1800")
	env.set(loanFieldNextPaymentDate, "09/15/2026")
	env.selectOption(t, loanFieldFromAccount, "Checking")
	env.app.loanWizard.Fields()[loanFieldTrackAsset].Checked = true
	env.set(loanFieldAssetName, "123 Main St")
	env.set(loanFieldAssetValue, "450000")

	msg := env.submit(t)
	if _, ok := msg.(errMsg); !ok {
		t.Fatalf("submit msg = %T, want errMsg (asset collision)", msg)
	}

	// Rollback: the loan account and schedule must NOT persist, and there is no
	// undo step (the compound's Execute failed and was rolled back).
	if _, err := env.accountSvc.GetByName("Mortgage"); err == nil {
		t.Error("loan account should have been rolled back")
	}
	if all, _ := env.schedSvc.List(); len(all) != 0 {
		t.Errorf("schedule should have been rolled back, got %d", len(all))
	}
	if env.app.undoManager.UndoLen() != 0 {
		t.Errorf("failed atomic op should push no undo step, got %d", env.app.undoManager.UndoLen())
	}
}
