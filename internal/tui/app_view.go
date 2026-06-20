package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/tui/dialog"
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

	// Overlay transaction dialog if visible
	if a.txnDialog != nil && a.txnDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.txnDialog)
	}

	// Overlay create-category sub-dialog if visible (sits on top of the
	// hidden transaction dialog).
	if a.createCatDialog != nil && a.createCatDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.createCatDialog)
	}

	// Overlay split dialog if visible
	if a.splitDialog != nil && a.splitDialog.IsVisible() {
		overlay := a.splitDialog.Render(a.styles)
		layout = widget.OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay transfer dialog if visible
	if a.transferDialog != nil && a.transferDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.transferDialog)
	}

	// Overlay scheduled dialog if visible
	if a.schedDialog != nil && a.schedDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.schedDialog)
	}

	// Overlay scheduled-preview dialog if visible. For multi-line
	// previews the header dialog and the embedded split editor stack
	// vertically; Tab transitions focus between the two surfaces
	// (header → split editor, Shift+Tab from split editor → header).
	if a.schedPreviewDialog != nil && a.schedPreviewDialog.IsVisible() {
		overlay := a.schedPreviewDialog.HeaderDialog().Render(a.styles)
		if a.schedPreviewDialog.IsMultiLine() {
			if sd := a.schedPreviewDialog.SplitDialog(); sd != nil {
				overlay = lipgloss.JoinVertical(lipgloss.Left, overlay, sd.Render(a.styles))
			}
		}
		layout = widget.OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay paycheck wizard if visible.
	if a.paycheckWizard != nil && a.paycheckWizard.IsVisible() {
		overlay := a.paycheckWizard.Render(a.styles)
		layout = widget.OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay account dialog if visible
	if a.acctDialog != nil && a.acctDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.acctDialog)
	}

	// Overlay backup dialog if visible
	if a.backupDialog != nil && a.backupDialog.dialog.IsVisible() {
		layout = a.overlayDialog(layout, a.backupDialog.dialog)
	}

	// Overlay file dialog if visible
	if a.fileDialog != nil && a.fileDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.fileDialog)
	}

	// Overlay reconciliation start dialog if visible
	if a.reconDialog != nil && a.reconDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.reconDialog)
	}

	// Overlay close-account dialog if visible
	if a.closeAcctDialog != nil && a.closeAcctDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.closeAcctDialog)
	}

	// Overlay security dialog if visible
	if a.securityDialog != nil && a.securityDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.securityDialog)
	}

	// Overlay price dialog if visible
	if a.priceDialog != nil && a.priceDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.priceDialog)
	}

	// Overlay price import dialog if visible
	if a.priceImportDialog != nil && a.priceImportDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.priceImportDialog)
	}

	// Overlay investment type selector dialog if visible
	if a.investmentTypeSelector != nil && a.investmentTypeSelector.IsVisible() {
		layout = a.overlayDialog(layout, a.investmentTypeSelector)
	}

	// Overlay buy dialog if visible
	if a.buyDialog != nil && a.buyDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.buyDialog)
	}

	// Overlay sell dialog if visible
	if a.sellDialog != nil && a.sellDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.sellDialog)
	}

	// Overlay fee-liquidation dialog if visible
	if a.feeLiquidationDialog != nil && a.feeLiquidationDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.feeLiquidationDialog)
	}

	// Overlay dividend dialog if visible
	if a.dividendDialog != nil && a.dividendDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.dividendDialog)
	}

	// Overlay cash operation dialog if visible
	if a.cashOperationDialog != nil && a.cashOperationDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.cashOperationDialog)
	}

	// Overlay transfer shares dialog if visible
	if a.transferSharesDialog != nil && a.transferSharesDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.transferSharesDialog)
	}

	// Overlay stock split dialog if visible
	if a.stockSplitDialog != nil && a.stockSplitDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.stockSplitDialog)
	}

	// Overlay merger dialog if visible
	if a.mergerDialog != nil && a.mergerDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.mergerDialog)
	}

	// Overlay merger confirmation if visible
	if a.mergerConfirmData != nil {
		overlay := a.renderMergerConfirmation()
		layout = widget.OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay spin-off dialog if visible
	if a.spinOffDialog != nil && a.spinOffDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.spinOffDialog)
	}

	// Overlay confirmation dialog if visible
	if a.confirmDialog != nil && a.confirmDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.confirmDialog)
	}

	// Overlay About dialog if visible
	if a.aboutDialog != nil && a.aboutDialog.IsVisible() {
		layout = a.overlayDialog(layout, a.aboutDialog)
	}

	// Overlay help if visible
	if a.showHelp {
		overlay := renderHelpOverlay(a.styles, a.currentView, a.width, a.height)
		layout = widget.OverlayCenter(layout, overlay, a.width, a.height)
	}

	return layout
}

// dialogMaxHeight is the height bound passed to base dialogs so a form taller
// than the screen scrolls its field region instead of overflowing past the
// status bar. It reserves the header row and the status-bar row, leaving the
// dialog the full content area between them.
func (a *App) dialogMaxHeight() int {
	return max(a.height-2, 3)
}

// overlayDialog renders a base dialog bounded to dialogMaxHeight and overlays
// it centered on the layout. Bounding makes a too-tall form (e.g. a Sell with
// many lots) scroll within the screen rather than spilling over the status
// bar; dialogs that already fit are unaffected.
func (a *App) overlayDialog(layout string, d *dialog.Dialog) string {
	d.SetMaxHeight(a.dialogMaxHeight())
	return widget.OverlayCenter(layout, d.Render(a.styles), a.width, a.height)
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
	default:
		return common
	}
}

// renderError renders an error message.
func (a *App) renderError() string {
	return a.styles.Error.Render(fmt.Sprintf("Error: %v\n\nPress any key to continue", a.err))
}
