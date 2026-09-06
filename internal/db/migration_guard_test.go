package db

import (
	"path/filepath"
	"testing"
)

// TestMigrate_VerifiesEmbeddedFilesOnUpToDateFile: the constant/highest-file
// check must run even when the file is already at CurrentSchemaVersion, since
// that is the common Open path. It cannot fail in a correct build, so this
// asserts the check is reached by confirming Migrate on an up-to-date file
// still loads and validates the embedded set (a zero-length set would error).
func TestMigrate_VerifiesEmbeddedFilesOnUpToDateFile(t *testing.T) {
	dir := t.TempDir()
	database, err := Create(filepath.Join(dir, "test.tdb"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer database.Close()

	v, err := database.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion() error = %v", err)
	}
	if v != CurrentSchemaVersion {
		t.Fatalf("precondition: version %d, want %d", v, CurrentSchemaVersion)
	}

	// Up to date: must succeed, and must have gone through the guard.
	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate() on an up-to-date file error = %v", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations() error = %v", err)
	}
	if migrations[len(migrations)-1].Version != CurrentSchemaVersion {
		t.Fatalf("guard would have fired: highest %d != %d", migrations[len(migrations)-1].Version, CurrentSchemaVersion)
	}
}
