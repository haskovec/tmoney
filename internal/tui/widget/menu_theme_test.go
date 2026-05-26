package widget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/tui/theme"
)

// TestBuildThemeMenuItems_PopulatesFromBuiltins covers TH-022: opening
// View → Theme should show one menu entry per built-in theme, in the
// order BuiltinIDs() returns them. Each item carries the theme ID as
// its data payload and a MenuActionLoadTheme action so TH-023's
// dispatch can reach reloadTheme without re-deriving the ID from the
// label.
func TestBuildThemeMenuItems_PopulatesFromBuiltins(t *testing.T) {
	got := BuildThemeMenuItems("")

	ids := theme.BuiltinIDs()
	if len(got) != len(ids) {
		t.Fatalf("len(items) = %d, want %d (BuiltinIDs)", len(got), len(ids))
	}

	for i, id := range ids {
		if got[i].Action != MenuActionLoadTheme {
			t.Errorf("items[%d].action = %d, want MenuActionLoadTheme", i, got[i].Action)
		}
		if got[i].Data != id {
			t.Errorf("items[%d].data = %q, want %q", i, got[i].Data, id)
		}
		if !strings.Contains(got[i].Label, id) {
			t.Errorf("items[%d].label = %q, want it to contain %q", i, got[i].Label, id)
		}
	}
}

// TestBuildThemeMenuItems_ActiveHasCheckmark — the active theme's label
// is prefixed with "✓ " so the dropdown shows which one is currently
// applied. Other items use a 2-column non-checkmark prefix so the IDs
// stay column-aligned in the dropdown.
func TestBuildThemeMenuItems_ActiveHasCheckmark(t *testing.T) {
	got := BuildThemeMenuItems("turbo-vision")

	var active *MenuItem
	for i := range got {
		if got[i].Data == "turbo-vision" {
			active = &got[i]
			break
		}
	}
	if active == nil {
		t.Fatal("no item with data=turbo-vision")
	}
	if !strings.HasPrefix(active.Label, "✓ ") {
		t.Errorf("active label = %q, want a %q prefix", active.Label, "✓ ")
	}

	// All other items must NOT have the checkmark prefix.
	for _, item := range got {
		if item.Data == "turbo-vision" {
			continue
		}
		if strings.HasPrefix(item.Label, "✓") {
			t.Errorf("inactive item %q should not start with ✓", item.Label)
		}
	}
}

// TestBuildThemeMenuItems_NoActive — when the active ID is empty (no
// theme persisted yet) no item should carry the checkmark.
func TestBuildThemeMenuItems_NoActive(t *testing.T) {
	got := BuildThemeMenuItems("")
	for _, item := range got {
		if strings.HasPrefix(item.Label, "✓") {
			t.Errorf("item %q should not start with ✓ when activeID is empty", item.Label)
		}
	}
}

// TestBuildThemeMenuItems_UnknownActive — an active ID that doesn't
// match any built-in (e.g., a removed user theme persisted in config)
// must not crash and must not mark any item as active.
func TestBuildThemeMenuItems_UnknownActive(t *testing.T) {
	got := BuildThemeMenuItems("nonexistent")
	for _, item := range got {
		if strings.HasPrefix(item.Label, "✓") {
			t.Errorf("item %q should not start with ✓ when activeID is unknown", item.Label)
		}
	}
}

// seedUserThemes writes the given filenames (each `<stem>.toml`) into a
// tempdir and points XDG_CONFIG_HOME at its parent so DiscoverUserThemes
// finds them. Returns nothing — t.Setenv reverts the env on cleanup and
// the tempdir is removed by t.TempDir.
func seedUserThemes(t *testing.T, stems ...string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	dir := filepath.Join(tmp, "tmoney", "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir user themes: %v", err)
	}
	for _, stem := range stems {
		path := filepath.Join(dir, stem+".toml")
		if err := os.WriteFile(path, []byte("name = \""+stem+"\"\n"), 0o600); err != nil {
			t.Fatalf("seed %s.toml: %v", stem, err)
		}
	}
}

// TestBuildThemeMenuItems_IncludesUserThemes covers TH-027: the View →
// Theme submenu must list user-authored themes alongside the embedded
// built-ins. Built-in IDs come from BuiltinIDs(); user-only IDs come
// from DiscoverUserThemes(). Both kinds carry the MenuActionLoadTheme
// action so reloadTheme can find them via LoadTheme (which already
// prefers the user dir).
func TestBuildThemeMenuItems_IncludesUserThemes(t *testing.T) {
	seedUserThemes(t, "mine", "wal")

	got := BuildThemeMenuItems("")

	have := map[string]bool{}
	for _, item := range got {
		have[item.Data] = true
		if item.Action != MenuActionLoadTheme {
			t.Errorf("item %q action = %d, want MenuActionLoadTheme", item.Data, item.Action)
		}
	}

	for _, id := range theme.BuiltinIDs() {
		if !have[id] {
			t.Errorf("built-in %q missing from submenu", id)
		}
	}
	for _, id := range []string{"mine", "wal"} {
		if !have[id] {
			t.Errorf("user theme %q missing from submenu", id)
		}
	}
}

// TestBuildThemeMenuItems_UserOverrideDedupes — when a user theme
// shadows a built-in by ID (e.g. user `default.toml` overrides the
// embedded one), the submenu must list that ID exactly once. LoadTheme
// already prefers the user file at load time; the menu must not show
// the same ID twice.
func TestBuildThemeMenuItems_UserOverrideDedupes(t *testing.T) {
	seedUserThemes(t, "default", "mine")

	got := BuildThemeMenuItems("")

	count := map[string]int{}
	for _, item := range got {
		count[item.Data]++
	}
	if count["default"] != 1 {
		t.Errorf("data=\"default\" appears %d times, want 1", count["default"])
	}
	if count["mine"] != 1 {
		t.Errorf("data=\"mine\" appears %d times, want 1", count["mine"])
	}
}

// TestBuildThemeMenuItems_ActiveUserTheme — the active marker still
// works when the active theme is a user-only ID (not a built-in).
func TestBuildThemeMenuItems_ActiveUserTheme(t *testing.T) {
	seedUserThemes(t, "mine")

	got := BuildThemeMenuItems("mine")

	var active *MenuItem
	for i := range got {
		if got[i].Data == "mine" {
			active = &got[i]
			break
		}
	}
	if active == nil {
		t.Fatal("no item with data=mine")
	}
	if !strings.HasPrefix(active.Label, "✓ ") {
		t.Errorf("active label = %q, want a %q prefix", active.Label, "✓ ")
	}
	for _, item := range got {
		if item.Data == "mine" {
			continue
		}
		if strings.HasPrefix(item.Label, "✓") {
			t.Errorf("inactive item %q should not start with ✓", item.Label)
		}
	}
}

// TestBuildThemeMenuItems_SortedAlphabetical — items are listed in a
// stable, alphabetical order so the submenu doesn't reshuffle as
// users add or remove user themes. With "mine" added, the order
// should be: default, light, mine, turbo-vision.
func TestBuildThemeMenuItems_SortedAlphabetical(t *testing.T) {
	seedUserThemes(t, "mine")

	got := BuildThemeMenuItems("")

	ids := make([]string, len(got))
	for i, item := range got {
		ids[i] = item.Data
	}
	want := []string{"default", "light", "mine", "turbo-vision"}
	if len(ids) != len(want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("ids[%d] = %q, want %q (full list: %v)", i, ids[i], want[i], ids)
		}
	}
}
