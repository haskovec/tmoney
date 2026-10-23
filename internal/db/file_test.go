package db

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
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

// TestValidateFileReadsAPlainPath: DuckDB reads "<name>:" at the start of a
// path as the prefix of a storage extension (md:, sqlite:, postgres:), and
// installs a known one from the network. A file whose name starts that way is
// still a plain DuckDB file to ValidateFile.
func TestValidateFileReadsAPlainPath(t *testing.T) {
	dir := t.TempDir()
	src := walOnlyFile(t, filepath.Join(t.TempDir(), "src.tdb"))
	for _, suffix := range []string{"", ".wal"} {
		copyFileTo(t, src+suffix, filepath.Join(dir, "abc:fin.tdb"+suffix))
	}
	t.Chdir(dir)
	if err := ValidateFile("abc:fin.tdb"); err != nil {
		t.Errorf("ValidateFile(abc:fin.tdb) error = %v, want a valid TMoney file", err)
	}
}

// TestValidateFileThroughALinkAndDotDot: "link/../b.tdb" names the file that
// the os resolves through the link, not the one that a text clean of the path
// gives. ValidateFile must check the file that the backup copies.
func TestValidateFileThroughALinkAndDotDot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "real", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := walOnlyFile(t, filepath.Join(t.TempDir(), "src.tdb"))
	for _, suffix := range []string{"", ".wal"} {
		copyFileTo(t, src+suffix, filepath.Join(root, "real", "b.tdb"+suffix))
	}
	if err := os.Symlink(filepath.Join(root, "real", "sub"), filepath.Join(root, "link")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	if err := ValidateFile(filepath.Join(root, "link") + string(filepath.Separator) + ".." + string(filepath.Separator) + "b.tdb"); err != nil {
		t.Errorf("ValidateFile(link/../b.tdb) error = %v, want the valid file real/b.tdb", err)
	}
}

// TestValidateFileUnreadable: a file that this process cannot read can still
// be a good database (a backup that another user made of a database with mode
// 0600: a backup takes the mode of its database). DuckDB reports it as an "IO
// Error", the same as a file that a crash cut short, so ValidateFile must find
// it before the attach.
func TestValidateFileUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has only a read-only attribute, no permission bits")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads a file of any mode")
	}
	path := walOnlyFile(t, filepath.Join(t.TempDir(), "fin.tdb"))
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	err := ValidateFile(path)
	var unreadable *UnreadableFileError
	if !errors.As(err, &unreadable) || !errors.Is(err, fs.ErrPermission) {
		t.Errorf("ValidateFile(mode 000) error = %v (%T), want an *UnreadableFileError for fs.ErrPermission", err, err)
	}
}

// TestCheckedPoolOnEveryConnection: USE holds for one connection only, and
// database/sql can replace the connection of a pool at any call. Each
// connection of the pool of ValidateFile must read the attached file, with no
// extension install or load. Here the pool closes each connection after its
// use.
func TestCheckedPoolOnEveryConnection(t *testing.T) {
	path := walOnlyFile(t, filepath.Join(t.TempDir(), "fin.tdb"))
	pool, err := openChecked(path)
	if err != nil {
		t.Fatalf("openChecked() error = %v", err)
	}
	defer func() { _ = pool.Close() }()
	pool.SetConnMaxLifetime(time.Nanosecond)

	file := &DB{conn: pool, path: path}
	for i := range 3 {
		if err := file.validateTMoneyFile(); err != nil {
			t.Fatalf("pass %d: validateTMoneyFile() error = %v", i, err)
		}
		if _, err := file.SchemaVersion(); err != nil {
			t.Fatalf("pass %d: SchemaVersion() error = %v", i, err)
		}
		for _, setting := range []string{"autoinstall_known_extensions", "autoload_known_extensions"} {
			var on bool
			if err := pool.QueryRow(`SELECT current_setting('` + setting + `')`).Scan(&on); err != nil || on {
				t.Errorf("pass %d: %s = %v, %v; want false", i, setting, on, err)
			}
		}
	}
}
