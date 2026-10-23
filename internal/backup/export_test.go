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

// SetHeldElsewhere replaces, for one test, the check of Restore whether
// another process has the database open.
func SetHeldElsewhere(t *testing.T, f func(path string) (bool, error)) {
	t.Helper()
	saved := heldElsewhere
	heldElsewhere = f
	t.Cleanup(func() { heldElsewhere = saved })
}
