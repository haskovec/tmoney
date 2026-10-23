//go:build windows

package db

import (
	"errors"
	"os"
	"syscall"
)

// ERROR_SHARING_VIOLATION and ERROR_LOCK_VIOLATION.
const (
	errSharingViolation = syscall.Errno(32)
	errLockViolation    = syscall.Errno(33)
)

// isSharingViolation reports whether err is the error of an open that another
// handle's share mode does not allow. The code, not the text: Windows gives
// the text in the language of the system.
func isSharingViolation(err error) bool {
	return errors.Is(err, errSharingViolation) || errors.Is(err, errLockViolation)
}

// HeldElsewhere reports whether another process has the database file at path
// open. It opens the file with no sharing, which fails with a sharing
// violation while any other handle with data access is open: DuckDB's
// read-write open shares only deletes, and its read-only open shares reads and
// deletes, so a plain open for reading would miss a read-only holder. Not
// tested on Windows.
func HeldElsewhere(path string) (bool, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err == nil {
		_ = syscall.CloseHandle(h)
		return false, nil
	}
	if isSharingViolation(err) {
		return true, nil
	}
	return false, &os.PathError{Op: "open", Path: path, Err: err}
}
