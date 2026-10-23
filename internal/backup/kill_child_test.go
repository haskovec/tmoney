//go:build !windows

package backup

import (
	"os"
	"testing"
)

// The environment of the child process of the kill tests (kill_test.go).
// Exported for the external test package.
const (
	KillChildPathEnv = "TMONEY_KILL_CHILD_DB"
	KillChildSrcEnv  = "TMONEY_KILL_CHILD_SRC"
	KillChildKindEnv = "TMONEY_KILL_CHILD_KIND"
)

// TestKillChild is the child half of the kill tests. It is skipped in a normal
// run. It never returns on its own: the parent kills it in the copy.
func TestKillChild(t *testing.T) {
	dbPath := os.Getenv(KillChildPathEnv)
	if dbPath == "" {
		t.Skip("helper process for the kill tests")
	}
	var err error
	switch os.Getenv(KillChildKindEnv) {
	case "manual":
		_, err = CreateManualBackup(dbPath)
	case "auto":
		_, err = CreateAutoBackup(dbPath)
	case "restore":
		// The source is a FIFO that only the copy may read, so the check of
		// the backup before the copy is skipped; restore_validate_test.go
		// covers the checks.
		validateDatabase = func(string) error { return nil }
		_, err = Restore(dbPath, os.Getenv(KillChildSrcEnv))
	}
	t.Fatalf("the copy returned (err=%v); the parent should have killed this process first", err)
}
