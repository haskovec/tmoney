package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"
)

// WalColors mirrors the shape of pywal's `colors.json` output. Only
// the fields TMoney consumes when generating a theme are decoded; any
// extra keys (e.g. `wallpaper`, `alpha`, `special.cursor`) are kept
// intact for round-tripping but unused by the helper.
type WalColors struct {
	Wallpaper string        `json:"wallpaper"`
	Alpha     string        `json:"alpha"`
	Special   WalSpecial    `json:"special"`
	Colors    WalColorTable `json:"colors"`
}

// WalSpecial holds pywal's `special.*` group (background, foreground,
// cursor). Cursor is kept for completeness but unused by the slot
// mapping in v1.
type WalSpecial struct {
	Background string `json:"background"`
	Foreground string `json:"foreground"`
	Cursor     string `json:"cursor"`
}

// WalColorTable holds pywal's `colors.color0` … `colors.color15`
// 16-entry palette.
type WalColorTable struct {
	Color0  string `json:"color0"`
	Color1  string `json:"color1"`
	Color2  string `json:"color2"`
	Color3  string `json:"color3"`
	Color4  string `json:"color4"`
	Color5  string `json:"color5"`
	Color6  string `json:"color6"`
	Color7  string `json:"color7"`
	Color8  string `json:"color8"`
	Color9  string `json:"color9"`
	Color10 string `json:"color10"`
	Color11 string `json:"color11"`
	Color12 string `json:"color12"`
	Color13 string `json:"color13"`
	Color14 string `json:"color14"`
	Color15 string `json:"color15"`
}

// ReadWalColors parses the pywal `colors.json` file at path. A missing
// file produces the user-facing message documented in
// specs/theming.md so the CLI can surface it verbatim and exit 1.
// Unparseable JSON is wrapped with the path for context.
func ReadWalColors(path string) (*WalColors, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("pywal cache not found at %s — is pywal installed and has it run?", path)
		}
		return nil, fmt.Errorf("read pywal colors %s: %w", path, err)
	}
	var wc WalColors
	if err := json.Unmarshal(data, &wc); err != nil {
		return nil, fmt.Errorf("parse pywal colors %s: %w", path, err)
	}
	return &wc, nil
}

// walToThemeTOML renders a TMoney theme TOML file from a parsed pywal
// palette. The slot mapping follows the table in specs/theming.md
// (Pywal Helper → Slot mapping). Symbol slots and shortcut display
// options are intentionally omitted so they fall back to the default
// theme. sourcePath is embedded in the comment header so users can see
// where the colors came from; ts is the generation timestamp (RFC3339).
func walToThemeTOML(wc *WalColors, sourcePath string, ts time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Generated from %s on %s.\n", sourcePath, ts.UTC().Format(time.RFC3339))
	b.WriteString("# Re-run `tmoney theme generate-from-wal` to regenerate after pywal updates.\n")
	b.WriteString("# Live-swap is not automatic — re-select 'wal' in View → Theme to apply.\n")
	b.WriteString("\n")
	b.WriteString(`name = "wal"` + "\n")
	b.WriteString(`description = "Generated from pywal palette"` + "\n")
	b.WriteString("\n")
	b.WriteString(`border_style = "single"` + "\n")
	b.WriteString("\n")

	b.WriteString("# Desktop — reserved; not painted in v1.\n")
	fmt.Fprintf(&b, "# desktop.bg = %q\n", wc.Special.Background)
	b.WriteString("\n")

	b.WriteString("# Windows.\n")
	fmt.Fprintf(&b, "window.bg = %q\n", wc.Special.Background)
	fmt.Fprintf(&b, "window.fg = %q\n", wc.Special.Foreground)
	fmt.Fprintf(&b, "window.border.fg = %q\n", wc.Colors.Color7)
	fmt.Fprintf(&b, "window.title.fg = %q\n", wc.Colors.Color3)
	b.WriteString("\n")

	b.WriteString("# Menu bar.\n")
	fmt.Fprintf(&b, "menubar.bg = %q\n", wc.Colors.Color0)
	fmt.Fprintf(&b, "menubar.fg = %q\n", wc.Colors.Color7)
	fmt.Fprintf(&b, "menubar.active.bg = %q\n", wc.Colors.Color4)
	fmt.Fprintf(&b, "menubar.active.fg = %q\n", wc.Special.Background)
	fmt.Fprintf(&b, "menubar.shortcut.fg = %q\n", wc.Colors.Color1)
	b.WriteString("\n")

	b.WriteString("# Status bar.\n")
	fmt.Fprintf(&b, "statusbar.bg = %q\n", wc.Colors.Color0)
	fmt.Fprintf(&b, "statusbar.fg = %q\n", wc.Colors.Color7)
	b.WriteString("\n")

	b.WriteString("# Dialogs.\n")
	fmt.Fprintf(&b, "dialog.bg = %q\n", wc.Colors.Color8)
	fmt.Fprintf(&b, "dialog.fg = %q\n", wc.Special.Foreground)
	fmt.Fprintf(&b, "dialog.border.fg = %q\n", wc.Colors.Color7)
	fmt.Fprintf(&b, "dialog.title.fg = %q\n", wc.Colors.Color3)
	b.WriteString("\n")

	b.WriteString("# Tables.\n")
	fmt.Fprintf(&b, "table.header.fg = %q\n", wc.Colors.Color3)
	fmt.Fprintf(&b, "table.row.fg = %q\n", wc.Special.Foreground)
	fmt.Fprintf(&b, "table.selected.bg = %q\n", wc.Colors.Color4)
	fmt.Fprintf(&b, "table.selected.fg = %q\n", wc.Special.Background)
	b.WriteString("\n")

	b.WriteString("# Semantic text.\n")
	fmt.Fprintf(&b, "text.positive = %q\n", wc.Colors.Color2)
	fmt.Fprintf(&b, "text.negative = %q\n", wc.Colors.Color1)
	fmt.Fprintf(&b, "text.alert = %q\n", wc.Colors.Color3)
	fmt.Fprintf(&b, "text.muted = %q\n", wc.Colors.Color8)
	fmt.Fprintf(&b, "text.title = %q\n", wc.Special.Foreground)
	fmt.Fprintf(&b, "text.error = %q\n", wc.Colors.Color1)

	return b.String()
}
