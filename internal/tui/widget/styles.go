package widget

import (
	"image/color"
	"strings"

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
	// ColorMenubarShortcutFg is the foreground for the shortcut letter
	// in menu-bar items (e.g. the "F" in "File"). NoColor{} means
	// "inherit from ColorHeaderFg" — the default theme look where the
	// letter is the same color as the rest, distinguished only by the
	// underline. Turbo Vision sets this to red so the shortcut letter
	// stands out on the gray menu bar without an underline.
	ColorMenubarShortcutFg color.Color = lipgloss.NoColor{}
	// ColorDialogBg paints the dialog box background. NoColor{} leaves
	// the terminal default visible (today's look); themes set it for a
	// solid panel (Turbo Vision gray, light theme off-white).
	ColorDialogBg color.Color = lipgloss.NoColor{}
	// ColorDialogFg / ColorDialogBorder / ColorDialogTitle hold the
	// theme's dialog foreground/border/title colors. Empty defaults to
	// the existing ColorTitle / ColorBorder behavior; explicit values
	// override.
	ColorDialogFg     color.Color = lipgloss.NoColor{}
	ColorDialogBorder color.Color = ColorBorder
	ColorDialogTitle  color.Color = ColorTitle
	// ColorDialogButtonFg / ColorDialogButtonBg are the unfocused
	// dialog-button face. NoColor{} on both means "render as plain
	// `[ Label ]` text" (today's look).
	ColorDialogButtonFg color.Color = lipgloss.NoColor{}
	ColorDialogButtonBg color.Color = lipgloss.NoColor{}
	// ColorDialogButtonFocusedFg / ColorDialogButtonFocusedBg are the
	// focused dialog-button face. NoColor{} on both means "use Reverse
	// + Bold" (today's focused look).
	ColorDialogButtonFocusedFg color.Color = lipgloss.NoColor{}
	ColorDialogButtonFocusedBg color.Color = lipgloss.NoColor{}
	// ColorDialogButtonShortcutFg colors the first letter of a focused
	// button's label (the Turbo Vision yellow-letter highlight). NoColor{}
	// disables the highlight so the letter takes the focused button's
	// regular foreground color.
	ColorDialogButtonShortcutFg color.Color = lipgloss.NoColor{}
)

// IsTransparent reports whether c is the lipgloss.NoColor sentinel.
func IsTransparent(c color.Color) bool {
	_, ok := c.(lipgloss.NoColor)
	return ok
}

// MenubarShortcutUnderline controls whether the menu-bar shortcut
// letter is underlined. Themes set this via `menubar.shortcut.underline`
// — Turbo Vision turns it off because the red color already
// distinguishes the letter; the default theme keeps it on.
var MenubarShortcutUnderline = true

// menubarShortcutInherits is true when the theme's shortcut fg is the
// same color as the menu bar's main fg (or unset). In that case the
// shortcut letter is meant to look identical to the rest in the
// inactive state, distinguished only by the underline; the active
// variant then inverts to ColorHeaderBg so the letter stays visible
// against the flipped active background. Themes that override
// shortcut.fg with a contrasting color (Turbo Vision red) opt out of
// the inversion and keep the explicit color in both states.
var menubarShortcutInherits = true

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
	Dialog               lipgloss.Style
	DialogTitle          lipgloss.Style
	DialogButton         lipgloss.Style
	DialogButtonFocused  lipgloss.Style
	DialogButtonShortcut lipgloss.Style

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

	// Dialog. ColorDialogBg paints the panel; transparent leaves the
	// terminal default (today's look). When a theme sets dialog.bg, we
	// also extend the border background so the rounded-border edges
	// blend with the panel rather than punching a transparent hole.
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorDialogBorder).
		Padding(1, 2)
	if !IsTransparent(ColorDialogBg) {
		dialogStyle = dialogStyle.
			Background(ColorDialogBg).
			BorderBackground(ColorDialogBg)
		if !IsTransparent(ColorDialogFg) {
			dialogStyle = dialogStyle.Foreground(ColorDialogFg)
		}
	}
	s.Dialog = dialogStyle

	dialogTitleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorDialogTitle)
	if !IsTransparent(ColorDialogBg) {
		dialogTitleStyle = dialogTitleStyle.Background(ColorDialogBg)
	}
	s.DialogTitle = dialogTitleStyle

	// Unfocused dialog button. When neither fg nor bg is set we render
	// plain `[ Label ]` text (today's look). Themes opt in by setting
	// dialog.button.fg/bg.
	btnStyle := lipgloss.NewStyle()
	if !IsTransparent(ColorDialogButtonFg) {
		btnStyle = btnStyle.Foreground(ColorDialogButtonFg)
	}
	if !IsTransparent(ColorDialogButtonBg) {
		btnStyle = btnStyle.Background(ColorDialogButtonBg)
	}
	s.DialogButton = btnStyle

	// Focused dialog button. When neither fg nor bg is set we fall back
	// to the original Reverse+Bold treatment so existing themes keep
	// working unchanged.
	if IsTransparent(ColorDialogButtonFocusedFg) && IsTransparent(ColorDialogButtonFocusedBg) {
		s.DialogButtonFocused = lipgloss.NewStyle().Reverse(true).Bold(true)
	} else {
		focusedStyle := lipgloss.NewStyle().Bold(true)
		if !IsTransparent(ColorDialogButtonFocusedFg) {
			focusedStyle = focusedStyle.Foreground(ColorDialogButtonFocusedFg)
		}
		if !IsTransparent(ColorDialogButtonFocusedBg) {
			focusedStyle = focusedStyle.Background(ColorDialogButtonFocusedBg)
		}
		s.DialogButtonFocused = focusedStyle
	}

	// Shortcut letter on the focused button (Turbo Vision's yellow first
	// letter on the highlighted action). Inherits the focused button's
	// background so the colored letter sits on the same face. When the
	// theme leaves shortcut.fg unset, the style is the focused-button
	// style so renderButtonRow can apply it uniformly without branching.
	if IsTransparent(ColorDialogButtonShortcutFg) {
		s.DialogButtonShortcut = s.DialogButtonFocused
	} else {
		shortcutStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorDialogButtonShortcutFg)
		if !IsTransparent(ColorDialogButtonFocusedBg) {
			shortcutStyle = shortcutStyle.Background(ColorDialogButtonFocusedBg)
		}
		s.DialogButtonShortcut = shortcutStyle
	}

	// Menu bar
	s.MenuBarItem = lipgloss.NewStyle().
		Foreground(ColorHeaderFg).
		Background(ColorHeaderBg)

	s.MenuBarActive = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorHeaderBg).
		Background(ColorHeaderFg)

	// Shortcut letter on the inactive (normal) menu bar. When the theme
	// sets menubar.shortcut.fg, use that explicit color (e.g. red for
	// Turbo Vision). When it's unset, inherit ColorHeaderFg so the
	// letter is the same color as the rest and only the underline marks
	// it — today's default look.
	shortcutFg := ColorMenubarShortcutFg
	if _, transparent := shortcutFg.(lipgloss.NoColor); transparent {
		shortcutFg = ColorHeaderFg
	}
	s.MenuBarShortcut = lipgloss.NewStyle().
		Foreground(shortcutFg).
		Background(ColorHeaderBg).
		Underline(MenubarShortcutUnderline)

	// Shortcut letter on the active (highlighted) menu item. When the
	// theme's shortcut fg matches menubar.fg (or is empty), the user
	// wants the inherited "underline-only" treatment, so we invert to
	// ColorHeaderBg to stay visible on the flipped active background.
	// When the theme overrides shortcut.fg with a contrasting color
	// (Turbo Vision's red), reuse it in the active state too — the
	// letter color is consistent across states and was chosen to be
	// readable on both the inactive and active backgrounds.
	activeShortcutFg := shortcutFg
	if menubarShortcutInherits {
		activeShortcutFg = ColorHeaderBg
	}
	s.MenuBarActiveShortcut = lipgloss.NewStyle().
		Bold(true).
		Foreground(activeShortcutFg).
		Background(ColorHeaderFg).
		Underline(MenubarShortcutUnderline)

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

// ApplyTheme reseeds the package-level Color* vars from t and rebuilds
// every Style field in s from those vars. This is the live-swap entry
// point: Phase 5's reloadTheme calls ApplyTheme followed by Resize so
// the next render reflects the new palette.
//
// The Color* vars are kept up to date defensively — every theme-relevant
// inline call site was promoted to a Styles field in TH-018, but if a
// new inline slips in it will still see the active theme's color rather
// than a stale boot-time value.
func (s *Styles) ApplyTheme(t *theme.Theme) {
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
	ColorMenubarShortcutFg = themeColor(t.Menubar.Shortcut.Fg)
	MenubarShortcutUnderline = t.Menubar.Shortcut.Underline
	menubarShortcutInherits = t.Menubar.Shortcut.Fg == "" || t.Menubar.Shortcut.Fg == t.Menubar.Fg

	ColorDialogBg = themeColor(t.Dialog.Bg)
	ColorDialogFg = themeColor(t.Dialog.Fg)
	ColorDialogBorder = themeColor(t.Dialog.Border.Fg)
	ColorDialogTitle = themeColor(t.Dialog.Title.Fg)
	if IsTransparent(ColorDialogBorder) {
		ColorDialogBorder = ColorBorder
	}
	if IsTransparent(ColorDialogTitle) {
		ColorDialogTitle = ColorTitle
	}
	ColorDialogButtonFg = themeColor(t.Dialog.Button.Fg)
	ColorDialogButtonBg = themeColor(t.Dialog.Button.Bg)
	ColorDialogButtonFocusedFg = themeColor(t.Dialog.Button.Focused.Fg)
	ColorDialogButtonFocusedBg = themeColor(t.Dialog.Button.Focused.Bg)
	ColorDialogButtonShortcutFg = themeColor(t.Dialog.Button.Shortcut.Fg)

	s.initBaseStyles()
}

// bgSGR returns the "set background" SGR sequence lipgloss emits for c,
// or "" when c is transparent. We render a marker character with c as
// the background and split off the prefix; this gives a bit-for-bit
// match with what lipgloss would emit so re-emitting it via repaintBg
// is idempotent.
func bgSGR(c color.Color) string {
	if _, transparent := c.(lipgloss.NoColor); transparent {
		return ""
	}
	sample := lipgloss.NewStyle().Background(c).Render("\x00")
	idx := strings.IndexByte(sample, 0)
	if idx <= 0 {
		return ""
	}
	return sample[:idx]
}

// RepaintBg re-emits the background SGR for c after every SGR full-reset
// in s. Without this, inner styled spans (Bold, Muted, Placeholder, etc.)
// close with `\x1b[m` which clears the outer Background, and any
// raw-text gap or following styled chunk shows terminal-default until
// the next render boundary — visible as dark bands inside a colored
// region (Turbo Vision desktop fill, dialog panel, etc.).
//
// No-op when c is transparent.
func RepaintBg(s string, c color.Color) string {
	bg := bgSGR(c)
	if bg == "" {
		return s
	}
	// `\x1b[0m` is checked first; `\x1b[m` is not a substring of
	// `\x1b[0m` (third byte differs) so the second pass is safe.
	s = strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+bg)
	s = strings.ReplaceAll(s, "\x1b[m", "\x1b[m"+bg)
	return s
}

// repaintDesktop is repaintBg specialized to the active desktop color.
// Used by the main content area to keep the Turbo Vision blue fill
// continuous through inner SGR resets.
func repaintDesktop(s string) string {
	return RepaintBg(s, ColorDesktopBg)
}

// RenderViewContent wraps viewContent in the Content style at the given
// dimensions, repainting the desktop background on inner SGR resets so
// styled chunks and raw-text gaps don't punch holes through the fill.
// Use this in place of `s.Content.Width(...).Height(...).Render(...)`
// for any string that holds the main content area for a view.
func (s *Styles) RenderViewContent(viewContent string, width, height int) string {
	return s.Content.
		Width(width).
		Height(height).
		Render(repaintDesktop(viewContent))
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
	// The sidebar's BorderRight is already counted inside its Width
	// budget — lipgloss includes borders in Width() — so the content area
	// fills the rest of the terminal exactly. Subtracting an extra column
	// here used to leave a 1-char gap of bare terminal background on the
	// far right of every two-pane view.
	return s.width - sw
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
