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
	if a.corporateActionDetail != nil && a.corporateActionView != nil &&
		a.currentView == ViewCorporateActions {
		overlay := a.renderCorporateActionDetails()
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
	var viewContent string
	switch a.currentView {
	case ViewDashboard:
		viewContent = a.renderDashboard()
	case ViewRegister:
		viewContent = a.renderRegister()
	case ViewScheduled:
		viewContent = a.renderScheduled()
	case ViewReports:
		viewContent = a.renderReports()
	case ViewReconciliation:
		viewContent = a.renderReconciliation()
	case ViewSecurities:
		viewContent = a.renderSecurityView()
	case ViewPrices:
		viewContent = a.renderPriceView()
	case ViewInvestmentRegister:
		viewContent = a.renderInvestmentRegister()
	case ViewPortfolio:
		viewContent = a.renderPortfolioView()
	case ViewCorporateActions:
		viewContent = a.renderCorporateActionView()
	case ViewAmortization:
		viewContent = a.renderAmortizationView()
	default:
		viewContent = "Unknown view"
	}

	// Reconciliation, Securities, Prices, Corporate Actions, and Amortization
	// views are full-screen (no sidebar)
	if a.currentView == ViewReconciliation || a.currentView == ViewSecurities || a.currentView == ViewPrices || a.currentView == ViewCorporateActions || a.currentView == ViewAmortization {
		return a.styles.RenderViewContent(viewContent, a.width, height)
	}

	sidebarWidth := a.styles.SidebarWidth()
	if sidebarWidth == 0 {
		// Small layout: no sidebar, full-width content
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

// getKeyHints returns key hints for the current view.
func (a *App) getKeyHints() string {
	common := "Alt+key/F10 menu  1 dashboard  2 scheduled  3 reports  4 securities  5 prices  ? help  ctrl+q quit"

	switch a.currentView {
	case ViewDashboard:
		return "↑↓ navigate  ←→ collapse/expand  enter select  " + common
	case ViewRegister:
		return "↑↓ navigate  enter edit  n new  t transfer  c clear  v void  r reconcile  d delete  esc back  " + common
	case ViewScheduled:
		return "↑↓ navigate  enter post  s skip  n new  t transfer  e edit  d delete  esc back  " + common
	case ViewReports:
		return "←→ period  n net worth  s spending  y year  m month  esc back  " + common
	case ViewReconciliation:
		return "space toggle  enter finish  esc cancel  a check all  u uncheck all  ? help"
	case ViewSecurities:
		return "↑↓ navigate  n new  enter edit  h hide/unhide  d delete  f filter hidden  u update prices  a actions  / search  esc back  " + common
	case ViewPrices:
		if a.priceView != nil && a.priceView.mode == pricesViewDetail {
			return "↑↓ navigate  enter edit  n new  d delete  i import  / search  esc back  " + common
		}
		return "↑↓ navigate  enter view history  / search  esc back  " + common
	case ViewInvestmentRegister:
		return "↑↓ navigate  enter edit  n new  c clear  d delete  p portfolio  esc back  " + common
	case ViewPortfolio:
		return "↑↓ navigate  enter lot detail  r register  esc back  " + common
	case ViewCorporateActions:
		return "↑↓ navigate  / filter  enter details  d delete  esc back  " + common
	case ViewAmortization:
		return "↑↓ navigate  g/G first/last  esc back  " + common
	default:
		return common
	}
}

// renderError renders an error message.
func (a *App) renderError() string {
	return a.styles.Error.Render(fmt.Sprintf("Error: %v\n\nPress any key to continue", a.err))
}
