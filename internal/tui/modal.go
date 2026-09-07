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
// Add a member when a walk needs it, not before. Mouse input is not a member
// either: a surface is asked through modalMouseAction (app_mouse.go) and the
// result goes to the entry's onAction, or the entry declares its own onMouse.
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
	// one. It is the existing handleXKey method, which calls the surface's
	// HandleKey and hands the result to onAction.
	onKey func(*App, tea.KeyPressMsg) (tea.Model, tea.Cmd)

	// onAction dispatches a DialogAction, and BOTH input paths call it.
	// specs/tui.md:687-695 requires that clicking a dialog button be exactly
	// equivalent to the keyboard action; one dispatcher per surface makes that
	// true by construction rather than by keeping two switches in step. The
	// two switches had in fact drifted — see the phase 2 notes in the design.
	onAction func(*App, dialog.DialogAction) (tea.Model, tea.Cmd)

	// onMouse replaces the default "HandleMouse then onAction" walk. Only the
	// surfaces that need it declare one, and each says why at its entry.
	onMouse func(*App, tea.MouseMsg) (tea.Model, tea.Cmd)
}

// modals returns every modal surface in priority order: index 0 receives keys
// first and paints last, so it sits on top.
//
// This is the single source of an order that handleKeyPress, renderLayout and
// isDialogVisible each used to spell out by hand, in three different orders,
// across three files, and that handleDialogMouse walked a fourth way. Adding a
// surface here is the whole edit.
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
		// The help overlay has one clickable target, its [x] close box, so its
		// mouse handling is a hit test rather than a DialogAction.
		{
			name:    "help",
			modal:   helpOverlayModal{a},
			onKey:   (*App).handleHelpOverlayKey,
			onMouse: (*App).handleHelpOverlayMouse,
		},
		{
			name:     "confirm",
			modal:    a.confirmDialog,
			onKey:    (*App).handleConfirmDialogKey,
			onAction: (*App).confirmDialogAction,
		},
		{
			name:     "about",
			modal:    a.aboutDialog,
			onKey:    (*App).handleAboutDialogKey,
			onAction: (*App).aboutDialogAction,
		},
		{
			name:     "backup",
			modal:    a.backupDialog.Dialog(),
			onKey:    (*App).handleBackupDialogKey,
			onAction: (*App).backupDialogAction,
		},
		// The file dialog adds a double-click-to-activate step in browse mode
		// before the ordinary action dispatch.
		{
			name:     "file",
			modal:    a.fileDialog,
			onKey:    (*App).handleFileDialogKey,
			onAction: (*App).fileDialogAction,
			onMouse:  (*App).handleFileDialogMouse,
		},
		{
			name:     "import",
			modal:    a.importDialog,
			onKey:    (*App).handleImportDialogKey,
			onAction: (*App).importDialogAction,
		},
		{
			name:     "linkTransfers",
			modal:    a.linkTransfersDialog,
			onKey:    (*App).handleLinkTransfersDialogKey,
			onAction: (*App).linkTransfersDialogAction,
		},
		// The split editor's rows are not dialog.Field rows, so DialogBounds cannot
		// map a click to them. Its own hit test (HandleMouseLocal) does, and the
		// override feeds the result through the same dispatcher the keyboard uses.
		{
			name:     "split",
			modal:    a.splitDialog,
			onKey:    (*App).handleSplitDialogKey,
			onAction: (*App).splitDialogAction,
			onMouse:  (*App).handleSplitDialogMouse,
		},
		// createCat must outrank the eight surfaces it diverts from, so it takes
		// keys and paints above them. Split is the exception: it outranks
		// createCat, which is safe only because split hides itself on divert.
		{
			name:     "createCategory",
			modal:    a.createCatDialog,
			onKey:    (*App).handleCreateCatDialogKey,
			onAction: (*App).createCatDialogAction,
		},
		{
			name:     "transaction",
			modal:    a.txnDialog,
			onKey:    (*App).handleTransactionDialogKey,
			onAction: (*App).transactionDialogAction,
		},
		{
			name:     "transfer",
			modal:    a.transferDialog,
			onKey:    (*App).handleTransferDialogKey,
			onAction: (*App).transferDialogAction,
		},
		{
			name:     "scheduled",
			modal:    a.schedDialog,
			onKey:    (*App).handleScheduledDialogKey,
			onAction: (*App).scheduledDialogAction,
		},
		// The preview stacks a header dialog over an embedded split editor and
		// routes to whichever panel the click lands in, so the mouse path maps
		// the click itself. Both panels then dispatch through the same action
		// functions the keyboard uses (schedulePreviewAction for the header).
		{
			name:     "schedulePreview",
			modal:    a.schedPreviewDialog,
			onKey:    (*App).handleSchedulePreviewDialogKey,
			onAction: (*App).schedulePreviewAction,
			onMouse:  (*App).handleSchedulePreviewMouse,
		},
		{
			name:     "paycheckWizard",
			modal:    a.paycheckWizard,
			onKey:    (*App).handlePaycheckWizardKey,
			onAction: (*App).paycheckWizardAction,
		},
		{
			name:     "loanWizard",
			modal:    a.loanWizard,
			onKey:    (*App).handleLoanWizardKey,
			onAction: (*App).loanWizardAction,
		},
		{
			name:     "account",
			modal:    a.acctDialog,
			onKey:    (*App).handleAccountDialogKey,
			onAction: (*App).accountDialogAction,
		},
		{
			name:     "reconciliation",
			modal:    a.reconDialog,
			onKey:    (*App).handleReconDialogKey,
			onAction: (*App).reconDialogAction,
		},
		{
			name:     "closeAccount",
			modal:    a.closeAcctDialog,
			onKey:    (*App).handleCloseAcctDialogKey,
			onAction: (*App).closeAcctDialogAction,
		},
		{
			name:     "security",
			modal:    a.securityDialog,
			onKey:    (*App).handleSecurityDialogKey,
			onAction: (*App).securityDialogAction,
		},
		{
			name:     "price",
			modal:    a.priceDialog,
			onKey:    (*App).handlePriceDialogKey,
			onAction: (*App).priceDialogAction,
		},
		{
			name:     "priceImport",
			modal:    a.priceImportDialog,
			onKey:    (*App).handlePriceImportDialogKey,
			onAction: (*App).priceImportDialogAction,
		},
		{
			name:     "buy",
			modal:    a.buyDialog,
			onKey:    (*App).handleBuyDialogKey,
			onAction: (*App).buyDialogAction,
		},
		{
			name:     "sell",
			modal:    a.sellDialog,
			onKey:    (*App).handleSellDialogKey,
			onAction: (*App).sellDialogAction,
		},
		{
			name:     "feeLiquidation",
			modal:    a.feeLiquidationDialog,
			onKey:    (*App).handleFeeLiquidationDialogKey,
			onAction: (*App).feeLiquidationDialogAction,
		},
		{
			name:     "dividend",
			modal:    a.dividendDialog,
			onKey:    (*App).handleDividendDialogKey,
			onAction: (*App).dividendDialogAction,
		},
		{
			name:     "transferShares",
			modal:    a.transferSharesDialog,
			onKey:    (*App).handleTransferSharesDialogKey,
			onAction: (*App).transferSharesDialogAction,
		},
		{
			name:     "stockSplit",
			modal:    a.stockSplitDialog,
			onKey:    (*App).handleStockSplitDialogKey,
			onAction: (*App).stockSplitDialogAction,
		},
		// The merger confirmation outranks the merger dialog that produced it,
		// which is safe either way: submitMergerDialog closes the dialog before
		// loading the confirmation. It is a rendered y/n overlay rather than a
		// dialog.Dialog, so its [x], Cancel and Merge targets are hit-tested by
		// its own handler.
		{
			name:    "mergerConfirm",
			modal:   mergerConfirmModal{a},
			onKey:   (*App).handleMergerConfirmKey,
			onMouse: (*App).handleMergerConfirmMouse,
		},
		{
			name:     "merger",
			modal:    a.mergerDialog,
			onKey:    (*App).handleMergerDialogKey,
			onAction: (*App).mergerDialogAction,
		},
		{
			name:     "spinOff",
			modal:    a.spinOffDialog,
			onKey:    (*App).handleSpinOffDialogKey,
			onAction: (*App).spinOffDialogAction,
		},
		{
			name:     "cashOperation",
			modal:    a.cashOperationDialog,
			onKey:    (*App).handleCashOperationDialogKey,
			onAction: (*App).cashOperationDialogAction,
		},
		{
			name:     "investmentTypeSelector",
			modal:    a.investmentTypeSelector,
			onKey:    (*App).handleInvestmentTypeSelectorKey,
			onAction: (*App).investmentTypeSelectorAction,
		},
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

// handleHelpOverlayMouse closes the overlay on a left click in its [x] box and
// swallows every other click. The overlay is a rendered string, not a dialog,
// so there is no DialogAction to dispatch.
func (a *App) handleHelpOverlayMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft {
		return a, nil
	}
	m := msg.Mouse()
	if helpOverlayCloseHit(a.styles, a.currentView, a.width, a.height, m.X, m.Y) {
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
