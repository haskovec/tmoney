package dbtest

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
)

// openChildEnv, when set in the environment of a test binary whose TestMain
// calls RunOpenChild, names a database that the binary opens once, as another
// tmoney would, instead of running its tests.
const openChildEnv = "TMONEY_TEST_OPEN_CHILD"

// RunOpenChild runs the child of OpenElsewhere and ends the process, when the
// environment asks for it. Otherwise it returns at once. Call it first in
// TestMain.
func RunOpenChild() {
	path := os.Getenv(openChildEnv)
	if path == "" {
		return
	}
	database, err := db.OpenWaiting(path, 0, nil)
	switch {
	case err == nil:
		_ = database.Close()
		fmt.Println("opened")
	case errors.Is(err, db.ErrGateBusy):
		fmt.Println("busy")
	case db.IsLockedElsewhere(err):
		fmt.Println("locked")
	default:
		fmt.Println("error:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// OpenElsewhere opens the database at path once in another process, as
// another tmoney would, but with no wait for the gate of the file (see
// db.OpenWaiting). It returns what that open found: "opened", "busy" (a
// tmoney holds the gate: it closes the file, or copies it) or "locked" (a
// process has the file open). The test binary must call RunOpenChild in its
// TestMain.
func OpenElsewhere(t *testing.T, path string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), openChildEnv+"="+path)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("the open in another process failed: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}
