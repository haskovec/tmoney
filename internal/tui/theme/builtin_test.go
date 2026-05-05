package theme

import (
	"reflect"
	"testing"
)

func TestBuiltinIDs(t *testing.T) {
	got := BuiltinIDs()
	want := []string{"default", "light", "turbo-vision"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BuiltinIDs() = %v, want %v", got, want)
	}
}

func TestLoadBuiltin_UnknownID(t *testing.T) {
	_, _, err := LoadBuiltin("does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown id, got nil")
	}
}

// TestEmbeddedDefault_MatchesCurrentPalette parses the embedded
// default theme and asserts every slot matches the corresponding
// in-code Color* value from internal/tui/styles.go. We can't import
// the tui package from here (cyclic), so the expected values are
// duplicated as constants — if styles.go changes, this test is
// expected to fail and serves as a tripwire.
func TestEmbeddedDefault_MatchesCurrentPalette(t *testing.T) {
	tm, issues, err := LoadBuiltin("default")
	if err != nil {
		t.Fatalf("LoadBuiltin(default): %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}

	const (
		colorPositive   = "34"
		colorNegative   = "160"
		colorPending    = "245"
		colorAlert      = "214"
		colorBorder     = "240"
		colorHeaderFg   = "15"
		colorHeaderBg   = "62"
		colorStatusFg   = "252"
		colorStatusBg   = "236"
		colorSelectedFg = "15"
		colorSelectedBg = "62"
		colorMuted      = "245"
		colorTitle      = "15"
	)

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"Menubar.Fg", tm.Menubar.Fg, colorHeaderFg},
		{"Menubar.Bg", tm.Menubar.Bg, colorHeaderBg},
		{"Menubar.Active.Fg", tm.Menubar.Active.Fg, colorHeaderBg},
		{"Menubar.Active.Bg", tm.Menubar.Active.Bg, colorHeaderFg},

		{"Statusbar.Fg", tm.Statusbar.Fg, colorStatusFg},
		{"Statusbar.Bg", tm.Statusbar.Bg, colorStatusBg},

		{"Window.Bg", tm.Window.Bg, ""}, // transparent
		{"Window.Fg", tm.Window.Fg, colorHeaderFg},
		{"Window.Border.Fg", tm.Window.Border.Fg, colorBorder},
		{"Window.Title.Fg", tm.Window.Title.Fg, colorTitle},

		{"Dialog.Bg", tm.Dialog.Bg, ""},
		{"Dialog.Fg", tm.Dialog.Fg, colorHeaderFg},
		{"Dialog.Border.Fg", tm.Dialog.Border.Fg, colorBorder},
		{"Dialog.Title.Fg", tm.Dialog.Title.Fg, colorTitle},

		{"Table.Header.Fg", tm.Table.Header.Fg, colorHeaderFg},
		{"Table.Row.Fg", tm.Table.Row.Fg, colorHeaderFg},
		{"Table.Selected.Fg", tm.Table.Selected.Fg, colorSelectedFg},
		{"Table.Selected.Bg", tm.Table.Selected.Bg, colorSelectedBg},

		{"Text.Positive", tm.Text.Positive, colorPositive},
		{"Text.Negative", tm.Text.Negative, colorNegative},
		{"Text.Alert", tm.Text.Alert, colorAlert},
		{"Text.Muted", tm.Text.Muted, colorMuted},
		{"Text.Title", tm.Text.Title, colorTitle},
		{"Text.Error", tm.Text.Error, colorNegative},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}

	// ColorPending is the same numeric code as ColorMuted; assert the
	// muted slot matches both so a future divergence trips this.
	if tm.Text.Muted != colorPending {
		t.Errorf("Text.Muted = %q, want %q (ColorPending)", tm.Text.Muted, colorPending)
	}

	if string(tm.BorderStyle) != "single" {
		t.Errorf("BorderStyle = %q, want single", tm.BorderStyle)
	}
	if !tm.Menubar.Shortcut.Underline {
		t.Error("Menubar.Shortcut.Underline should be true in default theme")
	}
	if !tm.Statusbar.Shortcut.Underline {
		t.Error("Statusbar.Shortcut.Underline should be true in default theme")
	}
}

func TestEmbeddedTurboVision_Parses(t *testing.T) {
	tm, issues, err := LoadBuiltin("turbo-vision")
	if err != nil {
		t.Fatalf("LoadBuiltin(turbo-vision): %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}

	if string(tm.BorderStyle) != "double" {
		t.Errorf("BorderStyle = %q, want double", tm.BorderStyle)
	}
	if tm.Window.Title.Fg != "#ffff55" {
		t.Errorf("Window.Title.Fg = %q, want #ffff55", tm.Window.Title.Fg)
	}
	if tm.Menubar.Shortcut.Fg != "#aa0000" {
		t.Errorf("Menubar.Shortcut.Fg = %q, want #aa0000", tm.Menubar.Shortcut.Fg)
	}
	if tm.Menubar.Shortcut.Underline {
		t.Error("Menubar.Shortcut.Underline = true, want false (TV uses color-only shortcut letters)")
	}
	if tm.Desktop.Bg != "#0000aa" {
		t.Errorf("Desktop.Bg = %q, want #0000aa (Turbo Vision desktop fill)", tm.Desktop.Bg)
	}
}

func TestEmbeddedLight_Parses(t *testing.T) {
	tm, issues, err := LoadBuiltin("light")
	if err != nil {
		t.Fatalf("LoadBuiltin(light): %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}

	// Spot-check: light theme should be readable on a near-white
	// terminal. The dialog panel is the most distinctive surface;
	// confirm it picks an off-white background and dark foreground.
	if tm.Dialog.Bg != "#f5f5f5" {
		t.Errorf("Dialog.Bg = %q, want #f5f5f5 (off-white panel)", tm.Dialog.Bg)
	}
	if tm.Dialog.Fg != "#1a1a1a" {
		t.Errorf("Dialog.Fg = %q, want #1a1a1a (near-black text)", tm.Dialog.Fg)
	}
	if tm.Window.Fg != "#1a1a1a" {
		t.Errorf("Window.Fg = %q, want #1a1a1a", tm.Window.Fg)
	}
	if string(tm.BorderStyle) != "single" {
		t.Errorf("BorderStyle = %q, want single", tm.BorderStyle)
	}
}
