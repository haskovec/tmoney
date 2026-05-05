package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/tui/theme"
)

// LayoutMode represents the responsive layout mode based on terminal width.
type LayoutMode int

const (
	// LayoutSmall is for terminals under 80 columns (sidebar collapsed, single-pane).
	LayoutSmall LayoutMode = iota
	// LayoutMedium is for terminals 80-120 columns (standard two-pane layout).
	LayoutMedium
	// LayoutLarge is for terminals over 120 columns (additional detail panels).
	LayoutLarge
)

// Layout breakpoint thresholds.
const (
	BreakpointMedium = 80
	BreakpointLarge  = 120
)

// Sidebar width constants.
const (
	SidebarWidthMedium = 20
	// SidebarWidthLarge is the floor used at the large-layout breakpoint.
	// Beyond that breakpoint the sidebar grows with terminal width up to
	// SidebarWidthMax so long account names like "Wealthfront Joint
	// Checking" stop getting truncated on wide terminals.
	SidebarWidthLarge = 24
	SidebarWidthMax   = 40
	// sidebarGrowthDivisor controls how aggressively the sidebar grows
	// past BreakpointLarge: one extra column per N extra terminal columns.
	sidebarGrowthDivisor = 8
)

// Color palette using ANSI 256 color codes for broad terminal compatibility.
// The design uses terminal default background for transparency.
var (
	// ColorPositive is green for positive amounts / income.
	ColorPositive = lipgloss.Color("34")
	// ColorNegative is red for negative amounts / expenses.
	ColorNegative = lipgloss.Color("160")
	// ColorPending is dim gray for pending/uncleared items.
	ColorPending = lipgloss.Color("245")
	// ColorAlert is yellow/orange for alerts and due items.
	ColorAlert = lipgloss.Color("214")
	// ColorBorder is subtle/dim for borders.
	ColorBorder = lipgloss.Color("240")
	// ColorHeaderFg is white for header foreground text.
	ColorHeaderFg = lipgloss.Color("15")
	// ColorHeaderBg is a purple accent for the header/menu bar background.
	ColorHeaderBg = lipgloss.Color("62")
	// ColorStatusFg is light gray for status bar text.
	ColorStatusFg = lipgloss.Color("252")
	// ColorStatusBg is dark gray for status bar background.
	ColorStatusBg = lipgloss.Color("236")
	// ColorSelectedFg is the foreground for selected/highlighted rows.
	ColorSelectedFg = lipgloss.Color("15")
	// ColorSelectedBg is the background for selected/highlighted rows.
	ColorSelectedBg = lipgloss.Color("62")
	// ColorMuted is a dim color for secondary/less important text.
	ColorMuted = lipgloss.Color("245")
	// ColorTitle is bold white for section titles.
	ColorTitle = lipgloss.Color("15")
	// ColorDesktopBg paints the otherwise-empty cells in the main content
	// area and the sidebar. lipgloss.NoColor{} means "transparent" — short
	// rows and unused vertical space show the terminal default. A real
	// color gives the Turbo Vision "blue desktop" look.
	ColorDesktopBg color.Color = lipgloss.NoColor{}
)

// Styles holds all the reusable lipgloss styles for the application.
type Styles struct {
	// Layout
	width  int
	height int

	// Header / Menu bar
	Header lipgloss.Style

	// Status bar
	StatusBar lipgloss.Style

	// Content area
	Content lipgloss.Style

	// Sidebar
	Sidebar      lipgloss.Style
	SidebarItem  lipgloss.Style
	SidebarGroup lipgloss.Style

	// Titles and headings
	Title       lipgloss.Style
	SectionHead lipgloss.Style

	// Table styles
	TableHeader lipgloss.Style
	TableRow    lipgloss.Style
	SelectedRow lipgloss.Style

	// Amount styles
	Positive lipgloss.Style
	Negative lipgloss.Style
	Pending  lipgloss.Style

	// Special row styles
	VoidRow lipgloss.Style

	// Alerts and notifications
	Alert lipgloss.Style

	// Error display
	Error lipgloss.Style
	// FieldError is a compact inline error style for dialog field validation.
	FieldError lipgloss.Style

	// Dialog / Modal
	Dialog       lipgloss.Style
	DialogTitle  lipgloss.Style
	DialogButton lipgloss.Style

	// Menu bar
	MenuBarItem           lipgloss.Style
	MenuBarActive         lipgloss.Style
	MenuBarShortcut       lipgloss.Style
	MenuBarActiveShortcut lipgloss.Style
	MenuDropdownItem      lipgloss.Style
	MenuDropdownActive    lipgloss.Style

	// Borders
	Border lipgloss.Style

	// General text styles
	Bold  lipgloss.Style
	Muted lipgloss.Style

	// Placeholder is used for placeholder text inside text fields.
	Placeholder lipgloss.Style

	// OverlayBox is the base style for overlay popups (corporate action
	// history, merger confirmation, etc.). Callers add a .Width(...).
	OverlayBox lipgloss.Style
}

// NewStyles creates a new Styles instance with default dimensions.
func NewStyles() Styles {
	s := Styles{}
	s.initBaseStyles()
	return s
}

// initBaseStyles sets up all the base styles without width/height constraints.
func (s *Styles) initBaseStyles() {
	// Header / Menu bar
	s.Header = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorHeaderFg).
		Background(ColorHeaderBg).
		Padding(0, 1)

	// Status bar
	s.StatusBar = lipgloss.NewStyle().
		Foreground(ColorStatusFg).
		Background(ColorStatusBg).
		Padding(0, 1)

	// Content area. Background paints empty cells (short content rows
	// and unused rows below content) when the active theme sets
	// `desktop.bg`. NoColor leaves the terminal default visible.
	s.Content = lipgloss.NewStyle().
		Background(ColorDesktopBg)

	// Sidebar. Same desktop-bg treatment as Content so the two panes
	// share a continuous backdrop.
	s.Sidebar = lipgloss.NewStyle().
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorBorder).
		Background(ColorDesktopBg)

	s.SidebarItem = lipgloss.NewStyle().
		PaddingLeft(2)

	s.SidebarGroup = lipgloss.NewStyle().
		Bold(true).
		PaddingLeft(1)

	// Titles
	s.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorTitle)

	s.SectionHead = lipgloss.NewStyle().
		Bold(true).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorBorder)

	// Table
	s.TableHeader = lipgloss.NewStyle().
		Bold(true).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorBorder)

	s.TableRow = lipgloss.NewStyle()

	s.SelectedRow = lipgloss.NewStyle().
		Foreground(ColorSelectedFg).
		Background(ColorSelectedBg)

	// Amounts
	s.Positive = lipgloss.NewStyle().
		Foreground(ColorPositive)

	s.Negative = lipgloss.NewStyle().
		Foreground(ColorNegative)

	s.Pending = lipgloss.NewStyle().
		Foreground(ColorPending)

	// Special row styles
	s.VoidRow = lipgloss.NewStyle().
		Foreground(ColorMuted).
		Strikethrough(true)

	// Alerts
	s.Alert = lipgloss.NewStyle().
		Foreground(ColorAlert).
		Bold(true)

	// Error
	s.Error = lipgloss.NewStyle().
		Foreground(ColorNegative).
		Bold(true).
		Padding(1, 2)

	// Field-level inline error (compact, no padding)
	s.FieldError = lipgloss.NewStyle().
		Foreground(ColorNegative)

	// Dialog
	s.Dialog = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(1, 2)

	s.DialogTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorTitle)

	s.DialogButton = lipgloss.NewStyle().
		Padding(0, 2).
		Border(lipgloss.NormalBorder()).
		BorderForeground(ColorBorder)

	// Menu bar
	s.MenuBarItem = lipgloss.NewStyle().
		Foreground(ColorHeaderFg).
		Background(ColorHeaderBg)

	s.MenuBarActive = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorHeaderBg).
		Background(ColorHeaderFg)

	s.MenuBarShortcut = lipgloss.NewStyle().
		Foreground(ColorHeaderFg).
		Background(ColorHeaderBg).
		Underline(true)

	s.MenuBarActiveShortcut = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorHeaderBg).
		Background(ColorHeaderFg).
		Underline(true)

	s.MenuDropdownItem = lipgloss.NewStyle().
		Foreground(ColorStatusFg).
		Background(ColorStatusBg)

	s.MenuDropdownActive = lipgloss.NewStyle().
		Foreground(ColorSelectedFg).
		Background(ColorSelectedBg)

	// Border
	s.Border = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(ColorBorder)

	// Text
	s.Bold = lipgloss.NewStyle().
		Bold(true)

	s.Muted = lipgloss.NewStyle().
		Foreground(ColorMuted)

	s.Placeholder = lipgloss.NewStyle().
		Foreground(ColorMuted)

	s.OverlayBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(1, 2)
}

// themeColor turns a theme slot string into a renderable color.
// Empty input means "transparent / use the terminal default" and is
// represented by lipgloss.NoColor{}.
func themeColor(s string) color.Color {
	if s == "" {
		return lipgloss.NoColor{}
	}
	return lipgloss.Color(s)
}

// applyTheme reseeds the package-level Color* vars from t and rebuilds
// every Style field in s from those vars. This is the live-swap entry
// point: Phase 5's reloadTheme calls applyTheme followed by Resize so
// the next render reflects the new palette.
//
// The Color* vars are kept up to date defensively — every theme-relevant
// inline call site was promoted to a Styles field in TH-018, but if a
// new inline slips in it will still see the active theme's color rather
// than a stale boot-time value.
func (s *Styles) applyTheme(t *theme.Theme) {
	ColorPositive = themeColor(t.Text.Positive)
	ColorNegative = themeColor(t.Text.Negative)
	ColorPending = themeColor(t.Text.Muted)
	ColorAlert = themeColor(t.Text.Alert)
	ColorBorder = themeColor(t.Window.Border.Fg)
	ColorHeaderFg = themeColor(t.Menubar.Fg)
	ColorHeaderBg = themeColor(t.Menubar.Bg)
	ColorStatusFg = themeColor(t.Statusbar.Fg)
	ColorStatusBg = themeColor(t.Statusbar.Bg)
	ColorSelectedFg = themeColor(t.Table.Selected.Fg)
	ColorSelectedBg = themeColor(t.Table.Selected.Bg)
	ColorMuted = themeColor(t.Text.Muted)
	ColorTitle = themeColor(t.Text.Title)
	ColorDesktopBg = themeColor(t.Desktop.Bg)

	s.initBaseStyles()
}

// Resize updates all width-dependent styles for the given terminal dimensions.
func (s *Styles) Resize(width, height int) {
	s.width = width
	s.height = height

	s.Header = s.Header.Width(width)
	s.StatusBar = s.StatusBar.Width(width)
	s.Content = s.Content.Width(width)

	s.Sidebar = s.Sidebar.Width(computeSidebarWidth(width))
}

// ContentWidth returns the available width for the main content area,
// accounting for the sidebar in two-pane layouts.
func (s *Styles) ContentWidth() int {
	sw := computeSidebarWidth(s.width)
	if sw == 0 {
		return s.width
	}
	// Sidebar width + 1 for the border column.
	return s.width - sw - 1
}

// SidebarWidth returns the sidebar width for the current layout mode.
func (s *Styles) SidebarWidth() int {
	return computeSidebarWidth(s.width)
}

// computeSidebarWidth picks a sidebar width given terminal width. Below
// the medium breakpoint it returns 0 (no sidebar); at the medium
// breakpoint it returns SidebarWidthMedium; at the large breakpoint and
// above it grows past SidebarWidthLarge proportionally with terminal
// width, capped at SidebarWidthMax.
func computeSidebarWidth(width int) int {
	switch GetLayoutMode(width) {
	case LayoutSmall:
		return 0
	case LayoutMedium:
		return SidebarWidthMedium
	case LayoutLarge:
		return min(SidebarWidthLarge+(width-BreakpointLarge)/sidebarGrowthDivisor, SidebarWidthMax)
	}
	return 0
}

// GetLayoutMode determines the layout mode based on terminal width.
func GetLayoutMode(width int) LayoutMode {
	if width < BreakpointMedium {
		return LayoutSmall
	}
	if width < BreakpointLarge {
		return LayoutMedium
	}
	return LayoutLarge
}
