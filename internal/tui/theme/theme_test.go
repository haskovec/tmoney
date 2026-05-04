package theme

import (
	"strings"
	"testing"
)

// TestDefaultTheme_AllSlotsPopulated walks every slot on the default
// theme and asserts each one is non-empty — except the documented
// "transparent" background sentinels (`*.bg` slots that are
// intentionally empty so the terminal default shows through).
func TestDefaultTheme_AllSlotsPopulated(t *testing.T) {
	dt := DefaultTheme()

	if dt.Name == "" {
		t.Error("Name is empty")
	}
	if dt.Description == "" {
		t.Error("Description is empty")
	}
	if !dt.BorderStyle.IsValid() {
		t.Errorf("BorderStyle %q is not a valid enum value", dt.BorderStyle)
	}

	// background slots that are allowed to be the transparent sentinel
	transparentOK := map[string]bool{
		"Desktop.Bg": true,
		"Window.Bg":  true,
		"Dialog.Bg":  true,
	}

	checkColor := func(slot, value string) {
		t.Helper()
		if value == "" && !transparentOK[slot] {
			t.Errorf("slot %s is empty (only background slots may be transparent)", slot)
		}
	}

	checkColor("Desktop.Bg", dt.Desktop.Bg)

	checkColor("Menubar.Fg", dt.Menubar.Fg)
	checkColor("Menubar.Bg", dt.Menubar.Bg)
	checkColor("Menubar.Active.Fg", dt.Menubar.Active.Fg)
	checkColor("Menubar.Active.Bg", dt.Menubar.Active.Bg)
	checkColor("Menubar.Shortcut.Fg", dt.Menubar.Shortcut.Fg)

	checkColor("Statusbar.Fg", dt.Statusbar.Fg)
	checkColor("Statusbar.Bg", dt.Statusbar.Bg)
	checkColor("Statusbar.Shortcut.Fg", dt.Statusbar.Shortcut.Fg)

	checkColor("Window.Bg", dt.Window.Bg)
	checkColor("Window.Fg", dt.Window.Fg)
	checkColor("Window.Border.Fg", dt.Window.Border.Fg)
	checkColor("Window.Title.Fg", dt.Window.Title.Fg)

	checkColor("Dialog.Bg", dt.Dialog.Bg)
	checkColor("Dialog.Fg", dt.Dialog.Fg)
	checkColor("Dialog.Border.Fg", dt.Dialog.Border.Fg)
	checkColor("Dialog.Title.Fg", dt.Dialog.Title.Fg)

	checkColor("Table.Header.Fg", dt.Table.Header.Fg)
	checkColor("Table.Row.Fg", dt.Table.Row.Fg)
	checkColor("Table.Selected.Fg", dt.Table.Selected.Fg)
	checkColor("Table.Selected.Bg", dt.Table.Selected.Bg)

	checkColor("Text.Positive", dt.Text.Positive)
	checkColor("Text.Negative", dt.Text.Negative)
	checkColor("Text.Alert", dt.Text.Alert)
	checkColor("Text.Muted", dt.Text.Muted)
	checkColor("Text.Title", dt.Text.Title)
	checkColor("Text.Error", dt.Text.Error)

	if strings.TrimSpace(dt.Symbols.MenuSeparator) == "" {
		t.Errorf("Symbols.MenuSeparator is empty")
	}
	if dt.Symbols.FocusIndicator == "" {
		t.Errorf("Symbols.FocusIndicator is empty")
	}
	if dt.Symbols.Checkmark == "" {
		t.Errorf("Symbols.Checkmark is empty")
	}

	// Default has shortcut underlines on (the spec's documented baseline).
	if !dt.Menubar.Shortcut.Underline {
		t.Error("Menubar.Shortcut.Underline should default to true")
	}
	if !dt.Statusbar.Shortcut.Underline {
		t.Error("Statusbar.Shortcut.Underline should default to true")
	}
}

// TestDefaultTheme_Independent ensures successive DefaultTheme()
// calls return distinct pointers; mutating one must not affect the
// next.
func TestDefaultTheme_Independent(t *testing.T) {
	a := DefaultTheme()
	b := DefaultTheme()
	if a == b {
		t.Fatal("DefaultTheme() returned the same pointer twice")
	}
	a.Text.Negative = "mutated"
	if b.Text.Negative == "mutated" {
		t.Error("mutating one DefaultTheme leaked into another")
	}
}

func TestBorderStyle_IsValid(t *testing.T) {
	cases := map[BorderStyle]bool{
		BorderSingle:  true,
		BorderDouble:  true,
		BorderRounded: true,
		BorderThick:   true,
		"":            false,
		"wavy":        false,
		"none":        false,
	}
	for bs, want := range cases {
		if got := bs.IsValid(); got != want {
			t.Errorf("%q.IsValid() = %v, want %v", bs, got, want)
		}
	}
}
