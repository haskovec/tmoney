package widget

import (
	"testing"
	"time"
)

func TestClickTracker_FirstClickIsSingle(t *testing.T) {
	now := time.Unix(0, 0)
	tr := NewClickTracker(400 * time.Millisecond)
	tr.SetNowFn(func() time.Time { return now })

	if tr.Click(5) {
		t.Fatal("first click should not be a double click")
	}
}

func TestClickTracker_SameRowWithinThresholdIsDouble(t *testing.T) {
	now := time.Unix(0, 0)
	tr := NewClickTracker(400 * time.Millisecond)
	tr.SetNowFn(func() time.Time { return now })

	tr.Click(5)
	now = now.Add(200 * time.Millisecond)
	if !tr.Click(5) {
		t.Fatal("second click on same row within threshold should be a double click")
	}
}

func TestClickTracker_SameRowAtThresholdBoundaryIsDouble(t *testing.T) {
	now := time.Unix(0, 0)
	tr := NewClickTracker(400 * time.Millisecond)
	tr.SetNowFn(func() time.Time { return now })

	tr.Click(5)
	now = now.Add(400 * time.Millisecond)
	if !tr.Click(5) {
		t.Fatal("click exactly at threshold should still count as double")
	}
}

func TestClickTracker_SameRowOutsideThresholdIsSingle(t *testing.T) {
	now := time.Unix(0, 0)
	tr := NewClickTracker(400 * time.Millisecond)
	tr.SetNowFn(func() time.Time { return now })

	tr.Click(5)
	now = now.Add(401 * time.Millisecond)
	if tr.Click(5) {
		t.Fatal("click past threshold on same row should be a single click")
	}
}

func TestClickTracker_DifferentRowIsSingleAndResets(t *testing.T) {
	now := time.Unix(0, 0)
	tr := NewClickTracker(400 * time.Millisecond)
	tr.SetNowFn(func() time.Time { return now })

	tr.Click(5)
	now = now.Add(50 * time.Millisecond)
	if tr.Click(7) {
		t.Fatal("click on different row should not be double")
	}
	now = now.Add(50 * time.Millisecond)
	if !tr.Click(7) {
		t.Fatal("subsequent click on the new row within threshold should be double")
	}
}

func TestClickTracker_DoubleClickConsumesState(t *testing.T) {
	now := time.Unix(0, 0)
	tr := NewClickTracker(400 * time.Millisecond)
	tr.SetNowFn(func() time.Time { return now })

	tr.Click(5)
	now = now.Add(50 * time.Millisecond)
	tr.Click(5) // double
	now = now.Add(50 * time.Millisecond)
	if tr.Click(5) {
		t.Fatal("third quick click should be a fresh single, not another double")
	}
}

func TestClickTracker_ZeroValueDefaultsToTimeNow(t *testing.T) {
	tr := NewClickTracker(400 * time.Millisecond)
	// Should not panic without SetNowFn; using real time.Now.
	if tr.Click(1) {
		t.Fatal("first real-time click should not be a double")
	}
}
