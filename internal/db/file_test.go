package db

import (
	"bytes"
	"errors"
	"fmt"
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

// TestCheckpointFileNeverCreatesADatabase: DuckDB's read-write open creates a
// missing file, and deletes a .wal beside it. CheckpointFile must not make a
// database or drop a log: a missing file is an error, and nothing changes.
func TestCheckpointFileNeverCreatesADatabase(t *testing.T) {
	for _, suffix := range append([]string{""}, WALSuffixes...) {
		t.Run("leftover"+suffix, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "gone.tdb")
			log := []byte("fictional log bytes")
			if suffix != "" {
				if err := os.WriteFile(path+suffix, log, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var notFound *FileNotFoundError
			if err := CheckpointFile(path); !errors.As(err, &notFound) {
				t.Errorf("CheckpointFile of a missing file: %v, want a *FileNotFoundError", err)
			}
			if _, err := os.Lstat(path); err == nil {
				t.Errorf("CheckpointFile created %s", filepath.Base(path))
			}
			if suffix != "" {
				if got, err := os.ReadFile(path + suffix); err != nil || string(got) != string(log) {
					t.Errorf("the leftover %s changed: %q, %v", suffix, got, err)
				}
			}
		})
	}
}

// TestCheckpointFileLeavesAnIgnoredLog: DuckDB ignores a log that does not
// read as one and leaves it after CHECKPOINT. Its bytes can still hold
// committed rows (a size field that points past the end), so CheckpointFile
// leaves it too; RemoveWAL removes it, on a copy only.
func TestCheckpointFileLeavesAnIgnoredLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fin.tdb")
	database, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	log := make([]byte, 4096)
	if err := os.WriteFile(path+".wal", log, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckpointFile(path); err != nil {
		t.Fatalf("CheckpointFile() error = %v", err)
	}
	if got, err := os.ReadFile(path + ".wal"); err != nil || len(got) != len(log) {
		t.Errorf("CheckpointFile removed or changed the ignored log: %d bytes, %v", len(got), err)
	}
	for range 2 { // the second call has nothing to remove
		if err := RemoveWAL(path); err != nil {
			t.Fatalf("RemoveWAL() error = %v", err)
		}
	}
	if HasWAL(path) {
		t.Error("RemoveWAL left a log")
	}
}

// TestValidateFileUnreadableLog: DuckDB opens the logs beside the file that it
// attaches, and reports a log that this process cannot read as an "IO Error",
// the same as a damaged file. A good file with such a log beside it is not
// corrupt: ValidateFile must say which file it cannot read. Restore copies
// every log with the backup, so each suffix counts, also one that DuckDB opens
// only during a stopped checkpoint.
func TestValidateFileUnreadableLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has only a read-only attribute, no permission bits")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads a file of any mode")
	}
	src := walOnlyFile(t, filepath.Join(t.TempDir(), "src.tdb"))
	for _, suffix := range WALSuffixes {
		t.Run(suffix, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fin.tdb")
			copyFileTo(t, src, path)
			copyFileTo(t, src+".wal", path+suffix)
			if err := os.Chmod(path+suffix, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(path+suffix, 0o600) })

			err := ValidateFile(path)
			var unreadable *UnreadableFileError
			if !errors.As(err, &unreadable) || !errors.Is(err, fs.ErrPermission) {
				t.Fatalf("ValidateFile(good file, mode-000 %s) error = %v (%T), want an *UnreadableFileError", suffix, err, err)
			}
			if filepath.Base(unreadable.Path) != "fin.tdb"+suffix {
				t.Errorf("UnreadableFileError.Path = %s, want the log fin.tdb%s", unreadable.Path, suffix)
			}
		})
	}
}

// TestValidateFileChecksTheLogOfTheTarget: for a symlink, DuckDB opens the
// log beside the file that the link points to, not beside the link.
func TestValidateFileChecksTheLogOfTheTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has only a read-only attribute, no permission bits")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads a file of any mode")
	}
	src := walOnlyFile(t, filepath.Join(t.TempDir(), "src.tdb"))
	target := filepath.Join(t.TempDir(), "fin.tdb")
	copyFileTo(t, src, target)
	link := filepath.Join(t.TempDir(), "link.tdb")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("no symlinks here:", err)
	}
	for _, p := range []string{link, target} {
		copyFileTo(t, src+".wal", p+".wal")
		if err := os.Chmod(p+".wal", 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(p+".wal", 0o600) })
	}
	var unreadable *UnreadableFileError
	err := ValidateFile(link)
	if !errors.As(err, &unreadable) || filepath.Base(unreadable.Path) != "fin.tdb.wal" {
		t.Errorf("ValidateFile(link) error = %v (%T), want an *UnreadableFileError for the log of the target, fin.tdb.wal", err, err)
	}
}

// TestFilePath: DuckDB opens the file at the end of a link, and keeps the
// logs beside it. FilePath gives that file for a link in the last part of the
// path, and the path as it is for anything else: the backup package compares
// the paths of a linked folder as text.
func TestFilePath(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"real", filepath.Join("real", "sub"), "links", filepath.Join("x", "y"), filepath.Join("x", "real")} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{filepath.Join("real", "fin.tdb"), filepath.Join("x", "real", "fin.tdb")} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("fictional database"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	real := filepath.Join(root, "real", "fin.tdb")
	for link, target := range map[string]string{ // as the link holds it
		filepath.Join("links", "abs.tdb"):      real,
		filepath.Join("links", "rel.tdb"):      filepath.Join("..", "real", "fin.tdb"),
		filepath.Join("links", "hop1.tdb"):     "hop2.tdb",
		filepath.Join("links", "hop2.tdb"):     real,
		filepath.Join("links", "dangling.tdb"): filepath.Join(root, "real", "gone.tdb"),
		"folder":                               filepath.Join(root, "real"),
		"inner":                                filepath.Join(root, "x", "y"),
		filepath.Join("x", "y", "up.tdb"):      filepath.Join("..", "real", "fin.tdb"),
		"sublink":                              filepath.Join(root, "real", "sub"),
	} {
		if err := os.Symlink(target, filepath.Join(root, link)); err != nil {
			t.Skip("no symlinks here:", err)
		}
	}
	resolve := func(path string) string {
		r, err := filepath.EvalSymlinks(path)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	for _, c := range []struct{ name, path, want string }{
		{"a plain file", real, real},
		{"a file in a linked folder", filepath.Join(root, "folder", "fin.tdb"), filepath.Join(root, "folder", "fin.tdb")},
		{"an absolute link", filepath.Join(root, "links", "abs.tdb"), resolve(real)},
		{"a relative link", filepath.Join(root, "links", "rel.tdb"), resolve(real)},
		{"a chain of links", filepath.Join(root, "links", "hop1.tdb"), resolve(real)},
		// The link is in x/y, so its "../real" is x/real, not the real/ that
		// a text join of inner/../real gives.
		{"a relative link in a linked folder", filepath.Join(root, "inner", "up.tdb"), resolve(filepath.Join(root, "x", "real", "fin.tdb"))},
		{"a dangling link", filepath.Join(root, "links", "dangling.tdb"), filepath.Join(root, "links", "dangling.tdb")},
		// The os resolves sublink before the "..": real/sub/.. is real. A
		// text clean removes "sublink/.." and names root/fin.tdb.
		{"a link and .. in the path", filepath.Join(root, "sublink") + string(filepath.Separator) + ".." + string(filepath.Separator) + "fin.tdb", resolve(real)},
	} {
		if got := FilePath(c.path); got != c.want {
			t.Errorf("%s: FilePath(%s) = %s, want %s", c.name, c.path, got, c.want)
		}
	}
}

// TestCheckpointFileInAnyFolder: duckdb-go parses a DSN as a URL, so an open
// by a DSN failed in a folder with a "%" in its name, and a "#" cut the path.
// Through a link, Restore folds its copy beside the real file, which can be in
// such a folder.
func TestCheckpointFileInAnyFolder(t *testing.T) {
	src := walOnlyFile(t, filepath.Join(t.TempDir(), "src.tdb"))
	plain := t.TempDir() // the folder of a subtest has its name, so a "%" too
	for i, folder := range []string{"Budget #2", "100% saved", "it's"} {
		t.Run(folder, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), folder)
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "fin.tdb")
			for _, suffix := range []string{"", ".wal"} {
				copyFileTo(t, src+suffix, path+suffix)
			}
			if err := CheckpointFile(path); err != nil {
				t.Fatalf("CheckpointFile() error = %v", err)
			}
			if HasWAL(path) {
				t.Error("the log is still there after the fold")
			}
			alone := filepath.Join(plain, fmt.Sprintf("alone-%d.tdb", i))
			copyFileTo(t, path, alone)
			assertProbe(t, alone, "fictional_wal_row")
		})
	}
}

// TestCheckpointFileThroughALinkAndDotDot: "link/../b.tdb" names the file that
// the os resolves through the link. A text clean of the path names another
// file, and the attach of that one made a new, empty database there.
func TestCheckpointFileThroughALinkAndDotDot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "real", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := walOnlyFile(t, filepath.Join(t.TempDir(), "src.tdb"))
	real := filepath.Join(root, "real", "b.tdb")
	for _, suffix := range []string{"", ".wal"} {
		copyFileTo(t, src+suffix, real+suffix)
	}
	if err := os.Symlink(filepath.Join(root, "real", "sub"), filepath.Join(root, "link")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	path := filepath.Join(root, "link") + string(filepath.Separator) + ".." + string(filepath.Separator) + "b.tdb"
	if err := CheckpointFile(path); err != nil {
		t.Fatalf("CheckpointFile(link/../b.tdb) error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "b.tdb")); err == nil {
		t.Error("CheckpointFile made a database at root/b.tdb")
	}
	if HasWAL(real) {
		t.Error("the log of real/b.tdb was not folded")
	}
	alone := filepath.Join(t.TempDir(), "alone.tdb")
	copyFileTo(t, real, alone)
	assertProbe(t, alone, "fictional_wal_row")
}

// TestCheckpointFileRefusesAQuestionMark: DuckDB looks for the log of a path
// with a "?" in another place (it puts ".wal" before the "?"), so a fold there
// saw no log and returned no error, and the copy of a backup then lost the
// rows of its log. It must fail, and change nothing.
func TestCheckpointFileRefusesAQuestionMark(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip(`Windows does not allow "?" in a file name`)
	}
	src := walOnlyFile(t, filepath.Join(t.TempDir(), "src.tdb"))
	dir := filepath.Join(t.TempDir(), "Budget?")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "fin.tdb")
	for _, suffix := range []string{"", ".wal"} {
		copyFileTo(t, src+suffix, path+suffix)
	}
	log, _ := os.ReadFile(path + ".wal")
	if err := CheckpointFile(path); err == nil {
		t.Error(`CheckpointFile of a path with a "?" and a log: no error`)
	}
	if got, err := os.ReadFile(path + ".wal"); err != nil || !bytes.Equal(got, log) {
		t.Errorf("the log changed: %d bytes, %v", len(got), err)
	}
}
