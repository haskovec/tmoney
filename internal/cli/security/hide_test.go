package security_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/db"
	securitydom "github.com/haskovec/tmoney/internal/security"
)

func TestSecurityHide_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"security", "hide", "AAPL"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(security hide AAPL) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestSecurityHide_MissingTicker(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"security", "hide"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(security hide) without ticker should return error")
	}
	if !strings.Contains(err.Error(), "arg") {
		t.Errorf("expected Cobra arg-count error, got: %v", err)
	}
}

func TestSecurityHide_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"security", "hide", "FAKE", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("hiding non-existent security should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestSecurityHide_Success(t *testing.T) {
	dbPath, _ := clitest.CreateTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"security", "hide", "AAPL", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(security hide AAPL): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "hidden successfully") {
		t.Errorf("output should confirm hiding, got: %s", output)
	}
	if !strings.Contains(output, "AAPL") {
		t.Errorf("output should contain ticker, got: %s", output)
	}

	stdout.Reset()
	if err := cli.ExecuteWith([]string{"security", "list", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(security list): %v", err)
	}
	if strings.Contains(stdout.String(), "AAPL") {
		t.Error("hidden security should not appear in default listing")
	}
}

func TestSecurityHide_AlreadyHidden(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}

	repo := securitydom.NewRepository(database)
	sec := securitydom.NewSecurity("AAPL", "Apple Inc.", securitydom.TypeStock)
	sec.Hide()
	if err := repo.Create(sec); err != nil {
		t.Fatalf("setup: create security: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{"security", "hide", "AAPL", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("hiding already-hidden security should return error")
	}
}

func TestSecurityHide_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"security", "hide", "AAPL", "EXTRA", "--file", "x.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(security hide AAPL EXTRA) should return error")
	}
}

func TestSecurityHide_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"security", "hide", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(security hide --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "hide") {
		t.Errorf("expected `security hide --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestSecurityCmd_HelpListsHide(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"security", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(security --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "hide") {
		t.Errorf("expected `security --help` to list `hide`; got:\n%s", stdout.String())
	}
}
