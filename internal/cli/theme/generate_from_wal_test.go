package theme_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	tuitheme "github.com/haskovec/tmoney/internal/tui/theme"
)

// installSampleWalCache writes the testdata sample colors.json into
// $XDG_CACHE_HOME/wal/colors.json so the subcommand can find it. Returns
// the resolved path so callers can assert on it.
func installSampleWalCache(t *testing.T, cacheRoot string) string {
	t.Helper()
	walDir := filepath.Join(cacheRoot, "wal")
	if err := os.MkdirAll(walDir, 0o755); err != nil {
		t.Fatalf("setup: mkdir %s: %v", walDir, err)
	}
	src, err := os.ReadFile(filepath.Join("testdata", "wal-sample-colors.json"))
	if err != nil {
		t.Fatalf("setup: read fixture: %v", err)
	}
	dst := filepath.Join(walDir, "colors.json")
	if err := os.WriteFile(dst, src, 0o644); err != nil {
		t.Fatalf("setup: write %s: %v", dst, err)
	}
	return dst
}

func TestThemeGenerateFromWal_DefaultOutputWritesToUserThemesDir(t *testing.T) {
	cacheRoot := t.TempDir()
	configRoot := t.TempDir()
	installSampleWalCache(t, cacheRoot)
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	t.Setenv("XDG_CONFIG_HOME", configRoot)

	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"theme", "generate-from-wal"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}

	expected := filepath.Join(configRoot, "tmoney", "themes", "wal.toml")
	data, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("expected wal.toml at %s: %v", expected, err)
	}
	if !strings.Contains(string(data), `name = "wal"`) {
		t.Errorf("default-output file missing wal content; got:\n%s", data)
	}
	// Round-trip: file must parse cleanly as a theme.
	if _, issues, perr := tuitheme.Parse(data); perr != nil || len(issues) > 0 {
		t.Errorf("generated theme failed to parse: err=%v issues=%v", perr, issues)
	}
	// Status message goes to stderr so stdout stays clean for piping.
	if !strings.Contains(stderr.String(), expected) {
		t.Errorf("expected stderr to mention output path %q, got %q", expected, stderr.String())
	}
}

func TestThemeGenerateFromWal_StdoutOutput(t *testing.T) {
	cacheRoot := t.TempDir()
	installSampleWalCache(t, cacheRoot)
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	// Point XDG_CONFIG_HOME at a tempdir too so a missing/HOME-rooted
	// default path can never accidentally satisfy this test.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"theme", "generate-from-wal", "--output", "-"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith: %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, `name = "wal"`) {
		t.Errorf("stdout missing wal theme content; got:\n%s", out)
	}
	if !strings.Contains(out, `border_style = "single"`) {
		t.Errorf("stdout missing border_style line; got:\n%s", out)
	}
	// No file should have been written under the (empty) default dir.
	defaultDir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "tmoney", "themes")
	if entries, _ := os.ReadDir(defaultDir); len(entries) != 0 {
		t.Errorf("--output - should not write to default dir; found entries: %v", entries)
	}
}

func TestThemeGenerateFromWal_CustomOutputPath(t *testing.T) {
	cacheRoot := t.TempDir()
	installSampleWalCache(t, cacheRoot)
	t.Setenv("XDG_CACHE_HOME", cacheRoot)

	target := filepath.Join(t.TempDir(), "nested", "custom-wal.toml")

	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"theme", "generate-from-wal", "--output", target}, stdout, stderr); err != nil {
		t.Fatalf("executeWith: %v\nstderr=%s", err, stderr)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected file at %s: %v", target, err)
	}
	if !strings.Contains(string(data), `name = "wal"`) {
		t.Errorf("custom-output file missing wal content; got:\n%s", data)
	}
}

func TestThemeGenerateFromWal_MissingPywalCacheReturnsError(t *testing.T) {
	// XDG_CACHE_HOME points at an empty tempdir → no wal/colors.json.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"theme", "generate-from-wal"}, stdout, stderr)
	if err == nil {
		t.Fatalf("expected error for missing pywal cache, got nil")
	}
	for _, want := range []string{
		"pywal cache not found at",
		"is pywal installed and has it run?",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message should contain %q; got %q", want, err.Error())
		}
	}
}

func TestThemeGenerateFromWal_HelpListsOutputFlag(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"theme", "generate-from-wal", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith --help: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"--output", "-o", "stdout"} {
		if !strings.Contains(out, want) {
			t.Errorf("help should mention %q; got:\n%s", want, out)
		}
	}
}
