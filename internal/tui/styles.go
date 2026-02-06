package tui

import "github.com/charmbracelet/lipgloss"

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
	SidebarWidthLarge  = 24
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

	// Alerts and notifications
	Alert lipgloss.Style

	// Error display
	Error lipgloss.Style

	// Dialog / Modal
	Dialog       lipgloss.Style
	DialogTitle  lipgloss.Style
	DialogButton lipgloss.Style

	// Menu bar
	MenuBarItem        lipgloss.Style
	MenuBarActive      lipgloss.Style
	MenuDropdownItem   lipgloss.Style
	MenuDropdownActive lipgloss.Style

	// Borders
	Border lipgloss.Style

	// General text styles
	Bold  lipgloss.Style
	Muted lipgloss.Style
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

	// Content area
	s.Content = lipgloss.NewStyle()

	// Sidebar
	s.Sidebar = lipgloss.NewStyle().
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorBorder)

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

	// Alerts
	s.Alert = lipgloss.NewStyle().
		Foreground(ColorAlert).
		Bold(true)

	// Error
	s.Error = lipgloss.NewStyle().
		Foreground(ColorNegative).
		Bold(true).
		Padding(1, 2)

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
}

// Resize updates all width-dependent styles for the given terminal dimensions.
func (s *Styles) Resize(width, height int) {
	s.width = width
	s.height = height

	s.Header = s.Header.Width(width)
	s.StatusBar = s.StatusBar.Width(width)
	s.Content = s.Content.Width(width)

	mode := GetLayoutMode(width)
	switch mode {
	case LayoutSmall:
		// No sidebar — full width for content
		s.Sidebar = s.Sidebar.Width(0)
	case LayoutMedium:
		s.Sidebar = s.Sidebar.Width(SidebarWidthMedium)
	case LayoutLarge:
		s.Sidebar = s.Sidebar.Width(SidebarWidthLarge)
	}
}

// ContentWidth returns the available width for the main content area,
// accounting for the sidebar in two-pane layouts.
func (s *Styles) ContentWidth() int {
	mode := GetLayoutMode(s.width)
	switch mode {
	case LayoutSmall:
		return s.width
	case LayoutMedium:
		// Sidebar width + 1 for border
		return s.width - SidebarWidthMedium - 1
	case LayoutLarge:
		return s.width - SidebarWidthLarge - 1
	}
	return s.width
}

// SidebarWidth returns the sidebar width for the current layout mode.
func (s *Styles) SidebarWidth() int {
	mode := GetLayoutMode(s.width)
	switch mode {
	case LayoutSmall:
		return 0
	case LayoutMedium:
		return SidebarWidthMedium
	case LayoutLarge:
		return SidebarWidthLarge
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
