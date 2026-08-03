package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/tui/dialog"
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
	// Transfer occurrences post through the transfer owner; production wires
	// this in app.NewServices (see scheduled/transfer_port.go).
	schedSvc.SetTransferPort(transfer.NewService(txnRepo,
		investment.NewRepository(database), transaction.NewSplitRepository(database), accountRepo,
		category.NewRepository(database), database))
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

// --- Edit-as-loan + demotion guard (Phase 7, part 2) ---

// seedLoanAccount creates an active loan account with the given owed balance
// (stored negated) and APR.
func (env *loanWizardEnv) seedLoanAccount(t *testing.T, name, owed, apr string) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, account.TypeLoan, "USD", types.MustNewMoney(owed).Neg(), types.NewDate(2020, time.January, 1))
	acct.SetInterestRate(types.MustNewMoney(apr))
	if err := env.accountSvc.Create(acct); err != nil {
		t.Fatalf("seed loan account: %v", err)
	}
	return acct
}

// seedLoanShapedSchedule creates a strictly loan-shaped monthly schedule using
// BuildLoanSnapshot (tagged splits) — bypassing the undo manager.
func (env *loanWizardEnv) seedLoanShapedSchedule(t *testing.T, loanAcct *account.Account, owed, apr, pi string, interestCatID types.ID, next types.Date) *scheduled.Transaction {
	t.Helper()
	parent, splits, _, err := scheduled.BuildLoanSnapshot(scheduled.LoanSnapshotInput{
		LoanAccountID: loanAcct.ID,
		APR:           types.MustNewMoney(apr),
		Owed:          types.MustNewMoney(owed),
		PIPayment:     types.MustNewMoney(pi),
		InterestCatID: interestCatID,
	})
	if err != nil {
		t.Fatalf("BuildLoanSnapshot: %v", err)
	}
	st := scheduled.NewTransactionWithAmount(env.funding.ID, scheduled.FrequencyMonthly, next, parent)
	st.SetDayOfMonth(next.Time().Day())
	st.Splits = scheduled.SplitCollection(splits)
	if err := env.schedSvc.Create(st); err != nil {
		t.Fatalf("create loan-shaped schedule: %v", err)
	}
	return st
}

func hasEditAsLoanButton(d *dialog.Dialog) bool {
	for _, b := range d.Buttons() {
		if b.Label == "Edit as loan →" {
			return true
		}
	}
	return false
}

func loanScheduleSplit(st *scheduled.Transaction, section string) *scheduled.Split {
	for _, sp := range st.Splits {
		if sp.LoanSection.Valid && sp.LoanSection.String == section {
			return sp
		}
	}
	return nil
}

func TestLoanWizard_EditRoundTrip(t *testing.T) {
	env := newLoanWizardEnv(t)

	// Create a loan via the wizard (round-trip starts from a real created loan).
	env.set(loanFieldName, "Mortgage")
	env.set(loanFieldCurrentBalance, "380000")
	env.set(loanFieldAPR, "6.5")
	env.set(loanFieldPayment, "2401.86")
	env.set(loanFieldNextPaymentDate, "08/01/2026")
	env.selectOption(t, loanFieldFromAccount, "Checking")
	if _, ok := env.submit(t).(loanWizardSavedMsg); !ok {
		t.Fatal("create failed")
	}

	st := env.findLoanSchedule(t)
	loanAcct, err := env.accountSvc.GetByName("Mortgage")
	if err != nil {
		t.Fatalf("loan account: %v", err)
	}
	owed, _ := env.accountSvc.BalanceAsOf(loanAcct.ID, st.NextDate)

	accounts, _ := env.accountSvc.List(true)
	cats, _ := env.categorySvc.List()
	env.app.loanWizard, env.app.loanWizardState = buildEditLoanWizard(accounts, cats, st, owed.Neg())

	// Prefill: name / APR / P&I; balance and next-date hidden in edit mode.
	f := env.app.loanWizard.Fields()
	if f[loanFieldName].Value != "Mortgage" {
		t.Errorf("name prefill = %q", f[loanFieldName].Value)
	}
	if f[loanFieldAPR].Value != "6.5" {
		t.Errorf("APR prefill = %q", f[loanFieldAPR].Value)
	}
	if f[loanFieldPayment].Value != "2401.86" {
		t.Errorf("P&I prefill = %q", f[loanFieldPayment].Value)
	}
	if !f[loanFieldCurrentBalance].Hidden || !f[loanFieldNextPaymentDate].Hidden {
		t.Error("current balance and next-payment-date should be hidden in edit mode")
	}

	// Change APR and P&I, then save.
	f[loanFieldAPR].Value = "7"
	f[loanFieldPayment].Value = "2500"
	if _, ok := env.submit(t).(loanWizardSavedMsg); !ok {
		t.Fatal("edit save failed")
	}

	// Account APR updated.
	loanAcct2, _ := env.accountSvc.GetByName("Mortgage")
	if !loanAcct2.InterestRate.Money.Equal(types.MustNewMoney("7")) {
		t.Errorf("APR after edit = %v, want 7", loanAcct2.InterestRate)
	}

	// Schedule still loan-shaped, interest recomputed at the new APR:
	// round(380000 * 7 / 1200) = 2216.67.
	st2 := env.findLoanSchedule(t)
	if !env.schedSvc.IsLoanShaped(st2) {
		t.Error("schedule should still be loan-shaped after edit")
	}
	interest := loanScheduleSplit(st2, scheduled.LoanSectionInterest)
	if interest == nil || !interest.Amount.Equal(types.MustNewMoney("-2216.67")) {
		t.Errorf("recomputed interest = %v, want -2216.67", interest)
	}

	// Single undo step for the edit restores the account (and schedule) together.
	if env.app.undoManager.UndoLen() != 2 {
		t.Fatalf("undo len = %d, want 2 (create + edit)", env.app.undoManager.UndoLen())
	}
	if _, err := env.app.undoManager.Undo(); err != nil {
		t.Fatalf("undo: %v", err)
	}
	loanAcct3, _ := env.accountSvc.GetByName("Mortgage")
	if !loanAcct3.InterestRate.Money.Equal(types.MustNewMoney("6.5")) {
		t.Errorf("undo did not restore APR: %v, want 6.5", loanAcct3.InterestRate)
	}
}

func TestLoanWizard_EditAdoptionTagsUntaggedSchedule(t *testing.T) {
	env := newLoanWizardEnv(t)
	loanAcct := env.seedLoanAccount(t, "Mortgage", "380000", "6.5")
	interestCat, err := env.categorySvc.GetOrCreateLoanInterestCategory()
	if err != nil {
		t.Fatalf("interest cat: %v", err)
	}

	// Hand-built, UNtagged loan schedule (interest categorized to Loan:Interest,
	// principal transfer to the loan) — loan-adoptable, not loan-shaped.
	interest := scheduled.NewCategorizedSplit(types.NilID, interestCat.ID, types.MustNewMoney("-2058.33"))
	principal := scheduled.NewTransferSplit(types.NilID, loanAcct.ID, types.MustNewMoney("-343.53"))
	st := scheduled.NewTransactionWithAmount(env.funding.ID, scheduled.FrequencyMonthly, types.NewDate(2026, time.August, 1), types.MustNewMoney("-2401.86"))
	st.SetDayOfMonth(1)
	st.Splits = scheduled.SplitCollection{interest, principal}
	if err := env.schedSvc.Create(st); err != nil {
		t.Fatalf("create untagged schedule: %v", err)
	}

	if env.schedSvc.IsLoanShaped(st) {
		t.Fatal("untagged schedule should not be loan-shaped")
	}
	if !env.schedSvc.IsLoanAdoptable(st) {
		t.Fatal("untagged schedule should be loan-adoptable")
	}

	accounts, _ := env.accountSvc.List(true)
	cats, _ := env.categorySvc.List()
	owed, _ := env.accountSvc.BalanceAsOf(loanAcct.ID, st.NextDate)
	env.app.loanWizard, env.app.loanWizardState = buildEditLoanWizard(accounts, cats, st, owed.Neg())

	// P&I prefill = |principal| + |interest| = 343.53 + 2058.33 = 2401.86.
	if got := env.app.loanWizard.Fields()[loanFieldPayment].Value; got != "2401.86" {
		t.Errorf("P&I prefill = %q, want 2401.86", got)
	}

	if _, ok := env.submit(t).(loanWizardSavedMsg); !ok {
		t.Fatal("adoption save failed")
	}

	st2 := env.findLoanSchedule(t)
	if !env.schedSvc.IsLoanShaped(st2) {
		t.Error("schedule should be loan-shaped (adopted) after save")
	}
	for _, sp := range st2.Splits {
		if !sp.LoanSection.Valid {
			t.Errorf("adopted split missing loan_section tag: %+v", sp)
		}
	}
}

func TestLoanWizard_EditAsLoanButtonAndDispatch(t *testing.T) {
	env := newLoanWizardEnv(t)
	loanAcct := env.seedLoanAccount(t, "Mortgage", "380000", "6.5")
	interestCat, _ := env.categorySvc.GetOrCreateLoanInterestCategory()
	st := env.seedLoanShapedSchedule(t, loanAcct, "380000", "6.5", "2401.86", interestCat.ID, types.NewDate(2026, time.August, 1))

	if !env.app.scheduleWantsLoanEdit(st) {
		t.Error("loan-shaped schedule should want a loan edit")
	}

	env.app.schedDialog = dialog.NewDialog("Edit Scheduled Transaction")
	env.app.maybeAddEditAsLoanButton(st)
	if !hasEditAsLoanButton(env.app.schedDialog) {
		t.Error("Edit as loan → button was not added for a loan-shaped schedule")
	}

	// A plain single-line schedule neither wants a loan edit nor gets the button.
	plain := scheduled.NewTransactionWithAmount(env.funding.ID, scheduled.FrequencyMonthly, types.NewDate(2026, time.August, 1), types.MustNewMoney("-100"))
	plain.SetCategory(interestCat.ID)
	if err := env.schedSvc.Create(plain); err != nil {
		t.Fatalf("create plain schedule: %v", err)
	}
	if env.app.scheduleWantsLoanEdit(plain) {
		t.Error("plain schedule should not want a loan edit")
	}
}

func TestLoanWizard_DemotionGuardOnGenericSplitEdit(t *testing.T) {
	env := newLoanWizardEnv(t)
	loanAcct := env.seedLoanAccount(t, "Mortgage", "380000", "6.5")
	interestCat, _ := env.categorySvc.GetOrCreateLoanInterestCategory()
	st := env.seedLoanShapedSchedule(t, loanAcct, "380000", "6.5", "2401.86", interestCat.ID, types.NewDate(2026, time.August, 1))

	accounts, _ := env.accountSvc.List(true)
	cats, _ := env.categorySvc.List()
	catOptions, catIDs := buildCategoryOptions(cats)
	accountOptions, accountIDs := buildSplitTransferAccountOptions(accounts)

	// Simulate the generic split editor open on this loan-shaped schedule.
	env.app.pendingSplitScheduled = &pendingSplitScheduled{
		mode:      scheduledDialogModeEdit,
		existing:  st,
		accountID: env.funding.ID,
		amount:    st.Amount.Money,
		frequency: scheduled.FrequencyMonthly,
		interval:  1,
		startDate: st.StartDate,
	}
	env.app.splitDialog = NewSplitDialogFromExisting(st.Amount.Money, catOptions, catIDs, transactionSplitsFromScheduled(st))
	env.app.splitDialog.SetTransferTargets(accountOptions, accountIDs, env.funding.ID)

	// Save through the generic editor → the demotion guard fires; nothing saved.
	_, cmd := env.app.submitScheduledSplitDialog()
	if env.app.confirmDialog == nil {
		t.Fatal("demotion guard did not fire (no confirm dialog)")
	}
	if cmd != nil {
		t.Error("save should be deferred behind the confirm dialog")
	}
	if reloaded, _ := env.schedSvc.GetByID(st.ID); !env.schedSvc.IsLoanShaped(reloaded) {
		t.Error("schedule must stay loan-shaped until the user confirms")
	}

	// Confirm → the deferred save runs and demotes the schedule.
	fn := env.app.confirmAction
	if fn == nil {
		t.Fatal("no confirm action captured")
	}
	if _, ok := fn().(scheduledDialogSavedMsg); !ok {
		t.Fatal("deferred demoting save did not succeed")
	}
	reloaded, _ := env.schedSvc.GetByID(st.ID)
	if env.schedSvc.IsLoanShaped(reloaded) {
		t.Error("schedule should be demoted (loan_section tags stripped) after confirm")
	}
}

// --- Inline category creation (Phase 7 deferral) ---

// selectedLabel returns the currently-selected option label of a combo field.
func (env *loanWizardEnv) selectedLabel(idx int) string {
	f := env.app.loanWizard.Fields()[idx]
	if f.SelectedIndex < 0 || f.SelectedIndex >= len(f.Options) {
		return ""
	}
	return f.Options[f.SelectedIndex]
}

// openAddNewFromLoan focuses fieldIdx, seeds its combo query, and diverts into
// the create-category sub-dialog exactly as the DialogActionAddNew path does.
func (env *loanWizardEnv) openAddNewFromLoan(t *testing.T, fieldIdx int, query string) {
	t.Helper()
	env.app.loanWizard.Fields()[fieldIdx].Query = query
	env.app.loanWizard.SetFocusIndex(fieldIdx)
	env.app.openCreateCategorySubDialogFromLoan()
	if env.app.createCatDialog == nil {
		t.Fatal("create-category sub-dialog was not opened")
	}
}

func TestLoanWizard_CategoryFieldsAreCombosWithAddNew(t *testing.T) {
	env := newLoanWizardEnv(t)
	fields := env.app.loanWizard.Fields()

	for _, idx := range []int{loanFieldInterestCategory, loanFieldPrincipalCategory, loanEscrowCatIndex(0), loanEscrowCatIndex(loanMaxEscrowLines - 1)} {
		f := fields[idx]
		if f.Type != dialog.FieldCombo {
			t.Errorf("field %d type = %v, want FieldCombo", idx, f.Type)
		}
		if f.AddNewLabel != loanAddNewCategoryLabel {
			t.Errorf("field %d AddNewLabel = %q, want %q", idx, f.AddNewLabel, loanAddNewCategoryLabel)
		}
	}
}

func TestLoanWizard_OpenAddNewSeedsSubDialogAndHidesWizard(t *testing.T) {
	env := newLoanWizardEnv(t)
	env.openAddNewFromLoan(t, loanEscrowCatIndex(0), "Housing:PMI")

	if env.app.createCatSource != createCatSourceLoanWizard {
		t.Errorf("createCatSource = %v, want loan wizard", env.app.createCatSource)
	}
	if env.app.createCatLoanField != loanEscrowCatIndex(0) {
		t.Errorf("createCatLoanField = %d, want %d", env.app.createCatLoanField, loanEscrowCatIndex(0))
	}
	if env.app.loanWizard.IsVisible() {
		t.Error("loan wizard should be hidden while the sub-dialog is open")
	}
	// The sub-dialog is seeded from the typed "Parent:Name" query.
	sub := env.app.createCatDialog.Fields()
	if sub[0].Value != "PMI" {
		t.Errorf("sub-dialog Name = %q, want PMI", sub[0].Value)
	}
	// The originating combo's stale query was consumed.
	if q := env.app.loanWizard.Fields()[loanEscrowCatIndex(0)].Query; q != "" {
		t.Errorf("originating combo Query = %q, want cleared", q)
	}
}

func TestLoanWizard_CancelAddNewRestoresWizard(t *testing.T) {
	env := newLoanWizardEnv(t)
	env.openAddNewFromLoan(t, loanFieldInterestCategory, "")
	env.app.cancelCreateCatDialog()

	if env.app.createCatDialog != nil {
		t.Error("sub-dialog should be cleared on cancel")
	}
	if !env.app.loanWizard.IsVisible() {
		t.Error("loan wizard should be re-shown on cancel")
	}
	if env.app.createCatLoanField != -1 {
		t.Errorf("createCatLoanField = %d, want -1 after cancel", env.app.createCatLoanField)
	}
}

func TestLoanWizard_CreateCategoryFromEscrowSelectsAndReveals(t *testing.T) {
	env := newLoanWizardEnv(t)
	env.openAddNewFromLoan(t, loanEscrowCatIndex(0), "PMI")

	if err := env.app.applyCreatedCategory(createCategoryRequest{
		Name: "PMI", Type: category.TypeExpense,
	}); err != nil {
		t.Fatalf("applyCreatedCategory: %v", err)
	}

	// Wizard re-shown, sub-dialog cleared, source reset.
	if !env.app.loanWizard.IsVisible() {
		t.Error("wizard should be re-shown after create")
	}
	if env.app.createCatDialog != nil || env.app.createCatLoanField != -1 {
		t.Error("create-category scratch state should be reset")
	}
	// The escrow row now selects the freshly-created category.
	if got := env.selectedLabel(loanEscrowCatIndex(0)); got != "PMI" {
		t.Errorf("escrow row 0 selection = %q, want PMI", got)
	}
	// The category was persisted.
	if _, err := env.categorySvc.GetByName("PMI", nil); err != nil {
		t.Errorf("PMI category not persisted: %v", err)
	}
	// Selecting a category on escrow row 0 reveals row 1.
	if env.app.loanWizard.Fields()[loanEscrowCatIndex(1)].Hidden {
		t.Error("escrow row 1 should be revealed after row 0 has a category")
	}
}

// TestLoanWizard_CreateCategoryPreservesOtherSelectionsByID is the key
// correctness test: inserting a new category shifts the sorted option indices,
// so a previously-filled escrow row (and the interest field) must be re-resolved
// by ID, not by their stale positional index.
func TestLoanWizard_CreateCategoryPreservesOtherSelectionsByID(t *testing.T) {
	env := newLoanWizardEnv(t)

	// Pre-select an interest category and escrow row 0 = Housing > Property Tax.
	env.selectOption(t, loanFieldInterestCategory, "Housing > Home Insurance")
	env.selectOption(t, loanEscrowCatIndex(0), "Housing > Property Tax")

	// Create a new top-level category that sorts BEFORE the Housing entries
	// ("AAA …" < "Housing …"), from escrow row 1 — this shifts every later index.
	env.openAddNewFromLoan(t, loanEscrowCatIndex(1), "AAA Flood Insurance")
	if err := env.app.applyCreatedCategory(createCategoryRequest{
		Name: "AAA Flood Insurance", Type: category.TypeExpense,
	}); err != nil {
		t.Fatalf("applyCreatedCategory: %v", err)
	}

	// The triggering row selects the new category.
	if got := env.selectedLabel(loanEscrowCatIndex(1)); got != "AAA Flood Insurance" {
		t.Errorf("escrow row 1 selection = %q, want AAA Flood Insurance", got)
	}
	// Escrow row 0's selection survived the index shift (by ID, not position).
	if got := env.selectedLabel(loanEscrowCatIndex(0)); got != "Housing > Property Tax" {
		t.Errorf("escrow row 0 selection = %q, want Housing > Property Tax (preserved by ID)", got)
	}
	// The interest field's selection likewise survived.
	if got := env.selectedLabel(loanFieldInterestCategory); got != "Housing > Home Insurance" {
		t.Errorf("interest selection = %q, want Housing > Home Insurance (preserved by ID)", got)
	}
}

func TestLoanWizard_CreateInterestCategoryFromInterestCombo(t *testing.T) {
	env := newLoanWizardEnv(t)
	env.set(loanFieldAPR, "6.5") // interest field visible
	env.app.refreshLoanWizardDerived()

	env.openAddNewFromLoan(t, loanFieldInterestCategory, "Mortgage Interest")
	if err := env.app.applyCreatedCategory(createCategoryRequest{
		Name: "Mortgage Interest", Type: category.TypeExpense,
	}); err != nil {
		t.Fatalf("applyCreatedCategory: %v", err)
	}
	if got := env.selectedLabel(loanFieldInterestCategory); got != "Mortgage Interest" {
		t.Errorf("interest selection = %q, want Mortgage Interest", got)
	}

	// The new interest category flows through to a saved schedule.
	env.set(loanFieldName, "Mortgage")
	env.set(loanFieldCurrentBalance, "300000")
	env.set(loanFieldPayment, "2000")
	env.set(loanFieldNextPaymentDate, "08/01/2026")
	env.selectOption(t, loanFieldFromAccount, "Checking")
	if msg := env.submit(t); msg == nil {
		t.Fatal("submit failed after inline interest-category creation")
	}
	st := env.findLoanSchedule(t)
	interestCat, err := env.categorySvc.GetByName("Mortgage Interest", nil)
	if err != nil {
		t.Fatalf("interest category not found: %v", err)
	}
	var found bool
	for _, sp := range st.Splits {
		if sp.LoanSection.Valid && sp.LoanSection.String == scheduled.LoanSectionInterest {
			found = true
			if !sp.CategoryID.Valid || sp.CategoryID.ID != interestCat.ID {
				t.Errorf("interest line category = %v, want %v", sp.CategoryID, interestCat.ID)
			}
		}
	}
	if !found {
		t.Error("no interest line in the saved schedule")
	}
}

// TestLoanWizard_EditPrefilledComboSurvivesTab is the regression guard for the
// combo-prefill bug: converting the pickers to combos meant a prefilled
// selection (Edit-as-loan) whose ComboHighlight still pointed at row 0 was
// silently reset to "(None)"/first-category the first time the user tabbed over
// it — silent data loss on save. setSelectByID now syncs ComboHighlight.
func TestLoanWizard_EditPrefilledComboSurvivesTab(t *testing.T) {
	env := newLoanWizardEnv(t)

	// Create a loan; its interest defaults to the get-or-created Loan:Interest.
	env.set(loanFieldName, "Mortgage")
	env.set(loanFieldCurrentBalance, "380000")
	env.set(loanFieldAPR, "6.5")
	env.set(loanFieldPayment, "2401.86")
	env.set(loanFieldNextPaymentDate, "08/01/2026")
	env.selectOption(t, loanFieldFromAccount, "Checking")
	if _, ok := env.submit(t).(loanWizardSavedMsg); !ok {
		t.Fatal("create failed")
	}

	// Reopen in Edit-as-loan mode: the interest combo is prefilled to the real
	// Loan > Interest category (not at index 0 of the options).
	st := env.findLoanSchedule(t)
	loanAcct, _ := env.accountSvc.GetByName("Mortgage")
	owed, _ := env.accountSvc.BalanceAsOf(loanAcct.ID, st.NextDate)
	accounts, _ := env.accountSvc.List(true)
	cats, _ := env.categorySvc.List()
	env.app.loanWizard, env.app.loanWizardState = buildEditLoanWizard(accounts, cats, st, owed.Neg())

	interest := env.app.loanWizard.Fields()[loanFieldInterestCategory]
	if env.selectedLabel(loanFieldInterestCategory) != loanInterestDefaultDisplay {
		t.Fatalf("interest prefill = %q, want %q", env.selectedLabel(loanFieldInterestCategory), loanInterestDefaultDisplay)
	}
	if interest.ComboHighlight != interest.SelectedIndex {
		t.Errorf("ComboHighlight=%d != SelectedIndex=%d after prefill (a Tab would reset the selection)",
			interest.ComboHighlight, interest.SelectedIndex)
	}

	// Tabbing over the prefilled combo must not reset it.
	env.app.loanWizard.SetFocusIndex(loanFieldInterestCategory)
	env.app.loanWizard.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if got := env.selectedLabel(loanFieldInterestCategory); got != loanInterestDefaultDisplay {
		t.Errorf("interest selection after Tab = %q, want %q (regression: prefilled combo reset)", got, loanInterestDefaultDisplay)
	}
}

// TestLoanWizard_CreateLoanInterestFromEscrowKeepsDefault guards the
// synthetic-default edge case: when the interest combo sits on the synthetic
// "Loan > Interest" (NilID) default and the user creates the *real*
// Loan:Interest category from a different (escrow) field, the rebuild drops the
// synthetic NilID row — so the interest field must fall back to the rebuilt
// default index, not to the first-alphabetical category.
func TestLoanWizard_CreateLoanInterestFromEscrowKeepsDefault(t *testing.T) {
	env := newLoanWizardEnv(t)
	env.set(loanFieldAPR, "6.5") // interest field visible
	env.app.refreshLoanWizardDerived()

	if env.selectedLabel(loanFieldInterestCategory) != loanInterestDefaultDisplay {
		t.Fatalf("interest should start on the synthetic default, got %q", env.selectedLabel(loanFieldInterestCategory))
	}

	// Create the real Loan:Interest category from an escrow field.
	env.openAddNewFromLoan(t, loanEscrowCatIndex(0), "Loan:Interest")
	if err := env.app.applyCreatedCategory(createCategoryRequest{
		Name: "Interest", ParentName: "Loan", NewParent: true, Type: category.TypeExpense,
	}); err != nil {
		t.Fatalf("applyCreatedCategory: %v", err)
	}

	// The interest field still resolves to Loan > Interest — not the first
	// alphabetical category it would fall through to without the default fallback.
	if got := env.selectedLabel(loanFieldInterestCategory); got != loanInterestDefaultDisplay {
		t.Errorf("interest selection = %q, want %q (default preserved across synthetic-row collapse)", got, loanInterestDefaultDisplay)
	}
	// The state ID list stays consistent with the field index (the save logic
	// trusts state.interestIDs[SelectedIndex]).
	idx := env.app.loanWizard.Fields()[loanFieldInterestCategory].SelectedIndex
	st := env.app.loanWizardState
	if idx < 0 || idx >= len(st.interestIDs) {
		t.Fatalf("interest SelectedIndex %d out of range for interestIDs (len %d)", idx, len(st.interestIDs))
	}
	realCat, _ := env.categorySvc.GetByName("Interest", ptrParentID(t, env, "Loan"))
	if st.interestIDs[idx] != realCat.ID {
		t.Errorf("interest resolves to ID %v, want the real Loan:Interest %v", st.interestIDs[idx], realCat.ID)
	}
}

// ptrParentID looks up a top-level parent category by name and returns a pointer
// to its ID, for resolving a child via categorySvc.GetByName.
func ptrParentID(t *testing.T, env *loanWizardEnv, parent string) *types.ID {
	t.Helper()
	p, err := env.categorySvc.GetByName(parent, nil)
	if err != nil {
		t.Fatalf("parent %q not found: %v", parent, err)
	}
	return &p.ID
}

// --- Principal category (Phase 9) ---

func TestLoanWizard_DefaultPrincipalCategoryOnCreate(t *testing.T) {
	env := newLoanWizardEnv(t)
	env.set(loanFieldName, "Mortgage")
	env.set(loanFieldCurrentBalance, "380000")
	env.set(loanFieldAPR, "6.5")
	env.set(loanFieldPayment, "2401.86")
	env.set(loanFieldNextPaymentDate, "08/01/2026")
	env.selectOption(t, loanFieldFromAccount, "Checking")

	// The principal combo defaults to the synthetic Loan > Principal row.
	if got := env.selectedLabel(loanFieldPrincipalCategory); got != loanPrincipalDefaultDisplay {
		t.Fatalf("principal default = %q, want %q", got, loanPrincipalDefaultDisplay)
	}

	if _, ok := env.submit(t).(loanWizardSavedMsg); !ok {
		t.Fatal("create failed")
	}

	// Loan:Principal was get-or-created and the principal transfer line carries it.
	loanParent, err := env.categorySvc.GetByName("Loan", nil)
	if err != nil {
		t.Fatalf("Loan parent not created: %v", err)
	}
	principalCat, err := env.categorySvc.GetByName("Principal", &loanParent.ID)
	if err != nil {
		t.Fatalf("Loan:Principal not created: %v", err)
	}
	st := env.findLoanSchedule(t)
	p := loanScheduleSplit(st, scheduled.LoanSectionPrincipal)
	if p == nil || !p.CategoryID.Valid || p.CategoryID.ID != principalCat.ID {
		t.Errorf("principal line category = %+v, want Loan:Principal %v", p, principalCat.ID)
	}
}

func TestLoanWizard_PrincipalCategoryClearable(t *testing.T) {
	env := newLoanWizardEnv(t)
	env.set(loanFieldName, "Mortgage")
	env.set(loanFieldCurrentBalance, "380000")
	env.set(loanFieldAPR, "6.5")
	env.set(loanFieldPayment, "2401.86")
	env.set(loanFieldNextPaymentDate, "08/01/2026")
	env.selectOption(t, loanFieldFromAccount, "Checking")
	env.selectOption(t, loanFieldPrincipalCategory, "(None)") // clear the default

	if _, ok := env.submit(t).(loanWizardSavedMsg); !ok {
		t.Fatal("create failed")
	}
	st := env.findLoanSchedule(t)
	p := loanScheduleSplit(st, scheduled.LoanSectionPrincipal)
	if p == nil || p.CategoryID.Valid {
		t.Errorf("principal line category = %+v, want unset ((None) selected)", p)
	}
	// The Loan parent exists (interest default) but has no Principal child.
	if loanParent, err := env.categorySvc.GetByName("Loan", nil); err == nil {
		if _, err := env.categorySvc.GetByName("Principal", &loanParent.ID); err == nil {
			t.Error("Loan:Principal should not be created when principal is set to (None)")
		}
	}
}

func TestLoanWizard_EditRoundTripsPrincipalCategory(t *testing.T) {
	env := newLoanWizardEnv(t)
	env.set(loanFieldName, "Mortgage")
	env.set(loanFieldCurrentBalance, "380000")
	env.set(loanFieldAPR, "6.5")
	env.set(loanFieldPayment, "2401.86")
	env.set(loanFieldNextPaymentDate, "08/01/2026")
	env.selectOption(t, loanFieldFromAccount, "Checking")
	if _, ok := env.submit(t).(loanWizardSavedMsg); !ok {
		t.Fatal("create failed")
	}

	st := env.findLoanSchedule(t)
	loanAcct, _ := env.accountSvc.GetByName("Mortgage")
	owed, _ := env.accountSvc.BalanceAsOf(loanAcct.ID, st.NextDate)
	accounts, _ := env.accountSvc.List(true)
	cats, _ := env.categorySvc.List()
	env.app.loanWizard, env.app.loanWizardState = buildEditLoanWizard(accounts, cats, st, owed.Neg())

	// The principal combo prefills to the real Loan > Principal category.
	if got := env.selectedLabel(loanFieldPrincipalCategory); got != loanPrincipalDefaultDisplay {
		t.Errorf("principal prefill = %q, want %q", got, loanPrincipalDefaultDisplay)
	}

	if _, ok := env.submit(t).(loanWizardSavedMsg); !ok {
		t.Fatal("edit save failed")
	}
	st2 := env.findLoanSchedule(t)
	loanParent, _ := env.categorySvc.GetByName("Loan", nil)
	principalCat, _ := env.categorySvc.GetByName("Principal", &loanParent.ID)
	p := loanScheduleSplit(st2, scheduled.LoanSectionPrincipal)
	if p == nil || !p.CategoryID.Valid || p.CategoryID.ID != principalCat.ID {
		t.Errorf("principal line category after edit = %+v, want %v (round-tripped)", p, principalCat.ID)
	}
}

// TestLoanWizard_EditUncategorizedPrincipalStaysUnlabeled pins the round-trip
// for an old-shape loan: a principal line with no category must prefill to
// "(None)" (not silently pick up the Loan:Principal default) and stay unlabeled
// on save.
func TestLoanWizard_EditUncategorizedPrincipalStaysUnlabeled(t *testing.T) {
	env := newLoanWizardEnv(t)
	loanAcct := env.seedLoanAccount(t, "Mortgage", "380000", "6.5")
	interestCat, _ := env.categorySvc.GetOrCreateLoanInterestCategory()
	// seedLoanShapedSchedule passes no PrincipalCatID → a bare principal line.
	st := env.seedLoanShapedSchedule(t, loanAcct, "380000", "6.5", "2401.86", interestCat.ID, types.NewDate(2026, time.August, 1))

	accounts, _ := env.accountSvc.List(true)
	cats, _ := env.categorySvc.List()
	owed, _ := env.accountSvc.BalanceAsOf(loanAcct.ID, st.NextDate)
	env.app.loanWizard, env.app.loanWizardState = buildEditLoanWizard(accounts, cats, st, owed.Neg())

	if got := env.selectedLabel(loanFieldPrincipalCategory); got != "(None)" {
		t.Errorf("principal prefill = %q, want (None) for an old-shape loan", got)
	}

	if _, ok := env.submit(t).(loanWizardSavedMsg); !ok {
		t.Fatal("edit save failed")
	}
	st2 := env.findLoanSchedule(t)
	p := loanScheduleSplit(st2, scheduled.LoanSectionPrincipal)
	if p == nil || p.CategoryID.Valid {
		t.Errorf("principal line category after edit = %+v, want still unset", p)
	}
}

func TestLoanWizard_CreatePrincipalCategoryFromPrincipalCombo(t *testing.T) {
	env := newLoanWizardEnv(t)
	env.openAddNewFromLoan(t, loanFieldPrincipalCategory, "Debt:Principal")
	if err := env.app.applyCreatedCategory(createCategoryRequest{
		Name: "Principal", ParentName: "Debt", NewParent: true, Type: category.TypeExpense,
	}); err != nil {
		t.Fatalf("applyCreatedCategory: %v", err)
	}
	// The principal combo now selects the freshly-created category.
	if got := env.selectedLabel(loanFieldPrincipalCategory); got != "Debt > Principal" {
		t.Errorf("principal selection = %q, want Debt > Principal", got)
	}

	// It flows through to the saved principal line.
	env.set(loanFieldName, "Mortgage")
	env.set(loanFieldCurrentBalance, "300000")
	env.set(loanFieldAPR, "6.5")
	env.set(loanFieldPayment, "2000")
	env.set(loanFieldNextPaymentDate, "08/01/2026")
	env.selectOption(t, loanFieldFromAccount, "Checking")
	if _, ok := env.submit(t).(loanWizardSavedMsg); !ok {
		t.Fatal("submit failed after inline principal-category creation")
	}
	st := env.findLoanSchedule(t)
	debtParent, err := env.categorySvc.GetByName("Debt", nil)
	if err != nil {
		t.Fatalf("Debt parent not found: %v", err)
	}
	principalCat, err := env.categorySvc.GetByName("Principal", &debtParent.ID)
	if err != nil {
		t.Fatalf("Debt:Principal not found: %v", err)
	}
	p := loanScheduleSplit(st, scheduled.LoanSectionPrincipal)
	if p == nil || !p.CategoryID.Valid || p.CategoryID.ID != principalCat.ID {
		t.Errorf("principal line category = %+v, want Debt:Principal %v", p, principalCat.ID)
	}
}
