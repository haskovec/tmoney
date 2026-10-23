//go:build unix

package db

import (
	"io"
	"os"
	"syscall"
)

// HeldElsewhere reports whether another process has the database file at path
// open. DuckDB holds an fcntl lock on its database file while it has it open:
// a write lock for a read-write open, a read lock for a read-only one. F_GETLK
// asks the kernel whether a write lock would conflict with either, so the
// answer does not depend on DuckDB's error text. The file opens for reading,
// so a read-only file is checked too.
//
// It does not see a lock of this process, and the close of its descriptor
// drops every fcntl lock that this process holds on the file (see the note
// above ValidateFile): close the database here first.
func HeldElsewhere(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	lock := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: io.SeekStart}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_GETLK, &lock); err != nil {
		return false, err
	}
	return lock.Type != syscall.F_UNLCK, nil
}

// isSharingViolation is for Windows only (see lock_windows.go).
func isSharingViolation(error) bool { return false }
