package cmdutil

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli/clitest"
)

// captureStderr runs fn with os.Stderr redirected and returns what it wrote.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()
	fn()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

// A failed open-time repair is printed, and the command still gets its
// services: failing here would lock the user out of the repair tools.
func TestOpenServices_WarnsOnRepairFailureAndContinues(t *testing.T) {
	path := clitest.DamagedHealFile(t)

	var openErr error
	stderr := captureStderr(t, func() {
		database, svc, err := OpenServices(path)
		openErr = err
		if err == nil {
			if svc == nil {
				t.Error("OpenServices returned no services")
			}
			_ = database.Close()
		}
	})

	if openErr != nil {
		t.Fatalf("OpenServices() error = %v, want the repair failure shown, not returned", openErr)
	}
	if !strings.Contains(stderr, "warning: startup repair failed") || !strings.Contains(stderr, `account "Broken Brokerage"`) {
		t.Errorf("stderr = %q, want a startup repair warning that names the broken account", stderr)
	}
}
