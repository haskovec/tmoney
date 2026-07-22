package category_test

import (
	"testing"

	categorydom "github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/types"
)

// newCatFile creates a fresh database file and returns a closed handle's path
// plus an open handle. Callers seed via the returned db then close it before
// invoking a CLI command (the CLI opens the file itself).
func newCatFile(t *testing.T) (*db.DB, string) {
	t.Helper()
	return dbtest.NewFile(t, "test.tdb")
}

// reopenCat reopens the DB at path and returns a category repository for
// verifying a command's persisted effect.
func reopenCat(t *testing.T, path string) (*db.DB, *categorydom.Repository) {
	t.Helper()
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	return database, categorydom.NewRepository(database)
}

// seedTopLevel creates a top-level category and returns its ID.
func seedTopLevel(t *testing.T, database *db.DB, name string, typ categorydom.Type) types.ID {
	t.Helper()
	repo := categorydom.NewRepository(database)
	cat := categorydom.NewCategory(name, typ)
	if err := repo.Create(cat); err != nil {
		t.Fatalf("seed top-level %q: %v", name, err)
	}
	return cat.ID
}

// seedChild creates a subcategory under parentID and returns its ID.
func seedChild(t *testing.T, database *db.DB, name string, parentID types.ID, typ categorydom.Type) types.ID {
	t.Helper()
	repo := categorydom.NewRepository(database)
	cat := categorydom.NewSubcategory(name, parentID, typ)
	if err := repo.Create(cat); err != nil {
		t.Fatalf("seed child %q: %v", name, err)
	}
	return cat.ID
}
