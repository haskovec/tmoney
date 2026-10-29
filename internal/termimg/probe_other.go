//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package termimg

import (
	"errors"
	"time"
)

// Probe has no /dev/tty to ask on this OS, so the CLI prints text only.
func Probe(time.Duration) (Reply, error) {
	return Reply{}, errors.New("no terminal query on this OS")
}
