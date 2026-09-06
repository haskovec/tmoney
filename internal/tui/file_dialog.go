package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// fileDialogMode indicates which file dialog is active.
type fileDialogMode int

const (
	fileDialogModeNew fileDialogMode = iota
	fileDialogModeOpen
	fileDialogModeOpenRecent
	fileDialogModeBrowse
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
func buildNewFileDialog() *dialog.Dialog {
	d := dialog.NewDialog("New File")

	defaultPath := filepath.Join(db.DefaultDirectory(), "new.tdb")
	f := d.AddTextField("Path", defaultPath, "Path to new .tdb file", 0)
	f.Required = true

	d.SetButtons([]dialog.DialogButton{
		{Label: "Create", Primary: true},
		{Label: "Cancel"},
	})
	d.SetVisible(true)
	return d
}

// buildOpenFileDialog creates a dialog for opening an existing database file.
func buildOpenFileDialog() *dialog.Dialog {
	d := dialog.NewDialog("Open File")

	defaultPath := filepath.Join(db.DefaultDirectory(), "")
	f := d.AddTextField("Path", defaultPath, "Path to .tdb file", 0)
	f.Required = true

	d.SetButtons([]dialog.DialogButton{
		{Label: "Open", Primary: true},
		{Label: "Cancel"},
	})
	d.SetVisible(true)
	return d
}

// buildOpenRecentDialog creates a dialog for selecting from recent files.
func buildOpenRecentDialog(recentFiles []string) *dialog.Dialog {
	d := dialog.NewDialog("Open Recent")

	if len(recentFiles) == 0 {
		d.AddSelectField("File", []string{"(no recent files)"}, 0)
	} else {
		options := make([]string, len(recentFiles))
		copy(options, recentFiles)
		d.AddSelectField("File", options, 0)
	}

	d.SetButtons([]dialog.DialogButton{
		{Label: "Open", Primary: true},
		{Label: "Cancel"},
	})
	d.SetVisible(true)
	return d
}

// closeFileDialog clears the file dialog state.
func (a *App) closeFileDialog() {
	a.fileDialog = nil
}

// handleFileDialogMouse adds the browse-mode double-click before the ordinary
// action dispatch: a double-click on a list row activates that entry (navigate
// into a directory, or open a .tdb file) without a separate Open press.
func (a *App) handleFileDialogMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	listItemRow := -1
	if click, ok := msg.(tea.MouseClickMsg); ok &&
		a.fileDialogMode == fileDialogModeBrowse &&
		click.Button == tea.MouseLeft {
		listItemRow = a.browseDialogListHit(msg)
	}

	action := a.fileDialog.HandleMouse(msg, a.width, a.height)

	if listItemRow >= 0 {
		if a.browseDialogClicks == nil {
			a.browseDialogClicks = widget.NewClickTracker(widget.DoubleClickThreshold)
		}
		if a.browseDialogClicks.Click(listItemRow) {
			return a.submitFileDialog()
		}
	}

	return a.fileDialogAction(action)
}

// handleFileDialogKey routes key events to the file dialog.
func (a *App) handleFileDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.fileDialog == nil {
		return a, nil
	}
	return a.fileDialogAction(a.fileDialog.HandleKey(msg))
}

// fileDialogAction dispatches a DialogAction for the file dialog. Both the keyboard
// and the mouse path call it, so clicking a button is exactly equivalent to
// the keyboard action -- the rule specs/tui.md states and the two hand-kept
// switches used to break.
func (a *App) fileDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionSubmit:
		return a.submitFileDialog()
	case dialog.DialogActionCancel:
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

	case fileDialogModeBrowse:
		if len(fields) < 1 {
			return a, nil
		}
		selected := fields[0].SelectedOption()
		if selected == "" || selected == "(empty directory)" {
			return a, nil
		}
		if selected == "../" {
			// Navigate to parent directory
			parent := filepath.Dir(a.browseDir)
			a.openBrowseDialog(parent)
			return a, nil
		}
		if dirName, ok := strings.CutSuffix(selected, "/"); ok {
			// Navigate into subdirectory
			subdir := filepath.Join(a.browseDir, dirName)
			a.openBrowseDialog(subdir)
			return a, nil
		}
		// It's a .tdb file — open it
		fullPath := filepath.Join(a.browseDir, selected)
		a.closeFileDialog()
		return a, a.submitOpenFile(fullPath)
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
	// Close the previously deferred database (from an earlier switch).
	// The current a.db is kept alive as prevDB so that any in-flight
	// goroutines from loadDashboardData/loadSidebarData/etc. that still
	// hold service references to it won't panic on a nil *sql.DB conn.
	if a.prevDB != nil {
		_ = a.prevDB.Close()
	}
	a.prevDB = a.db

	// Set new database and reinitialize ALL services
	a.db = newDB
	svc := newTUIServices(newDB)
	a.accountSvc = svc.Account
	a.transactionSvc = svc.Transaction
	a.categorySvc = svc.Category
	a.payeeSvc = svc.Payee
	a.scheduledTxnSvc = svc.Scheduled
	a.reportSvc = svc.Report
	a.reconciliationSvc = svc.Reconciliation
	a.securitySvc = svc.Security
	a.priceSvc = svc.Price
	a.investmentSvc = svc.Investment
	a.investmentValuationSvc = svc.InvestmentValuation
	a.investmentEditSvc = svc.InvestmentEdit
	a.investmentRepo = svc.InvestmentRepo
	a.corporateActionSvc = svc.CorporateAction
	a.transferSvc = svc.Transfer
	a.lotRepo = svc.LotRepo
	a.positionRepo = svc.PositionRepo

	// Clear all cached view data
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

// listDirectoryEntries returns entries for a file browser dialog.
// Returns sorted: "../" first, then directories (with "/" suffix), then .tdb files.
// Hidden files/directories (starting with ".") are excluded.
func listDirectoryEntries(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var dirs []string
	var files []string

	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, name+"/")
		} else if strings.HasSuffix(strings.ToLower(name), ".tdb") {
			files = append(files, name)
		}
	}

	// Build result: ../ first, then dirs, then files
	var result []string
	result = append(result, "../")
	result = append(result, dirs...)
	result = append(result, files...)

	// If only "../" (no real entries), show empty message
	if len(result) == 1 {
		return []string{"(empty directory)"}, nil
	}

	return result, nil
}

// buildBrowseDialog creates a file browser dialog for the given directory.
func buildBrowseDialog(dir string, entries []string) *dialog.Dialog {
	// widget.Truncate long directory paths for the title
	title := "Open File — " + dir
	if len(title) > 52 {
		title = "Open File — ..." + dir[len(dir)-37:]
	}

	d := dialog.NewDialog(title)
	d.SetWidth(60)
	d.AddListField("File", entries, 0, 12)
	d.SetButtons([]dialog.DialogButton{
		{Label: "Open", Primary: true},
		{Label: "Cancel"},
	})
	d.SetVisible(true)
	return d
}

// browseDialogListHit returns the absolute list-item index a mouse event lands
// on inside the browse dialog's list field, or -1 when the click is outside the
// list (title, padding, buttons, etc.). Callers use this to drive double-click
// activation on directory entries.
func (a *App) browseDialogListHit(msg tea.MouseMsg) int {
	if a.fileDialog == nil || !a.fileDialog.IsVisible() {
		return -1
	}
	d := a.fileDialog
	startCol, startRow, endCol, endRow := d.DialogBounds(a.width, a.height)
	m := msg.Mouse()
	if m.X < startCol || m.X >= endCol || m.Y < startRow || m.Y >= endRow {
		return -1
	}
	contentWidth := max(d.Width()-dialog.DialogHorizontalOverhead, 10)
	localX := m.X - startCol - 3
	localY := m.Y - startRow - 2
	hit := d.HitTestContent(localX, localY, contentWidth)
	if hit.Zone != dialog.DialogHitField || hit.ListItemIndex < 0 {
		return -1
	}
	return hit.ListItemIndex
}

// openBrowseDialog builds and sets the browse dialog for the given directory.
func (a *App) openBrowseDialog(dir string) {
	entries, err := listDirectoryEntries(dir)
	if err != nil {
		a.err = fmt.Errorf("failed to read directory: %w", err)
		return
	}

	a.browseDir = dir
	a.fileDialogMode = fileDialogModeBrowse
	a.fileDialog = buildBrowseDialog(dir, entries)
}
