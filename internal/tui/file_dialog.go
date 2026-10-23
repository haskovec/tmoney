package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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

// fileSurface is the Open / Save As / browse file dialog together with the form state that
// belongs to it. Its zero value is closed; closeFileDialog resets it to that.
type fileSurface struct {
	modalSurface
	mode      fileDialogMode
	browseDir string
	clicks    *widget.ClickTracker // double-click tracker for the browse list
}

func (s *fileSurface) IsVisible() bool { return s != nil && s.dlg.IsVisible() }

// closeFileDialog clears the file dialog state.
func (a *App) closeFileDialog() {
	a.file = fileSurface{}
}

// handleFileDialogMouse adds the browse-mode double-click before the ordinary
// action dispatch: a double-click on a list row activates that entry (navigate
// into a directory, or open a .tdb file) without a separate Open press.
func (a *App) handleFileDialogMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	listItemRow := -1
	if click, ok := msg.(tea.MouseClickMsg); ok &&
		a.file.mode == fileDialogModeBrowse &&
		click.Button == tea.MouseLeft {
		listItemRow = a.browseDialogListHit(msg)
	}

	action := a.file.dlg.HandleMouse(msg, a.width, a.height)

	if listItemRow >= 0 {
		if a.file.clicks == nil {
			a.file.clicks = widget.NewClickTracker(widget.DoubleClickThreshold)
		}
		if a.file.clicks.Click(listItemRow) {
			return a.submitFileDialog()
		}
	}

	return a.fileDialogAction(action)
}

// handleFileDialogKey routes key events to the file dialog.
func (a *App) handleFileDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.file.dlg == nil {
		return a, nil
	}
	return a.fileDialogAction(a.file.dlg.HandleKey(msg))
}

// fileDialogAction dispatches a DialogAction for the file dialog, from either input path.
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
	if a.file.dlg == nil {
		return a, nil
	}

	mode := a.file.mode
	fields := a.file.dlg.Fields()

	a.file.dlg.ClearErrors()

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
			parent := filepath.Dir(a.file.browseDir)
			a.openBrowseDialog(parent)
			return a, nil
		}
		if dirName, ok := strings.CutSuffix(selected, "/"); ok {
			// Navigate into subdirectory
			subdir := filepath.Join(a.file.browseDir, dirName)
			a.openBrowseDialog(subdir)
			return a, nil
		}
		// It's a .tdb file — open it
		fullPath := filepath.Join(a.file.browseDir, selected)
		a.closeFileDialog()
		return a, a.submitOpenFile(fullPath)
	}

	return a, nil
}

// fileOpFailedMsg is sent when File > New or Open of path fails.
type fileOpFailedMsg struct {
	path string
	err  error
}

// startFileOperation records that a File > New or Open of path starts. When one
// on the same file is still in flight it reports false and tells the user to
// wait: two opens of one file gave two handles on it (see submitOpenFile), and
// two creates of one path let the loser delete the file that the winner uses.
// Operations on other files still run, so an open that never returns (a share
// that hangs) blocks only its own file. The message of the command ends the
// operation (endFileOperation).
func (a *App) startFileOperation(path string) bool {
	for _, busy := range a.filesInFlight {
		if sameTarget(busy, path) {
			a.statusbar.AddNotification(
				fmt.Sprintf("Wait: %s is still opening", filepath.Base(busy)),
				widget.NotificationInfo,
			)
			return false
		}
	}
	a.filesInFlight = append(a.filesInFlight, path)
	return true
}

// endFileOperation records that the File > New or Open of path reported.
func (a *App) endFileOperation(path string) {
	for i, busy := range a.filesInFlight {
		if busy == path {
			a.filesInFlight = append(a.filesInFlight[:i], a.filesInFlight[i+1:]...)
			return
		}
	}
}

// sameTarget reports whether paths a and b name one file, also one that does
// not exist yet (File > New).
func sameTarget(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	return (errA == nil && errB == nil && absA == absB) || sameFile(a, b)
}

// submitNewFile returns a command that creates a new database file, or nil
// while another file operation is in flight (see startFileOperation).
func (a *App) submitNewFile(path string) tea.Cmd {
	if !a.startFileOperation(path) {
		return nil
	}
	return func() tea.Msg {
		// Create the directory if needed
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fileOpFailedMsg{path: path, err: fmt.Errorf("failed to create directory: %w", err)}
		}

		database, err := db.Create(path)
		if err != nil {
			return fileOpFailedMsg{path: path, err: fmt.Errorf("failed to create database: %w", err)}
		}

		return fileDialogSavedMsg{db: database, path: path}
	}
}

// fileAlreadyOpenMsg is sent when File > Open names the file of db, the open
// database when the command started.
type fileAlreadyOpenMsg struct {
	path string
	db   *db.DB
}

// submitOpenFile returns a command that opens an existing database file, or nil
// while another file operation is in flight (see startFileOperation).
//
// It never opens a second handle on a file this app has open. DuckDB caches one
// instance per path, so the second handle shares the first one's instance: a
// backup or restore through it closes only its own pool, the instance lives on,
// and the backup misses the rows still in <path>.wal. The open file stays as it
// is, and the kept handle of the previous file (prevDB) closes first.
func (a *App) submitOpenFile(path string) tea.Cmd {
	if !a.startFileOperation(path) {
		return nil
	}
	current, prev := a.db, a.prevDB
	return func() tea.Msg {
		if current != nil && sameFile(path, current.Path()) {
			return fileAlreadyOpenMsg{path: path, db: current}
		}
		if prev != nil && sameFile(path, prev.Path()) {
			_ = prev.Close() // idempotent; switchDatabase closes it again
		}
		database, err := db.Open(path)
		if err != nil {
			return fileOpFailedMsg{path: path, err: fmt.Errorf("failed to open database: %w", err)}
		}

		return fileDialogSavedMsg{db: database, path: path}
	}
}

// alreadyOpen handles the report that File > Open named the open file. When
// another open switched the database in the meantime, the file is not the open
// one any more, so the open runs again against the new state.
func (a *App) alreadyOpen(msg fileAlreadyOpenMsg) tea.Cmd {
	a.endFileOperation(msg.path)
	if msg.db != a.db {
		return a.submitOpenFile(msg.path)
	}
	a.statusbar.AddNotification(
		fmt.Sprintf("%s is already open", filepath.Base(msg.path)),
		widget.NotificationInfo,
	)
	return nil
}

// sameFile reports whether a and b name the same file, also through a symlink
// or another spelling of the path. It is false if either one cannot be read.
func sameFile(a, b string) bool {
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(infoA, infoB)
}

// switchDatabase closes the old database, sets the new one, reinitializes
// services, clears cached view data, and returns commands to reload everything.
func (a *App) switchDatabase(newDB *db.DB) (tea.Model, tea.Cmd) {
	// Close the previously deferred database (from an earlier switch).
	// The current a.db is kept alive as prevDB so that any in-flight
	// goroutines from dashboard.load/loadSidebarData/etc. that still
	// hold service references to it won't panic on a nil *sql.DB conn.
	if a.prevDB != nil {
		_ = a.prevDB.Close()
	}
	a.prevDB = a.db

	// Set new database and reinitialize ALL services
	a.db = newDB
	svc, prepareErr := newTUIServices(newDB)
	a.services = *svc
	// A repair failure belongs to the file it came from.
	a.statusbar.ClearSticky()
	if prepareErr != nil {
		a.surfacePrepareError(prepareErr)
	}

	// The undo history describes rows in the file we just left, and every
	// command on it captured the OLD services when it was built. Undoing one
	// after the switch would write to a closed database. Clear it, exactly as
	// reloadAfterRestore does when the rows change underneath it.
	a.undoManager.Clear()

	// Clear all cached view data
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
		a.dashboard.load(a.dashboardDeps()),
	)
}

// nearestExistingDir returns dir when it is a directory, else its nearest
// parent that is one. It returns the filesystem root when nothing on the path
// exists, and listDirectoryEntries then reports the real error.
func nearestExistingDir(dir string) string {
	for {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
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
	// Cut a long path from the left, in cells, so its end stays in view. The
	// title shares its row with a 1-cell gap and the [x] close button.
	const width = 60
	const prefix = "Open File — "
	maxTitle := width - dialog.DialogHorizontalOverhead - 4
	shown := dialog.OneLine(dir) // measure a tab or a line break as the space it draws as
	title := prefix + shown
	if over := lipgloss.Width(title) - maxTitle; over > 0 {
		// TruncateLeft keeps a wide character that the cut falls inside, so
		// cut one more cell when the title is still too wide.
		for cut := over + 1; lipgloss.Width(title) > maxTitle; cut++ {
			title = prefix + ansi.TruncateLeft(shown, cut, "…")
		}
	}

	d := dialog.NewDialog(title)
	d.SetWidth(width)
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
	if a.file.dlg == nil || !a.file.dlg.IsVisible() {
		return -1
	}
	d := a.file.dlg
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
// A directory that does not exist (a new install has no ~/Documents/TMoney
// yet) gives way to its nearest parent that does, so the browser opens and
// not the error page.
func (a *App) openBrowseDialog(dir string) {
	dir = nearestExistingDir(dir)
	entries, err := listDirectoryEntries(dir)
	if err != nil {
		a.err = fmt.Errorf("failed to read directory: %w", err)
		return
	}

	a.file.browseDir = dir
	a.file.mode = fileDialogModeBrowse
	a.file.dlg = buildBrowseDialog(dir, entries)
}
