package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

func TestSecurityDelete_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"security", "delete", "AAPL"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(security delete AAPL) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestSecurityDelete_MissingTicker(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"security", "delete"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(security delete) without ticker should return error")
	}
	if !strings.Contains(err.Error(), "arg") {
		t.Errorf("expected Cobra arg-count error, got: %v", err)
	}
}

func TestSecurityDelete_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"security", "delete", "FAKE", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("deleting non-existent security should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestSecurityDelete_Success(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"security", "delete", "AAPL", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security delete AAPL): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "deleted successfully") {
		t.Errorf("output should confirm deletion, got: %s", output)
	}
	if !strings.Contains(output, "AAPL") {
		t.Errorf("output should contain ticker, got: %s", output)
	}

	stdout.Reset()
	if err := executeWith([]string{"security", "show", "AAPL", "--file", dbPath}, stdout, stderr); err == nil {
		t.Error("deleted security should not be found via `security show`")
	}
}

func TestSecurityDelete_WithPricesSuggestsHide(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}

	repo := security.NewRepository(database)
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := repo.Create(sec); err != nil {
		t.Fatalf("setup: create security: %v", err)
	}

	if _, err := database.Conn().Exec(
		`INSERT INTO security_prices (id, security_id, date, price, source, created_at)
		 VALUES (?, ?, '2024-01-01', 150.00, 'manual', CURRENT_TIMESTAMP)`,
		types.NewID().String(), sec.ID.String(),
	); err != nil {
		t.Fatalf("setup: insert price: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"security", "delete", "AAPL", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("deleting security with prices should return error")
	}
	if !strings.Contains(err.Error(), "security hide") {
		t.Errorf("error should suggest using `security hide`, got: %v", err)
	}
}

func TestSecurityDelete_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"security", "delete", "AAPL", "EXTRA", "--file", "x.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(security delete AAPL EXTRA) should return error")
	}
}

func TestSecurityDelete_Help(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"security", "delete", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security delete --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "delete") {
		t.Errorf("expected `security delete --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestSecurityCmd_HelpListsDelete(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"security", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "delete") {
		t.Errorf("expected `security --help` to list `delete`; got:\n%s", stdout.String())
	}
}
