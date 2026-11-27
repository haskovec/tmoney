package cli

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
)

// cliChild returns tmoney in a child process with args, with its own HOME and
// config folder, so that it does not touch the user's config.
func cliChild(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	home := t.TempDir()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = []string{
		runAsCLIEnv + "=1",
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"PATH=" + os.Getenv("PATH"),
	}
	return cmd
}

// closedDBFile returns the path of a new database that no process has open.
func closedDBFile(t *testing.T) string {
	t.Helper()
	database, path := dbtest.NewFile(t, "fin.tdb")
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// lines collects the lines that a child writes, while the test reads them.
type lines struct {
	mu   sync.Mutex
	text []string
}

func (l *lines) add(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.text = append(l.text, s)
}

// count returns how many lines are s.
func (l *lines) count(s string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, line := range l.text {
		if line == s {
			n++
		}
	}
	return n
}

func (l *lines) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.text, "\n")
}

// While another tmoney holds the gate of the file (it closes the file, or
// makes or restores a backup of it), a CLI command waits for the gate and
// says so once. When the gate is free, the command runs. The db commands
// that do not open the file through cmdutil.OpenServices say so too.
func TestACommandWaitsWhileAnotherTmoneyHoldsTheGate(t *testing.T) {
	commands := map[string]func(t *testing.T, path string) []string{
		"account list": func(*testing.T, string) []string { return []string{"account", "list"} },
		"db backup":    func(*testing.T, string) []string { return []string{"db", "backup"} },
		"db reindex":   func(*testing.T, string) []string { return []string{"db", "reindex"} },
		"db restore": func(t *testing.T, path string) []string {
			t.Helper()
			good, err := backup.CreateManualBackup(path)
			if err != nil {
				t.Fatal(err)
			}
			return []string{"db", "restore", good}
		},
	}
	for name, args := range commands {
		t.Run(name, func(t *testing.T) {
			path := closedDBFile(t)
			waitsForTheGate(t, path, args(t, path)...)
		})
	}
}

// waitsForTheGate runs tmoney -f path args in a child while this process
// holds the gate of path, and checks that the child waits and says so once,
// and that it runs when the gate is free.
func waitsForTheGate(t *testing.T, path string, args ...string) {
	t.Helper()
	release, err := db.HoldGate(path, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	cmd := cliChild(t, append([]string{"-f", path}, args...)...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	got := &lines{}
	read := make(chan struct{})
	go func() {
		defer close(read)
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			got.add(scanner.Text())
		}
	}()
	exited := make(chan error, 1)
	go func() {
		<-read // Wait closes the pipe: read it to the end first
		exited <- cmd.Wait()
	}()

	waitLine := "Waiting for another tmoney to finish with " + path + "..."
	deadline := time.Now().Add(20 * time.Second)
	for got.count(waitLine) == 0 {
		select {
		case err := <-exited:
			t.Fatalf("the command ended while the gate was held (%v); stderr:\n%s", err, got)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("no wait line in 20 s; stderr:\n%s", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(time.Second) // hold the gate a while longer
	select {
	case err := <-exited:
		t.Fatalf("the command ended while the gate was held (%v); stderr:\n%s", err, got)
	default:
	}

	release()
	select {
	case err := <-exited:
		if err != nil {
			t.Errorf("the command after the release: %v; stderr:\n%s", err, got)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("the command did not end in 20 s after the release; stderr:\n%s", got)
	}
	if n := got.count(waitLine); n != 1 {
		t.Errorf("the wait line shows %d times, want 1; stderr:\n%s", n, got)
	}
}

// A tmoney that only has the file open does not hold the gate. So a CLI
// command on that file fails at once with DuckDB's lock error, and db backup
// refuses the file and makes no backup.
func TestACommandOnAFileOpenInAnotherTmoneyFailsAtOnce(t *testing.T) {
	database, path := dbtest.NewFile(t, "fin.tdb")
	defer func() { _ = database.Close() }()

	start := time.Now()
	out, err := cliChild(t, "-f", path, "account", "list").CombinedOutput()
	took := time.Since(start)
	if err == nil || !db.IsLockedElsewhere(errors.New(string(out))) {
		t.Errorf("account list on a file open elsewhere: %v; output:\n%s\nwant DuckDB's lock error", err, out)
	}
	if took > time.Second {
		t.Errorf("account list on a file open elsewhere took %v, want less than 1 s", took)
	}

	out, err = cliChild(t, "-f", path, "db", "backup").CombinedOutput()
	want := "Error: failed to create backup: " + path + " is open in another program; close it, or use File > Create Backup in that tmoney\n"
	if err == nil || string(out) != want {
		t.Errorf("db backup of a file open elsewhere: %v; output %q, want %q", err, out, want)
	}
	backups, lerr := backup.ListBackups(path)
	if lerr != nil {
		t.Fatal(lerr)
	}
	if len(backups) != 0 {
		t.Errorf("the refused db backup made %d backups", len(backups))
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmoney-partial-") {
			t.Errorf("the refused db backup left %s", e.Name())
		}
	}
}
