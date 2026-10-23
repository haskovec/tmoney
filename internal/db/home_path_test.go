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

// TestTildeUserPath: DuckDB expands any leading "~", also in "~user/x", but the
// os reads "~user" as a folder in the working folder. Create must make the file
// there, where every os call on the path looks, and nothing in the home folder.
func TestTildeUserPath(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(work)
	if err := os.Mkdir("~fictional", 0o755); err != nil {
		t.Fatal(err)
	}

	database, err := Create("~fictional/new.tdb")
	if err != nil {
		t.Fatalf("Create(~fictional/new.tdb) error = %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(work, "~fictional", "new.tdb")); err != nil {
		t.Errorf("no database in the working folder's ~fictional: %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(home)); len(entries) > 0 {
		for _, e := range entries {
			if e.Name() == filepath.Base(home)+"fictional" {
				t.Errorf("Create wrote %s next to the home folder", e.Name())
			}
		}
	}
}
