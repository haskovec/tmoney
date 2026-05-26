package widget

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/tui/theme"
)

func TestGetLayoutMode(t *testing.T) {
	tests := []struct {
		name     string
		width    int
		expected LayoutMode
	}{
		{"very small terminal", 40, LayoutSmall},
		{"small terminal at boundary", 79, LayoutSmall},
		{"medium at lower boundary", 80, LayoutMedium},
		{"medium terminal", 100, LayoutMedium},
		{"medium at upper boundary", 119, LayoutMedium},
		{"large at lower boundary", 120, LayoutLarge},
		{"large terminal", 200, LayoutLarge},
		{"zero width", 0, LayoutSmall},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetLayoutMode(tt.width)
			if got != tt.expected {
				t.Errorf("GetLayoutMode(%d) = %v, want %v", tt.width, got, tt.expected)
			}
		})
	}
}

func TestNewStyles(t *testing.T) {
	s := NewStyles()

	// Verify that key styles are initialized (non-zero)
	if s.Header.GetBold() != true {
		t.Error("Header style should be bold")
	}
	if s.Error.GetBold() != true {
		t.Error("Error style should be bold")
	}
	if s.Title.GetBold() != true {
		t.Error("Title style should be bold")
	}
	if s.Alert.GetBold() != true {
		t.Error("Alert style should be bold")
	}
	if s.Bold.GetBold() != true {
		t.Error("Bold style should be bold")
	}
}

func TestStyles_Resize_Small(t *testing.T) {
	s := NewStyles()
	s.Resize(60, 24)

	if s.width != 60 {
		t.Errorf("width = %d, want 60", s.width)
	}
	if s.height != 24 {
		t.Errorf("height = %d, want 24", s.height)
	}
	if s.SidebarWidth() != 0 {
		t.Errorf("SidebarWidth() = %d, want 0 for small layout", s.SidebarWidth())
	}
	if s.ContentWidth() != 60 {
		t.Errorf("ContentWidth() = %d, want 60 for small layout", s.ContentWidth())
	}
}

func TestStyles_Resize_Medium(t *testing.T) {
	s := NewStyles()
	s.Resize(100, 30)

	if s.SidebarWidth() != SidebarWidthMedium {
		t.Errorf("SidebarWidth() = %d, want %d for medium layout", s.SidebarWidth(), SidebarWidthMedium)
	}
	expectedContent := 100 - SidebarWidthMedium
	if s.ContentWidth() != expectedContent {
		t.Errorf("ContentWidth() = %d, want %d for medium layout", s.ContentWidth(), expectedContent)
	}
}

func TestStyles_Resize_Large(t *testing.T) {
	s := NewStyles()
	s.Resize(120, 40)

	// At the exact large-layout breakpoint the sidebar is the floor value.
	if s.SidebarWidth() != SidebarWidthLarge {
		t.Errorf("SidebarWidth() = %d, want %d at the large breakpoint", s.SidebarWidth(), SidebarWidthLarge)
	}
	expectedContent := 120 - SidebarWidthLarge
	if s.ContentWidth() != expectedContent {
		t.Errorf("ContentWidth() = %d, want %d for large layout", s.ContentWidth(), expectedContent)
	}
}

// TestStyles_Resize_LargeScales verifies the sidebar grows with terminal
// width once we're past the large breakpoint, so wide terminals get
// extra room to show long account names.
func TestStyles_Resize_LargeScales(t *testing.T) {
	tests := []struct {
		width       int
		wantSidebar int
		wantContent int
	}{
		{120, SidebarWidthLarge, 120 - SidebarWidthLarge},                   // breakpoint: floor
		{150, SidebarWidthLarge + (150-120)/sidebarGrowthDivisor, 150 - 27}, // 27
		{200, SidebarWidthLarge + (200-120)/sidebarGrowthDivisor, 200 - 34}, // 34
		{400, SidebarWidthMax, 400 - SidebarWidthMax},                       // capped
	}

	for _, tt := range tests {
		s := NewStyles()
		s.Resize(tt.width, 40)
		if s.SidebarWidth() != tt.wantSidebar {
			t.Errorf("SidebarWidth(width=%d) = %d, want %d", tt.width, s.SidebarWidth(), tt.wantSidebar)
		}
		if s.ContentWidth() != tt.wantContent {
			t.Errorf("ContentWidth(width=%d) = %d, want %d", tt.width, s.ContentWidth(), tt.wantContent)
		}
	}
}

func TestStyles_Resize_UpdatesWidthDependentStyles(t *testing.T) {
	s := NewStyles()
	s.Resize(100, 30)

	if s.Header.GetWidth() != 100 {
		t.Errorf("Header width = %d, want 100", s.Header.GetWidth())
	}
	if s.StatusBar.GetWidth() != 100 {
		t.Errorf("StatusBar width = %d, want 100", s.StatusBar.GetWidth())
	}
	if s.Content.GetWidth() != 100 {
		t.Errorf("Content width = %d, want 100", s.Content.GetWidth())
	}

	// Resize again to verify updates
	s.Resize(120, 40)

	if s.Header.GetWidth() != 120 {
		t.Errorf("Header width after resize = %d, want 120", s.Header.GetWidth())
	}
	if s.StatusBar.GetWidth() != 120 {
		t.Errorf("StatusBar width after resize = %d, want 120", s.StatusBar.GetWidth())
	}
}

func TestStyles_RenderDoesNotPanic(t *testing.T) {
	s := NewStyles()
	s.Resize(80, 24)

	// Verify rendering with each style doesn't panic
	styles := []struct {
		name  string
		style func() string
	}{
		{"Header", func() string { return s.Header.Render("test") }},
		{"StatusBar", func() string { return s.StatusBar.Render("test") }},
		{"Content", func() string { return s.Content.Render("test") }},
		{"Sidebar", func() string { return s.Sidebar.Render("test") }},
		{"SidebarItem", func() string { return s.SidebarItem.Render("test") }},
		{"SidebarGroup", func() string { return s.SidebarGroup.Render("test") }},
		{"Title", func() string { return s.Title.Render("test") }},
		{"SectionHead", func() string { return s.SectionHead.Render("test") }},
		{"TableHeader", func() string { return s.TableHeader.Render("test") }},
		{"TableRow", func() string { return s.TableRow.Render("test") }},
		{"SelectedRow", func() string { return s.SelectedRow.Render("test") }},
		{"Positive", func() string { return s.Positive.Render("+100.00") }},
		{"Negative", func() string { return s.Negative.Render("-50.00") }},
		{"Pending", func() string { return s.Pending.Render("pending") }},
		{"Alert", func() string { return s.Alert.Render("due today") }},
		{"Error", func() string { return s.Error.Render("error msg") }},
		{"Dialog", func() string { return s.Dialog.Render("dialog") }},
		{"DialogTitle", func() string { return s.DialogTitle.Render("title") }},
		{"DialogButton", func() string { return s.DialogButton.Render("OK") }},
		{"Border", func() string { return s.Border.Render("box") }},
		{"Bold", func() string { return s.Bold.Render("bold") }},
		{"Muted", func() string { return s.Muted.Render("muted") }},
		{"Placeholder", func() string { return s.Placeholder.Render("placeholder") }},
		{"OverlayBox", func() string { return s.OverlayBox.Render("box") }},
	}

	for _, tt := range styles {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.style()
			if result == "" {
				t.Errorf("%s.Render() returned empty string", tt.name)
			}
		})
	}
}

func TestColorConstants(t *testing.T) {
	// Verify color constants are non-nil
	colors := []struct {
		name  string
		color color.Color
	}{
		{"ColorPositive", ColorPositive},
		{"ColorNegative", ColorNegative},
		{"ColorPending", ColorPending},
		{"ColorAlert", ColorAlert},
		{"ColorBorder", ColorBorder},
		{"ColorHeaderFg", ColorHeaderFg},
		{"ColorHeaderBg", ColorHeaderBg},
		{"ColorStatusFg", ColorStatusFg},
		{"ColorStatusBg", ColorStatusBg},
		{"ColorSelectedFg", ColorSelectedFg},
		{"ColorSelectedBg", ColorSelectedBg},
		{"ColorMuted", ColorMuted},
		{"ColorTitle", ColorTitle},
	}

	for _, tt := range colors {
		t.Run(tt.name, func(t *testing.T) {
			if tt.color == nil {
				t.Errorf("%s should not be nil", tt.name)
			}
		})
	}
}

// restoreDefaultTheme reapplies the embedded default theme. Use as a
// t.Cleanup so a test that calls applyTheme doesn't leak palette state
// into the package-level Color* vars and confuse later tests.
func restoreDefaultTheme(t *testing.T) {
	t.Helper()
	def, _, err := theme.LoadBuiltin("default")
	if err != nil {
		t.Fatalf("restoreDefaultTheme: load default: %v", err)
	}
	s := NewStyles()
	s.ApplyTheme(def)
}

// TestStyles_ApplyTheme verifies that applying a theme rebuilds the
// Styles fields from the theme's slot values. Specifically: turbo-vision
// has menubar.fg = "#000000" and table.selected.bg = "#00aaaa", and
// after applyTheme those colors are reflected on the Header and
// SelectedRow styles. This is the load-bearing property the live-swap
// mechanism in Phase 5 depends on.
func TestStyles_ApplyTheme(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })

	turbo, _, err := theme.LoadBuiltin("turbo-vision")
	if err != nil {
		t.Fatalf("LoadBuiltin: %v", err)
	}

	s := NewStyles()
	s.ApplyTheme(turbo)

	wantHeaderFg := lipgloss.Color("#000000")
	if got := s.Header.GetForeground(); got != wantHeaderFg {
		t.Errorf("Header.GetForeground() = %v, want %v", got, wantHeaderFg)
	}

	wantSelectedBg := lipgloss.Color("#00aaaa")
	if got := s.SelectedRow.GetBackground(); got != wantSelectedBg {
		t.Errorf("SelectedRow.GetBackground() = %v, want %v", got, wantSelectedBg)
	}

	// Turbo Vision: text.positive = "#55ff55"
	wantPositive := lipgloss.Color("#55ff55")
	if got := s.Positive.GetForeground(); got != wantPositive {
		t.Errorf("Positive.GetForeground() = %v, want %v", got, wantPositive)
	}

	// Turbo Vision: window.border.fg = "#ffffff"
	wantBorder := lipgloss.Color("#ffffff")
	if got := s.OverlayBox.GetBorderTopForeground(); got != wantBorder {
		t.Errorf("OverlayBox.GetBorderTopForeground() = %v, want %v", got, wantBorder)
	}

	// Turbo Vision: text.muted = "#5555ff" — Placeholder should follow.
	wantMuted := lipgloss.Color("#5555ff")
	if got := s.Placeholder.GetForeground(); got != wantMuted {
		t.Errorf("Placeholder.GetForeground() = %v, want %v", got, wantMuted)
	}

	// Turbo Vision: desktop.bg = "#0000aa" → Content and Sidebar paint
	// their empty cells blue so short rows and unused vertical space
	// match the classic TV desktop fill.
	wantDesktop := lipgloss.Color("#0000aa")
	if got := s.Content.GetBackground(); got != wantDesktop {
		t.Errorf("Content.GetBackground() = %v, want %v", got, wantDesktop)
	}
	if got := s.Sidebar.GetBackground(); got != wantDesktop {
		t.Errorf("Sidebar.GetBackground() = %v, want %v", got, wantDesktop)
	}
}

// TestRepaintDesktop_TransparentNoop verifies that with the default
// theme's empty desktop.bg, repaintDesktop returns the input unchanged
// (lipgloss.NoColor short-circuit). This protects the default look —
// no extra SGR codes leak into output that would display on
// non-Turbo-Vision setups.
func TestRepaintDesktop_TransparentNoop(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })
	def, _, err := theme.LoadBuiltin("default")
	if err != nil {
		t.Fatalf("LoadBuiltin(default): %v", err)
	}
	s := NewStyles()
	s.ApplyTheme(def)

	in := "\x1b[1mDASH\x1b[m  raw  \x1b[38;5;240mMay\x1b[m"
	got := repaintDesktop(in)
	if got != in {
		t.Errorf("repaintDesktop on transparent desktop should be a no-op\n got=%q\nwant=%q", got, in)
	}
}

// TestRepaintDesktop_RestoresBgAfterResets is the load-bearing fix for
// the Turbo Vision blue desktop. lipgloss's outer Content.Background
// only emits the bg SGR at the start and end of each line; inner
// `\x1b[m` resets from Bold/Muted/Title renders wipe it. This test
// asserts that after repaintDesktop, every reset is followed by the
// bg-set SGR so subsequent raw text and styled chunks render on blue
// rather than terminal default.
func TestRepaintDesktop_RestoresBgAfterResets(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })
	turbo, _, err := theme.LoadBuiltin("turbo-vision")
	if err != nil {
		t.Fatalf("LoadBuiltin(turbo-vision): %v", err)
	}
	s := NewStyles()
	s.ApplyTheme(turbo)

	bg := bgSGR(ColorDesktopBg)
	if bg == "" {
		t.Fatalf("bgSGR should be non-empty for Turbo Vision; ColorDesktopBg=%v", ColorDesktopBg)
	}

	in := "\x1b[1mDASH\x1b[m  raw  \x1b[38;5;240mMay\x1b[m"
	got := repaintDesktop(in)

	// Each `\x1b[m` should now be immediately followed by the bg SGR.
	want := "\x1b[1mDASH\x1b[m" + bg + "  raw  \x1b[38;5;240mMay\x1b[m" + bg
	if got != want {
		t.Errorf("repaintDesktop did not restore bg after resets\n got=%q\nwant=%q", got, want)
	}
}

// TestRenderViewContent_TurboVisionFillsGaps is the end-to-end guard:
// rendering a multi-chunk dashboard-shaped row through RenderViewContent
// under Turbo Vision should produce output where every cell carries the
// blue bg SGR — no terminal-default bands. We verify by checking that
// the line, after stripping the bg SGR, contains no remaining `\x1b[m`
// resets that aren't immediately followed by another SGR set (i.e. no
// "naked" resets that would expose terminal default).
func TestRenderViewContent_TurboVisionFillsGaps(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })
	turbo, _, err := theme.LoadBuiltin("turbo-vision")
	if err != nil {
		t.Fatalf("LoadBuiltin(turbo-vision): %v", err)
	}
	s := NewStyles()
	s.ApplyTheme(turbo)

	// Mimic dashboard's title row shape: bold title + raw spaces + muted date.
	title := s.Bold.Render("DASHBOARD") + "                              " + s.Muted.Render("May 4")
	out := s.RenderViewContent(title, 60, 3)

	// After repaintDesktop, every `\x1b[m` should be followed either
	// by another SGR open (`\x1b[`) or by the trailing pad/end of
	// string. There must NOT be a `\x1b[m` immediately followed by a
	// space — that would mean a raw-text gap is back to terminal
	// default.
	if idx := strings.Index(out, "\x1b[m "); idx >= 0 {
		t.Errorf("found naked reset followed by raw space at %d (terminal-default gap):\n%q", idx, out)
	}
}

// TestStyles_ApplyTheme_MenubarShortcut_TurboVision asserts that the
// menu-bar shortcut style picks up the theme's explicit shortcut color
// and underline setting. Turbo Vision uses red letters with no
// underline (menubar.shortcut.fg = "#aa0000",
// menubar.shortcut.underline = false), distinct from the default
// theme's same-color-underlined treatment.
func TestStyles_ApplyTheme_MenubarShortcut_TurboVision(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })

	turbo, _, err := theme.LoadBuiltin("turbo-vision")
	if err != nil {
		t.Fatalf("LoadBuiltin(turbo-vision): %v", err)
	}
	s := NewStyles()
	s.ApplyTheme(turbo)

	wantRed := lipgloss.Color("#aa0000")
	if got := s.MenuBarShortcut.GetForeground(); got != wantRed {
		t.Errorf("MenuBarShortcut.GetForeground() = %v, want %v", got, wantRed)
	}
	if s.MenuBarShortcut.GetUnderline() {
		t.Error("MenuBarShortcut.GetUnderline() = true, want false (TV uses color-only)")
	}
	if got := s.MenuBarActiveShortcut.GetForeground(); got != wantRed {
		t.Errorf("MenuBarActiveShortcut.GetForeground() = %v, want %v", got, wantRed)
	}
	if s.MenuBarActiveShortcut.GetUnderline() {
		t.Error("MenuBarActiveShortcut.GetUnderline() = true, want false")
	}
}

// TestStyles_ApplyTheme_MenubarShortcut_DefaultInherits asserts that
// when menubar.shortcut.fg is empty (default theme), the shortcut
// foreground inherits ColorHeaderFg so the letter is the same color
// as the rest of the menu — distinguished only by the underline. The
// active variant inherits ColorHeaderBg so it stays visible against
// the flipped active background.
func TestStyles_ApplyTheme_MenubarShortcut_DefaultInherits(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })

	def, _, err := theme.LoadBuiltin("default")
	if err != nil {
		t.Fatalf("LoadBuiltin(default): %v", err)
	}
	s := NewStyles()
	s.ApplyTheme(def)

	if got := s.MenuBarShortcut.GetForeground(); got != ColorHeaderFg {
		t.Errorf("MenuBarShortcut.GetForeground() = %v, want ColorHeaderFg=%v (inherited)", got, ColorHeaderFg)
	}
	if !s.MenuBarShortcut.GetUnderline() {
		t.Error("MenuBarShortcut.GetUnderline() = false, want true (default theme uses underline)")
	}
	if got := s.MenuBarActiveShortcut.GetForeground(); got != ColorHeaderBg {
		t.Errorf("MenuBarActiveShortcut.GetForeground() = %v, want ColorHeaderBg=%v (inverted)", got, ColorHeaderBg)
	}
	if !s.MenuBarActiveShortcut.GetUnderline() {
		t.Error("MenuBarActiveShortcut.GetUnderline() = false, want true")
	}
}

// TestStyles_ApplyTheme_DefaultDesktopTransparent asserts that the
// default theme's empty desktop.bg leaves Content and Sidebar with no
// background paint, so terminals show their own background through.
// This protects the "transparent passthrough" promise of the default
// palette across live-swap.
func TestStyles_ApplyTheme_DefaultDesktopTransparent(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })

	def, _, err := theme.LoadBuiltin("default")
	if err != nil {
		t.Fatalf("LoadBuiltin(default): %v", err)
	}

	s := NewStyles()
	s.ApplyTheme(def)

	if got := s.Content.GetBackground(); got != (lipgloss.NoColor{}) {
		t.Errorf("Content.GetBackground() = %v, want NoColor", got)
	}
	if got := s.Sidebar.GetBackground(); got != (lipgloss.NoColor{}) {
		t.Errorf("Sidebar.GetBackground() = %v, want NoColor", got)
	}
}

// TestStyles_ApplyTheme_DefaultRoundtrip applies the default theme and
// asserts the package-level Color* vars match the values shipped by
// styles.go. This protects against accidental drift between the
// embedded `themes/default.toml` and the in-code defaults.
func TestStyles_ApplyTheme_DefaultRoundtrip(t *testing.T) {
	t.Cleanup(func() { restoreDefaultTheme(t) })

	def, _, err := theme.LoadBuiltin("default")
	if err != nil {
		t.Fatalf("LoadBuiltin: %v", err)
	}

	s := NewStyles()
	s.ApplyTheme(def)

	cases := []struct {
		name string
		got  color.Color
		want color.Color
	}{
		{"Header.fg", s.Header.GetForeground(), lipgloss.Color("15")},
		{"Header.bg", s.Header.GetBackground(), lipgloss.Color("62")},
		{"Positive.fg", s.Positive.GetForeground(), lipgloss.Color("34")},
		{"Negative.fg", s.Negative.GetForeground(), lipgloss.Color("160")},
		{"Alert.fg", s.Alert.GetForeground(), lipgloss.Color("214")},
		{"StatusBar.fg", s.StatusBar.GetForeground(), lipgloss.Color("252")},
		{"StatusBar.bg", s.StatusBar.GetBackground(), lipgloss.Color("236")},
		{"SelectedRow.bg", s.SelectedRow.GetBackground(), lipgloss.Color("62")},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// TestStyles_PlaceholderAndOverlayBox verifies the Placeholder and
// OverlayBox fields are wired to the muted/border palette so live-swap
// in Phase 5 will actually propagate the theme to field placeholders
// and corporate-action overlay popups.
func TestStyles_PlaceholderAndOverlayBox(t *testing.T) {
	s := NewStyles()

	if got := s.Placeholder.GetForeground(); got != ColorMuted {
		t.Errorf("Placeholder foreground = %v, want %v", got, ColorMuted)
	}
	if got := s.OverlayBox.GetBorderTopForeground(); got != ColorBorder {
		t.Errorf("OverlayBox border foreground = %v, want %v", got, ColorBorder)
	}
	if !s.OverlayBox.GetBorderTop() {
		t.Error("OverlayBox should have a top border")
	}
	if got := s.OverlayBox.GetPaddingTop(); got != 1 {
		t.Errorf("OverlayBox padding-top = %d, want 1", got)
	}
	if got := s.OverlayBox.GetPaddingLeft(); got != 2 {
		t.Errorf("OverlayBox padding-left = %d, want 2", got)
	}
}

func TestBreakpointConstants(t *testing.T) {
	if BreakpointMedium >= BreakpointLarge {
		t.Errorf("BreakpointMedium (%d) should be less than BreakpointLarge (%d)",
			BreakpointMedium, BreakpointLarge)
	}
	if SidebarWidthMedium <= 0 {
		t.Errorf("SidebarWidthMedium should be positive, got %d", SidebarWidthMedium)
	}
	if SidebarWidthLarge <= SidebarWidthMedium {
		t.Errorf("SidebarWidthLarge (%d) should be greater than SidebarWidthMedium (%d)",
			SidebarWidthLarge, SidebarWidthMedium)
	}
}
