package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/applog"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// TestApp_ReloadTheme_Builtin exercises the success path of TH-020:
// loading a known built-in theme repaints app.styles, persists the ID
// onto cfg, and returns a tea.Cmd that emits a tea.WindowSizeMsg with
// the App's current dimensions so the next render reflects the new
// palette.
func TestApp_ReloadTheme_Builtin(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })

	a := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		styles:      widget.NewStyles(),
		cfg:         &config.Config{},
		width:       100,
		height:      30,
	}

	cmd := a.reloadTheme("turbo-vision")
	if cmd == nil {
		t.Fatal("reloadTheme() returned nil cmd")
	}

	// (a) styles reflect Turbo Vision: header.fg = "#000000",
	// table.selected.bg = "#00aaaa".
	wantHeaderFg := lipgloss.Color("#000000")
	if got := a.styles.Header.GetForeground(); got != wantHeaderFg {
		t.Errorf("Header.GetForeground() = %v, want %v", got, wantHeaderFg)
	}
	wantSelectedBg := lipgloss.Color("#00aaaa")
	if got := a.styles.SelectedRow.GetBackground(); got != wantSelectedBg {
		t.Errorf("SelectedRow.GetBackground() = %v, want %v", got, wantSelectedBg)
	}

	// (c) cfg updated.
	if a.cfg.Theme != "turbo-vision" {
		t.Errorf("cfg.Theme = %q, want %q", a.cfg.Theme, "turbo-vision")
	}

	// (b) returned cmd produces a WindowSizeMsg matching current dimensions.
	msg := cmd()
	sz, ok := msg.(tea.WindowSizeMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want tea.WindowSizeMsg", msg)
	}
	if sz.Width != 100 || sz.Height != 30 {
		t.Errorf("WindowSizeMsg = %+v, want {Width:100 Height:30}", sz)
	}
}

// TestApp_ReloadTheme_UnknownID exercises the failure path: an unknown
// theme ID leaves cfg.Theme and the active palette untouched and the
// returned cmd produces a themeReloadFailedMsg. Phase 9 will wire that
// into a status-bar toast and a log entry; for now we just assert the
// message kind so later wiring has a stable shape to plug into.
func TestApp_ReloadTheme_UnknownID(t *testing.T) {
	// Isolate the applog write so the test doesn't pollute the user's
	// real ~/.config/tmoney/log.txt — TH-032 logs theme failures.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	a := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		styles:      widget.NewStyles(),
		cfg:         &config.Config{Theme: "default"},
		width:       80,
		height:      24,
	}

	cmd := a.reloadTheme("nonexistent")
	if cmd == nil {
		t.Fatal("reloadTheme() returned nil cmd")
	}

	if a.cfg.Theme != "default" {
		t.Errorf("cfg.Theme = %q, want %q (must not change on failure)", a.cfg.Theme, "default")
	}

	failed, ok := findMsgInBatch[themeReloadFailedMsg](cmd)
	if !ok {
		t.Fatalf("themeReloadFailedMsg not found in cmd output")
	}
	if failed.id != "nonexistent" {
		t.Errorf("themeReloadFailedMsg.id = %q, want %q", failed.id, "nonexistent")
	}
	if failed.err == nil {
		t.Error("themeReloadFailedMsg.err = nil, want non-nil error describing the failure")
	}
}

// findMsgInBatch invokes cmd, then if the result is a tea.BatchMsg it
// invokes each contained Cmd and returns the first message that
// matches type T. If cmd's direct result is already a T, returns it.
// Useful for tests that need to dig a specific message out of a
// reloadTheme-style batched response.
func findMsgInBatch[T any](cmd tea.Cmd) (T, bool) {
	var zero T
	if cmd == nil {
		return zero, false
	}
	msg := cmd()
	if v, ok := msg.(T); ok {
		return v, true
	}
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return zero, false
	}
	for _, c := range batch {
		if c == nil {
			continue
		}
		if v, ok := c().(T); ok {
			return v, true
		}
	}
	return zero, false
}

// TestApp_HandleMenuAction_LoadTheme covers TH-023: dispatching
// widget.MenuActionLoadTheme with a theme ID payload routes to reloadTheme,
// which repaints styles, persists the ID onto cfg, and returns a
// tea.Cmd that emits a tea.WindowSizeMsg matching the App's dims.
func TestApp_HandleMenuAction_LoadTheme(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })

	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		styles:      widget.NewStyles(),
		cfg:         &config.Config{},
		width:       100,
		height:      30,
	}

	_, cmd := app.handleMenuAction(widget.MenuActionLoadTheme, "turbo-vision")
	if cmd == nil {
		t.Fatal("handleMenuAction(widget.MenuActionLoadTheme, ...) returned nil cmd")
	}

	if app.cfg.Theme != "turbo-vision" {
		t.Errorf("cfg.Theme = %q, want %q", app.cfg.Theme, "turbo-vision")
	}

	wantHeaderFg := lipgloss.Color("#000000")
	if got := app.styles.Header.GetForeground(); got != wantHeaderFg {
		t.Errorf("Header.GetForeground() = %v, want %v", got, wantHeaderFg)
	}

	msg := cmd()
	sz, ok := msg.(tea.WindowSizeMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want tea.WindowSizeMsg", msg)
	}
	if sz.Width != 100 || sz.Height != 30 {
		t.Errorf("WindowSizeMsg = %+v, want {Width:100 Height:30}", sz)
	}
}

// TestApp_MenuSelect_LoadTheme covers the full submenu flow for TH-023:
// activating the View menu and selecting the entry whose data payload
// is "turbo-vision" emits the same cmd that reloadTheme would have
// produced. This proves the menu dispatch carries the theme ID payload
// through Select() into handleMenuAction.
func TestApp_MenuSelect_LoadTheme(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })

	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		styles:      widget.NewStyles(),
		cfg:         &config.Config{},
		width:       100,
		height:      30,
	}
	app.menubar.SetMenuItemsBuilder(widget.ViewMenuIndex, func() []widget.MenuItem {
		return widget.BuildThemeMenuItems(app.cfg.Theme)
	})

	app.menubar.ActivateMenu(widget.ViewMenuIndex)

	// Find the index of the turbo-vision item in the freshly-built submenu.
	current := app.menubar.CurrentMenu()
	if current == nil {
		t.Fatal("CurrentMenu() returned nil after ActivateMenu(widget.ViewMenuIndex)")
	}
	target := -1
	for i, item := range current.Items {
		if item.Data == "turbo-vision" {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatalf("no menu item with data=%q found in View submenu", "turbo-vision")
	}
	app.menubar.SetItemCursor(target)

	action, data := app.menubar.Select()
	if action != widget.MenuActionLoadTheme {
		t.Fatalf("Select() action = %v, want widget.MenuActionLoadTheme", action)
	}
	if data != "turbo-vision" {
		t.Fatalf("Select() data = %q, want %q", data, "turbo-vision")
	}

	_, cmd := app.handleMenuAction(action, data)
	if cmd == nil {
		t.Fatal("handleMenuAction(widget.MenuActionLoadTheme, ...) returned nil cmd")
	}
	if app.cfg.Theme != "turbo-vision" {
		t.Errorf("cfg.Theme = %q, want %q", app.cfg.Theme, "turbo-vision")
	}

	msg := cmd()
	if _, ok := msg.(tea.WindowSizeMsg); !ok {
		t.Fatalf("cmd() = %T, want tea.WindowSizeMsg", msg)
	}
}

// TestNewApp_AppliesPersistedTheme covers TH-029: an App constructed
// with cfg.Theme = "turbo-vision" must have the Turbo Vision palette
// applied to its styles, otherwise the View → Theme menu's checkmark
// disagrees with what the user actually sees on screen.
func TestNewApp_AppliesPersistedTheme(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "newapp_theme.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("db.Create: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := &config.Config{Theme: "turbo-vision"}
	a := NewApp(database, cfg)

	wantHeaderFg := lipgloss.Color("#000000")
	if got := a.styles.Header.GetForeground(); got != wantHeaderFg {
		t.Errorf("Header.GetForeground() = %v, want %v (turbo-vision header fg) — persisted theme not applied",
			got, wantHeaderFg)
	}
	wantSelectedBg := lipgloss.Color("#00aaaa")
	if got := a.styles.SelectedRow.GetBackground(); got != wantSelectedBg {
		t.Errorf("SelectedRow.GetBackground() = %v, want %v (turbo-vision selected bg)",
			got, wantSelectedBg)
	}
}

// TestNewApp_UnknownThemeFallsBackToDefault covers TH-029's documented
// fallback behavior: when cfg.Theme names a theme that no longer
// exists (e.g. a user-installed theme that was deleted), NewApp must
// not crash and must leave the styles on the embedded default palette.
// TH-032 also surfaces the failure as a toast + log entry.
func TestNewApp_UnknownThemeFallsBackToDefault(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })
	// Isolate the applog write triggered by TH-032's failure surfacing
	// so the test doesn't pollute the user's real config dir.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "newapp_unknown_theme.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("db.Create: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	defaultStyles := widget.NewStyles()
	wantHeaderFg := defaultStyles.Header.GetForeground()

	cfg := &config.Config{Theme: "nonexistent"}
	a := NewApp(database, cfg)

	if got := a.styles.Header.GetForeground(); got != wantHeaderFg {
		t.Errorf("Header.GetForeground() = %v, want %v (default; unknown theme should fall back)",
			got, wantHeaderFg)
	}
}

// TestApp_ReloadTheme_LogsAndToastsOnIssues covers TH-032's happy path:
// loading a user-dir theme that contains a malformed slot value should
// (a) still apply the partially-recovered theme, (b) append the parse
// issue to the applog file, and (c) set a status-bar toast naming the
// theme and counting the issues. Successful loads with no issues
// produce no toast (asserted in a sibling test).
func TestApp_ReloadTheme_LogsAndToastsOnIssues(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	themesDir := filepath.Join(tmp, "tmoney", "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// One malformed slot value plus one unknown key — each should log
	// a separate line. Picking a top-level unknown key ("typo") avoids
	// the BurntSushi toml parser reporting both the section *and* the
	// nested key as undecoded entries (which would inflate the count).
	body := `name = "broken"
typo = "ignored"
[text]
negative = "not-a-color"
`
	themePath := filepath.Join(themesDir, "broken.toml")
	if err := os.WriteFile(themePath, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	a := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		styles:      widget.NewStyles(),
		cfg:         &config.Config{},
		width:       80,
		height:      24,
	}

	cmd := a.reloadTheme("broken")
	if cmd == nil {
		t.Fatal("reloadTheme() returned nil cmd")
	}

	// widget.Toast on the status bar describes the issue count.
	toast := a.statusbar.Toast()
	if toast == nil {
		t.Fatal("widget.Toast() = nil, want non-nil after reloadTheme with issues")
	}
	if !strings.Contains(toast.Text, `"broken"`) {
		t.Errorf("toast text missing theme id: %q", toast.Text)
	}
	if !strings.Contains(toast.Text, "2 issues") {
		t.Errorf("toast text missing issue count: %q", toast.Text)
	}
	if toast.Level != widget.NotificationAlert {
		t.Errorf("toast level = %d, want %d", toast.Level, widget.NotificationAlert)
	}

	// Log file contains an entry for each issue.
	logPath, err := applog.LogPath()
	if err != nil {
		t.Fatalf("LogPath: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", logPath, err)
	}
	if !strings.Contains(toast.Text, logPath) {
		t.Errorf("toast text missing log path %q: %q", logPath, toast.Text)
	}
	logged := string(data)
	if !strings.Contains(logged, "text.negative") {
		t.Errorf("log missing malformed-value entry: %q", logged)
	}
	if !strings.Contains(logged, "typo") {
		t.Errorf("log missing unknown-key entry: %q", logged)
	}
	if !strings.Contains(logged, "[theme]") {
		t.Errorf("log entries missing [theme] category: %q", logged)
	}

	// The cmd batch must still carry the WindowSizeMsg (for the live
	// repaint) plus a widget.ToastClearMsg producer (for auto-clear).
	if _, ok := findMsgInBatch[tea.WindowSizeMsg](cmd); !ok {
		t.Error("cmd batch missing tea.WindowSizeMsg")
	}
	// findMsgInBatch needs a fresh cmd because invocation is one-shot.
	cmd = a.reloadTheme("broken")
	if _, ok := findMsgInBatch[widget.ToastClearMsg](cmd); !ok {
		t.Error("cmd batch missing widget.ToastClearMsg producer")
	}
}

// TestApp_ReloadTheme_NoIssues_NoToast covers the spec's negative
// requirement: a clean built-in load must not surface a toast.
func TestApp_ReloadTheme_NoIssues_NoToast(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	a := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		styles:      widget.NewStyles(),
		cfg:         &config.Config{},
		width:       80,
		height:      24,
	}

	a.reloadTheme("turbo-vision")
	if got := a.statusbar.Toast(); got != nil {
		t.Errorf("widget.Toast() = %+v, want nil for clean built-in load", got)
	}
}

// TestApp_ReloadTheme_FailureLogsAndToasts asserts that the failure
// path (unknown ID, no user-dir match) also writes a log entry and
// raises a toast — the user's ID was wrong, so silence isn't helpful.
func TestApp_ReloadTheme_FailureLogsAndToasts(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	a := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		styles:      widget.NewStyles(),
		cfg:         &config.Config{Theme: "default"},
		width:       80,
		height:      24,
	}

	cmd := a.reloadTheme("nonexistent")
	if cmd == nil {
		t.Fatal("reloadTheme() returned nil cmd")
	}

	toast := a.statusbar.Toast()
	if toast == nil {
		t.Fatal("widget.Toast() = nil, want non-nil after failure path")
	}
	if !strings.Contains(toast.Text, `"nonexistent"`) {
		t.Errorf("toast missing theme id: %q", toast.Text)
	}
	if !strings.Contains(toast.Text, "failed to load") {
		t.Errorf("toast missing failure verb: %q", toast.Text)
	}

	logPath, err := applog.LogPath()
	if err != nil {
		t.Fatalf("LogPath: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", logPath, err)
	}
	logged := string(data)
	if !strings.Contains(logged, "nonexistent") {
		t.Errorf("log missing theme id: %q", logged)
	}
	if !strings.Contains(logged, "failed to load") {
		t.Errorf("log missing failure phrase: %q", logged)
	}
}

// TestNewApp_PersistedThemeWithIssues_Surfaces covers the TH-029 +
// TH-032 join: a persisted theme that parses with issues must apply
// (best-effort) and surface the issues via toast + log so the user
// notices on next launch instead of silently inheriting a broken
// palette.
func TestNewApp_PersistedThemeWithIssues_Surfaces(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	themesDir := filepath.Join(tmp, "tmoney", "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	body := `name = "broken-startup"
[text]
negative = "not-a-color"
`
	if err := os.WriteFile(filepath.Join(themesDir, "broken-startup.toml"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dbPath := filepath.Join(tmp, "newapp_persisted_issues.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("db.Create: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := &config.Config{Theme: "broken-startup"}
	a := NewApp(database, cfg)

	if a.statusbar.Toast() == nil {
		t.Error("startup toast not set for theme with issues")
	}

	// Init() should batch in a widget.ClearToastCmd so the startup toast
	// auto-clears like any other.
	cmd := a.Init()
	if _, ok := findMsgInBatch[widget.ToastClearMsg](cmd); !ok {
		t.Error("Init() batch missing widget.ToastClearMsg producer for startup toast")
	}

	logPath, err := applog.LogPath()
	if err != nil {
		t.Fatalf("LogPath: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", logPath, err)
	}
	if !strings.Contains(string(data), "text.negative") {
		t.Errorf("startup log missing parse issue: %q", data)
	}
}
