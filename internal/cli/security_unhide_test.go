package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/security"
)

func TestSecurityUnhide_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"security", "unhide", "AAPL"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(security unhide AAPL) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestSecurityUnhide_MissingTicker(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"security", "unhide"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(security unhide) without ticker should return error")
	}
	if !strings.Contains(err.Error(), "arg") {
		t.Errorf("expected Cobra arg-count error, got: %v", err)
	}
}

func TestSecurityUnhide_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"security", "unhide", "FAKE", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("unhiding non-existent security should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestSecurityUnhide_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	repo := security.NewRepository(database)
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec.Hide()
	if err := repo.Create(sec); err != nil {
		t.Fatalf("setup: create security: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"security", "unhide", "AAPL", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security unhide AAPL): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "unhidden successfully") {
		t.Errorf("output should confirm unhiding, got: %s", output)
	}
	if !strings.Contains(output, "AAPL") {
		t.Errorf("output should contain ticker, got: %s", output)
	}

	stdout.Reset()
	if err := executeWith([]string{"security", "list", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security list): %v", err)
	}
	if !strings.Contains(stdout.String(), "AAPL") {
		t.Error("unhidden security should appear in default listing")
	}
}

func TestSecurityUnhide_NotHidden(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"security", "unhide", "AAPL", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("unhiding visible security should return error")
	}
}

func TestSecurityUnhide_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"security", "unhide", "AAPL", "EXTRA", "--file", "x.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(security unhide AAPL EXTRA) should return error")
	}
}

func TestSecurityUnhide_Help(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"security", "unhide", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security unhide --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "unhide") {
		t.Errorf("expected `security unhide --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestSecurityCmd_HelpListsUnhide(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"security", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "unhide") {
		t.Errorf("expected `security --help` to list `unhide`; got:\n%s", stdout.String())
	}
}
