package tui

import "time"

// ClickTracker detects double-clicks on the same row within a configurable
// time threshold. Bubbletea v1 mouse messages do not carry a click count,
// so views that want to distinguish a single click (selection) from a
// double click (activation) feed each click into a tracker.
type ClickTracker struct {
	threshold time.Duration
	nowFn     func() time.Time
	hasLast   bool
	lastRow   int
	lastTime  time.Time
}

// NewClickTracker returns a tracker that treats two clicks on the same row
// within threshold as a double click.
func NewClickTracker(threshold time.Duration) *ClickTracker {
	return &ClickTracker{threshold: threshold}
}

// SetNowFn overrides the clock used for click timing. Tests inject a fake
// clock; production callers leave it unset so time.Now is used.
func (c *ClickTracker) SetNowFn(fn func() time.Time) {
	c.nowFn = fn
}

// Click records a click on row and returns true if it forms a double click
// with the immediately preceding click. A double click consumes the tracker
// state so a third quick click on the same row counts as a fresh single.
func (c *ClickTracker) Click(row int) bool {
	now := c.now()
	if c.hasLast && c.lastRow == row && now.Sub(c.lastTime) <= c.threshold {
		c.hasLast = false
		return true
	}
	c.hasLast = true
	c.lastRow = row
	c.lastTime = now
	return false
}

func (c *ClickTracker) now() time.Time {
	if c.nowFn != nil {
		return c.nowFn()
	}
	return time.Now()
}
