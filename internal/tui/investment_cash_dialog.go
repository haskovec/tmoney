package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
)

// cashOperationDialogSavedMsg is sent when a cash operation transaction has been saved.
// savedDate carries the transaction date so the App can use it as the
// session's sticky-date seed for subsequent dialog opens.
type cashOperationDialogSavedMsg struct {
	savedDate types.Date
	// savedID is the ID of the saved transaction so the investment register
	// can move the cursor onto its row after reload.
	savedID types.ID
}

// buildCashOperationDialog creates a dialog.Dialog for cash-only investment operations
// (Deposit, Withdrawal, Fee, Interest). These share the same fields: Date, Amount, Memo.
func buildCashOperationDialog(title string, editTxn *investment.Transaction) *dialog.Dialog {
	d := dialog.NewDialog(title)
	d.SetWidth(70)

	// Date
	dateVal := ""
	if editTxn != nil {
		dateVal = editTxn.Date.Time().Format("01/02/2006")
	}
	f := d.AddDateField("Date", dateVal)
	f.Required = true

	// Amount
	amountVal := ""
	if editTxn != nil && !editTxn.TotalAmount.IsZero() {
		amt := editTxn.TotalAmount
		if amt.IsNegative() {
			amt = amt.Neg()
		}
		amountVal = fmt.Sprintf("%.2f", amt.Float64())
	}
	f = d.AddNumericField("Amount", amountVal, "500.00", 12)
	f.Required = true

	// Memo
	memoVal := ""
	if editTxn != nil && editTxn.Memo.Valid {
		memoVal = editTxn.Memo.String
	}
	d.AddTextField("Memo", memoVal, "Optional memo", 0)

	d.SetVisible(true)
	return d
}

// closeCashOperationDialog clears the cash operation dialog state.
func (a *App) closeCashOperationDialog() {
	a.cashOperationDialog = nil
	a.cashOperationType = ""
}

// handleCashOperationDialogKey routes key events to the cash operation dialog.
func (a *App) handleCashOperationDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.cashOperationDialog == nil {
		return a, nil
	}

	action := a.cashOperationDialog.HandleKey(msg)
	switch action {
	case dialog.DialogActionSubmit:
		return a.submitCashOperationDialog()
	case dialog.DialogActionCancel:
		a.closeCashOperationDialog()
		return a, nil
	}

	return a, nil
}

// submitCashOperationDialog parses dialog fields, validates, and saves a cash operation transaction.
func (a *App) submitCashOperationDialog() (tea.Model, tea.Cmd) {
	if a.cashOperationDialog == nil {
		return a, nil
	}

	fields := a.cashOperationDialog.Fields()
	if len(fields) < 3 {
		return a, nil
	}

	a.cashOperationDialog.ClearErrors()
	hasErrors := false

	// Date (index 0)
	date, err := parseDateInput(fields[0].Value)
	if err != nil {
		fields[0].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	// Amount (index 1)
	amount, err := parseOptionalMoneyInput(fields[1].Value)
	if err != nil {
		fields[1].Error = "Invalid amount"
		hasErrors = true
	}
	if amount == nil {
		fields[1].Error = "Amount is required"
		hasErrors = true
	}

	if hasErrors {
		return a, nil
	}

	// Memo (index 2)
	memo := strings.TrimSpace(fields[2].Value)

	// Get account ID
	accountID := types.NilID
	if a.investmentRegister != nil && a.investmentRegister.account != nil {
		accountID = a.investmentRegister.account.ID
	}

	editTxnID := a.investmentEditTxnID
	txnType := a.cashOperationType
	amountVal := *amount

	// Close dialog before async save
	a.closeCashOperationDialog()

	return a, func() tea.Msg {
		if a.investmentSvc == nil {
			return errMsg{err: fmt.Errorf("investment service not available")}
		}

		var saved *investment.Transaction
		var txnErr error
		if editTxnID != types.NilID {
			switch txnType {
			case investment.TransactionTypeDeposit:
				saved, txnErr = a.investmentSvc.UpdateDeposit(editTxnID, accountID, date, amountVal, memo)
			case investment.TransactionTypeWithdrawal:
				saved, txnErr = a.investmentSvc.UpdateWithdrawal(editTxnID, accountID, date, amountVal, memo)
			case investment.TransactionTypeFee:
				saved, txnErr = a.investmentSvc.UpdateFee(editTxnID, accountID, date, amountVal, memo)
			case investment.TransactionTypeInterest:
				saved, txnErr = a.investmentSvc.UpdateInterest(editTxnID, accountID, date, amountVal, memo)
			default:
				return errMsg{err: fmt.Errorf("unsupported cash operation type: %s", txnType)}
			}
		} else {
			switch txnType {
			case investment.TransactionTypeDeposit:
				saved, txnErr = a.investmentSvc.Deposit(accountID, date, amountVal, memo)
			case investment.TransactionTypeWithdrawal:
				saved, txnErr = a.investmentSvc.Withdrawal(accountID, date, amountVal, memo)
			case investment.TransactionTypeFee:
				saved, txnErr = a.investmentSvc.Fee(accountID, date, amountVal, memo)
			case investment.TransactionTypeInterest:
				saved, txnErr = a.investmentSvc.Interest(accountID, date, amountVal, memo)
			default:
				return errMsg{err: fmt.Errorf("unsupported cash operation type: %s", txnType)}
			}
		}

		if txnErr != nil {
			return errMsg{err: fmt.Errorf("failed to save %s transaction: %w", txnType.DisplayName(), txnErr)}
		}

		return cashOperationDialogSavedMsg{savedDate: date, savedID: saved.ID}
	}
}
