package transferlink

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// linkTestEnv bundles the repositories a transferlink test needs.
type linkTestEnv struct {
	database *db.DB
	svc      *Service
	txnRepo  *transaction.Repository
	catRepo  *category.Repository
	checking *account.Account
	savings  *account.Account
}

func newLinkTestEnv(t *testing.T) *linkTestEnv {
	t.Helper()
	database := dbtest.New(t)
	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)
	accountRepo := account.NewRepository(database)
	catRepo := category.NewRepository(database)

	svc := NewService(txnRepo, transferRepo, splitRepo, accountRepo, database)

	openDate := types.NewDate(2000, time.January, 1)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, openDate)
	if err := accountRepo.Create(checking); err != nil {
		t.Fatalf("create checking: %v", err)
	}
	savings := account.NewAccount("Savings", account.TypeSavings, "USD", types.ZeroMoney, openDate)
	if err := accountRepo.Create(savings); err != nil {
		t.Fatalf("create savings: %v", err)
	}

	return &linkTestEnv{
		database: database,
		svc:      svc,
		txnRepo:  txnRepo,
		catRepo:  catRepo,
		checking: checking,
		savings:  savings,
	}
}

func (e *linkTestEnv) newCategory(t *testing.T, name string) *category.Category {
	t.Helper()
	cat := category.NewCategory(name, category.TypeExpense)
	if err := e.catRepo.Create(cat); err != nil {
		t.Fatalf("create category %q: %v", name, err)
	}
	return cat
}

// newTxn persists a plain (non-transfer) transaction in the given account with
// an optional category, mirroring an imported row before linking.
func (e *linkTestEnv) newTxn(t *testing.T, acctID types.ID, amount string, cat *category.Category) *transaction.Transaction {
	t.Helper()
	txn := transaction.NewTransaction(acctID, types.NewDate(2024, time.March, 1), types.MustNewMoney(amount))
	if cat != nil {
		txn.SetCategory(cat.ID)
	}
	if err := e.txnRepo.Create(txn); err != nil {
		t.Fatalf("create txn: %v", err)
	}
	return txn
}

// linkPair runs FindUnlinked + Link, expecting exactly one clean candidate to
// be linked, and returns the reloaded (persisted) from/to legs.
func (e *linkTestEnv) linkPair(t *testing.T, fromID, toID types.ID) (from, to *transaction.Transaction) {
	t.Helper()
	res, err := e.svc.FindUnlinked(DefaultMaxDateDiffDays)
	if err != nil {
		t.Fatalf("FindUnlinked: %v", err)
	}
	if len(res.Clean) != 1 {
		t.Fatalf("expected 1 clean candidate, got %d (ambiguous=%d)", len(res.Clean), len(res.Ambiguous))
	}
	linked, errs := e.svc.Link(res.Clean)
	if len(errs) != 0 {
		t.Fatalf("Link returned errors: %v", errs)
	}
	if linked != 1 {
		t.Fatalf("expected 1 linked, got %d", linked)
	}

	from, err = e.txnRepo.GetByID(fromID)
	if err != nil {
		t.Fatalf("reload from: %v", err)
	}
	to, err = e.txnRepo.GetByID(toID)
	if err != nil {
		t.Fatalf("reload to: %v", err)
	}
	if !from.IsTransfer() || !to.IsTransfer() {
		t.Fatalf("both legs should be transfers after link: from=%v to=%v", from.IsTransfer(), to.IsTransfer())
	}
	return from, to
}

func assertCategory(t *testing.T, txn *transaction.Transaction, want types.NullableID, label string) {
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

func TestLinkOne_AdoptsCategory_OneLegCategorized(t *testing.T) {
	// From leg categorized, To leg bare → category mirrored onto the To leg.
	t.Run("from categorized", func(t *testing.T) {
		env := newLinkTestEnv(t)
		bills := env.newCategory(t, "Bills")
		fromTxn := env.newTxn(t, env.checking.ID, "-100.00", bills)
		toTxn := env.newTxn(t, env.savings.ID, "100.00", nil)

		from, to := env.linkPair(t, fromTxn.ID, toTxn.ID)
		want := types.NullableID{ID: bills.ID, Valid: true}
		assertCategory(t, from, want, "from")
		assertCategory(t, to, want, "to")
	})

	// To leg categorized, From leg bare → category mirrored onto the From leg.
	t.Run("to categorized", func(t *testing.T) {
		env := newLinkTestEnv(t)
		bills := env.newCategory(t, "Bills")
		fromTxn := env.newTxn(t, env.checking.ID, "-100.00", nil)
		toTxn := env.newTxn(t, env.savings.ID, "100.00", bills)

		from, to := env.linkPair(t, fromTxn.ID, toTxn.ID)
		want := types.NullableID{ID: bills.ID, Valid: true}
		assertCategory(t, from, want, "from")
		assertCategory(t, to, want, "to")
	})
}

func TestLinkOne_BothDiffer_OutflowWins(t *testing.T) {
	env := newLinkTestEnv(t)
	fromCat := env.newCategory(t, "Bills")
	toCat := env.newCategory(t, "Groceries")
	fromTxn := env.newTxn(t, env.checking.ID, "-100.00", fromCat)
	toTxn := env.newTxn(t, env.savings.ID, "100.00", toCat)

	from, to := env.linkPair(t, fromTxn.ID, toTxn.ID)
	// The outflow (From / negative) leg's category wins on both legs.
	want := types.NullableID{ID: fromCat.ID, Valid: true}
	assertCategory(t, from, want, "from")
	assertCategory(t, to, want, "to")
}

func TestLinkOne_BothSame_Unchanged(t *testing.T) {
	env := newLinkTestEnv(t)
	bills := env.newCategory(t, "Bills")
	fromTxn := env.newTxn(t, env.checking.ID, "-100.00", bills)
	toTxn := env.newTxn(t, env.savings.ID, "100.00", bills)

	from, to := env.linkPair(t, fromTxn.ID, toTxn.ID)
	want := types.NullableID{ID: bills.ID, Valid: true}
	assertCategory(t, from, want, "from")
	assertCategory(t, to, want, "to")
}

func TestLinkOne_BothEmpty_NoCategory(t *testing.T) {
	env := newLinkTestEnv(t)
	fromTxn := env.newTxn(t, env.checking.ID, "-100.00", nil)
	toTxn := env.newTxn(t, env.savings.ID, "100.00", nil)

	from, to := env.linkPair(t, fromTxn.ID, toTxn.ID)
	assertCategory(t, from, types.NullableID{}, "from")
	assertCategory(t, to, types.NullableID{}, "to")
}

func TestLinkOne_RollbackRestoresCategories(t *testing.T) {
	// Force transferRepo.Update to fail (by deleting the From row before the
	// link write) and assert linkOne restores both legs' original categories
	// and clears the in-memory transfer link.
	env := newLinkTestEnv(t)
	fromCat := env.newCategory(t, "Bills")
	fromTxn := env.newTxn(t, env.checking.ID, "-100.00", fromCat)
	toTxn := env.newTxn(t, env.savings.ID, "100.00", nil)

	// Delete the persisted From row so the pair Update hits a missing row.
	if err := env.txnRepo.Delete(fromTxn.ID); err != nil {
		t.Fatalf("delete from row: %v", err)
	}

	c := &Candidate{
		From:        fromTxn,
		To:          toTxn,
		FromAccount: env.checking.Name,
		ToAccount:   env.savings.Name,
	}
	if err := env.svc.linkOne(c); err == nil {
		t.Fatalf("expected linkOne to fail on a missing From row")
	}

	// Categories restored to originals.
	assertCategory(t, c.From, types.NullableID{ID: fromCat.ID, Valid: true}, "from after rollback")
	assertCategory(t, c.To, types.NullableID{}, "to after rollback")
	// Transfer link cleared on both in-memory structs.
	if c.From.IsTransfer() || c.To.IsTransfer() {
		t.Fatalf("transfer link should be rolled back: from=%v to=%v", c.From.IsTransfer(), c.To.IsTransfer())
	}
}
