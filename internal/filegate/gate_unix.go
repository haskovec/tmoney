//go:build unix

package filegate

import (
	"errors"
	"os"
	"syscall"
)

// lock takes an exclusive flock on f, with no wait. A flock belongs to the
// open file description, so the close of another descriptor of the file
// does not drop it (on a local file system). Another open file description
// of the file conflicts with it, also in this process: that is why gate.go
// counts the holds of this process.
func lock(f *os.File) error {
	return control(f, func(fd int) error { return syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) })
}

// unlock drops the flock on f. The close of f drops it too.
func unlock(f *os.File) error {
	return control(f, func(fd int) error { return syscall.Flock(fd, syscall.LOCK_UN) })
}

// isHeld reports whether err is the error of a lock that another open file
// description holds.
func isHeld(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}

// control runs fn on the descriptor of f, and again after EINTR. It goes
// through SyscallConn, not Fd: Fd puts the file in blocking mode.
func control(f *os.File, fn func(fd int) error) error {
	rc, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var opErr error
	if err := rc.Control(func(fd uintptr) {
		for {
			opErr = fn(int(fd))
			if !errors.Is(opErr, syscall.EINTR) {
				return
			}
		}
	}); err != nil {
		return err
	}
	return opErr
}
