// Package filegate is a lock between tmoney processes on one database file:
// the gate. DuckDB's own lock on the file is down whenever tmoney has the file
// closed: during an open, from before the last close to the end of the copy
// of a backup, and across each close and reopen. A tmoney process holds the
// gate for those windows, so that another tmoney does not open the file in
// one of them. A running tmoney does not hold the gate. So "gate busy" means
// "another tmoney changes the state of the file: wait", and "gate free, but
// DuckDB's lock held" means "another tmoney runs on the file: fail at once".
//
// The gate is an advisory lock (flock on unix, LockFileEx on Windows) on an
// empty file beside the database: .<name>.tmoney-lock. The lock is not on the
// database file itself, for four reasons. On unix, the close of any
// descriptor of a file drops the fcntl locks of the process on it. On macOS,
// a flock on the database file conflicts with this process's own DuckDB
// lock. A restore renames a new file over the database. And on Linux a flock
// does not stop DuckDB, which uses fcntl locks.
//
// The gate file is never written and never deleted: a delete while another
// process waits for the lock would let two processes hold the gate.
//
// In one process the gate counts its holders: a hold of a gate that this
// process holds already returns at once. The callers order their work inside
// the process with their own locks.
//
// Only tmoney takes the gate. It does not keep out another DuckDB client.
package filegate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// suffix ends the name of a gate file. The name names the app, as the
// partial files of the backup package do, so a user who sees it knows whose
// it is.
const suffix = ".tmoney-lock"

// pollInterval is the pause between two tries of a gate that another process
// holds. A var so tests can change it.
var pollInterval = 25 * time.Millisecond

// ErrBusy is the error of Hold when another process held the gate until the
// deadline. A *BusyError wraps it.
var ErrBusy = errors.New("the file is busy: another tmoney is closing it, or is making or restoring a backup of it")

// BusyError reports that another process held the gate of File until the
// deadline.
type BusyError struct {
	File string
}

func (e *BusyError) Error() string {
	return fmt.Sprintf("%s is busy: another tmoney is closing it, or is making or restoring a backup of it. Try again in a moment",
		filepath.Base(e.File))
}

// Unwrap returns ErrBusy.
func (e *BusyError) Unwrap() error { return ErrBusy }

// Path returns the path of the gate file of the database file at file. file
// must be resolved (no link as its last part) and absolute, so that each
// path to one file finds the same gate file.
func Path(file string) string {
	return filepath.Join(filepath.Dir(file), "."+filepath.Base(file)+suffix)
}

// held is a gate that this process holds.
type held struct {
	info os.FileInfo
	// files holds the descriptor with the lock first, then the descriptor
	// of each later hold. All stay open until the last release: on NFS,
	// flock has the semantics of fcntl locks, and the close of any
	// descriptor of the file drops the lock.
	files []*os.File
	count int
}

var (
	// mu guards gates. It is a leaf: Hold takes no other lock while it
	// holds mu, and calls waiting only without it.
	mu    sync.Mutex
	gates []*held
)

// errHeld is the result of one try when another process holds the gate.
var errHeld = errors.New("the gate is held")

// Hold takes the gate of the database file at file (see Path). While another
// process holds the gate, Hold tries again every pollInterval until deadline.
// It calls waiting, when not nil, once, at the first wait. At the deadline
// the error is a *BusyError and release is nil. A deadline that has passed
// gives one try and no call of waiting.
//
// Call release when the work that needs the gate ends. Each release works
// once; a second call of the same release does nothing.
//
// When the gate cannot work, Hold returns a release that does nothing, and no
// error: a folder that this user cannot write and that has no gate file, or
// a file system that has no such locks. The gate then never keeps the user
// from a database that DuckDB can open, and DuckDB's own lock still applies.
func Hold(file string, deadline time.Time, waiting func()) (release func(), err error) {
	told := false
	for {
		release, err := try(file)
		if !errors.Is(err, errHeld) {
			return release, err
		}
		left := time.Until(deadline)
		if left <= 0 {
			return nil, &BusyError{File: file}
		}
		if !told && waiting != nil {
			told = true
			waiting()
		}
		time.Sleep(min(pollInterval, left))
	}
}

// noGate is the release of a gate that cannot work.
func noGate() {}

// try takes the gate once, with no wait. It returns errHeld when another
// process holds the gate.
func try(file string) (func(), error) {
	f, err := openGateFile(Path(file))
	if err != nil {
		return noGate, nil
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return noGate, nil
	}
	mu.Lock()
	defer mu.Unlock()
	// The same gate file by another path (a link to the folder, or another
	// case of the name on a file system that ignores case) is the same gate.
	// A second lock by this process on another descriptor would conflict
	// with the first one, also in this process.
	for _, g := range gates {
		if os.SameFile(g.info, info) {
			g.count++
			g.files = append(g.files, f)
			return g.release(), nil
		}
	}
	switch err := lock(f); {
	case err == nil:
	case isHeld(err):
		_ = f.Close()
		return nil, errHeld
	default: // the file system has no such locks
		_ = f.Close()
		return noGate, nil
	}
	g := &held{info: info, files: []*os.File{f}, count: 1}
	gates = append(gates, g)
	return g.release(), nil
}

// openGateFile opens the gate file at path, and makes it when it is not
// there. A user who cannot write the gate file can still lock it: flock and
// LockFileEx need no write access. Hold never writes the file.
func openGateFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o666)
	if err == nil {
		return f, nil
	}
	return os.OpenFile(path, os.O_RDONLY, 0)
}

// release returns the release of one hold of g.
func (g *held) release() func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			mu.Lock()
			defer mu.Unlock()
			g.count--
			if g.count > 0 {
				return
			}
			_ = unlock(g.files[0])
			for _, f := range g.files {
				_ = f.Close()
			}
			for i, h := range gates {
				if h == g {
					gates = append(gates[:i], gates[i+1:]...)
					break
				}
			}
		})
	}
}
