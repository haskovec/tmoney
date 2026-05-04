package tui

import (
	"image/color"
	"testing"
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
	expectedContent := 100 - SidebarWidthMedium - 1
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
	expectedContent := 120 - SidebarWidthLarge - 1
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
		{120, SidebarWidthLarge, 120 - SidebarWidthLarge - 1},                   // breakpoint: floor
		{150, SidebarWidthLarge + (150-120)/sidebarGrowthDivisor, 150 - 27 - 1}, // 27
		{200, SidebarWidthLarge + (200-120)/sidebarGrowthDivisor, 200 - 34 - 1}, // 34
		{400, SidebarWidthMax, 400 - SidebarWidthMax - 1},                       // capped
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
