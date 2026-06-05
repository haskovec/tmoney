package security_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/dbtest"
)

func TestSecurityShow_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"security", "show", "AAPL"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(security show AAPL) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestSecurityShow_MissingTicker(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"security", "show"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(security show) without ticker should return error")
	}
	if !strings.Contains(err.Error(), "arg") {
		t.Errorf("expected Cobra arg-count error, got: %v", err)
	}
}

func TestSecurityShow_NotFound(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"security", "show", "FAKE", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(security show FAKE) should return error for non-existent security")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestSecurityShow_DisplaysFullDetails(t *testing.T) {
	dbPath, _ := clitest.CreateTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"security", "show", "AAPL", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(security show AAPL): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	for _, want := range []string{
		"SECURITY: AAPL",
		"Apple Inc.",
		"Stock",
		"Large Cap Stock",
		"USD",
		"NASDAQ",
		"Active",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got:\n%s", want, output)
		}
	}
}

func TestSecurityShow_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"security", "show", "AAPL", "EXTRA", "--file", "x.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(security show AAPL EXTRA) should return error")
	}
}

func TestSecurityShow_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"security", "show", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(security show --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "show") {
		t.Errorf("expected `security show --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestSecurityCmd_HelpListsShow(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"security", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(security --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "show") {
		t.Errorf("expected `security --help` to list `show`; got:\n%s", stdout.String())
	}
}
