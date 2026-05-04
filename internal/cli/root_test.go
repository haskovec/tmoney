package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// stubLaunchers swaps tuiLauncher and legacyRunner with capturing stubs
// for the duration of a test, returning a restore func.
func stubLaunchers(t *testing.T) (tuiCalls *[]string, legacyCalls *[][]string, restore func()) {
	t.Helper()
	tui := []string{}
	legacy := [][]string{}

	origTUI := tuiLauncher
	origLegacy := legacyRunner

	tuiLauncher = func(file string) error {
		tui = append(tui, file)
		return nil
	}
	legacyRunner = func(args []string, stdout, stderr io.Writer) error {
		// We can't quite use io.Writer here without imports; just track args.
		legacy = append(legacy, append([]string(nil), args...))
		return nil
	}
	return &tui, &legacy, func() {
		tuiLauncher = origTUI
		legacyRunner = origLegacy
	}
}

func TestExecute_NoArgs_LaunchesTUI(t *testing.T) {
	tui, legacy, restore := stubLaunchers(t)
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
	if len(*legacy) != 0 {
		t.Errorf("expected 0 legacy calls, got %d", len(*legacy))
	}
}

func TestExecute_PositionalFile_LaunchesTUI(t *testing.T) {
	tui, _, restore := stubLaunchers(t)
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
	tui, _, restore := stubLaunchers(t)
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
	tui, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"-f", "foo.tdb"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith() unexpected error: %v", err)
	}
	if len(*tui) != 1 || (*tui)[0] != "foo.tdb" {
		t.Errorf("expected TUI launch with foo.tdb, got %v", *tui)
	}
}

func TestExecute_LegacyFlag_RoutesToLegacy(t *testing.T) {
	tui, legacy, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	args := []string{"--list-accounts", "-f", "foo.tdb"}
	if err := executeWith(args, stdout, stderr); err != nil {
		t.Fatalf("executeWith() unexpected error: %v", err)
	}
	if len(*tui) != 0 {
		t.Errorf("expected 0 TUI launches, got %d", len(*tui))
	}
	if len(*legacy) != 1 {
		t.Fatalf("expected 1 legacy call, got %d", len(*legacy))
	}
	if !equal((*legacy)[0], args) {
		t.Errorf("expected legacy args %v, got %v", args, (*legacy)[0])
	}
}

func TestExecute_LegacyError_PropagatesAsError(t *testing.T) {
	origLegacy := legacyRunner
	defer func() { legacyRunner = origLegacy }()

	wantErr := errors.New("boom")
	legacyRunner = func(args []string, stdout, stderr io.Writer) error {
		return wantErr
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"--list-accounts"}, stdout, stderr); !errors.Is(err, wantErr) {
		t.Errorf("expected error %v, got %v", wantErr, err)
	}
}

func TestExecute_Help_ShowsUsage(t *testing.T) {
	_, _, restore := stubLaunchers(t)
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

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
