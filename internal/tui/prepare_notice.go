package tui

import (
	"github.com/haskovec/tmoney/internal/applog"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// surfacePrepareError shows that the open-time repairs (app.Services.Prepare)
// failed. The full error, which can name several accounts, goes to the app
// log; the status bar gets a sticky alert. It is neither a timed toast nor a
// queued notification, because the scheduled due-count refresh that follows
// every open clears the queue, and the failure must not vanish behind a view
// that looks normal. It lasts while this file is open; switchDatabase clears
// it before it repairs the next file. The TUI keeps running: writes do not
// depend on these repairs.
func (a *App) surfacePrepareError(err error) {
	_ = applog.Append("startup", "repair failed: "+err.Error())
	if a.statusbar == nil {
		return
	}
	text := "Startup repair failed"
	if path, lerr := applog.LogPath(); lerr == nil {
		text += ", see " + path
	}
	a.statusbar.SetSticky(text, widget.NotificationAlert)
}
