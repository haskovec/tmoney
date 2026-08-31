package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// mergerConfirmParams holds the validated merger parameters between dialog and confirmation.
type mergerConfirmParams struct {
	sourceSecurityID types.ID
	targetSecurityID types.ID
	mergerDate       types.Date
	exchangeRatio    float64
	cashPerShare     float64
}

// mergerAffectedAccount holds information about an account affected by the merger.
type mergerAffectedAccount struct {
	accountID   types.ID
	accountName string
	trackLots   bool
	lots        []*investment.Lot    // populated for lot-tracking accounts
	position    *investment.Position // populated for non-lot-tracking accounts
}

// mergerConfirmData holds the loaded data for the merger confirmation overlay.
type mergerConfirmData struct {
	sourceTicker  string
	targetTicker  string
	exchangeRatio float64
	cashPerShare  float64
	date          string
	accounts      []mergerAffectedAccount
}

// mergerConfirmDataMsg is sent when merger confirmation data has been loaded.
type mergerConfirmDataMsg struct {
	data *mergerConfirmData
}

// loadMergerConfirmData returns a command that loads affected accounts, positions, and lots
// for the source security to display in the merger confirmation overlay.
func (a *App) loadMergerConfirmData() tea.Cmd {
	params := a.mergerConfirmParams
	if params == nil {
		return nil
	}

	sourceID := params.sourceSecurityID
	targetID := params.targetSecurityID
	exchangeRatio := params.exchangeRatio
	cashPerShare := params.cashPerShare
	mergerDate := params.mergerDate

	return func() tea.Msg {
		data := &mergerConfirmData{
			exchangeRatio: exchangeRatio,
			cashPerShare:  cashPerShare,
			date:          mergerDate.Time().Format("01/02/2006"),
		}

		// Resolve source and target tickers
		if a.securitySvc != nil {
			if src, err := a.securitySvc.GetByID(sourceID); err == nil {
				data.sourceTicker = src.Ticker
			}
			if tgt, err := a.securitySvc.GetByID(targetID); err == nil {
				data.targetTicker = tgt.Ticker
			}
		}

		// Load open lots for the source security (lot-tracking accounts)
		var lots []*investment.Lot
		if a.lotRepo != nil {
			var err error
			lots, err = a.lotRepo.GetOpenLotsBySecurity(sourceID)
			if err != nil {
				return errMsg{err: fmt.Errorf("failed to load lots: %w", err)}
			}
		}

		// Load positions for the source security (non-lot-tracking accounts)
		var positions []*investment.Position
		if a.positionRepo != nil {
			var err error
			positions, err = a.positionRepo.GetPositionsBySecurity(sourceID)
			if err != nil {
				return errMsg{err: fmt.Errorf("failed to load positions: %w", err)}
			}
		}

		// Group lots by account
		lotsByAccount := make(map[types.ID][]*investment.Lot)
		for _, lot := range lots {
			lotsByAccount[lot.AccountID] = append(lotsByAccount[lot.AccountID], lot)
		}

		// Build affected accounts list
		seen := make(map[types.ID]bool)

		// Lot-tracking accounts
		for acctID, acctLots := range lotsByAccount {
			acctName := resolveAccountName(a, acctID)
			data.accounts = append(data.accounts, mergerAffectedAccount{
				accountID:   acctID,
				accountName: acctName,
				trackLots:   true,
				lots:        acctLots,
			})
			seen[acctID] = true
		}

		// Non-lot-tracking accounts (positions not already covered by lots)
		for _, pos := range positions {
			if seen[pos.AccountID] {
				continue
			}
			acctName := resolveAccountName(a, pos.AccountID)
			data.accounts = append(data.accounts, mergerAffectedAccount{
				accountID:   pos.AccountID,
				accountName: acctName,
				trackLots:   false,
				position:    pos,
			})
		}

		return mergerConfirmDataMsg{data: data}
	}
}

// resolveAccountName looks up an account name by ID, returning a fallback if not found.
func resolveAccountName(a *App, accountID types.ID) string {
	if a.accountSvc != nil {
		if acct, err := a.accountSvc.GetByID(accountID); err == nil {
			return acct.Name
		}
	}
	return accountID.String()[:8] + "..."
}

// closeMergerConfirmation clears the merger confirmation state.
func (a *App) closeMergerConfirmation() {
	a.mergerConfirmData = nil
	a.mergerConfirmParams = nil
}

// handleMergerConfirmKey handles key events in the merger confirmation overlay.
func (a *App) handleMergerConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.mergerConfirmData == nil {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Escape):
		a.closeMergerConfirmation()
		return a, nil

	case msg.String() == "enter" || msg.String() == "y":
		return a.executeMerger()
	}

	return a, nil
}

// mergerConfirmButtonLabels are the overlay's action buttons in render order.
// Shared by renderMergerConfirmation and the hit test so the two cannot drift.
var mergerConfirmButtonLabels = []string{"Cancel", "Merge"}

// mergerConfirmMouseAction maps a screen click to the overlay's interactive
// elements: the title row's [x] and the two-button row at the bottom.
// Everything else — the summary body, the affected-account block, the
// separators, the keyboard hint, the border and padding ring, the gaps between
// buttons — and everything outside the panel is inert. That mirrors
// Dialog.HandleMouse, which returns no action for an out-of-bounds click; no
// dialog in this app dismisses on a click outside.
//
// The overlay is re-rendered to measure it, the way helpOverlayCloseHit does,
// so the geometry is read off exactly what app_view composited.
func (a *App) mergerConfirmMouseAction(x, y int) dialog.DialogAction {
	if a.mergerConfirmData == nil {
		return dialog.DialogActionNone
	}
	overlay := a.renderMergerConfirmation()
	if overlay == "" {
		return dialog.DialogActionNone
	}
	lines := strings.Split(overlay, "\n")
	overlayWidth := 0
	for _, ln := range lines {
		if w := lipgloss.Width(ln); w > overlayWidth {
			overlayWidth = w
		}
	}
	if overlayWidth == 0 {
		return dialog.DialogActionNone
	}
	startCol, startRow := widget.OverlayTopLeft(overlay, a.width, a.height)

	// Content-local offsets inside the panel. OverlayBox is a rounded border
	// plus Padding(1, 2), so border (1) + h-padding (2) on X and border (1) +
	// v-padding (1) on Y.
	localX := x - startCol - 3
	localY := y - startRow - 2
	contentWidth := max(overlayWidth-dialog.DialogHorizontalOverhead, 10)
	if localX < 0 || localX >= contentWidth || localY < 0 {
		return dialog.DialogActionNone
	}

	// Title row: the [x] occupies the last three content columns.
	if localY == 0 {
		if localX >= contentWidth-3 {
			return dialog.DialogActionCancel
		}
		return dialog.DialogActionNone
	}

	// The button row is the last content line. A render is border,
	// padding-top, content..., padding-bottom, border — so the final content
	// row is content-local len(lines)-5.
	if localY == len(lines)-5 {
		switch dialog.ButtonRowHitTest(mergerConfirmButtonLabels, localX, contentWidth) {
		case 0:
			return dialog.DialogActionCancel
		case 1:
			return dialog.DialogActionSubmit
		}
	}

	return dialog.DialogActionNone
}

// handleMergerConfirmMouse routes a mouse event through the merger
// confirmation overlay, mirroring handleMergerConfirmKey: the [x] and Cancel
// close it without touching data, Merge runs the merger. Wheel events reach
// here too (handleMouseWheel routes through handleDialogMouse) and are
// ignored — the overlay has no scroll surface.
func (a *App) handleMergerConfirmMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if a.mergerConfirmData == nil {
		return a, nil
	}
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft {
		return a, nil
	}
	m := msg.Mouse()
	switch a.mergerConfirmMouseAction(m.X, m.Y) {
	case dialog.DialogActionSubmit:
		return a.executeMerger()
	case dialog.DialogActionCancel:
		a.closeMergerConfirmation()
	}
	return a, nil
}

// executeMerger executes the merger using the stored confirmation parameters.
func (a *App) executeMerger() (tea.Model, tea.Cmd) {
	params := a.mergerConfirmParams
	if params == nil {
		return a, nil
	}

	sourceID := params.sourceSecurityID
	targetID := params.targetSecurityID
	mergerDate := params.mergerDate
	mergerParams := investment.MergerParams{
		ExchangeRatio: params.exchangeRatio,
		CashPerShare:  params.cashPerShare,
	}

	a.closeMergerConfirmation()

	return a, func() tea.Msg {
		if a.corporateActionSvc == nil {
			return errMsg{err: fmt.Errorf("corporate action service not available")}
		}

		_, err := a.corporateActionSvc.Merger(sourceID, targetID, mergerDate, mergerParams)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to execute merger: %w", err)}
		}

		return mergerDialogSavedMsg{savedDate: mergerDate}
	}
}

// renderMergerConfirmation renders the merger confirmation overlay.
func (a *App) renderMergerConfirmation() string {
	if a.mergerConfirmData == nil {
		return ""
	}

	data := a.mergerConfirmData
	overlayWidth := max(min(a.width-8, 70), 30)
	// Content width inside OverlayBox's border (1 each side) and padding
	// (2 each side) — the same overhead a dialog panel has. The previous
	// overlayWidth-4 was two columns too wide, so every separator wrapped
	// onto a stub second row.
	innerWidth := max(overlayWidth-dialog.DialogHorizontalOverhead, 10)

	var sections []string

	// Title row, with the [x] close button right-aligned — the same three
	// lines the split editor and the help overlay use.
	title := a.styles.Title.Render("Confirm Merger")
	closeBtn := a.styles.Muted.Render("[x]")
	titleGap := max(innerWidth-lipgloss.Width(title)-lipgloss.Width(closeBtn), 1)
	sections = append(sections, title+strings.Repeat(" ", titleGap)+closeBtn)

	// Separator
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", innerWidth)))

	// Merger summary
	sections = append(sections, fmt.Sprintf("  Source:  %s", data.sourceTicker))
	sections = append(sections, fmt.Sprintf("  Target:  %s", data.targetTicker))
	sections = append(sections, fmt.Sprintf("  Date:    %s", data.date))
	sections = append(sections, fmt.Sprintf("  Ratio:   %.4f", data.exchangeRatio))
	if data.cashPerShare > 0 {
		sections = append(sections, fmt.Sprintf("  Cash:    $%.2f/share", data.cashPerShare))
	}

	// Separator
	sections = append(sections, "")
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", innerWidth)))

	// Affected accounts
	if len(data.accounts) == 0 {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No accounts hold this security"))
	} else {
		sections = append(sections, a.styles.Title.Render(fmt.Sprintf("  Affected Accounts (%d)", len(data.accounts))))
		for _, acct := range data.accounts {
			sections = append(sections, "")
			sections = append(sections, fmt.Sprintf("  %s", acct.accountName))
			if acct.trackLots && len(acct.lots) > 0 {
				for _, lot := range acct.lots {
					sections = append(sections, fmt.Sprintf("    Lot %s: %s shares @ $%s",
						lot.PurchaseDate.Time().Format("2006-01-02"),
						lot.Shares.String(),
						lot.CostPerShare.String(),
					))
				}
			} else if !acct.trackLots && acct.position != nil {
				sections = append(sections, fmt.Sprintf("    Position: %s shares @ $%s avg",
					acct.position.Shares.String(),
					acct.position.AverageCostPerShare.String(),
				))
			}
		}
	}

	// Separator
	sections = append(sections, "")
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", innerWidth)))

	// Hint, then the action buttons. The hint stays: it is the only place
	// the y accelerator and the esc path are documented, and buttons show
	// neither. Cancel comes first and Merge second, mirroring the No/Yes
	// order showConfirmDialog uses for a destructive choice.
	//
	// Neither button renders focused. This overlay keeps no focus state and
	// Enter always confirms, so a highlighted Cancel would imply Enter
	// activates it — an actively dangerous lie.
	sections = append(sections, a.styles.Muted.Render("  enter/y confirm  esc cancel"))
	// The button row must stay the LAST content line: the hit test locates it
	// from the rendered height rather than by counting sections, because the
	// body grows with the account and lot count, and on a narrow terminal the
	// hint itself wraps to two rows.
	sections = append(sections, dialog.RenderButtonRow(a.styles, []dialog.ButtonSpec{
		{Label: "Cancel"},
		{Label: "Merge"},
	}, innerWidth))

	content := strings.Join(sections, "\n")

	return a.styles.OverlayBox.Width(overlayWidth).Render(content)
}
