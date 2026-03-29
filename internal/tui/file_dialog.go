package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/db"
)

// fileDialogMode indicates which file dialog is active.
type fileDialogMode int

const (
	fileDialogModeNew fileDialogMode = iota
	fileDialogModeOpen
	fileDialogModeOpenRecent
)

// fileDialogSavedMsg is sent when a file operation completes with a new database.
type fileDialogSavedMsg struct {
	db   *db.DB
	path string
}

// File dialog field indices.
const (
	fileFieldPath = 0
)

// buildNewFileDialog creates a dialog for creating a new database file.
func buildNewFileDialog() *Dialog {
	d := NewDialog("New File")

	defaultPath := filepath.Join(db.DefaultDirectory(), "new.tdb")
	f := d.AddTextField("Path", defaultPath, "Path to new .tdb file", 0)
	f.Required = true

	d.SetVisible(true)
	return d
}

// buildOpenFileDialog creates a dialog for opening an existing database file.
func buildOpenFileDialog() *Dialog {
	d := NewDialog("Open File")

	defaultPath := filepath.Join(db.DefaultDirectory(), "")
	f := d.AddTextField("Path", defaultPath, "Path to .tdb file", 0)
	f.Required = true

	d.SetVisible(true)
	return d
}

// buildOpenRecentDialog creates a dialog for selecting from recent files.
func buildOpenRecentDialog(recentFiles []string) *Dialog {
	d := NewDialog("Open Recent")

	if len(recentFiles) == 0 {
		d.AddSelectField("File", []string{"(no recent files)"}, 0)
	} else {
		options := make([]string, len(recentFiles))
		copy(options, recentFiles)
		d.AddSelectField("File", options, 0)
	}

	d.SetVisible(true)
	return d
}

// closeFileDialog clears the file dialog state.
func (a *App) closeFileDialog() {
	a.fileDialog = nil
}

// handleFileDialogKey routes key events to the file dialog.
func (a *App) handleFileDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.fileDialog == nil {
		return a, nil
	}

	action := a.fileDialog.HandleKey(msg)
	switch action {
	case DialogActionSubmit:
		return a.submitFileDialog()
	case DialogActionCancel:
		a.closeFileDialog()
		return a, nil
	}

	return a, nil
}

// submitFileDialog dispatches the appropriate submit handler based on dialog mode.
func (a *App) submitFileDialog() (tea.Model, tea.Cmd) {
	if a.fileDialog == nil {
		return a, nil
	}

	mode := a.fileDialogMode
	fields := a.fileDialog.Fields()

	a.fileDialog.ClearErrors()

	switch mode {
	case fileDialogModeNew:
		if len(fields) < 1 {
			return a, nil
		}
		path := strings.TrimSpace(fields[fileFieldPath].Value)
		if path == "" {
			fields[fileFieldPath].Error = "File path is required"
			return a, nil
		}
		a.closeFileDialog()
		return a, a.submitNewFile(path)

	case fileDialogModeOpen:
		if len(fields) < 1 {
			return a, nil
		}
		path := strings.TrimSpace(fields[fileFieldPath].Value)
		if path == "" {
			fields[fileFieldPath].Error = "File path is required"
			return a, nil
		}
		a.closeFileDialog()
		return a, a.submitOpenFile(path)

	case fileDialogModeOpenRecent:
		if len(fields) < 1 {
			return a, nil
		}
		selected := fields[0].SelectedOption()
		if selected == "" || selected == "(no recent files)" {
			return a, nil
		}
		a.closeFileDialog()
		return a, a.submitOpenFile(selected)
	}

	return a, nil
}

// submitNewFile returns a command that creates a new database file.
func (a *App) submitNewFile(path string) tea.Cmd {
	return func() tea.Msg {
		// Create the directory if needed
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return errMsg{err: fmt.Errorf("failed to create directory: %w", err)}
		}

		database, err := db.Create(path)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to create database: %w", err)}
		}

		return fileDialogSavedMsg{db: database, path: path}
	}
}

// submitOpenFile returns a command that opens an existing database file.
func (a *App) submitOpenFile(path string) tea.Cmd {
	return func() tea.Msg {
		database, err := db.Open(path)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to open database: %w", err)}
		}

		return fileDialogSavedMsg{db: database, path: path}
	}
}

// switchDatabase closes the old database, sets the new one, reinitializes
// services, clears cached view data, and returns commands to reload everything.
func (a *App) switchDatabase(newDB *db.DB) (tea.Model, tea.Cmd) {
	// Close old database
	if a.db != nil {
		a.db.Close()
	}

	// Set new database and reinitialize services
	a.db = newDB
	svc := app.NewServices(newDB)
	a.accountSvc = svc.Account
	a.transactionSvc = svc.Transaction
	a.categorySvc = svc.Category
	a.payeeSvc = svc.Payee
	a.scheduledTxnSvc = svc.Scheduled
	a.reportSvc = svc.Report

	// Clear all cached view data
	a.dashboard = nil
	a.register = nil
	a.table = nil
	a.scheduled = nil
	a.scheduledTable = nil
	a.reports = nil

	// Update config
	if a.cfg != nil {
		a.cfg.AddRecentFile(newDB.Path())
		// Best-effort save; don't fail the switch on config error
		_ = a.cfg.Save()
	}

	// Switch to dashboard and reload everything
	a.switchView(ViewDashboard)
	a.updateStatusBar()

	return a, tea.Batch(
		a.loadSidebarData(),
		a.loadScheduledDueCount(),
		a.loadDashboardData(),
	)
}
