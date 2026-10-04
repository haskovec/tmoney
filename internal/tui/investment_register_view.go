package tui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// investmentRegisterViewState is everything the investment register view
// owns. Its zero value is the view before its first load, with no filter.
type investmentRegisterViewState struct {
	data  *investmentRegisterData
	table *widget.Table

	// The security filter (the `/` key). While filterSearching is true the
	// user is typing a substring query that live-narrows the register by
	// security ticker/name; pressing Enter on a query matching exactly one
	// security locks the filter (searching=false, query cleared) to
	// filterLockedSec. NilID means no security is locked; the filter is active
	// when either searching or a security is locked. Cleared when the user
	// leaves the register (the view's leave hook) or presses Esc.
	filterSearching bool
	filterQuery     string
	filterLockedSec types.ID

	// After a save+reload, the table build step moves the cursor onto the row
	// whose transaction ID matches, so a saved row stays under the cursor even
	// when it sorts into the middle of the list. The save paths set it; NilID
	// means "no pending selection"; the build step clears it after applying.
	pendingSelectID types.ID
}

// investmentRegisterData holds the loaded data for the investment account register view.
type investmentRegisterData struct {
	account       *account.Account
	transactions  []*investment.Transaction
	securityNames map[types.ID]string // SecurityID -> Ticker (or name when tickerless)
	// securityFullNames maps SecurityID -> the security's full name. Kept
	// alongside securityNames so the security filter can match on both the
	// ticker and the name, and so the active-filter line can show
	// "TICKER — Full Name" (degrading to name-only for tickerless holdings).
	securityFullNames map[types.ID]string
	cashBalance       types.Money
	valuation         *investment.AccountValuation
}

// investmentRegisterLoadedMsg is sent when investment register data has been loaded.
type investmentRegisterLoadedMsg struct {
	data *investmentRegisterData
}

// investmentRegisterDeps is what the investment register view needs from
// outside itself. Every dep is a func, because switchDatabase replaces App's
// services and closes the previous *db.DB; and deps are passed to each call,
// never stored in the view state. Both rules are pinned by the guards that run
// over viewControllers. config is the user's config, for the valuation
// options; like the services, it is read when the load runs.
type investmentRegisterDeps struct {
	accounts    func() *account.Service
	investments func() *investment.Service
	valuations  func() *investment.ValuationService
	securities  func() *security.Service
	config      func() *config.Config
}

// investmentRegisterDeps binds the investment register view to the services
// App owns. Every accessor may return nil, because an App built by a test has
// no services, so each caller keeps its own nil guard.
func (a *App) investmentRegisterDeps() investmentRegisterDeps {
	return investmentRegisterDeps{
		accounts:    func() *account.Service { return a.services.Account },
		investments: func() *investment.Service { return a.services.Investment },
		valuations:  func() *investment.ValuationService { return a.services.InvestmentValuation },
		securities:  func() *security.Service { return a.services.Security },
		config:      func() *config.Config { return a.cfg },
	}
}

// load returns a command that loads all data needed for the investment register view.
func (s *investmentRegisterViewState) load(d investmentRegisterDeps, accountID types.ID) tea.Cmd {
	return func() tea.Msg {
		data := &investmentRegisterData{
			securityNames:     make(map[types.ID]string),
			securityFullNames: make(map[types.ID]string),
		}

		// Load account
		if accounts := d.accounts(); accounts != nil {
			acct, err := accounts.GetByID(accountID)
			if err != nil {
				return errMsg{err: err}
			}
			data.account = acct
		}

		// Load the transactions and the valuation through the read model
		if valuations := d.valuations(); valuations != nil {
			txns, err := valuations.ListTransactions(accountID, investment.TransactionFilter{})
			if err != nil {
				return errMsg{err: err}
			}
			data.transactions = txns

			val, err := valuations.GetAccountValuation(accountID, types.Today(), valuationOptionsFor(d.config()))
			if err != nil {
				return errMsg{err: err}
			}
			data.valuation = val
		}

		// Load cash balance via service
		if investments := d.investments(); investments != nil {
			cash, err := investments.GetCashBalance(accountID)
			if err != nil {
				return errMsg{err: err}
			}
			data.cashBalance = cash
		}

		// Load security names for display
		if secSvc := d.securities(); secSvc != nil {
			securities, err := secSvc.List(security.Filter{})
			if err == nil {
				for _, sec := range securities {
					data.securityNames[sec.ID] = securityLabel(sec)
					data.securityFullNames[sec.ID] = sec.Name
				}
			}
		}

		return investmentRegisterLoadedMsg{data: data}
	}
}

// selectedTransaction returns the currently selected investment transaction based on table cursor.
func (s *investmentRegisterViewState) selectedTransaction() *investment.Transaction {
	if s.data == nil || s.table == nil {
		return nil
	}

	txns := s.visibleTransactions()
	cursor := s.table.Cursor()
	if cursor < 0 || cursor >= len(txns) {
		return nil
	}
	return txns[cursor]
}

// handleInvestmentRegisterKeys handles key presses in the investment register view.
func (a *App) handleInvestmentRegisterKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// While typing a security filter query, every key drives the filter
	// (see handleSearchKey). handleKeyPress routes here
	// with an early guard so global bindings don't steal keystrokes.
	if a.investmentRegister.filterSearching {
		a.investmentRegister.handleSearchKey(msg, a.keys, a.styles, a.height)
		return a, nil
	}

	// Esc clears a locked filter regardless of which pane has focus. This must
	// precede the sidebar delegation below, otherwise a locked filter with the
	// sidebar focused would swallow Esc (handleSidebarKeys has no Esc branch)
	// instead of clearing. Reached via the global Esc exception in handleKeyPress.
	if key.Matches(msg, a.keys.Escape) && a.investmentRegister.filterActive() {
		a.investmentRegister.resetFilter()
		a.investmentRegister.buildTable(a.styles)
		return a, nil
	}

	// Handle Tab to switch focus between sidebar and table
	if key.Matches(msg, a.keys.Tab) || key.Matches(msg, a.keys.ShiftTab) {
		if a.sidebar.IsFocused() {
			a.sidebar.SetFocused(false)
			if a.investmentRegister.table != nil {
				a.investmentRegister.table.SetFocused(true)
			}
		} else {
			a.sidebar.SetFocused(true)
			if a.investmentRegister.table != nil {
				a.investmentRegister.table.SetFocused(false)
			}
		}
		return a, nil
	}

	// If sidebar has focus, delegate to sidebar handling
	if a.sidebar.IsFocused() {
		return a.handleSidebarKeys(msg)
	}

	// widget.Table-focused key handling
	if a.investmentRegister.table == nil || a.investmentRegister.data == nil {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Up):
		a.investmentRegister.table.MoveUp()
	case key.Matches(msg, a.keys.Down):
		a.investmentRegister.table.MoveDown()
	case msg.String() == "home" || msg.String() == "g":
		a.investmentRegister.table.MoveToTop()
	case msg.String() == "end" || msg.String() == "G":
		a.investmentRegister.table.MoveToBottom()
	case msg.String() == "pgup":
		tableHeight := max(a.height-6, 1)
		a.investmentRegister.table.PageUp(tableHeight)
	case msg.String() == "pgdown":
		tableHeight := max(a.height-6, 1)
		a.investmentRegister.table.PageDown(tableHeight)
	case key.Matches(msg, a.keys.Search):
		// Enter the security filter. Starting a new query drops any locked
		// security so the user types fresh.
		a.investmentRegister.filterSearching = true
		a.investmentRegister.filterQuery = ""
		a.investmentRegister.filterLockedSec = types.NilID
		a.investmentRegister.buildTable(a.styles)
		if a.investmentRegister.table != nil {
			a.investmentRegister.table.SetCursor(0)
		}
	case a.investmentRegister.data.account != nil && a.investmentRegister.data.account.IsClosed() &&
		(msg.String() == "c" || key.Matches(msg, a.keys.New) || key.Matches(msg, a.keys.Enter) || key.Matches(msg, a.keys.Delete)):
		// A closed account is frozen: navigation and `p` (portfolio) still
		// work, but mutating actions are a no-op with an explanatory toast.
		a.statusbar.AddNotification("Account is closed — reopen to make changes", widget.NotificationAlert)
		return a, nil
	case key.Matches(msg, a.keys.New):
		a.openInvestmentTypeSelector(false)
	case key.Matches(msg, a.keys.Enter):
		txn := a.investmentRegister.selectedTransaction()
		if txn != nil {
			if notice, refused := a.investmentRegister.shareTransferEditRefusal(a.investmentRegisterDeps(), txn); refused {
				a.statusbar.AddNotification(notice, widget.NotificationAlert)
				return a, nil
			}
			a.openInvestmentTypeSelector(true)
		}
	case msg.String() == "c":
		return a.toggleInvestmentTransactionStatus()
	case msg.String() == "p":
		// Switch to portfolio view
		if a.investmentRegister.data != nil && a.investmentRegister.data.account != nil {
			a.portfolio.data = nil // Clear old data while loading
			a.switchView(ViewPortfolio)
			return a, a.portfolio.load(a.portfolioDeps(), a.investmentRegister.data.account.ID)
		}
	case key.Matches(msg, a.keys.Delete):
		txn := a.investmentRegister.selectedTransaction()
		if txn != nil {
			txnID := txn.ID
			// Transfer-typed rows have a paired counterpart in another
			// account that must be deleted with them — surface that in
			// the confirmation prompt so the user isn't surprised when
			// the savings (or other-investment) side also disappears.
			prompt := fmt.Sprintf("Delete this %s transaction?", txn.Type.DisplayName())
			if txn.TransferID.Valid {
				prompt = fmt.Sprintf("Delete this %s transaction? Both sides will be removed.", txn.Type.DisplayName())
			}
			// Classified here, synchronously: the confirm callback runs on a
			// separate goroutine and must not re-read register state.
			isTransferLeg := isCashTransferLeg(txn)
			var transferID types.ID
			if isTransferLeg {
				transferID = txn.TransferID.ID
			}
			a.showConfirmDialog(
				"Delete Transaction",
				prompt,
				func() tea.Msg {
					if a.services.Investment == nil {
						return errMsg{err: fmt.Errorf("investment service not available")}
					}
					// A cash-transfer row is one LEG of a pair whose counterpart may
					// live in the other ledger, so the transfer owner deletes it —
					// investment.Service.DeleteTransaction refuses it outright, and
					// deleting the leg alone would orphan the counterpart. Routing it
					// through undo also makes it Ctrl+Z-able, which it never was.
					//
					// Share transfers stay on DeleteTransaction: both their legs are
					// investment rows owned by this package, and their cascade also
					// reverses lot and position effects.
					if isTransferLeg {
						if a.services.Transfer == nil || a.undoManager == nil {
							return errMsg{err: fmt.Errorf("transfer service not available")}
						}
						cmd := undo.NewDeleteTransferCommand(a.services.Transfer, transferID)
						if err := a.undoManager.Execute(cmd); err != nil {
							return errMsg{err: err}
						}
						return investmentTransactionDeletedMsg{}
					}
					if err := a.services.Investment.DeleteTransaction(txnID); err != nil {
						return errMsg{err: err}
					}
					return investmentTransactionDeletedMsg{}
				},
			)
		}
	}

	return a, nil
}

// preselectSecurityCombo sets a freshly-built investment dialog's "Security"
// combo to the given security, used so a NEW transaction opened while the
// register is filtered to a security defaults to it. It is a no-op when secID
// is nil or is not among the dialog's options.
func preselectSecurityCombo(d *dialog.Dialog, secIDs []types.ID, secID types.ID) {
	if d == nil || secID.IsNil() {
		return
	}
	for i, id := range secIDs {
		if id == secID {
			if f := d.FieldByLabel("Security"); f != nil {
				f.SelectIndex(i)
			}
			return
		}
	}
}

// toggleInvestmentTransactionStatus toggles the cleared status of the selected investment transaction.
func (a *App) toggleInvestmentTransactionStatus() (tea.Model, tea.Cmd) {
	txn := a.investmentRegister.selectedTransaction()
	if txn == nil {
		return a, nil
	}

	txnID := txn.ID
	currentStatus := txn.Status
	// Classified synchronously — the closure below runs on another goroutine.
	isTransferLeg := isCashTransferLeg(txn)

	return a, func() tea.Msg {
		if a.services.Investment == nil {
			return errMsg{err: fmt.Errorf("investment service not available")}
		}

		var cleared bool
		switch currentStatus {
		case investment.TransactionStatusPending:
			cleared = true
		case investment.TransactionStatusCleared:
			cleared = false
		default:
			return nil
		}

		// A cash-transfer leg's status belongs to the transfer owner, exactly as in
		// the regular register. Three reasons this is not just symmetry:
		// SetClearedStatus is a second write path onto a transfer leg; it is not
		// undoable; and it rewrites the whole row where SetLegStatus writes only the
		// status column, which matters on DuckDB where an UPDATE touching indexed
		// or FK columns is internally a DELETE+INSERT.
		//
		// Only this leg moves — clearing your side of a transfer says your
		// institution posted it, independent of the other account.
		if isTransferLeg {
			if a.services.Transfer == nil || a.undoManager == nil {
				return errMsg{err: fmt.Errorf("transfer service not available")}
			}
			status := transaction.StatusCleared
			if !cleared {
				status = transaction.StatusUncleared
			}
			cmd := undo.NewSetTransferLegStatusCommand(a.services.Transfer, txnID, status)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: err}
			}
			return investmentTransactionClearedMsg{}
		}

		// Route through the service so the closed-account freeze gate applies.
		if err := a.services.Investment.SetClearedStatus(txnID, cleared); err != nil {
			return errMsg{err: err}
		}
		return investmentTransactionClearedMsg{}
	}
}

// isCashTransferLeg reports whether an investment row is one leg of a
// whole-transaction cash transfer, and therefore owned by transfer.Service
// rather than investment.Service.
//
// This is the routing decision that the investment register's delete and clear
// keys both turn on, and getting it wrong is not a cosmetic error:
// investment.Service.DeleteTransaction REFUSES such a leg (deleting it alone
// would orphan a counterpart that may live in the other ledger), and
// SetClearedStatus would quietly become a second write path onto a transfer leg.
//
// Share transfers deliberately return false: both their legs are investment rows
// owned by internal/investment, and their delete cascade also reverses lot and
// position effects, which the transfer owner does not do.
func isCashTransferLeg(txn *investment.Transaction) bool {
	return txn != nil &&
		txn.TransferID.Valid &&
		txn.Type == investment.TransactionTypeTransferCash
}

// investmentTransactionDeletedMsg is sent when an investment transaction has been deleted.
type investmentTransactionDeletedMsg struct{}

// investmentTransactionClearedMsg is sent when an investment transaction's status has been toggled.
type investmentTransactionClearedMsg struct{}

// shareTransferEditRefusal returns the notice for a share-transfer row that
// this register cannot edit. The edit dialog always sends from the register's
// account, so only the sending leg can be edited here. The service refuses the
// other rows too (UpdateTransferShares); checking first keeps the user from
// filling in a dialog that would be rejected.
func (s *investmentRegisterViewState) shareTransferEditRefusal(d investmentRegisterDeps, txn *investment.Transaction) (string, bool) {
	if txn.Type != investment.TransactionTypeTransferShares || txn.IsShareTransferSource() {
		return "", false
	}
	if !txn.IsShareTransferDestination() {
		return "This share transfer has no cost basis, so its direction is unknown. It cannot be edited.", true
	}
	source := "its source account"
	if accounts := d.accounts(); txn.TransferAccountID.Valid && accounts != nil {
		if acct, err := accounts.GetByID(txn.TransferAccountID.ID); err == nil {
			source = acct.Name
		}
	}
	return fmt.Sprintf("Edit this share transfer from %s.", source), true
}

// investmentRegisterShortcuts returns the shortcut section for the investment register help overlay.
func investmentRegisterShortcuts() shortcutSection {
	return shortcutSection{
		Title: "Investment Register",
		Entries: []shortcutEntry{
			{Key: "n", Description: "New transaction"},
			{Key: "Enter", Description: "Edit transaction"},
			{Key: "c", Description: "Toggle cleared"},
			{Key: "d", Description: "Delete transaction"},
			{Key: "/", Description: "Filter by security"},
			{Key: "p", Description: "Portfolio view"},
			{Key: "Tab", Description: "Switch sidebar/table"},
			{Key: "Esc", Description: "Go back"},
		},
	}
}
