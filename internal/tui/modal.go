package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// Modal is a surface that takes input away from the view underneath while it
// is visible. Every dialog, wizard and overlay in the TUI is one.
//
// The method set is deliberately narrow: it holds what the registry walks
// call, and nothing else. Two members that look like they belong are absent,
// each for a measured reason.
//
//   - HandleKey. Every surface already has one — 30 of the 31 key arms are
//     `X.HandleKey(msg)` followed by a switch on the returned DialogAction.
//     But that switch is what varies (Submit, Cancel, AddNew, Alternate), and
//     every arm of it needs *App: to call a service, close a sibling surface,
//     or divert into create-category. So key dispatch is per-surface glue and
//     lives on modalEntry.onKey. Putting HandleKey here would add a member
//     that 31 types satisfy and no walk calls.
//   - SetVisible. Only the create-category divert calls it, from the
//     originating surface rather than from the registry.
//
// Add a member when a walk needs it, not before. Phase 2 adds HandleMouse.
//
// Every implementation must be nil-safe, because a nil *dialog.Dialog stored
// in a Modal is not a nil interface and the walks below run over surfaces that
// have never been built. See dialog.Dialog.IsVisible.
type Modal interface {
	IsVisible() bool
	Render(styles widget.Styles) string
}

// modalEntry is one surface and the glue App supplies for it.
type modalEntry struct {
	// name identifies the surface in test failures and in the order assertion.
	// It is not shown to the user.
	name  string
	modal Modal
	// onKey receives every key while this surface is the frontmost visible
	// one. Through phase 2 these stay the existing handleXKey methods, which
	// is what makes the cascade collapse a pure routing change.
	onKey func(*App, tea.KeyPressMsg) (tea.Model, tea.Cmd)
}

// modals returns every modal surface in priority order: index 0 receives keys
// first and paints last, so it sits on top.
//
// This is the single source of an order that handleKeyPress, renderLayout and
// isDialogVisible each used to spell out by hand, in three different orders,
// across three files. Adding a surface here is the whole edit; app_mouse.go
// keeps its own cascade until phase 2.
//
// The order is handleKeyPress's, because that one was already load-bearing:
// the first visible surface wins the key. Paint simply runs it backwards. The
// two disagreed before this collapsed them, and the disagreements were only
// ever between surfaces that cannot be visible at once — see the
// co-occurrence tests in modal_order_test.go, which pin the pairs that can.
//
// corporateActionDetail is deliberately absent: it is a view-embedded panel,
// not a modal. See isDialogVisible.
func (a *App) modals() []modalEntry {
	return []modalEntry{
		{"help", helpOverlayModal{a}, (*App).handleHelpOverlayKey},
		{"confirm", a.confirmDialog, (*App).handleConfirmDialogKey},
		{"about", a.aboutDialog, (*App).handleAboutDialogKey},
		{"backup", a.backupDialog.Dialog(), (*App).handleBackupDialogKey},
		{"file", a.fileDialog, (*App).handleFileDialogKey},
		{"import", a.importDialog, (*App).handleImportDialogKey},
		{"linkTransfers", a.linkTransfersDialog, (*App).handleLinkTransfersDialogKey},
		{"split", a.splitDialog, (*App).handleSplitDialogKey},
		// createCat must outrank the eight surfaces it diverts from, so it
		// takes keys and paints above them. Split is the exception: it
		// outranks createCat, which is safe only because split hides itself
		// before diverting (split_dialog.go).
		{"createCategory", a.createCatDialog, (*App).handleCreateCatDialogKey},
		{"transaction", a.txnDialog, (*App).handleTransactionDialogKey},
		{"transfer", a.transferDialog, (*App).handleTransferDialogKey},
		{"scheduled", a.schedDialog, (*App).handleScheduledDialogKey},
		{"schedulePreview", a.schedPreviewDialog, (*App).handleSchedulePreviewDialogKey},
		{"paycheckWizard", a.paycheckWizard, (*App).handlePaycheckWizardKey},
		{"loanWizard", a.loanWizard, (*App).handleLoanWizardKey},
		{"account", a.acctDialog, (*App).handleAccountDialogKey},
		{"reconciliation", a.reconDialog, (*App).handleReconDialogKey},
		{"closeAccount", a.closeAcctDialog, (*App).handleCloseAcctDialogKey},
		{"security", a.securityDialog, (*App).handleSecurityDialogKey},
		{"price", a.priceDialog, (*App).handlePriceDialogKey},
		{"priceImport", a.priceImportDialog, (*App).handlePriceImportDialogKey},
		{"buy", a.buyDialog, (*App).handleBuyDialogKey},
		{"sell", a.sellDialog, (*App).handleSellDialogKey},
		{"feeLiquidation", a.feeLiquidationDialog, (*App).handleFeeLiquidationDialogKey},
		{"dividend", a.dividendDialog, (*App).handleDividendDialogKey},
		{"transferShares", a.transferSharesDialog, (*App).handleTransferSharesDialogKey},
		{"stockSplit", a.stockSplitDialog, (*App).handleStockSplitDialogKey},
		// The merger confirmation outranks the merger dialog that produced it,
		// which is safe either way: submitMergerDialog closes the dialog
		// before loading the confirmation.
		{"mergerConfirm", mergerConfirmModal{a}, (*App).handleMergerConfirmKey},
		{"merger", a.mergerDialog, (*App).handleMergerDialogKey},
		{"spinOff", a.spinOffDialog, (*App).handleSpinOffDialogKey},
		{"cashOperation", a.cashOperationDialog, (*App).handleCashOperationDialogKey},
		{"investmentTypeSelector", a.investmentTypeSelector, (*App).handleInvestmentTypeSelectorKey},
	}
}

// frontmostModal returns the highest-priority visible surface, or nil.
func (a *App) frontmostModal() *modalEntry {
	for _, e := range a.modals() {
		if e.modal.IsVisible() {
			return &e
		}
	}
	return nil
}

// paintModals overlays every visible surface onto layout, lowest priority
// first so the highest ends up on top.
func (a *App) paintModals(layout string) string {
	entries := a.modals()
	for i := len(entries) - 1; i >= 0; i-- {
		m := entries[i].modal
		if !m.IsVisible() {
			continue
		}
		// Bounding a base dialog to the screen makes a too-tall form (a Sell
		// with many lots) scroll rather than spill past the status bar. The
		// write is load-bearing beyond paint: HandleMouse computes geometry
		// through DialogBounds → RenderedHeight, which reads this same bound,
		// so a tall dialog's click coordinates are correct only because the
		// paint pass ran first. Only *dialog.Dialog is bounded, which is what
		// the hand-written cascade did.
		if b, ok := m.(interface{ SetMaxHeight(int) }); ok {
			b.SetMaxHeight(a.dialogMaxHeight())
		}
		layout = widget.OverlayCenter(layout, m.Render(a.styles), a.width, a.height)
	}
	return layout
}

// helpOverlayModal adapts the help overlay, whose visibility is a bare bool on
// App and whose body is a free function, to Modal.
type helpOverlayModal struct{ a *App }

func (h helpOverlayModal) IsVisible() bool { return h.a != nil && h.a.showHelp }

func (h helpOverlayModal) Render(styles widget.Styles) string {
	return renderHelpOverlay(styles, h.a.currentView, h.a.width, h.a.height)
}

// handleHelpOverlayKey closes the overlay on ? or Esc and swallows every other
// key. Extracted from handleKeyPress, which inlined it as the one arm of the
// cascade that was not already a method.
func (a *App) handleHelpOverlayKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, a.keys.Help) || key.Matches(msg, a.keys.Escape) {
		a.showHelp = false
	}
	return a, nil
}

// mergerConfirmModal adapts the merger confirmation, whose visibility is the
// presence of its loaded data rather than a flag, to Modal.
type mergerConfirmModal struct{ a *App }

func (m mergerConfirmModal) IsVisible() bool { return m.a != nil && m.a.mergerConfirmData != nil }

func (m mergerConfirmModal) Render(styles widget.Styles) string {
	return m.a.renderMergerConfirmation()
}

// Dialog returns the backup dialog's underlying base dialog, or nil when no
// backup dialog is open. Nil-safe so the registry can hold it directly rather
// than reaching through a possibly-nil state struct.
func (b *backupDialogState) Dialog() *dialog.Dialog {
	if b == nil {
		return nil
	}
	return b.dialog
}
