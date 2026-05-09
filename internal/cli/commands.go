package cli

import (
	"fmt"
	"os"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/db"
)

// openServices opens the database and creates all services via the shared registry.
// It also does a best-effort update of the recent files in the config.
// Auto-posts due scheduled transactions and prints a summary if any were posted.
func openServices(file string) (*db.DB, *app.Services, error) {
	database, err := db.Open(file)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Best-effort update recent files
	if cfg, err := config.Load(); err == nil {
		cfg.AddRecentFile(file)
		_ = cfg.Save()
	}

	svc := app.NewServices(database)

	// Auto-post due scheduled transactions on file open
	if summary, err := svc.Scheduled.AutoPost(); err == nil && summary.PostedCount > 0 {
		fmt.Fprintf(os.Stdout, "Auto-posted %d scheduled transaction(s)\n", summary.PostedCount)
	}

	return database, svc, nil
}

// autoBackupAfterModification creates an auto-backup after a data-modifying CLI command.
func autoBackupAfterModification(dbPath string) {
	// Best-effort: don't fail the CLI command if backup fails
	_, _ = backup.CreateAutoBackup(dbPath)
}
