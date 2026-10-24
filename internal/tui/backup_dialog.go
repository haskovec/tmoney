package tui

import (
	"fmt"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// backupCreatedMsg is sent when a manual backup has been created.
type backupCreatedMsg struct {
	path string
}

// restoreConfirmedMsg is sent when a restore is complete. The database handle
// is unchanged (see db.DB.WithFileClosed); only its contents are new.
type restoreConfirmedMsg struct {
	// db is the handle whose file was restored. File > Open can switch to
	// another file while the restore runs.
	db               *db.DB
	safetyBackupPath string
	// reopenErr is set when the restore replaced the file but the handle did
	// not reopen on it.
	reopenErr error
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
// database first and waits for a heal in flight (see db.DB.CloseAndSettle); the
// caller's later Close is a no-op. When a reader still holds the old instance,
// the backup is skipped: the heal could open the file under the copy. Errors are
// ignored: the TUI is tearing down and has no surface to report them on.
func createAutoBackupOnQuit(database *db.DB) {
	if settled, err := database.CloseAndSettle(db.SettleTimeout); err != nil || !settled {
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
		return restoreFile(database, backupPath, backup.Restore)
	})

	return a, nil
}

// restoreFile runs restore (backup.Restore) on the file of database, with the
// file closed and the same handle reopened after (see db.DB.WithFileClosed),
// and reports the outcome to Update.
//
// A failed restore leaves the old data, and the reopened handle shows it. But
// when restore succeeded and only the reopen failed, the file is the restored
// one all the same: the message is restoreConfirmedMsg, so the session drops
// what it derived from the old file (see afterRestore). A later heal or file
// operation reopens the handle on the restored data.
func restoreFile(database *db.DB, backupPath string, restore func(dbPath, backupPath string) (string, error)) tea.Msg {
	var safetyPath string
	restored := false
	err := database.WithFileClosed(func(path string) error {
		var err error
		safetyPath, err = restore(path, backupPath)
		restored = err == nil
		return err
	})
	if err != nil && !restored {
		if safetyPath != "" { // Restore stopped after its safety backup
			return errMsg{err: fmt.Errorf("failed to restore (a backup of the current state is %s): %w", backupFilename(safetyPath), err)}
		}
		return errMsg{err: fmt.Errorf("failed to restore: %w", err)}
	}
	return restoreConfirmedMsg{db: database, safetyBackupPath: safetyPath, reopenErr: err}
}

// reloadAfterRestore discards everything derived from the old file contents —
// cached view data and the undo history, whose commands describe rows that no
// longer exist — and reloads from the restored data. The database handle and
// the services stay as they are.
func (a *App) reloadAfterRestore() (tea.Model, tea.Cmd) {
	a.undoManager.Clear()

	a.dashboard.data = nil
	a.register.data = nil
	a.register.table = nil
	a.scheduled.data = nil
	a.scheduled.table = nil
	a.reports.data = nil
	a.securities.data = nil
	a.securities.table = nil
	a.prices.data = nil
	a.prices.table = nil
	a.investmentRegister.data = nil
	a.investmentRegister.table = nil
	a.investmentRegister.resetFilter()
	a.portfolio.data = nil
	a.portfolio.holdingsTable = nil
	a.portfolio.lotsTable = nil

	a.switchView(ViewDashboard)
	a.updateStatusBar()

	return a, tea.Batch(
		a.loadSidebarData(),
		a.loadScheduledDueCount(),
		a.dashboard.load(a.dashboardDeps()),
	)
}

// backupFilename extracts just the filename from a backup path for display.
func backupFilename(path string) string {
	return filepath.Base(path)
}

// afterRestore rebuilds the app over the restored database and reports where
// the pre-restore safety copy went. When the handle did not reopen, the views
// cannot load: it reports that error, and the views load at the next refresh.
func (a *App) afterRestore(msg restoreConfirmedMsg) (tea.Model, tea.Cmd) {
	if msg.db != a.db {
		// The restored file is not the open one any more: nothing here
		// derives from it.
		a.statusbar.AddNotification(
			fmt.Sprintf("Restored %s from backup (safety backup: %s)",
				backupFilename(msg.db.Path()), backupFilename(msg.safetyBackupPath)),
			widget.NotificationInfo,
		)
		return a, nil
	}
	model, cmd := a.reloadAfterRestore()
	if msg.reopenErr != nil {
		// The loads cannot run on a closed handle, so the sidebar and the due
		// count would keep the old file's data until some later reload.
		a.sidebar.SetAccounts(nil)
		a.applyScheduledDueCount(0)
		a.err = fmt.Errorf("restored from backup (safety backup: %s), but the database did not open again: %w. Restart tmoney if it does not come back",
			backupFilename(msg.safetyBackupPath), msg.reopenErr)
		return model, nil
	}
	a.statusbar.AddNotification(
		fmt.Sprintf("Restored from backup (safety backup: %s)", backupFilename(msg.safetyBackupPath)),
		widget.NotificationInfo,
	)
	return model, cmd
}
