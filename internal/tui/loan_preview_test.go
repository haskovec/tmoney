package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/loan"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// loanPreviewEnv is a DB-backed harness for Phase 6 (post-time preview
// integration for loan-shaped schedules). It builds a funding account, a
// loan account, an interest category, a payee, and a strictly loan-shaped
// monthly schedule whose month-one snapshot is computed for openingOwed —
// so a later balance change (extra principal) makes the live recompute
// diverge from the stored template, which is exactly what recompute-at-post
// must surface.
type loanPreviewEnv struct {
	app       *App
	database  *db.DB
	txnRepo   *transaction.Repository
	splitRepo *transaction.SplitRepository
	schedRepo *scheduled.Repository
	schedSvc  *scheduled.Service
	txnSvc    *transaction.Service
	funding   *account.Account
	loan      *account.Account
	interest  *category.Category
	servicer  *payee.Payee
	dueTxn    *scheduled.Transaction
	nextDate  types.Date
	// template snapshot amounts (signed, as stored) for divergence assertions.
	tmplInterest  types.Money
	tmplPrincipal types.Money
}

// newLoanPreviewEnv builds the harness. owed/apr/pi are the loan terms used
// for the loan account balance and the month-one snapshot; apr="0" builds a
// 0% loan (no interest line).
func newLoanPreviewEnv(t *testing.T, owed, apr, pi string, nextDate types.Date) *loanPreviewEnv {
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
	loanAcct := account.NewAccount("Mortgage", account.TypeLoan, "USD", types.MustNewMoney(owed).Neg(), opened)
	loanAcct.SetInterestRate(types.MustNewMoney(apr))
	if err := accountRepo.Create(loanAcct); err != nil {
		t.Fatalf("create loan: %v", err)
	}
	interestCat := category.NewCategory("Interest", category.TypeExpense)
	if err := categoryRepo.Create(interestCat); err != nil {
		t.Fatalf("create interest category: %v", err)
	}
	servicer := payee.NewPayee("Servicer")
	if err := payeeRepo.Create(servicer); err != nil {
		t.Fatalf("create payee: %v", err)
	}

	interest, principal, _, err := loan.SplitPayment(
		types.MustNewMoney(owed), types.MustNewMoney(apr), types.MustNewMoney(pi))
	if err != nil {
		t.Fatalf("month-one SplitPayment: %v", err)
	}

	var splits scheduled.SplitCollection
	total := types.ZeroMoney
	if interest.IsPositive() {
		is := scheduled.NewCategorizedSplit(types.NewID(), interestCat.ID, interest.Neg())
		is.LoanSection = types.NullableString{String: scheduled.LoanSectionInterest, Valid: true}
		splits = append(splits, is)
		total = total.Add(interest.Neg())
	}
	ps := scheduled.NewTransferSplit(types.NewID(), loanAcct.ID, principal.Neg())
	ps.LoanSection = types.NullableString{String: scheduled.LoanSectionPrincipal, Valid: true}
	splits = append(splits, ps)
	total = total.Add(principal.Neg())

	st := scheduled.NewTransactionWithAmount(funding.ID, scheduled.FrequencyMonthly, nextDate, total)
	st.NextDate = nextDate
	st.SetDayOfMonth(nextDate.Time().Day())
	st.SetPayee(servicer.ID)
	st.SetMemo("Mortgage payment")
	for _, sp := range splits {
		sp.ScheduledTransactionID = st.ID
	}
	st.Splits = splits
	if err := schedSvc.Create(st); err != nil {
		t.Fatalf("create loan schedule: %v", err)
	}

	app := &App{
		currentView:     ViewScheduled,
		width:           120,
		height:          30,
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
		scheduled: &scheduledViewData{
			allTxns:       []*scheduled.Transaction{st},
			dueTxns:       []*scheduled.Transaction{st},
			dueCount:      1,
			payeeNames:    map[types.ID]string{servicer.ID: servicer.Name},
			accountNames:  map[types.ID]string{funding.ID: funding.Name, loanAcct.ID: loanAcct.Name},
			categoryNames: map[types.ID]string{interestCat.ID: interestCat.Name},
		},
	}
	app.buildScheduledTable()
	app.sidebar.SetFocused(false)
	app.scheduledTable.SetFocused(true)

	return &loanPreviewEnv{
		app:           app,
		database:      database,
		txnRepo:       txnRepo,
		splitRepo:     splitTxnRepo,
		schedRepo:     schedRepo,
		schedSvc:      schedSvc,
		txnSvc:        txnSvc,
		funding:       funding,
		loan:          loanAcct,
		interest:      interestCat,
		servicer:      servicer,
		dueTxn:        st,
		nextDate:      nextDate,
		tmplInterest:  interest.Neg(),
		tmplPrincipal: principal.Neg(),
	}
}

// addLoanTxn posts a plain (uncategorized) transaction on the loan account,
// used to move its balance (an ad-hoc extra-principal payment reduces the
// negative balance toward zero).
func (env *loanPreviewEnv) addLoanTxn(t *testing.T, date types.Date, amount string) {
	t.Helper()
	txn := transaction.NewTransaction(env.loan.ID, date, types.MustNewMoney(amount))
	if err := env.txnSvc.Create(txn); err != nil {
		t.Fatalf("add loan txn: %v", err)
	}
}

// openPreview drives the async loader and constructs the preview dialog from
// its result, exactly as the message loop does. It returns the loader's
// message so callers can assert on the loan-shape/blocked signal.
func (env *loanPreviewEnv) openPreview(t *testing.T) tea.Msg {
	t.Helper()
	cmd := env.app.loadSchedulePreviewData()
	if cmd == nil {
		t.Fatal("loadSchedulePreviewData returned nil cmd")
	}
	msg := cmd()
	model, _ := env.app.Update(msg)
	env.app = model.(*App)
	return msg
}

// rowAmounts returns the embedded split editor's per-row amount strings.
func (env *loanPreviewEnv) rowAmounts(t *testing.T) []string {
	t.Helper()
	sd := env.app.schedPreviewDialog.SplitDialog()
	if sd == nil {
		t.Fatal("preview should embed a SplitDialog")
	}
	out := make([]string, len(sd.rows))
	for i := range sd.rows {
		out[i] = sd.rows[i].amountField.Value
	}
	return out
}

// TestSchedulePreview_LoanSeedsComputedSplits verifies a loan-shaped preview
// seeds its lines from the live-balance recompute (ComputeLoanSplits) rather
// than the stored month-one template — an extra-principal payment before the
// preview opens must lower the seeded interest.
func TestSchedulePreview_LoanSeedsComputedSplits(t *testing.T) {
	nextDate := types.NewDate(2024, time.July, 1)
	env := newLoanPreviewEnv(t, "250000.00", "6.5", "1580.17", nextDate)

	// Ad-hoc extra principal before the payment date lowers the owed balance.
	env.addLoanTxn(t, types.NewDate(2024, time.June, 15), "50000.00")

	msg := env.openPreview(t)
	dataMsg, ok := msg.(schedulePreviewDataMsg)
	if !ok {
		t.Fatalf("loader msg = %T, want schedulePreviewDataMsg", msg)
	}
	if dataMsg.loanSplits == nil {
		t.Fatal("loader must attach loanSplits for a loan-shaped schedule")
	}

	p := env.app.schedPreviewDialog
	if p == nil || !p.IsMultiLine() {
		t.Fatal("preview should be a multi-line dialog")
	}
	if !p.IsLoanShaped() {
		t.Fatal("preview should be marked loan-shaped")
	}

	// Expected recompute at the (reduced) live balance.
	want, err := env.schedSvc.ComputeLoanSplits(env.dueTxn, nextDate)
	if err != nil {
		t.Fatalf("ComputeLoanSplits: %v", err)
	}
	// owed = 250000 - 50000 = 200000 → interest = round(200000*6.5/1200) = 1083.33.
	wantInterest := types.MustNewMoney("-1083.33")
	if !want.Interest.Neg().Equal(wantInterest) {
		t.Fatalf("recompute interest = %s, want %s", want.Interest.Neg(), wantInterest)
	}

	got := env.rowAmounts(t)
	if len(got) != 2 {
		t.Fatalf("expected 2 seeded rows, got %d: %v", len(got), got)
	}
	if got[0] != wantInterest.String() {
		t.Errorf("seeded interest row = %s, want %s (the recompute)", got[0], wantInterest)
	}
	// The template's stored interest was for owed=250000 (1354.17) — the seed
	// must NOT be that stale value.
	if got[0] == env.tmplInterest.String() {
		t.Errorf("seeded interest row = %s equals the stale template value; recompute did not run", got[0])
	}
	wantPrincipal := want.Principal.Neg().String()
	if got[1] != wantPrincipal {
		t.Errorf("seeded principal row = %s, want %s", got[1], wantPrincipal)
	}
}

// TestSchedulePreview_LoanReseedsOnDateChange verifies that editing the
// preview's Date recomputes the seed for the new occurrence date (the
// reseed-until-frozen rule).
func TestSchedulePreview_LoanReseedsOnDateChange(t *testing.T) {
	nextDate := types.NewDate(2024, time.July, 1)
	env := newLoanPreviewEnv(t, "250000.00", "6.5", "1580.17", nextDate)
	// Extra principal dated AFTER the next payment date — invisible at the
	// original date, visible once the preview date advances past it.
	env.addLoanTxn(t, types.NewDate(2024, time.July, 15), "50000.00")

	env.openPreview(t)
	p := env.app.schedPreviewDialog
	if !p.IsLoanShaped() {
		t.Fatal("preview should be loan-shaped")
	}
	// At the original date the extra principal is not yet counted.
	if got := env.rowAmounts(t)[0]; got != "-1354.17" {
		t.Fatalf("initial interest row = %s, want -1354.17", got)
	}
	if !p.loanSeedDate.Equal(nextDate) {
		t.Fatalf("loanSeedDate = %s, want %s", p.loanSeedDate, nextDate)
	}

	// Edit the Date to a month later (past the extra-principal date) and let
	// the reseed run.
	laterDate := types.NewDate(2024, time.August, 1)
	p.HeaderDialog().Fields()[previewFieldDate].Value = "08/01/2024"
	env.app.maybeReseedLoanPreview()

	if !p.loanSeedDate.Equal(laterDate) {
		t.Fatalf("after date edit loanSeedDate = %s, want %s", p.loanSeedDate, laterDate)
	}
	// owed at August = 250000 - 50000 = 200000 → interest 1083.33.
	if got := env.rowAmounts(t)[0]; got != "-1083.33" {
		t.Errorf("reseeded interest row = %s, want -1083.33", got)
	}
}

// TestSchedulePreview_LoanFreezesReseedAfterLineEdit verifies that once the
// user edits a line amount, subsequent Date edits no longer reseed (user
// values win).
func TestSchedulePreview_LoanFreezesReseedAfterLineEdit(t *testing.T) {
	nextDate := types.NewDate(2024, time.July, 1)
	env := newLoanPreviewEnv(t, "250000.00", "6.5", "1580.17", nextDate)
	env.addLoanTxn(t, types.NewDate(2024, time.July, 15), "50000.00")

	env.openPreview(t)
	p := env.app.schedPreviewDialog

	// User penny-tweaks the interest line (and rebalances principal so the
	// lines still sum). This must freeze reseeding permanently.
	sd := p.SplitDialog()
	sd.rows[0].amountField.Value = "-1354.16"
	sd.rows[1].amountField.Value = "-226.01"
	env.app.freezeLoanSeedIfEdited()
	if !p.loanSeedFrozen {
		t.Fatal("editing a line amount should freeze loan reseeding")
	}

	// A later Date edit must NOT reseed now.
	p.HeaderDialog().Fields()[previewFieldDate].Value = "08/01/2024"
	env.app.maybeReseedLoanPreview()

	if !p.loanSeedDate.Equal(nextDate) {
		t.Errorf("frozen preview reseeded to %s; date edits must be inert after a line edit", p.loanSeedDate)
	}
	if got := env.rowAmounts(t)[0]; got != "-1354.16" {
		t.Errorf("interest row = %s, want the user's -1354.16 (reseed must not overwrite edits)", got)
	}
}

// TestSchedulePreview_LoanFreezeViaKey confirms the split-editor key path
// wires freeze detection: typing into a line amount field freezes reseeding.
func TestSchedulePreview_LoanFreezeViaKey(t *testing.T) {
	nextDate := types.NewDate(2024, time.July, 1)
	env := newLoanPreviewEnv(t, "250000.00", "6.5", "1580.17", nextDate)

	env.openPreview(t)
	p := env.app.schedPreviewDialog

	// Route focus into the split editor's first amount field and type a digit.
	p.splitFocus = true
	sd := p.SplitDialog()
	sd.focus = splitFocusRows
	sd.rowIndex = 0
	sd.fieldFocus = splitFieldAmount

	if p.loanSeedFrozen {
		t.Fatal("preview should not start frozen")
	}
	env.app.handleSchedulePreviewMultiLineKey(tea.KeyPressMsg{Code: '9', Text: "9"})
	if !p.loanSeedFrozen {
		t.Error("typing into a line amount should freeze loan reseeding")
	}
}

// TestSchedulePreview_LoanPayoffPostAndToast drives a final (clamped) payment
// through the preview: the parent shrinks to the clamped draft, the schedule
// completes, and the payoff toast is surfaced.
func TestSchedulePreview_LoanPayoffPostAndToast(t *testing.T) {
	nextDate := types.NewDate(2024, time.July, 1)
	env := newLoanPreviewEnv(t, "250000.00", "6.5", "1580.17", nextDate)
	// A large ad-hoc principal payment leaves only $200 owed — one clamped
	// final payment finishes the loan.
	env.addLoanTxn(t, types.NewDate(2024, time.June, 15), "249800.00")

	env.openPreview(t)
	p := env.app.schedPreviewDialog
	if p == nil || !p.IsLoanShaped() {
		t.Fatal("preview should be loan-shaped")
	}

	// owed = 200 → interest round(200*6.5/1200)=1.08; principal clamps to 200.
	got := env.rowAmounts(t)
	if got[0] != types.MustNewMoney("-1.08").String() {
		t.Errorf("seeded interest = %s, want -1.08", got[0])
	}
	if got[1] != types.MustNewMoney("-200").String() {
		t.Errorf("seeded principal = %s, want -200 (clamped)", got[1])
	}

	model, cmd := env.app.submitSchedulePreviewDialog()
	env.app = model.(*App)
	if env.app.schedPreviewDialog != nil {
		t.Error("preview should close on submit")
	}
	if cmd == nil {
		t.Fatal("submit must return a cmd")
	}
	msg := cmd()
	if e, ok := msg.(errMsg); ok {
		t.Fatalf("submit errored: %v", e.err)
	}
	posted, ok := msg.(scheduledPostedMsg)
	if !ok {
		t.Fatalf("submit msg = %T, want scheduledPostedMsg", msg)
	}
	if !posted.loanPaidOff {
		t.Error("posting the final payment should report loanPaidOff")
	}

	// Posted parent is the clamped total draft (interest 1.08 + principal 200).
	txns, err := env.txnRepo.ListByAccount(env.funding.ID)
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("expected 1 posted funding txn, got %d", len(txns))
	}
	if want := types.MustNewMoney("-201.08"); !txns[0].Amount.Equal(want) {
		t.Errorf("posted parent = %s, want %s (clamped draft, not the template -1580.17)", txns[0].Amount, want)
	}

	// The schedule is completed after payoff (the loan balance reached 0).
	st, err := env.schedRepo.GetByID(env.dueTxn.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !st.IsCompleted() {
		t.Error("schedule should be completed after payoff")
	}

	// Feeding the msg through Update surfaces the payoff toast.
	model2, _ := env.app.Update(posted)
	env.app = model2.(*App)
	toast := env.app.statusbar.Toast()
	if toast == nil {
		t.Fatal("payoff should set a status-bar toast")
	}
	if toast.Text == "" || toast.Level != widget.NotificationInfo {
		t.Errorf("payoff toast = %+v, want non-empty Info toast", toast)
	}
}

// TestSchedulePreview_LoanPaidOffAtOpen verifies that opening the preview on
// an already-paid-off loan refuses (does not open) and completes the schedule
// — the TUI realization of "manual post of a paid-off loan".
func TestSchedulePreview_LoanPaidOffAtOpen(t *testing.T) {
	nextDate := types.NewDate(2024, time.July, 1)
	env := newLoanPreviewEnv(t, "250000.00", "6.5", "1580.17", nextDate)
	// Pay the whole thing off before opening the preview.
	env.addLoanTxn(t, types.NewDate(2024, time.June, 15), "250000.00")

	msg := env.openPreview(t)
	blocked, ok := msg.(schedulePreviewLoanBlockedMsg)
	if !ok {
		t.Fatalf("loader msg = %T, want schedulePreviewLoanBlockedMsg", msg)
	}
	if !blocked.paidOff {
		t.Errorf("blocked msg paidOff = false, want true (loan is paid off)")
	}

	// The preview must not have opened.
	if env.app.schedPreviewDialog != nil {
		t.Error("preview should not open for a paid-off loan")
	}
	// The schedule is completed so it stops recurring as a zombie.
	st, err := env.schedRepo.GetByID(env.dueTxn.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !st.IsCompleted() {
		t.Error("a paid-off loan schedule should be marked completed on refusal")
	}
	// The toast is surfaced.
	if env.app.statusbar.Toast() == nil {
		t.Error("paid-off open should set a status-bar toast")
	}
}

// TestSchedulePreview_LoanSubmitRefusesPaidOffDate covers the reseed-refusal
// desync: editing the Date to a value where the loan is already paid off (a
// future-dated payoff exists) must refuse the post with an error rather than
// silently posting the stale (pre-payoff) split at the new date.
func TestSchedulePreview_LoanSubmitRefusesPaidOffDate(t *testing.T) {
	nextDate := types.NewDate(2024, time.July, 1)
	env := newLoanPreviewEnv(t, "250000.00", "6.5", "1580.17", nextDate)
	// A payoff dated after the next payment date: owed>0 at 07/01 (preview
	// opens), owed==0 as of 08/01.
	env.addLoanTxn(t, types.NewDate(2024, time.July, 15), "250000.00")

	env.openPreview(t)
	p := env.app.schedPreviewDialog
	if p == nil || !p.IsLoanShaped() {
		t.Fatal("preview should be loan-shaped")
	}

	// Advance the Date to the paid-off month. The live reseed is refused
	// (ErrLoanPaidOff) and silently leaves the stale seed on display.
	p.HeaderDialog().Fields()[previewFieldDate].Value = "08/01/2024"
	env.app.maybeReseedLoanPreview()

	model, cmd := env.app.submitSchedulePreviewDialog()
	env.app = model.(*App)
	if cmd != nil {
		t.Error("submit should refuse (nil cmd) when the loan is paid off at the posting date")
	}
	if env.app.schedPreviewDialog == nil {
		t.Fatal("dialog should stay open on a refused submit")
	}
	if env.app.schedPreviewDialog.HeaderDialog().ErrorMsg() == "" {
		t.Error("a refused paid-off submit should surface a header error")
	}

	// Nothing may be posted, and the schedule must remain due (it is NOT paid
	// off as of its real next date — only at the user's chosen future date).
	txns, err := env.txnRepo.ListByAccount(env.funding.ID)
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(txns) != 0 {
		t.Errorf("no funding transaction should be posted, got %d", len(txns))
	}
	st, err := env.schedRepo.GetByID(env.dueTxn.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if st.IsCompleted() {
		t.Error("schedule must not be completed — the loan is not paid off at its next date")
	}
}

// TestSchedulePreview_LoanSubmitRecomputesAtPostingDate covers submit's
// authority: even if the on-screen seed is stale (a reseed did not run for the
// current Date), submit recomputes the split at the posting date so the posted
// interest/principal always matches the balance as of that date.
func TestSchedulePreview_LoanSubmitRecomputesAtPostingDate(t *testing.T) {
	nextDate := types.NewDate(2024, time.July, 1)
	env := newLoanPreviewEnv(t, "250000.00", "6.5", "1580.17", nextDate)
	// Extra principal after the next date: owed=250000 at 07/01, 200000 at 08/01.
	env.addLoanTxn(t, types.NewDate(2024, time.July, 15), "50000.00")

	env.openPreview(t)
	// Seeded for 07/01 → interest 1354.17. Now advance the Date WITHOUT a
	// reseed, leaving the display stale on purpose.
	if got := env.rowAmounts(t)[0]; got != "-1354.17" {
		t.Fatalf("initial interest = %s, want -1354.17", got)
	}
	laterDate := types.NewDate(2024, time.August, 1)
	env.app.schedPreviewDialog.HeaderDialog().Fields()[previewFieldDate].Value = "08/01/2024"

	model, cmd := env.app.submitSchedulePreviewDialog()
	env.app = model.(*App)
	if cmd == nil {
		t.Fatal("submit should proceed for a valid future date")
	}
	if msg := cmd(); msg != nil {
		if e, ok := msg.(errMsg); ok {
			t.Fatalf("submit errored: %v", e.err)
		}
		if _, ok := msg.(scheduledPostedMsg); !ok {
			t.Fatalf("submit msg = %T, want scheduledPostedMsg", msg)
		}
	}

	txns, err := env.txnRepo.ListByAccount(env.funding.ID)
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("expected 1 posted funding txn, got %d", len(txns))
	}
	if !txns[0].Date.Equal(laterDate) {
		t.Errorf("posted date = %s, want %s", txns[0].Date, laterDate)
	}
	splits, err := env.splitRepo.ListByTransaction(txns[0].ID)
	if err != nil {
		t.Fatalf("ListByTransaction: %v", err)
	}
	// The interest split must be the recompute at 08/01 (owed 200000 → 1083.33),
	// NOT the stale on-screen 07/01 value (1354.17).
	wantInterest := types.MustNewMoney("-1083.33")
	staleInterest := types.MustNewMoney("-1354.17")
	var sawRecomputed bool
	for _, sp := range splits {
		if sp.Amount.Equal(staleInterest) {
			t.Errorf("posted the stale on-screen interest %s; submit did not recompute", staleInterest)
		}
		if sp.Amount.Equal(wantInterest) {
			sawRecomputed = true
		}
	}
	if !sawRecomputed {
		t.Errorf("expected a recomputed interest split of %s among posted splits", wantInterest)
	}
}

// TestSchedulePreview_LoanMemoEditFreezesReseed covers that a non-amount line
// edit (a memo, here) also freezes reseeding, so a later Date edit does not
// rebuild the editor and silently discard the user's edit.
func TestSchedulePreview_LoanMemoEditFreezesReseed(t *testing.T) {
	nextDate := types.NewDate(2024, time.July, 1)
	env := newLoanPreviewEnv(t, "250000.00", "6.5", "1580.17", nextDate)
	// A reseed at 08/01 WOULD change the amounts (extra principal on 07/15),
	// so if freezing failed the memo edit would be lost by the rebuild.
	env.addLoanTxn(t, types.NewDate(2024, time.July, 15), "50000.00")

	env.openPreview(t)
	p := env.app.schedPreviewDialog

	// Edit only a memo — no amount change.
	sd := p.SplitDialog()
	sd.rows[0].memoField.Value = "escrow true-up"
	env.app.freezeLoanSeedIfEdited()
	if !p.loanSeedFrozen {
		t.Fatal("a memo edit should freeze loan reseeding")
	}

	// A later Date edit must not reseed (which would discard the memo).
	p.HeaderDialog().Fields()[previewFieldDate].Value = "08/01/2024"
	env.app.maybeReseedLoanPreview()
	if !p.loanSeedDate.Equal(nextDate) {
		t.Errorf("frozen preview reseeded to %s; a line edit must freeze reseeding", p.loanSeedDate)
	}
	if got := env.app.schedPreviewDialog.SplitDialog().rows[0].memoField.Value; got != "escrow true-up" {
		t.Errorf("memo = %q, want it preserved (reseed must not rebuild the editor)", got)
	}
}

// TestSchedulePreview_GenericMultiLineNotLoanShaped guards that a generic
// (non-loan) multi-line schedule is untouched: the loader attaches no
// loanSplits and the preview is not marked loan-shaped, so it seeds from the
// template verbatim.
func TestSchedulePreview_GenericMultiLineNotLoanShaped(t *testing.T) {
	env := newSchedulePreviewMultiLineEnv(t)

	cmd := env.app.loadSchedulePreviewData()
	if cmd == nil {
		t.Fatal("loadSchedulePreviewData returned nil")
	}
	msg := cmd()
	dataMsg, ok := msg.(schedulePreviewDataMsg)
	if !ok {
		t.Fatalf("loader msg = %T, want schedulePreviewDataMsg", msg)
	}
	if dataMsg.loanSplits != nil {
		t.Error("generic multi-line schedule must not attach loanSplits")
	}
	model, _ := env.app.Update(msg)
	app := model.(*App)
	if app.schedPreviewDialog == nil || !app.schedPreviewDialog.IsMultiLine() {
		t.Fatal("generic multi-line preview should open")
	}
	if app.schedPreviewDialog.IsLoanShaped() {
		t.Error("generic multi-line preview must not be marked loan-shaped")
	}
}
