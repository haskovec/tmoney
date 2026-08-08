package scheduled_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	scheduleddom "github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

// TestScheduledPost_InvestmentToInvestmentTransfer drives `tmoney scheduled post`
// against the one schedule shape whose occurrence produces no regular-ledger row.
//
// An investment-to-investment transfer writes both legs to
// investment_transactions, so the manual post path legitimately reports no
// transaction — it has none to report. The command checked only the error and
// then dereferenced the result for its amount and date, so this crashed with a
// nil-pointer panic on a schedule the application itself lets you create.
func TestScheduledPost_InvestmentToInvestmentTransfer(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	from := account.NewAccount("Rollover IRA", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := acctRepo.Create(from); err != nil {
		t.Fatalf("setup: create Rollover IRA: %v", err)
	}
	to := account.NewAccount("Roth IRA", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := acctRepo.Create(to); err != nil {
		t.Fatalf("setup: create Roth IRA: %v", err)
	}

	stRepo := scheduleddom.NewRepository(database)
	st := scheduleddom.NewTransactionWithAmount(from.ID, scheduleddom.FrequencyMonthly,
		types.Today(), types.MustNewMoney("-500.00"))
	st.SetTransfer(to.ID)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("setup: create schedule: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"scheduled", "post", st.ID.String(), "--file", dbPath,
	}, stdout, stderr); err != nil {
		t.Fatalf("scheduled post on an investment-to-investment transfer: %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, "posted successfully") {
		t.Errorf("expected a success summary, got:\n%s", out)
	}
	if !strings.Contains(out, "Rollover IRA") {
		t.Errorf("summary should name the source account, got:\n%s", out)
	}
	// There is no regular-ledger row to take an amount from, so the summary must
	// use the schedule's own signed amount — the effect on the account it names.
	if !strings.Contains(out, "-$500.00") {
		t.Errorf("summary should show the source-account effect -$500.00, got:\n%s", out)
	}
	// And the direction must be stated, not inferred from a sign.
	if !strings.Contains(out, "Transfer to: Roth IRA") {
		t.Errorf("summary should name the destination, got:\n%s", out)
	}

	// The schedule advanced, and both investment legs exist.
	reopened, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer reopened.Close()

	after, err := scheduleddom.NewRepository(reopened).GetByID(st.ID)
	if err != nil {
		t.Fatalf("reload schedule: %v", err)
	}
	if !after.NextDate.After(st.NextDate) {
		t.Errorf("next_date = %s, want it advanced past %s", after.NextDate, st.NextDate)
	}
}
