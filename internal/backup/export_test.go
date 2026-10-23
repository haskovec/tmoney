package backup

import "testing"

// SetBeforeSwap sets, for one test, the hook that Restore runs right before it
// renames its copy over the database.
func SetBeforeSwap(t *testing.T, f func(dbPath string)) {
	t.Helper()
	beforeSwap = f
	t.Cleanup(func() { beforeSwap = nil })
}
