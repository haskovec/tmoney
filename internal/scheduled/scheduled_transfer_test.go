package scheduled

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/types"
)

// transferTestEnv wires a scheduled service plus the transaction repo so tests
// can inspect both legs of a posted transfer pair.
type transferTestEnv struct {
	svc          *Service
	txnRepo      *transaction.Repository
	splitRepo    *transaction.SplitRepository
	accountRepo  *account.Repository
	categoryRepo *category.Repository
}

func newTransferTestEnv(t *testing.T) *transferTestEnv {
	t.Helper()
	database := createTestDB(t)
	stRepo := NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)
	accountRepo := account.NewRepository(database)
	payeeRepo := payee.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	txnSvc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)
	svc := NewService(stRepo, txnRepo, txnSvc, database, accountRepo)
	// Transfer occurrences post through the transfer owner; production wires this
	// in app.NewServices for the import-cycle reason in transfer_port.go.
	svc.SetTransferPort(transfer.NewService(txnRepo, investment.NewRepository(database),
		splitRepo, accountRepo, categoryRepo, database))
	return &transferTestEnv{
		svc:          svc,
		txnRepo:      txnRepo,
		splitRepo:    splitRepo,
		accountRepo:  accountRepo,
		categoryRepo: categoryRepo,
	}
}

// category creates and persists an expense category, returning it so tests can
// label a transfer schedule and later assert the posted legs carry its ID.
func (e *transferTestEnv) category(t *testing.T, name string) *category.Category {
	t.Helper()
	cat := category.NewCategory(name, category.TypeExpense)
	if err := e.categoryRepo.Create(cat); err != nil {
		t.Fatalf("create category %q: %v", name, err)
	}
	return cat
}

func (e *transferTestEnv) account(t *testing.T, name string, typ account.Type) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, typ, "USD", types.ZeroMoney, types.Today())
	if err := e.accountRepo.Create(acct); err != nil {
		t.Fatalf("create account %q: %v", name, err)
	}
	return acct
}

// newTransferSchedule builds a single-line transfer schedule (From -> To) with
// the stored amount as the signed effect on the source (negative).
func newTransferSchedule(from, to types.ID, magnitude string) *Transaction {
	amt := types.MustNewMoney(magnitude).Neg()
	st := NewTransactionWithAmount(from, FrequencyMonthly, types.Today(), amt)
	st.SetTransfer(to)
	return st
}

func TestTransferSchedule_Validate(t *testing.T) {
	from := types.NewID()
	to := types.NewID()

	t.Run("valid transfer schedule passes", func(t *testing.T) {
		st := newTransferSchedule(from, to, "200.00")
		st.AccountID = from
		if errs := st.Validate(); errs.HasErrors() {
			t.Errorf("expected valid, got %v", errs)
		}
	})

	t.Run("self-transfer rejected", func(t *testing.T) {
		st := newTransferSchedule(from, from, "200.00")
		st.AccountID = from
		errs := st.Validate()
		if !errs.HasErrors() {
			t.Fatal("expected error for self-transfer")
		}
	})

	t.Run("transfer with category is accepted (categorized transfer)", func(t *testing.T) {
		st := newTransferSchedule(from, to, "200.00")
		st.AccountID = from
		st.CategoryID = types.NullableID{ID: types.NewID(), Valid: true}
		if errs := st.Validate(); errs.HasErrors() {
			t.Fatalf("a categorized transfer schedule should validate, got %v", errs)
		}
	})

	t.Run("transfer with splits rejected", func(t *testing.T) {
		st := newTransferSchedule(from, to, "200.00")
		st.AccountID = from
		st.Splits = SplitCollection{NewTransferSplit(st.ID, types.NewID(), types.MustNewMoney("-200.00"))}
		if errs := st.Validate(); !errs.HasErrors() {
			t.Fatal("expected error when both transfer and splits set")
		}
	})

	t.Run("transfer without amount rejected", func(t *testing.T) {
		st := NewTransaction(from, FrequencyMonthly, types.Today())
		st.AccountID = from
		st.SetTransfer(to)
		if errs := st.Validate(); !errs.HasErrors() {
			t.Fatal("expected error for transfer with no amount")
		}
	})
}

func TestTransferSchedule_PredicatesAndExclusivity(t *testing.T) {
	st := NewTransactionWithAmount(types.NewID(), FrequencyMonthly, types.Today(), types.MustNewMoney("-200.00"))
	st.SetCategory(types.NewID())
	if st.IsTransfer() {
		t.Fatal("category schedule should not report IsTransfer")
	}

	to := types.NewID()
	st.SetTransfer(to)
	if !st.IsTransfer() {
		t.Fatal("expected IsTransfer after SetTransfer")
	}
	// SetTransfer no longer clears an existing category: a transfer may carry
	// an optional category as a label — a categorized transfer (migration 029).
	if !st.HasCategory() {
		t.Error("SetTransfer should preserve an existing category (categorized transfer)")
	}

	st.ClearTransfer()
	if st.IsTransfer() {
		t.Error("ClearTransfer should clear the transfer destination")
	}
}

func TestService_PostSingleLineTransfer(t *testing.T) {
	t.Run("creates a clean linked transfer pair and advances the schedule", func(t *testing.T) {
		env := newTransferTestEnv(t)
		checking := env.account(t, "Checking", account.TypeChecking)
		visa := env.account(t, "Visa", account.TypeCreditCard)

		st := newTransferSchedule(checking.ID, visa.ID, "200.00")
		if err := env.svc.Create(st); err != nil {
			t.Fatalf("Create: %v", err)
		}
		originalNext := st.NextDate

		from, err := env.svc.Post(st.ID, nil)
		if err != nil {
			t.Fatalf("Post: %v", err)
		}

		// From side: Checking -200, transfer-linked.
		if from.AccountID != checking.ID {
			t.Errorf("from.AccountID = %v, want Checking", from.AccountID)
		}
		if !from.Amount.Equal(types.MustNewMoney("-200.00")) {
			t.Errorf("from.Amount = %s, want -200.00", from.Amount.String())
		}
		if !from.IsTransfer() {
			t.Fatal("from side should be a transfer")
		}

		// To side: Visa +200.
		to, err := env.txnRepo.ListByAccount(visa.ID)
		if err != nil {
			t.Fatalf("ListByAccount(Visa): %v", err)
		}
		if len(to) != 1 {
			t.Fatalf("expected 1 row in Visa, got %d", len(to))
		}
		if !to[0].Amount.Equal(types.MustNewMoney("200.00")) {
			t.Errorf("to.Amount = %s, want 200.00", to[0].Amount.String())
		}
		if !to[0].TransferID.Valid || to[0].TransferID.ID != from.TransferID.ID {
			t.Error("legs should share a transfer_id")
		}

		// Schedule advanced.
		updated, _ := env.svc.GetByID(st.ID)
		if updated.NextDate.Equal(originalNext) {
			t.Error("schedule should have advanced")
		}
		if !updated.IsTransfer() {
			t.Error("template should still be a transfer after posting")
		}
	})

	t.Run("post-time amount override applies to this occurrence only", func(t *testing.T) {
		env := newTransferTestEnv(t)
		checking := env.account(t, "Checking", account.TypeChecking)
		visa := env.account(t, "Visa", account.TypeCreditCard)

		st := newTransferSchedule(checking.ID, visa.ID, "200.00")
		if err := env.svc.Create(st); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// User edits the estimate to the real statement balance. The preview
		// passes the magnitude; sign is normalized via Abs.
		override := types.MustNewMoney("213.47")
		from, err := env.svc.Post(st.ID, &override)
		if err != nil {
			t.Fatalf("Post: %v", err)
		}
		if !from.Amount.Equal(types.MustNewMoney("-213.47")) {
			t.Errorf("from.Amount = %s, want -213.47", from.Amount.String())
		}

		// Template keeps the estimate.
		updated, _ := env.svc.GetByID(st.ID)
		if !updated.Amount.Money.Equal(types.MustNewMoney("-200.00")) {
			t.Errorf("template amount = %s, want -200.00 (unchanged)", updated.Amount.Money.String())
		}
	})

	t.Run("carries the memo onto both legs", func(t *testing.T) {
		env := newTransferTestEnv(t)
		checking := env.account(t, "Checking", account.TypeChecking)
		savings := env.account(t, "Savings", account.TypeSavings)

		st := newTransferSchedule(checking.ID, savings.ID, "500.00")
		st.SetMemo("monthly savings")
		if err := env.svc.Create(st); err != nil {
			t.Fatalf("Create: %v", err)
		}
		from, err := env.svc.Post(st.ID, nil)
		if err != nil {
			t.Fatalf("Post: %v", err)
		}
		if !from.Memo.Valid || from.Memo.String != "monthly savings" {
			t.Errorf("from memo = %v, want \"monthly savings\"", from.Memo)
		}
	})
}

func TestService_AutoPostTransfer(t *testing.T) {
	env := newTransferTestEnv(t)
	checking := env.account(t, "Checking", account.TypeChecking)
	savings := env.account(t, "Savings", account.TypeSavings)

	st := newTransferSchedule(checking.ID, savings.ID, "500.00")
	st.SetAutoPost(true)
	st.SetPostLeadDays(0)
	if err := env.svc.Create(st); err != nil {
		t.Fatalf("Create: %v", err)
	}

	summary, err := env.svc.AutoPost()
	if err != nil {
		t.Fatalf("AutoPost: %v", err)
	}
	if summary.PostedCount != 1 {
		t.Fatalf("expected 1 auto-posted transfer, got %d", summary.PostedCount)
	}

	rows, err := env.txnRepo.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("ListByAccount(Savings): %v", err)
	}
	if len(rows) != 1 || !rows[0].Amount.Equal(types.MustNewMoney("500.00")) {
		t.Fatalf("expected one +500.00 row in Savings, got %#v", rows)
	}
}
