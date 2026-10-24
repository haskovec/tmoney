package db

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// newClosableDB creates a database file and returns its open handle.
func newClosableDB(t *testing.T) *DB {
	t.Helper()
	database, err := Create(filepath.Join(t.TempDir(), "fictional.tdb"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

// TestCloseForGoodBeforeWithFileClosedRunsNoFn: the TUI drops a handle with
// CloseForGood at a file switch. A backup or restore that the user asked for
// before, and that starts only now, must not run on it, and must not reopen
// it: nothing would close it again. A plain Close let both happen.
func TestCloseForGoodBeforeWithFileClosedRunsNoFn(t *testing.T) {
	database := newClosableDB(t)
	if err := database.CloseForGood(); err != nil {
		t.Fatal(err)
	}
	ran := false
	err := database.WithFileClosed(func(string) error { ran = true; return nil })
	if !errors.Is(err, ErrClosedForGood) || !errors.Is(err, ErrClosed) || ran {
		t.Errorf("WithFileClosed after CloseForGood = %v, fn ran = %v; want ErrClosedForGood and no fn", err, ran)
	}
	if !database.isClosed() {
		t.Error("WithFileClosed reopened a handle that was closed for good")
	}
}

// TestCloseForGoodStopsAQueuedWithFileClosed: a second file operation waits
// for the writer lock of the first one while the handle is closed for good.
// The first one ends without a reopen; the second one does not run.
func TestCloseForGoodStopsAQueuedWithFileClosed(t *testing.T) {
	database := newClosableDB(t)
	inFn, release := make(chan struct{}), make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- database.WithFileClosed(func(string) error {
			close(inFn)
			<-release
			return nil
		})
	}()
	<-inFn
	secondRan := false
	second := make(chan error, 1)
	go func() {
		second <- database.WithFileClosed(func(string) error { secondRan = true; return nil })
	}()
	waitParkedInLock(t, "(*DB).WithFileClosed") // the second one waits for txMu

	if err := database.CloseForGood(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Errorf("the first WithFileClosed = %v, want fn's nil", err)
	}
	if err := <-second; !errors.Is(err, ErrClosedForGood) || secondRan {
		t.Errorf("the queued WithFileClosed = %v, fn ran = %v; want ErrClosedForGood and no fn", err, secondRan)
	}
	if !database.isClosed() {
		t.Error("a file operation reopened a handle that was closed for good")
	}
}

// TestWithFileClosedReportsFnResultWhenItsReopenIsDropped: a close on purpose
// during fn drops the reopen. The caller of that close knows that the handle
// stays closed; the caller of WithFileClosed must get fn's own result, so a
// backup that was written does not report "database is closed".
func TestWithFileClosedReportsFnResultWhenItsReopenIsDropped(t *testing.T) {
	errFake := errors.New("fictional failure of fn")
	for _, c := range []struct {
		name  string
		close func(*DB) error
		fnErr error
	}{
		{"Close, fn ok", (*DB).Close, nil},
		{"Close, fn fails", (*DB).Close, errFake},
		{"CloseForGood, fn ok", (*DB).CloseForGood, nil},
		{"CloseForGood, fn fails", (*DB).CloseForGood, errFake},
	} {
		t.Run(c.name, func(t *testing.T) {
			database := newClosableDB(t)
			inFn, release := make(chan struct{}), make(chan struct{})
			done := make(chan error, 1)
			go func() {
				done <- database.WithFileClosed(func(string) error {
					close(inFn)
					<-release
					return c.fnErr
				})
			}()
			<-inFn
			if err := c.close(database); err != nil {
				t.Fatal(err)
			}
			close(release)
			err := <-done
			if !errors.Is(err, c.fnErr) || (c.fnErr == nil && err != nil) || errors.Is(err, ErrClosed) {
				t.Errorf("WithFileClosed() = %v, want fn's result %v only", err, c.fnErr)
			}
			if !database.isClosed() {
				t.Error("the handle reopened after a close on purpose")
			}
		})
	}
}

// TestErrClosedOfFnIsNotClosedForGood: ErrClosedForGood tells the caller
// that fn did not run. An ErrClosed that fn returns is fn's failure, and the
// caller must not take it for that refusal.
func TestErrClosedOfFnIsNotClosedForGood(t *testing.T) {
	for name, fnErr := range map[string]error{
		"ErrClosed":         ErrClosed,
		"wrapped ErrClosed": fmt.Errorf("fictional copy failure: %w", ErrClosed),
	} {
		t.Run(name, func(t *testing.T) {
			database := newClosableDB(t)
			ran := false
			err := database.WithFileClosed(func(string) error { ran = true; return fnErr })
			if !ran || !errors.Is(err, fnErr) || errors.Is(err, ErrClosedForGood) {
				t.Errorf("WithFileClosed() = %v, fn ran = %v; want fn's %v, not ErrClosedForGood", err, ran, fnErr)
			}
		})
	}
}

// TestWithFileClosedSaysClosedForGoodBeforeItWaits: a handle closed for good
// gives ErrClosedForGood also when an earlier reopen still waits for a read,
// or when a heal that is blocked holds reopenMu. It gave errFileBusy or
// errHealBlocked ("restart tmoney"), and the TUI showed that in place of "did
// not start".
func TestWithFileClosedSaysClosedForGoodBeforeItWaits(t *testing.T) {
	saved := reopenWaitTimeout
	reopenWaitTimeout = 300 * time.Millisecond
	t.Cleanup(func() { reopenWaitTimeout = saved })

	t.Run("an earlier reopen waits for a read", func(t *testing.T) {
		database := newClosableDB(t)
		if err := database.CloseForGood(); err != nil {
			t.Fatal(err)
		}
		database.fileBusy.Store(true) // as the reopen of a WithFileClosed that timed out
		t.Cleanup(func() { database.fileBusy.Store(false) })
		ran := false
		err := database.WithFileClosed(func(string) error { ran = true; return nil })
		if !errors.Is(err, ErrClosedForGood) || ran {
			t.Errorf("WithFileClosed() = %v, fn ran = %v; want ErrClosedForGood and no fn", err, ran)
		}
	})
	t.Run("closed for good while it waits for a blocked heal", func(t *testing.T) {
		database := newClosableDB(t)
		database.reopenMu.Lock() // as a heal that is blocked in its open
		ran := false
		done := make(chan error, 1)
		go func() {
			done <- database.WithFileClosed(func(string) error { ran = true; return nil })
		}()
		waitInStack(t, "(*DB).WithFileClosed", "db.lockBy(")
		if err := database.CloseForGood(); err != nil {
			t.Fatal(err)
		}
		err := <-done
		database.reopenMu.Unlock()
		if !errors.Is(err, ErrClosedForGood) || ran {
			t.Errorf("WithFileClosed() = %v, fn ran = %v; want ErrClosedForGood and no fn", err, ran)
		}
	})
}

// TestDroppedReopenDoesNotOpenTheFile: a reopen that a close on purpose
// dropped must not open the file even for a moment. The open would get the
// DuckDB instance of a new handle on the same file, or block behind it.
func TestDroppedReopenDoesNotOpenTheFile(t *testing.T) {
	for name, closeIt := range map[string]func(*DB) error{"Close": (*DB).Close, "CloseForGood": (*DB).CloseForGood} {
		t.Run(name, func(t *testing.T) {
			database := newClosableDB(t)
			opens := 0
			reopened = func(string) { opens++ }
			t.Cleanup(func() { reopened = nil })
			inFn, release := make(chan struct{}), make(chan struct{})
			done := make(chan error, 1)
			go func() {
				done <- database.WithFileClosed(func(string) error {
					close(inFn)
					<-release
					return nil
				})
			}()
			<-inFn
			if err := closeIt(database); err != nil {
				t.Fatal(err)
			}
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if opens != 0 {
				t.Errorf("the dropped reopen opened the file %d time(s)", opens)
			}
		})
	}
}

// waitParkedInLock waits until a goroutine whose stack holds fn waits for a
// mutex, so that a test knows that a call is queued behind a lock.
func waitParkedInLock(t *testing.T, fn string) {
	t.Helper()
	waitInStack(t, fn, "Mutex).Lock")
}

// waitInStack waits until a goroutine whose stack holds fn is in frame.
func waitInStack(t *testing.T, fn, frame string) {
	t.Helper()
	buf := make([]byte, 1<<20)
	deadline := time.Now().Add(10 * time.Second)
	for {
		n := runtime.Stack(buf, true)
		for _, g := range strings.Split(string(buf[:n]), "\n\n") {
			if strings.Contains(g, fn) && strings.Contains(g, frame) {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no goroutine in %s is in %s", fn, frame)
		}
		time.Sleep(time.Millisecond)
	}
}
