package integration

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transferlink"
	"github.com/haskovec/tmoney/internal/types"
)

type linkFixture struct {
	db          *db.DB
	accountRepo *account.Repository
	txnRepo     *transaction.Repository
	splitRepo   *transaction.SplitRepository
	xferRepo    *transaction.TransferRepository
	svc         *transferlink.Service
	checking    *account.Account
	savings     *account.Account
	visa        *account.Account
}

func newLinkFixture(t *testing.T) (*linkFixture, func()) {
	t.Helper()

	database := dbtest.New(t)

	accountRepo := account.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	xferRepo := transaction.NewTransferRepository(database, txnRepo)

	svc := transferlink.NewService(txnRepo, xferRepo, splitRepo, accountRepo, database)

	checking := account.NewAccount("Checking", account.TypeChecking, "USD",
		types.MustNewMoney("0"), types.NewDate(2024, 1, 1))
	savings := account.NewAccount("Savings", account.TypeSavings, "USD",
		types.MustNewMoney("0"), types.NewDate(2024, 1, 1))
	visa := account.NewAccount("Visa", account.TypeCreditCard, "USD",
		types.MustNewMoney("0"), types.NewDate(2024, 1, 1))
	for _, a := range []*account.Account{checking, savings, visa} {
		if err := accountRepo.Create(a); err != nil {
			t.Fatalf("Failed to create account %s: %v", a.Name, err)
		}
	}

	f := &linkFixture{
		db:          database,
		accountRepo: accountRepo,
		txnRepo:     txnRepo,
		splitRepo:   splitRepo,
		xferRepo:    xferRepo,
		svc:         svc,
		checking:    checking,
		savings:     savings,
		visa:        visa,
	}
	cleanup := func() {}
	return f, cleanup
}

func (f *linkFixture) addTxn(t *testing.T, accountID types.ID, date types.Date, amount string) *transaction.Transaction {
	t.Helper()
	tx := transaction.NewTransaction(accountID, date, types.MustNewMoney(amount))
	if err := f.txnRepo.Create(tx); err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}
	return tx
}

func TestTransferLink_CleanPair(t *testing.T) {
	f, cleanup := newLinkFixture(t)
	defer cleanup()

	out := f.addTxn(t, f.checking.ID, types.NewDate(2024, 1, 10), "-200.00")
	in := f.addTxn(t, f.savings.ID, types.NewDate(2024, 1, 11), "200.00")

	res, err := f.svc.FindUnlinked(transferlink.DefaultMaxDateDiffDays)
	if err != nil {
		t.Fatalf("FindUnlinked: %v", err)
	}
	if len(res.Clean) != 1 {
		t.Fatalf("expected 1 clean candidate, got %d", len(res.Clean))
	}
	if len(res.Ambiguous) != 0 {
		t.Fatalf("expected 0 ambiguous, got %d", len(res.Ambiguous))
	}
	c := res.Clean[0]
	if c.From.ID != out.ID || c.To.ID != in.ID {
		t.Errorf("wrong direction: from=%s to=%s", c.From.ID, c.To.ID)
	}
	if c.DateDiffDays != 1 {
		t.Errorf("date diff = %d, want 1", c.DateDiffDays)
	}

	linked, errs := f.svc.Link(res.Clean)
	if len(errs) != 0 {
		t.Fatalf("link errors: %v", errs)
	}
	if linked != 1 {
		t.Errorf("linked count = %d, want 1", linked)
	}

	// Re-fetch and confirm both sides are now transfers with matching IDs.
	outAfter, err := f.txnRepo.GetByID(out.ID)
	if err != nil {
		t.Fatalf("GetByID out: %v", err)
	}
	inAfter, err := f.txnRepo.GetByID(in.ID)
	if err != nil {
		t.Fatalf("GetByID in: %v", err)
	}
	if !outAfter.IsTransfer() || !inAfter.IsTransfer() {
		t.Errorf("both sides should be transfers after Link")
	}
	if outAfter.TransferID.ID != inAfter.TransferID.ID {
		t.Errorf("transfer IDs do not match")
	}
	if outAfter.TransferAccountID.ID != f.savings.ID {
		t.Errorf("out.TransferAccountID = %s, want savings", outAfter.TransferAccountID.ID)
	}
	if inAfter.TransferAccountID.ID != f.checking.ID {
		t.Errorf("in.TransferAccountID = %s, want checking", inAfter.TransferAccountID.ID)
	}
}

func TestTransferLink_OutsideDateWindow(t *testing.T) {
	f, cleanup := newLinkFixture(t)
	defer cleanup()

	f.addTxn(t, f.checking.ID, types.NewDate(2024, 1, 1), "-100.00")
	f.addTxn(t, f.savings.ID, types.NewDate(2024, 1, 20), "100.00")

	res, err := f.svc.FindUnlinked(5)
	if err != nil {
		t.Fatalf("FindUnlinked: %v", err)
	}
	if len(res.Clean)+len(res.Ambiguous) != 0 {
		t.Errorf("expected no candidates, got %d clean + %d ambiguous",
			len(res.Clean), len(res.Ambiguous))
	}
}

func TestTransferLink_AmbiguousMatch(t *testing.T) {
	f, cleanup := newLinkFixture(t)
	defer cleanup()

	// Two -100 outs in checking, one +100 in in savings — the in-side
	// matches both outs, so every candidate is ambiguous.
	f.addTxn(t, f.checking.ID, types.NewDate(2024, 1, 10), "-100.00")
	f.addTxn(t, f.checking.ID, types.NewDate(2024, 1, 11), "-100.00")
	f.addTxn(t, f.savings.ID, types.NewDate(2024, 1, 11), "100.00")

	res, err := f.svc.FindUnlinked(5)
	if err != nil {
		t.Fatalf("FindUnlinked: %v", err)
	}
	if len(res.Clean) != 0 {
		t.Errorf("expected 0 clean candidates, got %d", len(res.Clean))
	}
	if len(res.Ambiguous) != 2 {
		t.Errorf("expected 2 ambiguous candidates, got %d", len(res.Ambiguous))
	}
}

func TestTransferLink_SkipsAlreadyLinked(t *testing.T) {
	f, cleanup := newLinkFixture(t)
	defer cleanup()

	out := f.addTxn(t, f.checking.ID, types.NewDate(2024, 1, 10), "-50.00")
	in := f.addTxn(t, f.savings.ID, types.NewDate(2024, 1, 10), "50.00")

	// Pre-link them.
	transferID := types.NewID()
	out.SetTransfer(transferID, f.savings.ID)
	in.SetTransfer(transferID, f.checking.ID)
	if err := f.xferRepo.Update(&transaction.TransferPair{
		FromTransaction: out,
		ToTransaction:   in,
	}); err != nil {
		t.Fatalf("pre-link: %v", err)
	}

	res, err := f.svc.FindUnlinked(5)
	if err != nil {
		t.Fatalf("FindUnlinked: %v", err)
	}
	if len(res.Clean)+len(res.Ambiguous) != 0 {
		t.Errorf("expected already-linked to be skipped, got %d candidates",
			len(res.Clean)+len(res.Ambiguous))
	}
}

func TestTransferLink_SkipsSplits(t *testing.T) {
	f, cleanup := newLinkFixture(t)
	defer cleanup()

	out := f.addTxn(t, f.checking.ID, types.NewDate(2024, 1, 10), "-75.00")
	f.addTxn(t, f.savings.ID, types.NewDate(2024, 1, 10), "75.00")

	// Add a split row to the checking-side transaction so it's no longer
	// eligible for transfer linking.
	catRepo := category.NewRepository(f.db)
	cat := category.NewCategory("Misc", category.TypeExpense)
	if err := catRepo.Create(cat); err != nil {
		t.Fatalf("create category: %v", err)
	}
	split := transaction.NewSplit(out.ID, cat.ID, types.MustNewMoney("-75.00"))
	if err := f.splitRepo.Create(split); err != nil {
		t.Fatalf("split create: %v", err)
	}

	res, err := f.svc.FindUnlinked(5)
	if err != nil {
		t.Fatalf("FindUnlinked: %v", err)
	}
	if len(res.Clean)+len(res.Ambiguous) != 0 {
		t.Errorf("expected split transaction to be skipped, got %d candidates",
			len(res.Clean)+len(res.Ambiguous))
	}
}

func TestTransferLink_SameAccountIgnored(t *testing.T) {
	f, cleanup := newLinkFixture(t)
	defer cleanup()

	f.addTxn(t, f.checking.ID, types.NewDate(2024, 1, 10), "-25.00")
	f.addTxn(t, f.checking.ID, types.NewDate(2024, 1, 10), "25.00")

	res, err := f.svc.FindUnlinked(5)
	if err != nil {
		t.Fatalf("FindUnlinked: %v", err)
	}
	if len(res.Clean)+len(res.Ambiguous) != 0 {
		t.Errorf("expected same-account pair to be ignored, got %d candidates",
			len(res.Clean)+len(res.Ambiguous))
	}
}

func TestTransferLink_LinkInvalidCandidate(t *testing.T) {
	f, cleanup := newLinkFixture(t)
	defer cleanup()

	out := f.addTxn(t, f.checking.ID, types.NewDate(2024, 1, 10), "-30.00")
	mismatch := f.addTxn(t, f.savings.ID, types.NewDate(2024, 1, 10), "29.00")

	bad := &transferlink.Candidate{
		From:        out,
		To:          mismatch,
		FromAccount: "Checking",
		ToAccount:   "Savings",
	}
	linked, errs := f.svc.Link([]*transferlink.Candidate{bad})
	if linked != 0 {
		t.Errorf("linked = %d, want 0 for invalid candidate", linked)
	}
	if len(errs) != 1 {
		t.Errorf("errors = %d, want 1", len(errs))
	}
	if out.IsTransfer() || mismatch.IsTransfer() {
		t.Errorf("transactions should not be linked when validation fails")
	}
}
