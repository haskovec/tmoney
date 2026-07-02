package scheduled

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

// TestListDue_ExcludesCompleted is the regression guard for the zombie-due bug:
// a naturally-exhausted fixed-occurrence schedule (occurrences_remaining == 0)
// whose next_date is in the past must not surface as due.
func TestListDue_ExcludesCompleted(t *testing.T) {
	svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
	acct := createTestAccountForScheduled(t, accountRepo, "Checking")

	pastDue := types.NewDate(2026, time.June, 1)

	// A one-shot schedule that will exhaust after a single post.
	oneShot := NewTransactionWithAmount(acct.ID, FrequencyMonthly, pastDue, types.MustNewMoney("-50.00"))
	oneShot.SetDayOfMonth(1)
	oneShot.SetOccurrences(1)
	if err := svc.Create(oneShot); err != nil {
		t.Fatalf("create one-shot schedule: %v", err)
	}

	// An ordinary indefinite schedule that is also past-due.
	ongoing := NewTransactionWithAmount(acct.ID, FrequencyMonthly, pastDue, types.MustNewMoney("-25.00"))
	ongoing.SetDayOfMonth(1)
	if err := svc.Create(ongoing); err != nil {
		t.Fatalf("create ongoing schedule: %v", err)
	}

	// Before posting, both are due.
	due, err := svc.ListDue()
	if err != nil {
		t.Fatalf("ListDue: %v", err)
	}
	if len(due) != 2 {
		t.Fatalf("expected 2 due before posting, got %d", len(due))
	}

	// Post the one-shot: it exhausts (occurrences_remaining -> 0) but its
	// next_date stays in the past.
	if _, err := svc.Post(oneShot.ID, nil); err != nil {
		t.Fatalf("post one-shot: %v", err)
	}

	completed, err := svc.GetByID(oneShot.ID)
	if err != nil {
		t.Fatalf("reload one-shot: %v", err)
	}
	if !completed.IsCompleted() {
		t.Fatal("one-shot schedule should be completed after its single post")
	}

	due, err = svc.ListDue()
	if err != nil {
		t.Fatalf("ListDue after post: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("expected 1 due after the one-shot completed, got %d", len(due))
	}
	if due[0].ID != ongoing.ID {
		t.Errorf("the remaining due schedule should be the ongoing one, got %s", due[0].ID)
	}
	for _, st := range due {
		if st.ID == oneShot.ID {
			t.Error("a completed schedule must not appear in ListDue (zombie-due regression)")
		}
	}
}

// TestListDue_ExcludesEndDatePassed guards the end_date branch of the
// completion predicate, mirroring Transaction.IsCompleted: a schedule whose
// next_date sits past its end_date is complete and must not be due. The state
// is created directly through the repository (bypassing service validation) to
// exercise the SQL clause in isolation.
func TestListDue_ExcludesEndDatePassed(t *testing.T) {
	svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
	acct := createTestAccountForScheduled(t, accountRepo, "Checking")

	st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.NewDate(2026, time.June, 1), types.MustNewMoney("-10.00"))
	st.SetDayOfMonth(1)
	st.EndDate = types.NullableDate{Date: types.NewDate(2026, time.June, 15), Valid: true}
	// next_date already past the end date — the terminal end_date state.
	st.NextDate = types.NewDate(2026, time.July, 1)
	if err := svc.repo.Create(st); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := svc.GetByID(st.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !got.IsCompleted() {
		t.Fatal("a schedule with next_date past its end_date should be completed")
	}

	due, err := svc.ListDue()
	if err != nil {
		t.Fatalf("ListDue: %v", err)
	}
	for _, d := range due {
		if d.ID == st.ID {
			t.Error("a schedule past its end_date must not appear in ListDue")
		}
	}
}
