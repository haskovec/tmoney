package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// WALSuffixes name the files beside a DuckDB database file that can hold
// committed rows that the file does not: the write-ahead log, and the logs that
// DuckDB 1.5 keeps beside it while a checkpoint runs (<path>.wal.checkpoint)
// and while it recovers (<path>.wal.recovery). A crash, a kill, or a fatal
// error that invalidates the instance can leave any of them, and the next open
// reads them.
var WALSuffixes = []string{".wal", ".wal.checkpoint", ".wal.recovery"}

// HasWAL reports whether any of the WAL files of the database at path exists.
func HasWAL(path string) bool {
	return len(WALFiles(path)) > 0
}

// WALFiles returns the WAL files that exist beside the database at path, in
// the order of WALSuffixes.
func WALFiles(path string) []string {
	base := FilePath(path)
	var files []string
	for _, suffix := range WALSuffixes {
		if _, err := os.Lstat(base + suffix); err == nil {
			files = append(files, base+suffix)
		}
	}
	return files
}

// FilePath returns the path of the file that DuckDB opens for path: path
// itself, or, when the last part of path is a symbolic link (a database kept
// in a synced folder through a link), the file at the end of the link. DuckDB
// keeps the WAL files beside that file, and a rename must replace that file,
// not the link. A link to a folder does not count: the backup package names
// and compares the files in it as text. A path that is not a link, or whose
// link does not resolve, comes back as it is.
//
// The exception is a path in which a text clean names another file than the
// os opens, such as "link/../fin.tdb": the os resolves the link before the
// "..", and filepath.Dir and Join do not. FilePath resolves such a path too.
//
// It uses EvalSymlinks, not a loop over Readlink: a relative target is
// relative to the folder that the link is really in, which a text join of a
// path through a linked folder gets wrong.
func FilePath(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return path
	}
	if info.Mode()&os.ModeSymlink == 0 {
		if clean, err := os.Lstat(filepath.Clean(path)); err == nil && os.SameFile(info, clean) {
			return path
		}
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

// The functions in this file work on a database file that is not open in this
// process: a backup, or the backup package's own copy of a database. They open
// the file with a new DuckDB instance, and on POSIX systems the close of that
// instance's file handle releases every lock that this process holds on the
// file, so another process could then open a database that is open here.

// ValidateFile checks, without changing the file, that path is a complete
// TMoney database this app can open: DuckDB attaches it read-only, the
// _metadata table names TMoney (validateTMoneyFile), and its schema version is
// not newer than this app. It runs no migrations and writes nothing.
//
// Restore calls it on a backup, and on its copy of the backup before that copy
// replaces the database. DuckDB refuses to open a file that is shorter than its
// header says, so a backup cut short by a crash during its copy fails here, not
// after the swap.
//
// A file that this process cannot read gives an *UnreadableFileError, and one
// that another process has open gives a *LockedFileError, not a
// *CorruptedFileError: each can be a good database. DuckDB reports both as an
// "IO Error", the same as a file that a crash cut short, so ValidateFile opens
// the file, and the WAL files beside the file that it attaches, for reading
// before the attach, and looks for DuckDB's lock error after it.
//
// The file is attached to an in-memory database by a SQL string, not opened by
// a DSN: duckdb-go parses a DSN as a URL, so a "#" in a folder name moved the
// read-only option into the URL fragment (a read-write open), and a "%" failed
// the parse. The attach names the DuckDB storage and an absolute path, so a
// file name that starts like an extension prefix ("md:", "sqlite:") is a plain
// file, and a SQLite file with the right tables is not a TMoney database; and
// the instance installs and loads no extensions, so no check goes to the
// network.
func ValidateFile(path string) error {
	if err := readCheck(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &FileNotFoundError{Path: path}
		}
		return err
	}
	// EvalSymlinks before Abs: Abs cleans "link/.." away as text, but the os
	// resolves the link first, and so does the copy of the file.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return &UnreadableFileError{Path: path, Err: err}
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return &DatabaseError{Op: "validate", Err: err}
	}
	// DuckDB opens the logs beside the file that it attaches, not beside a
	// link to it. Restore copies every log with the backup.
	for _, suffix := range WALSuffixes {
		if err := readCheck(abs + suffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	conn, err := openChecked(abs)
	if err != nil {
		return &DatabaseError{Op: "validate", Err: err}
	}
	defer func() { _ = conn.Close() }()
	if err := conn.Ping(); err != nil { // the first connection attaches the file
		// DuckDB's text can be in the language of the system: ask the os too.
		if held, herr := HeldElsewhere(abs); IsLockedElsewhere(err) || (herr == nil && held) {
			return &LockedFileError{Path: path, Err: err}
		}
		return &CorruptedFileError{Path: path, Err: err}
	}

	file := &DB{conn: conn, path: path}
	if err := file.validateTMoneyFile(); err != nil {
		return err
	}
	version, err := file.SchemaVersion()
	if err != nil {
		return err
	}
	if version > CurrentSchemaVersion {
		return &VersionMismatchError{FileVersion: version, AppVersion: CurrentSchemaVersion}
	}
	return nil
}

// readCheck opens path for reading and closes it. A file that this process
// cannot read is an *UnreadableFileError, one that another process holds is a
// *LockedFileError, and a missing file is the os error, which wraps
// fs.ErrNotExist.
func readCheck(path string) error {
	f, err := os.Open(path)
	switch {
	case err == nil:
		_ = f.Close()
		return nil
	case errors.Is(err, fs.ErrNotExist):
		return err
	case IsLockedElsewhere(err) || isSharingViolation(err): // Windows: a read-write DuckDB open shares no reads
		return &LockedFileError{Path: path, Err: err}
	default:
		return &UnreadableFileError{Path: path, Err: err}
	}
}

// openChecked opens a pool on an in-memory DuckDB instance whose connections
// see the file at abs, attached read-only as "checked", as their default
// database. validateTMoneyFile and SchemaVersion then read it with their
// usual queries.
//
// Every connection gets the same state, so that the checks do not depend on
// database/sql giving back one connection: it can replace a connection at any
// call. The extension settings are instance options in the DSN, which apply
// before the first connection. The ATTACH is for the whole instance, so a
// connection after the first one finds it there. USE is for one connection
// only, so each new connection runs it again.
func openChecked(abs string) (*sql.DB, error) {
	quoted := "'" + strings.ReplaceAll(abs, "'", "''") + "'"
	return openPoolInit(":memory:?autoinstall_known_extensions=false&autoload_known_extensions=false",
		func(c driver.ExecerContext) error {
			for _, q := range []string{
				`ATTACH IF NOT EXISTS ` + quoted + ` AS checked (TYPE DUCKDB, READ_ONLY)`,
				`USE checked`,
			} {
				if _, err := c.ExecContext(context.Background(), q, nil); err != nil {
					return err
				}
			}
			return nil
		})
}

// IsLockedElsewhere reports whether err is DuckDB's report that another
// process holds the lock on the database file: another TMoney, or another
// DuckDB client, has it open.
func IsLockedElsewhere(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Could not set lock on file") || // POSIX
		strings.Contains(msg, "being used by another process") // Windows
}

// CheckpointFile writes the WAL files beside the DuckDB file at path (see
// WALSuffixes) into the file, so that the file alone holds what an open of it
// shows. The open replays them, as every open does, and CHECKPOINT writes the
// rows to the file and removes the logs. The backup package calls it on its
// own copy of a database whose logs still hold committed rows, and Restore on
// the database it replaces: a crash, or an instance that a fatal error
// invalidated, skips the checkpoint at close.
//
// A missing file is a *FileNotFoundError: DuckDB's read-write open would
// create it, and delete a .wal beside it, so CheckpointFile never makes a
// database or drops a log.
//
// DuckDB ignores a log that it reads as torn (all zeros after a crash, or a
// first entry whose size points past the end) and leaves it beside the file,
// also after CHECKPOINT. CheckpointFile leaves it too: its bytes can still
// hold committed rows. RemoveWAL removes it from a copy. A damaged log fails
// the open, and CheckpointFile returns that error and changes nothing. DuckDB
// has no check that a log belongs to the file: it replays the log of another
// file as it would at any open of the database.
//
// The read-write open also fails when another process has the database open:
// DuckDB locks the file (see IsLockedElsewhere and HeldElsewhere).
//
// The file is attached read-write to an in-memory database by a SQL string,
// as in ValidateFile, not opened by a DSN: a "%" or a "#" in a folder name
// broke the DSN, and through a link Restore folds a copy beside the real
// file, in whatever folder that is.
func CheckpointFile(path string) error {
	if _, err := os.Stat(path); err != nil { // Stat follows a link, as DuckDB does
		if errors.Is(err, fs.ErrNotExist) {
			return &FileNotFoundError{Path: path}
		}
		return err
	}
	// EvalSymlinks before Abs, as in ValidateFile: Abs cleans "link/.." away as
	// text, and the attach of that path made a new database where none was.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return err
	}
	// DuckDB puts ".wal" before the first "?" of a path (the query of a
	// URL), so it looks for the log of such a file in another place, folds
	// nothing, and reports no error.
	if strings.Contains(abs, "?") && HasWAL(abs) {
		return fmt.Errorf("cannot apply the write-ahead log of %s: DuckDB does not find the log of a path with a \"?\"", path)
	}
	conn, err := openPoolInit(":memory:?autoinstall_known_extensions=false&autoload_known_extensions=false", nil)
	if err != nil {
		return err
	}
	quoted := "'" + strings.ReplaceAll(abs, "'", "''") + "'"
	_, err = conn.Exec(`ATTACH ` + quoted + ` AS folded (TYPE DUCKDB)`)
	if err == nil {
		_, err = conn.Exec(`CHECKPOINT folded`)
	}
	// No connection is checked out, so Close ends the instance now, and
	// with it the attach.
	if cerr := conn.Close(); err == nil {
		err = cerr
	}
	return err
}

// RemoveWAL removes the WAL files of the database at path. After a
// CheckpointFile without an error, a log that is still there is one that
// DuckDB ignored, so the file alone holds what an open shows. The backup
// package calls it on its own copy only: on a database, such a log is kept
// (see CheckpointFile).
func RemoveWAL(path string) error {
	for _, file := range WALFiles(path) {
		if err := os.Remove(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}
