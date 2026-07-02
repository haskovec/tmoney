package tui

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/applog"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// surfaceValueAdjustmentCollision emits a one-time notice — an applog
// line plus a status-bar toast — when a pre-existing *user* category
// named "Value Adjustment" prevented the system category from being
// seeded, so spending-report exclusion will not apply to it.
//
// Gated on config.ValueAdjustmentNoticeShown so it fires once across
// sessions rather than on every launch, mirroring how NewApp surfaces
// theme issues. The status-bar toast's auto-clear is scheduled by
// App.Init, which batches widget.ClearToastCmd whenever a startup toast
// is present.
func (a *App) surfaceValueAdjustmentCollision() {
	if a.cfg == nil || a.cfg.ValueAdjustmentNoticeShown {
		return
	}

	_ = applog.Append("category", fmt.Sprintf(
		"%q already exists as a user category; the system category was not created, so it will not be excluded from spending reports",
		category.ValueAdjustmentCategoryName))

	if a.statusbar != nil {
		text := fmt.Sprintf("Category %q already exists — it won't be excluded from spending reports", category.ValueAdjustmentCategoryName)
		if path, err := applog.LogPath(); err == nil {
			text = fmt.Sprintf("%s, see %s", text, path)
		}
		a.statusbar.SetToast(text, widget.NotificationAlert)
	}

	a.cfg.ValueAdjustmentNoticeShown = true
	// Best-effort persist so the notice does not re-fire next launch;
	// under `go test` Save is a no-op and a write failure is non-fatal.
	_ = a.cfg.Save()
}
