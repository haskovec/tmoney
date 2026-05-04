package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
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
