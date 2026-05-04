package theme

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestDiscoverUserThemes covers TH-025: a fixture directory containing
// `wal.toml` and `mine.toml` produces the IDs `["mine", "wal"]`
// (sorted, alphabetical, filename stems).
func TestDiscoverUserThemes(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	themesDir := filepath.Join(tmp, "tmoney", "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatalf("mkdir themes dir: %v", err)
	}
	for _, name := range []string{"wal.toml", "mine.toml"} {
		if err := os.WriteFile(filepath.Join(themesDir, name), []byte("name = \"x\"\n"), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	got, err := DiscoverUserThemes()
	if err != nil {
		t.Fatalf("DiscoverUserThemes: %v", err)
	}
	want := []string{"mine", "wal"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DiscoverUserThemes() = %v, want %v", got, want)
	}
}

// TestDiscoverUserThemes_MissingDir verifies that a missing user theme
// directory is treated as an empty list (first-run users have no
// themes installed) and not as an error.
func TestDiscoverUserThemes_MissingDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	got, err := DiscoverUserThemes()
	if err != nil {
		t.Fatalf("DiscoverUserThemes (missing dir): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("DiscoverUserThemes() = %v, want empty slice for missing dir", got)
	}
}

// TestDiscoverUserThemes_IgnoresNonTOML covers the silent-skip
// behavior for non-.toml files and subdirectories — only the .toml
// files are picked up as theme IDs.
func TestDiscoverUserThemes_IgnoresNonTOML(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	themesDir := filepath.Join(tmp, "tmoney", "themes")
	if err := os.MkdirAll(filepath.Join(themesDir, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	files := map[string]string{
		"keep.toml":   "name = \"keep\"\n",
		"README.md":   "ignore me\n",
		"backup.bak":  "ignore me\n",
		"hidden.toml": "name = \"hidden\"\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(themesDir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	got, err := DiscoverUserThemes()
	if err != nil {
		t.Fatalf("DiscoverUserThemes: %v", err)
	}
	want := []string{"hidden", "keep"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DiscoverUserThemes() = %v, want %v", got, want)
	}
}

// TestUserThemesDir_XDGConfigHome verifies the XDG path takes
// precedence over $HOME — matches config.Dir() semantics so the two
// stay in lockstep when users move their config root.
func TestUserThemesDir_XDGConfigHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	got, err := UserThemesDir()
	if err != nil {
		t.Fatalf("UserThemesDir: %v", err)
	}
	want := filepath.Join(tmp, "tmoney", "themes")
	if got != want {
		t.Errorf("UserThemesDir() = %q, want %q", got, want)
	}
}

// TestUserThemesDir_FallbackHome verifies the $HOME/.config/tmoney/themes
// fallback when XDG_CONFIG_HOME is empty.
func TestUserThemesDir_FallbackHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	got, err := UserThemesDir()
	if err != nil {
		t.Fatalf("UserThemesDir: %v", err)
	}
	want := filepath.Join(home, ".config", "tmoney", "themes")
	if got != want {
		t.Errorf("UserThemesDir() = %q, want %q", got, want)
	}
}
