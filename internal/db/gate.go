package db

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/haskovec/tmoney/internal/filegate"
)

// GateWait is how long an open waits for the gate of the file (see
// filegate) while another tmoney holds it: while that tmoney opens or closes
// the file, or makes or restores a backup of it. A var so tests can shorten
// it.
var GateWait = 10 * time.Second

// ErrGateBusy is the error of a wait for the gate that ran out: another
// tmoney held the gate of the file until the deadline. The error is a
// *filegate.BusyError, which wraps ErrGateBusy.
var ErrGateBusy = filegate.ErrBusy

// holdGate takes the gate of the database at path and waits for it until
// deadline (see filegate.Hold). The gate is that of the file that DuckDB
// opens (FilePath), at its absolute path, so that a link and its file, and
// each working folder, find the same gate.
//
// For the lock order, see reopenMu.
func holdGate(path string, deadline time.Time, waiting func()) (release func(), err error) {
	file := FilePath(ExpandHome(path))
	if abs, err := filepath.Abs(file); err == nil {
		file = abs
	}
	return filegate.Hold(file, deadline, waiting)
}

// createWaiting, when not nil, is the waiting func of the gate wait of
// Create. A test hook: it shows when a Create waits for the gate.
var createWaiting func()

// HoldGate takes the gate of the database at path for a caller that has no
// handle on it, such as the CLI's db backup and db restore, and waits up to
// wait for it. It calls waiting, when not nil, once, when the wait starts.
// Call release when the work ends.
//
// A missing file has no gate: HoldGate then returns a release that does
// nothing, and makes no gate file. The caller reports the missing file.
// Every other path gets the gate, also one that is not a database: a restore
// makes it one.
func HoldGate(path string, wait time.Duration, waiting func()) (release func(), err error) {
	path = ExpandHome(path)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return func() {}, nil
	}
	return holdGate(path, time.Now().Add(wait), waiting)
}

// duckDBMagic is at duckDBMagicOffset in the header of every DuckDB file,
// after the checksum of the header.
const (
	duckDBMagic       = "DUCK"
	duckDBMagicOffset = 8
)

// canBeDatabase reports whether the file at path, which os.Stat described
// with info, can be a DuckDB database. Only such a file gets a gate from
// OpenWaiting, because the gate file stays beside the file for good
// (filegate): a wrong path, such as a CSV file or a folder, must not get one.
// A file that is too short to tell can be a database that DuckDB is making
// now, so it gets the gate.
//
// It reads only a regular file, because a read of a FIFO can block for ever.
// The close of the read drops the fcntl locks of this process on the file,
// as the DuckDB open that comes next does too. So the caller must not have
// the file open (the TUI refuses to open the file that it has open).
func canBeDatabase(path string, info os.FileInfo) bool {
	if !info.Mode().IsRegular() {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return true // the open reports it
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, duckDBMagicOffset+len(duckDBMagic))
	if _, err := io.ReadFull(f, head); err != nil {
		return true // too short to tell
	}
	return string(head[duckDBMagicOffset:]) == duckDBMagic
}
