package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

func TestShouldShowChartPanel(t *testing.T) {
	cases := []struct {
		width int
		want  bool
	}{
		{0, false},
		{1, false},
		{80, false},
		{119, false},
		{120, true},
		{121, true},
		{200, true},
	}
	for _, tc := range cases {
		if got := shouldShowChartPanel(tc.width); got != tc.want {
			t.Errorf("shouldShowChartPanel(%d) = %v, want %v", tc.width, got, tc.want)
		}
	}
}

func TestNoPriceHistoryMessage(t *testing.T) {
	got := noPriceHistoryMessage()
	if got != "No price history" {
		t.Errorf("noPriceHistoryMessage() = %q, want %q", got, "No price history")
	}
}

func TestSinglePriceMessage(t *testing.T) {
	d, err := types.ParseDate("2026-04-15")
	if err != nil {
		t.Fatalf("ParseDate: %v", err)
	}
	m, err := types.NewMoney("185.50")
	if err != nil {
		t.Fatalf("NewMoney: %v", err)
	}
	p := &price.Price{
		Date:  d,
		Price: m,
	}

	got := singlePriceMessage(p)
	if !strings.Contains(got, "Only one price on file") {
		t.Errorf("singlePriceMessage missing leading sentence; got %q", got)
	}
	if !strings.Contains(got, "≥ 2 points") {
		t.Errorf("singlePriceMessage missing minimum-points note; got %q", got)
	}
	if !strings.Contains(got, "$185.50") {
		t.Errorf("singlePriceMessage missing formatted value; got %q", got)
	}
	if !strings.Contains(got, "2026-04-15") {
		t.Errorf("singlePriceMessage missing formatted date; got %q", got)
	}
}

func TestSinglePriceMessageNil(t *testing.T) {
	got := singlePriceMessage(nil)
	if got == "" {
		t.Errorf("singlePriceMessage(nil) returned empty string; expected fallback message")
	}
}

func TestPriceHistoryToSeriesEmpty(t *testing.T) {
	times, values := priceHistoryToSeries(nil)
	if len(times) != 0 || len(values) != 0 {
		t.Errorf("expected empty slices for nil input, got times=%v values=%v", times, values)
	}

	times, values = priceHistoryToSeries([]*price.Price{})
	if len(times) != 0 || len(values) != 0 {
		t.Errorf("expected empty slices for empty input, got times=%v values=%v", times, values)
	}
}

func TestPriceHistoryToSeriesSingle(t *testing.T) {
	d, _ := types.ParseDate("2026-04-15")
	m, _ := types.NewMoney("185.50")
	p := &price.Price{Date: d, Price: m}

	times, values := priceHistoryToSeries([]*price.Price{p})
	if len(times) != 1 || len(values) != 1 {
		t.Fatalf("expected length-1 slices, got len(times)=%d len(values)=%d", len(times), len(values))
	}
	if !times[0].Equal(d.Time()) {
		t.Errorf("times[0] = %v, want %v", times[0], d.Time())
	}
	if values[0] != 185.50 {
		t.Errorf("values[0] = %v, want %v", values[0], 185.50)
	}
}

func TestPriceHistoryToSeriesReversesNewestFirst(t *testing.T) {
	// GetPriceHistory returns newest first; the converter must emit
	// chronological (oldest first) order so ntcharts can plot left-to-right.
	mk := func(date, val string) *price.Price {
		d, _ := types.ParseDate(date)
		m, _ := types.NewMoney(val)
		return &price.Price{Date: d, Price: m}
	}
	input := []*price.Price{
		mk("2026-04-15", "300.00"), // newest
		mk("2026-04-10", "200.00"),
		mk("2026-04-01", "100.00"), // oldest
	}

	times, values := priceHistoryToSeries(input)
	if len(times) != 3 || len(values) != 3 {
		t.Fatalf("expected length-3 slices, got %d/%d", len(times), len(values))
	}

	// Ascending by time
	for i := 1; i < len(times); i++ {
		if times[i].Before(times[i-1]) {
			t.Errorf("times not ascending at index %d: %v before %v", i, times[i], times[i-1])
		}
	}

	want := []float64{100.00, 200.00, 300.00}
	for i, v := range want {
		if values[i] != v {
			t.Errorf("values[%d] = %v, want %v", i, values[i], v)
		}
	}
}

func TestPriceHistoryToSeriesAlreadyAscending(t *testing.T) {
	mk := func(date, val string) *price.Price {
		d, _ := types.ParseDate(date)
		m, _ := types.NewMoney(val)
		return &price.Price{Date: d, Price: m}
	}
	// Service contract is newest-first, but defensively the converter
	// should still sort if given ascending input — exercise that path.
	input := []*price.Price{
		mk("2026-04-01", "100.00"),
		mk("2026-04-10", "200.00"),
		mk("2026-04-15", "300.00"),
	}
	times, values := priceHistoryToSeries(input)
	for i := 1; i < len(times); i++ {
		if times[i].Before(times[i-1]) {
			t.Errorf("times not ascending at index %d", i)
		}
	}
	if values[0] != 100.00 || values[2] != 300.00 {
		t.Errorf("values not in expected order: %v", values)
	}
}

func TestClampYRangeDistinct(t *testing.T) {
	min, max := clampYRange([]float64{100, 110, 105})
	if min != 100 || max != 110 {
		t.Errorf("clampYRange distinct values = (%v, %v), want (100, 110)", min, max)
	}
}

func TestClampYRangeAllEqual(t *testing.T) {
	v := 200.0
	min, max := clampYRange([]float64{v, v, v})
	// Spec: ±0.5% of value
	wantMin := v - 0.005*v
	wantMax := v + 0.005*v
	if min != wantMin || max != wantMax {
		t.Errorf("clampYRange all-equal = (%v, %v), want (%v, %v)", min, max, wantMin, wantMax)
	}
	if max <= min {
		t.Errorf("clampYRange all-equal must produce non-zero spread; got min=%v max=%v", min, max)
	}
}

func TestClampYRangeAllZero(t *testing.T) {
	min, max := clampYRange([]float64{0, 0, 0})
	if min != -0.5 || max != 0.5 {
		t.Errorf("clampYRange all-zero = (%v, %v), want (-0.5, 0.5)", min, max)
	}
	if max <= min {
		t.Errorf("clampYRange all-zero must produce non-zero spread")
	}
}

func TestClampYRangeSingle(t *testing.T) {
	// Single value behaves like all-equal: pad ±0.5%.
	min, max := clampYRange([]float64{50})
	wantMin := 50 - 0.005*50
	wantMax := 50 + 0.005*50
	if min != wantMin || max != wantMax {
		t.Errorf("clampYRange single = (%v, %v), want (%v, %v)", min, max, wantMin, wantMax)
	}
}

func TestClampYRangeEmpty(t *testing.T) {
	// Defensive: empty input shouldn't panic; return a sane default.
	min, max := clampYRange(nil)
	if max <= min {
		t.Errorf("clampYRange empty must still produce non-zero spread; got min=%v max=%v", min, max)
	}
}

// Compile-time guard that time package usage stays meaningful.
var _ = time.Time{}
