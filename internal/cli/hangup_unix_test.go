//go:build unix

package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"

	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/dbtest"
)

// syncBuffer is a bytes.Buffer that the reader of the pty writes while the
// test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// ptyTUI is tmoney's TUI, run in a child process on a pseudo-terminal.
type ptyTUI struct {
	cmd *exec.Cmd
	// out is what the child drew on the pty. stderr is its standard error,
	// kept apart from the pty, so that what the child writes after a hangup
	// (an "Error:" line, a panic, a race report) is not lost with the pty.
	out, stderr *syncBuffer
	// hangUp closes the master side of the pty, which is what a terminal does
	// when its tab or window closes: the kernel hangs up the pty and sends
	// SIGHUP.
	hangUp func()
	ptmx   *os.File
}

// ptyOptions changes how startTUIOnPTY starts the child.
type ptyOptions struct {
	// bin is the binary to run; the test binary when empty.
	bin string
	// args come after "-f <dbPath>".
	args []string
	// env is added to the child's environment.
	env []string
}

// startTUIOnPTY starts tmoney's TUI on dbPath in a child process whose
// controlling terminal is a new pseudo-terminal, and waits until the TUI has
// drawn its first screen.
//
// The child gets only the variables set below and in opts.env: its own HOME
// and config folder, so it does not touch the user's config, and a fixed
// TERM. No other variable comes from the terminal that runs the test (for
// example TMUX, TERM_PROGRAM, COLORTERM or OSC7577_TOKEN).
func startTUIOnPTY(t *testing.T, dbPath string, opts ptyOptions) *ptyTUI {
	t.Helper()
	home := t.TempDir()
	bin := opts.bin
	if bin == "" {
		bin = os.Args[0]
	}
	cmd := exec.Command(bin, append([]string{"-f", dbPath}, opts.args...)...)
	cmd.Env = append([]string{
		runAsCLIEnv + "=1",
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"TERM=xterm-256color",
		"PATH=" + os.Getenv("PATH"),
	}, opts.env...)
	// pty sets only the streams that are nil, and the TUI draws on stdout.
	stderr := &syncBuffer{}
	cmd.Stderr = stderr

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		t.Fatalf("start tmoney on a pty: %v", err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	// Drain the pty, or the child blocks on a full buffer. The reader waits in
	// select with a timeout, never in a read: a read blocked on the master
	// holds it open in the kernel, so a Close would not hang the pty up.
	out := &syncBuffer{}
	fd := int(ptmx.Fd())
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		buf := make([]byte, 32*1024)
		for {
			select {
			case <-stop:
				return
			default:
			}
			var rset unix.FdSet
			rset.Set(fd)
			timeout := unix.NsecToTimeval((20 * time.Millisecond).Nanoseconds())
			n, err := unix.Select(fd+1, &rset, nil, nil, &timeout)
			if errors.Is(err, unix.EINTR) || (err == nil && n == 0) {
				continue
			}
			if err != nil {
				return
			}
			m, err := unix.Read(fd, buf)
			if m > 0 {
				_, _ = out.Write(buf[:m])
			}
			if err != nil && !errors.Is(err, unix.EINTR) && !errors.Is(err, unix.EAGAIN) {
				return
			}
			if m == 0 && err == nil {
				return // EOF
			}
		}
	}()
	var hangUpOnce sync.Once
	hangUp := func() {
		hangUpOnce.Do(func() {
			close(stop)
			<-stopped
			if err := ptmx.Close(); err != nil {
				t.Errorf("close the pty: %v", err)
			}
		})
	}
	t.Cleanup(hangUp)

	// The status bar's key hints are on every screen.
	deadline := time.Now().Add(30 * time.Second)
	for !strings.Contains(out.String(), "navigate") {
		if time.Now().After(deadline) {
			t.Fatalf("the TUI did not draw within 30s; output so far:\n%q\nstderr:\n%s", out.String(), stderr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	return &ptyTUI{cmd: cmd, out: out, stderr: stderr, hangUp: hangUp, ptmx: ptmx}
}

// send writes keys to the child's terminal, as a user who types them.
func (p *ptyTUI) send(t *testing.T, keys string) {
	t.Helper()
	if _, err := p.ptmx.WriteString(keys); err != nil {
		t.Fatalf("type %q: %v", keys, err)
	}
}

// waitExit waits for the child to end, or fails the test after timeout.
func (p *ptyTUI) waitExit(t *testing.T, timeout time.Duration) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case err := <-done:
		var exitErr *exec.ExitError
		if err != nil && !errors.As(err, &exitErr) {
			t.Fatalf("wait for tmoney: %v; stderr:\n%s", err, p.stderr)
		}
	case <-time.After(timeout):
		_ = p.cmd.Process.Kill()
		t.Fatalf("tmoney did not exit within %v after the terminal closed; stderr:\n%s", timeout, p.stderr)
	}
}

// When the terminal goes away, tmoney ends as if the user quit: it is not
// killed by the SIGHUP, it closes the database, and it makes the quit backup.
// Before the fix, the SIGHUP stopped the process at once, and no backup was
// made.
func TestTUI_TerminalHangupClosesDatabaseAndBacksUp(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "hangup.tdb")
	if err := database.Close(); err != nil {
		t.Fatalf("close the seeded database: %v", err)
	}

	tui := startTUIOnPTY(t, dbPath, ptyOptions{})
	tui.hangUp() // the terminal closes
	// Shutdown waits up to db.SettleTimeout (5 s) for work in flight.
	tui.waitExit(t, 20*time.Second)

	ws, ok := tui.cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		t.Fatalf("unexpected wait status type %T", tui.cmd.ProcessState.Sys())
	}
	if ws.Signaled() {
		t.Fatalf("tmoney was killed by %v; it must catch the hangup and shut down; stderr:\n%s", ws.Signal(), tui.stderr)
	}
	if code := ws.ExitStatus(); code != 0 {
		t.Errorf("tmoney exited with code %d after a hangup, want 0; stderr:\n%s", code, tui.stderr)
	}

	backups, err := backup.ListBackups(dbPath)
	if err != nil {
		t.Fatalf("list backups: %v", err)
	}
	if len(backups) == 0 {
		t.Errorf("no quit backup was made after the hangup; stderr:\n%s", tui.stderr)
	}
}
