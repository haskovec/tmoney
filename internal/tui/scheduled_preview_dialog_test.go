package tui

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// TestSchedulePreviewDialog_OpensWithTemplateValues covers MS-018: the
// preview dialog scaffolding must open pre-filled with the template's
// values for both single-line and multi-line scheduled transactions.
// Edits to the dialog will eventually flow into the real transaction
// (MS-020); for now the scaffolding only needs to seed the fields
// correctly.
func TestSchedulePreviewDialog_OpensWithTemplateValues(t *testing.T) {
	t.Run("single-line preview seeds scalar fields from the template", func(t *testing.T) {
		accountID := types.NewID()
		payeeID := types.NewID()
		categoryID := types.NewID()
		nextDate := types.NewDate(2026, 4, 15)
		amount, _ := types.NewMoney("-1500.00")

		template := scheduled.NewTransaction(accountID, scheduled.FrequencyMonthly, nextDate)
		template.NextDate = nextDate
		template.Amount = types.NullableMoney{Money: amount, Valid: true}
		template.SetPayee(payeeID)
		template.SetCategory(categoryID)
		template.SetMemo("Monthly rent")

		payees := []*payee.Payee{
			{BaseModel: types.BaseModel{ID: payeeID}, Name: "Landlord"},
		}
		categoryOptions := []string{"(None)", "Other", "Rent"}
		categoryIDs := []types.ID{types.NilID, types.NewID(), categoryID}
		accounts := []*account.Account{
			{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: account.TypeChecking},
		}

		preview := NewSchedulePreviewDialog(template, accounts, payees, categoryOptions, categoryIDs)
		if preview == nil {
			t.Fatal("NewSchedulePreviewDialog returned nil for a valid template")
		}
		if preview.IsMultiLine() {
			t.Fatal("preview should be single-line when template has no splits")
		}
		if preview.SplitDialog() != nil {
			t.Fatal("single-line preview should not embed a split dialog")
		}
		if !preview.IsVisible() {
			t.Error("preview dialog should be visible after construction")
		}
		if preview.Template() != template {
			t.Error("Template accessor should return the source template")
		}

		hd := preview.HeaderDialog()
		if hd == nil {
			t.Fatal("HeaderDialog should be non-nil")
		}
		fields := hd.Fields()
		// Single-line preview should expose six fields:
		// Date, Payee, Category, Amount, Memo, Status.
		if len(fields) != 6 {
			t.Fatalf("expected 6 fields on single-line preview, got %d", len(fields))
		}

		// Date pre-fills with next_date in MM/DD/YYYY form.
		if got, want := fields[previewFieldDate].Value, "04/15/2026"; got != want {
			t.Errorf("date field = %q, want %q", got, want)
		}
		// Payee resolves from the payees slice.
		if got, want := fields[previewFieldPayee].Value, "Landlord"; got != want {
			t.Errorf("payee field = %q, want %q", got, want)
		}
		// Category combo points at the template's category.
		if got, want := fields[previewSingleFieldCat].SelectedIndex, 2; got != want {
			t.Errorf("category selectedIndex = %d, want %d", got, want)
		}
		// Amount renders the template amount.
		if got, want := fields[previewSingleFieldAmount].Value, amount.String(); got != want {
			t.Errorf("amount field = %q, want %q", got, want)
		}
		// Memo pre-fills from the template.
		if got, want := fields[previewSingleFieldMemo].Value, "Monthly rent"; got != want {
			t.Errorf("memo field = %q, want %q", got, want)
		}
		// Status defaults to Uncleared (a fresh post is always uncleared
		// until the user toggles it).
		if got, want := fields[previewSingleFieldStatus].SelectedIndex, previewStatusUnclearedIdx; got != want {
			t.Errorf("status selectedIndex = %d, want %d", got, want)
		}
	})

	t.Run("multi-line preview seeds header and split editor from the template", func(t *testing.T) {
		accountID := types.NewID()
		retirementAccountID := types.NewID()
		payeeID := types.NewID()
		incomeCatID := types.NewID()
		taxCatID := types.NewID()
		nextDate := types.NewDate(2026, 1, 23)
		netAmount, _ := types.NewMoney("4090.00")
		grossAmount, _ := types.NewMoney("5000.00")
		taxAmount, _ := types.NewMoney("-410.00")
		retireAmount, _ := types.NewMoney("-500.00")

		template := scheduled.NewTransaction(accountID, scheduled.FrequencyBiweekly, nextDate)
		template.NextDate = nextDate
		template.Amount = types.NullableMoney{Money: netAmount, Valid: true}
		template.SetPayee(payeeID)
		template.SetMemo("Paycheck")

		template.Splits = scheduled.SplitCollection{
			scheduled.NewCategorizedSplit(template.ID, incomeCatID, grossAmount),
			scheduled.NewCategorizedSplit(template.ID, taxCatID, taxAmount),
			scheduled.NewTransferSplit(template.ID, retirementAccountID, retireAmount),
		}

		payees := []*payee.Payee{
			{BaseModel: types.BaseModel{ID: payeeID}, Name: "Employer Inc"},
		}
		categoryOptions := []string{"(None)", "Salary", "Federal Tax"}
		categoryIDs := []types.ID{types.NilID, incomeCatID, taxCatID}
		accounts := []*account.Account{
			{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: account.TypeChecking},
			{BaseModel: types.BaseModel{ID: retirementAccountID}, Name: "401k", Active: true, Type: account.TypeInvestment},
		}

		preview := NewSchedulePreviewDialog(template, accounts, payees, categoryOptions, categoryIDs)
		if preview == nil {
			t.Fatal("NewSchedulePreviewDialog returned nil for a multi-line template")
		}
		if !preview.IsMultiLine() {
			t.Fatal("preview should report multi-line for a template with splits")
		}
		if !preview.IsVisible() {
			t.Error("preview dialog should be visible after construction")
		}

		hd := preview.HeaderDialog()
		if hd == nil {
			t.Fatal("HeaderDialog should be non-nil")
		}
		fields := hd.Fields()
		// Multi-line preview header carries date, payee, memo, status —
		// no scalar category or amount (those live in the split editor).
		if len(fields) != 4 {
			t.Fatalf("expected 4 header fields on multi-line preview, got %d", len(fields))
		}
		if got, want := fields[previewFieldDate].Value, "01/23/2026"; got != want {
			t.Errorf("date field = %q, want %q", got, want)
		}
		if got, want := fields[previewFieldPayee].Value, "Employer Inc"; got != want {
			t.Errorf("payee field = %q, want %q", got, want)
		}
		if got, want := fields[previewMultiFieldMemo].Value, "Paycheck"; got != want {
			t.Errorf("memo field = %q, want %q", got, want)
		}
		if got, want := fields[previewMultiFieldStatus].SelectedIndex, previewStatusUnclearedIdx; got != want {
			t.Errorf("status selectedIndex = %d, want %d", got, want)
		}

		sd := preview.SplitDialog()
		if sd == nil {
			t.Fatal("multi-line preview should embed a SplitDialog")
		}
		if !sd.totalAmount.Equal(netAmount) {
			t.Errorf("split totalAmount = %s, want %s",
				sd.totalAmount.String(), netAmount.String())
		}
		// Imbalance starts at zero because the template's lines net to
		// the parent's amount.
		if !sd.IsSaveEnabled() {
			t.Errorf("preview should open balanced (template guarantees signed sum); remaining=%s",
				sd.remaining().String())
		}

		rows := sd.Rows()
		if len(rows) != 3 {
			t.Fatalf("expected 3 split rows, got %d", len(rows))
		}

		// Row 0 — categorized income line.
		if got, want := rows[0].amountField.Value, grossAmount.String(); got != want {
			t.Errorf("row[0] amount = %q, want %q", got, want)
		}
		if rows[0].transferMode {
			t.Errorf("row[0] should be in category mode (income line)")
		}
		if got, want := rows[0].categoryIndex, 1; got != want {
			t.Errorf("row[0] categoryIndex = %d, want %d", got, want)
		}

		// Row 1 — categorized tax line (negative amount).
		if got, want := rows[1].amountField.Value, taxAmount.String(); got != want {
			t.Errorf("row[1] amount = %q, want %q", got, want)
		}
		if got, want := rows[1].categoryIndex, 2; got != want {
			t.Errorf("row[1] categoryIndex = %d, want %d", got, want)
		}

		// Row 2 — transfer line to the retirement account. The template
		// child carried TransferAccountID; the split dialog seeded the
		// row in category mode at index 0 (the (None) slot) because
		// NewSplitDialogFromExisting doesn't currently introspect
		// transfer-shape children — but it must at minimum preserve the
		// amount so the imbalance indicator works.
		if got, want := rows[2].amountField.Value, retireAmount.String(); got != want {
			t.Errorf("row[2] amount = %q, want %q", got, want)
		}

		// SetTransferTargets should have filtered out the parent
		// account (the schedule's own account) so users can't
		// self-transfer.
		for _, id := range sd.transferAccountIDs {
			if id == accountID {
				t.Errorf("transfer target picker still contains the schedule's own account")
			}
		}
		if len(sd.transferAccountIDs) == 0 {
			t.Error("transfer target picker should contain at least one account (the 401k)")
		}
	})

	t.Run("nil template yields nil preview", func(t *testing.T) {
		if got := NewSchedulePreviewDialog(nil, nil, nil, nil, nil); got != nil {
			t.Errorf("NewSchedulePreviewDialog(nil, …) = %v, want nil", got)
		}
	})
}

// schedulePreviewTestEnv bundles the DB-backed services and a due
// single-line scheduled transaction used by MS-020's save/cancel tests.
type schedulePreviewTestEnv struct {
	app          *App
	database     *db.DB
	txnRepo      *transaction.Repository
	schedRepo    *scheduled.Repository
	schedSvc     *scheduled.Service
	acct         *account.Account
	dueTxn       *scheduled.Transaction
	rentCat      *category.Category
	landlord     *payee.Payee
	originalNext types.Date
}

// newSchedulePreviewTestEnv builds a real DB + services + App with one
// due monthly single-line scheduled transaction, ready for the preview
// dialog to be opened against it.
func newSchedulePreviewTestEnv(t *testing.T) *schedulePreviewTestEnv {
	t.Helper()

	tempDir := t.TempDir()
	database, err := db.Create(filepath.Join(tempDir, "test.tdb"))
	if err != nil {
		t.Fatalf("db.Create: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	accountRepo := account.NewRepository(database)
	payeeRepo := payee.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	schedRepo := scheduled.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitTxnRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)

	txnSvc := transaction.NewService(txnRepo, splitTxnRepo, transferRepo, payeeRepo, database)
	schedSvc := scheduled.NewService(schedRepo, txnRepo, txnSvc, database)
	accountSvc := account.NewService(accountRepo, database)
	payeeSvc := payee.NewService(payeeRepo, database)
	categorySvc := category.NewService(categoryRepo, database)

	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(acct); err != nil {
		t.Fatalf("Create account: %v", err)
	}

	landlord := payee.NewPayee("Landlord")
	if err := payeeRepo.Create(landlord); err != nil {
		t.Fatalf("Create payee: %v", err)
	}

	rentCat := category.NewCategory("Rent", category.TypeExpense)
	if err := categoryRepo.Create(rentCat); err != nil {
		t.Fatalf("Create category: %v", err)
	}

	nextDate := types.Today()
	dueTxn := &scheduled.Transaction{
		BaseModel:  types.BaseModel{ID: types.NewID()},
		AccountID:  acct.ID,
		Frequency:  scheduled.FrequencyMonthly,
		Interval:   1,
		StartDate:  nextDate,
		NextDate:   nextDate,
		PayeeID:    types.NullableID{ID: landlord.ID, Valid: true},
		CategoryID: types.NullableID{ID: rentCat.ID, Valid: true},
		Amount:     types.NullableMoney{Money: types.MustNewMoney("-1500.00"), Valid: true},
	}
	dueTxn.SetMemo("Monthly rent")
	if err := schedSvc.Create(dueTxn); err != nil {
		t.Fatalf("Create scheduled txn: %v", err)
	}

	app := &App{
		currentView:     ViewScheduled,
		width:           120,
		height:          30,
		keys:            defaultKeyMap(),
		menubar:         NewMenuBar(),
		statusbar:       NewStatusBar(),
		sidebar:         NewSidebar(),
		styles:          NewStyles(),
		accountSvc:      accountSvc,
		payeeSvc:        payeeSvc,
		categorySvc:     categorySvc,
		scheduledTxnSvc: schedSvc,
		transactionSvc:  txnSvc,
		undoManager:     undo.NewManager(),
		scheduled: &scheduledViewData{
			allTxns:       []*scheduled.Transaction{dueTxn},
			dueTxns:       []*scheduled.Transaction{dueTxn},
			dueCount:      1,
			payeeNames:    map[types.ID]string{landlord.ID: landlord.Name},
			accountNames:  map[types.ID]string{acct.ID: acct.Name},
			categoryNames: map[types.ID]string{rentCat.ID: rentCat.Name},
		},
	}
	app.buildScheduledTable()
	app.sidebar.SetFocused(false)
	app.scheduledTable.SetFocused(true)

	// Open the preview dialog directly so tests can manipulate field
	// values without going through the async data load.
	categoryOptions, categoryIDs := buildCategoryOptions([]*category.Category{rentCat})
	app.schedPreviewDialog = NewSchedulePreviewDialog(
		dueTxn,
		[]*account.Account{acct},
		[]*payee.Payee{landlord},
		categoryOptions,
		categoryIDs,
	)
	if app.schedPreviewDialog == nil {
		t.Fatal("NewSchedulePreviewDialog returned nil")
	}

	return &schedulePreviewTestEnv{
		app:          app,
		database:     database,
		txnRepo:      txnRepo,
		schedRepo:    schedRepo,
		schedSvc:     schedSvc,
		acct:         acct,
		dueTxn:       dueTxn,
		rentCat:      rentCat,
		landlord:     landlord,
		originalNext: nextDate,
	}
}

// TestSchedulePreview_SaveCreatesTransactionAndAdvances covers MS-020:
// pressing Save on the preview dialog must (1) create a real transaction
// carrying any edits the user made to the header fields, (2) advance the
// schedule's next_date by one cadence using the *template's* original
// next_date (not the edited posting date), and (3) close the dialog.
//
// The user's per-instance edits flow into the real transaction but never
// flow back to the template — re-opening the preview later would show
// the original template values.
func TestSchedulePreview_SaveCreatesTransactionAndAdvances(t *testing.T) {
	env := newSchedulePreviewTestEnv(t)

	// User edits the memo so we can assert per-instance overrides reach
	// the persisted transaction. Date and amount stay at template values
	// so the schedule advancement basis is unambiguous.
	headerFields := env.app.schedPreviewDialog.HeaderDialog().Fields()
	headerFields[previewSingleFieldMemo].Value = "May rent (edited)"

	// Track what the schedule's next_date *should* become after a single
	// advance from the original next_date. Computed from the template's
	// cadence, not from any edited date.
	expectedNext := env.dueTxn.CalculateNextDate()

	model, cmd := env.app.submitSchedulePreviewDialog()
	updated, ok := model.(*App)
	if !ok {
		t.Fatalf("submitSchedulePreviewDialog returned %T, want *App", model)
	}

	// Dialog must close synchronously so the UI returns to the
	// scheduled view immediately; the actual save happens in the tea.Cmd.
	if updated.schedPreviewDialog != nil {
		t.Error("preview dialog should be cleared on Save")
	}
	if cmd == nil {
		t.Fatal("Save must return a tea.Cmd that performs the create+advance")
	}

	msg := cmd()
	if errM, ok := msg.(errMsg); ok {
		t.Fatalf("Save command produced an error: %v", errM.err)
	}
	if _, ok := msg.(scheduledPostedMsg); !ok {
		t.Fatalf("Save should produce scheduledPostedMsg, got %T", msg)
	}

	// Verify the real transaction was created with the edited memo, in
	// the schedule's account, on the template's next_date, and with the
	// template's payee/category/amount.
	posted, err := env.txnRepo.ListByAccount(env.acct.ID)
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(posted) != 1 {
		t.Fatalf("expected 1 posted transaction, got %d", len(posted))
	}
	txn := posted[0]
	if !txn.Date.Equal(env.originalNext) {
		t.Errorf("posted transaction date = %s, want %s",
			txn.Date.Time().Format("2006-01-02"),
			env.originalNext.Time().Format("2006-01-02"))
	}
	if !txn.Amount.Equal(env.dueTxn.Amount.Money) {
		t.Errorf("posted transaction amount = %s, want %s",
			txn.Amount.String(), env.dueTxn.Amount.Money.String())
	}
	if !txn.HasPayee() || txn.PayeeID.ID != env.landlord.ID {
		t.Errorf("posted transaction payee = %v, want %s", txn.PayeeID, env.landlord.ID)
	}
	if !txn.HasCategory() || txn.CategoryID.ID != env.rentCat.ID {
		t.Errorf("posted transaction category = %v, want %s", txn.CategoryID, env.rentCat.ID)
	}
	if !txn.Memo.Valid || txn.Memo.String != "May rent (edited)" {
		t.Errorf("posted transaction memo = %q, want %q", txn.Memo.String, "May rent (edited)")
	}

	// Verify the schedule advanced to the template's *next* occurrence.
	// The advancement is based on the template's original next_date so
	// any one-off date edit in the preview never shifts the cadence.
	reloaded, err := env.schedRepo.GetByID(env.dueTxn.ID)
	if err != nil {
		t.Fatalf("reload schedule: %v", err)
	}
	if !reloaded.NextDate.Equal(expectedNext) {
		t.Errorf("schedule next_date = %s, want %s (advanced from original %s)",
			reloaded.NextDate.Time().Format("2006-01-02"),
			expectedNext.Time().Format("2006-01-02"),
			env.originalNext.Time().Format("2006-01-02"))
	}

	// Template's other fields (payee/category/amount/memo) must be
	// unchanged so the next occurrence opens with the original values.
	if !reloaded.HasCategory() || reloaded.CategoryID.ID != env.rentCat.ID {
		t.Errorf("template category should be unchanged after save")
	}
	if !reloaded.Memo.Valid || reloaded.Memo.String != "Monthly rent" {
		t.Errorf("template memo = %q, want %q (per-instance memo edit leaked into template)",
			reloaded.Memo.String, "Monthly rent")
	}
}

// TestSchedulePreview_Cancel_NoChanges covers MS-020: pressing Esc on
// the preview dialog must leave both the transaction store and the
// schedule entirely untouched. The dialog itself closes.
func TestSchedulePreview_Cancel_NoChanges(t *testing.T) {
	env := newSchedulePreviewTestEnv(t)

	// Edit a field to make the test stronger: the dirty state should
	// be discarded, not silently saved.
	headerFields := env.app.schedPreviewDialog.HeaderDialog().Fields()
	headerFields[previewSingleFieldMemo].Value = "dirty edit that must NOT persist"

	// Cancel via the keypress handler so we cover the user-facing path.
	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, _ := env.app.handleSchedulePreviewDialogKey(escKey)
	updated, ok := model.(*App)
	if !ok {
		t.Fatalf("handleSchedulePreviewDialogKey returned %T, want *App", model)
	}

	if updated.schedPreviewDialog != nil {
		t.Error("preview dialog should be cleared on Cancel")
	}

	// No transactions in any account.
	posted, err := env.txnRepo.ListByAccount(env.acct.ID)
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(posted) != 0 {
		t.Errorf("Cancel must not create any transaction; got %d", len(posted))
	}

	// Schedule's next_date and other fields unchanged.
	reloaded, err := env.schedRepo.GetByID(env.dueTxn.ID)
	if err != nil {
		t.Fatalf("reload schedule: %v", err)
	}
	if !reloaded.NextDate.Equal(env.originalNext) {
		t.Errorf("schedule next_date = %s, want unchanged %s",
			reloaded.NextDate.Time().Format("2006-01-02"),
			env.originalNext.Time().Format("2006-01-02"))
	}
	if !reloaded.Memo.Valid || reloaded.Memo.String != "Monthly rent" {
		t.Errorf("template memo = %q, want unchanged %q", reloaded.Memo.String, "Monthly rent")
	}
}

// TestSchedulePreview_EditDate_OneOff covers MS-021: editing the date
// in the preview must apply to the posted transaction but never shift
// the schedule's cadence — advancement is always computed from the
// template's original next_date.
func TestSchedulePreview_EditDate_OneOff(t *testing.T) {
	env := newSchedulePreviewTestEnv(t)

	// Pick a date one week later than the template's next_date so the
	// edit is unambiguous. The advancement basis (originalNext) is
	// captured by the test env before any edit.
	editedDate := env.originalNext.AddDays(7)
	headerFields := env.app.schedPreviewDialog.HeaderDialog().Fields()
	headerFields[previewFieldDate].Value = editedDate.Time().Format("01/02/2006")

	// Cadence advancement is computed from the template's *original*
	// next_date, not the edited posting date.
	expectedNext := env.dueTxn.CalculateNextDate()

	model, cmd := env.app.submitSchedulePreviewDialog()
	updated, ok := model.(*App)
	if !ok {
		t.Fatalf("submitSchedulePreviewDialog returned %T, want *App", model)
	}
	if updated.schedPreviewDialog != nil {
		t.Error("preview dialog should be cleared on Save")
	}
	if cmd == nil {
		t.Fatal("Save must return a tea.Cmd")
	}
	if msg := cmd(); msg != nil {
		if e, ok := msg.(errMsg); ok {
			t.Fatalf("Save command produced an error: %v", e.err)
		}
	}

	// The posted transaction's date is the *edited* date the user typed.
	posted, err := env.txnRepo.ListByAccount(env.acct.ID)
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(posted) != 1 {
		t.Fatalf("expected 1 posted transaction, got %d", len(posted))
	}
	if !posted[0].Date.Equal(editedDate) {
		t.Errorf("posted transaction date = %s, want edited date %s",
			posted[0].Date.Time().Format("2006-01-02"),
			editedDate.Time().Format("2006-01-02"))
	}

	// The schedule's next_date advances by one cadence from the
	// template's original next_date — NOT from the edited date. If the
	// advancement leaked off the edited date, this assertion fails.
	reloaded, err := env.schedRepo.GetByID(env.dueTxn.ID)
	if err != nil {
		t.Fatalf("reload schedule: %v", err)
	}
	if !reloaded.NextDate.Equal(expectedNext) {
		t.Errorf("schedule next_date = %s, want %s (one cadence past the *original* %s, not the edited %s)",
			reloaded.NextDate.Time().Format("2006-01-02"),
			expectedNext.Time().Format("2006-01-02"),
			env.originalNext.Time().Format("2006-01-02"),
			editedDate.Time().Format("2006-01-02"))
	}
}

// schedulePreviewMultiLineEnv bundles a real DB + services with a due
// multi-line scheduled transaction (paycheck-shaped: gross income line
// + tax line netting to a fixed parent amount). Used by the multi-line
// preview tests in MS-021.
type schedulePreviewMultiLineEnv struct {
	app          *App
	database     *db.DB
	txnRepo      *transaction.Repository
	splitRepo    *transaction.SplitRepository
	schedRepo    *scheduled.Repository
	schedSvc     *scheduled.Service
	acct         *account.Account
	dueTxn       *scheduled.Transaction
	incomeCat    *category.Category
	taxCat       *category.Category
	employer     *payee.Payee
	originalNext types.Date
	netAmount    types.Money
	grossAmount  types.Money
	taxAmount    types.Money
}

// newSchedulePreviewMultiLineEnv builds a DB-backed multi-line paycheck
// schedule and opens the preview dialog against it. The schedule's
// children are intentionally categorized-only so the tests can drive
// the lines without depending on transfer-line seeding (which the
// existing scheduled.NewSplitDialogFromExisting doesn't yet introspect
// — that's tracked separately and out of scope for MS-021).
func newSchedulePreviewMultiLineEnv(t *testing.T) *schedulePreviewMultiLineEnv {
	t.Helper()

	tempDir := t.TempDir()
	database, err := db.Create(filepath.Join(tempDir, "test.tdb"))
	if err != nil {
		t.Fatalf("db.Create: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	accountRepo := account.NewRepository(database)
	payeeRepo := payee.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	schedRepo := scheduled.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitTxnRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)

	txnSvc := transaction.NewService(txnRepo, splitTxnRepo, transferRepo, payeeRepo, database)
	schedSvc := scheduled.NewService(schedRepo, txnRepo, txnSvc, database)
	accountSvc := account.NewService(accountRepo, database)
	payeeSvc := payee.NewService(payeeRepo, database)
	categorySvc := category.NewService(categoryRepo, database)

	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(acct); err != nil {
		t.Fatalf("Create account: %v", err)
	}
	employer := payee.NewPayee("Employer Inc")
	if err := payeeRepo.Create(employer); err != nil {
		t.Fatalf("Create payee: %v", err)
	}
	incomeCat := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(incomeCat); err != nil {
		t.Fatalf("Create income category: %v", err)
	}
	taxCat := category.NewCategory("Federal Tax", category.TypeExpense)
	if err := categoryRepo.Create(taxCat); err != nil {
		t.Fatalf("Create tax category: %v", err)
	}

	netAmount, _ := types.NewMoney("900.00")
	grossAmount, _ := types.NewMoney("1000.00")
	taxAmount, _ := types.NewMoney("-100.00")

	nextDate := types.Today()
	dueTxn := scheduled.NewTransaction(acct.ID, scheduled.FrequencyBiweekly, nextDate)
	dueTxn.NextDate = nextDate
	dueTxn.Amount = types.NullableMoney{Money: netAmount, Valid: true}
	dueTxn.SetPayee(employer.ID)
	dueTxn.SetMemo("Paycheck")
	dueTxn.Splits = scheduled.SplitCollection{
		scheduled.NewCategorizedSplit(dueTxn.ID, incomeCat.ID, grossAmount),
		scheduled.NewCategorizedSplit(dueTxn.ID, taxCat.ID, taxAmount),
	}
	if err := schedSvc.Create(dueTxn); err != nil {
		t.Fatalf("Create scheduled txn: %v", err)
	}

	app := &App{
		currentView:     ViewScheduled,
		width:           120,
		height:          30,
		keys:            defaultKeyMap(),
		menubar:         NewMenuBar(),
		statusbar:       NewStatusBar(),
		sidebar:         NewSidebar(),
		styles:          NewStyles(),
		accountSvc:      accountSvc,
		payeeSvc:        payeeSvc,
		categorySvc:     categorySvc,
		scheduledTxnSvc: schedSvc,
		transactionSvc:  txnSvc,
		undoManager:     undo.NewManager(),
		scheduled: &scheduledViewData{
			allTxns:       []*scheduled.Transaction{dueTxn},
			dueTxns:       []*scheduled.Transaction{dueTxn},
			dueCount:      1,
			payeeNames:    map[types.ID]string{employer.ID: employer.Name},
			accountNames:  map[types.ID]string{acct.ID: acct.Name},
			categoryNames: map[types.ID]string{incomeCat.ID: incomeCat.Name, taxCat.ID: taxCat.Name},
		},
	}
	app.buildScheduledTable()
	app.sidebar.SetFocused(false)
	app.scheduledTable.SetFocused(true)

	categoryOptions, categoryIDs := buildCategoryOptions([]*category.Category{incomeCat, taxCat})
	app.schedPreviewDialog = NewSchedulePreviewDialog(
		dueTxn,
		[]*account.Account{acct},
		[]*payee.Payee{employer},
		categoryOptions,
		categoryIDs,
	)
	if app.schedPreviewDialog == nil {
		t.Fatal("NewSchedulePreviewDialog returned nil")
	}
	if !app.schedPreviewDialog.IsMultiLine() {
		t.Fatal("preview should be multi-line for a template with splits")
	}

	return &schedulePreviewMultiLineEnv{
		app:          app,
		database:     database,
		txnRepo:      txnRepo,
		splitRepo:    splitTxnRepo,
		schedRepo:    schedRepo,
		schedSvc:     schedSvc,
		acct:         acct,
		dueTxn:       dueTxn,
		incomeCat:    incomeCat,
		taxCat:       taxCat,
		employer:     employer,
		originalNext: nextDate,
		netAmount:    netAmount,
		grossAmount:  grossAmount,
		taxAmount:    taxAmount,
	}
}

// TestSchedulePreview_EditLineAmount_OneOff covers MS-021: editing a
// line amount in the multi-line preview must flow into the posted
// transaction's splits, but the template's stored children must NOT
// be mutated — opening the preview again on the next due occurrence
// would show the original template values.
func TestSchedulePreview_EditLineAmount_OneOff(t *testing.T) {
	env := newSchedulePreviewMultiLineEnv(t)

	// User edits the tax line by $1 (e.g., a FICA true-up). Rebalance
	// the income line so the lines still net to the parent amount.
	sd := env.app.schedPreviewDialog.SplitDialog()
	if sd == nil {
		t.Fatal("multi-line preview should embed a SplitDialog")
	}
	if len(sd.rows) != 2 {
		t.Fatalf("expected 2 seeded rows, got %d", len(sd.rows))
	}
	// Row 0 = income (gross). Row 1 = tax. Edit the tax by $1, then
	// reduce the income by $1 to keep the parent net at $900.
	sd.rows[0].amountField.Value = "999.00"
	sd.rows[1].amountField.Value = "-99.00"
	if !sd.IsSaveEnabled() {
		t.Fatalf("rebalanced lines should leave the dialog balanced; remaining=%s",
			sd.remaining().String())
	}

	model, cmd := env.app.submitSchedulePreviewDialog()
	updated, ok := model.(*App)
	if !ok {
		t.Fatalf("submitSchedulePreviewDialog returned %T, want *App", model)
	}
	if updated.schedPreviewDialog != nil {
		t.Error("preview dialog should be cleared on Save")
	}
	if cmd == nil {
		t.Fatal("Save must return a tea.Cmd")
	}
	if msg := cmd(); msg != nil {
		if e, ok := msg.(errMsg); ok {
			t.Fatalf("Save command produced an error: %v", e.err)
		}
		if _, ok := msg.(scheduledPostedMsg); !ok {
			t.Fatalf("expected scheduledPostedMsg, got %T", msg)
		}
	}

	// Posted transaction must carry the EDITED line amounts.
	posted, err := env.txnRepo.ListByAccount(env.acct.ID)
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(posted) != 1 {
		t.Fatalf("expected 1 posted transaction, got %d", len(posted))
	}
	if !posted[0].Amount.Equal(env.netAmount) {
		t.Errorf("posted parent amount = %s, want %s",
			posted[0].Amount.String(), env.netAmount.String())
	}
	postedSplits, err := env.splitRepo.ListByTransaction(posted[0].ID)
	if err != nil {
		t.Fatalf("ListByTransaction: %v", err)
	}
	if len(postedSplits) != 2 {
		t.Fatalf("expected 2 posted splits, got %d", len(postedSplits))
	}
	wantGross, _ := types.NewMoney("999.00")
	wantTax, _ := types.NewMoney("-99.00")
	var sawGross, sawTax bool
	for _, sp := range postedSplits {
		switch {
		case sp.Amount.Equal(wantGross):
			sawGross = true
		case sp.Amount.Equal(wantTax):
			sawTax = true
		}
	}
	if !sawGross || !sawTax {
		t.Errorf("posted splits = %+v, want edited amounts %s and %s",
			postedSplits, wantGross.String(), wantTax.String())
	}

	// Template's stored children must be UNCHANGED — per-instance line
	// edits never flow back to the template. Reload from the DB to be
	// sure we're not just reading the in-memory pointer.
	reloaded, err := env.schedRepo.GetByID(env.dueTxn.ID)
	if err != nil {
		t.Fatalf("reload schedule: %v", err)
	}
	if len(reloaded.Splits) != 2 {
		t.Fatalf("template should still have 2 children, got %d", len(reloaded.Splits))
	}
	var tmplGross, tmplTax bool
	for _, sp := range reloaded.Splits {
		switch {
		case sp.Amount.Equal(env.grossAmount):
			tmplGross = true
		case sp.Amount.Equal(env.taxAmount):
			tmplTax = true
		}
	}
	if !tmplGross || !tmplTax {
		t.Errorf("template splits leaked the preview's edits; got %+v, want originals %s and %s",
			reloaded.Splits, env.grossAmount.String(), env.taxAmount.String())
	}
}

// TestSchedulePreview_ImbalancedSaveDisabled covers MS-021: when the
// multi-line preview's signed line sum does not equal the parent
// amount, Save must be rejected — no transaction is created and the
// schedule's next_date does not advance.
func TestSchedulePreview_ImbalancedSaveDisabled(t *testing.T) {
	env := newSchedulePreviewMultiLineEnv(t)

	sd := env.app.schedPreviewDialog.SplitDialog()
	if sd == nil {
		t.Fatal("multi-line preview should embed a SplitDialog")
	}
	// Edit ONE line without rebalancing — lines no longer sum to the
	// parent amount. IsSaveEnabled must report false (the visible Save
	// button is rendered muted per MS-013) and a Save attempt must be
	// rejected.
	sd.rows[1].amountField.Value = "-50.00"
	if sd.IsSaveEnabled() {
		t.Fatalf("dialog should be imbalanced after a one-sided edit; remaining=%s",
			sd.remaining().String())
	}

	model, cmd := env.app.submitSchedulePreviewDialog()
	updated, ok := model.(*App)
	if !ok {
		t.Fatalf("submitSchedulePreviewDialog returned %T, want *App", model)
	}
	// Dialog must stay open so the user can fix the imbalance.
	if updated.schedPreviewDialog == nil {
		t.Error("preview dialog should remain open while imbalanced")
	}
	// No async save command — the submit was rejected synchronously.
	if cmd != nil {
		t.Errorf("imbalanced submit should not return a tea.Cmd, got %T", cmd)
	}

	// No transaction was created.
	posted, err := env.txnRepo.ListByAccount(env.acct.ID)
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(posted) != 0 {
		t.Errorf("imbalanced save must not create any transaction; got %d", len(posted))
	}

	// Schedule's next_date is unchanged.
	reloaded, err := env.schedRepo.GetByID(env.dueTxn.ID)
	if err != nil {
		t.Fatalf("reload schedule: %v", err)
	}
	if !reloaded.NextDate.Equal(env.originalNext) {
		t.Errorf("schedule next_date = %s, want unchanged %s",
			reloaded.NextDate.Time().Format("2006-01-02"),
			env.originalNext.Time().Format("2006-01-02"))
	}
}
