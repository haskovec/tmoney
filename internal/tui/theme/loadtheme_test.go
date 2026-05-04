package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadTheme_UserOverride covers TH-026: a user dir entry named
// `default.toml` shadows the embedded built-in of the same ID, so users
// can theme "default" without having to invent a new ID.
func TestLoadTheme_UserOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	themesDir := filepath.Join(tmp, "tmoney", "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatalf("mkdir themes dir: %v", err)
	}
	const userTOML = `name = "user-default"
[text]
negative = "#ff00aa"
`
	if err := os.WriteFile(filepath.Join(themesDir, "default.toml"), []byte(userTOML), 0o600); err != nil {
		t.Fatalf("seed default.toml: %v", err)
	}

	tm, issues, err := LoadTheme("default")
	if err != nil {
		t.Fatalf("LoadTheme(default): %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	if tm.Name != "user-default" {
		t.Errorf("Name = %q, want %q (user file should shadow built-in)", tm.Name, "user-default")
	}
	if tm.Text.Negative != "#ff00aa" {
		t.Errorf("Text.Negative = %q, want %q (user override slot)", tm.Text.Negative, "#ff00aa")
	}
}

// TestLoadTheme_FallsBackToBuiltin verifies that when a user dir has no
// matching file, LoadTheme falls through to the embedded built-in. This
// is the common path on a fresh install.
func TestLoadTheme_FallsBackToBuiltin(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	themesDir := filepath.Join(tmp, "tmoney", "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatalf("mkdir themes dir: %v", err)
	}

	tm, _, err := LoadTheme("turbo-vision")
	if err != nil {
		t.Fatalf("LoadTheme(turbo-vision): %v", err)
	}
	if string(tm.BorderStyle) != "double" {
		t.Errorf("BorderStyle = %q, want %q (embedded turbo-vision)", tm.BorderStyle, "double")
	}
}

// TestLoadTheme_MissingUserDir verifies the first-run case: the user
// theme directory does not exist at all. LoadTheme must still resolve
// embedded IDs without erroring on the missing directory.
func TestLoadTheme_MissingUserDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	tm, _, err := LoadTheme("light")
	if err != nil {
		t.Fatalf("LoadTheme(light): %v", err)
	}
	if tm.Name == "" {
		t.Error("Name should be non-empty for embedded light theme")
	}
}

// TestLoadTheme_UnknownID verifies that an ID present in neither the
// user dir nor the embedded set surfaces as an error — callers (the
// menu code, persisted-theme bootstrap, etc.) need to know to fall
// back.
func TestLoadTheme_UnknownID(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	_, _, err := LoadTheme("does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown id, got nil")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error should reference the unknown id, got %v", err)
	}
}

// TestLoadTheme_UserOnlyID verifies a user-only ID (no built-in
// counterpart) loads successfully — this is the primary use case for
// the user dir, e.g. a pywal-generated `wal.toml`.
func TestLoadTheme_UserOnlyID(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	themesDir := filepath.Join(tmp, "tmoney", "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatalf("mkdir themes dir: %v", err)
	}
	const userTOML = `name = "Wal"
[text]
positive = "#00ff00"
`
	if err := os.WriteFile(filepath.Join(themesDir, "wal.toml"), []byte(userTOML), 0o600); err != nil {
		t.Fatalf("seed wal.toml: %v", err)
	}

	tm, _, err := LoadTheme("wal")
	if err != nil {
		t.Fatalf("LoadTheme(wal): %v", err)
	}
	if tm.Name != "Wal" {
		t.Errorf("Name = %q, want %q", tm.Name, "Wal")
	}
	if tm.Text.Positive != "#00ff00" {
		t.Errorf("Text.Positive = %q, want %q", tm.Text.Positive, "#00ff00")
	}
}
