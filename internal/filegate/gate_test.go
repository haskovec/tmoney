package filegate

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// childEnv, when set in the environment of the test binary, makes TestMain
// run a child process instead of the tests:
//
//   - "try:<file>" tries the gate of file once and prints "ok" or "busy";
//   - "hold:<file>" holds the gate of file, prints "held", and holds it until
//     its stdin closes.
const childEnv = "FILEGATE_TEST_CHILD"

func TestMain(m *testing.M) {
	if mode, file, ok := strings.Cut(os.Getenv(childEnv), ":"); ok {
		os.Exit(runChild(mode, file))
	}
	os.Exit(m.Run())
}

func runChild(mode, file string) int {
	switch mode {
	case "try":
		release, err := Hold(file, time.Now(), nil)
		switch {
		case errors.Is(err, ErrBusy):
			fmt.Println("busy")
		case err != nil:
			fmt.Println("error", err)
			return 1
		default:
			release()
			fmt.Println("ok")
		}
	case "hold":
		release, err := Hold(file, time.Now(), nil)
		if err != nil {
			fmt.Println("error", err)
			return 1
		}
		fmt.Println("held")
		_, _ = io.Copy(io.Discard, os.Stdin) // until the parent closes stdin, or kills this process
		release()
	default:
		fmt.Println("error: unknown mode", mode)
		return 1
	}
	return 0
}

func childCmd(mode, file string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), childEnv+"="+mode+":"+file)
	return cmd
}

// tryElsewhere makes another process try the gate of file once, and returns
// its answer: "ok" or "busy".
func tryElsewhere(t *testing.T, file string) string {
	t.Helper()
	out, err := childCmd("try", file).Output()
	if err != nil {
		t.Fatalf("the child that tries the gate failed: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// holdElsewhere makes another process hold the gate of file. It returns when
// the child holds it. let makes the child release the gate and waits for its
// end; kill kills the child.
func holdElsewhere(t *testing.T, file string) (let, kill func()) {
	t.Helper()
	cmd := childCmd("hold", file)
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
	return func() { _ = stdin.Close(); wait() },
		func() { _ = cmd.Process.Kill(); wait() }
}

// dbFile returns the path of a database file in a new folder. The gate never
// reads the file, so its content does not matter.
func dbFile(t *testing.T) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "fin.tdb")
	if err := os.WriteFile(file, []byte("not a real database"), 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

func hold(t *testing.T, file string) func() {
	t.Helper()
	release, err := Hold(file, time.Now(), nil)
	if err != nil {
		t.Fatalf("Hold(%s) = %v, want the gate", file, err)
	}
	return release
}

func TestPathIsAHiddenFileBesideTheDatabase(t *testing.T) {
	got := Path(filepath.Join("a", "b", "fin.tdb"))
	if want := filepath.Join("a", "b", ".fin.tdb.tmoney-lock"); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func TestAnotherProcessIsKeptOutWhileTheGateIsHeld(t *testing.T) {
	file := dbFile(t)
	release := hold(t, file)
	if got := tryElsewhere(t, file); got != "busy" {
		t.Errorf("another process while this one holds the gate: %s, want busy", got)
	}
	release()
	if got := tryElsewhere(t, file); got != "ok" {
		t.Errorf("another process after the release: %s, want ok", got)
	}
	if _, err := os.Stat(Path(file)); err != nil {
		t.Errorf("the gate file is gone after the release: %v", err)
	}
}

// caseInsensitive reports whether the file system of dir ignores the case of
// names.
func caseInsensitive(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "caseprobe")
	if err := os.WriteFile(probe, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(probe) }()
	_, err := os.Stat(filepath.Join(dir, "CASEPROBE"))
	return err == nil
}

// A second hold in this process returns at once, also through another path to
// the same gate file: a second lock by this process on another descriptor
// would conflict with its first lock. The gate stays held until the last
// release, and a second call of one release does not count down again.
func TestASecondHoldInThisProcessCountsUp(t *testing.T) {
	for _, tc := range []struct {
		name  string
		other func(t *testing.T, file string) string
	}{
		{"same path", func(t *testing.T, file string) string { return file }},
		{"through a folder link", func(t *testing.T, file string) string {
			link := filepath.Join(t.TempDir(), "linked")
			if err := os.Symlink(filepath.Dir(file), link); err != nil {
				t.Skip("no symbolic links here:", err)
			}
			return filepath.Join(link, filepath.Base(file))
		}},
		{"through an upper-case name", func(t *testing.T, file string) string {
			if !caseInsensitive(t, filepath.Dir(file)) {
				t.Skip("this file system does not ignore case: FIN.tdb is another file")
			}
			return filepath.Join(filepath.Dir(file), "FIN.tdb")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := dbFile(t)
			other := tc.other(t, file)
			first := hold(t, file)
			start := time.Now()
			second, err := Hold(other, time.Now().Add(5*time.Second), func() { t.Error("the second hold waited") })
			if took := time.Since(start); err != nil || took > time.Second {
				t.Fatalf("second hold in this process through %s: %v after %v, want the gate at once", other, err, took)
			}
			first()
			first() // a second call of one release does nothing
			if got := tryElsewhere(t, file); got != "busy" {
				t.Errorf("another process after one of two releases: %s, want busy", got)
			}
			second()
			if got := tryElsewhere(t, file); got != "ok" {
				t.Errorf("another process after the last release: %s, want ok", got)
			}
		})
	}
}

func TestHoldWaitsForAnotherProcessAndSaysSoOnce(t *testing.T) {
	file := dbFile(t)
	let, _ := holdElsewhere(t, file)
	go func() { time.Sleep(300 * time.Millisecond); let() }()
	waits := 0
	start := time.Now()
	release, err := Hold(file, time.Now().Add(5*time.Second), func() { waits++ })
	took := time.Since(start)
	if err != nil {
		t.Fatalf("Hold = %v, want the gate when the other process lets go", err)
	}
	release()
	if took < 250*time.Millisecond || waits != 1 {
		t.Errorf("Hold returned after %v and called waiting %d times, want about 300ms and 1 call", took, waits)
	}
}

func TestHoldGivesUpAtTheDeadlineWithABusyError(t *testing.T) {
	file := dbFile(t)
	holdElsewhere(t, file)
	waits := 0
	release, err := Hold(file, time.Now().Add(100*time.Millisecond), func() { waits++ })
	var busy *BusyError
	if !errors.As(err, &busy) || !errors.Is(err, ErrBusy) || release != nil {
		t.Fatalf("Hold = %v, %v; want a *BusyError that wraps ErrBusy, and no release", release != nil, err)
	}
	const want = "fin.tdb is busy: another tmoney is closing it, or is making or restoring a backup of it. Try again in a moment"
	if err.Error() != want {
		t.Errorf("error text = %q, want %q", err, want)
	}
	if waits != 1 {
		t.Errorf("waiting was called %d times, want 1", waits)
	}

	// A deadline that has passed gives one try, and no wait.
	start := time.Now()
	_, err = Hold(file, time.Now(), func() { t.Error("waiting was called for a deadline that has passed") })
	if !errors.Is(err, ErrBusy) || time.Since(start) > time.Second {
		t.Errorf("Hold with a deadline that has passed = %v after %v, want ErrBusy at once", err, time.Since(start))
	}
}

func TestTheGateOfAKilledHolderIsFree(t *testing.T) {
	file := dbFile(t)
	_, kill := holdElsewhere(t, file)
	kill()
	release, err := Hold(file, time.Now().Add(time.Second), nil)
	if err != nil {
		t.Fatalf("Hold after the holder was killed = %v, want the gate", err)
	}
	release()
}

// The close of other descriptors of the gate file or of the database file in
// this process does not drop the gate: a backup opens and closes the database
// file during its copy, DuckDB opens and closes it, and each hold that counts
// up and each try opens and closes a descriptor of the gate file.
func TestTheGateSurvivesTheCloseOfOtherDescriptors(t *testing.T) {
	file := dbFile(t)
	release := hold(t, file)
	defer release()
	for _, p := range []string{Path(file), file} {
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, f)
		_ = f.Close()
	}
	hold(t, file)() // a hold that counts up, and its release
	if got := tryElsewhere(t, file); got != "busy" {
		t.Errorf("another process after the close of other descriptors: %s, want busy", got)
	}
}

// Many holds and releases in this process at the same time keep the count
// right: after the last release, the gate is free. Run it with -race.
func TestConcurrentHoldsInThisProcess(t *testing.T) {
	file := dbFile(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				release, err := Hold(file, time.Now().Add(5*time.Second), nil)
				if err != nil {
					t.Errorf("Hold in this process = %v, want the gate", err)
					return
				}
				release()
				release()
			}
		})
	}
	wg.Wait()
	mu.Lock()
	n := len(gates)
	mu.Unlock()
	if n != 0 {
		t.Errorf("%d gates stay held after the last release, want 0", n)
	}
	if got := tryElsewhere(t, file); got != "ok" {
		t.Errorf("another process after the last release: %s, want ok", got)
	}
}

// A folder that this user cannot write, with no gate file in it: the gate
// cannot work, so Hold does not keep the user from the database.
func TestHoldDegradesInAFolderThatCannotBeWritten(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs folder permissions that apply to this user")
	}
	file := dbFile(t)
	dir := filepath.Dir(file)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	release, err := Hold(file, time.Now(), nil)
	if err != nil || release == nil {
		t.Fatalf("Hold = %v, want a release that does nothing and no error", err)
	}
	if got := tryElsewhere(t, file); got != "ok" {
		t.Errorf("another process while the gate cannot work: %s, want ok", got)
	}
	release()
	release()
	if _, err := os.Stat(Path(file)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a gate file is in a folder that cannot be written: %v", err)
	}
}

// A gate file that is there, but that this user cannot write, still locks:
// flock and LockFileEx need no write access.
func TestAGateFileThatCannotBeWrittenStillLocks(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs file permissions that apply to this user")
	}
	file := dbFile(t)
	if err := os.WriteFile(Path(file), nil, 0o444); err != nil {
		t.Fatal(err)
	}
	release := hold(t, file)
	if got := tryElsewhere(t, file); got != "busy" {
		t.Errorf("another process while this one holds a read-only gate file: %s, want busy", got)
	}
	release()
	if got := tryElsewhere(t, file); got != "ok" {
		t.Errorf("another process after the release: %s, want ok", got)
	}
}
