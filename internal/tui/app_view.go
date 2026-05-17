package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
			layout = overlayDropdown(layout, dropdown, offset, 1, a.width)
		}
	}

	// Overlay transaction dialog if visible
	if a.txnDialog != nil && a.txnDialog.IsVisible() {
		overlay := a.txnDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay create-category sub-dialog if visible (sits on top of the
	// hidden transaction dialog).
	if a.createCatDialog != nil && a.createCatDialog.IsVisible() {
		overlay := a.createCatDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay split dialog if visible
	if a.splitDialog != nil && a.splitDialog.IsVisible() {
		overlay := a.splitDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay transfer dialog if visible
	if a.transferDialog != nil && a.transferDialog.IsVisible() {
		overlay := a.transferDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay scheduled dialog if visible
	if a.schedDialog != nil && a.schedDialog.IsVisible() {
		overlay := a.schedDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay scheduled-preview dialog if visible (MS-019 scaffolding;
	// MS-020 will land the save handler). For multi-line previews the
	// header is the primary surface — the embedded split editor's
	// rendering is wired in subsequent slices.
	if a.schedPreviewDialog != nil && a.schedPreviewDialog.IsVisible() {
		overlay := a.schedPreviewDialog.HeaderDialog().Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay account dialog if visible
	if a.acctDialog != nil && a.acctDialog.IsVisible() {
		overlay := a.acctDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay backup dialog if visible
	if a.backupDialog != nil && a.backupDialog.dialog.IsVisible() {
		overlay := a.backupDialog.dialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay file dialog if visible
	if a.fileDialog != nil && a.fileDialog.IsVisible() {
		overlay := a.fileDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay reconciliation start dialog if visible
	if a.reconDialog != nil && a.reconDialog.IsVisible() {
		overlay := a.reconDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay security dialog if visible
	if a.securityDialog != nil && a.securityDialog.IsVisible() {
		overlay := a.securityDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay price dialog if visible
	if a.priceDialog != nil && a.priceDialog.IsVisible() {
		overlay := a.priceDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay price import dialog if visible
	if a.priceImportDialog != nil && a.priceImportDialog.IsVisible() {
		overlay := a.priceImportDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay investment type selector dialog if visible
	if a.investmentTypeSelector != nil && a.investmentTypeSelector.IsVisible() {
		overlay := a.investmentTypeSelector.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay buy dialog if visible
	if a.buyDialog != nil && a.buyDialog.IsVisible() {
		overlay := a.buyDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay sell dialog if visible
	if a.sellDialog != nil && a.sellDialog.IsVisible() {
		overlay := a.sellDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay dividend dialog if visible
	if a.dividendDialog != nil && a.dividendDialog.IsVisible() {
		overlay := a.dividendDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay cash operation dialog if visible
	if a.cashOperationDialog != nil && a.cashOperationDialog.IsVisible() {
		overlay := a.cashOperationDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay transfer cash dialog if visible
	if a.transferCashDialog != nil && a.transferCashDialog.IsVisible() {
		overlay := a.transferCashDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay transfer shares dialog if visible
	if a.transferSharesDialog != nil && a.transferSharesDialog.IsVisible() {
		overlay := a.transferSharesDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay stock split dialog if visible
	if a.stockSplitDialog != nil && a.stockSplitDialog.IsVisible() {
		overlay := a.stockSplitDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay merger dialog if visible
	if a.mergerDialog != nil && a.mergerDialog.IsVisible() {
		overlay := a.mergerDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay merger confirmation if visible
	if a.mergerConfirmData != nil {
		overlay := a.renderMergerConfirmation()
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay spin-off dialog if visible
	if a.spinOffDialog != nil && a.spinOffDialog.IsVisible() {
		overlay := a.spinOffDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay confirmation dialog if visible
	if a.confirmDialog != nil && a.confirmDialog.IsVisible() {
		overlay := a.confirmDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay About dialog if visible
	if a.aboutDialog != nil && a.aboutDialog.IsVisible() {
		overlay := a.aboutDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay help if visible
	if a.showHelp {
		overlay := renderHelpOverlay(a.styles, a.currentView, a.width, a.height)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	return layout
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
	default:
		viewContent = "Unknown view"
	}

	// Reconciliation, Securities, and Prices views are full-screen (no sidebar)
	if a.currentView == ViewReconciliation || a.currentView == ViewSecurities || a.currentView == ViewPrices || a.currentView == ViewCorporateActions {
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
		return "↑↓ navigate  enter post  s skip  n new  e edit  d delete  esc back  " + common
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
	default:
		return common
	}
}

// renderError renders an error message.
func (a *App) renderError() string {
	return a.styles.Error.Render(fmt.Sprintf("Error: %v\n\nPress any key to continue", a.err))
}
