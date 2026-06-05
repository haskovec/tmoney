package security_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/dbtest"
	securitydom "github.com/haskovec/tmoney/internal/security"
)

func TestSecurityUnhide_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"security", "unhide", "AAPL"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(security unhide AAPL) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestSecurityUnhide_MissingTicker(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"security", "unhide"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(security unhide) without ticker should return error")
	}
	if !strings.Contains(err.Error(), "arg") {
		t.Errorf("expected Cobra arg-count error, got: %v", err)
	}
}

func TestSecurityUnhide_NotFound(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"security", "unhide", "FAKE", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("unhiding non-existent security should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestSecurityUnhide_Success(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	repo := securitydom.NewRepository(database)
	sec := securitydom.NewSecurity("AAPL", "Apple Inc.", securitydom.TypeStock)
	sec.Hide()
	if err := repo.Create(sec); err != nil {
		t.Fatalf("setup: create security: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"security", "unhide", "AAPL", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(security unhide AAPL): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "unhidden successfully") {
		t.Errorf("output should confirm unhiding, got: %s", output)
	}
	if !strings.Contains(output, "AAPL") {
		t.Errorf("output should contain ticker, got: %s", output)
	}

	stdout.Reset()
	if err := cli.ExecuteWith([]string{"security", "list", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(security list): %v", err)
	}
	if !strings.Contains(stdout.String(), "AAPL") {
		t.Error("unhidden security should appear in default listing")
	}
}

func TestSecurityUnhide_NotHidden(t *testing.T) {
	dbPath, _ := clitest.CreateTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"security", "unhide", "AAPL", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("unhiding visible security should return error")
	}
}

func TestSecurityUnhide_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"security", "unhide", "AAPL", "EXTRA", "--file", "x.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(security unhide AAPL EXTRA) should return error")
	}
}

func TestSecurityUnhide_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"security", "unhide", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(security unhide --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "unhide") {
		t.Errorf("expected `security unhide --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestSecurityCmd_HelpListsUnhide(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"security", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(security --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "unhide") {
		t.Errorf("expected `security --help` to list `unhide`; got:\n%s", stdout.String())
	}
}
