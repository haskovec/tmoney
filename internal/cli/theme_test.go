package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestThemeSubcommand_HelpListsChildren(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"theme", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(theme --help) unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"list", "generate-from-wal", "Available Commands", "Usage:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected `theme --help` output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestThemeSubcommand_NoArgsPrintsHelp(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"theme"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(theme) unexpected error: %v", err)
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "Available Commands") {
		t.Errorf("expected bare `theme` to print usage with subcommands, got:\nstdout=%s\nstderr=%s",
			stdout.String(), stderr.String())
	}
}

func TestExecute_RootHelpListsThemeSubcommand(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(--help) unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "theme") {
		t.Errorf("expected root --help to mention `theme` subcommand, got:\n%s", stdout.String())
	}
}
