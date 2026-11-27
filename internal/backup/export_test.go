package backup

import "testing"

// SetBeforeSwap sets, for one test, the hook that Restore runs right before it
// renames its copy over the database.
func SetBeforeSwap(t *testing.T, f func(dbPath string)) {
	t.Helper()
	beforeSwap = f
	t.Cleanup(func() { beforeSwap = nil })
}

// SetAfterSafetyBackup sets, for one test, the hook that Restore runs right
// after the safety backup, with the database file.
func SetAfterSafetyBackup(t *testing.T, f func(file string)) {
	t.Helper()
	afterSafetyBackup = f
	t.Cleanup(func() { afterSafetyBackup = nil })
}

// SetAfterCopy sets, for one test, the hook that a backup runs right after
// its copy of the database, before its checks, with the database file.
func SetAfterCopy(t *testing.T, f func(file string)) {
	t.Helper()
	afterCopy = f
	t.Cleanup(func() { afterCopy = nil })
}

// SetHeldElsewhere replaces, for one test, the check of a backup or Restore
// whether another process has the database open.
func SetHeldElsewhere(t *testing.T, f func(path string) (bool, error)) {
	t.Helper()
	saved := heldElsewhere
	heldElsewhere = f
	t.Cleanup(func() { heldElsewhere = saved })
}
