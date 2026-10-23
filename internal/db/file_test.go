package db

import (
	"os"
	"path/filepath"
	"testing"
)

// TestValidateFileChangesNothing: ValidateFile opens the file read-only, also
// in a folder whose name a URL parser reads in another way. duckdb-go reads
// the options of a DSN as a URL query, so in "Budget #2/fin.tdb?access_mode=..."
// the option is in the URL fragment, and the open was read-write: it wrote the
// rows of the WAL into the file and deleted the WAL.
func TestValidateFileChangesNothing(t *testing.T) {
	src := walOnlyFile(t, filepath.Join(t.TempDir(), "src.tdb"))
	for _, folder := range []string{"plain", "Budget #2", "100% saved", "it's"} {
		t.Run(folder, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), folder)
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "fin.tdb")
			for _, suffix := range []string{"", ".wal"} {
				copyFileTo(t, src+suffix, path+suffix)
			}
			before := fileSnapshot(path)

			if err := ValidateFile(path); err != nil {
				t.Fatalf("ValidateFile() error = %v", err)
			}
			if after := fileSnapshot(path); after != before {
				t.Errorf("ValidateFile changed the files: before %s, after %s", before, after)
			}
		})
	}
}

// walOnlyFile writes a closed TMoney database at path, in a plain folder, whose
// last committed row is only in <path>.wal, and returns path. The tests copy
// it into the other folders, because Create cannot open a path with a "%".
func walOnlyFile(t *testing.T, path string) string {
	t.Helper()
	database, err := Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	for _, q := range []string{`CHECKPOINT`, `PRAGMA disable_checkpoint_on_shutdown`} {
		if _, err := database.Conn().Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	insertProbe(t, database, "fictional_wal_row")
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".wal"); err != nil {
		t.Fatalf("no WAL after the close: %v", err)
	}
	return path
}
