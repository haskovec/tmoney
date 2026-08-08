package scheduled

import (
	"errors"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// Two schedules reach their last occurrence by different routes — an occurrence
// count running out, or the next date passing end_date — and only the first one
// used to reach a terminal state. The tests here pin both.

// TestAdvanceSchedule_EndDateBoundedScheduleCompletes is the root-cause test.
//
// AdvanceSchedule is the ONLY thing that moves NextDate, and it used to refuse to
// move it past end_date. IsCompleted's end-date branch asks exactly the opposite
// question — "is NextDate past end_date?" — so that branch could never fire, and
// an end-date-bounded schedule stayed forever due on its final occurrence.
//
// The strongest evidence this was a bug and not a design is that the branch was
// already written, and that MarkCompleted's own doc comment warns about "the
// end_date trick, which strands NextDate == EndDate".
func TestAdvanceSchedule_EndDateBoundedScheduleCompletes(t *testing.T) {
	// Monthly from June 1, ending August 1: three occurrences, then done.
	st := NewTransactionWithAmount(types.NewID(), FrequencyMonthly,
		types.NewDate(2024, time.June, 1), types.MustNewMoney("-50.00"))
	st.SetEndDate(types.NewDate(2024, time.August, 1))

	for i, want := range []types.Date{
		types.NewDate(2024, time.July, 1),
		types.NewDate(2024, time.August, 1),
	} {
		if st.IsCompleted() {
			t.Fatalf("schedule reported completed before occurrence %d", i+1)
		}
		if !st.AdvanceSchedule() {
			t.Fatalf("AdvanceSchedule() returned false at occurrence %d, want true", i+1)
		}
		if !st.NextDate.Equal(want) {
			t.Fatalf("after occurrence %d next_date = %s, want %s", i+1, st.NextDate, want)
		}
	}

	// The third occurrence is the last one the end date allows.
	if st.IsCompleted() {
		t.Fatal("schedule reported completed before its final occurrence")
	}
	if st.AdvanceSchedule() {
		t.Error("AdvanceSchedule() returned true past the end date, want false")
	}
	if !st.IsCompleted() {
		t.Error("schedule is not completed after its final occurrence — it stays due forever, " +
			"and AutoPost's loop condition can never go false")
	}
	if !st.NextDate.After(st.EndDate.Date) {
		t.Errorf("next_date = %s, want a date past end_date %s — the terminal state IsCompleted "+
			"tests for is unreachable otherwise", st.NextDate, st.EndDate.Date)
	}
}

// TestAutoPost_EndDateBoundedScheduleTerminates is the same bug seen from
// AutoPost, where its consequence is a hang rather than a stuck schedule: the
// loop guard never goes false, so the catch-up spins inside one open transaction
// inserting rows that never commit, until memory or disk runs out. The user sees
// the application freeze on file open.
//
// If the bug ever returns this test FAILS on the occurrence count rather than
// hanging, because AutoPost now also honours AdvanceSchedule's "that was the
// last one" return. The two guards are independent on purpose.
func TestAutoPost_EndDateBoundedScheduleTerminates(t *testing.T) {
	svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
	acct := createTestAccountForScheduled(t, accountRepo, "Checking")

	st := NewTransactionWithAmount(acct.ID, FrequencyMonthly,
		types.NewDate(2024, time.June, 1), types.MustNewMoney("-50.00"))
	st.SetEndDate(types.NewDate(2024, time.August, 1))
	st.SetAutoPost(true)
	st.SetPostLeadDays(0)
	if err := svc.Create(st); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	summary, err := svc.AutoPost()
	if err != nil {
		t.Fatalf("AutoPost(): %v", err)
	}
	if summary.PostedCount != 3 {
		t.Errorf("PostedCount = %d, want 3 (June, July and August)", summary.PostedCount)
	}

	after, err := svc.GetByID(st.ID)
	if err != nil {
		t.Fatalf("GetByID(): %v", err)
	}
	if !after.IsCompleted() {
		t.Error("the schedule is still not completed, so the next file open posts it all over again")
	}

	// A second run must be a no-op. This is the assertion that would catch a
	// schedule that looks advanced but was never persisted.
	second, err := svc.AutoPost()
	if err != nil {
		t.Fatalf("second AutoPost(): %v", err)
	}
	if second.PostedCount != 0 {
		t.Errorf("second AutoPost posted %d more occurrences, want 0", second.PostedCount)
	}
}

// TestAutoPost_InvestmentToInvestmentTransferAdvancesTheSchedule pins the second
// bug: an occurrence that posts no REGULAR-ledger row is still an occurrence.
//
// An investment-to-investment transfer writes both legs to
// investment_transactions, so AutoPost's result.Transactions — which holds
// regular rows only — stays empty. The persist gate keyed on that list, so the
// advance made in memory was never written and the schedule re-posted on every
// single file open, silently duplicating the transfer. PostedCount was still
// incremented, so the TUI reported success and pushed an undo entry over a
// Results slice that had no entry to undo.
func TestAutoPost_InvestmentToInvestmentTransferAdvancesTheSchedule(t *testing.T) {
	env := newTransferTestEnv(t)
	from := env.account(t, "Rollover IRA", account.TypeInvestment)
	to := env.account(t, "Roth IRA", account.TypeInvestment)

	// newTransferTestEnv opens its accounts today, so the occurrence is dated
	// today too — an earlier date would trip the opening-date guard.
	st := newTransferSchedule(from.ID, to.ID, "500.00")
	st.SetOccurrences(1)
	st.SetAutoPost(true)
	st.SetPostLeadDays(0)
	if err := env.svc.Create(st); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	originalNext := st.NextDate

	summary, err := env.svc.AutoPost()
	if err != nil {
		t.Fatalf("AutoPost(): %v", err)
	}
	if summary.PostedCount != 1 {
		t.Fatalf("PostedCount = %d, want 1", summary.PostedCount)
	}

	// The occurrence must be reportable, or undo cannot reach the posted pair.
	if len(summary.Results) != 1 {
		t.Fatalf("summary.Results has %d entries, want 1 — undo cannot reach a candidate it never sees",
			len(summary.Results))
	}
	if len(summary.Results[0].TransferIDs) != 1 {
		t.Errorf("TransferIDs has %d entries, want 1 — undo addresses the pair by transfer_id",
			len(summary.Results[0].TransferIDs))
	}

	// The advance must have been PERSISTED, not just made in memory.
	after, err := env.svc.GetByID(st.ID)
	if err != nil {
		t.Fatalf("GetByID(): %v", err)
	}
	if after.NextDate.Equal(originalNext) && !after.IsCompleted() {
		t.Error("the schedule did not advance on disk — every file open re-posts this transfer")
	}

	// Which a second run proves directly.
	second, err := env.svc.AutoPost()
	if err != nil {
		t.Fatalf("second AutoPost(): %v", err)
	}
	if second.PostedCount != 0 {
		t.Errorf("second AutoPost posted %d more occurrences, want 0 — the transfer is duplicated on every open",
			second.PostedCount)
	}

	invRepo := investment.NewRepository(env.svc.db)
	for _, acct := range []struct {
		name string
		id   types.ID
	}{{"Rollover IRA", from.ID}, {"Roth IRA", to.ID}} {
		rows, rerr := invRepo.ListByAccount(acct.id, investment.TransactionFilter{})
		if rerr != nil {
			t.Fatalf("ListByAccount(%s): %v", acct.name, rerr)
		}
		if len(rows) != 1 {
			t.Errorf("%s holds %d investment rows, want exactly 1", acct.name, len(rows))
		}
	}
}

// TestPost_EndDateBoundedScheduleRefusesAfterItsLastOccurrence covers the
// manual half of the same bug, which is quieter than the AutoPost hang and
// arguably worse: a schedule bounded by end_date could be posted again and again
// past its end, because the guard that should have refused it asks IsCompleted
// and IsCompleted could never be true.
//
// Five places in the codebase were already written for the terminal state this
// restores — IsCompleted's end-date branch, ListDue's `next_date <= end_date`
// predicate, the TUI's upcoming filter, the CLI's "no more occurrences" line,
// and this guard.
func TestPost_EndDateBoundedScheduleRefusesAfterItsLastOccurrence(t *testing.T) {
	svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
	acct := createTestAccountForScheduled(t, accountRepo, "Checking")

	// Two occurrences: June 1 and July 1. Validation requires end_date to be
	// after start_date, so one occurrence cannot be expressed this way.
	start := types.NewDate(2024, time.June, 1)
	st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, start, types.MustNewMoney("-50.00"))
	st.SetEndDate(types.NewDate(2024, time.July, 1))
	if err := svc.Create(st); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	for i := 1; i <= 2; i++ {
		if _, err := svc.Post(st.ID, nil); err != nil {
			t.Fatalf("Post() %d: %v", i, err)
		}
	}

	// The series is over. A third post must be refused, not silently accepted.
	_, err := svc.Post(st.ID, nil)
	if err == nil {
		t.Fatal("Post() succeeded past the end date — the schedule can be posted forever")
	}
	var completed *CompletedError
	if !errors.As(err, &completed) {
		t.Errorf("Post() error = %T (%v), want *CompletedError", err, err)
	}

	// And exactly two transactions exist.
	txns, lerr := transaction.NewRepository(svc.db).ListByAccount(acct.ID)
	if lerr != nil {
		t.Fatalf("ListByAccount(): %v", lerr)
	}
	if len(txns) != 2 {
		t.Errorf("%d transactions posted, want exactly 2", len(txns))
	}
}

// TestListPredicates_ExcludeCompletedSchedules pins the three list queries
// against Transaction.IsCompleted.
//
// ListDue already carried both completion clauses. ListUpcoming and
// ListAutoPostDue carried neither, which was masked while completion was
// unreachable: a schedule stranded on its last occurrence sat in ListDue
// forever. Making completion reachable moved the problem rather than solving it
// — the row's next_date is now a fixed FUTURE date, so it satisfies
// ListUpcoming's `next_date <= today + N` permanently, sorts to the front of an
// ascending order that every live schedule keeps moving away from, and consumes
// a slot in the dashboard's upcoming panel forever.
func TestListPredicates_ExcludeCompletedSchedules(t *testing.T) {
	svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
	acct := createTestAccountForScheduled(t, accountRepo, "Checking")

	// Exhausted by end date.
	byEndDate := NewTransactionWithAmount(acct.ID, FrequencyMonthly,
		types.NewDate(2024, time.June, 1), types.MustNewMoney("-50.00"))
	byEndDate.SetEndDate(types.NewDate(2024, time.July, 1))
	byEndDate.SetAutoPost(true)
	byEndDate.SetPostLeadDays(0)
	if err := svc.Create(byEndDate); err != nil {
		t.Fatalf("Create(byEndDate): %v", err)
	}

	// Exhausted by occurrence count.
	byCount := NewTransactionWithAmount(acct.ID, FrequencyMonthly,
		types.NewDate(2024, time.June, 1), types.MustNewMoney("-60.00"))
	byCount.SetOccurrences(2)
	byCount.SetAutoPost(true)
	byCount.SetPostLeadDays(0)
	if err := svc.Create(byCount); err != nil {
		t.Fatalf("Create(byCount): %v", err)
	}

	// Run both to exhaustion.
	if _, err := svc.AutoPost(); err != nil {
		t.Fatalf("AutoPost(): %v", err)
	}
	for _, st := range []*Transaction{byEndDate, byCount} {
		after, err := svc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID(): %v", err)
		}
		if !after.IsCompleted() {
			t.Fatalf("schedule %s is not completed; the fixture is wrong", st.ID)
		}
	}

	// None of the three list surfaces may still offer them.
	due, err := svc.ListDue()
	if err != nil {
		t.Fatalf("ListDue(): %v", err)
	}
	if len(due) != 0 {
		t.Errorf("ListDue returned %d completed schedules, want 0", len(due))
	}

	upcoming, err := svc.ListUpcoming(3650)
	if err != nil {
		t.Fatalf("ListUpcoming(): %v", err)
	}
	if len(upcoming) != 0 {
		t.Errorf("ListUpcoming returned %d completed schedules, want 0 — each is a permanent "+
			"phantom on the dashboard, on a date that will never occur", len(upcoming))
	}

	second, err := svc.AutoPost()
	if err != nil {
		t.Fatalf("second AutoPost(): %v", err)
	}
	if second.PostedCount != 0 || len(second.Results) != 0 {
		t.Errorf("second AutoPost saw completed candidates: posted=%d results=%d, want 0/0",
			second.PostedCount, len(second.Results))
	}
}

// TestAdvanceSchedule_ExhaustedByCountStillAdvances covers the other terminal
// route. The occurrences branch used to report terminality by withholding the
// advance, leaving next_date on the occurrence just posted — so re-arming the
// schedule through the TUI's Occurrences field, which resets the counter, posted
// that same occurrence a second time.
func TestAdvanceSchedule_ExhaustedByCountStillAdvances(t *testing.T) {
	start := types.NewDate(2024, time.June, 1)
	st := NewTransactionWithAmount(types.NewID(), FrequencyMonthly, start, types.MustNewMoney("-50.00"))
	st.SetOccurrences(2)

	if !st.AdvanceSchedule() {
		t.Fatal("AdvanceSchedule() returned false on the first of two occurrences")
	}
	if !st.NextDate.Equal(types.NewDate(2024, time.July, 1)) {
		t.Fatalf("next_date = %s after occurrence 1, want 2024-07-01", st.NextDate)
	}

	if st.AdvanceSchedule() {
		t.Error("AdvanceSchedule() returned true after the last occurrence, want false")
	}
	if !st.IsCompleted() {
		t.Error("schedule is not completed after its last occurrence")
	}
	if !st.NextDate.Equal(types.NewDate(2024, time.August, 1)) {
		t.Errorf("next_date = %s after the last occurrence, want 2024-08-01 — leaving it on the "+
			"occurrence just posted means re-arming the schedule re-posts that occurrence", st.NextDate)
	}

	// Re-arming must resume AFTER what was already posted, not repeat it.
	st.SetOccurrences(2)
	if st.IsCompleted() {
		t.Fatal("re-armed schedule still reports completed")
	}
	if !st.NextDate.Equal(types.NewDate(2024, time.August, 1)) {
		t.Errorf("re-armed next_date = %s, want 2024-08-01", st.NextDate)
	}
}
