package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
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
	for _, suffix := range WALSuffixes {
		if _, err := os.Lstat(path + suffix); err == nil {
			return true
		}
	}
	return false
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
// the file for reading before the attach, and looks for DuckDB's lock error
// after it.
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
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &FileNotFoundError{Path: path}
		}
		if IsLockedElsewhere(err) { // Windows: DuckDB opens a file with no sharing
			return &LockedFileError{Path: path, Err: err}
		}
		return &UnreadableFileError{Path: path, Err: err}
	}
	_ = f.Close()
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
	conn, err := openChecked(abs)
	if err != nil {
		return &DatabaseError{Op: "validate", Err: err}
	}
	defer func() { _ = conn.Close() }()
	if err := conn.Ping(); err != nil { // the first connection attaches the file
		if IsLockedElsewhere(err) {
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
// DuckDB ignores a log that does not read as one (all zeros after a crash, or
// garbage) and leaves it beside the file, also after CHECKPOINT. The file then
// already holds what the open showed, so CheckpointFile removes such logs. A
// log that DuckDB cannot replay (damaged, or the log of another file) fails
// the open, and CheckpointFile returns that error and changes nothing.
func CheckpointFile(path string) error {
	conn, err := openPoolAt(path)
	if err != nil {
		return err
	}
	_, err = conn.Exec(`CHECKPOINT`)
	// No connection is checked out, so Close ends the instance now.
	if cerr := conn.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	for _, suffix := range WALSuffixes {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
