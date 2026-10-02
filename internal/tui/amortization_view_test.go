package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// loadAmort drives the async loader and feeds the result through Update so the
// projection table is built, exactly as the message loop does.
func loadAmort(t *testing.T, env *loanPreviewEnv, accountID types.ID) {
	t.Helper()
	msg := env.app.amortization.load(env.app.amortizationDeps(), accountID)()
	if _, ok := msg.(errMsg); ok {
		t.Fatalf("amortization.load returned errMsg: %v", msg.(errMsg).err)
	}
	model, _ := env.app.Update(msg)
	env.app = model.(*App)
	env.app.currentView = ViewAmortization
}

// renderAmort renders the view through its table entry, as the app does, so
// the entry's closure that passes styles and the screen size is under test too.
func renderAmort(t *testing.T, app *App) string {
	t.Helper()
	e, ok := viewFor(ViewAmortization)
	if !ok {
		t.Fatal("no view table entry for ViewAmortization")
	}
	return e.render(app)
}

func TestAmortizationView_LoadsProjection(t *testing.T) {
	env := newLoanPreviewEnv(t, "380000", "6.5", "2401.86", types.NewDate(2026, time.August, 1))
	loadAmort(t, env, env.loan.ID)

	d := env.app.amortization.data
	if d == nil {
		t.Fatal("amortization.data is nil")
	}
	if !d.hasSchedule {
		t.Fatal("hasSchedule = false; want true")
	}
	if want := types.MustNewMoney("2401.86"); !d.piPayment.Equal(want) {
		t.Errorf("piPayment = %s; want %s", d.piPayment, want)
	}
	if !d.escrowTotal.IsZero() {
		t.Errorf("escrowTotal = %s; want 0", d.escrowTotal)
	}
	if d.stats.PaymentsRemaining != 360 {
		t.Errorf("PaymentsRemaining = %d; want 360", d.stats.PaymentsRemaining)
	}
	if want := types.MustNewMoney("484667.97"); !d.stats.TotalInterestRemaining.Equal(want) {
		t.Errorf("TotalInterestRemaining = %s; want %s", d.stats.TotalInterestRemaining, want)
	}
	if d.stats.Truncated {
		t.Error("stats.Truncated = true; want false for a 360-month loan")
	}

	row0 := d.projection.Rows[0]
	if want := types.MustNewMoney("2058.33"); !row0.Interest.Equal(want) {
		t.Errorf("row0.Interest = %s; want %s", row0.Interest, want)
	}
	if want := types.MustNewMoney("343.53"); !row0.Principal.Equal(want) {
		t.Errorf("row0.Principal = %s; want %s", row0.Principal, want)
	}
	if want := types.MustNewMoney("379656.47"); !row0.BalanceAfter.Equal(want) {
		t.Errorf("row0.BalanceAfter = %s; want %s", row0.BalanceAfter, want)
	}

	if env.app.amortization.table == nil {
		t.Fatal("amortization.table was not built")
	}

	out := renderAmort(t, env.app)
	for _, want := range []string{"AMORTIZATION", "MORTGAGE", "2056-07-01"} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q\n%s", want, out)
		}
	}
}

func TestAmortizationView_NoSchedule(t *testing.T) {
	env := newLoanPreviewEnv(t, "380000", "6.5", "2401.86", types.NewDate(2026, time.August, 1))

	// A second loan account with an APR but no payment schedule.
	carLoan := account.NewAccount("Car Loan", account.TypeLoan, "USD",
		types.MustNewMoney("-20000"), types.NewDate(2024, time.January, 1))
	carLoan.SetInterestRate(types.MustNewMoney("5.9"))
	if err := env.app.services.Account.Create(carLoan); err != nil {
		t.Fatalf("create car loan: %v", err)
	}

	loadAmort(t, env, carLoan.ID)

	d := env.app.amortization.data
	if d.hasSchedule {
		t.Error("hasSchedule = true; want false (no schedule targets this loan)")
	}
	if want := types.MustNewMoney("20000"); !d.owed.Equal(want) {
		t.Errorf("owed = %s; want %s", d.owed, want)
	}
	if env.app.amortization.table != nil {
		t.Error("amortization.table should be nil with no projection")
	}

	out := renderAmort(t, env.app)
	if !strings.Contains(out, "No loan payment schedule") {
		t.Errorf("render missing no-schedule hint\n%s", out)
	}
}

func TestAmortizationView_ClampFinalPayment(t *testing.T) {
	// owed 4000, P&I 2401.86 → two payments; the second is clamped to the
	// remaining balance (principal would otherwise exceed it).
	env := newLoanPreviewEnv(t, "4000", "6.5", "2401.86", types.NewDate(2026, time.August, 1))
	loadAmort(t, env, env.loan.ID)

	d := env.app.amortization.data
	if d.stats.PaymentsRemaining != 2 {
		t.Fatalf("PaymentsRemaining = %d; want 2", d.stats.PaymentsRemaining)
	}
	last := d.projection.Rows[len(d.projection.Rows)-1]
	if !last.Final {
		t.Error("final row Final = false; want true (clamped)")
	}
	if !last.BalanceAfter.IsZero() {
		t.Errorf("final BalanceAfter = %s; want 0.00", last.BalanceAfter)
	}
}

func TestAmortizationView_Truncated(t *testing.T) {
	// P&I barely above month-one interest (~2058.33) → tiny principal, projection
	// runs past the 1,200-row cap and is flagged truncated.
	env := newLoanPreviewEnv(t, "380000", "6.5", "2058.34", types.NewDate(2026, time.August, 1))
	loadAmort(t, env, env.loan.ID)

	d := env.app.amortization.data
	if !d.stats.Truncated {
		t.Fatalf("stats.Truncated = false; want true")
	}
	if !d.stats.PayoffDate.IsZero() {
		t.Errorf("truncated PayoffDate = %s; want zero date", d.stats.PayoffDate)
	}

	out := renderAmort(t, env.app)
	if !strings.Contains(out, "100y+") {
		t.Errorf("render missing 100y+ for truncated projection\n%s", out)
	}
}

func TestAmortizationView_MissingAPR(t *testing.T) {
	env := newLoanPreviewEnv(t, "380000", "6.5", "2401.86", types.NewDate(2026, time.August, 1))

	// Clear the loan's APR after the schedule exists — a loan-shaped schedule
	// with no computable rate.
	loanAcct, err := env.app.services.Account.GetByID(env.loan.ID)
	if err != nil {
		t.Fatalf("get loan: %v", err)
	}
	loanAcct.ClearInterestRate()
	if err := env.app.services.Account.Update(loanAcct); err != nil {
		t.Fatalf("clear APR: %v", err)
	}

	loadAmort(t, env, env.loan.ID)

	d := env.app.amortization.data
	if !d.hasSchedule {
		t.Error("hasSchedule = false; want true (schedule still loan-shaped)")
	}
	if d.aprValid {
		t.Error("aprValid = true; want false after clearing the rate")
	}
	if env.app.amortization.table != nil {
		t.Error("amortization.table should be nil without an APR")
	}

	out := renderAmort(t, env.app)
	if !strings.Contains(out, "no interest rate set") {
		t.Errorf("render missing missing-APR hint\n%s", out)
	}
}

func TestRegisterKey_A_OpensAmortizationForLoan(t *testing.T) {
	env := newLoanPreviewEnv(t, "380000", "6.5", "2401.86", types.NewDate(2026, time.August, 1))

	env.app.currentView = ViewRegister
	env.app.register.data = &registerData{account: env.loan}
	env.app.register.table = widget.NewTable(registerColumns(false))
	env.app.sidebar.SetFocused(false)
	env.app.register.table.SetFocused(true)

	model, cmd := env.app.handleRegisterKeys(tea.KeyPressMsg{Code: 'a', Text: "a"})
	env.app = model.(*App)
	if env.app.currentView != ViewAmortization {
		t.Fatalf("currentView = %v; want ViewAmortization", env.app.currentView)
	}
	if cmd == nil {
		t.Fatal("expected a load command from pressing 'a' on a loan register")
	}
	// Drive the returned load command through Update to confirm it wires up.
	model, _ = env.app.Update(cmd())
	env.app = model.(*App)
	if env.app.amortization.data == nil || !env.app.amortization.data.hasSchedule {
		t.Error("amortization data not loaded after opening the view")
	}
}

func TestRegisterKey_A_NoOpForNonLoan(t *testing.T) {
	env := newLoanPreviewEnv(t, "380000", "6.5", "2401.86", types.NewDate(2026, time.August, 1))

	env.app.currentView = ViewRegister
	env.app.register.data = &registerData{account: env.funding} // checking, not a loan
	env.app.register.table = widget.NewTable(registerColumns(false))
	env.app.sidebar.SetFocused(false)
	env.app.register.table.SetFocused(true)

	model, cmd := env.app.handleRegisterKeys(tea.KeyPressMsg{Code: 'a', Text: "a"})
	env.app = model.(*App)
	if env.app.currentView != ViewRegister {
		t.Errorf("currentView = %v; want ViewRegister (no-op on non-loan)", env.app.currentView)
	}
	if cmd != nil {
		t.Error("expected no command for 'a' on a non-loan register")
	}
}

// The view table's key closure hands the view the key bindings and the
// screen height: ↓ moves one row, and PgDn moves a page of height−10 rows.
func TestAmortizationView_KeysReachTheTable(t *testing.T) {
	env := newLoanPreviewEnv(t, "380000", "6.5", "2401.86", types.NewDate(2026, time.August, 1))
	loadAmort(t, env, env.loan.ID)
	tbl := env.app.amortization.table
	if tbl == nil || tbl.RowCount() < 100 {
		t.Fatal("want a projection table of more than 100 rows")
	}
	if env.app.height <= 10 {
		t.Fatalf("test premise: a page is height−10 rows, so the screen must be taller than 10; got %d", env.app.height)
	}

	press(env.app, tea.KeyPressMsg{Code: tea.KeyDown})
	if got := tbl.Cursor(); got != 1 {
		t.Fatalf("cursor after ↓ = %d, want 1", got)
	}
	press(env.app, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if want := 1 + env.app.height - 10; tbl.Cursor() != want {
		t.Errorf("cursor after PgDn = %d, want %d (a page is height−10 rows)", tbl.Cursor(), want)
	}
}

// The render closure hands the view the screen size: the drill-in fits the
// screen it is given, in both directions.
func TestAmortizationView_RenderFitsTheScreen(t *testing.T) {
	env := newLoanPreviewEnv(t, "380000", "6.5", "2401.86", types.NewDate(2026, time.August, 1))
	loadAmort(t, env, env.loan.ID)

	lines := strings.Split(renderAmort(t, env.app), "\n")
	if len(lines) > env.app.height {
		t.Errorf("render is %d lines on a %d-line screen", len(lines), env.app.height)
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > env.app.width {
			t.Errorf("line %d is %d cells wide on a %d-cell screen", i, w, env.app.width)
			break
		}
	}
}
