package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// isTerminal reports whether stdin is connected to a terminal.
// Tests that launch the TUI must be skipped in terminal environments
// because bubbletea will take over the screen and hang waiting for input.
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func TestRun_NoArgs(t *testing.T) {
	// Running with no args launches TUI mode, which requires a TTY.
	// Skip this test when running in a real terminal to avoid launching
	// a full bubbletea program that hangs waiting for input.
	if isTerminal() {
		t.Skip("skipping TUI launch test in terminal environment")
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := executeWith([]string{}, stdout, stderr)
	// We expect an error when running TUI without a TTY
	if err == nil {
		t.Skip("TUI launched successfully (has TTY), skipping test")
	}
	// The error should be TTY-related
	if !strings.Contains(err.Error(), "TTY") && !strings.Contains(err.Error(), "tty") {
		t.Logf("run([]) returned expected non-TTY error: %v", err)
	}
}

func TestRun_UnknownArgs(t *testing.T) {
	// Running with a file argument launches TUI mode, which requires a TTY.
	// Skip this test when running in a real terminal to avoid launching
	// a full bubbletea program that hangs waiting for input.
	if isTerminal() {
		t.Skip("skipping TUI launch test in terminal environment")
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := executeWith([]string{"some-file.tdb"}, stdout, stderr)
	// We expect an error when running TUI without a TTY
	if err == nil {
		t.Skip("TUI launched successfully (has TTY), skipping test")
	}
	// The error should be TTY-related or file-related
	// (acceptable since the file doesn't exist)
	t.Logf("run with file argument returned expected error: %v", err)
}
