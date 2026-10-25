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

// modalSurface is embedded by every per-surface state struct. It holds the
// dialog handle and, with it, all of Modal except IsVisible, so a surface
// declares only the form data it owns.
//
// IsVisible is deliberately not here. A promoted method cannot nil-check the
// outer pointer, and the registry holds nil surfaces most of the time, so every
// embedder declares its own:
//
//	func (s *sellSurface) IsVisible() bool { return s != nil && s.dlg.IsVisible() }
//
// Omitting it is a compile error (the type stops implementing Modal); omitting
// the nil check inside it is caught by TestGuard_EverySurfaceIsVisibleIsNilSafe.
// The other methods promote safely because every walk gates on IsVisible first.
type modalSurface struct {
	dlg *dialog.Dialog
}

func (m *modalSurface) Render(styles widget.Styles) string { return m.dlg.Render(styles) }

func (m *modalSurface) SetMaxHeight(h int) { m.dlg.SetMaxHeight(h) }

func (m *modalSurface) SetVisible(v bool) { m.dlg.SetVisible(v) }

// Dialog returns the underlying base dialog. Tests use it to place a click.
func (m *modalSurface) Dialog() *dialog.Dialog { return m.dlg }

func (m *modalSurface) HandleMouse(msg tea.MouseMsg, w, h int) dialog.DialogAction {
	return m.dlg.HandleMouse(msg, w, h)
}

// mouseTarget is a surface whose hit-testing needs only the screen size.
// *dialog.Dialog and every embedder of modalSurface satisfy it; the paycheck
// wizard does not, because its layout is style-dependent.
type mouseTarget interface {
	HandleMouse(msg tea.MouseMsg, screenWidth, screenHeight int) dialog.DialogAction
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

	// close shuts the surface with no save. switchDatabase calls it on every
	// surface that has one, hidden or not: each acts on the open file, and its
	// IDs, its data and its Save belong to the file that the switch leaves.
	// Nil means that the surface shows nothing of a file (help, about), and a
	// switch leaves it open.
	close func(*App)
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
// corporateActions.detail is deliberately absent: it is a view-embedded
// panel, not a modal. See isDialogVisible.
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
			modal:    &a.confirm,
			onKey:    (*App).handleConfirmDialogKey,
			onAction: (*App).confirmDialogAction,
			close:    func(a *App) { a.confirm = confirmSurface{} },
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
			close:    func(a *App) { a.backupDialog = nil },
		},
		// The file dialog adds a double-click-to-activate step in browse mode
		// before the ordinary action dispatch.
		{
			name:     "file",
			modal:    &a.file,
			onKey:    (*App).handleFileDialogKey,
			onAction: (*App).fileDialogAction,
			onMouse:  (*App).handleFileDialogMouse,
			close:    (*App).closeFileDialog,
		},
		{
			name:     "import",
			modal:    &a.importer,
			onKey:    (*App).handleImportDialogKey,
			onAction: (*App).importDialogAction,
			close:    (*App).closeImportDialog,
		},
		{
			name:     "linkTransfers",
			modal:    &a.linkTransfers,
			onKey:    (*App).handleLinkTransfersDialogKey,
			onAction: (*App).linkTransfersDialogAction,
			close:    (*App).closeLinkTransfersDialog,
		},
		// The split editor's rows are not dialog.Field rows, so DialogBounds cannot
		// map a click to them. Its own hit test (HandleMouseLocal) does, and the
		// override feeds the result through the same dispatcher the keyboard uses.
		{
			name:     "split",
			modal:    &a.split,
			onKey:    (*App).handleSplitDialogKey,
			onAction: (*App).splitDialogAction,
			onMouse:  (*App).handleSplitDialogMouse,
			close:    func(a *App) { a.split.close() },
		},
		// createCat must outrank the eight surfaces it diverts from, so it takes
		// keys and paints above them. Split is the exception: it outranks
		// createCat, which is safe only because split hides itself on divert.
		{
			name:     "createCategory",
			modal:    &a.createCat,
			onKey:    (*App).handleCreateCatDialogKey,
			onAction: (*App).createCatDialogAction,
			close:    func(a *App) { a.createCat.close() },
		},
		{
			name:     "transaction",
			modal:    &a.txn,
			onKey:    (*App).handleTransactionDialogKey,
			onAction: (*App).transactionDialogAction,
			close:    (*App).closeTransactionDialog,
		},
		{
			name:     "transfer",
			modal:    &a.transfer,
			onKey:    (*App).handleTransferDialogKey,
			onAction: (*App).transferDialogAction,
			close:    func(a *App) { a.transfer.close() },
		},
		{
			name:     "scheduled",
			modal:    &a.sched,
			onKey:    (*App).handleScheduledDialogKey,
			onAction: (*App).scheduledDialogAction,
			close:    (*App).closeScheduledDialog,
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
			close:    (*App).closeSchedulePreviewDialog,
		},
		{
			name:     "paycheckWizard",
			modal:    &a.paycheck,
			onKey:    (*App).handlePaycheckWizardKey,
			onAction: (*App).paycheckWizardAction,
			close:    func(a *App) { a.paycheck.close() },
		},
		{
			name:     "loanWizard",
			modal:    &a.loan,
			onKey:    (*App).handleLoanWizardKey,
			onAction: (*App).loanWizardAction,
			close:    func(a *App) { a.loan.close() },
		},
		{
			name:     "account",
			modal:    &a.acct,
			onKey:    (*App).handleAccountDialogKey,
			onAction: (*App).accountDialogAction,
			close:    (*App).closeAccountDialog,
		},
		{
			name:     "reconciliation",
			modal:    a.reconDialog,
			onKey:    (*App).handleReconDialogKey,
			onAction: (*App).reconDialogAction,
			close:    func(a *App) { a.reconDialog = nil },
		},
		{
			name:     "closeAccount",
			modal:    &a.closeAcct,
			onKey:    (*App).handleCloseAcctDialogKey,
			onAction: (*App).closeAcctDialogAction,
			close:    func(a *App) { a.closeAcct = closeAcctSurface{} },
		},
		{
			name:     "security",
			modal:    &a.security,
			onKey:    (*App).handleSecurityDialogKey,
			onAction: (*App).securityDialogAction,
			close:    func(a *App) { a.security = securitySurface{} },
		},
		{
			name:     "price",
			modal:    &a.price,
			onKey:    (*App).handlePriceDialogKey,
			onAction: (*App).priceDialogAction,
			close:    func(a *App) { a.price = priceSurface{} },
		},
		{
			name:     "priceImport",
			modal:    a.priceImportDialog,
			onKey:    (*App).handlePriceImportDialogKey,
			onAction: (*App).priceImportDialogAction,
			close:    func(a *App) { a.priceImportDialog = nil },
		},
		{
			name:     "buy",
			modal:    &a.buy,
			onKey:    (*App).handleBuyDialogKey,
			onAction: (*App).buyDialogAction,
			close:    (*App).closeBuyDialog,
		},
		{
			name:     "sell",
			modal:    &a.sell,
			onKey:    (*App).handleSellDialogKey,
			onAction: (*App).sellDialogAction,
			close:    (*App).closeSellDialog,
		},
		{
			name:     "feeLiquidation",
			modal:    &a.feeLiquidation,
			onKey:    (*App).handleFeeLiquidationDialogKey,
			onAction: (*App).feeLiquidationDialogAction,
			close:    (*App).closeFeeLiquidationDialog,
		},
		{
			name:     "dividend",
			modal:    &a.dividend,
			onKey:    (*App).handleDividendDialogKey,
			onAction: (*App).dividendDialogAction,
			close:    (*App).closeDividendDialog,
		},
		{
			name:     "transferShares",
			modal:    &a.transferShares,
			onKey:    (*App).handleTransferSharesDialogKey,
			onAction: (*App).transferSharesDialogAction,
			close:    (*App).closeTransferSharesDialog,
		},
		{
			name:     "stockSplit",
			modal:    &a.stockSplit,
			onKey:    (*App).handleStockSplitDialogKey,
			onAction: (*App).stockSplitDialogAction,
			close:    (*App).closeStockSplitDialog,
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
			close:   (*App).closeMergerConfirmation,
		},
		{
			name:     "merger",
			modal:    &a.merger,
			onKey:    (*App).handleMergerDialogKey,
			onAction: (*App).mergerDialogAction,
			close:    (*App).closeMergerDialog,
		},
		{
			name:     "spinOff",
			modal:    &a.spinOff,
			onKey:    (*App).handleSpinOffDialogKey,
			onAction: (*App).spinOffDialogAction,
			close:    (*App).closeSpinOffDialog,
		},
		{
			name:     "cashOperation",
			modal:    &a.cashOperation,
			onKey:    (*App).handleCashOperationDialogKey,
			onAction: (*App).cashOperationDialogAction,
			close:    (*App).closeCashOperationDialog,
		},
		{
			name:     "investmentTypeSelector",
			modal:    a.investmentTypeSelector,
			onKey:    (*App).handleInvestmentTypeSelectorKey,
			onAction: (*App).investmentTypeSelectorAction,
			close:    func(a *App) { a.investmentTypeSelector = nil },
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

func (m mergerConfirmModal) IsVisible() bool { return m.a != nil && m.a.mergerConfirm.data != nil }

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
