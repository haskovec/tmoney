package tui

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/db"
)

// backupCreatedMsg is sent when a manual backup has been created.
type backupCreatedMsg struct {
	path string
}

// restoreConfirmedMsg is sent when the restore+reopen is complete.
type restoreConfirmedMsg struct {
	db               *db.DB
	safetyBackupPath string
}

// buildRestoreBackupDialog creates a dialog for selecting a backup to restore from.
func buildRestoreBackupDialog(dbPath string) (*Dialog, []backup.BackupInfo, error) {
	backups, err := backup.ListBackups(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list backups: %w", err)
	}

	d := NewDialog("Restore from Backup")

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
	dbPath := a.db.Path()
	return func() tea.Msg {
		backupPath, err := backup.CreateManualBackup(dbPath)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to create backup: %w", err)}
		}
		return backupCreatedMsg{path: backupPath}
	}
}

// createAutoBackupOnQuit creates an auto-backup before the TUI exits.
// This is best-effort; errors are silently ignored.
func createAutoBackupOnQuit(dbPath string) {
	backup.CreateAutoBackup(dbPath)
}

// backupDialogState holds the state for the restore backup dialog.
type backupDialogState struct {
	dialog  *Dialog
	backups []backup.BackupInfo
}

// handleBackupDialogKey handles key input for the backup restore dialog.
func (a *App) handleBackupDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.backupDialog == nil {
		return a, nil
	}

	action := a.backupDialog.dialog.HandleKey(msg)
	switch action {
	case DialogActionSubmit:
		return a.submitBackupDialog()
	case DialogActionCancel:
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
	a.showConfirmDialog("Confirm Restore", msg, func() tea.Msg {
		safetyPath, err := backup.Restore(a.db.Path(), backupPath)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to restore: %w", err)}
		}

		dbPath := a.db.Path()
		newDB, err := db.Open(dbPath)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to reopen database after restore: %w", err)}
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
