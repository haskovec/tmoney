package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestParse_ValidTheme(t *testing.T) {
	data := mustRead(t, filepath.Join("testdata", "turbo-vision-min.toml"))
	tm, issues, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() returned error: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %d: %+v", len(issues), issues)
	}

	wantStr := []struct {
		name string
		got  string
		want string
	}{
		{"Name", tm.Name, "Turbo Vision"},
		{"Description", tm.Description, "Classic Borland Turbo Vision blue + double borders"},
		{"BorderStyle", string(tm.BorderStyle), "double"},

		{"Desktop.Bg", tm.Desktop.Bg, ""},

		{"Menubar.Fg", tm.Menubar.Fg, "#000000"},
		{"Menubar.Bg", tm.Menubar.Bg, "#aaaaaa"},
		{"Menubar.Active.Fg", tm.Menubar.Active.Fg, "#ffffff"},
		{"Menubar.Active.Bg", tm.Menubar.Active.Bg, "#000000"},
		{"Menubar.Shortcut.Fg", tm.Menubar.Shortcut.Fg, "#aa0000"},

		{"Statusbar.Fg", tm.Statusbar.Fg, "#000000"},
		{"Statusbar.Bg", tm.Statusbar.Bg, "#aaaaaa"},
		{"Statusbar.Shortcut.Fg", tm.Statusbar.Shortcut.Fg, "#aa0000"},

		{"Window.Bg", tm.Window.Bg, "#0000aa"},
		{"Window.Fg", tm.Window.Fg, "#aaaaaa"},
		{"Window.Border.Fg", tm.Window.Border.Fg, "#ffffff"},
		{"Window.Title.Fg", tm.Window.Title.Fg, "#ffff55"},

		{"dialog.Dialog.Bg", tm.Dialog.Bg, "#aaaaaa"},
		{"dialog.Dialog.Fg", tm.Dialog.Fg, "#000000"},
		{"dialog.Dialog.Border.Fg", tm.Dialog.Border.Fg, "#000000"},
		{"dialog.Dialog.Title.Fg", tm.Dialog.Title.Fg, "#000000"},

		{"widget.Table.Header.Fg", tm.Table.Header.Fg, "#ffff55"},
		{"widget.Table.Row.Fg", tm.Table.Row.Fg, "#aaaaaa"},
		{"widget.Table.Selected.Fg", tm.Table.Selected.Fg, "#000000"},
		{"widget.Table.Selected.Bg", tm.Table.Selected.Bg, "#00aaaa"},

		{"Text.Positive", tm.Text.Positive, "#55ff55"},
		{"Text.Negative", tm.Text.Negative, "#ff5555"},
		{"Text.Alert", tm.Text.Alert, "#ffff55"},
		{"Text.Muted", tm.Text.Muted, "#5555ff"},
		{"Text.Title", tm.Text.Title, "#ffffff"},
		{"Text.Error", tm.Text.Error, "#ff5555"},

		{"Symbols.MenuSeparator", tm.Symbols.MenuSeparator, " │ "},
		{"Symbols.FocusIndicator", tm.Symbols.FocusIndicator, "▶ "},
		{"Symbols.Checkmark", tm.Symbols.Checkmark, "✓"},
	}
	for _, w := range wantStr {
		if w.got != w.want {
			t.Errorf("%s = %q, want %q", w.name, w.got, w.want)
		}
	}

	if tm.Menubar.Shortcut.Underline {
		t.Errorf("Menubar.Shortcut.Underline = true, want false (Turbo Vision uses color-only)")
	}
	if tm.Statusbar.Shortcut.Underline {
		t.Errorf("Statusbar.Shortcut.Underline = true, want false")
	}
}

func TestParse_UnparseableTOML(t *testing.T) {
	_, _, err := Parse([]byte("this is { not valid TOML"))
	if err == nil {
		t.Fatal("expected error for malformed TOML, got nil")
	}
	if !strings.Contains(err.Error(), "parse theme TOML") {
		t.Errorf("error %q lacks expected prefix", err)
	}
}

func TestIsValidColor(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"", true},
		{"#000000", true},
		{"#ffffff", true},
		{"#FFFFFF", true},
		{"#aaBBcc", true},
		{"0", true},
		{"255", true},
		{"127", true},

		{"#fff", false},     // too short
		{"#abcdefg", false}, // bad hex
		{"red", false},
		{"256", false}, // out of range
		{"-1", false},
		{"0xff", false},
		{" 12 ", false}, // whitespace
	}
	for _, c := range cases {
		if got := isValidColor(c.s); got != c.want {
			t.Errorf("isValidColor(%q) = %v, want %v", c.s, got, c.want)
		}
	}
}
