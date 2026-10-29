//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package termimg

import (
	"time"

	"golang.org/x/sys/unix"
)

// Probe sends the query to the terminal on /dev/tty and reads the replies
// until the DA1 reply, or until timeout. With no /dev/tty it returns an
// error. The terminal mode is restored on every path.
func Probe(timeout time.Duration) (Reply, error) {
	fd, err := unix.Open("/dev/tty", unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return Reply{}, err
	}
	defer func() { _ = unix.Close(fd) }()

	saved, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
	if err != nil {
		return Reply{}, err
	}
	raw := *saved
	// No echo and no line buffer, so the replies are not shown and come at
	// once. No signals, so a Ctrl+C cannot end the program before the mode
	// is restored. VMIN 0 and VTIME 1: a read returns after 100 ms with
	// nothing, so the timeout works.
	raw.Lflag &^= unix.ECHO | unix.ICANON | unix.ISIG
	raw.Cc[unix.VMIN] = 0
	raw.Cc[unix.VTIME] = 1
	if err := unix.IoctlSetTermios(fd, ioctlSetTermios, &raw); err != nil {
		return Reply{}, err
	}
	defer func() { _ = unix.IoctlSetTermios(fd, ioctlSetTermios, saved) }()

	if _, err := unix.Write(fd, []byte(probeQuery)); err != nil {
		return Reply{}, err
	}
	var got []byte
	buf := make([]byte, 256)
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); {
		n, err := unix.Read(fd, buf)
		if err != nil && err != unix.EINTR && err != unix.EAGAIN {
			return Reply{}, err
		}
		if n <= 0 {
			continue
		}
		got = append(got, buf[:n]...)
		if r, done := parseReply(got); done {
			return r, nil
		}
	}
	r, _ := parseReply(got)
	return r, nil
}
