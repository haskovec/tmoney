package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/filegate"
)

// stubResumeLauncher swaps resumeLauncher with a stub that records its calls.
func stubResumeLauncher(t *testing.T) *[][2]string {
	t.Helper()
	calls := [][2]string{}
	orig := resumeLauncher
	resumeLauncher = func(file, resume string) error {
		calls = append(calls, [2]string{file, resume})
		return nil
	}
	t.Cleanup(func() { resumeLauncher = orig })
	return &calls
}

// The terminal runs "tmoney -f <file> --resume <value>": that is a resume,
// not a normal start.
func TestExecute_ResumeFlag_LaunchesResume(t *testing.T) {
	tui, restore := stubLaunchers(t)
	defer restore()
	resumes := stubResumeLauncher(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := ExecuteWith([]string{"-f", "/data/f.tdb", "--resume", "scheduled"}, stdout, stderr); err != nil {
		t.Fatalf("ExecuteWith: %v", err)
	}
	if len(*tui) != 0 {
		t.Errorf("a resume made a normal TUI launch: %q", *tui)
	}
	if want := [][2]string{{"/data/f.tdb", "scheduled"}}; len(*resumes) != 1 || (*resumes)[0] != want[0] {
		t.Errorf("resume launches = %q, want %q", *resumes, want)
	}
}

// --resume is for the terminal, not for users: --help does not list it.
func TestExecute_ResumeFlag_IsHidden(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := ExecuteWith([]string{"--help"}, stdout, stderr); err != nil {
		t.Fatalf("ExecuteWith(--help): %v", err)
	}
	if strings.Contains(stdout.String()+stderr.String(), "resume") {
		t.Errorf("--help lists --resume:\n%s", stdout.String())
	}
}

// isolateConfig gives the test its own HOME and config folder.
func isolateConfig(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
}

// A resume never creates a file: a file that is gone is an error.
func TestRunTUIResume_MissingFileIsAnError(t *testing.T) {
	isolateConfig(t)
	file := filepath.Join(t.TempDir(), "moved.tdb")

	err := runTUIResume(file, "dashboard")
	if err == nil || !strings.Contains(err.Error(), "cannot resume") || !strings.Contains(err.Error(), file) {
		t.Fatalf("runTUIResume on a missing file: err = %v, want \"cannot resume: <file> does not exist\"", err)
	}
	if _, statErr := os.Stat(file); !os.IsNotExist(statErr) {
		t.Errorf("runTUIResume created %s (stat: %v)", file, statErr)
	}
}

// stubOpen makes openDatabase fail with lockErr for the first locked calls,
// and then succeed. It returns the count of calls.
func stubOpen(t *testing.T, locked int, lockErr error) *int {
	t.Helper()
	return stubOpenWaiting(t, locked, lockErr, false)
}

// stubOpenWaiting is stubOpen, and with gateWait each call first calls its
// waiting func, as an open that waits for the gate of the file does.
func stubOpenWaiting(t *testing.T, locked int, lockErr error, gateWait bool) *int {
	t.Helper()
	calls := 0
	origOpen, origLocked, origInterval := openDatabase, lockedElsewhere, lockRetryInterval
	openDatabase = func(_ string, waiting func()) (*db.DB, error) {
		calls++
		if gateWait {
			waiting()
		}
		if calls <= locked {
			return nil, lockErr
		}
		return nil, nil
	}
	lockedElsewhere = func(_ string, err error) bool { return errors.Is(err, errTestLocked) }
	lockRetryInterval = time.Millisecond
	t.Cleanup(func() { openDatabase, lockedElsewhere, lockRetryInterval = origOpen, origLocked, origInterval })
	return &calls
}

var errTestLocked = errors.New("the file is locked by another process")

func TestOpenWaitingForLock(t *testing.T) {
	t.Run("a normal start tries once", func(t *testing.T) {
		calls := stubOpen(t, 5, errTestLocked)
		var out bytes.Buffer
		if _, err := openWaitingForLock("f.tdb", 0, &out); !errors.Is(err, errTestLocked) {
			t.Errorf("err = %v, want the lock error", err)
		}
		if *calls != 1 || out.Len() != 0 {
			t.Errorf("calls = %d, output %q; want 1 call and no output", *calls, out.String())
		}
	})
	t.Run("a resume waits for the other tmoney", func(t *testing.T) {
		calls := stubOpen(t, 3, errTestLocked)
		var out bytes.Buffer
		if _, err := openWaitingForLock("f.tdb", 5*time.Second, &out); err != nil {
			t.Errorf("err = %v, want the open after the lock went", err)
		}
		if *calls != 4 {
			t.Errorf("calls = %d, want 4", *calls)
		}
		if got := strings.Count(out.String(), "Waiting for another tmoney to close f.tdb"); got != 1 {
			t.Errorf("the wait message shows %d times, want 1:\n%s", got, out.String())
		}
	})
	t.Run("a resume stops waiting at the deadline", func(t *testing.T) {
		stubOpen(t, 1<<30, errTestLocked)
		var out bytes.Buffer
		start := time.Now()
		if _, err := openWaitingForLock("f.tdb", 50*time.Millisecond, &out); !errors.Is(err, errTestLocked) {
			t.Errorf("err = %v, want the lock error", err)
		}
		if took := time.Since(start); took > 2*time.Second {
			t.Errorf("the wait took %v, want about 50ms", took)
		}
	})
	t.Run("a wait for the gate and for the lock says it once", func(t *testing.T) {
		calls := stubOpenWaiting(t, 1, errTestLocked, true)
		var out bytes.Buffer
		if _, err := openWaitingForLock("f.tdb", 5*time.Second, &out); err != nil {
			t.Errorf("err = %v, want the open after the lock went", err)
		}
		if *calls != 2 {
			t.Errorf("calls = %d, want 2", *calls)
		}
		if got := strings.Count(out.String(), "\n"); got != 1 || !strings.Contains(out.String(), "Waiting for another tmoney to close f.tdb...") {
			t.Errorf("output = %q, want exactly one wait line", out.String())
		}
	})
	t.Run("a normal start says that it waits for the gate", func(t *testing.T) {
		stubOpenWaiting(t, 0, nil, true)
		var out bytes.Buffer
		if _, err := openWaitingForLock("f.tdb", 0, &out); err != nil {
			t.Errorf("err = %v, want the open", err)
		}
		if out.String() != "Waiting for another tmoney to close f.tdb...\n" {
			t.Errorf("output = %q, want the wait line", out.String())
		}
	})
	t.Run("a gate that stays busy is final", func(t *testing.T) {
		busy := &filegate.BusyError{File: "f.tdb"}
		calls := stubOpen(t, 5, busy)
		lockedElsewhere = func(string, error) bool { return true } // the other tmoney also has the file open
		if _, err := openWaitingForLock("f.tdb", 5*time.Second, &bytes.Buffer{}); !errors.Is(err, db.ErrGateBusy) {
			t.Errorf("err = %v, want the busy gate", err)
		}
		if *calls != 1 {
			t.Errorf("calls = %d, want 1", *calls)
		}
	})
	t.Run("another error does not wait", func(t *testing.T) {
		other := errors.New("not a TMoney file")
		calls := stubOpen(t, 5, other)
		if _, err := openWaitingForLock("f.tdb", 5*time.Second, &bytes.Buffer{}); !errors.Is(err, other) {
			t.Errorf("err = %v, want %v", err, other)
		}
		if *calls != 1 {
			t.Errorf("calls = %d, want 1", *calls)
		}
	})
}
