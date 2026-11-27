package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/tui"
)

// runTUI launches the interactive TUI mode for the given database file.
// Empty file means "use the last-opened file from config or the default
// location".
func runTUI(file string) error {
	return launchTUI(file, "")
}

// runTUIResume launches the TUI for a program resume (OSC 7577): the
// terminal runs the command that the TUI reported before the terminal closed,
// with resume as the --resume value (see tui/resume.go).
func runTUIResume(file, resume string) error {
	return launchTUI(file, resume)
}

// resumeLockWait is how long a resume waits for the file while another
// process has it open. At a terminal restart, the tmoney that the terminal
// closed can still be in its shutdown; the spec also lets a terminal run a
// resume more than once. A var so tests can shorten it.
var resumeLockWait = 10 * time.Second

// lockRetryInterval is the pause between the opens of that wait. A var so
// tests can shorten it.
var lockRetryInterval = 200 * time.Millisecond

// launchTUI opens file and runs the TUI on it; resume is the --resume value,
// or "" for a normal start.
func launchTUI(file, resume string) error {
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}

	if file == "" {
		file = cfg.ResolveDefaultFile()
	}
	if file == "" {
		file = filepath.Join(db.DefaultDirectory(), "default.tdb")
	}

	var database *db.DB
	if _, statErr := os.Stat(file); os.IsNotExist(statErr) {
		// A resume never creates a file. The file was moved, renamed or
		// deleted, or its volume is not mounted: an empty database at the old
		// path would show no data, and the quit backup would copy it.
		if resume != "" {
			return fmt.Errorf("cannot resume: %s does not exist", file)
		}
		dir := filepath.Dir(file)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		database, err = db.Create(file)
		if err != nil {
			return fmt.Errorf("failed to create database: %w", err)
		}
	} else {
		wait := time.Duration(0)
		if resume != "" {
			wait = resumeLockWait
		}
		database, err = openWaitingForLock(file, wait, os.Stderr)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
	}
	defer database.Close()

	cfg.AddRecentFile(file)
	_ = cfg.Save()

	return tui.Run(database, cfg, resume)
}

// openDatabase and lockedElsewhere are db.OpenWaiting and the check of a lock
// that another process holds; tests replace them. openDatabase waits up to
// db.GateWait for the gate of the file, and calls waiting once when that wait
// starts.
var (
	openDatabase = func(file string, waiting func()) (*db.DB, error) {
		return db.OpenWaiting(file, db.GateWait, waiting)
	}
	lockedElsewhere = func(path string, err error) bool {
		if db.IsLockedElsewhere(err) {
			return true
		}
		held, herr := db.HeldElsewhere(db.ExpandHome(path))
		return herr == nil && held
	}
)

// openWaitingForLock opens file. While another process has it open, it tries
// again for up to wait, and says once on w that it waits. With wait 0 it
// tries once, as a normal start does.
//
// Each open also waits for the gate of the file while another tmoney closes
// it, or makes or restores a backup of it (see db.OpenWaiting). That wait
// says the same line, and only once for both waits. When the gate stays busy
// to its deadline, the error is final: the other tmoney did not finish in
// time, and a try again would wait as long.
func openWaitingForLock(file string, wait time.Duration, w io.Writer) (*db.DB, error) {
	deadline := time.Now().Add(wait)
	told := false
	tell := func() {
		if !told {
			_, _ = fmt.Fprintf(w, "Waiting for another tmoney to close %s...\n", file)
			told = true
		}
	}
	for {
		database, err := openDatabase(file, tell)
		if err == nil || wait <= 0 || !time.Now().Before(deadline) || errors.Is(err, db.ErrGateBusy) || !lockedElsewhere(file, err) {
			return database, err
		}
		tell()
		time.Sleep(lockRetryInterval)
	}
}
