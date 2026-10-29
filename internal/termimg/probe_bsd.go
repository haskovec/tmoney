//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package termimg

import "golang.org/x/sys/unix"

const (
	ioctlGetTermios = unix.TIOCGETA
	ioctlSetTermios = unix.TIOCSETA
)
