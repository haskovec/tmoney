package scheduled

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/types"
)

// An investment↔investment transfer keeps both legs in investment_transactions,
// which have nowhere to store a category. A schedule carrying that combination
// is not mislabelled — it is unpostable, and in a batch its refusal used to
// abort every other schedule due that day.
//
// Creation now refuses it, HealTransferCategories clears the rows older binaries
// let through, and auto-post skips any that appear in between (an account's type
// can be changed after the heal has already run).

// newInvToInvScheduleWithCategory builds the poisoned row directly through the
// repository, which is the only way to get one now that the service layer
// refuses it — and is exactly how an older binary left it behind.
func newInvToInvScheduleWithCategory(t *testing.T, env *transferTestEnv) (*Transaction, *account.Account, *account.Account) {
	t.Helper()
	from := env.account(t, "Rollover IRA", account.TypeInvestment)
	to := env.account(t, "Roth IRA", account.TypeInvestment)

	cat := category.NewCategory("Retirement", category.TypeExpense)
	if err := env.categoryRepo.Create(cat); err != nil {
		t.Fatalf("create category: %v", err)
	}

	st := newTransferSchedule(from.ID, to.ID, "500.00")
	st.SetCategory(cat.ID)
	if err := env.svc.repo.Create(st); err != nil {
		t.Fatalf("create poisoned schedule: %v", err)
	}
	return st, from, to
}

func TestHealTransferCategories_ClearsAnUnusableCategory(t *testing.T) {
	env := newTransferTestEnv(t)
	st, _, _ := newInvToInvScheduleWithCategory(t, env)

	healed, err := env.svc.HealTransferCategories()
	if err != nil {
		t.Fatalf("HealTransferCategories(): %v", err)
	}
	if healed != 1 {
		t.Errorf("healed %d rows, want 1", healed)
	}

	after, err := env.svc.GetByID(st.ID)
	if err != nil {
		t.Fatalf("GetByID(): %v", err)
	}
	if after.HasCategory() {
		t.Error("the unusable category survived the heal, so the schedule still cannot post")
	}

	// And the schedule now posts, which is the whole point.
	if _, perr := env.svc.Post(st.ID, nil); perr != nil {
		t.Errorf("Post() after heal: %v", perr)
	}
}

func TestHealTransferCategories_LeavesUsableCategoriesAlone(t *testing.T) {
	env := newTransferTestEnv(t)
	checking := env.account(t, "Checking", account.TypeChecking)
	savings := env.account(t, "Savings", account.TypeSavings)

	cat := category.NewCategory("Savings Plan", category.TypeExpense)
	if err := env.categoryRepo.Create(cat); err != nil {
		t.Fatalf("create category: %v", err)
	}

	st := newTransferSchedule(checking.ID, savings.ID, "200.00")
	st.SetCategory(cat.ID)
	if err := env.svc.Create(st); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	healed, err := env.svc.HealTransferCategories()
	if err != nil {
		t.Fatalf("HealTransferCategories(): %v", err)
	}
	if healed != 0 {
		t.Errorf("healed %d rows, want 0 — a bank-to-bank pair stores a category fine", healed)
	}

	after, err := env.svc.GetByID(st.ID)
	if err != nil {
		t.Fatalf("GetByID(): %v", err)
	}
	if !after.HasCategory() {
		t.Error("the heal cleared a category the pair can store")
	}
}

// TestAutoPost_UnpostableCategoryDoesNotAbortTheBatch is the severe half. One
// schedule that can never post must not stop every other schedule from posting
// that day — which is what happened, because the transfer owner's refusal is
// neither a closed-account error nor a loan error, so it fell through to the
// hard-error return that discards the whole summary.
func TestAutoPost_UnpostableCategoryDoesNotAbortTheBatch(t *testing.T) {
	env := newTransferTestEnv(t)
	poisoned, _, _ := newInvToInvScheduleWithCategory(t, env)
	poisoned.SetAutoPost(true)
	poisoned.SetPostLeadDays(0)
	if err := env.svc.repo.Update(poisoned); err != nil {
		t.Fatalf("arm the poisoned schedule: %v", err)
	}

	// A perfectly ordinary schedule that must still post.
	checking := env.account(t, "Checking", account.TypeChecking)
	healthy := NewTransactionWithAmount(checking.ID, FrequencyMonthly,
		types.Today(), types.MustNewMoney("-75.00"))
	healthy.SetOccurrences(1)
	healthy.SetAutoPost(true)
	healthy.SetPostLeadDays(0)
	if err := env.svc.Create(healthy); err != nil {
		t.Fatalf("Create(healthy): %v", err)
	}

	summary, err := env.svc.AutoPost()
	if err != nil {
		t.Fatalf("AutoPost() aborted the batch: %v", err)
	}
	if summary.PostedCount != 1 {
		t.Errorf("PostedCount = %d, want 1 — the healthy schedule must still post", summary.PostedCount)
	}
	if summary.SkippedCount != 1 {
		t.Errorf("SkippedCount = %d, want 1", summary.SkippedCount)
	}

	var skipped *AutoPostResult
	for i := range summary.Results {
		if summary.Results[i].ScheduledTransactionID == poisoned.ID {
			skipped = &summary.Results[i]
		}
	}
	if skipped == nil {
		t.Fatal("the unpostable schedule was not reported at all")
	}
	if skipped.SkipReason == "" {
		t.Error("the skip carries no reason")
	}

	// The healthy schedule really did commit.
	after, gerr := env.svc.GetByID(healthy.ID)
	if gerr != nil {
		t.Fatalf("GetByID(healthy): %v", gerr)
	}
	if !after.IsCompleted() {
		t.Error("the healthy schedule did not advance; the batch was rolled back after all")
	}

	// And the unpostable one was left untouched, not half-processed.
	poisonedAfter, gerr := env.svc.GetByID(poisoned.ID)
	if gerr != nil {
		t.Fatalf("GetByID(poisoned): %v", gerr)
	}
	if !poisonedAfter.NextDate.Equal(poisoned.NextDate) {
		t.Errorf("the skipped schedule advanced to %s; a skip must leave it due", poisonedAfter.NextDate)
	}
}
