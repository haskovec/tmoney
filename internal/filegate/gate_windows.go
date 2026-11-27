//go:build windows

package filegate

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// allBytes is the length of the range that the lock covers: the whole gate
// file, which nobody reads or writes. A byte-range lock on Windows is
// mandatory, so it must never be on the database file itself.
const allBytes = ^uint32(0)

// lock takes an exclusive LockFileEx lock on f, with no wait. A lock through
// another handle conflicts with it, also in this process: that is why
// gate.go counts the holds of this process. Not tested on Windows.
func lock(f *os.File) error {
	return control(f, func(h windows.Handle) error {
		return windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
			0, allBytes, allBytes, &windows.Overlapped{})
	})
}

// unlock drops the lock on f. Windows also drops it when the handle closes,
// but its documentation says that this can take some time, so release
// calls unlock before the close.
func unlock(f *os.File) error {
	return control(f, func(h windows.Handle) error {
		return windows.UnlockFileEx(h, 0, allBytes, allBytes, &windows.Overlapped{})
	})
}

// isHeld reports whether err is the error of a lock that another handle
// holds: ERROR_LOCK_VIOLATION, or ERROR_IO_PENDING for a handle that is open
// for overlapped I/O (os.OpenFile does not open one so).
func isHeld(err error) bool {
	return errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING)
}

// control runs fn on the handle of f. The handle stays valid for the call.
func control(f *os.File, fn func(h windows.Handle) error) error {
	rc, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var opErr error
	if err := rc.Control(func(fd uintptr) { opErr = fn(windows.Handle(fd)) }); err != nil {
		return err
	}
	return opErr
}
