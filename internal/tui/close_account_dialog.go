package tui

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// buildCloseAccountDialog builds the Close Account confirmation dialog: a single
// editable Close Date field (defaulting to today) plus a body that explains the
// freeze and warns about any scheduled transactions that reference the account.
func buildCloseAccountDialog(acct *account.Account, scheduledCount int) *dialog.Dialog {
	d := dialog.NewDialog("Close Account")

	msg := fmt.Sprintf("%q will be frozen — no new transactions, edits, or transfers, "+
		"and it drops out of account pickers. Reopen to make changes.", acct.Name)
	if scheduledCount > 0 {
		msg += fmt.Sprintf("\n\nWarning: %d scheduled transaction(s) reference this account; "+
			"they will be skipped on auto-post and refused on manual post until redirected.", scheduledCount)
	}
	d.SetMessage(msg)

	f := d.AddDateField("Close Date", types.Today().Time().Format("01/02/2006"))
	f.Required = true

	d.SetButtons([]dialog.DialogButton{
		{Label: "Close Account", Primary: true},
		{Label: "Cancel"},
	})
	return d
}

// showCloseAccountDialog opens the Close Account dialog for the sidebar's
// selected account, capturing the target ID so a later reselection can't
// retarget the close.
func (a *App) showCloseAccountDialog() {
	accountID := a.sidebar.SelectedAccountID()
	if accountID == types.NilID || a.accountSvc == nil {
		return
	}
	acct, err := a.accountSvc.GetByID(accountID)
	if err != nil {
		a.statusbar.AddNotification(fmt.Sprintf("Cannot close account: %v", err), widget.NotificationAlert)
		return
	}
	if acct.IsClosed() {
		a.statusbar.AddNotification("Account is already closed", widget.NotificationAlert)
		return
	}

	scheduledCount := 0
	if a.scheduledTxnSvc != nil {
		if refs, rerr := a.scheduledTxnSvc.ListReferencing(accountID); rerr == nil {
			scheduledCount = len(refs)
		}
	}

	a.closeAcctTargetID = accountID
	a.closeAcctDialog = buildCloseAccountDialog(acct, scheduledCount)
	a.closeAcctDialog.SetVisible(true)
}

// handleCloseAcctDialogKey routes keys for the Close Account dialog.
func (a *App) handleCloseAcctDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return a.closeAcctDialogAction(a.closeAcctDialog.HandleKey(msg))
}

// closeAcctDialogAction dispatches a DialogAction for the close acct dialog, from either input path.
func (a *App) closeAcctDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionCancel:
		a.closeAcctDialog.SetVisible(false)
		a.closeAcctDialog = nil
		return a, nil
	case dialog.DialogActionSubmit:
		return a.submitCloseAccountDialog()
	}
	return a, nil
}

// submitCloseAccountDialog validates the close date and closes the account via
// an undoable command, surfacing zero-balance / date-range errors in the dialog.
func (a *App) submitCloseAccountDialog() (tea.Model, tea.Cmd) {
	closeDate, err := parseDateInput(a.closeAcctDialog.Fields()[0].Value)
	if err != nil {
		a.closeAcctDialog.SetErrorMsg("Invalid date format. Use MM/DD/YYYY.")
		return a, nil
	}

	accountID := a.closeAcctTargetID
	if accountID == types.NilID || a.accountSvc == nil || a.undoManager == nil {
		a.closeAcctDialog.SetErrorMsg("Account service not available.")
		return a, nil
	}

	cmd := undo.NewCloseAccountCommand(a.accountSvc, accountID, closeDate)
	if err := a.undoManager.Execute(cmd); err != nil {
		a.closeAcctDialog.SetErrorMsg(closeAccountErrorMessage(err))
		return a, nil
	}

	a.closeAcctDialog.SetVisible(false)
	a.closeAcctDialog = nil
	return a, func() tea.Msg { return accountClosedMsg{} }
}

// closeAccountErrorMessage renders a friendly dialog message for the typed
// close failures.
func closeAccountErrorMessage(err error) string {
	var balErr *account.HasBalanceError
	var dateErr *account.InvalidCloseDateError
	switch {
	case errors.As(err, &balErr):
		return "Cannot close: the account balance must be zero."
	case errors.As(err, &dateErr):
		return fmt.Sprintf("Close date must be between %s and %s.", dateErr.Earliest, dateErr.Today)
	default:
		return fmt.Sprintf("Failed to close account: %v", err)
	}
}

// selectedAccountClosed reports whether the sidebar's selected account is closed.
func (a *App) selectedAccountClosed() bool {
	acct := a.sidebar.SelectedAccount()
	return acct != nil && acct.IsClosed()
}

// reopenSelectedAccount reopens the sidebar's selected account (instant +
// undoable). Reopening clears the close date.
func (a *App) reopenSelectedAccount() tea.Cmd {
	accountID := a.sidebar.SelectedAccountID()
	return func() tea.Msg {
		if a.accountSvc == nil || a.undoManager == nil {
			return errMsg{err: fmt.Errorf("account service not available")}
		}
		cmd := undo.NewReopenAccountCommand(a.accountSvc, accountID)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to reopen account: %w", err)}
		}
		return accountClosedMsg{}
	}
}
