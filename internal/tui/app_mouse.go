package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// handleMouseEvent handles mouse events (clicks, wheel scrolling).
func (a *App) handleMouseEvent(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.MouseWheelMsg:
		return a.handleMouseWheel(msg)
	case tea.MouseClickMsg:
		if msg.Mouse().Button != tea.MouseLeft {
			return a, nil
		}
	default:
		return a, nil
	}

	m := msg.Mouse()

	// Route mouse clicks to dialog when a modal dialog is visible
	if a.isDialogVisible() {
		return a.handleDialogMouse(msg)
	}

	// widget.Menu bar (row 0)
	if m.Y == 0 {
		return a.handleMouseMenuBar(msg)
	}

	// If menu dropdown is open, check if click is on dropdown
	if a.menubar.IsActive() {
		colOffset, dropdownWidth, itemCount := a.menubar.DropdownBounds()
		if m.Y >= 1 && m.Y <= itemCount &&
			m.X >= colOffset && m.X < colOffset+dropdownWidth {
			itemIdx := a.menubar.HitTestDropdown(m.Y - 1)
			if itemIdx >= 0 {
				a.menubar.SetItemCursor(itemIdx)
				action, data := a.menubar.Select()
				return a.handleMenuAction(action, data)
			}
		}
		// Click outside dropdown closes it
		a.menubar.Deactivate()
		// Fall through to handle the click in the content area
	}

	// Status bar (last row) - ignore
	if m.Y >= a.height-1 {
		return a, nil
	}

	// Content area
	if m.Y >= 1 && m.Y < a.height-1 {
		return a.handleMouseContent(msg)
	}

	return a, nil
}

// handleMouseMenuBar handles mouse clicks on the menu bar (row 0).
func (a *App) handleMouseMenuBar(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	idx := a.menubar.HitTestBar(msg.Mouse().X)
	if idx >= 0 {
		if a.menubar.IsActive() && a.menubar.Cursor() == idx {
			a.menubar.Deactivate()
		} else {
			a.menubar.ActivateMenu(idx)
		}
	} else if a.menubar.IsActive() {
		a.menubar.Deactivate()
	}
	return a, nil
}

// handleMouseContent handles mouse clicks in the content area.
func (a *App) handleMouseContent(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := msg.Mouse()
	contentY := m.Y - 1 // Offset for header row

	sidebarWidth := a.styles.SidebarWidth()

	// Full-screen views (no sidebar). Must match the full-screen list in
	// renderContent (app_view.go) so left-region clicks reach the table
	// instead of the hidden sidebar.
	if sidebarWidth == 0 || a.currentView == ViewReconciliation ||
		a.currentView == ViewSecurities || a.currentView == ViewPrices ||
		a.currentView == ViewCorporateActions || a.currentView == ViewAmortization {
		// The dashboard has no sidebar in the small layout but still renders
		// its clickable ▸/▾ account headers (content starts at column 0).
		if a.currentView == ViewDashboard {
			return a.handleMouseDashboard(m, contentY, 0)
		}
		return a.handleMouseTable(msg, contentY)
	}

	// Sidebar zone
	if m.X < sidebarWidth {
		return a.handleMouseSidebar(msg, contentY)
	}

	// Border column - ignore
	if m.X == sidebarWidth {
		return a, nil
	}

	// Content zone (right of sidebar). On the dashboard, clicking an
	// investment account's ▸/▾ header toggles its holdings (content begins
	// one column past the sidebar border).
	if a.currentView == ViewDashboard {
		return a.handleMouseDashboard(m, contentY, sidebarWidth+1)
	}
	a.focusContent()
	return a.handleMouseTable(msg, contentY)
}

// handleMouseDashboard toggles expand/collapse when a click lands on an
// investment account's ▸/▾ header row in the dashboard's ASSETS column.
// contentStartX is the absolute column where the content pane begins (0 in the
// sidebar-less small layout, sidebarWidth+1 otherwise). A click on any other
// row, or in the LIABILITIES column of a header row, is ignored.
func (a *App) handleMouseDashboard(m tea.Mouse, contentY, contentStartX int) (tea.Model, tea.Cmd) {
	acctID, ok := a.dashboardAccountRows[contentY]
	if !ok {
		return a, nil
	}

	// Restrict to the ASSETS (left) column so a click on a LIABILITIES row
	// that happens to share this row can't toggle an unrelated asset. relX is
	// measured from contentStartX (the content-zone origin this router uses,
	// one column past the sidebar border); the ASSETS column spans 2 cols of
	// left padding + colWidth, with colWidth reusing renderAssetLiabilityColumns'
	// formula. The bound is generous but stays a column short of LIABILITIES
	// (which begins after a 2-col gutter), so the two never overlap.
	colWidth := max((a.styles.ContentWidth()-6)/2, 20)
	relX := m.X - contentStartX
	if relX < 0 || relX >= 2+colWidth {
		return a, nil
	}

	if a.dashboardExpandedAccounts == nil {
		a.dashboardExpandedAccounts = make(map[types.ID]bool)
	}
	a.dashboardExpandedAccounts[acctID] = !a.dashboardExpandedAccounts[acctID]
	// Keep the sidebar cursor on the clicked account so a follow-up keyboard
	// ←/→ operates on the same one.
	a.sidebar.SetCursorToAccount(acctID)
	return a, nil
}

// handleMouseTable handles mouse clicks in the table/content area.
// Single click moves the cursor; on tables that support drill-in
// (currently the prices list), a second click on the same row within
// the double-click threshold opens the row.
func (a *App) handleMouseTable(_ tea.MouseMsg, contentY int) (tea.Model, tea.Cmd) {
	tableY := contentY - a.tableContentRowOffset()

	tbl := a.activeTable()
	if tbl == nil {
		return a, nil
	}

	rowIdx := tbl.HitTest(tableY)
	if rowIdx < 0 {
		return a, nil
	}
	tbl.SetCursor(rowIdx)

	// Reconciliation: clicking a row toggles its cleared checkbox — the
	// primary action on this view (Space does the same on the keyboard).
	if a.currentView == ViewReconciliation {
		return a.toggleReconciliationCheck()
	}

	// Prices landing list: a single click selects the row and refreshes the
	// chart panel for the newly highlighted ticker (mirroring keyboard
	// navigation); a second click on the same row within the threshold
	// drills into that ticker's price history.
	if a.currentView == ViewPrices && a.priceView != nil && a.priceView.mode == pricesViewList {
		if a.priceListClicks == nil {
			a.priceListClicks = widget.NewClickTracker(widget.DoubleClickThreshold)
		}
		if a.priceListClicks.Click(rowIdx) {
			return a, a.drillIntoSelectedListRow()
		}
		return a, a.schedulePriceListChartFetchIfActive()
	}

	return a, nil
}

// handleMouseWheel handles mouse wheel scrolling.
func (a *App) handleMouseWheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if a.isDialogVisible() {
		return a.handleDialogMouse(msg)
	}

	wheelUp := msg.Mouse().Button == tea.MouseWheelUp

	if a.sidebar.IsFocused() {
		if wheelUp {
			a.sidebar.MoveUp()
		} else {
			a.sidebar.MoveDown()
		}
		return a, nil
	}

	tbl := a.activeTable()
	if tbl != nil {
		if wheelUp {
			tbl.MoveUp()
		} else {
			tbl.MoveDown()
		}
	}

	// On the prices landing list, refresh the chart panel for the row the
	// wheel scrolled to — same root cause as the single-click path above.
	// Returns nil (no-op) on every other view.
	return a, a.schedulePriceListChartFetchIfActive()
}

// handleDialogMouse routes a mouse event to the frontmost visible modal.
//
// The default is "ask the surface for a DialogAction, then dispatch it through
// the same onAction the keyboard uses". That is what closes the gap
// specs/tui.md:687-695 forbids: clicking a dialog button is exactly equivalent
// to the keyboard action, because it is now literally the same code. The old
// cascade kept a second copy of every switch, and 11 of them had drifted --
// their Cancel arms inlined `X.SetVisible(false); X = nil` and leaked whatever
// else the surface owned, where Esc called the surface's close helper.
//
// Surfaces that are not that shape declare an onMouse override in the
// registry, each with its reason.
func (a *App) handleDialogMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	e := a.frontmostModal()
	if e == nil {
		// Reached when a non-registry surface holds the gate open -- today only
		// the corporate-action details panel, which has one clickable target,
		// its [x], and otherwise swallows the click so the table it covers
		// cannot move.
		if a.corporateActionDetail != nil {
			return a.handleCorporateActionDetailMouse(msg)
		}
		return a, nil
	}
	if e.onMouse != nil {
		return e.onMouse(a, msg)
	}
	if e.onAction == nil {
		return a, nil
	}
	return e.onAction(a, a.modalMouseAction(e.modal, msg))
}

// modalMouseAction asks a surface to interpret a mouse event.
//
// The two cases are the whole reason HandleMouse is not a Modal method: the
// base dialog's hit-testing needs no styles and the paycheck wizard's does.
// Widening dialog.Dialog.HandleMouse to take a parameter it ignores would
// churn 18 call sites in the dialog package's own tests to satisfy an
// interface, so the divergence is stated here instead, in one place.
func (a *App) modalMouseAction(m Modal, msg tea.MouseMsg) dialog.DialogAction {
	switch s := m.(type) {
	case *dialog.Dialog:
		return s.HandleMouse(msg, a.width, a.height)
	case *PaycheckWizard:
		return s.HandleMouse(msg, a.styles, a.width, a.height)
	default:
		return dialog.DialogActionNone
	}
}
