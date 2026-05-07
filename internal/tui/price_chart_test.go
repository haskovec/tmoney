package tui

import (
	"errors"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
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
	// 100..110 has range 10, target=4 → step 2; min/max already on
	// step boundaries so they pass through unchanged.
	min, max, step := clampYRange([]float64{100, 110, 105})
	if min != 100 || max != 110 || step != 2 {
		t.Errorf("clampYRange distinct = (%v, %v, %v), want (100, 110, 2)", min, max, step)
	}
}

func TestClampYRangeSnapsToStepBoundaries(t *testing.T) {
	// Reproduces the screenshot bug: AGTHX prices ~81.5..83.4 used to
	// produce uneven label spacing because the raw min straddled an
	// integer-rounding boundary. Snapped range must land on step.
	min, max, step := clampYRange([]float64{81.55, 82.10, 82.97, 83.41})
	if step <= 0 {
		t.Fatalf("step must be positive; got %v", step)
	}
	if min > 81.55 {
		t.Errorf("snapped min=%v should be <= raw min 81.55", min)
	}
	if max < 83.41 {
		t.Errorf("snapped max=%v should be >= raw max 83.41", max)
	}
	if mod := math.Mod(min, step); math.Abs(mod) > 1e-9 && math.Abs(mod-step) > 1e-9 {
		t.Errorf("snapped min=%v not a multiple of step %v", min, step)
	}
	if mod := math.Mod(max, step); math.Abs(mod) > 1e-9 && math.Abs(mod-step) > 1e-9 {
		t.Errorf("snapped max=%v not a multiple of step %v", max, step)
	}
}

func TestClampYRangeAllEqual(t *testing.T) {
	v := 200.0
	min, max, step := clampYRange([]float64{v, v, v})
	// Spec: ±0.5% of value, step 1 fallback.
	wantMin := v - 0.005*v
	wantMax := v + 0.005*v
	if min != wantMin || max != wantMax || step != 1 {
		t.Errorf("clampYRange all-equal = (%v, %v, %v), want (%v, %v, 1)", min, max, step, wantMin, wantMax)
	}
	if max <= min {
		t.Errorf("clampYRange all-equal must produce non-zero spread; got min=%v max=%v", min, max)
	}
}

func TestClampYRangeAllZero(t *testing.T) {
	min, max, step := clampYRange([]float64{0, 0, 0})
	if min != -0.5 || max != 0.5 || step != 1 {
		t.Errorf("clampYRange all-zero = (%v, %v, %v), want (-0.5, 0.5, 1)", min, max, step)
	}
}

func TestClampYRangeSingle(t *testing.T) {
	min, max, step := clampYRange([]float64{50})
	wantMin := 50 - 0.005*50
	wantMax := 50 + 0.005*50
	if min != wantMin || max != wantMax || step != 1 {
		t.Errorf("clampYRange single = (%v, %v, %v), want (%v, %v, 1)", min, max, step, wantMin, wantMax)
	}
}

func TestClampYRangeEmpty(t *testing.T) {
	min, max, step := clampYRange(nil)
	if max <= min {
		t.Errorf("clampYRange empty must still produce non-zero spread; got min=%v max=%v", min, max)
	}
	if step <= 0 {
		t.Errorf("clampYRange empty must return positive step; got %v", step)
	}
}

func TestNiceStep(t *testing.T) {
	cases := []struct {
		rangeSize float64
		ticks     int
		want      float64
	}{
		{10, 4, 2},    // rough=2.5 → norm=2.5 → "2"
		{2.5, 4, 0.5}, // rough=0.625 → mag=0.1, norm=6.25 → "5"
		{1, 4, 0.2},   // rough=0.25 → mag=0.1, norm=2.5 → "2"
		{100, 4, 20},  // rough=25 → mag=10, norm=2.5 → "2"
		{0, 4, 1},     // degenerate
		{-5, 4, 1},    // negative
		{10, 0, 1},    // zero ticks
	}
	for _, tc := range cases {
		if got := niceStep(tc.rangeSize, tc.ticks); got != tc.want {
			t.Errorf("niceStep(%v, %d) = %v, want %v", tc.rangeSize, tc.ticks, got, tc.want)
		}
	}
}

func TestNiceYLabelFormatter(t *testing.T) {
	// Integer step: zero decimals.
	f := niceYLabelFormatter(1)
	if got := f(0, 81.7); got != "81" {
		t.Errorf("formatter(step=1, 81.7) = %q, want %q", got, "81")
	}
	// Sub-integer step picks decimals from the step width.
	f = niceYLabelFormatter(0.5)
	if got := f(0, 81.7); got != "81.5" {
		t.Errorf("formatter(step=0.5, 81.7) = %q, want %q", got, "81.5")
	}
	if got := f(0, 82.0); got != "82.0" {
		t.Errorf("formatter(step=0.5, 82.0) = %q, want %q", got, "82.0")
	}
	// Floor (not round) so adjacent rows on the same step bucket render
	// the same label — that's how ntcharts dedupes into evenly-spaced
	// labels. step=0.5 buckets [81.5, 82.0): all should format as "81.5".
	f = niceYLabelFormatter(0.5)
	if a, b := f(0, 81.51), f(0, 81.99); a != b {
		t.Errorf("values within the same step bucket should format identically; got %q vs %q", a, b)
	}
	// Crossing the bucket boundary changes the label.
	if a, b := f(0, 81.49), f(0, 81.51); a == b {
		t.Errorf("values across a bucket boundary should differ; both formatted as %q", a)
	}
}

// Compile-time guard that time package usage stays meaningful.
var _ = time.Time{}

// =============================================================================
// PC-006: Chart panel rendering
// =============================================================================

func mkPrice(t *testing.T, secID types.ID, dateStr, val string) *price.Price {
	t.Helper()
	d, err := types.ParseDate(dateStr)
	if err != nil {
		t.Fatalf("ParseDate(%q): %v", dateStr, err)
	}
	m, err := types.NewMoney(val)
	if err != nil {
		t.Fatalf("NewMoney(%q): %v", val, err)
	}
	p := price.NewPrice(secID, d, m, price.SourceManual)
	return p
}

func TestBuildChartPanel_NilSecurityReturnsEmpty(t *testing.T) {
	got := buildChartPanel(40, 12, nil, nil)
	if got != "" {
		t.Errorf("buildChartPanel(nil sec) = %q, want empty", got)
	}
}

func TestBuildChartPanel_TooSmallReturnsEmpty(t *testing.T) {
	sec := newAAPL()
	for _, dim := range []struct{ w, h int }{
		{0, 10},
		{10, 0},
		{5, 5},   // below the minimum useful width
		{40, 2},  // not tall enough for top + middle + bottom border
		{-1, 12}, // pathological
	} {
		got := buildChartPanel(dim.w, dim.h, sec, nil)
		if got != "" {
			t.Errorf("buildChartPanel(w=%d, h=%d) = %q, want empty", dim.w, dim.h, got)
		}
	}
}

func TestBuildChartPanel_TitleEmbedsTickerAndName(t *testing.T) {
	sec := newAAPL()
	prices := []*price.Price{
		mkPrice(t, sec.ID, "2026-04-15", "300.00"),
		mkPrice(t, sec.ID, "2026-04-10", "200.00"),
		mkPrice(t, sec.ID, "2026-04-01", "100.00"),
	}
	out := buildChartPanel(50, 12, sec, prices)
	if out == "" {
		t.Fatal("buildChartPanel returned empty for valid input")
	}
	if !strings.Contains(out, "AAPL") {
		t.Errorf("chart panel missing ticker; got:\n%s", out)
	}
	if !strings.Contains(out, "Apple Inc.") {
		t.Errorf("chart panel missing security name; got:\n%s", out)
	}
}

func TestBuildChartPanel_HasBoxBorder(t *testing.T) {
	sec := newAAPL()
	prices := []*price.Price{
		mkPrice(t, sec.ID, "2026-04-01", "100.00"),
		mkPrice(t, sec.ID, "2026-04-15", "300.00"),
	}
	out := buildChartPanel(50, 12, sec, prices)
	// Must contain both vertical and horizontal box-drawing runes.
	if !strings.ContainsRune(out, '─') {
		t.Errorf("chart panel missing horizontal border rune; got:\n%s", out)
	}
	if !strings.ContainsRune(out, '│') {
		t.Errorf("chart panel missing vertical border rune; got:\n%s", out)
	}
}

func TestBuildChartPanel_ZeroPricesShowsPlaceholder(t *testing.T) {
	sec := newAAPL()
	out := buildChartPanel(50, 12, sec, nil)
	if !strings.Contains(out, "No price history") {
		t.Errorf("0-price panel must show placeholder; got:\n%s", out)
	}
	if !strings.Contains(out, "AAPL") {
		t.Errorf("0-price panel must still show ticker title; got:\n%s", out)
	}
}

func TestBuildChartPanel_OnePriceShowsPlaceholder(t *testing.T) {
	sec := newAAPL()
	p := mkPrice(t, sec.ID, "2026-04-15", "185.50")
	out := buildChartPanel(50, 12, sec, []*price.Price{p})
	if !strings.Contains(out, "Only one price on file") {
		t.Errorf("1-price panel must show placeholder; got:\n%s", out)
	}
	if !strings.Contains(out, "$185.50") {
		t.Errorf("1-price panel must show formatted value; got:\n%s", out)
	}
	if !strings.Contains(out, "2026-04-15") {
		t.Errorf("1-price panel must show formatted date; got:\n%s", out)
	}
}

func TestBuildChartPanel_FlatLineDoesNotPanic(t *testing.T) {
	sec := newAAPL()
	prices := []*price.Price{
		mkPrice(t, sec.ID, "2026-04-01", "100.00"),
		mkPrice(t, sec.ID, "2026-04-08", "100.00"),
		mkPrice(t, sec.ID, "2026-04-15", "100.00"),
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("buildChartPanel panicked on flat-line input: %v", r)
		}
	}()
	out := buildChartPanel(50, 12, sec, prices)
	if out == "" {
		t.Errorf("flat-line input should still produce a non-empty panel render")
	}
}

func newAAPL() *security.Security {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	return sec
}

// =============================================================================
// PC-011: historyCache
// =============================================================================

func TestHistoryCache_GetCallsLoaderOnceForSameID(t *testing.T) {
	c := newHistoryCache()
	id := types.NewID()
	want := []*price.Price{mkPrice(t, id, "2026-04-01", "100.00")}

	var calls int32
	loader := func() ([]*price.Price, error) {
		atomic.AddInt32(&calls, 1)
		return want, nil
	}

	got1, err := c.Get(id, loader)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	got2, err := c.Get(id, loader)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("loader called %d times, want 1", n)
	}
	if len(got1) != 1 || len(got2) != 1 || got1[0] != want[0] || got2[0] != want[0] {
		t.Errorf("Get returned unexpected slice; got1=%v got2=%v want=%v", got1, got2, want)
	}
}

func TestHistoryCache_GetSeparateIDsLoadIndependently(t *testing.T) {
	c := newHistoryCache()
	idA := types.NewID()
	idB := types.NewID()

	var calls int32
	loader := func() ([]*price.Price, error) {
		atomic.AddInt32(&calls, 1)
		return []*price.Price{mkPrice(t, idA, "2026-04-01", "100.00")}, nil
	}

	if _, err := c.Get(idA, loader); err != nil {
		t.Fatalf("Get(A): %v", err)
	}
	if _, err := c.Get(idB, loader); err != nil {
		t.Fatalf("Get(B): %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Errorf("loader called %d times, want 2 (one per ID)", n)
	}
}

func TestHistoryCache_EvictForcesReload(t *testing.T) {
	c := newHistoryCache()
	id := types.NewID()

	var calls int32
	loader := func() ([]*price.Price, error) {
		atomic.AddInt32(&calls, 1)
		return []*price.Price{mkPrice(t, id, "2026-04-01", "100.00")}, nil
	}

	if _, err := c.Get(id, loader); err != nil {
		t.Fatalf("first Get: %v", err)
	}
	c.Evict(id)
	if _, err := c.Get(id, loader); err != nil {
		t.Fatalf("post-Evict Get: %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Errorf("loader called %d times, want 2 (cache miss after Evict)", n)
	}
}

func TestHistoryCache_EvictUnknownIDIsNoOp(t *testing.T) {
	c := newHistoryCache()
	c.Evict(types.NewID()) // Must not panic on unknown key.
}

func TestHistoryCache_ClearEvictsAllEntries(t *testing.T) {
	c := newHistoryCache()
	idA := types.NewID()
	idB := types.NewID()

	var calls int32
	loader := func() ([]*price.Price, error) {
		atomic.AddInt32(&calls, 1)
		return []*price.Price{mkPrice(t, idA, "2026-04-01", "100.00")}, nil
	}

	if _, err := c.Get(idA, loader); err != nil {
		t.Fatalf("Get(A): %v", err)
	}
	if _, err := c.Get(idB, loader); err != nil {
		t.Fatalf("Get(B): %v", err)
	}
	c.Clear()
	if _, err := c.Get(idA, loader); err != nil {
		t.Fatalf("post-Clear Get(A): %v", err)
	}
	if _, err := c.Get(idB, loader); err != nil {
		t.Fatalf("post-Clear Get(B): %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 4 {
		t.Errorf("loader called %d times, want 4 (cache miss for both after Clear)", n)
	}
}

func TestHistoryCache_LoaderErrorIsNotCached(t *testing.T) {
	c := newHistoryCache()
	id := types.NewID()

	wantErr := errors.New("boom")
	var calls int32
	failingLoader := func() ([]*price.Price, error) {
		atomic.AddInt32(&calls, 1)
		return nil, wantErr
	}

	if _, err := c.Get(id, failingLoader); !errors.Is(err, wantErr) {
		t.Fatalf("first Get error = %v, want %v", err, wantErr)
	}
	if _, err := c.Get(id, failingLoader); !errors.Is(err, wantErr) {
		t.Fatalf("second Get error = %v, want %v", err, wantErr)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Errorf("loader called %d times, want 2 (errors must not be cached)", n)
	}

	// After a successful load, the value is cached even though earlier
	// attempts errored.
	want := []*price.Price{mkPrice(t, id, "2026-04-01", "100.00")}
	successLoader := func() ([]*price.Price, error) {
		return want, nil
	}
	if _, err := c.Get(id, successLoader); err != nil {
		t.Fatalf("recovery Get: %v", err)
	}
	if _, err := c.Get(id, failingLoader); err != nil {
		t.Fatalf("post-recovery Get: %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Errorf("failing loader was called after a successful cache fill; calls=%d", n)
	}
}

func TestHistoryCache_ConcurrentGetsAreSafe(t *testing.T) {
	c := newHistoryCache()
	const goroutines = 32
	const idCount = 4

	ids := make([]types.ID, idCount)
	for i := range ids {
		ids[i] = types.NewID()
	}

	loader := func(id types.ID) priceHistoryLoader {
		return func() ([]*price.Price, error) {
			return []*price.Price{mkPrice(t, id, "2026-04-01", "100.00")}, nil
		}
	}

	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := range 50 {
				id := ids[(i+j)%idCount]
				if _, err := c.Get(id, loader(id)); err != nil {
					t.Errorf("Get: %v", err)
					return
				}
				if j%17 == 0 {
					c.Evict(id)
				}
				if j%29 == 0 {
					c.Clear()
				}
			}
		}(i)
	}
	wg.Wait()
}
