package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
)

func TestSecurityEdit_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"security", "edit", "AAPL", "--name", "Whatever"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(security edit AAPL) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestSecurityEdit_MissingTicker(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"security", "edit"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(security edit) without ticker should return error")
	}
	if !strings.Contains(err.Error(), "arg") {
		t.Errorf("expected Cobra arg-count error, got: %v", err)
	}
}

func TestSecurityEdit_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"security", "edit", "FAKE", "--file", dbPath, "--name", "X"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(security edit FAKE) should return error for non-existent security")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestSecurityEdit_ChangeName(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{
		"security", "edit", "AAPL",
		"--file", dbPath,
		"--name", "Apple Corporation",
	}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security edit AAPL): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "Security updated successfully") {
		t.Error("output should confirm update")
	}
	if !strings.Contains(output, "Apple Corporation") {
		t.Error("output should show updated name")
	}

	stdout.Reset()
	if err := executeWith([]string{"security", "show", "AAPL", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security show AAPL): %v", err)
	}
	if !strings.Contains(stdout.String(), "Apple Corporation") {
		t.Error("name change should be persisted")
	}
}

func TestSecurityEdit_ChangeTicker(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{
		"security", "edit", "AAPL",
		"--file", dbPath,
		"--ticker", "AAPL2",
	}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security edit AAPL): %v\nstderr=%s", err, stderr)
	}

	if !strings.Contains(stdout.String(), "AAPL2") {
		t.Error("output should show new ticker")
	}

	stdout.Reset()
	if err := executeWith([]string{"security", "show", "AAPL", "--file", dbPath}, stdout, stderr); err == nil {
		t.Error("old ticker should not be found after rename")
	}

	stdout.Reset()
	if err := executeWith([]string{"security", "show", "AAPL2", "--file", dbPath}, stdout, stderr); err != nil {
		t.Errorf("new ticker should be found, got error: %v", err)
	}
}

func TestSecurityEdit_ChangeType(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{
		"security", "edit", "AAPL",
		"--file", dbPath,
		"--type", "etf",
	}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security edit AAPL): %v\nstderr=%s", err, stderr)
	}

	if !strings.Contains(stdout.String(), "ETF") {
		t.Error("output should show updated type")
	}
}

func TestSecurityEdit_InvalidType(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"security", "edit", "AAPL",
		"--file", dbPath,
		"--type", "invalid",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("editing with invalid type should return error")
	}
	if !strings.Contains(err.Error(), "invalid --type") {
		t.Errorf("error should mention invalid --type, got: %v", err)
	}
}

func TestSecurityEdit_InvalidAssetClass(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"security", "edit", "AAPL",
		"--file", dbPath,
		"--asset-class", "not_a_real_class",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("editing with invalid asset class should return error")
	}
	if !strings.Contains(err.Error(), "invalid --asset-class") {
		t.Errorf("error should mention invalid --asset-class, got: %v", err)
	}
}

func TestSecurityEdit_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"security", "edit", "AAPL", "EXTRA", "--file", "x.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(security edit AAPL EXTRA) should return error")
	}
}

func TestSecurityEdit_Help(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"security", "edit", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security edit --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "edit") {
		t.Errorf("expected `security edit --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestSecurityCmd_HelpListsEdit(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"security", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "edit") {
		t.Errorf("expected `security --help` to list `edit`; got:\n%s", stdout.String())
	}
}
