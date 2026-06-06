package dialog

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haskovec/tmoney/internal/tui/widget"
)

// buildTallDialog builds a dialog shaped like the lot-tracked Sell dialog:
// Date, Security, Shares, then many per-lot numeric fields, then trailing
// money fields. Returns the dialog and the labels of the lot fields (index i
// is dialog field index 3+i).
func buildTallDialog(numLots int) (*Dialog, []string) {
	d := NewDialog("Sell Securities")
	d.SetWidth(70)
	d.AddDateField("Date", "02/11/2026")
	d.AddTextField("Security", "VWO", "", 20)
	d.AddNumericField("Shares", "11.08311", "", 12)
	lotLabels := make([]string, numLots)
	for i := range numLots {
		label := fmt.Sprintf("Lot#%02d", i)
		lotLabels[i] = label
		d.AddNumericField(label, "", "0", 12)
	}
	d.AddNumericField("Total", "", "", 12)
	d.AddNumericField("PricePer", "", "", 12)
	d.AddNumericField("Commission", "0", "", 12)
	d.AddTextField("Memo", "", "", 30)
	return d, lotLabels
}

func TestDialog_Render_ClampsToMaxHeightAndScrolls(t *testing.T) {
	styles := widget.NewStyles()
	d, lots := buildTallDialog(40)

	const maxH = 20
	d.SetMaxHeight(maxH)

	// Focus the Date field (top of the form): it must be visible and the box
	// must already be clamped to maxHeight (the form is far taller than 20).
	d.SetFocusIndex(0)
	top := d.Render(styles)
	if h := lipgloss.Height(top); h > maxH {
		t.Fatalf("rendered height %d exceeds maxHeight %d (top focus)", h, maxH)
	}
	if !strings.Contains(widget.StripAnsi(top), "Date") {
		t.Error("Date field should be visible when focused at the top")
	}

	// Focus a deep lot field: it must scroll into view, stay within maxHeight,
	// and the top fields must scroll away.
	const deepLot = 30
	d.SetFocusIndex(3 + deepLot)
	out := d.Render(styles)
	// The overflowing dialog should fill its budget exactly — no more, no less.
	if h := lipgloss.Height(out); h != maxH {
		t.Fatalf("rendered height %d, want exactly maxHeight %d (deep focus)", h, maxH)
	}
	if rh := d.RenderedHeight(); rh != maxH {
		t.Fatalf("RenderedHeight() = %d, want clamped to maxHeight %d", rh, maxH)
	}
	plain := widget.StripAnsi(out)
	if !strings.Contains(plain, lots[deepLot]) {
		t.Errorf("focused lot %q must be visible after scrolling; got:\n%s", lots[deepLot], plain)
	}
	if strings.Contains(plain, "Date") {
		t.Errorf("the Date field should have scrolled out of view; got:\n%s", plain)
	}
	if !strings.ContainsAny(plain, "█│") {
		t.Errorf("a scrollbar (█/│) should be drawn when content overflows; got:\n%s", plain)
	}
	// Title and buttons stay pinned regardless of scroll position.
	if !strings.Contains(plain, "Sell Securities") {
		t.Error("title should remain pinned while scrolled")
	}
	if !strings.Contains(plain, "Save") || !strings.Contains(plain, "Cancel") {
		t.Error("Save/Cancel buttons should remain pinned/visible while scrolled")
	}
}

// A dialog that fits within the budget must render byte-for-byte identically
// whether or not a (generous) maxHeight is set — guarding the no-scroll path.
func TestDialog_Render_FittingDialogUnaffectedByMaxHeight(t *testing.T) {
	styles := widget.NewStyles()
	build := func() *Dialog {
		d := NewDialog("Test")
		d.AddTextField("Name", "John", "", 0)
		d.AddCheckboxField("Active", true)
		return d
	}

	unbounded := build().Render(styles)

	bounded := build()
	bounded.SetMaxHeight(100) // far larger than the dialog needs
	got := bounded.Render(styles)

	if unbounded != got {
		t.Errorf("a fitting dialog must render identically with a generous maxHeight\nunbounded:\n%q\nbounded:\n%q", unbounded, got)
	}
}

// While scrolled, a click on a visible field row must map to that field (not
// the field that occupied the same screen row before scrolling), and the
// pinned button row must still resolve to a button.
func TestDialog_HitTestContent_ScrolledMapping(t *testing.T) {
	styles := widget.NewStyles()
	d, _ := buildTallDialog(40)
	d.SetMaxHeight(20)

	const focus = 3 + 30 // lot #30 → dialog field index 33
	d.SetFocusIndex(focus)
	_ = d.Render(styles) // clamps d.fieldScroll for the focused field

	contentWidth := d.Width() - DialogHorizontalOverhead
	lenTop := scrollPinnedTopRows

	// Every visible field is single-line, so field i's content sits at block
	// line 2*i+1; its on-screen row is lenTop + (blockLine - fieldScroll).
	focusRow := lenTop + (2*focus + 1 - d.fieldScroll)
	hit := d.HitTestContent(10, focusRow, contentWidth)
	if hit.Zone != DialogHitField || hit.FieldIndex != focus {
		t.Fatalf("click on focused row %d: got zone=%d field=%d, want field %d", focusRow, hit.Zone, hit.FieldIndex, focus)
	}

	// The pinned button row is the last content line; it must still hit a button.
	viewport := max(d.effectiveMaxContent()-lenTop-d.bottomRowCount(), 1)
	buttonRow := lenTop + viewport + d.bottomRowCount() - 1
	foundButton := false
	for x := range contentWidth {
		if d.HitTestContent(x, buttonRow, contentWidth).Zone == DialogHitButton {
			foundButton = true
			break
		}
	}
	if !foundButton {
		t.Errorf("expected a button hit on the pinned button row at y=%d", buttonRow)
	}
}

// End-to-end mouse path while scrolled: a real screen-coordinate click (routed
// through DialogBounds + HandleMouse, exactly as the app delivers it) must (a)
// focus the visible lot row it lands on — not whatever field occupied that row
// before scrolling — and (b) still submit when the pinned Save button is
// clicked. This is the integration the bug fix must not break.
func TestDialog_HandleMouse_ScrolledClickFocusesAndSubmits(t *testing.T) {
	styles := widget.NewStyles()
	d, _ := buildTallDialog(40)
	d.SetVisible(true)
	d.SetMaxHeight(20)

	const screenW, screenH = 80, 24

	// Focus a deep lot so the window scrolls; render to commit the scroll.
	d.SetFocusIndex(3 + 30)
	_ = d.Render(styles)

	startCol, startRow, _, _ := d.DialogBounds(screenW, screenH)
	contentWidth := d.Width() - DialogHorizontalOverhead
	lenTop := scrollPinnedTopRows
	viewport := max(d.effectiveMaxContent()-lenTop-d.bottomRowCount(), 1)

	// (a) Click a *different* visible lot (lot#27 = field 30, single-line at
	// block line 2*30+1). Its on-screen content row is lenTop + (line - scroll).
	const target = 3 + 27
	clickLocalY := lenTop + (2*target + 1 - d.fieldScroll)
	if clickLocalY < lenTop || clickLocalY >= lenTop+viewport {
		t.Fatalf("test setup: target lot not in the visible window (localY=%d, window=[%d,%d))", clickLocalY, lenTop, lenTop+viewport)
	}
	d.HandleMouse(tea.MouseClickMsg{
		X:      startCol + 3 + 12, // +3 = border+left padding; 12 lands in the field area
		Y:      startRow + 2 + clickLocalY,
		Button: tea.MouseLeft,
	}, screenW, screenH)
	if d.FocusIndex() != target {
		t.Fatalf("scrolled click on lot row: FocusIndex()=%d, want %d", d.FocusIndex(), target)
	}

	// (b) Click the pinned Save button (button index 0, primary) → submit.
	buttonLocalY := lenTop + viewport + d.bottomRowCount() - 1
	action := DialogActionNone
	for lx := range contentWidth {
		if h := d.HitTestContent(lx, buttonLocalY, contentWidth); h.Zone == DialogHitButton && h.ButtonIndex == 0 {
			action = d.HandleMouse(tea.MouseClickMsg{
				X:      startCol + 3 + lx,
				Y:      startRow + 2 + buttonLocalY,
				Button: tea.MouseLeft,
			}, screenW, screenH)
			break
		}
	}
	if action != DialogActionSubmit {
		t.Errorf("clicking pinned Save while scrolled: action=%d, want DialogActionSubmit (%d)", action, DialogActionSubmit)
	}
}

// A message-heavy dialog (no fields, long body) must also stay within its
// height bound: the message lives in the scrollable body, not the pinned top,
// so it can't push the box past the status bar. The box stays rectangular.
func TestDialog_Render_MessageHeavyDialogStaysBounded(t *testing.T) {
	styles := widget.NewStyles()
	d := NewDialog("Notice")
	d.SetWidth(50)
	msg := make([]string, 30)
	for i := range msg {
		msg[i] = fmt.Sprintf("line %02d of a very long message body", i)
	}
	d.SetMessage(strings.Join(msg, "\n"))

	const maxH = 10
	d.SetMaxHeight(maxH)
	out := d.Render(styles)

	if h := lipgloss.Height(out); h > maxH {
		t.Fatalf("message-heavy dialog height %d exceeds maxHeight %d", h, maxH)
	}
	plain := widget.StripAnsi(out)
	if !strings.Contains(plain, "Notice") {
		t.Error("title should stay pinned")
	}
	if !strings.Contains(plain, "line 00") {
		t.Error("the top of the message should be visible")
	}
	// The box must be rectangular — every line the same visual width.
	lines := strings.Split(out, "\n")
	w0 := lipgloss.Width(lines[0])
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != w0 {
			t.Errorf("line %d width %d != %d (box not rectangular)", i, w, w0)
		}
	}
}

// While scrolled, the reserved scrollbar/gutter columns must be inert: clicking
// them neither focuses a field nor does anything, even though a normal click on
// the same row hits the field.
func TestDialog_HitTestContent_ScrollbarGutterInert(t *testing.T) {
	styles := widget.NewStyles()
	d, _ := buildTallDialog(40)
	d.SetMaxHeight(20)

	const focus = 3 + 30
	d.SetFocusIndex(focus)
	_ = d.Render(styles)

	contentWidth := d.Width() - DialogHorizontalOverhead
	fieldWidth := max(contentWidth-2, 5)
	focusRow := scrollPinnedTopRows + (2*focus + 1 - d.fieldScroll)

	// Sanity: a normal click on that row hits the focused field.
	if hit := d.HitTestContent(10, focusRow, contentWidth); hit.Zone != DialogHitField || hit.FieldIndex != focus {
		t.Fatalf("setup: expected field %d at row %d, got zone=%d field=%d", focus, focusRow, hit.Zone, hit.FieldIndex)
	}
	// The reserved gutter + scrollbar columns on the same row are inert.
	for _, x := range []int{fieldWidth, contentWidth - 1} {
		if hit := d.HitTestContent(x, focusRow, contentWidth); hit.Zone != DialogHitNone {
			t.Errorf("click on scrollbar gutter x=%d should be inert, got zone=%d field=%d", x, hit.Zone, hit.FieldIndex)
		}
	}
}
