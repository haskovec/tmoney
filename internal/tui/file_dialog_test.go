package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// =============================================================================
// Pure Function Tests - buildNewFileDialog
// =============================================================================

func TestBuildNewFileDialog(t *testing.T) {
	t.Run("creates dialog with correct title", func(t *testing.T) {
		d := buildNewFileDialog()
		if d.Title() != "New File" {
			t.Errorf("title = %q, want 'New File'", d.Title())
		}
	})

	t.Run("is visible after creation", func(t *testing.T) {
		d := buildNewFileDialog()
		if !d.IsVisible() {
			t.Error("dialog should be visible")
		}
	})

	t.Run("has one field", func(t *testing.T) {
		d := buildNewFileDialog()
		fields := d.Fields()
		if len(fields) != 1 {
			t.Fatalf("expected 1 field, got %d", len(fields))
		}
	})

	t.Run("path field has correct label", func(t *testing.T) {
		d := buildNewFileDialog()
		fields := d.Fields()
		if fields[fileFieldPath].Label != "Path" {
			t.Errorf("label = %q, want 'Path'", fields[fileFieldPath].Label)
		}
	})

	t.Run("path field is required", func(t *testing.T) {
		d := buildNewFileDialog()
		fields := d.Fields()
		if !fields[fileFieldPath].Required {
			t.Error("Path field should be required")
		}
	})

	t.Run("path field defaults to default directory with new.tdb", func(t *testing.T) {
		d := buildNewFileDialog()
		fields := d.Fields()

		expected := filepath.Join(db.DefaultDirectory(), "new.tdb")
		if fields[fileFieldPath].Value != expected {
			t.Errorf("path default = %q, want %q", fields[fileFieldPath].Value, expected)
		}
	})

	t.Run("path field is text type", func(t *testing.T) {
		d := buildNewFileDialog()
		fields := d.Fields()
		if fields[fileFieldPath].Type != dialog.FieldText {
			t.Errorf("field type = %v, want dialog.FieldText", fields[fileFieldPath].Type)
		}
	})
}

// =============================================================================
// Pure Function Tests - buildOpenFileDialog
// =============================================================================

func TestBuildOpenFileDialog(t *testing.T) {
	t.Run("creates dialog with correct title", func(t *testing.T) {
		d := buildOpenFileDialog()
		if d.Title() != "Open File" {
			t.Errorf("title = %q, want 'Open File'", d.Title())
		}
	})

	t.Run("is visible after creation", func(t *testing.T) {
		d := buildOpenFileDialog()
		if !d.IsVisible() {
			t.Error("dialog should be visible")
		}
	})

	t.Run("has one field", func(t *testing.T) {
		d := buildOpenFileDialog()
		fields := d.Fields()
		if len(fields) != 1 {
			t.Fatalf("expected 1 field, got %d", len(fields))
		}
	})

	t.Run("path field is required", func(t *testing.T) {
		d := buildOpenFileDialog()
		fields := d.Fields()
		if !fields[fileFieldPath].Required {
			t.Error("Path field should be required")
		}
	})

	t.Run("path field defaults to default directory", func(t *testing.T) {
		d := buildOpenFileDialog()
		fields := d.Fields()

		expected := filepath.Join(db.DefaultDirectory(), "")
		if fields[fileFieldPath].Value != expected {
			t.Errorf("path default = %q, want %q", fields[fileFieldPath].Value, expected)
		}
	})
}

// =============================================================================
// Pure Function Tests - buildOpenRecentDialog
// =============================================================================

func TestBuildOpenRecentDialog(t *testing.T) {
	t.Run("creates dialog with correct title", func(t *testing.T) {
		d := buildOpenRecentDialog(nil)
		if d.Title() != "Open Recent" {
			t.Errorf("title = %q, want 'Open Recent'", d.Title())
		}
	})

	t.Run("is visible after creation", func(t *testing.T) {
		d := buildOpenRecentDialog(nil)
		if !d.IsVisible() {
			t.Error("dialog should be visible")
		}
	})

	t.Run("shows no recent files message when nil", func(t *testing.T) {
		d := buildOpenRecentDialog(nil)
		fields := d.Fields()
		if len(fields) != 1 {
			t.Fatalf("expected 1 field, got %d", len(fields))
		}
		if fields[0].Type != dialog.FieldSelect {
			t.Errorf("field type = %v, want dialog.FieldSelect", fields[0].Type)
		}
		if len(fields[0].Options) != 1 {
			t.Fatalf("expected 1 option, got %d", len(fields[0].Options))
		}
		if fields[0].Options[0] != "(no recent files)" {
			t.Errorf("option = %q, want '(no recent files)'", fields[0].Options[0])
		}
	})

	t.Run("shows no recent files for empty slice", func(t *testing.T) {
		d := buildOpenRecentDialog([]string{})
		fields := d.Fields()
		if len(fields[0].Options) != 1 || fields[0].Options[0] != "(no recent files)" {
			t.Errorf("expected '(no recent files)', got %v", fields[0].Options)
		}
	})

	t.Run("populates recent files", func(t *testing.T) {
		files := []string{"/path/to/file1.tdb", "/path/to/file2.tdb", "/path/to/file3.tdb"}
		d := buildOpenRecentDialog(files)
		fields := d.Fields()

		if len(fields[0].Options) != 3 {
			t.Fatalf("expected 3 options, got %d", len(fields[0].Options))
		}
		for i, file := range files {
			if fields[0].Options[i] != file {
				t.Errorf("option[%d] = %q, want %q", i, fields[0].Options[i], file)
			}
		}
	})

	t.Run("defaults to first file selected", func(t *testing.T) {
		files := []string{"/path/to/file1.tdb", "/path/to/file2.tdb"}
		d := buildOpenRecentDialog(files)
		fields := d.Fields()

		if fields[0].SelectedIndex != 0 {
			t.Errorf("selectedIndex = %d, want 0", fields[0].SelectedIndex)
		}
	})
}

// =============================================================================
// App Integration Tests - File dialog.Dialog
// =============================================================================

func TestApp_CloseFileDialog(t *testing.T) {
	app := &App{
		fileDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New File")
			d.SetVisible(true)
			return d
		}(),
		fileDialogMode: fileDialogModeNew,
	}

	app.closeFileDialog()

	if app.fileDialog != nil {
		t.Error("dialog should be nil after close")
	}
}

func TestApp_HandleFileDialogKey_Cancel(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		fileDialog: func() *dialog.Dialog {
			d := buildNewFileDialog()
			return d
		}(),
		fileDialogMode: fileDialogModeNew,
	}

	escKey := tea.KeyPressMsg{Code: tea.KeyEsc}
	model, _ := app.Update(escKey)
	updatedApp := model.(*App)

	if updatedApp.fileDialog != nil {
		t.Error("file dialog should be nil after cancel")
	}
}

func TestApp_HandleFileDialogKey_NilDialog(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	model, cmd := app.handleFileDialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model != app {
		t.Error("should return same app")
	}
	if cmd != nil {
		t.Error("should return nil cmd")
	}
}

func TestApp_SubmitFileDialog_NilDialog(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	model, cmd := app.submitFileDialog()
	if model != app {
		t.Error("should return same app")
	}
	if cmd != nil {
		t.Error("should return nil cmd")
	}
}

func TestApp_SubmitFileDialog_NewFile_EmptyPath(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		fileDialog: func() *dialog.Dialog {
			d := buildNewFileDialog()
			d.Fields()[fileFieldPath].Value = ""
			return d
		}(),
		fileDialogMode: fileDialogModeNew,
	}

	_, cmd := app.submitFileDialog()
	if cmd != nil {
		t.Error("empty path should not return a cmd")
	}
	if app.fileDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.fileDialog.Fields()[fileFieldPath].Error == "" {
		t.Error("path field should have error")
	}
}

func TestApp_SubmitFileDialog_NewFile_WhitespacePath(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		fileDialog: func() *dialog.Dialog {
			d := buildNewFileDialog()
			d.Fields()[fileFieldPath].Value = "   "
			return d
		}(),
		fileDialogMode: fileDialogModeNew,
	}

	_, cmd := app.submitFileDialog()
	if cmd != nil {
		t.Error("whitespace path should not return a cmd")
	}
	if app.fileDialog == nil {
		t.Fatal("dialog should remain open")
	}
	if app.fileDialog.Fields()[fileFieldPath].Error == "" {
		t.Error("path field should have error for whitespace-only path")
	}
}

func TestApp_SubmitFileDialog_NewFile_ValidPath(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		fileDialog: func() *dialog.Dialog {
			d := buildNewFileDialog()
			d.Fields()[fileFieldPath].Value = "/tmp/test-tmoney.tdb"
			return d
		}(),
		fileDialogMode: fileDialogModeNew,
	}

	_, cmd := app.submitFileDialog()
	if cmd == nil {
		t.Error("valid new file path should return a non-nil cmd")
	}
	if app.fileDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
}

func TestApp_SubmitFileDialog_OpenFile_EmptyPath(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		fileDialog: func() *dialog.Dialog {
			d := buildOpenFileDialog()
			d.Fields()[fileFieldPath].Value = ""
			return d
		}(),
		fileDialogMode: fileDialogModeOpen,
	}

	_, cmd := app.submitFileDialog()
	if cmd != nil {
		t.Error("empty path should not return a cmd")
	}
	if app.fileDialog == nil {
		t.Fatal("dialog should remain open")
	}
	if app.fileDialog.Fields()[fileFieldPath].Error == "" {
		t.Error("path field should have error")
	}
}

func TestApp_SubmitFileDialog_OpenFile_ValidPath(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		fileDialog: func() *dialog.Dialog {
			d := buildOpenFileDialog()
			d.Fields()[fileFieldPath].Value = "/tmp/existing.tdb"
			return d
		}(),
		fileDialogMode: fileDialogModeOpen,
	}

	_, cmd := app.submitFileDialog()
	if cmd == nil {
		t.Error("valid open file path should return a non-nil cmd")
	}
	if app.fileDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
}

func TestApp_SubmitFileDialog_OpenRecent_NoRecentFiles(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		fileDialog: func() *dialog.Dialog {
			d := buildOpenRecentDialog(nil)
			return d
		}(),
		fileDialogMode: fileDialogModeOpenRecent,
	}

	_, cmd := app.submitFileDialog()
	if cmd != nil {
		t.Error("selecting '(no recent files)' should not return a cmd")
	}
}

func TestApp_SubmitFileDialog_OpenRecent_ValidSelection(t *testing.T) {
	files := []string{"/path/to/file1.tdb", "/path/to/file2.tdb"}
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		fileDialog: func() *dialog.Dialog {
			d := buildOpenRecentDialog(files)
			return d
		}(),
		fileDialogMode: fileDialogModeOpenRecent,
	}

	_, cmd := app.submitFileDialog()
	if cmd == nil {
		t.Error("valid recent file selection should return a non-nil cmd")
	}
	if app.fileDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
}

func TestApp_RenderLayout_WithFileDialog(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewDashboard,
		width:       100,
		height:      30,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		keys:        defaultKeyMap(),
		fileDialog:  buildNewFileDialog(),
	}

	output := app.renderLayout()
	if !strings.Contains(output, "New File") {
		t.Error("renderLayout() should contain 'New File' when file dialog is visible")
	}
}

func TestApp_HandleMenuAction_NewFile(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	app.handleMenuAction(widget.MenuActionNewFile, "")

	if app.fileDialog == nil {
		t.Error("widget.MenuActionNewFile should open the file dialog")
	}
	if app.fileDialogMode != fileDialogModeNew {
		t.Errorf("fileDialogMode = %d, want fileDialogModeNew", app.fileDialogMode)
	}
}

func TestApp_HandleMenuAction_OpenFile(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	app.handleMenuAction(widget.MenuActionOpenFile, "")

	if app.fileDialog == nil {
		t.Error("widget.MenuActionOpenFile should open the file dialog")
	}
	if app.fileDialogMode != fileDialogModeBrowse {
		t.Errorf("fileDialogMode = %d, want fileDialogModeBrowse", app.fileDialogMode)
	}
}

func TestApp_HandleMenuAction_OpenRecent(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	app.handleMenuAction(widget.MenuActionOpenRecent, "")

	if app.fileDialog == nil {
		t.Error("widget.MenuActionOpenRecent should open the file dialog")
	}
	if app.fileDialogMode != fileDialogModeOpenRecent {
		t.Errorf("fileDialogMode = %d, want fileDialogModeOpenRecent", app.fileDialogMode)
	}
}

// =============================================================================
// Button Label Tests
// =============================================================================

func TestBuildNewFileDialog_ButtonLabel(t *testing.T) {
	d := buildNewFileDialog()
	buttons := d.Buttons()
	if len(buttons) < 2 {
		t.Fatalf("expected at least 2 buttons, got %d", len(buttons))
	}
	if buttons[0].Label != "Create" {
		t.Errorf("primary button label = %q, want 'Create'", buttons[0].Label)
	}
}

func TestBuildOpenFileDialog_ButtonLabel(t *testing.T) {
	d := buildOpenFileDialog()
	buttons := d.Buttons()
	if len(buttons) < 2 {
		t.Fatalf("expected at least 2 buttons, got %d", len(buttons))
	}
	if buttons[0].Label != "Open" {
		t.Errorf("primary button label = %q, want 'Open'", buttons[0].Label)
	}
}

func TestBuildOpenRecentDialog_ButtonLabel(t *testing.T) {
	d := buildOpenRecentDialog([]string{"/a.tdb"})
	buttons := d.Buttons()
	if len(buttons) < 2 {
		t.Fatalf("expected at least 2 buttons, got %d", len(buttons))
	}
	if buttons[0].Label != "Open" {
		t.Errorf("primary button label = %q, want 'Open'", buttons[0].Label)
	}
}

// =============================================================================
// listDirectoryEntries Tests
// =============================================================================

func TestListDirectoryEntries(t *testing.T) {
	dir := t.TempDir()

	// Create .tdb files
	os.WriteFile(filepath.Join(dir, "finances.tdb"), []byte{}, 0644)
	os.WriteFile(filepath.Join(dir, "personal.tdb"), []byte{}, 0644)

	// Create non-.tdb files (should be excluded)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte{}, 0644)
	os.WriteFile(filepath.Join(dir, "data.csv"), []byte{}, 0644)

	// Create subdirectory
	os.Mkdir(filepath.Join(dir, "backups"), 0755)

	entries, err := listDirectoryEntries(dir)
	if err != nil {
		t.Fatalf("listDirectoryEntries() error: %v", err)
	}

	// Expected: ../, backups/, finances.tdb, personal.tdb
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d: %v", len(entries), entries)
	}
	if entries[0] != "../" {
		t.Errorf("entries[0] = %q, want '../'", entries[0])
	}
	if entries[1] != "backups/" {
		t.Errorf("entries[1] = %q, want 'backups/'", entries[1])
	}
	if entries[2] != "finances.tdb" {
		t.Errorf("entries[2] = %q, want 'finances.tdb'", entries[2])
	}
	if entries[3] != "personal.tdb" {
		t.Errorf("entries[3] = %q, want 'personal.tdb'", entries[3])
	}
}

func TestListDirectoryEntries_Empty(t *testing.T) {
	dir := t.TempDir()

	entries, err := listDirectoryEntries(dir)
	if err != nil {
		t.Fatalf("listDirectoryEntries() error: %v", err)
	}
	if len(entries) != 1 || entries[0] != "(empty directory)" {
		t.Errorf("expected ['(empty directory)'], got %v", entries)
	}
}

func TestListDirectoryEntries_HiddenFiles(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, ".hidden.tdb"), []byte{}, 0644)
	os.Mkdir(filepath.Join(dir, ".hidden_dir"), 0755)
	os.WriteFile(filepath.Join(dir, "visible.tdb"), []byte{}, 0644)

	entries, err := listDirectoryEntries(dir)
	if err != nil {
		t.Fatalf("listDirectoryEntries() error: %v", err)
	}

	// Should have: ../, visible.tdb (hidden entries excluded)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(entries), entries)
	}
	if entries[0] != "../" {
		t.Errorf("entries[0] = %q, want '../'", entries[0])
	}
	if entries[1] != "visible.tdb" {
		t.Errorf("entries[1] = %q, want 'visible.tdb'", entries[1])
	}
}

// =============================================================================
// Browse dialog.Dialog Tests
// =============================================================================

func TestBuildBrowseDialog(t *testing.T) {
	entries := []string{"../", "backups/", "default.tdb", "personal.tdb"}
	d := buildBrowseDialog("/tmp/test", entries)

	if !strings.Contains(d.Title(), "Open File") {
		t.Errorf("title = %q, should contain 'Open File'", d.Title())
	}

	fields := d.Fields()
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}
	if fields[0].Type != dialog.FieldList {
		t.Errorf("field type = %v, want dialog.FieldList", fields[0].Type)
	}
	if len(fields[0].Options) != 4 {
		t.Errorf("expected 4 options, got %d", len(fields[0].Options))
	}

	buttons := d.Buttons()
	if buttons[0].Label != "Open" {
		t.Errorf("primary button = %q, want 'Open'", buttons[0].Label)
	}
}

// TestApp_HandleMenuAction_OpenFile_AlwaysStartsInDefaultDir asserts that File>Open
// starts in db.DefaultDirectory() regardless of which file is currently open. The
// previous behavior used filepath.Dir(a.db.Path()), which left users stranded in
// temp directories when tests had run earlier.
func TestApp_HandleMenuAction_OpenFile_AlwaysStartsInDefaultDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	defaultDir := db.DefaultDirectory()
	if err := os.MkdirAll(defaultDir, 0o755); err != nil {
		t.Fatalf("setup: mkdir default dir: %v", err)
	}

	// Open a real DB in some other location so a.db is non-nil and points elsewhere.
	otherDir := t.TempDir()
	dbPath := filepath.Join(otherDir, "current.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	defer database.Close()

	app := &App{
		db:          database,
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	app.handleMenuAction(widget.MenuActionOpenFile, "")

	if app.browseDir != defaultDir {
		t.Errorf("browseDir = %q, want %q (Open File should start in DefaultDirectory, not the current file's directory)",
			app.browseDir, defaultDir)
	}
}

// TestApp_BrowseDialog_DoubleClickOnDotDot_NavigatesUp asserts that a double-click
// on the "../" list entry in the Open File browse dialog navigates to the parent
// directory without requiring a separate Open button press.
func TestApp_BrowseDialog_DoubleClickOnDotDot_NavigatesUp(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatalf("setup: mkdir child: %v", err)
	}
	// Put a stub .tdb so listDirectoryEntries returns the real "../" row
	// rather than the "(empty directory)" placeholder it falls back to.
	if err := os.WriteFile(filepath.Join(child, "stub.tdb"), []byte{}, 0o644); err != nil {
		t.Fatalf("setup: write stub: %v", err)
	}

	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		width:       100,
		height:      40,
	}
	app.styles.Resize(100, 40)

	app.openBrowseDialog(child)
	if app.fileDialog == nil {
		t.Fatal("setup: openBrowseDialog did not set fileDialog")
	}
	if app.browseDir != child {
		t.Fatalf("setup: browseDir = %q, want %q", app.browseDir, child)
	}

	now := time.Unix(0, 0)
	app.browseDialogClicks = widget.NewClickTracker(400 * time.Millisecond)
	app.browseDialogClicks.SetNowFn(func() time.Time { return now })

	d := app.fileDialog
	startCol, startRow, _, _ := d.DialogBounds(app.width, app.height)
	contentWidth := d.Width() - dialog.DialogHorizontalOverhead

	// Locate the screen row of the first list item ("../").
	dotDotY := -1
	for y := 0; y < d.ContentHeight(); y++ {
		hit := d.HitTestContent(5, y, contentWidth)
		if hit.Zone == dialog.DialogHitField && hit.ListItemIndex == 0 {
			dotDotY = y
			break
		}
	}
	if dotDotY < 0 {
		t.Fatal("setup: could not locate ../ list row in browse dialog")
	}
	clickMsg := tea.MouseClickMsg{
		X:      startCol + 3 + 5,
		Y:      startRow + 2 + dotDotY,
		Button: tea.MouseLeft,
	}

	// First click: selects only, does not navigate.
	if _, cmd := app.Update(clickMsg); cmd != nil {
		t.Fatal("first click should not return a navigation command")
	}
	if app.browseDir != child {
		t.Fatalf("after first click, browseDir = %q, want %q (no navigation yet)", app.browseDir, child)
	}

	// Second click within threshold: triggers navigation up.
	now = now.Add(100 * time.Millisecond)
	app.Update(clickMsg)

	if app.browseDir != parent {
		t.Errorf("after double-click on ../, browseDir = %q, want %q", app.browseDir, parent)
	}
}

// TestApp_BrowseDialog_DoubleClickOnSubdir_NavigatesIn asserts double-clicking
// a subdirectory list row drills into it without a separate Open press.
func TestApp_BrowseDialog_DoubleClickOnSubdir_NavigatesIn(t *testing.T) {
	parent := t.TempDir()
	subdir := filepath.Join(parent, "sub")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("setup: mkdir sub: %v", err)
	}

	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		width:       100,
		height:      40,
	}
	app.styles.Resize(100, 40)

	app.openBrowseDialog(parent)

	// Entries: ["../", "sub/"] — sub/ is index 1.
	now := time.Unix(0, 0)
	app.browseDialogClicks = widget.NewClickTracker(400 * time.Millisecond)
	app.browseDialogClicks.SetNowFn(func() time.Time { return now })

	d := app.fileDialog
	startCol, startRow, _, _ := d.DialogBounds(app.width, app.height)
	contentWidth := d.Width() - dialog.DialogHorizontalOverhead

	subY := -1
	for y := 0; y < d.ContentHeight(); y++ {
		hit := d.HitTestContent(5, y, contentWidth)
		if hit.Zone == dialog.DialogHitField && hit.ListItemIndex == 1 {
			subY = y
			break
		}
	}
	if subY < 0 {
		t.Fatal("setup: could not locate sub/ list row")
	}
	clickMsg := tea.MouseClickMsg{
		X:      startCol + 3 + 5,
		Y:      startRow + 2 + subY,
		Button: tea.MouseLeft,
	}

	app.Update(clickMsg)
	now = now.Add(100 * time.Millisecond)
	app.Update(clickMsg)

	if app.browseDir != subdir {
		t.Errorf("after double-click on sub/, browseDir = %q, want %q", app.browseDir, subdir)
	}
}
