package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// findIssue returns the first issue with the given key (dotted path)
// or nil if none was reported.
func findIssue(issues []Issue, key string) *Issue {
	for i := range issues {
		if issues[i].Key == key {
			return &issues[i]
		}
	}
	return nil
}

// TestParse_MalformedHex confirms that a known slot with an invalid
// color value falls back to the default theme's value and that one
// IssueMalformedValue is reported with the offending key.
func TestParse_MalformedHex(t *testing.T) {
	src := []byte(`
name = "Bad Hex"
text.negative = "not-a-color"
`)
	tm, issues, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	defaults := DefaultTheme()
	if tm.Text.Negative != defaults.Text.Negative {
		t.Errorf("Text.Negative = %q, want default %q", tm.Text.Negative, defaults.Text.Negative)
	}
	iss := findIssue(issues, "text.negative")
	if iss == nil {
		t.Fatalf("expected issue for text.negative, got %+v", issues)
	}
	if iss.Kind != IssueMalformedValue {
		t.Errorf("issue kind = %v, want IssueMalformedValue", iss.Kind)
	}
	if iss.Value != "not-a-color" {
		t.Errorf("issue value = %q, want %q", iss.Value, "not-a-color")
	}
}

// TestParse_OutOfRange256 confirms that an ANSI 256 number outside
// 0..255 is rejected per-slot.
func TestParse_OutOfRange256(t *testing.T) {
	src := []byte(`text.muted = "999"`)
	tm, issues, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	defaults := DefaultTheme()
	if tm.Text.Muted != defaults.Text.Muted {
		t.Errorf("Text.Muted = %q, want default %q", tm.Text.Muted, defaults.Text.Muted)
	}
	if findIssue(issues, "text.muted") == nil {
		t.Errorf("expected issue for text.muted, got %+v", issues)
	}
}

// TestParse_BadBorderStyle confirms that an unrecognized border style
// reverts to the default's value (single).
func TestParse_BadBorderStyle(t *testing.T) {
	src := []byte(`border_style = "wavy"`)
	tm, issues, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	defaults := DefaultTheme()
	if tm.BorderStyle != defaults.BorderStyle {
		t.Errorf("BorderStyle = %q, want default %q", tm.BorderStyle, defaults.BorderStyle)
	}
	iss := findIssue(issues, "border_style")
	if iss == nil {
		t.Fatalf("expected issue for border_style, got %+v", issues)
	}
	if iss.Value != "wavy" {
		t.Errorf("issue value = %q, want %q", iss.Value, "wavy")
	}
}

// TestParse_UnknownKey confirms a typo'd key is reported and ignored
// — the rest of the theme still loads.
func TestParse_UnknownKey(t *testing.T) {
	src := []byte(`
name = "Typo"
windows.bg = "#000000"
text.negative = "#ff0000"
`)
	tm, issues, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if tm.Text.Negative != "#ff0000" {
		t.Errorf("valid sibling slot was dropped: Text.Negative = %q", tm.Text.Negative)
	}
	iss := findIssue(issues, "windows.bg")
	if iss == nil {
		// Some TOML libs report the parent key; accept "windows" too.
		iss = findIssue(issues, "windows")
	}
	if iss == nil {
		t.Fatalf("expected unknown-key issue mentioning `windows`, got %+v", issues)
	}
	if iss.Kind != IssueUnknownKey {
		t.Errorf("issue kind = %v, want IssueUnknownKey", iss.Kind)
	}
}

// TestParse_MinimalTheme confirms that any slot the user theme omits
// keeps the default theme's value. Only `name` and the one specified
// slot should differ from default.
func TestParse_MinimalTheme(t *testing.T) {
	src := []byte(`
name = "Just Red"
text.negative = "#ff0000"
`)
	tm, issues, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %+v", issues)
	}
	defaults := DefaultTheme()
	if tm.Name != "Just Red" {
		t.Errorf("Name = %q, want %q", tm.Name, "Just Red")
	}
	if tm.Text.Negative != "#ff0000" {
		t.Errorf("Text.Negative = %q, want %q", tm.Text.Negative, "#ff0000")
	}
	// Spot-check a few slots fell back to default.
	if tm.Text.Positive != defaults.Text.Positive {
		t.Errorf("Text.Positive = %q, want default %q", tm.Text.Positive, defaults.Text.Positive)
	}
	if tm.Menubar.Bg != defaults.Menubar.Bg {
		t.Errorf("Menubar.Bg = %q, want default %q", tm.Menubar.Bg, defaults.Menubar.Bg)
	}
	if tm.BorderStyle != defaults.BorderStyle {
		t.Errorf("BorderStyle = %q, want default %q", tm.BorderStyle, defaults.BorderStyle)
	}
	if tm.Symbols.Checkmark != defaults.Symbols.Checkmark {
		t.Errorf("Symbols.Checkmark = %q, want default %q",
			tm.Symbols.Checkmark, defaults.Symbols.Checkmark)
	}
}

// TestParse_EmptyBackground confirms that an explicit empty string on
// a `*.bg` slot is preserved as the transparent sentinel (NOT
// replaced by the default).
func TestParse_EmptyBackground(t *testing.T) {
	// Use a non-default starting point: turbo-vision sets window.bg
	// to a concrete value. Then an explicit empty string should
	// override it.
	src := []byte(`
window.bg = ""
`)
	tm, issues, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues for explicit empty bg, got %+v", issues)
	}
	if tm.Window.Bg != "" {
		t.Errorf("Window.Bg = %q, want empty string (transparent sentinel)", tm.Window.Bg)
	}
}

// TestParseFromFile_NoName confirms that a theme with no `name` field
// falls back to the filename stem.
func TestParseFromFile_NoName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fancy-theme.toml")
	src := `text.negative = "#ff0000"`
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	tm, _, err := ParseFromFile(path)
	if err != nil {
		t.Fatalf("ParseFromFile() error: %v", err)
	}
	if tm.Name != "fancy-theme" {
		t.Errorf("Name = %q, want %q (stem of %s)", tm.Name, "fancy-theme", path)
	}
}

func TestParseFromFile_NameRespected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ignored-stem.toml")
	src := `name = "Real Name"`
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	tm, _, err := ParseFromFile(path)
	if err != nil {
		t.Fatalf("ParseFromFile() error: %v", err)
	}
	if tm.Name != "Real Name" {
		t.Errorf("Name = %q, want %q (explicit name beats stem)", tm.Name, "Real Name")
	}
}

func TestParseFromFile_MissingFile(t *testing.T) {
	_, _, err := ParseFromFile(filepath.Join(t.TempDir(), "does-not-exist.toml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "does-not-exist.toml") {
		t.Errorf("error %q does not mention the file path", err)
	}
}
