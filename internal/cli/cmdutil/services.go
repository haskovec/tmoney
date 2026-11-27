package cmdutil

import (
	"errors"
	"fmt"
	"os"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/db"
)

// OpenServices opens the database and creates all services via the shared registry.
// It also does a best-effort update of the recent files in the config.
// It runs the open-time repairs (app.Services.Prepare), then auto-posts due
// scheduled transactions and prints a summary if any were posted. A failure
// in either is printed to stderr as a warning, and the command still runs.
//
// While another tmoney closes the file, or makes or restores a backup of it,
// the open waits up to db.GateWait for it (see db.OpenWaiting), and says so
// once on stderr. A tmoney that has the file open fails the open at once.
func OpenServices(file string) (*db.DB, *app.Services, error) {
	database, err := db.OpenWaiting(file, db.GateWait, WaitNotice(file))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Best-effort update recent files
	if cfg, err := config.Load(); err == nil {
		cfg.AddRecentFile(file)
		_ = cfg.Save()
	}

	svc := app.NewServices(database)

	// Run the open-time repairs. A failure is shown, not fatal: failing here
	// would also lock the user out of the commands that fix it
	// (`investment rebuild-positions`, backup, export), and writes do not
	// depend on these repairs. Printing it means no command hides it.
	if err := svc.Prepare(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: startup repair failed: %v\n", err)
	}

	// Auto-post due scheduled transactions on file open, with the same rule
	// for a failure.
	summary, err := svc.Scheduled.AutoPost()
	switch {
	case err != nil:
		fmt.Fprintf(os.Stderr, "warning: auto-post of scheduled transactions failed: %v\n", err)
	case summary.PostedCount > 0:
		fmt.Fprintf(os.Stdout, "Auto-posted %d scheduled transaction(s)\n", summary.PostedCount)
	}

	return database, svc, nil
}

// WaitNotice returns the waiting func of a CLI command's wait for the gate
// of file (see db.OpenWaiting and db.HoldGate). It tells the user on stderr
// that the command waits for another tmoney, so that a wait of some seconds
// does not look like a hang.
func WaitNotice(file string) func() {
	return func() {
		fmt.Fprintf(os.Stderr, "Waiting for another tmoney to finish with %s...\n", file)
	}
}

// AutoBackupAfterModification creates an auto-backup after a data-modifying CLI
// command. It closes the database first, waits for a heal in flight, and
// copies the file while it holds the gate of the file (see
// db.DB.CloseAndSettleWith), so no other tmoney opens the file before the copy
// ends. Call it as the command's last database action; the deferred Close is
// then a no-op.
func AutoBackupAfterModification(database *db.DB) {
	// Best-effort: don't fail the CLI command if the close or backup fails, and
	// skip the backup while a heal could still open the file under the copy.
	ran, err := database.CloseAndSettleWith(db.SettleTimeout, func(path string) error {
		_, err := createAutoBackup(path)
		return err
	})
	switch {
	case !ran && err != nil:
		fmt.Fprintf(os.Stderr, "warning: auto-backup skipped: %v\n", err)
	case !ran:
		fmt.Fprintf(os.Stderr, "warning: auto-backup skipped: the database was still busy\n")
	case err != nil:
		fmt.Fprintf(os.Stderr, "warning: auto-backup failed: %v\n", err)
	}
}

// createAutoBackup is backup.CreateAutoBackup. Tests replace it to look at
// the file from another process during the copy, and to make it fail.
var createAutoBackup = backup.CreateAutoBackup

// RequireFile returns the standard error when no database file was supplied via
// the persistent --file flag. It folds the identical guard repeated across
// every data-touching command; callers do:
//
//	if err := cmdutil.RequireFile(opts.file); err != nil {
//		return err
//	}
func RequireFile(file string) error {
	if file == "" {
		return errors.New("--file is required to specify a database")
	}
	return nil
}
