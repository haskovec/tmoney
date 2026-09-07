package tui

import (
	"fmt"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/tui/dialog"
)

// backupCreatedMsg is sent when a manual backup has been created.
type backupCreatedMsg struct {
	path string
}

// restoreConfirmedMsg is sent when a restore is complete. The database handle
// is unchanged (see db.DB.WithFileClosed); only its contents are new.
type restoreConfirmedMsg struct {
	safetyBackupPath string
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

// createManualBackupCmd returns a command that creates a manual backup. The
// copy runs with the database closed and the same handle reopened after (see
// db.DB.WithFileClosed), so services, undo and the current view stay valid.
func (a *App) createManualBackupCmd() tea.Cmd {
	database := a.db
	return func() tea.Msg {
		var backupPath string
		err := database.WithFileClosed(func(path string) error {
			var err error
			backupPath, err = backup.CreateManualBackup(path)
			return err
		})
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to create backup: %w", err)}
		}
		return backupCreatedMsg{path: backupPath}
	}
}

// createAutoBackupOnQuit creates an auto-backup as the TUI exits. It closes the
// database first (see db.DB.Close); the caller's later Close is a no-op. Errors
// are ignored: the TUI is tearing down and has no surface to report them on.
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
	return a.backupDialogAction(a.backupDialog.dialog.HandleKey(msg))
}

// backupDialogAction dispatches a DialogAction for the backup dialog, from either input path.
func (a *App) backupDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
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
	database := a.db
	a.showConfirmDialog("Confirm Restore", msg, func() tea.Msg {
		// The file is swapped with the database closed and the same handle
		// reopened after (see db.DB.WithFileClosed). On failure Restore has put
		// the original file back, so the reopened handle is the untouched data.
		var safetyPath string
		err := database.WithFileClosed(func(path string) error {
			var err error
			safetyPath, err = backup.Restore(path, backupPath)
			return err
		})
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to restore: %w", err)}
		}
		return restoreConfirmedMsg{safetyBackupPath: safetyPath}
	})

	return a, nil
}

// reloadAfterRestore discards everything derived from the old file contents —
// cached view data and the undo history, whose commands describe rows that no
// longer exist — and reloads from the restored data. The database handle and
// the services stay as they are.
func (a *App) reloadAfterRestore() (tea.Model, tea.Cmd) {
	a.undoManager.Clear()

	a.dashboard = nil
	a.register = nil
	a.table = nil
	a.scheduled = nil
	a.scheduledTable = nil
	a.reports = nil
	a.securityView = nil
	a.securityTable = nil
	a.priceView = nil
	a.priceTable = nil
	a.investmentRegister = nil
	a.investmentTable = nil
	a.resetInvestmentRegisterFilter()
	a.portfolioData = nil
	a.portfolioHoldingsTable = nil
	a.portfolioLotsTable = nil

	a.switchView(ViewDashboard)
	a.updateStatusBar()

	return a, tea.Batch(
		a.loadSidebarData(),
		a.loadScheduledDueCount(),
		a.loadDashboardData(),
	)
}

// backupFilename extracts just the filename from a backup path for display.
func backupFilename(path string) string {
	return filepath.Base(path)
}
