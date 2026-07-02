package loan

import (
	"errors"
	"testing"

	"github.com/alpacahq/alpacadecimal"

	"github.com/haskovec/tmoney/internal/types"
)

// m is a terse helper for building a Money from a decimal string in tests.
func m(s string) types.Money { return types.MustNewMoney(s) }

// moneyEq compares two Money values by numeric value so "0" and "0.00" match.
func moneyEq(t *testing.T, got, want types.Money, ctx string) {
	t.Helper()
	if got.Cmp(want) != 0 {
		t.Errorf("%s: got %s, want %s", ctx, got, want)
	}
}

func TestMonthlyRate(t *testing.T) {
	// 6.5% APR → 0.065 / 12 at the library's full (16-place) division precision.
	// Pinned exactly: the per-month interest rounding depends on this value.
	want := alpacadecimal.RequireFromString("0.0054166666666667")
	if got := MonthlyRate(m("6.5")); got.Cmp(want) != 0 {
		t.Errorf("MonthlyRate(6.5) = %s, want %s", got, want)
	}
	if got := MonthlyRate(m("0")); !got.IsZero() {
		t.Errorf("MonthlyRate(0) = %s, want 0", got)
	}
	// 12% → 0.01 exactly.
	if got := MonthlyRate(m("12")); got.Cmp(alpacadecimal.RequireFromString("0.01")) != 0 {
		t.Errorf("MonthlyRate(12) = %s, want 0.01", got)
	}
}

func TestPayment(t *testing.T) {
	tests := []struct {
		name       string
		principal  string
		apr        string
		termMonths int
		want       string // expected payment; empty means expect an error
	}{
		// Amortizing loans — values computed independently (Python, ROUND_HALF_UP).
		{"30yr mortgage 380k @ 6.5%", "380000", "6.5", 360, "2401.86"},
		{"mid-life balance 312450.22 @ 6.5%", "312450.22", "6.5", 360, "1974.90"},
		{"car loan 32k @ 5.9% / 60mo", "32000", "5.9", 60, "617.16"},
		// 0% promotional loans: ceil-to-cent(P/n).
		{"0% 24000 / 36mo", "24000", "0", 36, "666.67"}, // 666.666… → ceil 666.67
		{"0% 10000 / 7mo", "10000", "0", 7, "1428.58"},  // 1428.571… → ceil 1428.58
		{"0% exact division", "1200", "0", 12, "100"},   // 100.00 exactly, no ceil bump
		{"zero principal", "0", "5", 12, "0"},
		// Invalid inputs.
		{"zero term", "1000", "5", 0, ""},
		{"negative term", "1000", "5", -12, ""},
		{"negative principal", "-1000", "5", 12, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Payment(m(tc.principal), m(tc.apr), tc.termMonths)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("Payment(%s,%s,%d) = %s, want error", tc.principal, tc.apr, tc.termMonths, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Payment(%s,%s,%d) unexpected error: %v", tc.principal, tc.apr, tc.termMonths, err)
			}
			moneyEq(t, got, m(tc.want), "payment")
		})
	}
}

// TestPaymentZeroPercentNoStrayCent verifies the 0% ceil rule pays the balance
// within the term (M×n ≥ P) rather than leaving a stray (n+1)th one-cent payment.
func TestPaymentZeroPercentNoStrayCent(t *testing.T) {
	pay, err := Payment(m("10000"), m("0"), 7)
	if err != nil {
		t.Fatal(err)
	}
	proj, err := Project(m("10000"), m("0"), pay, types.ZeroMoney, types.MustParseDate("2026-08-01"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(proj.Rows) != 7 {
		t.Errorf("0%% loan should pay off in exactly the term (7), got %d rows", len(proj.Rows))
	}
	moneyEq(t, proj.Rows[len(proj.Rows)-1].BalanceAfter, types.ZeroMoney, "final balance")
}

func TestSplitPayment(t *testing.T) {
	tests := []struct {
		name                        string
		owed, apr, pi               string
		wantInterest, wantPrincipal string
		wantFinal                   bool
		wantErr                     bool
	}{
		{
			name: "first payment on 380k @ 6.5%",
			owed: "380000", apr: "6.5", pi: "2401.86",
			wantInterest: "2058.33", wantPrincipal: "343.53", wantFinal: false,
		},
		{
			name: "final payment clamps principal to owed (0% loan)",
			owed: "100", apr: "0", pi: "400",
			wantInterest: "0", wantPrincipal: "100", wantFinal: true,
		},
		{
			name: "tiny residual clamps, interest rounds to zero",
			owed: "0.07", apr: "12", pi: "340",
			wantInterest: "0", wantPrincipal: "0.07", wantFinal: true,
		},
		{
			name: "exact natural payoff is not flagged final",
			owed: "100", apr: "0", pi: "100",
			wantInterest: "0", wantPrincipal: "100", wantFinal: false,
		},
		// Exact half-cent interest ties must round up per the spec. These fail if
		// the rate is pre-rounded (owed × round(APR/1200) understates the tie):
		//   owed × APR / 1200 lands exactly on X.XX5, e.g. 1020×1.30/1200 = 1.105.
		{
			name: "half-cent interest tie rounds up (1020 @ 1.30%)",
			owed: "1020.00", apr: "1.30", pi: "500",
			wantInterest: "1.11", wantPrincipal: "498.89", wantFinal: false,
		},
		{
			name: "half-cent interest tie rounds up (120 @ 1.15%)",
			owed: "120.00", apr: "1.15", pi: "50",
			wantInterest: "0.12", wantPrincipal: "49.88", wantFinal: false,
		},
		{
			name: "half-cent interest tie rounds up (6 @ 1.00%)",
			owed: "6.00", apr: "1.00", pi: "3",
			wantInterest: "0.01", wantPrincipal: "2.99", wantFinal: false,
		},
		{
			name: "negative amortization: payment below interest",
			owed: "1000", apr: "12", pi: "5",
			wantErr: true,
		},
		{
			name: "negative amortization: payment equals interest",
			owed: "1000", apr: "12", pi: "10",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			interest, principal, final, err := SplitPayment(m(tc.owed), m(tc.apr), m(tc.pi))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got interest=%s principal=%s", interest, principal)
				}
				if !errors.Is(err, ErrNegativeAmortization) {
					t.Fatalf("expected ErrNegativeAmortization, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			moneyEq(t, interest, m(tc.wantInterest), "interest")
			moneyEq(t, principal, m(tc.wantPrincipal), "principal")
			if final != tc.wantFinal {
				t.Errorf("final = %v, want %v", final, tc.wantFinal)
			}
			// Invariant: interest + principal never exceeds the P&I payment.
			if interest.Add(principal).Cmp(m(tc.pi)) > 0 {
				t.Errorf("interest+principal %s exceeds pi %s", interest.Add(principal), tc.pi)
			}
		})
	}
}

func TestProjectFullAmortization(t *testing.T) {
	next := types.MustParseDate("2026-08-01")

	t.Run("30yr mortgage ends exactly at zero", func(t *testing.T) {
		proj, err := Project(m("380000"), m("6.5"), m("2401.86"), types.ZeroMoney, next, 1)
		if err != nil {
			t.Fatal(err)
		}
		if proj.Truncated {
			t.Error("should not be truncated")
		}
		if len(proj.Rows) != 360 {
			t.Fatalf("expected 360 rows, got %d", len(proj.Rows))
		}
		// First three rows against independently computed values.
		want := []struct{ interest, principal, balance string }{
			{"2058.33", "343.53", "379656.47"},
			{"2056.47", "345.39", "379311.08"},
			{"2054.60", "347.26", "378963.82"},
		}
		for i, w := range want {
			moneyEq(t, proj.Rows[i].Interest, m(w.interest), "interest row")
			moneyEq(t, proj.Rows[i].Principal, m(w.principal), "principal row")
			moneyEq(t, proj.Rows[i].BalanceAfter, m(w.balance), "balance row")
		}
		// Penny accumulation lands exactly on zero.
		moneyEq(t, proj.Rows[359].BalanceAfter, types.ZeroMoney, "final balance")

		stats := RemainingStats(proj)
		if stats.PaymentsRemaining != 360 {
			t.Errorf("payments remaining = %d, want 360", stats.PaymentsRemaining)
		}
		moneyEq(t, stats.TotalInterestRemaining, m("484667.97"), "total interest")
		// 360 payments starting 2026-08-01 → last is 2056-07-01.
		if !stats.PayoffDate.Equal(types.MustParseDate("2056-07-01")) {
			t.Errorf("payoff date = %s, want 2056-07-01", stats.PayoffDate)
		}
	})

	t.Run("under-amortizing payment needs an extra final payment", func(t *testing.T) {
		// Payment(32k,5.9,60)=617.16 slightly under-amortizes, so a small 61st
		// payment clears the balance.
		proj, err := Project(m("32000"), m("5.9"), m("617.16"), types.ZeroMoney, next, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(proj.Rows) != 61 {
			t.Fatalf("expected 61 rows, got %d", len(proj.Rows))
		}
		moneyEq(t, proj.Rows[60].BalanceAfter, types.ZeroMoney, "final balance")
		moneyEq(t, RemainingStats(proj).TotalInterestRemaining, m("5029.74"), "total interest")
		if !proj.Rows[60].Final {
			t.Error("last row should be the clamped final payment")
		}
	})

	t.Run("small loan with zero-interest final row", func(t *testing.T) {
		proj, err := Project(m("1000"), m("12"), m("340"), types.ZeroMoney, next, 1)
		if err != nil {
			t.Fatal(err)
		}
		wantRows := []struct{ interest, principal, balance string }{
			{"10.00", "330.00", "670.00"},
			{"6.70", "333.30", "336.70"},
			{"3.37", "336.63", "0.07"},
			{"0", "0.07", "0"}, // interest rounds to zero on the residual; principal clamps
		}
		if len(proj.Rows) != len(wantRows) {
			t.Fatalf("expected %d rows, got %d", len(wantRows), len(proj.Rows))
		}
		for i, w := range wantRows {
			moneyEq(t, proj.Rows[i].Interest, m(w.interest), "interest")
			moneyEq(t, proj.Rows[i].Principal, m(w.principal), "principal")
			moneyEq(t, proj.Rows[i].BalanceAfter, m(w.balance), "balance")
		}
		moneyEq(t, RemainingStats(proj).TotalInterestRemaining, m("20.07"), "total interest")
	})
}

// TestProjectEscrowPassThrough verifies escrow is added to the draft but never
// enters the interest/principal math (never double-subtracted).
func TestProjectEscrowPassThrough(t *testing.T) {
	next := types.MustParseDate("2026-08-01")
	escrow := m("50")
	proj, err := Project(m("1000"), m("0"), m("400"), escrow, next, 1)
	if err != nil {
		t.Fatal(err)
	}
	// 0% loan, $400 P&I → 400, 400, 200(clamped) principal; escrow untouched.
	wantPrincipal := []string{"400", "400", "200"}
	wantDraft := []string{"450", "450", "250"}
	if len(proj.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(proj.Rows))
	}
	for i := range proj.Rows {
		moneyEq(t, proj.Rows[i].Principal, m(wantPrincipal[i]), "principal")
		moneyEq(t, proj.Rows[i].Escrow, escrow, "escrow")
		moneyEq(t, proj.Rows[i].TotalDraft, m(wantDraft[i]), "total draft")
		moneyEq(t, proj.Rows[i].Interest, types.ZeroMoney, "interest")
	}
}

func TestProjectTruncation(t *testing.T) {
	next := types.MustParseDate("2026-08-01")
	// $1M @ 6% with a payment only $1 above the first month's interest keeps
	// principal near $1/month for far longer than the 1,200-row cap.
	proj, err := Project(m("1000000"), m("6"), m("5001"), types.ZeroMoney, next, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !proj.Truncated {
		t.Error("expected Truncated = true")
	}
	if len(proj.Rows) != maxProjectionRows {
		t.Errorf("expected %d rows at cap, got %d", maxProjectionRows, len(proj.Rows))
	}
	stats := RemainingStats(proj)
	if !stats.Truncated {
		t.Error("stats should propagate Truncated")
	}
	if !stats.PayoffDate.IsZero() {
		t.Errorf("truncated projection should have no payoff date, got %s", stats.PayoffDate)
	}
}

func TestProjectNegativeAmortization(t *testing.T) {
	next := types.MustParseDate("2026-08-01")
	_, err := Project(m("1000"), m("12"), m("5"), types.ZeroMoney, next, 1)
	if !errors.Is(err, ErrNegativeAmortization) {
		t.Fatalf("expected ErrNegativeAmortization, got %v", err)
	}
}

func TestProjectPaidOff(t *testing.T) {
	next := types.MustParseDate("2026-08-01")
	for _, owed := range []string{"0", "-500"} {
		proj, err := Project(m(owed), m("6.5"), m("2401.86"), types.ZeroMoney, next, 1)
		if err != nil {
			t.Fatalf("owed=%s: unexpected error %v", owed, err)
		}
		if len(proj.Rows) != 0 {
			t.Errorf("owed=%s: expected 0 rows, got %d", owed, len(proj.Rows))
		}
		stats := RemainingStats(proj)
		if stats.PaymentsRemaining != 0 || stats.Truncated || !stats.PayoffDate.IsZero() {
			t.Errorf("owed=%s: unexpected stats %+v", owed, stats)
		}
		moneyEq(t, stats.TotalInterestRemaining, types.ZeroMoney, "interest")
	}
}

// TestProjectDates checks month-end clamping matches the scheduled engine.
func TestProjectDates(t *testing.T) {
	tests := []struct {
		name       string
		next       string
		dayOfMonth int
		wantDates  []string
	}{
		{
			name: "day 31 clamps to short months", next: "2027-01-31", dayOfMonth: 31,
			wantDates: []string{"2027-01-31", "2027-02-28", "2027-03-31"},
		},
		{
			name: "day 31 hits leap-year February", next: "2028-01-31", dayOfMonth: 31,
			wantDates: []string{"2028-01-31", "2028-02-29", "2028-03-31"},
		},
		{
			name: "last-day-of-month (-1)", next: "2027-01-15", dayOfMonth: -1,
			wantDates: []string{"2027-01-31", "2027-02-28", "2027-03-31"},
		},
		{
			name: "plain mid-month day", next: "2027-01-15", dayOfMonth: 15,
			wantDates: []string{"2027-01-15", "2027-02-15", "2027-03-15"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// 0% loan of 1000 with 400 P&I → exactly 3 payments.
			proj, err := Project(m("1000"), m("0"), m("400"), types.ZeroMoney,
				types.MustParseDate(tc.next), tc.dayOfMonth)
			if err != nil {
				t.Fatal(err)
			}
			if len(proj.Rows) != len(tc.wantDates) {
				t.Fatalf("expected %d rows, got %d", len(tc.wantDates), len(proj.Rows))
			}
			for i, want := range tc.wantDates {
				if !proj.Rows[i].Date.Equal(types.MustParseDate(want)) {
					t.Errorf("row %d date = %s, want %s", i+1, proj.Rows[i].Date, want)
				}
			}
		})
	}
}

func TestRemainingStatsEmpty(t *testing.T) {
	stats := RemainingStats(Projection{})
	if stats.PaymentsRemaining != 0 || stats.Truncated || !stats.PayoffDate.IsZero() {
		t.Errorf("empty projection stats = %+v", stats)
	}
	moneyEq(t, stats.TotalInterestRemaining, types.ZeroMoney, "interest")
}

func TestPowInt(t *testing.T) {
	two := alpacadecimal.NewFromInt(2)
	if got := powInt(two, 10); got.Cmp(alpacadecimal.NewFromInt(1024)) != 0 {
		t.Errorf("2^10 = %s, want 1024", got)
	}
	if got := powInt(two, 0); got.Cmp(alpacadecimal.NewFromInt(1)) != 0 {
		t.Errorf("2^0 = %s, want 1", got)
	}
	// Non-integer base stays exact under repeated squaring.
	onePointOne := alpacadecimal.RequireFromString("1.1")
	if got := powInt(onePointOne, 2); got.Cmp(alpacadecimal.RequireFromString("1.21")) != 0 {
		t.Errorf("1.1^2 = %s, want 1.21", got)
	}
}
