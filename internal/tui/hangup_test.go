package tui

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// waitFor polls ok until it reports true, or fails the test after a second.
func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// The first hangup quits once; a second one, which can come while shutdown
// runs (the shell and the kernel can both send one), does nothing.
func TestWatchHangup_FirstSignalQuitsOnce(t *testing.T) {
	sig := make(chan os.Signal, 1)
	done := make(chan struct{})
	defer close(done)
	var quits atomic.Int32
	w := watchHangup(sig, done, func() { quits.Add(1) })

	if w.hungUp.Load() {
		t.Fatal("hungUp is set before any signal")
	}
	sig <- syscall.SIGHUP
	waitFor(t, "the first quit", func() bool { return quits.Load() == 1 })
	if !w.hungUp.Load() {
		t.Error("hungUp is not set after a hangup")
	}

	sig <- syscall.SIGHUP
	// The send returns when the watcher took the signal; give it time to act.
	time.Sleep(20 * time.Millisecond)
	if n := quits.Load(); n != 1 {
		t.Errorf("quit ran %d times after two hangups, want 1", n)
	}
}

// Without a signal, nothing quits, and the watch ends when done closes.
func TestWatchHangup_DoneEndsTheWatch(t *testing.T) {
	sig := make(chan os.Signal, 1)
	done := make(chan struct{})
	var quits atomic.Int32
	w := watchHangup(sig, done, func() { quits.Add(1) })

	close(done)
	select {
	case <-w.stopped:
	case <-time.After(time.Second):
		t.Fatal("the watch did not end after done closed")
	}
	// Nothing reads sig any more: a late signal stays in the buffer.
	sig <- syscall.SIGHUP
	time.Sleep(20 * time.Millisecond)
	if len(sig) != 1 || w.hungUp.Load() || quits.Load() != 0 {
		t.Errorf("after done: buffered=%d hungUp=%v quits=%d, want 1, false and 0",
			len(sig), w.hungUp.Load(), quits.Load())
	}
}

// hangupTestModel is a model that does nothing, for a real Program.
type hangupTestModel struct{}

func (hangupTestModel) Init() tea.Cmd                         { return nil }
func (m hangupTestModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (hangupTestModel) View() tea.View                        { return tea.NewView("") }

// A hangup that comes before p.Run starts still ends the program. p.Quit does
// nothing before Run, so with it the hangup was lost and Run never returned.
func TestQuitOnHangup_BeforeRunStarts(t *testing.T) {
	p := tea.NewProgram(hangupTestModel{}, tea.WithInput(nil), tea.WithOutput(io.Discard))
	sig := make(chan os.Signal, 1)
	done := make(chan struct{})
	defer close(done)
	w := watchHangup(sig, done, quitOnHangup(p))

	sig <- syscall.SIGHUP
	waitFor(t, "the hangup", w.hungUp.Load)

	errc := make(chan error, 1)
	go func() {
		_, err := p.Run()
		errc <- err
	}()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("Run after an early hangup: %v", err)
		}
	case <-time.After(5 * time.Second):
		p.Kill()
		t.Fatal("Run did not return after a hangup that came before it started")
	}
}

func TestRunError(t *testing.T) {
	other := errors.New("boom")
	// The error of Bubble Tea's renderer on a terminal that is gone.
	dead := fmt.Errorf("bubbletea: error writing to screen: %w",
		&fs.PathError{Op: "write", Path: "/dev/stdout", Err: syscall.EIO})

	tests := []struct {
		name   string
		err    error
		hungUp bool
		want   error
	}{
		{"no error", nil, false, nil},
		{"other error", other, false, other},
		{"other error after a hangup", other, true, nil},
		{"dead terminal before the hangup was seen", dead, false, nil},
		{"dead terminal after a hangup", dead, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runError(tt.err, tt.hungUp); !errors.Is(got, tt.want) || (got == nil) != (tt.want == nil) {
				t.Errorf("runError(%v, %v) = %v, want %v", tt.err, tt.hungUp, got, tt.want)
			}
		})
	}
}
