package cli

import (
	"bytes"
	"strings"
	"testing"
)

// stubLaunchers swaps tuiLauncher with a capturing stub for the
// duration of a test, returning a restore func.
func stubLaunchers(t *testing.T) (tuiCalls *[]string, restore func()) {
	t.Helper()
	tui := []string{}

	origTUI := tuiLauncher

	tuiLauncher = func(file string) error {
		tui = append(tui, file)
		return nil
	}
	return &tui, func() {
		tuiLauncher = origTUI
	}
}

func TestExecute_NoArgs_LaunchesTUI(t *testing.T) {
	tui, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{}, stdout, stderr); err != nil {
		t.Fatalf("executeWith() unexpected error: %v", err)
	}
	if len(*tui) != 1 {
		t.Fatalf("expected 1 TUI launch, got %d", len(*tui))
	}
	if (*tui)[0] != "" {
		t.Errorf("expected empty file, got %q", (*tui)[0])
	}
}

func TestExecute_PositionalFile_LaunchesTUI(t *testing.T) {
	tui, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"foo.tdb"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith() unexpected error: %v", err)
	}
	if len(*tui) != 1 || (*tui)[0] != "foo.tdb" {
		t.Errorf("expected TUI launch with foo.tdb, got %v", *tui)
	}
}

func TestExecute_FileFlag_LaunchesTUI(t *testing.T) {
	tui, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"--file=foo.tdb"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith() unexpected error: %v", err)
	}
	if len(*tui) != 1 || (*tui)[0] != "foo.tdb" {
		t.Errorf("expected TUI launch with foo.tdb, got %v", *tui)
	}
}

func TestExecute_ShortFileFlag_LaunchesTUI(t *testing.T) {
	tui, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"-f", "foo.tdb"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith() unexpected error: %v", err)
	}
	if len(*tui) != 1 || (*tui)[0] != "foo.tdb" {
		t.Errorf("expected TUI launch with foo.tdb, got %v", *tui)
	}
}

func TestExecute_Help_ShowsUsage(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(--help) unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"Usage:", "Available Commands", "version"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected --help output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestIsLegacyInvocation(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"empty", []string{}, false},
		{"positional file only", []string{"foo.tdb"}, false},
		{"long file flag with =", []string{"--file=foo.tdb"}, false},
		{"long file flag separate", []string{"--file", "foo.tdb"}, false},
		{"short file flag", []string{"-f", "foo.tdb"}, false},
		{"help long", []string{"--help"}, false},
		{"version subcommand", []string{"version"}, false},
		{"version subcommand with extra arg", []string{"version", "--whatever"}, false},
		{"single legacy flag", []string{"--list-accounts"}, true},
		{"legacy flag mixed with file", []string{"--add-account", "--name", "foo"}, true},
		{"legacy flag after positional", []string{"foo.tdb", "--add-account"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isLegacyInvocation(tc.args)
			if got != tc.want {
				t.Errorf("isLegacyInvocation(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

