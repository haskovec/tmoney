// Package dbtest provides fast, isolated test databases.
//
// Creating a database with db.Create runs every schema migration from scratch,
// which is the dominant per-test cost in the suite. dbtest copies a pre-migrated
// template instead: the template is built once per test binary (the first time a
// helper is called) and each test gets a private copy, so a test pays a cheap
// file copy + open instead of replaying all migrations.
//
// This generalizes the hand-rolled TestMain pattern in internal/investment into a
// shared helper any package's createTestDB can delegate to. It is safe under
// t.Parallel: the template build is guarded by sync.Once and each test writes to
// its own t.TempDir.
//
//   - New    returns an open DB closed via t.Cleanup (services/repository tests).
//   - NewFile returns the open DB and its path and does NOT auto-close, for
//     fixtures that seed, close, and hand the path to a command that reopens it.
package dbtest

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
)

var (
	templateOnce sync.Once
	templatePath string
	templateErr  error
)

// buildTemplate creates one fully-migrated database for the current test binary.
// The directory is intentionally left for the OS to reclaim: it lives for the
// life of the process and there is no TestMain hook to clean it up.
func buildTemplate() {
	dir, err := os.MkdirTemp("", "tmoney-test-template-*")
	if err != nil {
		templateErr = err
		return
	}
	path := filepath.Join(dir, "template.tdb")
	database, err := db.Create(path)
	if err != nil {
		templateErr = err
		return
	}
	if err := database.Close(); err != nil {
		templateErr = err
		return
	}
	templatePath = path
}

// copyTemplate writes a private copy of the migrated template into the test's
// temp dir under name and returns its path.
func copyTemplate(t *testing.T, name string) string {
	t.Helper()

	templateOnce.Do(buildTemplate)
	if templateErr != nil {
		t.Fatalf("dbtest: build template: %v", templateErr)
	}

	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("dbtest: read template: %v", err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("dbtest: write test db: %v", err)
	}
	return path
}

// New returns a fresh, fully-migrated database isolated to the test, closed via
// t.Cleanup. It copies a pre-migrated template rather than re-running migrations,
// which is dramatically faster than db.Create per test.
func New(t *testing.T) *db.DB {
	t.Helper()

	database, err := db.Open(copyTemplate(t, "test.tdb"))
	if err != nil {
		t.Fatalf("dbtest: open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// NewFile is like New but also returns the on-disk path and does NOT auto-close
// the database. Use it for fixtures that seed the DB, close it, and hand the path
// to a command that reopens the file. The file is reclaimed when the test's temp
// dir is cleaned.
func NewFile(t *testing.T, name string) (*db.DB, string) {
	t.Helper()

	path := copyTemplate(t, name)
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("dbtest: open test db: %v", err)
	}
	return database, path
}
