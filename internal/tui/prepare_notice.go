package tui

import (
	"github.com/haskovec/tmoney/internal/applog"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// surfacePrepareError shows that the open-time repairs (app.Services.Prepare)
// failed. The full error, which can name several accounts, goes to the app
// log; the status bar gets an alert notification, not a timed toast, so the
// failure is not hidden behind a view that looks normal. The TUI keeps
// running: writes do not depend on these repairs.
func (a *App) surfacePrepareError(err error) {
	_ = applog.Append("startup", "repair failed: "+err.Error())
	if a.statusbar == nil {
		return
	}
	text := "Startup repair failed"
	if path, lerr := applog.LogPath(); lerr == nil {
		text += ", see " + path
	}
	a.statusbar.AddNotification(text, widget.NotificationAlert)
}
