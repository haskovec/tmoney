package tui

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

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
