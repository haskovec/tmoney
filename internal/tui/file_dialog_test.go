package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haskovec/tmoney/internal/db"
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
		if fields[fileFieldPath].Type != FieldText {
			t.Errorf("field type = %v, want FieldText", fields[fileFieldPath].Type)
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
		if fields[0].Type != FieldSelect {
			t.Errorf("field type = %v, want FieldSelect", fields[0].Type)
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
// App Integration Tests - File Dialog
// =============================================================================

func TestApp_CloseFileDialog(t *testing.T) {
	app := &App{
		fileDialog: func() *Dialog {
			d := NewDialog("New File")
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		fileDialog: func() *Dialog {
			d := buildNewFileDialog()
			return d
		}(),
		fileDialogMode: fileDialogModeNew,
	}

	escKey := tea.KeyMsg{Type: tea.KeyEsc}
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	model, cmd := app.handleFileDialogKey(tea.KeyMsg{Type: tea.KeyEnter})
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		fileDialog: func() *Dialog {
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		fileDialog: func() *Dialog {
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		fileDialog: func() *Dialog {
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		fileDialog: func() *Dialog {
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		fileDialog: func() *Dialog {
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		fileDialog: func() *Dialog {
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		fileDialog: func() *Dialog {
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
	styles := NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewDashboard,
		width:       100,
		height:      30,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	app.handleMenuAction(MenuActionNewFile)

	if app.fileDialog == nil {
		t.Error("MenuActionNewFile should open the file dialog")
	}
	if app.fileDialogMode != fileDialogModeNew {
		t.Errorf("fileDialogMode = %d, want fileDialogModeNew", app.fileDialogMode)
	}
}

func TestApp_HandleMenuAction_OpenFile(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	app.handleMenuAction(MenuActionOpenFile)

	if app.fileDialog == nil {
		t.Error("MenuActionOpenFile should open the file dialog")
	}
	if app.fileDialogMode != fileDialogModeBrowse {
		t.Errorf("fileDialogMode = %d, want fileDialogModeBrowse", app.fileDialogMode)
	}
}

func TestApp_HandleMenuAction_OpenRecent(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	app.handleMenuAction(MenuActionOpenRecent)

	if app.fileDialog == nil {
		t.Error("MenuActionOpenRecent should open the file dialog")
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
	if buttons[1].Label != "Create" {
		t.Errorf("primary button label = %q, want 'Create'", buttons[1].Label)
	}
}

func TestBuildOpenFileDialog_ButtonLabel(t *testing.T) {
	d := buildOpenFileDialog()
	buttons := d.Buttons()
	if len(buttons) < 2 {
		t.Fatalf("expected at least 2 buttons, got %d", len(buttons))
	}
	if buttons[1].Label != "Open" {
		t.Errorf("primary button label = %q, want 'Open'", buttons[1].Label)
	}
}

func TestBuildOpenRecentDialog_ButtonLabel(t *testing.T) {
	d := buildOpenRecentDialog([]string{"/a.tdb"})
	buttons := d.Buttons()
	if len(buttons) < 2 {
		t.Fatalf("expected at least 2 buttons, got %d", len(buttons))
	}
	if buttons[1].Label != "Open" {
		t.Errorf("primary button label = %q, want 'Open'", buttons[1].Label)
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
// Browse Dialog Tests
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
	if fields[0].Type != FieldList {
		t.Errorf("field type = %v, want FieldList", fields[0].Type)
	}
	if len(fields[0].Options) != 4 {
		t.Errorf("expected 4 options, got %d", len(fields[0].Options))
	}

	buttons := d.Buttons()
	if buttons[1].Label != "Open" {
		t.Errorf("primary button = %q, want 'Open'", buttons[1].Label)
	}
}
