package applog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLogPath_XDGConfigHome verifies the log path follows XDG when
// $XDG_CONFIG_HOME is set, mirroring config.Dir().
func TestLogPath_XDGConfigHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	got, err := LogPath()
	if err != nil {
		t.Fatalf("LogPath() error: %v", err)
	}
	want := filepath.Join(tmp, "tmoney", "log.txt")
	if got != want {
		t.Errorf("LogPath() = %q, want %q", got, want)
	}
}

// TestLogPath_Default verifies the fallback to $HOME/.config/tmoney/log.txt
// when XDG_CONFIG_HOME is unset.
func TestLogPath_Default(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error: %v", err)
	}

	got, err := LogPath()
	if err != nil {
		t.Fatalf("LogPath() error: %v", err)
	}
	want := filepath.Join(home, ".config", "tmoney", "log.txt")
	if got != want {
		t.Errorf("LogPath() = %q, want %q", got, want)
	}
}

// TestLogTheme_AppendsEntries — TH-030 acceptance test. Two issues
// written in sequence both end up on disk with timestamped lines.
func TestLogTheme_AppendsEntries(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if err := Append("theme", "first issue"); err != nil {
		t.Fatalf("Append() #1 error: %v", err)
	}
	if err := Append("theme", "second issue"); err != nil {
		t.Fatalf("Append() #2 error: %v", err)
	}

	path, err := LogPath()
	if err != nil {
		t.Fatalf("LogPath() error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", path, err)
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2; content:\n%s", len(lines), data)
	}

	for i, want := range []string{"first issue", "second issue"} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d = %q, missing %q", i, lines[i], want)
		}
		if !strings.Contains(lines[i], "[theme]") {
			t.Errorf("line %d = %q, missing [theme] category", i, lines[i])
		}
		// Timestamp at start of line must parse as RFC3339.
		ts := strings.SplitN(lines[i], " ", 2)[0]
		if _, err := time.Parse(time.RFC3339, ts); err != nil {
			t.Errorf("line %d timestamp %q not RFC3339: %v", i, ts, err)
		}
	}
}

// TestAppend_CreatesParentDir ensures Append() creates the tmoney
// config directory when it doesn't already exist (first-run case).
func TestAppend_CreatesParentDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if err := Append("theme", "hello"); err != nil {
		t.Fatalf("Append() error: %v", err)
	}

	dir := filepath.Join(tmp, "tmoney")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("tmoney dir not created: %v", err)
	}
}
