package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// View implements tea.Model.
func (a *App) View() tea.View {
	v := tea.NewView(a.viewContent())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "TMoney - Personal Finance Manager"
	return v
}

func (a *App) viewContent() string {
	if a.quitting {
		return "Goodbye!\n"
	}

	if !a.ready {
		return "Loading..."
	}

	if a.err != nil {
		return a.renderError()
	}

	// Build the main layout
	return a.renderLayout()
}

// renderLayout renders the main application layout.
func (a *App) renderLayout() string {
	// Calculate content area dimensions
	headerHeight := 1
	statusBarHeight := 1
	contentHeight := a.height - headerHeight - statusBarHeight

	contentHeight = max(contentHeight, 1)

	// Render components
	header := a.renderHeader()
	content := a.renderContent(contentHeight)
	statusBar := a.renderStatusBar()

	layout := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		content,
		statusBar,
	)

	// Overlay dropdown if menu is active
	if a.menubar.IsActive() {
		dropdown, offset := a.menubar.RenderDropdown(a.styles)
		if dropdown != "" {
			layout = widget.OverlayDropdown(layout, dropdown, offset, 1, a.width)
		}
	}

	// The read-only corporate-action details panel paints under every registry
	// surface. It is not a registry modal: its keys belong to the corporate-
	// action view handler and it can only exist while that view is active.
	// While it is open the view swallows every key, so no registry surface can
	// be raised over it, and the order between the two is unobservable.
	if a.corporateActions.detail != nil && a.corporateActions.data != nil &&
		a.currentView == ViewCorporateActions {
		overlay := a.corporateActions.renderDetails(a.styles, a.width)
		layout = widget.OverlayCenter(layout, overlay, a.width, a.height)
	}

	return a.paintModals(layout)
}

// dialogMaxHeight is the height bound passed to base dialogs so a form taller
// than the screen scrolls its field region instead of overflowing past the
// status bar. It reserves the header row and the status-bar row, leaving the
// dialog the full content area between them.
func (a *App) dialogMaxHeight() int {
	return max(a.height-2, 3)
}

// renderHeader renders the application header/menu bar.
func (a *App) renderHeader() string {
	return a.menubar.Render(a.styles, a.width)
}

// renderContent renders the main content area based on current view.
func (a *App) renderContent(height int) string {
	viewContent := "Unknown view"
	e, ok := viewFor(a.currentView)
	if ok {
		viewContent = e.render(a)
	}

	// A full-screen view, and every view in the small layout, has no sidebar
	// and takes the full width.
	sidebarWidth := a.styles.SidebarWidth()
	if e.fullScreen || sidebarWidth == 0 {
		return a.styles.RenderViewContent(viewContent, a.width, height)
	}

	// Two-pane layout: sidebar + content
	sidebar := a.sidebar.Render(a.styles, sidebarWidth, height)
	content := a.styles.RenderViewContent(viewContent, a.styles.ContentWidth(), height)

	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, content)
}

// renderStatusBar renders the status bar at the bottom.
func (a *App) renderStatusBar() string {
	return a.statusbar.Render(a.styles, a.width)
}

// commonKeyHints ends the key hints of every view but Reconciliation.
const commonKeyHints = "Alt+key/F10 menu  1 dashboard  2 scheduled  3 reports  4 securities  5 prices  ? help  ctrl+q quit"

// getKeyHints returns key hints for the current view, from the view table.
func (a *App) getKeyHints() string {
	if e, ok := viewFor(a.currentView); ok {
		return e.hints(a)
	}
	return commonKeyHints
}

// renderError renders an error message.
func (a *App) renderError() string {
	return a.styles.Error.Render(fmt.Sprintf("Error: %v\n\nPress any key to continue", a.err))
}

// handleWindowSize records a new terminal size and rebuilds whichever register
// table the new width changes.
//
// The register tables decide whether to show the running-balance column from
// the available width, which is fixed at build time. Rebuild a loaded register
// only when that decision actually flips, so the column appears/disappears
// live on resize without resetting scroll/cursor state (SetRows resets scroll)
// on every no-op resize tick.
func (a *App) handleWindowSize(width, height int) {
	a.width = width
	a.height = height
	a.styles.Resize(width, height)
	a.ready = true
	if a.register.data != nil && tableHasBalanceColumn(a.register.table) != a.shouldShowRegisterBalance() {
		a.buildRegisterTable()
	}
	// The effective decision also suppresses the balance column while a
	// security filter is active, so mirror that here to avoid a needless
	// rebuild (and scroll/cursor reset) on every resize tick while filtered.
	if a.investmentRegister.data != nil &&
		tableHasBalanceColumn(a.investmentRegister.table) != (a.shouldShowInvestmentBalance() && !a.investmentRegisterFilterActive()) {
		a.buildInvestmentRegisterTable()
	}
}
