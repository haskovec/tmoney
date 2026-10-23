package db

import (
	"os"
	"path/filepath"
	"testing"
)

// TestHomePath: DuckDB expands a leading "~" to the home folder, but the os
// calls on the path (the checks of Open, Create and the reopen, and every
// backup) do not. A handle on "~/x.tdb" must therefore hold the expanded path,
// or Create failed at its reopen and left the new file in the home folder.
func TestHomePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	want := filepath.Join(home, "fictional-home.tdb")

	database, err := Create("~/fictional-home.tdb")
	if err != nil {
		t.Fatalf("Create(~/...) error = %v", err)
	}
	if got := database.Path(); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("no database at %s: %v", want, err)
	}

	database, err = Open("~/fictional-home.tdb")
	if err != nil {
		t.Fatalf("Open(~/...) error = %v", err)
	}
	defer func() { _ = database.Close() }()
	if got := database.Path(); got != want {
		t.Errorf("Path() after Open = %q, want %q", got, want)
	}
}
