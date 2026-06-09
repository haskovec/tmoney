package scheduled

import (
	"errors"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/types"
)

// These tests pin the rule that a schedule may not target a closed account: it
// cannot be created against one, a manual post into one is refused (and the
// schedule stays due), and auto-post skips it rather than erroring the batch.

func closeSchedAccount(t *testing.T, repo *account.Repository, acct *account.Account) {
	t.Helper()
	acct.Close(types.NewDate(2000, time.February, 1))
	if err := repo.Update(acct); err != nil {
		t.Fatalf("failed to close test account: %v", err)
	}
}

func assertSchedClosed(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a ClosedAccountError, got nil")
	}
	var cerr *ClosedAccountError
	if !errors.As(err, &cerr) {
		t.Fatalf("expected *ClosedAccountError, got %T: %v", err, err)
	}
}

func TestService_Create_RejectsClosedAccount(t *testing.T) {
	svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
	acct := createTestAccountForScheduled(t, accountRepo, "Checking")
	closeSchedAccount(t, accountRepo, acct)

	st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), types.MustNewMoney("-50.00"))
	assertSchedClosed(t, svc.Create(st))
}

func TestService_Create_RejectsClosedTransferDestination(t *testing.T) {
	svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
	src := createTestAccountForScheduled(t, accountRepo, "Checking")
	dest := createTestAccountForScheduled(t, accountRepo, "Savings")
	closeSchedAccount(t, accountRepo, dest)

	st := NewTransactionWithAmount(src.ID, FrequencyMonthly, types.Today(), types.MustNewMoney("-50.00"))
	st.SetTransfer(dest.ID)
	assertSchedClosed(t, svc.Create(st))
}

func TestService_Post_RejectedOnClosedAccount_StaysDue(t *testing.T) {
	svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
	acct := createTestAccountForScheduled(t, accountRepo, "Checking")

	st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), types.MustNewMoney("-50.00"))
	if err := svc.Create(st); err != nil {
		t.Fatalf("Create (open) error = %v", err)
	}
	originalNext := st.NextDate

	closeSchedAccount(t, accountRepo, acct)

	_, err := svc.Post(st.ID, nil)
	assertSchedClosed(t, err)

	// The schedule must remain due (not advanced).
	after, gerr := svc.GetByID(st.ID)
	if gerr != nil {
		t.Fatalf("GetByID() error = %v", gerr)
	}
	if !after.NextDate.Equal(originalNext) {
		t.Errorf("schedule should stay due after a refused post: next %s, want %s", after.NextDate, originalNext)
	}
}

func TestService_AutoPost_SkipsClosedAccount(t *testing.T) {
	svc, accountRepo, payeeRepo, categoryRepo := createTestScheduledTransactionService(t)
	acct := createTestAccountForScheduled(t, accountRepo, "Checking")

	py := payee.NewPayee("Landlord")
	if err := payeeRepo.Create(py); err != nil {
		t.Fatalf("Failed to create payee: %v", err)
	}
	cat := category.NewCategory("Housing", category.TypeExpense)
	if err := categoryRepo.Create(cat); err != nil {
		t.Fatalf("Failed to create category: %v", err)
	}

	st := NewTransactionFull(acct.ID, FrequencyMonthly, types.Today(), types.MustNewMoney("-1500.00"),
		py.ID, cat.ID, "Monthly rent")
	st.SetAutoPost(true)
	st.SetPostLeadDays(0)
	if err := svc.Create(st); err != nil {
		t.Fatalf("Create (open) error = %v", err)
	}
	originalNext := st.NextDate

	closeSchedAccount(t, accountRepo, acct)

	summary, err := svc.AutoPost()
	if err != nil {
		t.Fatalf("AutoPost() should skip, not error, got %v", err)
	}
	if summary.PostedCount != 0 {
		t.Errorf("expected 0 posted for a closed-account schedule, got %d", summary.PostedCount)
	}
	if summary.SkippedCount != 1 {
		t.Errorf("expected 1 skipped, got %d", summary.SkippedCount)
	}

	after, gerr := svc.GetByID(st.ID)
	if gerr != nil {
		t.Fatalf("GetByID() error = %v", gerr)
	}
	if !after.NextDate.Equal(originalNext) {
		t.Errorf("skipped schedule should not advance: next %s, want %s", after.NextDate, originalNext)
	}
}

func TestService_ListReferencing(t *testing.T) {
	svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
	target := createTestAccountForScheduled(t, accountRepo, "Checking")
	other := createTestAccountForScheduled(t, accountRepo, "Savings")

	// Schedule with target as its source account.
	src := NewTransactionWithAmount(target.ID, FrequencyMonthly, types.Today(), types.MustNewMoney("-50.00"))
	if err := svc.Create(src); err != nil {
		t.Fatalf("Create source schedule error = %v", err)
	}
	// Transfer schedule with target as its destination.
	xfer := NewTransactionWithAmount(other.ID, FrequencyMonthly, types.Today(), types.MustNewMoney("-25.00"))
	xfer.SetTransfer(target.ID)
	if err := svc.Create(xfer); err != nil {
		t.Fatalf("Create transfer schedule error = %v", err)
	}
	// An unrelated schedule that must NOT be returned.
	unrelated := NewTransactionWithAmount(other.ID, FrequencyMonthly, types.Today(), types.MustNewMoney("-10.00"))
	if err := svc.Create(unrelated); err != nil {
		t.Fatalf("Create unrelated schedule error = %v", err)
	}

	refs, err := svc.ListReferencing(target.ID)
	if err != nil {
		t.Fatalf("ListReferencing() error = %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 schedules referencing the account (source + transfer dest), got %d", len(refs))
	}
}
