package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/termimg"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// testPieID is the image ID of the pie apps.
var testPieID = termimg.ID{Lo: 200, Hi: 17}

// newPieApp is newAllocationApp in a terminal that shows kitty images: a
// truecolor profile, a kitty "OK", and cells of 10×20 px.
func newPieApp(width, height int, rpt *report.Allocation) *App {
	app := newAllocationApp(width, height, rpt)
	app.cfg = &config.Config{}
	app.pie = pieState{id: testPieID, pieTerm: pieTerm{kittyOK: true, cellW: 10, cellH: 20, profile: colorprofile.TrueColor}}
	return app
}

var cmdSliceType = reflect.TypeFor[[]tea.Cmd]()

// terminalOutput runs cmd the way the Bubble Tea loop would and returns what
// it writes to the terminal: the tea.Raw strings, in order, and "<quit>" for
// a quit. The other messages go back into the app, a few levels deep.
func terminalOutput(t *testing.T, a *App, cmd tea.Cmd, depth int) string {
	t.Helper()
	if cmd == nil || depth == 0 {
		return ""
	}
	msg := cmd()
	if v := reflect.ValueOf(msg); v.IsValid() && v.Kind() == reflect.Slice && v.Type().ConvertibleTo(cmdSliceType) {
		// A batch or a sequence: run its commands in order.
		var sb strings.Builder
		for _, c := range v.Convert(cmdSliceType).Interface().([]tea.Cmd) {
			sb.WriteString(terminalOutput(t, a, c, depth))
		}
		return sb.String()
	}
	switch msg := msg.(type) {
	case nil:
		return ""
	case tea.RawMsg:
		return fmt.Sprint(msg.Msg)
	case tea.QuitMsg:
		return "<quit>"
	case errMsg:
		t.Fatalf("command failed: %v", msg.err)
	}
	_, next := a.Update(msg)
	return terminalOutput(t, a, next, depth-1)
}

// step sends msg to the app and returns what the app writes to the terminal
// as a result.
func (a *App) step(t *testing.T, msg tea.Msg) string {
	t.Helper()
	_, cmd := a.Update(msg)
	return terminalOutput(t, a, cmd, 4)
}

// transmits counts the pie images in out.
func transmits(out string) int {
	return strings.Count(out, fmt.Sprintf("\x1b_Ga=p,U=1,i=%d,", testPieID.Value()))
}

func placeholderRows(view string) int {
	n := 0
	for line := range strings.Lines(view) {
		if strings.ContainsRune(line, kitty.Placeholder) {
			n++
		}
	}
	return n
}

// The terminal gets the image after the first message, and the class table
// then shows the pie: rows of placeholder cells, with the legend on their
// right. Until the image is sent, the pie's cells are blank.
func TestAllocationPie_Shows(t *testing.T) {
	app := newPieApp(180, 40, allocationTestReport())
	if n := placeholderRows(app.reports.render(app.styles, app.height, app.reportsLook())); n != 0 {
		t.Errorf("before the transmit: %d placeholder rows, want 0", n)
	}
	out := app.step(t, tea.WindowSizeMsg{Width: 180, Height: 40})
	if transmits(out) != 1 {
		t.Fatalf("got %d transmits, want 1:\n%q", transmits(out), out)
	}
	key, ok := app.wantedPie()
	if !ok || app.pie.sent != key {
		t.Fatalf("sent = %+v, want the wanted pie %+v (ok %v)", app.pie.sent, key, ok)
	}
	view := app.reports.render(app.styles, app.height, app.reportsLook())
	if n := placeholderRows(view); n != key.rows {
		t.Errorf("got %d placeholder rows, want %d", n, key.rows)
	}
	want := termimg.PlaceholderLine(testPieID, 0, key.cols) + strings.Repeat(" ", allocationPieGap)
	if !strings.Contains(view, want) {
		t.Errorf("the first line of the pie is not its placeholder row 0 and the gap")
	}
	plain := widget.StripAnsi(view)
	for _, w := range []string{
		"■ Large Cap Stock            2      $6000.00   60.0%",
		"■ Mid Cap Stock              1      $2500.00   25.0%",
		"  TOTAL                      4     $10000.00  100.0%",
		"Not included (no price): STRK (Large Cap Stock). Add a price to include it.",
		"1 holding has no asset class. Press enter on Unclassified, then e to set a class.",
	} {
		if !strings.Contains(plain, w) {
			t.Errorf("view lacks %q:\n%s", w, plain)
		}
	}
	if strings.Contains(plain, "█") {
		t.Errorf("the legend has a bar:\n%s", plain)
	}

	// Nothing changed: no new transmit.
	if out := app.step(t, tea.KeyPressMsg{Code: tea.KeyDown}); transmits(out) != 0 {
		t.Errorf("a key with no change sent %d images", transmits(out))
	}
}

// The pie takes the free rows, at most 20, and its columns follow the cell
// size, so it is round. When the legend does not fit beside it, a smaller
// pie is tried; below 8 rows there is no pie.
func TestAllocationPie_Size(t *testing.T) {
	for _, tc := range []struct {
		width, height, cellW, cellH int
		rows, cols                  int
	}{
		// 4 note lines: the no-price note, the unclassified note, the blank
		// line and the hint. 40 - 2 chrome - 4 - 4 = 30 free rows.
		{180, 40, 10, 20, 20, 40},
		{180, 40, 23, 50, 20, 43},
		{180, 24, 10, 20, 14, 28},
		{180, 18, 10, 20, 8, 16},
		{180, 17, 10, 20, 0, 0},
		// No cell size: a cell of 1:2.
		{180, 40, 0, 0, 20, 40},
	} {
		app := newPieApp(tc.width, tc.height, allocationTestReport())
		app.pie.cellW, app.pie.cellH = tc.cellW, tc.cellH
		key, ok := app.wantedPie()
		if ok != (tc.rows > 0) || key.rows != tc.rows || key.cols != tc.cols {
			t.Errorf("%dx%d, cell %dx%d: pie %dx%d (ok %v), want %dx%d",
				tc.width, tc.height, tc.cellW, tc.cellH, key.cols, key.rows, ok, tc.cols, tc.rows)
		}
		if tc.cellW > 0 && ok && (key.w != key.cols*tc.cellW || key.h != key.rows*tc.cellH) {
			t.Errorf("%dx%d: image %dx%d px, want the cells' %dx%d", tc.width, tc.height,
				key.w, key.h, key.cols*tc.cellW, key.rows*tc.cellH)
		}
	}

	// A narrow screen: a smaller pie that leaves room for the legend.
	app := newPieApp(180, 40, allocationTestReport())
	width := allocationTableWidth(app.styles)
	legend := columnsWidth(allocationLegendColumns(allocationValueMin))
	app.pie.cellW, app.pie.cellH = 20, 20 // cols = rows
	for rows := allocationPieMaxRows; rows >= allocationPieMinRows; rows-- {
		if rows+allocationPieGap+legend <= width {
			if key, _ := app.wantedPie(); key.rows != rows {
				t.Errorf("square cells, table width %d: %d rows, want %d", width, key.rows, rows)
			}
			break
		}
	}
	app = newPieApp(70, 40, allocationTestReport())
	if width := allocationTableWidth(app.styles); width >= 2*allocationPieMinRows+allocationPieGap+legend {
		t.Fatalf("table width %d is not too narrow for the test", width)
	}
	if key, ok := app.wantedPie(); ok {
		t.Errorf("no room for the legend: pie %+v, want none", key)
	}
	if strings.ContainsRune(app.renderedReports(), '■') {
		t.Error("with no pie, the table has swatches")
	}
}

// The pie shows only where the terminal can show it (§6.3).
func TestAllocationPie_When(t *testing.T) {
	for _, tc := range []struct {
		name    string
		setting string
		change  func(*pieState)
		want    bool
	}{
		{"auto, kitty OK", "", func(*pieState) {}, true},
		{"256 colors", "auto", func(p *pieState) { p.profile = colorprofile.ANSI256 }, true},
		{"16 colors", "auto", func(p *pieState) { p.profile = colorprofile.ANSI }, false},
		{"16 colors, on", "on", func(p *pieState) { p.profile = colorprofile.ANSI }, false},
		{"no profile yet", "auto", func(p *pieState) { p.profile = colorprofile.Unknown }, false},
		{"no kitty reply", "auto", func(p *pieState) { p.kittyOK = false }, false},
		{"no kitty reply, on", "on", func(p *pieState) { p.kittyOK = false }, true},
		{"tmux", "auto", func(p *pieState) { p.inTmux = true }, false},
		{"tmux, on", "on", func(p *pieState) { p.inTmux = true }, true},
		{"off", "off", func(*pieState) {}, false},
	} {
		app := newPieApp(180, 40, allocationTestReport())
		app.cfg.ChartImages = tc.setting
		tc.change(&app.pie)
		if _, ok := app.wantedPie(); ok != tc.want {
			t.Errorf("%s: pie %v, want %v", tc.name, ok, tc.want)
		}
	}
}

// A section the pie cannot show has the table with bars, and the note that
// says why; a terminal that cannot show the pie gets no note.
func TestAllocationPie_NegativeCash(t *testing.T) {
	rpt := allocationTestReport()
	sec := &rpt.Sections[0]
	for i := range sec.Classes {
		if sec.Classes[i].Class == security.AssetClassCash {
			sec.Classes[i].Value = types.MustNewMoney("-1000.00")
		}
	}
	app := newPieApp(180, 40, rpt)
	if _, ok := app.wantedPie(); ok {
		t.Error("negative cash has a pie")
	}
	view := app.renderedReports()
	if !strings.Contains(view, "█") || !strings.Contains(view, "No pie: Cash is negative.") {
		t.Errorf("want the table with bars and the note:\n%s", view)
	}
	app.pie.kittyOK = false
	if view := app.renderedReports(); strings.Contains(view, "No pie") {
		t.Errorf("a terminal with no pie has the note:\n%s", view)
	}
}

// screenLine is the line of the app's screen that has text, and the screen
// column where text starts in it.
func screenLine(t *testing.T, a *App, text string) (y, x int) {
	t.Helper()
	for y, line := range strings.Split(a.viewContent(), "\n") {
		plain := ansi.Strip(line)
		if i := strings.Index(plain, text); i >= 0 {
			return y, ansi.StringWidth(plain[:i])
		}
	}
	t.Fatalf("no screen line has %q", text)
	return 0, 0
}

// A click on the legend selects its row; a click on the pie selects nothing
// (C9).
func TestAllocationPie_Clicks(t *testing.T) {
	app := newPieApp(180, 40, allocationTestReport())
	app.ready = true
	app.step(t, tea.WindowSizeMsg{Width: 180, Height: 40})
	y, x := screenLine(t, app, "Mid Cap Stock")
	key, _ := app.wantedPie()
	pieX := x - 2 - allocationPieGap - key.cols/2 // left of the swatch, the gap, then the pie

	app.step(t, tea.MouseClickMsg{X: pieX, Y: y, Button: tea.MouseLeft})
	if c := app.reports.allocation.table.Cursor(); c != 0 {
		t.Errorf("a click on the pie moved the cursor to %d", c)
	}
	app.step(t, tea.MouseClickMsg{X: x - 2 - allocationPieGap + 1, Y: y, Button: tea.MouseLeft})
	if c := app.reports.allocation.table.Cursor(); c != 0 {
		t.Errorf("a click on the gap moved the cursor to %d", c)
	}
	app.step(t, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if c := app.reports.allocation.table.Cursor(); c != 1 {
		t.Errorf("a click on Mid Cap Stock: cursor %d, want 1", c)
	}
	app.step(t, tea.MouseClickMsg{X: x - 2, Y: y + 1, Button: tea.MouseLeft})
	if c := app.reports.allocation.table.Cursor(); c != 2 {
		t.Errorf("a click on the swatch of Cash: cursor %d, want 2", c)
	}
}

// The drill-down hides the pie and keeps the image, so the pie comes back
// with no new transmit. Leaving the Reports view and quitting delete the
// image (C11); the delete goes before the quit.
func TestAllocationPie_Lifecycle(t *testing.T) {
	app := newPieApp(180, 40, allocationTestReport())
	app.ready = true
	if out := app.step(t, tea.WindowSizeMsg{Width: 180, Height: 40}); transmits(out) != 1 {
		t.Fatalf("got %d transmits, want 1", transmits(out))
	}
	del := termimg.Delete(testPieID)

	out := app.step(t, tea.KeyPressMsg{Code: tea.KeyEnter})
	if app.reports.allocation.drill == nil {
		t.Fatal("enter did not open the drill-down")
	}
	if transmits(out) != 0 || strings.Contains(out, del) {
		t.Errorf("the drill-down wrote %q", out)
	}
	if n := placeholderRows(app.reports.render(app.styles, app.height, app.reportsLook())); n != 0 {
		t.Errorf("the drill-down has %d placeholder rows", n)
	}
	out = app.step(t, tea.KeyPressMsg{Code: tea.KeyEscape})
	if transmits(out) != 0 || strings.Contains(out, del) {
		t.Errorf("back from the drill-down wrote %q", out)
	}
	if n := placeholderRows(app.reports.render(app.styles, app.height, app.reportsLook())); n == 0 {
		t.Error("back from the drill-down, the pie is not on screen")
	}

	// A resize sends a new image, of the new size.
	if out := app.step(t, tea.WindowSizeMsg{Width: 180, Height: 24}); transmits(out) != 1 {
		t.Errorf("a resize sent %d images, want 1", transmits(out))
	}

	app.switchView(ViewDashboard)
	if out := app.step(t, tea.KeyPressMsg{Code: tea.KeyDown}); out != del {
		t.Errorf("leaving Reports wrote %q, want the delete", out)
	}
	app.switchView(ViewReports)
	if out := app.step(t, tea.KeyPressMsg{Code: tea.KeyDown}); transmits(out) != 1 {
		t.Errorf("back in Reports: %d transmits, want 1", transmits(out))
	}
	if out := app.step(t, tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl}); out != del+"<quit>" {
		t.Errorf("quit wrote %q, want the delete, then the quit", out)
	}
}

// A draw that ends after the screen changed is not sent; the pie the screen
// needs now is drawn.
func TestAllocationPie_StaleDraw(t *testing.T) {
	app := newPieApp(180, 40, allocationTestReport())
	_, cmd := app.Update(tea.WindowSizeMsg{Width: 180, Height: 40})
	if !app.pie.drawing {
		t.Fatal("no draw started")
	}
	// The screen gets smaller while the first draw runs: 24 rows leave 14
	// for the pie.
	_, second := app.Update(tea.WindowSizeMsg{Width: 180, Height: 24})
	if second != nil {
		t.Error("a second draw started while the first runs")
	}
	out := terminalOutput(t, app, cmd, 4)
	if transmits(out) != 1 {
		t.Fatalf("got %d transmits, want 1 (the new size)", transmits(out))
	}
	key, _ := app.wantedPie()
	if app.pie.sent != key || key.rows != 14 {
		t.Errorf("sent %+v, want the pie of the new size %+v", app.pie.sent, key)
	}
	want := fmt.Sprintf("c=%d,r=%d,", key.cols, key.rows)
	if !strings.Contains(out, want) {
		t.Errorf("the transmit is not of the new size %s", want)
	}
}

// The terminal's replies set what the pie needs; the query asks for the
// cell size first. With the setting off, the app asks nothing.
func TestAllocationPie_Replies(t *testing.T) {
	app := newAllocationApp(180, 40, allocationTestReport())
	app.cfg = &config.Config{}
	if out := terminalOutput(t, app, app.queryPie(), 1); out != termimg.Query {
		t.Errorf("query = %q, want %q", out, termimg.Query)
	}
	app.Update(tea.ColorProfileMsg{Profile: colorprofile.ANSI256})
	app.Update(uv.CellSizeEvent{Width: 9, Height: 18})
	app.Update(uv.KittyGraphicsEvent{Options: kitty.Options{ID: 7}, Payload: []byte("OK")})
	if app.pie.kittyOK {
		t.Error("an OK for another image ID set kittyOK")
	}
	app.Update(uv.KittyGraphicsEvent{Options: kitty.Options{ID: termimg.QueryID}, Payload: []byte("EINVAL")})
	if app.pie.kittyOK {
		t.Error("an error reply set kittyOK")
	}
	app.Update(uv.KittyGraphicsEvent{Options: kitty.Options{ID: termimg.QueryID}, Payload: []byte("OK")})
	if !app.pie.kittyOK || app.pie.cellW != 9 || app.pie.cellH != 18 || app.pie.profile != colorprofile.ANSI256 {
		t.Errorf("pie terminal = %+v", app.pie.pieTerm)
	}

	app.cfg.ChartImages = "off"
	if cmd := app.queryPie(); cmd != nil {
		t.Error("with the setting off, the app sends the query")
	}
}

// View → Chart images moves Auto → On → Off → Auto. The pie follows at once,
// and a change from Off asks the terminal again.
func TestAllocationPie_CycleSetting(t *testing.T) {
	app := newPieApp(180, 40, allocationTestReport())
	app.step(t, tea.WindowSizeMsg{Width: 180, Height: 40})
	for _, want := range []string{"on", "off", "auto"} {
		_, cmd := app.handleMenuAction(widget.MenuActionCycleChartImages, "")
		cmd = app.syncPie(cmd)
		out := terminalOutput(t, app, cmd, 4)
		if app.cfg.ChartImages != want {
			t.Fatalf("setting = %q, want %q", app.cfg.ChartImages, want)
		}
		switch want {
		case "off":
			if out != termimg.Delete(testPieID) {
				t.Errorf("off wrote %q, want the delete", out)
			}
		case "auto":
			if !strings.HasPrefix(out, termimg.Query) || transmits(out) != 1 {
				t.Errorf("off to auto wrote %q, want the query and a transmit", out)
			}
		}
	}
	items := widget.BuildViewMenuItems("", false, app.chartImagesMode().Label())
	if items[1].Label != "  Chart images: Auto" {
		t.Errorf("menu item = %q", items[1].Label)
	}
}

// Bubble Tea runs each command in its own goroutine, so the writes to the
// terminal go one at a time. Here the transmit has not run when the owner
// leaves Reports and quits: the delete waits for the transmit, and the quit
// for the delete.
func TestAllocationPie_WritesInOrder(t *testing.T) {
	app := newPieApp(180, 40, allocationTestReport())
	app.ready = true
	_, draw := app.Update(tea.WindowSizeMsg{Width: 180, Height: 40})
	_, write := app.Update(draw())
	if !app.pie.writing || write == nil {
		t.Fatal("the drawn pie is not being written")
	}
	if app.pie.sent != (pieKey{}) {
		t.Error("the pie counts as sent before its write came back")
	}

	app.switchView(ViewDashboard)
	if _, cmd := app.Update(tea.KeyPressMsg{Code: tea.KeyDown}); strings.Contains(terminalOutput(t, app, cmd, 4), termimg.Delete(testPieID)) {
		t.Error("the delete did not wait for the transmit")
	}
	if _, cmd := app.Update(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl}); cmd != nil {
		t.Error("the quit did not wait for the transmit")
	}

	out := terminalOutput(t, app, write, 6)
	transmit := strings.Index(out, "\x1b_Ga=p,U=1,")
	del := strings.Index(out, termimg.Delete(testPieID))
	if transmit < 0 || del < transmit || !strings.HasSuffix(out, termimg.Delete(testPieID)+"<quit>") {
		t.Errorf("wrote %q, want the transmit, the delete, then the quit", out)
	}
	if app.pie.sent != (pieKey{}) || app.pie.writing {
		t.Errorf("after the quit: sent %+v, writing %v", app.pie.sent, app.pie.writing)
	}
}
