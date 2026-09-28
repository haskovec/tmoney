package tui

import (
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

func openDamaged(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(clitest.DamagedHealFile(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func statusText(a *App) string {
	return widget.StripAnsi(a.statusbar.Render(widget.NewStyles(), 200))
}

// Opening a file whose startup repair fails shows an alert that lasts: the
// due-count refresh that follows every open clears the queued
// notifications, and the alert must survive it. The app still opens, because
// writes do not depend on the repair.
func TestNewApp_StartupRepairAlertSurvivesRefresh(t *testing.T) {
	// The alert logs the error to the app log; keep it out of the real one.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	a := NewApp(openDamaged(t), nil)
	if a.services.Account == nil {
		t.Fatal("the app did not get its services")
	}
	if !strings.Contains(statusText(a), "Startup repair failed") {
		t.Fatalf("status bar = %q, want the startup repair alert", statusText(a))
	}

	for _, count := range []int{0, 2} {
		a.Update(scheduledDueCountMsg{count: count})
		got := statusText(a)
		if !strings.Contains(got, "Startup repair failed") {
			t.Errorf("after a due count of %d the alert is gone: %q", count, got)
		}
		if count > 0 && !strings.Contains(got, "2 scheduled due") {
			t.Errorf("the due-count alert is missing beside the repair alert: %q", got)
		}
	}
}

// The alert belongs to the file it came from: switching to a damaged file
// shows it after the refresh, and switching to a healthy file clears it.
func TestSwitchDatabase_StartupRepairAlert(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewApp(dbtest.New(t), nil)
	if strings.Contains(statusText(a), "Startup repair failed") {
		t.Fatal("a healthy file shows the repair alert")
	}

	a.switchDatabase(openDamaged(t))
	a.Update(scheduledDueCountMsg{count: 0})
	if !strings.Contains(statusText(a), "Startup repair failed") {
		t.Errorf("after switching to a damaged file, status bar = %q, want the repair alert", statusText(a))
	}

	a.switchDatabase(dbtest.New(t))
	if strings.Contains(statusText(a), "Startup repair failed") {
		t.Errorf("after switching to a healthy file the old alert remains: %q", statusText(a))
	}
}
