package undo_test

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// TestAutoPostCommand_UndoInvestmentToInvestmentTransfer closes the loop on the
// re-post bug.
//
// An investment-to-investment occurrence puts BOTH legs in
// investment_transactions, so it contributes no regular-ledger row. AutoPost used
// to admit a candidate to summary.Results only when it had such a row, so this
// candidate never appeared there — and undo, which walks Results, silently
// deleted nothing and restored nothing while reporting success.
//
// AutoPost now admits a candidate on its occurrence count, so the entry exists.
// This test asserts the rest of the chain was already correct: undo deletes the
// pair through the transfer owner (transaction.Service.Delete refuses a single
// leg) and restores the schedule.
func TestAutoPostCommand_UndoInvestmentToInvestmentTransfer(t *testing.T) {
	env := createScheduledTestEnv(t)

	from := account.NewAccount("Rollover IRA", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := env.accountRepo.Create(from); err != nil {
		t.Fatalf("create Rollover IRA: %v", err)
	}
	to := account.NewAccount("Roth IRA", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := env.accountRepo.Create(to); err != nil {
		t.Fatalf("create Roth IRA: %v", err)
	}

	st := scheduled.NewTransactionWithAmount(from.ID, scheduled.FrequencyMonthly,
		types.Today(), types.MustNewMoney("-500.00"))
	st.SetTransfer(to.ID)
	st.SetAutoPost(true)
	st.SetPostLeadDays(0)
	if err := env.scheduledSvc.Create(st); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	beforeNext := st.NextDate

	summary, err := env.scheduledSvc.AutoPost()
	if err != nil {
		t.Fatalf("AutoPost(): %v", err)
	}
	if summary.PostedCount != 1 {
		t.Fatalf("PostedCount = %d, want 1", summary.PostedCount)
	}
	if len(summary.Results) != 1 {
		t.Fatalf("summary.Results = %d, want 1 — undo has nothing to walk", len(summary.Results))
	}

	invRepo := investment.NewRepository(env.database)
	assertRows := func(when string, want int) {
		t.Helper()
		for _, acct := range []struct {
			name string
			id   types.ID
		}{{"Rollover IRA", from.ID}, {"Roth IRA", to.ID}} {
			rows, rerr := invRepo.ListByAccount(acct.id, investment.TransactionFilter{})
			if rerr != nil {
				t.Fatalf("ListByAccount(%s) %s: %v", acct.name, when, rerr)
			}
			if len(rows) != want {
				t.Errorf("%s holds %d investment rows %s, want %d", acct.name, len(rows), when, want)
			}
		}
	}
	assertRows("after auto-post", 1)

	cmd := undo.NewAutoPostCommand(env.txnSvc, env.transferSvc, env.scheduledSvc, summary)
	if uerr := cmd.Undo(); uerr != nil {
		t.Fatalf("Undo(): %v", uerr)
	}

	// Both legs are gone — deleting one alone is refused by transaction.Service
	// and would orphan the other.
	assertRows("after undo", 0)

	// And the schedule is back where it started.
	after, gerr := env.scheduledSvc.GetByID(st.ID)
	if gerr != nil {
		t.Fatalf("GetByID(): %v", gerr)
	}
	if !after.NextDate.Equal(beforeNext) {
		t.Errorf("next_date = %s after undo, want the pre-post %s", after.NextDate, beforeNext)
	}
}

// TestPostScheduledTransferCommand_UndoInvestmentToInvestment is the MANUAL-door
// twin of the test above, and it covers a bug the auto-post fix did not reach.
//
// PostScheduledTransferCommand used to learn the posted transfer's id by reading
// it off the regular-ledger leg. An investment-to-investment occurrence has no
// such leg, so the id stayed zero, Undo's guard skipped the delete, and the
// schedule was rewound anyway. The undo reported success while leaving both
// posted legs in the ledger — so the next post duplicated the transfer. The
// command's own doc comment claimed Undo "addresses the transfer, not a leg, so
// it works for every shape"; the code took the id from a leg.
func TestPostScheduledTransferCommand_UndoInvestmentToInvestment(t *testing.T) {
	env := createScheduledTestEnv(t)

	from := account.NewAccount("Rollover IRA", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := env.accountRepo.Create(from); err != nil {
		t.Fatalf("create Rollover IRA: %v", err)
	}
	to := account.NewAccount("Roth IRA", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := env.accountRepo.Create(to); err != nil {
		t.Fatalf("create Roth IRA: %v", err)
	}

	st := scheduled.NewTransactionWithAmount(from.ID, scheduled.FrequencyMonthly,
		types.Today(), types.MustNewMoney("-500.00"))
	st.SetTransfer(to.ID)
	if err := env.scheduledSvc.Create(st); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	beforeNext := st.NextDate

	invRepo := investment.NewRepository(env.database)
	countRows := func(when string) int {
		t.Helper()
		total := 0
		for _, id := range []types.ID{from.ID, to.ID} {
			rows, rerr := invRepo.ListByAccount(id, investment.TransactionFilter{})
			if rerr != nil {
				t.Fatalf("ListByAccount %s: %v", when, rerr)
			}
			total += len(rows)
		}
		return total
	}

	cmd := undo.NewPostScheduledTransferCommand(
		env.scheduledSvc, env.transferSvc, st.ID, types.Today(),
		types.MustNewMoney("500.00"), "one-off memo", false, types.NullableID{},
	)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(): %v", err)
	}
	if n := countRows("after post"); n != 2 {
		t.Fatalf("%d investment rows after the post, want 2 (both legs)", n)
	}

	if err := cmd.Undo(); err != nil {
		t.Fatalf("Undo(): %v", err)
	}

	// The whole point: BOTH legs must be gone. Deleting one alone is refused by
	// transaction.Service and would orphan the other.
	if n := countRows("after undo"); n != 0 {
		t.Errorf("%d investment rows survived the undo, want 0 — the undo reported success "+
			"but deleted nothing, so the next post duplicates the transfer", n)
	}

	after, gerr := env.scheduledSvc.GetByID(st.ID)
	if gerr != nil {
		t.Fatalf("GetByID(): %v", gerr)
	}
	if !after.NextDate.Equal(beforeNext) {
		t.Errorf("next_date = %s after undo, want the pre-post %s", after.NextDate, beforeNext)
	}
}
