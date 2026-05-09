// Package theme defines the data model for TMoney's skinnable theme
// system. Themes are loaded from TOML files; each theme controls the
// ~27 color slots, the global border style, three symbol strings, and
// a couple of boolean shortcut-display knobs that the TUI consumes.
//
// Background slots (`*.bg`) accept the empty string as a sentinel
// meaning "transparent — let the terminal default show through".
// Foreground slots and other text-bearing slots are populated in the
// default theme.
package theme

// BorderStyle is the global border style applied to bordered elements
// (windows, dialogs, tables, sidebar). One of the four enum values
// below; any other value in a TOML file is rejected per-slot and the
// default is restored.
type BorderStyle string

const (
	BorderSingle  BorderStyle = "single"
	BorderDouble  BorderStyle = "double"
	BorderRounded BorderStyle = "rounded"
	BorderThick   BorderStyle = "thick"
)

// IsValid reports whether b is one of the four allowed border styles.
func (b BorderStyle) IsValid() bool {
	switch b {
	case BorderSingle, BorderDouble, BorderRounded, BorderThick:
		return true
	}
	return false
}

// MenubarColors groups the menubar slots that share the `menubar.*`
// TOML prefix.
type MenubarColors struct {
	Fg       string         `toml:"fg"`
	Bg       string         `toml:"bg"`
	Active   FgBg           `toml:"active"`
	Shortcut ShortcutColors `toml:"shortcut"`
}

// StatusbarColors mirrors MenubarColors for the bottom status bar.
type StatusbarColors struct {
	Fg       string         `toml:"fg"`
	Bg       string         `toml:"bg"`
	Shortcut ShortcutColors `toml:"shortcut"`
}

// FgBg is a foreground+background pair, used for menubar.active and
// table.selected.
type FgBg struct {
	Fg string `toml:"fg"`
	Bg string `toml:"bg"`
}

// FgOnly is a foreground-only group. Used for slots like
// `window.border` and `window.title` whose TOML keys are
// `window.border.fg` and `window.title.fg` respectively.
type FgOnly struct {
	Fg string `toml:"fg"`
}

// ShortcutColors holds the optional shortcut-letter foreground and
// the underline knob for menu/status bars. An empty Fg means "inherit
// the parent menubar/statusbar foreground" at render time.
type ShortcutColors struct {
	Fg        string `toml:"fg"`
	Underline bool   `toml:"underline"`
}

// WindowColors holds the TOML group `window.*`.
type WindowColors struct {
	Bg     string `toml:"bg"`
	Fg     string `toml:"fg"`
	Border FgOnly `toml:"border"`
	Title  FgOnly `toml:"title"`
}

// DialogColors mirrors WindowColors for modal dialogs.
type DialogColors struct {
	Bg     string       `toml:"bg"`
	Fg     string       `toml:"fg"`
	Border FgOnly       `toml:"border"`
	Title  FgOnly       `toml:"title"`
	Button ButtonColors `toml:"button"`
}

// ButtonColors holds the optional dialog-button slots under
// `dialog.button.*`. All slots are optional: empty Fg/Bg renders the
// button as plain `[ Label ]` text, empty Focused.Fg/Focused.Bg
// renders the focused button with Reverse+Bold, and an empty
// Shortcut.Fg leaves the shortcut letter the same color as the rest
// of the label. Themes opt in by setting explicit colors.
type ButtonColors struct {
	Fg       string `toml:"fg"`
	Bg       string `toml:"bg"`
	Focused  FgBg   `toml:"focused"`
	Shortcut FgOnly `toml:"shortcut"`
}

// TableColors holds the TOML group `table.*`.
type TableColors struct {
	Header   FgOnly `toml:"header"`
	Row      FgOnly `toml:"row"`
	Selected FgBg   `toml:"selected"`
}

// TextColors holds the semantic text slots under `text.*`.
type TextColors struct {
	Positive string `toml:"positive"`
	Negative string `toml:"negative"`
	Alert    string `toml:"alert"`
	Muted    string `toml:"muted"`
	Title    string `toml:"title"`
	Error    string `toml:"error"`
}

// Symbols holds the three rendering glyphs that themes can override.
type Symbols struct {
	MenuSeparator  string `toml:"menu_separator"`
	FocusIndicator string `toml:"focus_indicator"`
	Checkmark      string `toml:"checkmark"`
}

// DesktopColors holds the TOML group `desktop.*`. v1 ignores the
// background at render time; the slot is reserved so theme files can
// set it for forward compatibility.
type DesktopColors struct {
	Bg string `toml:"bg"`
}

// Theme is the parsed in-memory representation of a theme TOML file.
// Field tags drive TOML decoding; struct nesting mirrors the dotted
// key hierarchy from the spec.
type Theme struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`

	// Version is reserved for future schema migrations. Parser ignores
	// it in v1 but accepts it without raising an unknown-key issue.
	Version int `toml:"version"`

	BorderStyle BorderStyle `toml:"border_style"`

	Desktop   DesktopColors   `toml:"desktop"`
	Menubar   MenubarColors   `toml:"menubar"`
	Statusbar StatusbarColors `toml:"statusbar"`
	Window    WindowColors    `toml:"window"`
	Dialog    DialogColors    `toml:"dialog"`
	Table     TableColors     `toml:"table"`
	Text      TextColors      `toml:"text"`
	Symbols   Symbols         `toml:"symbols"`
}

// defaultTheme returns a Theme populated from the existing palette in
// internal/tui/styles.go. This is the fallback used for any slot a
// user theme leaves out and the canonical reference for "today's
// look". Background slots are intentionally empty (transparent
// passthrough); every other slot is non-empty.
func defaultTheme() *Theme {
	return &Theme{
		Name:        "Default",
		Description: "TMoney's default palette (transparent backgrounds, ANSI 256 accents)",
		BorderStyle: BorderSingle,

		Desktop: DesktopColors{Bg: ""},

		Menubar: MenubarColors{
			Fg:     "15", // ColorHeaderFg
			Bg:     "62", // ColorHeaderBg
			Active: FgBg{Fg: "62", Bg: "15"},
			Shortcut: ShortcutColors{
				Fg:        "15",
				Underline: true,
			},
		},

		Statusbar: StatusbarColors{
			Fg: "252", // ColorStatusFg
			Bg: "236", // ColorStatusBg
			Shortcut: ShortcutColors{
				Fg:        "252",
				Underline: true,
			},
		},

		Window: WindowColors{
			Bg:     "",
			Fg:     "15",
			Border: FgOnly{Fg: "240"}, // ColorBorder
			Title:  FgOnly{Fg: "15"},  // ColorTitle
		},

		Dialog: DialogColors{
			Bg:     "",
			Fg:     "15",
			Border: FgOnly{Fg: "240"},
			Title:  FgOnly{Fg: "15"},
		},

		Table: TableColors{
			Header:   FgOnly{Fg: "15"},
			Row:      FgOnly{Fg: "15"},
			Selected: FgBg{Fg: "15", Bg: "62"},
		},

		Text: TextColors{
			Positive: "34",  // ColorPositive
			Negative: "160", // ColorNegative
			Alert:    "214", // ColorAlert
			Muted:    "245", // ColorMuted / ColorPending
			Title:    "15",  // ColorTitle
			Error:    "160", // Error reuses ColorNegative
		},

		Symbols: Symbols{
			MenuSeparator:  " │ ",
			FocusIndicator: "▶ ",
			Checkmark:      "✓",
		},
	}
}

// DefaultTheme returns a fresh copy of the built-in default theme. The
// returned pointer is safe to mutate; callers will not affect later
// callers.
func DefaultTheme() *Theme { return defaultTheme() }
