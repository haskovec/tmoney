package db

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/filegate"
)

// gateChildEnv, when set in the environment of the test binary, makes
// TestMain run a child process instead of the tests. The child is another
// tmoney on the same file:
//
//   - "open:<path>" opens the database once, with no wait for the gate, and
//     prints "opened", "busy" (the gate) or "locked" (DuckDB's lock), and
//     how many milliseconds the open took;
//   - "hold:<path>" holds the gate of the database, prints "held", and holds
//     it until its stdin closes;
//   - "resume:<path>" retries the open as a resume does, for up to 10 s.
//     When it opens, it writes rows and a checkpoint at once, and prints
//     "opened" and the time of the open in Unix nanoseconds;
//   - "create:<path>" creates the database. It prints "waiting" when its
//     Create starts to wait for the gate, and then "created", or "exists"
//     for a FileExistsError.
const gateChildEnv = "DB_GATE_CHILD"

func TestMain(m *testing.M) {
	if mode, path, ok := strings.Cut(os.Getenv(gateChildEnv), ":"); ok {
		os.Exit(runGateChild(mode, path))
	}
	os.Exit(m.Run())
}

func runGateChild(mode, path string) int {
	switch mode {
	case "open":
		start := time.Now()
		database, err := OpenWaiting(path, 0, nil)
		took := time.Since(start).Milliseconds()
		switch {
		case err == nil:
			_ = database.Close()
			fmt.Println("opened", took)
		case errors.Is(err, ErrGateBusy):
			fmt.Println("busy", took)
		case IsLockedElsewhere(err):
			fmt.Println("locked", took)
		default:
			fmt.Println("error", err)
			return 1
		}
	case "hold":
		release, err := HoldGate(path, 0, nil)
		if err != nil {
			fmt.Println("error", err)
			return 1
		}
		fmt.Println("held")
		_, _ = io.Copy(io.Discard, os.Stdin) // until the parent closes stdin, or kills this process
		release()
	case "resume":
		return resumeChild(path)
	case "create":
		createWaiting = func() { fmt.Println("waiting") }
		database, err := Create(path)
		var exists *FileExistsError
		switch {
		case err == nil:
			_ = database.Close()
			fmt.Println("created")
		case errors.As(err, &exists):
			fmt.Println("exists")
		default:
			fmt.Println("error", err)
			return 1
		}
	default:
		fmt.Println("error: unknown mode", mode)
		return 1
	}
	return 0
}

// resumeChild is the "resume" child. It retries as the TUI's resume start
// does (openWaitingForLock in internal/cli), but with a shorter pause, so
// that it tries often while the other tmoney closes and copies the file.
func resumeChild(path string) int {
	deadline := time.Now().Add(10 * time.Second)
	for {
		database, err := OpenWaiting(path, GateWait, nil)
		if err == nil {
			opened := time.Now().UnixNano()
			// Write at once, and make DuckDB write to the file itself, so
			// that an open during the copy changes the copy.
			_ = database.WithTx(func(tx Queryer) error {
				for i := range 300 {
					if _, err := tx.Exec(`INSERT INTO _metadata (key, value) VALUES (?, 'child')`, "child-"+strconv.Itoa(i)); err != nil {
						return err
					}
				}
				return nil
			})
			_, _ = database.Conn().Exec(`CHECKPOINT`)
			_ = database.Close()
			fmt.Println("opened", opened)
			return 0
		}
		held, herr := HeldElsewhere(path)
		locked := IsLockedElsewhere(err) || (herr == nil && held)
		if !locked || !time.Now().Before(deadline) {
			fmt.Println("error", err)
			return 1
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func gateChildCmd(mode, path string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), gateChildEnv+"="+mode+":"+path)
	return cmd
}

// openElsewhere makes another process open path once, with no wait for the
// gate, and returns its answer ("opened", "busy" or "locked") and how long
// its open took.
func openElsewhere(t *testing.T, path string) (string, time.Duration) {
	t.Helper()
	out, err := gateChildCmd("open", path).Output()
	if err != nil {
		t.Fatalf("the child open failed: %v: %s", err, out)
	}
	word, ms, _ := strings.Cut(strings.TrimSpace(string(out)), " ")
	n, err := strconv.Atoi(ms)
	if err != nil {
		t.Fatalf("the child open printed %q", out)
	}
	return word, time.Duration(n) * time.Millisecond
}

// holdGateElsewhere makes another process hold the gate of path, and returns
// when it holds it. let makes the child release the gate and waits for its
// end.
func holdGateElsewhere(t *testing.T, path string) (let func()) {
	t.Helper()
	cmd := gateChildCmd("hold", path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	wait := func() { once.Do(func() { _ = cmd.Wait() }) }
	t.Cleanup(func() { _ = cmd.Process.Kill(); wait() })
	line, _ := bufio.NewReader(stdout).ReadString('\n')
	if strings.TrimSpace(line) != "held" {
		t.Fatalf("the child did not hold the gate: %q", line)
	}
	return func() { _ = stdin.Close(); wait() }
}

// newGateDB creates a database with rows probe rows ("probe-<i>" in
// _metadata), and closes it at the end of the test.
func newGateDB(t *testing.T, rows int) (*DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fin.tdb")
	database, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.WithTx(func(tx Queryer) error {
		for i := range rows {
			if _, err := tx.Exec(`INSERT INTO _metadata (key, value) VALUES (?, 'present')`, "probe-"+strconv.Itoa(i)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return database, path
}

// A start that finds the file open in a running tmoney fails at once: the
// gate is free while a tmoney only has the file open.
func TestAnOpenOfAFileOpenElsewhereFailsAtOnce(t *testing.T) {
	_, path := newGateDB(t, 1)
	if word, took := openElsewhere(t, path); word != "locked" || took > time.Second {
		t.Errorf("open in another process: %s after %v, want locked at once", word, took)
	}
}

func TestCloseAndSettleWithHoldsTheGateAcrossFn(t *testing.T) {
	database, path := newGateDB(t, 1)
	var during string
	ran, err := database.CloseAndSettleWith(5*time.Second, func(p string) error {
		during, _ = openElsewhere(t, p)
		return nil
	})
	if !ran || err != nil {
		t.Fatalf("CloseAndSettleWith = %v, %v; want fn run with no error", ran, err)
	}
	if during != "busy" {
		t.Errorf("open in another process during fn: %s, want busy", during)
	}
	if after, _ := openElsewhere(t, path); after != "opened" {
		t.Errorf("open in another process after CloseAndSettleWith: %s, want opened", after)
	}
}

func TestWithFileClosedHoldsTheGateAcrossFn(t *testing.T) {
	database, path := newGateDB(t, 1)
	var during string
	if err := database.WithFileClosed(func(p string) error {
		during, _ = openElsewhere(t, p)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if during != "busy" {
		t.Errorf("open in another process during fn: %s, want busy", during)
	}
	// Reopened: DuckDB's lock is back, and the gate is free.
	if after, took := openElsewhere(t, path); after != "locked" || took > time.Second {
		t.Errorf("open in another process after WithFileClosed: %s after %v, want locked at once", after, took)
	}
}

func TestReconnectHoldsTheGateBetweenTheCloseAndTheOpen(t *testing.T) {
	database, path := newGateDB(t, 1)
	var during []string
	reopened = func(p string) {
		word, _ := openElsewhere(t, p)
		during = append(during, word)
	}
	t.Cleanup(func() { reopened = nil })
	if err := database.reconnect(); err != nil {
		t.Fatal(err)
	}
	reopened = nil
	if len(during) != 1 || during[0] != "busy" {
		t.Errorf("open in another process between the close and the open: %v, want [busy]", during)
	}
	if after, _ := openElsewhere(t, path); after != "locked" {
		t.Errorf("open in another process after the reconnect: %s, want locked", after)
	}
}

// When WithFileClosed gives up on a held read, the reopen that it leaves in
// the background keeps the gate until it ends.
func TestABackgroundReopenHoldsTheGateUntilTheReadEnds(t *testing.T) {
	shortReopenWait(t)
	database, rows := walOnlyRow(t)
	ran := false
	err := withFileClosedBy(t, database, rows, func(string) error { ran = true; return nil })
	if !errors.Is(err, errFileBusy) || ran {
		t.Fatalf("WithFileClosed with a held read = %v, fn ran %v; want errFileBusy and no fn", err, ran)
	}
	if word, _ := openElsewhere(t, database.Path()); word != "busy" {
		t.Errorf("open in another process while the read holds: %s, want busy", word)
	}
	_ = rows.Close()
	waitForReopen(t, database)
	if word, _ := openElsewhere(t, database.Path()); word != "locked" {
		t.Errorf("open in another process after the background reopen: %s, want locked", word)
	}
}

func TestOpenWaitingGivesUpWithABusyError(t *testing.T) {
	database, path := newGateDB(t, 1)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	holdGateElsewhere(t, path)
	waits := 0
	_, err := OpenWaiting(path, 50*time.Millisecond, func() { waits++ })
	var busy *filegate.BusyError
	if !errors.As(err, &busy) || !errors.Is(err, ErrGateBusy) {
		t.Fatalf("OpenWaiting = %v, want a *filegate.BusyError", err)
	}
	const want = "fin.tdb is busy: another tmoney is closing it, or is making or restoring a backup of it. Try again in a moment"
	if err.Error() != want {
		t.Errorf("error text = %q, want %q", err, want)
	}
	if waits != 1 {
		t.Errorf("waiting was called %d times, want 1", waits)
	}
}

// While another tmoney holds the gate, the file operations do not close the
// file. The handle stays as it was, except that CloseAndSettleWith closes it
// for good all the same.
func TestTheFileOperationsGiveUpOnAGateHeldElsewhere(t *testing.T) {
	t.Run("CloseAndSettleWith", func(t *testing.T) {
		database, path := newGateDB(t, 1)
		holdGateElsewhere(t, path)
		ran, err := database.CloseAndSettleWith(300*time.Millisecond, func(string) error {
			t.Error("fn ran while another process held the gate")
			return nil
		})
		if ran || err != nil {
			t.Errorf("CloseAndSettleWith = %v, %v; want false and no error", ran, err)
		}
		if !database.closedForGood() {
			t.Error("the database is not closed for good")
		}
	})
	t.Run("WithFileClosed", func(t *testing.T) {
		shortReopenWait(t)
		database, path := newGateDB(t, 1)
		holdGateElsewhere(t, path)
		err := database.WithFileClosed(func(string) error {
			t.Error("fn ran while another process held the gate")
			return nil
		})
		var dbErr *DatabaseError
		if !errors.Is(err, ErrGateBusy) || !errors.As(err, &dbErr) || dbErr.Op != "close for file operation" {
			t.Errorf("WithFileClosed = %v, want a DatabaseError that wraps ErrGateBusy", err)
		}
		insertProbe(t, database, "after") // nothing changed: the handle still works
	})
	t.Run("reconnect", func(t *testing.T) {
		shortReopenWait(t)
		database, path := newGateDB(t, 1)
		holdGateElsewhere(t, path)
		err := database.reconnect()
		var dbErr *DatabaseError
		if !errors.Is(err, ErrGateBusy) || !errors.As(err, &dbErr) || dbErr.Op != "reconnect" {
			t.Errorf("reconnect = %v, want a DatabaseError that wraps ErrGateBusy", err)
		}
		insertProbe(t, database, "after") // the old pool stays open
	})
}

func TestHoldGateOfAMissingFileMakesNoGateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.tdb")
	release, err := HoldGate(path, 0, nil)
	if err != nil || release == nil {
		t.Fatalf("HoldGate = %v, want a release that does nothing", err)
	}
	release()
	if _, err := os.Stat(filegate.Path(path)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("HoldGate made a gate file for a missing database: %v", err)
	}
}

// A Create that waited for the gate while another tmoney made the same file
// finds that file under the gate. It says that the file exists, and keeps
// the file: it does not open it, fail on its tables, and delete it.
func TestACreateThatWaitedForTheGateKeepsTheFileMadeMeanwhile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fin.tdb")
	release, err := holdGate(path, time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	cmd := gateChildCmd("create", path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	out := bufio.NewReader(stdout)
	if line, _ := out.ReadString('\n'); strings.TrimSpace(line) != "waiting" {
		t.Fatalf("the child Create did not wait for the gate: %q", line)
	}

	// This tmoney makes the file while the child waits: its own hold of the
	// gate counts up and does not wait. Then it closes the file, as
	// `tmoney db create` does, and lets the child in.
	database, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	release()

	if line, _ := out.ReadString('\n'); strings.TrimSpace(line) != "exists" {
		t.Errorf("the child Create after the wait = %q, want exists", line)
	}
	if err := ValidateFile(path); err != nil {
		t.Errorf("the file that this tmoney made is gone or broken: %v", err)
	}
}

// An open that waits for the gate while the file goes reports the missing
// file. It does not make a new, empty database at the path.
func TestAnOpenThatWaitedForTheGateOfAFileThatWentMakesNoFile(t *testing.T) {
	database, path := newGateDB(t, 1)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	let := holdGateElsewhere(t, path)
	waiting := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		opened, err := OpenWaiting(path, 20*time.Second, func() { close(waiting) })
		if err == nil {
			_ = opened.Close()
		}
		result <- err
	}()
	select {
	case <-waiting:
	case err := <-result:
		t.Fatalf("OpenWaiting did not wait for the gate: %v", err)
	case <-time.After(20 * time.Second):
		t.Fatal("OpenWaiting did not wait for the gate in 20 s")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	let()

	var notFound *FileNotFoundError
	if err := <-result; !errors.As(err, &notFound) {
		t.Errorf("OpenWaiting of a file that went during the wait = %v, want a FileNotFoundError", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the open made a file at the path of the file that went: %v", err)
	}
}

// The gate file stays beside the file for good, so an open of a path that
// cannot be a database makes none. The open fails as before.
func TestAnOpenOfAPathThatIsNotADatabaseMakesNoGateFile(t *testing.T) {
	dir := t.TempDir()
	csv := filepath.Join(dir, "statement.csv")
	if err := os.WriteFile(csv, []byte(strings.Repeat("date,payee,amount\n2026-01-02,Grocer,-12.00\n", 200)), 0o644); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(dir, "sub")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{csv, folder} {
		if database, err := Open(path); err == nil {
			_ = database.Close()
			t.Errorf("Open(%s) worked, want an error", filepath.Base(path))
		}
		if _, err := os.Stat(filegate.Path(path)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Open(%s) made a gate file: %v", filepath.Base(path), err)
		}
	}
}

func TestCanBeDatabase(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	database, tdb := newGateDB(t, 1)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		path string
		want bool
	}{
		"a TMoney file":                   {tdb, true},
		"an empty file (DuckDB makes it)": {write("empty.tdb", nil), true},
		"a file too short to tell":        {write("short.tdb", []byte("12345")), true},
		"a CSV file":                      {write("statement.csv", []byte("date,amount\n2026-01-02,-12.00\n")), false},
		"a folder":                        {dir, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			info, err := os.Stat(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if got := canBeDatabase(tc.path, info); got != tc.want {
				t.Errorf("canBeDatabase = %v, want %v", got, tc.want)
			}
		})
	}
}

// slowCopy copies src to dst in parts, with a pause after each part.
func slowCopy(src, dst string, parts int, pause time.Duration) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	part := info.Size()/int64(parts) + 1
	for {
		n, err := io.CopyN(out, in, part)
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 || err == io.EOF {
			break
		}
		time.Sleep(pause)
	}
	return out.Sync()
}

// startResumeChild starts a "resume" child on path and gives it time to
// retry on DuckDB's lock. wait waits for its end and returns its output.
func startResumeChild(t *testing.T, path string) (wait func() string) {
	t.Helper()
	child := gateChildCmd("resume", path)
	var out strings.Builder
	child.Stdout = &out
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	t.Cleanup(func() { _ = child.Process.Kill(); once.Do(func() { _ = child.Wait() }) })
	time.Sleep(200 * time.Millisecond) // the child retries on DuckDB's lock now
	return func() string {
		once.Do(func() { _ = child.Wait() })
		return strings.TrimSpace(out.String())
	}
}

// countCopyRows returns the probe rows of the parent and the rows of the
// child in the copy at copyPath.
func countCopyRows(t *testing.T, copyPath string) (probes, child int, err error) {
	t.Helper()
	copied, err := Open(copyPath)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = copied.Close() }()
	err = copied.Conn().QueryRow(`SELECT
		count(*) FILTER (WHERE key LIKE 'probe-%'),
		count(*) FILTER (WHERE key LIKE 'child-%')
		FROM _metadata`).Scan(&probes, &child)
	return probes, child, err
}

// The hand-off at a program resume: a new tmoney retries the open while the
// old one closes the file and makes its last copy. The new one must open the
// file only after the copy, and the copy must hold exactly the rows of the
// old one.
func TestTheLastCopyKeepsAResumedTmoneyOut(t *testing.T) {
	const rows = 200
	database, path := newGateDB(t, rows)
	wait := startResumeChild(t, path)

	copyPath := filepath.Join(t.TempDir(), "copy.tdb")
	var copyEnd int64
	ran, err := database.CloseAndSettleWith(5*time.Second, func(p string) error {
		err := slowCopy(p, copyPath, 4, 200*time.Millisecond)
		copyEnd = time.Now().UnixNano()
		return err
	})
	if !ran || err != nil {
		t.Fatalf("CloseAndSettleWith = %v, %v; want the copy made", ran, err)
	}

	out := wait()
	word, ts, _ := strings.Cut(out, " ")
	opened, _ := strconv.ParseInt(ts, 10, 64)
	if word != "opened" {
		t.Fatalf("the resumed tmoney did not open the file: %q", out)
	}
	if opened < copyEnd {
		t.Errorf("the resumed tmoney opened the file %v before the end of the copy", time.Duration(copyEnd-opened))
	}
	if err := ValidateFile(copyPath); err != nil {
		t.Fatalf("the copy is not a good database: %v", err)
	}
	probes, child, err := countCopyRows(t, copyPath)
	if err != nil {
		t.Fatal(err)
	}
	if probes != rows || child != 0 {
		t.Errorf("the copy has %d rows of the old tmoney and %d of the new one, want %d and 0", probes, child, rows)
	}
}

// The same hand-off without the gate: CloseAndSettle, then the copy. It shows
// the window that the gate closes. It is not a test of the app: it only
// logs. Run it with DB_GATE_CONTROL=1.
func TestControlTheHandOffWithoutTheGate(t *testing.T) {
	if os.Getenv("DB_GATE_CONTROL") != "1" {
		t.Skip("a control run: set DB_GATE_CONTROL=1")
	}
	const rows = 200
	database, path := newGateDB(t, rows)
	wait := startResumeChild(t, path)
	if settled, err := database.CloseAndSettle(5 * time.Second); !settled || err != nil {
		t.Fatalf("CloseAndSettle = %v, %v", settled, err)
	}
	copyPath := filepath.Join(t.TempDir(), "copy.tdb")
	copyErr := slowCopy(path, copyPath, 4, 200*time.Millisecond)
	copyEnd := time.Now().UnixNano()

	out := wait()
	word, ts, _ := strings.Cut(out, " ")
	opened, _ := strconv.ParseInt(ts, 10, 64)
	t.Logf("copy error: %v; the child said %q, %v before the end of the copy", copyErr, word, time.Duration(copyEnd-opened))
	if err := ValidateFile(copyPath); err != nil {
		t.Logf("the copy is not a good database: %v", err)
		return
	}
	probes, child, err := countCopyRows(t, copyPath)
	t.Logf("the copy has %d rows of the old tmoney and %d of the new one (error %v)", probes, child, err)
}
