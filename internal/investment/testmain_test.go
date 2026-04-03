package investment

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
)

// testDBTemplatePath holds the path to a pre-migrated database template file.
// This avoids running migrations for every single test, dramatically speeding
// up the test suite (121+ tests each creating a DuckDB with 10+ migrations).
var testDBTemplatePath string

func TestMain(m *testing.M) {
	// Create a temporary directory for the template DB
	tmpDir, err := os.MkdirTemp("", "investment-test-template-*")
	if err != nil {
		panic("failed to create temp dir for DB template: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	// Create the template database with all migrations applied
	templatePath := filepath.Join(tmpDir, "template.tdb")
	database, err := db.Create(templatePath)
	if err != nil {
		panic("failed to create template database: " + err.Error())
	}
	database.Close()

	testDBTemplatePath = templatePath

	os.Exit(m.Run())
}

// createTestDBFromTemplate creates a test database by copying the pre-migrated
// template instead of running all migrations from scratch. This is significantly
// faster than calling db.Create for each test.
func createTestDBFromTemplate(t *testing.T) *db.DB {
	t.Helper()
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "test.tdb")

	// Copy the template file
	data, err := os.ReadFile(testDBTemplatePath)
	if err != nil {
		t.Fatalf("Failed to read template DB: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("Failed to write test DB: %v", err)
	}

	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}
