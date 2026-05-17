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
