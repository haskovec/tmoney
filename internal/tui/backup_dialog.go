package tui

import (
	"fmt"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/tui/dialog"
)

// backupCreatedMsg is sent when a manual backup has been created. It carries the
// REOPENED database: taking the backup closed the live one (see
// createManualBackupCmd), so the app must switch to this handle.
type backupCreatedMsg struct {
	path string
	db   *db.DB
}

// restoreConfirmedMsg is sent when the restore+reopen is complete.
type restoreConfirmedMsg struct {
	db               *db.DB
	safetyBackupPath string
}

// reopenedAfterFailureMsg is sent when a restore or a manual backup failed AFTER the
// live database was closed, but the file could be reopened. The app must switch
// to the reopened handle — the one it holds is closed — and then report the
// error.
type reopenedAfterFailureMsg struct {
	db  *db.DB
	err error
}

// buildRestoreBackupDialog creates a dialog for selecting a backup to restore from.
func buildRestoreBackupDialog(dbPath string) (*dialog.Dialog, []backup.BackupInfo, error) {
	backups, err := backup.ListBackups(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list backups: %w", err)
	}

	d := dialog.NewDialog("Restore from Backup")

	if len(backups) == 0 {
		d.AddSelectField("Backup", []string{"(no backups available)"}, 0)
	} else {
		options := make([]string, len(backups))
		for i, b := range backups {
			options[i] = fmt.Sprintf("%s  %s  %s",
				b.Timestamp.Format("2006-01-02 15:04:05"),
				backup.FormatSize(b.Size),
				b.Type,
			)
		}
		d.AddSelectField("Backup", options, 0)
	}

	d.SetVisible(true)
	return d, backups, nil
}

// createManualBackupCmd returns a command that creates a manual backup.
func (a *App) createManualBackupCmd() tea.Cmd {
	current := a.db
	return func() tea.Msg {
		dbPath := current.Path()

		// Close before copying. An open DuckDB file cannot be copied faithfully:
		// committed writes may still be in its WAL, and on Windows the copy
		// cannot open the file at all (see db.DB.Close). A goroutine still
		// reading the old handle gets "database is closed", not a panic.
		if err := current.Close(); err != nil {
			return errMsg{err: fmt.Errorf("failed to close database before backup: %w", err)}
		}
		backupPath, backupErr := backup.CreateManualBackup(dbPath)

		// The app's handle is closed either way, so it must get a fresh one.
		newDB, openErr := db.Open(dbPath)
		if openErr != nil {
			return errMsg{err: fmt.Errorf("failed to reopen database after backup: %w — restart tmoney", openErr)}
		}
		if backupErr != nil {
			return reopenedAfterFailureMsg{db: newDB, err: fmt.Errorf("failed to create backup: %w", backupErr)}
		}
		return backupCreatedMsg{path: backupPath, db: newDB}
	}
}

// createAutoBackupOnQuit creates an auto-backup as the TUI exits. It closes the
// database first, because a copy of an open DuckDB file is incomplete (WAL) or
// impossible (Windows); see db.DB.Close. The caller's later Close is a no-op.
// This is best-effort; errors are silently ignored because the TUI is already
// tearing down and has no surface to report them on.
func createAutoBackupOnQuit(database *db.DB) {
	if err := database.Close(); err != nil {
		return
	}
	_, _ = backup.CreateAutoBackup(database.Path())
}

// backupDialogState holds the state for the restore backup dialog.
type backupDialogState struct {
	dialog  *dialog.Dialog
	backups []backup.BackupInfo
}

// handleBackupDialogKey handles key input for the backup restore dialog.
func (a *App) handleBackupDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.backupDialog == nil {
		return a, nil
	}

	action := a.backupDialog.dialog.HandleKey(msg)
	switch action {
	case dialog.DialogActionSubmit:
		return a.submitBackupDialog()
	case dialog.DialogActionCancel:
		a.backupDialog = nil
		return a, nil
	}

	return a, nil
}

// submitBackupDialog handles the restore backup dialog submission.
func (a *App) submitBackupDialog() (tea.Model, tea.Cmd) {
	if a.backupDialog == nil {
		return a, nil
	}

	fields := a.backupDialog.dialog.Fields()
	if len(fields) < 1 {
		return a, nil
	}

	selected := fields[0].SelectedOption()
	if selected == "" || selected == "(no backups available)" {
		a.backupDialog = nil
		return a, nil
	}

	// Find the selected backup index
	selectedIndex := fields[0].SelectedIndex
	if selectedIndex < 0 || selectedIndex >= len(a.backupDialog.backups) {
		a.backupDialog = nil
		return a, nil
	}

	selectedBackup := a.backupDialog.backups[selectedIndex]
	a.backupDialog = nil

	// Show confirmation dialog
	msg := fmt.Sprintf("Restore from backup dated %s?\nCurrent data will be overwritten.\nA backup of the current state will be created first.",
		selectedBackup.Timestamp.Format("2006-01-02 15:04:05"))
	backupPath := selectedBackup.Path
	current := a.db
	a.showConfirmDialog("Confirm Restore", msg, func() tea.Msg {
		dbPath := current.Path()

		// Close the live database BEFORE touching the file. Restore renames the
		// current file aside and moves the backup into its place; with the file
		// still open that rename fails on Windows, and elsewhere DuckDB's
		// per-path instance cache makes the later db.Open return the old,
		// still-open instance instead of reading the restored file. Closing
		// also makes the safety backup Restore takes complete (see db.DB.Close).
		// A goroutine still reading the old handle gets "database is closed",
		// not a panic.
		if err := current.Close(); err != nil {
			return errMsg{err: fmt.Errorf("failed to close database before restore: %w", err)}
		}

		safetyPath, restoreErr := backup.Restore(dbPath, backupPath)

		// Reopen whatever is now at dbPath: the restored file on success, the
		// untouched original on failure (Restore rolls its own rename back).
		// Either way the app's current handle is closed and must be replaced.
		newDB, openErr := db.Open(dbPath)
		if openErr != nil {
			if restoreErr != nil {
				return errMsg{err: fmt.Errorf("failed to restore: %w; and the database could not be reopened: %v — restart tmoney", restoreErr, openErr)}
			}
			return errMsg{err: fmt.Errorf("restored, but failed to reopen the database: %w — restart tmoney", openErr)}
		}
		if restoreErr != nil {
			return reopenedAfterFailureMsg{db: newDB, err: fmt.Errorf("failed to restore: %w", restoreErr)}
		}

		return restoreConfirmedMsg{
			db:               newDB,
			safetyBackupPath: safetyPath,
		}
	})

	return a, nil
}

// backupFilename extracts just the filename from a backup path for display.
func backupFilename(path string) string {
	return filepath.Base(path)
}
