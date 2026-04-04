package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/haskovec/tmoney/internal/investment"
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
	lots        []*investment.Lot     // populated for lot-tracking accounts
	position    *investment.Position  // populated for non-lot-tracking accounts
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
func (a *App) handleMergerConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.mergerConfirmData == nil {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Escape):
		a.closeMergerConfirmation()
		return a, nil

	case msg.Type == tea.KeyEnter || msg.String() == "y":
		return a.executeMerger()
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

		return mergerDialogSavedMsg{}
	}
}

// renderMergerConfirmation renders the merger confirmation overlay.
func (a *App) renderMergerConfirmation() string {
	if a.mergerConfirmData == nil {
		return ""
	}

	data := a.mergerConfirmData
	overlayWidth := max(min(a.width-8, 70), 30)
	innerWidth := overlayWidth - 4

	var sections []string

	// Title
	sections = append(sections, a.styles.Title.Render("Confirm Merger"))

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

	// Hint
	sections = append(sections, a.styles.Muted.Render("  enter/y confirm  esc cancel"))

	content := strings.Join(sections, "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(1, 2).
		Width(overlayWidth)

	return boxStyle.Render(content)
}
