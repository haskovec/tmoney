package tui

import (
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/db"
)

// Opening a file whose startup repair fails shows an alert, and the app still
// opens: writes do not depend on the repair.
func TestNewApp_ShowsStartupRepairFailure(t *testing.T) {
	// The alert logs the error to the app log; keep it out of the real one.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	database, err := db.Open(clitest.DamagedHealFile(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	a := NewApp(database, nil)

	found := false
	for _, n := range a.statusbar.Notifications() {
		if strings.HasPrefix(n.Text, "Startup repair failed") {
			found = true
		}
	}
	if !found {
		t.Errorf("notifications = %v, want a startup repair alert", a.statusbar.Notifications())
	}
	if a.services.Account == nil {
		t.Error("the app did not get its services")
	}
}
