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
	id termimg.ID
	// sent is the image the terminal has, by the last write that came back
	// written (pieWrittenMsg); zero when it has none.
	sent pieKey
	// writing is true from a write to the terminal, a transmit or a delete,
	// until its pieWrittenMsg. Bubble Tea runs each command in its own
	// goroutine, so two writes from two commands can pass each other: the
	// writes go one at a time, and a delete cannot pass the transmit before
	// it.
	writing bool
	// held is the command of a quit that waits for the write in flight, so
	// that the delete goes after it.
	held tea.Cmd
	// drawing is true while a draw command runs. One runs at a time, so a
	// resize does not start a draw for each size it goes through.
	drawing bool
	drawn   *pieDrawnMsg // an encoded image that waits for its write
	failed  pieKey       // an image that could not be encoded; not tried again
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

// pieWrittenMsg comes back after a write to the terminal: key is the image
// the terminal then has, zero after a delete.
type pieWrittenMsg struct{ key pieKey }

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
// when the screen needs an image the terminal does not have, writes it when
// it is drawn, and deletes the image when the screen needs none. A write
// waits for the write in flight; a quit waits too, so that its delete is
// the last write.
func (a *App) syncPie(cmd tea.Cmd) tea.Cmd {
	p := &a.pie
	if p.writing {
		if a.quitting {
			p.held = tea.Batch(p.held, cmd)
			return nil
		}
		return cmd
	}
	cmd = tea.Batch(p.held, cmd)
	p.held = nil
	key, ok := a.wantedPie()
	if p.drawn != nil && (!ok || p.drawn.key != key) {
		p.drawn = nil // the screen changed while it was drawn
	}
	switch {
	case !ok:
		if p.sent == (pieKey{}) {
			return cmd
		}
		return a.writePie(termimg.Delete(p.id), pieKey{}, cmd)
	case key == p.sent:
		return cmd
	case p.drawn != nil:
		seq := p.drawn.seq
		p.drawn = nil
		return a.writePie(seq, key, cmd)
	case key == p.failed || p.drawing:
		return cmd
	}
	sec, _ := a.reports.allocationSection()
	p.drawing = true
	return tea.Batch(cmd, drawPie(key, p.id, sec.PieSlices(key.dark)))
}

// writePie writes seq to the terminal, then runs cmd. The pieWrittenMsg
// after the write carries key, the image the terminal then has. A sequence
// sends its messages in order to the one event loop, which writes the
// tea.Raw before Update gets the pieWrittenMsg.
func (a *App) writePie(seq string, key pieKey, cmd tea.Cmd) tea.Cmd {
	a.pie.writing = true
	return tea.Sequence(tea.Raw(seq), func() tea.Msg { return pieWrittenMsg{key: key} }, cmd)
}

// pieDrawn keeps a drawn image for its write, if the screen still needs it.
// Else syncPie starts the draw of the image it needs now.
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
	a.pie.drawn = &msg
	return nil
}

// pieWritten records the image the terminal has after a write.
func (a *App) pieWritten(msg pieWrittenMsg) {
	a.pie.writing = false
	a.pie.sent = msg.key
}
