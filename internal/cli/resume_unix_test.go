//go:build unix

package cli

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/types"
)

// testResumeToken stands in for the token that a terminal with OSC 7577
// support sets in OSC7577_TOKEN.
const testResumeToken = "tmoneyTestToken_0123456789abcdef"

// resumeReport is one OSC 7577 report from the child's output.
type resumeReport struct {
	argv  []string
	clear bool
}

// resumeReports returns the OSC 7577 reports in out, in order. It fails the
// test on a report whose mac does not match testResumeToken.
func resumeReports(t *testing.T, out string) []resumeReport {
	t.Helper()
	var reports []resumeReport
	for rest := out; ; {
		i := strings.Index(rest, "\x1b]7577;")
		if i < 0 {
			return reports
		}
		rest = rest[i+len("\x1b]7577;"):]
		end := strings.IndexAny(rest, "\x07\x1b")
		if end < 0 {
			return reports // cut off; the next read has the rest
		}
		body := rest[:end]
		rest = rest[end:]

		signed, mac, ok := strings.Cut(body, ":mac=")
		if !ok {
			t.Fatalf("OSC 7577 report with no mac: %q", body)
		}
		sum := sha256.Sum256([]byte(signed + ":" + testResumeToken))
		if mac != hex.EncodeToString(sum[:]) {
			t.Fatalf("OSC 7577 report with a wrong mac: %q", body)
		}
		var r resumeReport
		for _, kv := range strings.Split(signed, ":") {
			k, v, _ := strings.Cut(kv, "=")
			switch k {
			case "clear":
				r.clear = v == "1"
			case "argv":
				for _, enc := range strings.Split(v, ",") {
					arg, err := base64.StdEncoding.DecodeString(enc)
					if err != nil {
						t.Fatalf("OSC 7577 argv element %q: %v", enc, err)
					}
					r.argv = append(r.argv, string(arg))
				}
			}
		}
		reports = append(reports, r)
	}
}

// copyTestBinary copies the test binary out of its go-build folder, which
// tmoney does not report (see tui.resumeExecutable), and returns the copy.
func copyTestBinary(t *testing.T) string {
	t.Helper()
	src, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = src.Close() }()
	// os.Executable resolves symlinks on Linux (/proc/self/exe), and gives
	// the path that ran on macOS. A folder with no symlink gives the same
	// path on both, so the report can be compared as text.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "tmoney")
	dst, err := os.OpenFile(bin, os.O_CREATE|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	if err := dst.Close(); err != nil {
		t.Fatal(err)
	}
	return bin
}

// In a terminal that supports OSC 7577, a resume opens the screen that its
// --resume value names, and tmoney reports the command that opens that file
// on that screen again. When the user quits, tmoney clears the record, so the
// terminal does not bring it back.
func TestTUI_ResumeReportsItsScreenAndClearsOnQuit(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "resume.tdb")
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.MustParseDate("2020-01-01"))
	if err := account.NewService(account.NewRepository(database), database).Create(acct); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := filepath.Abs(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	bin := copyTestBinary(t)
	screen := "register/" + acct.ID.String()

	tui := startTUIOnPTY(t, dbPath, ptyOptions{
		bin:  bin,
		args: []string{"--resume", screen},
		env:  []string{"OSC7577_TOKEN=" + testResumeToken},
	})

	want := []string{bin, "-f", file, "--resume", screen}
	deadline := time.Now().Add(20 * time.Second)
	for {
		reports := resumeReports(t, tui.out.String())
		if n := len(reports); n > 0 && !reports[n-1].clear && strings.Join(reports[n-1].argv, "\x00") == strings.Join(want, "\x00") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no OSC 7577 report of %q within 20s; reports: %+v; stderr:\n%s", want, reports, tui.stderr)
		}
		time.Sleep(20 * time.Millisecond)
	}

	tui.send(t, "\x11") // ctrl+q: the user quits
	deadline = time.Now().Add(20 * time.Second)
	for {
		reports := resumeReports(t, tui.out.String())
		if n := len(reports); n > 0 && reports[n-1].clear {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no OSC 7577 clear within 20s of the quit; reports: %+v; stderr:\n%s", reports, tui.stderr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	tui.waitExit(t, 20*time.Second)
	if code := tui.cmd.ProcessState.ExitCode(); code != 0 {
		t.Errorf("tmoney exited with code %d after a quit, want 0; stderr:\n%s", code, tui.stderr)
	}
}
