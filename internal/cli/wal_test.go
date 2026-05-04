package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/tui/theme"
)

func TestReadWalColors_Sample(t *testing.T) {
	wc, err := ReadWalColors(filepath.Join("testdata", "wal-sample-colors.json"))
	if err != nil {
		t.Fatalf("ReadWalColors returned unexpected error: %v", err)
	}
	if got, want := wc.Special.Background, "#1d1f21"; got != want {
		t.Errorf("Special.Background = %q, want %q", got, want)
	}
	if got, want := wc.Special.Foreground, "#c5c8c6"; got != want {
		t.Errorf("Special.Foreground = %q, want %q", got, want)
	}
	if got, want := wc.Colors.Color0, "#1d1f21"; got != want {
		t.Errorf("Colors.Color0 = %q, want %q", got, want)
	}
	if got, want := wc.Colors.Color3, "#fabd2f"; got != want {
		t.Errorf("Colors.Color3 = %q, want %q", got, want)
	}
	if got, want := wc.Colors.Color15, "#ffffff"; got != want {
		t.Errorf("Colors.Color15 = %q, want %q", got, want)
	}
}

func TestReadWalColors_AllPaletteEntriesPopulated(t *testing.T) {
	wc, err := ReadWalColors(filepath.Join("testdata", "wal-sample-colors.json"))
	if err != nil {
		t.Fatalf("ReadWalColors returned unexpected error: %v", err)
	}
	entries := []struct {
		name string
		val  string
	}{
		{"Color0", wc.Colors.Color0},
		{"Color1", wc.Colors.Color1},
		{"Color2", wc.Colors.Color2},
		{"Color3", wc.Colors.Color3},
		{"Color4", wc.Colors.Color4},
		{"Color5", wc.Colors.Color5},
		{"Color6", wc.Colors.Color6},
		{"Color7", wc.Colors.Color7},
		{"Color8", wc.Colors.Color8},
		{"Color9", wc.Colors.Color9},
		{"Color10", wc.Colors.Color10},
		{"Color11", wc.Colors.Color11},
		{"Color12", wc.Colors.Color12},
		{"Color13", wc.Colors.Color13},
		{"Color14", wc.Colors.Color14},
		{"Color15", wc.Colors.Color15},
	}
	for _, e := range entries {
		if e.val == "" {
			t.Errorf("Colors.%s is empty after parsing fixture", e.name)
		}
	}
}

func TestReadWalColors_MissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist", "colors.json")
	_, err := ReadWalColors(missing)
	if err == nil {
		t.Fatalf("expected error for missing file, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "pywal cache not found at") {
		t.Errorf("error message should mention pywal cache; got %q", msg)
	}
	if !strings.Contains(msg, missing) {
		t.Errorf("error message should include the requested path; got %q", msg)
	}
	if !strings.Contains(msg, "is pywal installed and has it run?") {
		t.Errorf("error message should include the install/run hint; got %q", msg)
	}
}

func TestReadWalColors_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "colors.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err := ReadWalColors(path)
	if err == nil {
		t.Fatalf("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parse pywal colors") {
		t.Errorf("error message should mention parse failure; got %q", err.Error())
	}
}

func TestWalToTheme_GeneratesExpectedTOML(t *testing.T) {
	wc, err := ReadWalColors(filepath.Join("testdata", "wal-sample-colors.json"))
	if err != nil {
		t.Fatalf("ReadWalColors: %v", err)
	}
	ts := time.Date(2026, 5, 3, 14, 22, 1, 0, time.UTC)
	src := "/home/alice/.cache/wal/colors.json"
	got := walToThemeTOML(wc, src, ts)

	wantSubstrings := []string{
		"# Generated from /home/alice/.cache/wal/colors.json on 2026-05-03T14:22:01Z.",
		"# Re-run `tmoney theme generate-from-wal` to regenerate after pywal updates.",
		"# Live-swap is not automatic — re-select 'wal' in View → Theme to apply.",
		`name = "wal"`,
		`description = "Generated from pywal palette"`,
		`border_style = "single"`,
		// desktop.bg is commented out — not painted in v1.
		`# desktop.bg = "#1d1f21"`,
		// window slots
		`window.bg = "#1d1f21"`,
		`window.fg = "#c5c8c6"`,
		`window.border.fg = "#c5c8c6"`,
		`window.title.fg = "#fabd2f"`,
		// menubar slots
		`menubar.bg = "#1d1f21"`,
		`menubar.fg = "#c5c8c6"`,
		`menubar.active.bg = "#81a2be"`,
		`menubar.active.fg = "#1d1f21"`,
		`menubar.shortcut.fg = "#cc6666"`,
		// statusbar slots
		`statusbar.bg = "#1d1f21"`,
		`statusbar.fg = "#c5c8c6"`,
		// dialog slots
		`dialog.bg = "#969896"`,
		`dialog.fg = "#c5c8c6"`,
		`dialog.border.fg = "#c5c8c6"`,
		`dialog.title.fg = "#fabd2f"`,
		// table slots
		`table.header.fg = "#fabd2f"`,
		`table.row.fg = "#c5c8c6"`,
		`table.selected.bg = "#81a2be"`,
		`table.selected.fg = "#1d1f21"`,
		// semantic text slots
		`text.positive = "#b5bd68"`,
		`text.negative = "#cc6666"`,
		`text.alert = "#fabd2f"`,
		`text.muted = "#969896"`,
		`text.title = "#c5c8c6"`,
		`text.error = "#cc6666"`,
	}
	for _, s := range wantSubstrings {
		if !strings.Contains(got, s) {
			t.Errorf("walToThemeTOML output missing substring %q.\nGot:\n%s", s, got)
		}
	}
}

func TestWalToTheme_OmitsSymbolAndShortcutSlots(t *testing.T) {
	wc, err := ReadWalColors(filepath.Join("testdata", "wal-sample-colors.json"))
	if err != nil {
		t.Fatalf("ReadWalColors: %v", err)
	}
	got := walToThemeTOML(wc, "irrelevant", time.Unix(0, 0).UTC())
	for _, forbidden := range []string{
		"symbols.menu_separator",
		"symbols.focus_indicator",
		"symbols.checkmark",
		"shortcut.underline",
	} {
		if strings.Contains(got, forbidden) {
			t.Errorf("walToThemeTOML output should not contain %q (helper omits so default fills in).\nGot:\n%s", forbidden, got)
		}
	}
}

func TestWalToTheme_OutputParsesAsValidTheme(t *testing.T) {
	wc, err := ReadWalColors(filepath.Join("testdata", "wal-sample-colors.json"))
	if err != nil {
		t.Fatalf("ReadWalColors: %v", err)
	}
	out := walToThemeTOML(wc, "src", time.Unix(0, 0).UTC())
	th, issues, err := theme.Parse([]byte(out))
	if err != nil {
		t.Fatalf("generated TOML failed to parse: %v", err)
	}
	if len(issues) > 0 {
		t.Fatalf("generated TOML produced parser issues: %+v", issues)
	}
	if th.Name != "wal" {
		t.Errorf("parsed theme name = %q, want %q", th.Name, "wal")
	}
}
