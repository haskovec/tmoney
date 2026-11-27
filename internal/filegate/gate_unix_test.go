//go:build unix

package filegate

import (
	"os"
	"syscall"
	"testing"
)

// The canaries below check the flock facts that the gate depends on. They
// were measured on macOS. If one fails on a system, the design of the gate
// does not hold there.

// flockNB tries an exclusive flock on f, with no wait.
func flockNB(t *testing.T, f *os.File) error {
	t.Helper()
	return control(f, func(fd int) error { return syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) })
}

func openGate(t *testing.T, file string) *os.File {
	t.Helper()
	f, err := os.OpenFile(Path(file), os.O_RDWR|os.O_CREATE, 0o666)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// Two open file descriptions of one file in one process conflict. That is why
// the gate counts the holds of this process.
func TestCanaryTwoFlocksInOneProcessConflict(t *testing.T) {
	file := dbFile(t)
	a, b := openGate(t, file), openGate(t, file)
	if err := flockNB(t, a); err != nil {
		t.Fatalf("first flock: %v", err)
	}
	if err := flockNB(t, b); !isHeld(err) {
		t.Errorf("second flock in this process = %v, want EWOULDBLOCK: two flocks in one process do not conflict here", err)
	}
}

// A flock stays when another descriptor of the same file closes. That is why
// the gate survives the opens and closes of the gate file in this process.
func TestCanaryAFlockStaysAfterTheCloseOfAnotherDescriptor(t *testing.T) {
	file := dbFile(t)
	a := openGate(t, file)
	if err := flockNB(t, a); err != nil {
		t.Fatalf("flock: %v", err)
	}
	b, err := os.Open(Path(file))
	if err != nil {
		t.Fatal(err)
	}
	_ = b.Close()
	if got := tryElsewhere(t, file); got != "busy" {
		t.Errorf("another process after the close of another descriptor: %s, want busy: the close dropped the flock", got)
	}
}
