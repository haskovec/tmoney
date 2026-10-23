//go:build !windows

package backup_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/backup"
)

// The kill tests stop a real backup or restore halfway with SIGKILL, the way a
// crash, a power loss or a killed terminal does. The source is a FIFO: the
// parent writes part of the "database" into it and keeps it open, so the child's
// copy blocks halfway with that part already in its output file. Then the
// parent kills the child and looks at what is left.

const (
	killChildPathEnv = "TMONEY_KILL_CHILD_DB"
	killChildSrcEnv  = "TMONEY_KILL_CHILD_SRC"
	killChildKindEnv = "TMONEY_KILL_CHILD_KIND"
)

// TestKillChild is the child half of the kill tests. It is skipped in a normal
// run. It never returns on its own: the parent kills it in the copy.
func TestKillChild(t *testing.T) {
	dbPath := os.Getenv(killChildPathEnv)
	if dbPath == "" {
		t.Skip("helper process for the kill tests")
	}
	var err error
	switch os.Getenv(killChildKindEnv) {
	case "manual":
		_, err = backup.CreateManualBackup(dbPath)
	case "auto":
		_, err = backup.CreateAutoBackup(dbPath)
	case "restore":
		_, err = backup.Restore(dbPath, os.Getenv(killChildSrcEnv))
	}
	t.Fatalf("the copy returned (err=%v); the parent should have killed this process first", err)
}

// fictionalLedger is the part of the source that reaches the child before the
// kill. 8 KiB fits in a pipe buffer, so the parent's write does not block.
var fictionalLedger = bytes.Repeat([]byte("2026-01-01;Fictional Payee;-12.34\n"), 241)[:8192]

// killHalfway starts the child on kind, feeds it fictionalLedger through the
// FIFO at fifo, waits until some file in dir other than skip holds all of it,
// and kills the child.
func killHalfway(t *testing.T, kind, dbPath, srcPath, fifo, dir string, skip map[string]bool) {
	t.Helper()
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestKillChild$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		killChildPathEnv+"="+dbPath, killChildSrcEnv+"="+srcPath, killChildKindEnv+"="+kind)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// O_NONBLOCK: the open fails with ENXIO until the child opens the FIFO to
	// read, instead of blocking forever if the child dies first.
	deadline := time.Now().Add(30 * time.Second)
	var w *os.File
	for {
		f, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			w = f
			break
		}
		if !errors.Is(err, syscall.ENXIO) || time.Now().After(deadline) {
			t.Fatalf("open FIFO for writing: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer func() { _ = w.Close() }()
	if _, err := w.Write(fictionalLedger); err != nil {
		t.Fatalf("write to FIFO: %v", err)
	}

	// Wait until the child's output file holds every byte it was given, so the
	// kill lands in the middle of the copy, not before it.
	for {
		if name, ok := fileOfSize(dir, int64(len(fictionalLedger)), skip); ok {
			t.Logf("killing the child while %s holds %d bytes", name, len(fictionalLedger))
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the child never wrote %d bytes; dir has %v", len(fictionalLedger), dirListing(t, dir))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatalf("kill child: %v", err)
	}
	_ = cmd.Wait()
}

func fileOfSize(dir string, size int64, skip map[string]bool) (string, bool) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if skip[e.Name()] || !e.Type().IsRegular() {
			continue
		}
		if info, err := e.Info(); err == nil && info.Size() == size {
			return e.Name(), true
		}
	}
	return "", false
}

func dirListing(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		info, _ := e.Info()
		out = append(out, filepath.Base(e.Name())+" ("+info.Mode().Type().String()+", "+strconv.FormatInt(info.Size(), 10)+" B)")
	}
	sort.Strings(out)
	return out
}

// TestKilledBackupLeavesNoTruncatedBackup: a backup killed halfway must not
// leave a file under a backup name, because ListBackups shows it and Restore
// takes it as a complete copy of the database.
func TestKilledBackupLeavesNoTruncatedBackup(t *testing.T) {
	for _, kind := range []string{"manual", "auto"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "fin.tdb")
			killHalfway(t, kind, dbPath, "", dbPath, dir, map[string]bool{"fin.tdb": true})

			t.Logf("left in the folder: %v", dirListing(t, dir))
			backups, err := backup.ListBackups(dbPath)
			if err != nil {
				t.Fatalf("ListBackups: %v", err)
			}
			for _, b := range backups {
				t.Errorf("a killed copy left a %d-byte file under the final backup name %s, and ListBackups offers it for restore",
					b.Size, filepath.Base(b.Path))
			}
		})
	}
}

// TestKilledRestoreLeavesDatabaseIntact: a restore killed while it copies the
// backup must leave the database as it was.
func TestKilledRestoreLeavesDatabaseIntact(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fin.tdb")
	original := bytes.Repeat([]byte("current fictional database\n"), 1000)
	if err := os.WriteFile(dbPath, original, 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "source-backup.fifo")
	killHalfway(t, "restore", dbPath, src, src, dir, map[string]bool{"fin.tdb": true, "source-backup.fifo": true})

	t.Logf("left in the folder: %v", dirListing(t, dir))
	got, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("the database is gone after a killed restore: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("the database changed after a killed restore: %d bytes, want %d", len(got), len(original))
	}
	backups, _ := backup.ListBackups(dbPath)
	for _, b := range backups {
		t.Errorf("a killed restore left %s (%d B) under a backup name", filepath.Base(b.Path), b.Size)
	}
}
