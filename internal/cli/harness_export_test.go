package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
)

// TestExecuteWith_DrivesRootByArgv proves an external _test package can
// drive the full CLI through the exported harness. This is the pattern
// every noun subpackage relies on after the package split (an external
// `package <noun>_test` importing cli for ExecuteWith is cycle-free).
func TestExecuteWith_DrivesRootByArgv(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(--help): %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	for _, want := range []string{"Usage:", "Available Commands", "version"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected --help output to contain %q, got:\n%s", want, out)
		}
	}
}

// TestSwapTUILauncher_InterceptsAndRestores proves the exported launcher
// seam captures launches driven through ExecuteWith and that nested swaps
// restore in LIFO order — all without ever invoking the real TUI. This is
// the replacement for the unexported stubLaunchers/tuiLauncher poking that
// noun subpackages cannot reach.
func TestSwapTUILauncher_InterceptsAndRestores(t *testing.T) {
	var outer []string
	restoreOuter := cli.SwapTUILauncher(func(file string) error {
		outer = append(outer, file)
		return nil
	})
	defer restoreOuter()

	var inner []string
	restoreInner := cli.SwapTUILauncher(func(file string) error {
		inner = append(inner, file)
		return nil
	})

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"inner.tdb"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(inner.tdb): %v\nstderr=%s", err, stderr)
	}
	if len(inner) != 1 || inner[0] != "inner.tdb" {
		t.Fatalf("inner launcher: expected [inner.tdb], got %v", inner)
	}

	// Restoring the inner swap must reinstate the outer launcher, proving
	// restore composition is LIFO-safe.
	restoreInner()
	if err := cli.ExecuteWith([]string{"outer.tdb"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(outer.tdb): %v\nstderr=%s", err, stderr)
	}
	if len(outer) != 1 || outer[0] != "outer.tdb" {
		t.Fatalf("outer launcher after restore: expected [outer.tdb], got %v", outer)
	}
	if len(inner) != 1 {
		t.Errorf("inner launcher should not fire after restore, got %v", inner)
	}
}
