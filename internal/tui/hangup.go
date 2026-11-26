package tui

import (
	"errors"
	"os"
	"sync/atomic"
	"syscall"

	tea "charm.land/bubbletea/v2"
)

// A terminal that closes (a closed tab or window, a terminal that quits or
// restarts, a dropped ssh session) sends SIGHUP. Bubble Tea catches SIGINT
// and SIGTERM, but not SIGHUP, and Go's default for SIGHUP is to exit at once:
// no defer runs, so shutdown does not run. DuckDB then does not checkpoint
// (the committed rows wait in the WAL for the next open), and the quit backup
// is not made.

// hangupWatch is the state of watchHangup.
type hangupWatch struct {
	// hungUp is set when the first hangup comes, before quit runs.
	hungUp atomic.Bool
	// stopped closes when the watch has ended.
	stopped chan struct{}
}

// watchHangup turns the first signal on sig into quit, which ends the program
// as a SIGTERM does, so that Run reaches shutdown. It does not set
// App.quitting: the user did not quit. Later signals on sig do nothing; they
// must not stop the process while shutdown closes the database and copies it.
// The watch ends when done closes; stop the signals to sig before that.
//
// After a hangup, Run returns the error of the dead terminal, which nobody can
// read; see runError.
func watchHangup(sig <-chan os.Signal, done <-chan struct{}, quit func()) *hangupWatch {
	w := &hangupWatch{stopped: make(chan struct{})}
	go func() {
		defer close(w.stopped)
		for {
			select {
			case <-sig:
				if w.hungUp.CompareAndSwap(false, true) {
					quit()
				}
			case <-done:
				return
			}
		}
	}()
	return w
}

// quitOnHangup returns the quit func for watchHangup: a QuitMsg sent to p, as
// Bubble Tea's own handler sends one for SIGTERM. It is not p.Quit, which does
// nothing before p.Run has started: a hangup in that window would use the
// watch's one quit for nothing, the later hangups would be ignored, and the
// program would run on with no terminal. A Send before Run waits until the
// event loop reads it; Run cancels the program's context on every return, and
// that ends the wait.
func quitOnHangup(p *tea.Program) func() {
	return func() { p.Send(tea.QuitMsg{}) }
}

// runError is the error Run returns for err, the error of Bubble Tea's
// Program.Run. After a hangup, or when the terminal is gone (EIO: a write can
// fail before the SIGHUP reaches watchHangup), err only says that the screen
// could not be written. There is no screen to show it on, so it is dropped.
func runError(err error, hungUp bool) error {
	if hungUp || errors.Is(err, syscall.EIO) {
		return nil
	}
	return err
}
