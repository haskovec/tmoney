package transaction

import (
	"errors"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/types"
)

// transferCatEnv wires a transaction Service plus a category repository against
// a shared test DB, with two regular accounts ready for transfers.
type transferCatEnv struct {
	svc      *Service
	catRepo  *category.Repository
	checking *account.Account
	savings  *account.Account
}

func newTransferCatEnv(t *testing.T) *transferCatEnv {
	t.Helper()
	database := dbtest.New(t)
	txnRepo := NewRepository(database)
	splitRepo := NewSplitRepository(database)
	transferRepo := NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	catRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

	openDate := types.NewDate(2000, time.January, 1)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, openDate)
	if err := accountRepo.Create(checking); err != nil {
		t.Fatalf("create checking: %v", err)
	}
	savings := account.NewAccount("Savings", account.TypeSavings, "USD", types.ZeroMoney, openDate)
	if err := accountRepo.Create(savings); err != nil {
		t.Fatalf("create savings: %v", err)
	}
	return &transferCatEnv{svc: svc, catRepo: catRepo, checking: checking, savings: savings}
}

func (e *transferCatEnv) category(t *testing.T, name string) *category.Category {
	t.Helper()
	cat := category.NewCategory(name, category.TypeExpense)
	if err := e.catRepo.Create(cat); err != nil {
		t.Fatalf("create category %q: %v", name, err)
	}
	return cat
}

// reload re-reads both legs of a transfer from the DB so assertions run against
// persisted state, not the in-memory pair.
func (e *transferCatEnv) reload(t *testing.T, transferID types.ID) *TransferPair {
	t.Helper()
	pair, err := e.svc.GetTransferPair(transferID)
	if err != nil {
		t.Fatalf("reload pair: %v", err)
	}
	return pair
}

func assertLegCategory(t *testing.T, txn *Transaction, want types.NullableID, label string) {
	t.Helper()
	if want.Valid {
		if !txn.CategoryID.Valid || txn.CategoryID.ID != want.ID {
			t.Fatalf("%s: want category %s, got valid=%v id=%v", label, want.ID, txn.CategoryID.Valid, txn.CategoryID.ID)
		}
		return
	}
	if txn.CategoryID.Valid {
		t.Fatalf("%s: want no category, got %s", label, txn.CategoryID.ID)
	}
}

func TestCreateTransfer_MirrorsCategoryToBothLegs(t *testing.T) {
	env := newTransferCatEnv(t)
	bills := env.category(t, "Bills")

	pair, err := env.svc.CreateTransfer(env.checking.ID, env.savings.ID, types.Today(),
		types.MustNewMoney("100.00"), "rent", types.NullableID{ID: bills.ID, Valid: true})
	if err != nil {
		t.Fatalf("CreateTransfer: %v", err)
	}

	want := types.NullableID{ID: bills.ID, Valid: true}
	reloaded := env.reload(t, pair.FromTransaction.TransferID.ID)
	assertLegCategory(t, reloaded.FromTransaction, want, "from")
	assertLegCategory(t, reloaded.ToTransaction, want, "to")
	// Memo also stamped on both legs (workaround retired).
	if reloaded.FromTransaction.Memo.String != "rent" || reloaded.ToTransaction.Memo.String != "rent" {
		t.Fatalf("memo not mirrored: from=%q to=%q", reloaded.FromTransaction.Memo.String, reloaded.ToTransaction.Memo.String)
	}
}

func TestCreateTransfer_NoCategoryLeavesBothBare(t *testing.T) {
	env := newTransferCatEnv(t)
	pair, err := env.svc.CreateTransfer(env.checking.ID, env.savings.ID, types.Today(),
		types.MustNewMoney("100.00"), "", types.NullableID{})
	if err != nil {
		t.Fatalf("CreateTransfer: %v", err)
	}
	reloaded := env.reload(t, pair.FromTransaction.TransferID.ID)
	assertLegCategory(t, reloaded.FromTransaction, types.NullableID{}, "from")
	assertLegCategory(t, reloaded.ToTransaction, types.NullableID{}, "to")
}

func TestCreateTransfer_RejectsSystemCategory(t *testing.T) {
	env := newTransferCatEnv(t)
	sysCat := category.NewSystemCategory("Transfer", category.TypeExpense)
	if err := env.catRepo.Create(sysCat); err != nil {
		t.Fatalf("create system category: %v", err)
	}

	_, err := env.svc.CreateTransfer(env.checking.ID, env.savings.ID, types.Today(),
		types.MustNewMoney("100.00"), "", types.NullableID{ID: sysCat.ID, Valid: true})
	var sysErr *SystemCategoryTransferError
	if !errors.As(err, &sysErr) {
		t.Fatalf("want SystemCategoryTransferError, got %v", err)
	}
}

func TestCreateTransfer_RejectsNonexistentCategory(t *testing.T) {
	env := newTransferCatEnv(t)
	_, err := env.svc.CreateTransfer(env.checking.ID, env.savings.ID, types.Today(),
		types.MustNewMoney("100.00"), "", types.NullableID{ID: types.NewID(), Valid: true})
	var nf *dberrors.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError for missing category, got %v", err)
	}
}

func TestUpdateTransfer_SetsAndClearsCategory(t *testing.T) {
	env := newTransferCatEnv(t)
	bills := env.category(t, "Bills")

	pair, err := env.svc.CreateTransfer(env.checking.ID, env.savings.ID, types.Today(),
		types.MustNewMoney("100.00"), "", types.NullableID{})
	if err != nil {
		t.Fatalf("CreateTransfer: %v", err)
	}
	transferID := pair.FromTransaction.TransferID.ID

	// Set a category via update → mirrored to both legs.
	if err := env.svc.UpdateTransfer(transferID, types.Today(), types.MustNewMoney("100.00"), "",
		StatusUncleared, types.NullableID{ID: bills.ID, Valid: true}); err != nil {
		t.Fatalf("UpdateTransfer set: %v", err)
	}
	want := types.NullableID{ID: bills.ID, Valid: true}
	set := env.reload(t, transferID)
	assertLegCategory(t, set.FromTransaction, want, "from after set")
	assertLegCategory(t, set.ToTransaction, want, "to after set")

	// Clear it (invalid NullableID) → both legs bare.
	if err := env.svc.UpdateTransfer(transferID, types.Today(), types.MustNewMoney("100.00"), "",
		StatusUncleared, types.NullableID{}); err != nil {
		t.Fatalf("UpdateTransfer clear: %v", err)
	}
	cleared := env.reload(t, transferID)
	assertLegCategory(t, cleared.FromTransaction, types.NullableID{}, "from after clear")
	assertLegCategory(t, cleared.ToTransaction, types.NullableID{}, "to after clear")
}

func TestUpdateTransfer_HealsLegacyDivergentPair(t *testing.T) {
	// Simulate a pre-feature transfer-link result: one transfer_id, two legs
	// carrying DIFFERENT categories. The first UpdateTransfer rewrites both to a
	// single value.
	env := newTransferCatEnv(t)
	catA := env.category(t, "Bills")
	catB := env.category(t, "Groceries")

	txnRepo := NewRepository(env.svc.db)
	transferID := types.NewID()
	from := NewTransaction(env.checking.ID, types.Today(), types.MustNewMoney("-100.00"))
	from.SetTransfer(transferID, env.savings.ID)
	from.SetCategory(catA.ID)
	to := NewTransaction(env.savings.ID, types.Today(), types.MustNewMoney("100.00"))
	to.SetTransfer(transferID, env.checking.ID)
	to.SetCategory(catB.ID)
	if err := txnRepo.Create(from); err != nil {
		t.Fatalf("create from leg: %v", err)
	}
	if err := txnRepo.Create(to); err != nil {
		t.Fatalf("create to leg: %v", err)
	}

	// Sanity: the persisted pair is divergent before healing.
	pre := env.reload(t, transferID)
	if pre.FromTransaction.CategoryID.ID == pre.ToTransaction.CategoryID.ID {
		t.Fatalf("expected divergent categories before healing")
	}

	// Heal by editing to a single value.
	if err := env.svc.UpdateTransfer(transferID, types.Today(), types.MustNewMoney("100.00"), "",
		StatusUncleared, types.NullableID{ID: catA.ID, Valid: true}); err != nil {
		t.Fatalf("UpdateTransfer heal: %v", err)
	}
	want := types.NullableID{ID: catA.ID, Valid: true}
	healed := env.reload(t, transferID)
	assertLegCategory(t, healed.FromTransaction, want, "from after heal")
	assertLegCategory(t, healed.ToTransaction, want, "to after heal")
}
