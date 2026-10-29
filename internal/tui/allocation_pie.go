package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/haskovec/tmoney/internal/chart"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/termimg"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// The allocation pie in the TUI. See specs/design-asset-allocation-report.md
// §6.5.

// pieTerm is what the terminal said it can do for the pie.
type pieTerm struct {
	kittyOK bool // the terminal replied "OK" to the kitty query (C7)
	// cellW and cellH are the cell size in pixels, 0 until the terminal
	// gives it.
	cellW, cellH int
	profile      colorprofile.Profile
	inTmux       bool
}

// pieKey is one pie image: the section it shows, its rows and columns on the
// screen, its pixels, and its colors. A change to any of them needs a new
// image.
type pieKey struct {
	rpt              *report.Allocation
	section          int
	rows, cols, w, h int
	dark             bool
}

// pieState is the pie image in the terminal. A run has one image ID, and
// each transmit replaces the image with that ID (C5).
type pieState struct {
	pieTerm
	id   termimg.ID
	sent pieKey // the image the terminal has; zero when it has none
	// drawing is true while a draw command runs. One runs at a time, so a
	// resize does not start a draw for each size it goes through.
	drawing bool
	failed  pieKey // an image that could not be encoded; not tried again
}

// pieView is what the class table needs to show the pie.
type pieView struct {
	mode termimg.Mode
	term pieTerm
	id   termimg.ID
	sent pieKey
}

// allowed reports whether the setting, the terminal and the color profile
// let the pie show, before the data and the room (§6.3). The pie needs 256
// colors for its image ID (C6).
func (p pieView) allowed() bool {
	return p.term.profile >= colorprofile.ANSI256 &&
		termimg.Show(p.mode, p.term.kittyOK, p.term.inTmux, true)
}

// reportsLook is what the Reports view draws with from outside itself: the
// terminal's background, and the pie.
type reportsLook struct {
	dark bool
	pie  pieView
}

// reportsLook is the look of the terminal now.
func (a *App) reportsLook() reportsLook {
	return reportsLook{dark: !a.lightBackground, pie: pieView{
		mode: a.chartImagesMode(),
		term: a.pie.pieTerm,
		id:   a.pie.id,
		sent: a.pie.sent,
	}}
}

// chartImagesMode is the "View → Chart images" setting.
func (a *App) chartImagesMode() termimg.Mode {
	if a.cfg == nil {
		return termimg.ModeAuto
	}
	return termimg.ParseMode(a.cfg.ChartImages)
}

// queryPie asks the terminal for its cell size and whether it shows kitty
// images. With the setting off it asks nothing.
func (a *App) queryPie() tea.Cmd {
	if a.chartImagesMode() == termimg.ModeOff {
		return nil
	}
	return tea.Raw(termimg.Query)
}

// pieDrawnMsg carries a pie image that a draw command encoded.
type pieDrawnMsg struct {
	key pieKey
	seq string // the transmit
	err error
}

// drawPie draws and encodes a pie image. It runs as a command, not in
// Update: a large pie takes long enough to delay the frame after a resize.
func drawPie(key pieKey, id termimg.ID, slices []chart.Slice) tea.Cmd {
	return func() tea.Msg {
		img := chart.Pie(slices, key.w, key.h, 2)
		seq, err := termimg.Transmit(img, id, key.cols, key.rows)
		return pieDrawnMsg{key: key, seq: seq, err: err}
	}
}

// wantedPie is the pie image the screen needs: the pie of the allocation
// report's class table, while the Reports view shows that report. The
// drill-down keeps the image, so the pie comes back with no new transmit
// (C11).
func (a *App) wantedPie() (pieKey, bool) {
	if a.quitting || a.currentView != ViewReports {
		return pieKey{}, false
	}
	return a.reports.allocationPie(a.styles, a.height, a.reportsLook())
}

// syncPie keeps the image in the terminal in step with the screen. Update
// calls it after each message, with the message's command. It starts a draw
// when the screen needs an image the terminal does not have, and deletes the
// image when the screen needs none. The delete goes before cmd, so that it
// is written before a quit.
func (a *App) syncPie(cmd tea.Cmd) tea.Cmd {
	key, ok := a.wantedPie()
	switch {
	case !ok:
		if a.pie.sent == (pieKey{}) {
			return cmd
		}
		a.pie.sent = pieKey{}
		del := tea.Raw(termimg.Delete(a.pie.id))
		if cmd == nil {
			return del
		}
		return tea.Sequence(del, cmd)
	case key == a.pie.sent || key == a.pie.failed || a.pie.drawing:
		return cmd
	}
	sec, _ := a.reports.allocationSection()
	a.pie.drawing = true
	return tea.Batch(cmd, drawPie(key, a.pie.id, sec.PieSlices(key.dark)))
}

// pieDrawn sends a drawn image to the terminal, if the screen still needs
// it. Else syncPie starts the draw of the image it needs now.
func (a *App) pieDrawn(msg pieDrawnMsg) tea.Cmd {
	a.pie.drawing = false
	if key, ok := a.wantedPie(); !ok || key != msg.key {
		return nil
	}
	if msg.err != nil {
		a.pie.failed = msg.key
		a.statusbar.SetToast("Could not draw the pie: "+msg.err.Error(), widget.NotificationAlert)
		return widget.ClearToastCmd()
	}
	a.pie.sent = msg.key
	return tea.Raw(msg.seq)
}
