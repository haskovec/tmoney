package scheduled

import (
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// The tests in this file pin the two properties that survive the collapse of
// AutoPost's inlined posting engine onto the shared one. They were written and
// run BEFORE that collapse, against the inlined engine, so the numbers below are
// recorded behavior rather than prediction.

// TestAutoPost_MultiOccurrenceVariableAmount pins divergence (1) from
// specs/design-service-decomposition.md section 5.2: how a VARIABLE-amount
// schedule prices each occurrence of a multi-occurrence catch-up.
//
// The estimate averages the last N transactions to the payee, read through the
// receiver's queryer. AutoPost runs a candidate's whole catch-up inside ONE
// transaction, so each occurrence's estimate sees the occurrences posted just
// before it — the average moves as the loop runs. A manual post cannot observe
// this, because it posts one occurrence per transaction.
//
// That feedback is exactly what a naive collapse would lose. Re-reading the
// schedule row through EstimateAmount, the way the manual path does, would price
// all three occurrences off the seeded history alone and post -150.00 three times.
func TestAutoPost_MultiOccurrenceVariableAmount(t *testing.T) {
	svc, accountRepo, payeeRepo, _ := createTestScheduledTransactionService(t)
	acct := createTestAccountForScheduled(t, accountRepo, "Checking")

	py := payee.NewPayee("Water Utility")
	if err := payeeRepo.Create(py); err != nil {
		t.Fatalf("Create payee: %v", err)
	}

	// Seeded history: the only two transactions the first occurrence can average.
	txnRepo := transaction.NewRepository(svc.db)
	for _, seed := range []struct {
		date   types.Date
		amount string
	}{
		{types.NewDate(2024, time.January, 1), "-100.00"},
		{types.NewDate(2024, time.February, 1), "-200.00"},
	} {
		txn := transaction.NewTransaction(acct.ID, seed.date, types.MustNewMoney(seed.amount))
		txn.SetPayee(py.ID)
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("seed %s: %v", seed.amount, err)
		}
	}

	// A variable-amount schedule priced from the last 2 transactions to this
	// payee. Three occurrences, all already overdue.
	st := NewTransaction(acct.ID, FrequencyMonthly, types.NewDate(2024, time.June, 1))
	st.SetPayee(py.ID)
	st.SetAmountEstimateCount(2)
	st.SetOccurrences(3)
	st.SetAutoPost(true)
	st.SetPostLeadDays(0)
	if err := svc.Create(st); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	summary, err := svc.AutoPost()
	if err != nil {
		t.Fatalf("AutoPost() error = %v", err)
	}
	if summary.PostedCount != 3 {
		t.Fatalf("PostedCount = %d, want 3", summary.PostedCount)
	}
	if len(summary.Results) != 1 {
		t.Fatalf("Results = %d, want 1", len(summary.Results))
	}

	posted := summary.Results[0].Transactions
	if len(posted) != 3 {
		t.Fatalf("posted %d occurrences, want 3", len(posted))
	}

	// Occurrence 1 averages the seeded -200 and -100      -> -150.00
	// Occurrence 2 averages -150 (just posted) and -200   -> -175.00
	// Occurrence 3 averages -175 (just posted) and -150   -> -162.50
	want := []struct {
		date   types.Date
		amount string
	}{
		{types.NewDate(2024, time.June, 1), "-150.00"},
		{types.NewDate(2024, time.July, 1), "-175.00"},
		{types.NewDate(2024, time.August, 1), "-162.50"},
	}
	for i, w := range want {
		if !posted[i].Amount.Equal(types.MustNewMoney(w.amount)) {
			t.Errorf("occurrence %d amount = %s, want %s", i+1, posted[i].Amount, w.amount)
		}
		if !posted[i].Date.Equal(w.date) {
			t.Errorf("occurrence %d date = %s, want %s", i+1, posted[i].Date, w.date)
		}
	}

	after, err := svc.GetByID(st.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if !after.IsCompleted() {
		t.Error("schedule should be completed after its last occurrence")
	}
}

// TestAutoPost_MultiOccurrenceCandidateFailsAsAWhole is the first half of
// AutoPost's transaction boundary: EVERY overdue occurrence of one candidate,
// plus that candidate's schedule advance, lives in a single transaction.
//
// The candidate below has two overdue occurrences. The fault lands on the second
// occurrence's INSERT, after the first has already succeeded. Nothing may survive
// — not the first occurrence's row, and not the advance. A collapse that moved
// the transaction boundary down to per-occurrence would leave the first
// occurrence committed and still pass every other test in this package.
//
// The second half — that a candidate's rollback does not disturb candidates
// already committed — needs an UNBOUND service and so cannot be asserted here;
// it is TestAutoPost_EarlierCandidateStaysCommittedWhenALaterOneFails below.
func TestAutoPost_MultiOccurrenceCandidateFailsAsAWhole(t *testing.T) {
	svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
	acct := createTestAccountForScheduled(t, accountRepo, "Checking")

	st := NewTransactionWithAmount(acct.ID, FrequencyMonthly,
		types.NewDate(2024, time.June, 1), types.MustNewMoney("-20.00"))
	st.SetOccurrences(2)
	st.SetAutoPost(true)
	st.SetPostLeadDays(0)
	if err := svc.Create(st); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	origNextDate := st.NextDate

	// Exec order inside the candidate's tx: occurrence 1 INSERT (#1), occurrence 2
	// INSERT (#2), schedule UPDATE (#3). Fail #2 — mid-candidate, one occurrence
	// already written.
	fw := &failingQueryer{failOn: 2}
	err := svc.db.WithTx(func(tx db.Queryer) error {
		fw.inner = tx
		_, e := svc.InTx(fw).AutoPost()
		return e
	})
	if err == nil {
		t.Fatal("expected the injected fault to surface, got nil")
	}
	if fw.execN != 2 {
		t.Errorf("AutoPost stopped at exec #%d, want #2 — it did not reach the second occurrence", fw.execN)
	}

	txnRepo := transaction.NewRepository(svc.db)
	txns, lerr := txnRepo.ListByAccount(acct.ID)
	if lerr != nil {
		t.Fatalf("ListByAccount(): %v", lerr)
	}
	if len(txns) != 0 {
		t.Errorf("%d rows survived the rollback, want 0 — the first occurrence was not rolled back with the candidate", len(txns))
	}

	after, gerr := svc.GetByID(st.ID)
	if gerr != nil {
		t.Fatalf("GetByID(): %v", gerr)
	}
	if !after.NextDate.Equal(origNextDate) {
		t.Errorf("next_date advanced despite rollback: got %s, want %s", after.NextDate, origNextDate)
	}
}

// TestAutoPost_EarlierCandidateStaysCommittedWhenALaterOneFails is the second
// half of the boundary: one transaction PER CANDIDATE, not one for the batch. A
// candidate that fails hard must not take down candidates already posted.
//
// The fault has to be a real one rather than an injected one. Injection reaches
// AutoPost only through svc.InTx, which binds the service — and a bound AutoPost
// joins the caller's transaction for every candidate, collapsing the very
// boundary under test. db.DB is a concrete type with no seam to wrap, so the
// failure here comes from wiring instead: the service has no transfer port, so
// the transfer candidate cannot post, while the plain candidate before it can.
func TestAutoPost_EarlierCandidateStaysCommittedWhenALaterOneFails(t *testing.T) {
	database := createTestDB(t)
	stRepo := NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	accountRepo := account.NewRepository(database)
	payeeRepo := payee.NewRepository(database)
	txnSvc := transaction.NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)
	// Deliberately no SetTransferPort: posting a transfer occurrence must fail.
	svc := NewService(stRepo, txnRepo, txnSvc, database, accountRepo)

	first := createTestAccountForScheduled(t, accountRepo, "First")
	second := createTestAccountForScheduled(t, accountRepo, "Second")
	third := createTestAccountForScheduled(t, accountRepo, "Third")

	// Candidates are ordered by next_date ASC, so the plain one is processed first.
	plain := NewTransactionWithAmount(first.ID, FrequencyMonthly,
		types.NewDate(2024, time.June, 1), types.MustNewMoney("-10.00"))
	plain.SetOccurrences(1)
	plain.SetAutoPost(true)
	plain.SetPostLeadDays(0)
	if err := svc.Create(plain); err != nil {
		t.Fatalf("Create(plain): %v", err)
	}

	xfer := NewTransactionWithAmount(second.ID, FrequencyMonthly,
		types.NewDate(2024, time.July, 1), types.MustNewMoney("-20.00"))
	xfer.SetTransfer(third.ID)
	xfer.SetOccurrences(2)
	xfer.SetAutoPost(true)
	xfer.SetPostLeadDays(0)
	if err := svc.Create(xfer); err != nil {
		t.Fatalf("Create(xfer): %v", err)
	}
	xferNextDate := xfer.NextDate

	_, err := svc.AutoPost()
	if err == nil {
		t.Fatal("expected the transfer candidate to fail, got nil")
	}
	if !strings.Contains(err.Error(), "transfer service") {
		t.Errorf("error = %v, want the missing-transfer-port failure", err)
	}

	// The plain candidate committed in its OWN transaction and is untouched.
	firstTxns, lerr := txnRepo.ListByAccount(first.ID)
	if lerr != nil {
		t.Fatalf("ListByAccount(First): %v", lerr)
	}
	if len(firstTxns) != 1 {
		t.Fatalf("First holds %d rows, want 1 — an earlier candidate lost its commit", len(firstTxns))
	}
	if !firstTxns[0].Amount.Equal(types.MustNewMoney("-10.00")) {
		t.Errorf("First row amount = %s, want -10.00", firstTxns[0].Amount)
	}
	plainAfter, gerr := svc.GetByID(plain.ID)
	if gerr != nil {
		t.Fatalf("GetByID(plain): %v", gerr)
	}
	if !plainAfter.IsCompleted() {
		t.Error("the committed candidate's schedule advance was rolled back with the failing candidate")
	}

	// The failing candidate wrote nothing and did not advance.
	for _, acct := range []struct {
		name string
		id   types.ID
	}{{"Second", second.ID}, {"Third", third.ID}} {
		txns, terr := txnRepo.ListByAccount(acct.id)
		if terr != nil {
			t.Fatalf("ListByAccount(%s): %v", acct.name, terr)
		}
		if len(txns) != 0 {
			t.Errorf("%s holds %d rows, want 0", acct.name, len(txns))
		}
	}
	xferAfter, gerr := svc.GetByID(xfer.ID)
	if gerr != nil {
		t.Fatalf("GetByID(xfer): %v", gerr)
	}
	if !xferAfter.NextDate.Equal(xferNextDate) {
		t.Errorf("failing candidate advanced: got %s, want %s", xferAfter.NextDate, xferNextDate)
	}
}

// TestPost_AmountOverrideBeatsTheTemplateAmount pins a precedence the suite did
// not cover and the collapse could have inverted for free.
//
// postOccurrenceSingle reads `if amount != nil` BEFORE falling back to
// st.Amount.Money. Reversing those two — the natural tidy-up when merging an
// if/else chain into a switch — would silently make `tmoney scheduled post <id>
// --amount ...` ignore the override on any schedule that already carries a fixed
// amount, and no other test would notice.
func TestPost_AmountOverrideBeatsTheTemplateAmount(t *testing.T) {
	svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
	acct := createTestAccountForScheduled(t, accountRepo, "Checking")

	st := NewTransactionWithAmount(acct.ID, FrequencyMonthly,
		types.NewDate(2024, time.June, 1), types.MustNewMoney("-50.00"))
	if err := svc.Create(st); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	override := types.MustNewMoney("-125.00")
	txn, err := svc.Post(st.ID, &override)
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	if !txn.Amount.Equal(override) {
		t.Errorf("posted amount = %s, want the override %s (the template's -50.00 won)", txn.Amount, override)
	}
}
