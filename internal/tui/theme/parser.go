package theme

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// IssueKind classifies a non-fatal problem found while parsing a
// theme file. Issues do not abort the load; the parser falls back to
// the default theme's value for the offending slot and continues.
type IssueKind int

const (
	// IssueMalformedValue indicates a known slot whose value is not a
	// recognized color or enum value. The slot reverts to the default
	// theme's value.
	IssueMalformedValue IssueKind = iota
	// IssueUnknownKey indicates a key in the TOML that does not map to
	// any known slot (likely a typo). The key is ignored.
	IssueUnknownKey
)

// Issue describes one non-fatal problem found while parsing.
type Issue struct {
	Kind   IssueKind
	Key    string // dotted path, e.g. "menubar.shortcut.fg"
	Value  string // offending raw value (only set for IssueMalformedValue)
	Reason string // short human-readable explanation
}

func (k IssueKind) String() string {
	switch k {
	case IssueMalformedValue:
		return "malformed-value"
	case IssueUnknownKey:
		return "unknown-key"
	}
	return "unknown"
}

// hexColorRE matches a 6-digit hex color, e.g. "#aabbcc" or "#FFFFFF".
var hexColorRE = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// isValidColor reports whether s is a recognized color value:
// the empty string (transparent sentinel), a 6-digit hex like
// "#rrggbb", or a stringified ANSI 256 number "0".."255".
func isValidColor(s string) bool {
	if s == "" {
		return true
	}
	if hexColorRE.MatchString(s) {
		return true
	}
	if n, err := strconv.Atoi(s); err == nil && n >= 0 && n <= 255 {
		return true
	}
	return false
}

// Parse decodes a theme TOML payload, validating each slot. The
// returned Theme always starts from defaultTheme() and selectively
// applies overrides — keys missing from the input keep the default
// value. Malformed slot values are reverted to the default and
// reported via the returned Issue list. Unknown keys are also
// reported. Only an unparseable TOML payload yields a non-nil error;
// in that case the returned *Theme is nil.
func Parse(data []byte) (*Theme, []Issue, error) {
	defaults := defaultTheme()
	t := defaultTheme()
	// Metadata is theme-specific; absence in the TOML must surface
	// as the zero value so the loader (or filename-stem fallback) can
	// fill it in. Colors and symbols still inherit from defaults.
	t.Name = ""
	t.Description = ""

	md, err := toml.Decode(string(data), t)
	if err != nil {
		return nil, nil, fmt.Errorf("parse theme TOML: %w", err)
	}

	var issues []Issue

	// Color-slot validation: for every known slot, if the TOML defined
	// it and the decoded value isn't a valid color, restore the
	// default's value and emit an issue.
	for _, slot := range colorSlots {
		if !md.IsDefined(slot.path...) {
			continue
		}
		val := slot.get(t)
		if isValidColor(val) {
			continue
		}
		slot.set(t, slot.get(defaults))
		issues = append(issues, Issue{
			Kind:   IssueMalformedValue,
			Key:    strings.Join(slot.path, "."),
			Value:  val,
			Reason: "not a valid color (expected #rrggbb, 0..255, or empty)",
		})
	}

	// border_style: enum validation.
	if md.IsDefined("border_style") && !t.BorderStyle.IsValid() {
		bad := string(t.BorderStyle)
		t.BorderStyle = defaults.BorderStyle
		issues = append(issues, Issue{
			Kind:   IssueMalformedValue,
			Key:    "border_style",
			Value:  bad,
			Reason: "must be one of: single, double, rounded, thick",
		})
	}

	// Unknown-key reporting.
	for _, k := range md.Undecoded() {
		issues = append(issues, Issue{
			Kind:   IssueUnknownKey,
			Key:    k.String(),
			Reason: "unknown key (typo?); ignored",
		})
	}

	return t, issues, nil
}

// ParseFromFile reads path and parses it as a theme TOML. If the
// theme's `name` field is empty after parsing, it falls back to the
// filename stem (e.g. "wal.toml" -> "wal").
func ParseFromFile(path string) (*Theme, []Issue, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, nil, err
	}
	t, issues, err := Parse(data)
	if err != nil {
		return nil, nil, err
	}
	if t.Name == "" {
		stem := filepath.Base(path)
		stem = strings.TrimSuffix(stem, filepath.Ext(stem))
		t.Name = stem
	}
	return t, issues, nil
}

// colorSlot binds a TOML dotted-key path to getter/setter closures so
// the parser can iterate every color slot uniformly.
type colorSlot struct {
	path []string
	get  func(*Theme) string
	set  func(*Theme, string)
}

// colorSlots enumerates every TOML key the parser treats as a color
// value. Used by Parse to validate inputs and by parser tests for
// completeness assertions.
var colorSlots = []colorSlot{
	{path: []string{"desktop", "bg"},
		get: func(t *Theme) string { return t.Desktop.Bg },
		set: func(t *Theme, v string) { t.Desktop.Bg = v }},

	{path: []string{"menubar", "fg"},
		get: func(t *Theme) string { return t.Menubar.Fg },
		set: func(t *Theme, v string) { t.Menubar.Fg = v }},
	{path: []string{"menubar", "bg"},
		get: func(t *Theme) string { return t.Menubar.Bg },
		set: func(t *Theme, v string) { t.Menubar.Bg = v }},
	{path: []string{"menubar", "active", "fg"},
		get: func(t *Theme) string { return t.Menubar.Active.Fg },
		set: func(t *Theme, v string) { t.Menubar.Active.Fg = v }},
	{path: []string{"menubar", "active", "bg"},
		get: func(t *Theme) string { return t.Menubar.Active.Bg },
		set: func(t *Theme, v string) { t.Menubar.Active.Bg = v }},
	{path: []string{"menubar", "shortcut", "fg"},
		get: func(t *Theme) string { return t.Menubar.Shortcut.Fg },
		set: func(t *Theme, v string) { t.Menubar.Shortcut.Fg = v }},

	{path: []string{"statusbar", "fg"},
		get: func(t *Theme) string { return t.Statusbar.Fg },
		set: func(t *Theme, v string) { t.Statusbar.Fg = v }},
	{path: []string{"statusbar", "bg"},
		get: func(t *Theme) string { return t.Statusbar.Bg },
		set: func(t *Theme, v string) { t.Statusbar.Bg = v }},
	{path: []string{"statusbar", "shortcut", "fg"},
		get: func(t *Theme) string { return t.Statusbar.Shortcut.Fg },
		set: func(t *Theme, v string) { t.Statusbar.Shortcut.Fg = v }},

	{path: []string{"window", "bg"},
		get: func(t *Theme) string { return t.Window.Bg },
		set: func(t *Theme, v string) { t.Window.Bg = v }},
	{path: []string{"window", "fg"},
		get: func(t *Theme) string { return t.Window.Fg },
		set: func(t *Theme, v string) { t.Window.Fg = v }},
	{path: []string{"window", "border", "fg"},
		get: func(t *Theme) string { return t.Window.Border.Fg },
		set: func(t *Theme, v string) { t.Window.Border.Fg = v }},
	{path: []string{"window", "title", "fg"},
		get: func(t *Theme) string { return t.Window.Title.Fg },
		set: func(t *Theme, v string) { t.Window.Title.Fg = v }},

	{path: []string{"dialog", "bg"},
		get: func(t *Theme) string { return t.Dialog.Bg },
		set: func(t *Theme, v string) { t.Dialog.Bg = v }},
	{path: []string{"dialog", "fg"},
		get: func(t *Theme) string { return t.Dialog.Fg },
		set: func(t *Theme, v string) { t.Dialog.Fg = v }},
	{path: []string{"dialog", "border", "fg"},
		get: func(t *Theme) string { return t.Dialog.Border.Fg },
		set: func(t *Theme, v string) { t.Dialog.Border.Fg = v }},
	{path: []string{"dialog", "title", "fg"},
		get: func(t *Theme) string { return t.Dialog.Title.Fg },
		set: func(t *Theme, v string) { t.Dialog.Title.Fg = v }},
	{path: []string{"dialog", "button", "fg"},
		get: func(t *Theme) string { return t.Dialog.Button.Fg },
		set: func(t *Theme, v string) { t.Dialog.Button.Fg = v }},
	{path: []string{"dialog", "button", "bg"},
		get: func(t *Theme) string { return t.Dialog.Button.Bg },
		set: func(t *Theme, v string) { t.Dialog.Button.Bg = v }},
	{path: []string{"dialog", "button", "focused", "fg"},
		get: func(t *Theme) string { return t.Dialog.Button.Focused.Fg },
		set: func(t *Theme, v string) { t.Dialog.Button.Focused.Fg = v }},
	{path: []string{"dialog", "button", "focused", "bg"},
		get: func(t *Theme) string { return t.Dialog.Button.Focused.Bg },
		set: func(t *Theme, v string) { t.Dialog.Button.Focused.Bg = v }},
	{path: []string{"dialog", "button", "shortcut", "fg"},
		get: func(t *Theme) string { return t.Dialog.Button.Shortcut.Fg },
		set: func(t *Theme, v string) { t.Dialog.Button.Shortcut.Fg = v }},

	{path: []string{"table", "header", "fg"},
		get: func(t *Theme) string { return t.Table.Header.Fg },
		set: func(t *Theme, v string) { t.Table.Header.Fg = v }},
	{path: []string{"table", "row", "fg"},
		get: func(t *Theme) string { return t.Table.Row.Fg },
		set: func(t *Theme, v string) { t.Table.Row.Fg = v }},
	{path: []string{"table", "selected", "fg"},
		get: func(t *Theme) string { return t.Table.Selected.Fg },
		set: func(t *Theme, v string) { t.Table.Selected.Fg = v }},
	{path: []string{"table", "selected", "bg"},
		get: func(t *Theme) string { return t.Table.Selected.Bg },
		set: func(t *Theme, v string) { t.Table.Selected.Bg = v }},

	{path: []string{"text", "positive"},
		get: func(t *Theme) string { return t.Text.Positive },
		set: func(t *Theme, v string) { t.Text.Positive = v }},
	{path: []string{"text", "negative"},
		get: func(t *Theme) string { return t.Text.Negative },
		set: func(t *Theme, v string) { t.Text.Negative = v }},
	{path: []string{"text", "alert"},
		get: func(t *Theme) string { return t.Text.Alert },
		set: func(t *Theme, v string) { t.Text.Alert = v }},
	{path: []string{"text", "muted"},
		get: func(t *Theme) string { return t.Text.Muted },
		set: func(t *Theme, v string) { t.Text.Muted = v }},
	{path: []string{"text", "title"},
		get: func(t *Theme) string { return t.Text.Title },
		set: func(t *Theme, v string) { t.Text.Title = v }},
	{path: []string{"text", "error"},
		get: func(t *Theme) string { return t.Text.Error },
		set: func(t *Theme, v string) { t.Text.Error = v }},
}
