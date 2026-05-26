package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// accountDialogMode indicates whether the dialog is creating or editing.
type accountDialogMode int

const (
	accountDialogModeNew accountDialogMode = iota
	accountDialogModeEdit
)

// accountDialogData holds the loaded data needed for the account dialog.
type accountDialogData struct {
	mode    accountDialogMode
	account *account.Account // non-nil when editing
}

// accountDialogDataMsg is sent when account dialog data has been loaded.
type accountDialogDataMsg struct {
	data *accountDialogData
}

// accountDialogSavedMsg is sent when an account has been saved.
type accountDialogSavedMsg struct{}

// accountDeletedMsg is sent when an account has been deleted.
type accountDeletedMsg struct{}

// accountClosedMsg is sent when an account has been closed/reopened.
type accountClosedMsg struct{}

// Account dialog field indices for new account dialog.
const (
	acctFieldName           = 0
	acctFieldType           = 1
	acctFieldCurrency       = 2
	acctFieldOpeningBalance = 3
	acctFieldOpeningDate    = 4
	acctFieldInstitution    = 5
	acctFieldAccountNumber  = 6
	acctFieldNotes          = 7
	acctFieldCreditLimit    = 8
	acctFieldInterestRate   = 9
)

// buildAccountTypeOptions returns display names for all account types.
func buildAccountTypeOptions() []string {
	types := account.AllTypes()
	options := make([]string, len(types))
	for i, t := range types {
		options[i] = t.DisplayName()
	}
	return options
}

// accountTypeFromIndex returns the AccountType for a given select index.
func accountTypeFromIndex(index int) account.Type {
	types := account.AllTypes()
	if index < 0 || index >= len(types) {
		return account.TypeChecking
	}
	return types[index]
}

// accountTypeToIndex returns the select index for a given AccountType.
func accountTypeToIndex(at account.Type) int {
	types := account.AllTypes()
	for i, t := range types {
		if t == at {
			return i
		}
	}
	return 0
}

// accountTypeShowsCreditLimit returns true if the account type should show the credit limit field.
func accountTypeShowsCreditLimit(at account.Type) bool {
	return at == account.TypeCreditCard
}

// accountTypeShowsInterestRate returns true if the account type should show the interest rate field.
func accountTypeShowsInterestRate(at account.Type) bool {
	switch at {
	case account.TypeChecking, account.TypeSavings, account.TypeCreditCard,
		account.TypeInvestment, account.TypeHSA, account.TypeLoan:
		return true
	default:
		return false
	}
}

// updateAccountFieldVisibility updates the Hidden state of credit limit and interest rate
// fields based on the currently selected account type.
func updateAccountFieldVisibility(d *dialog.Dialog) {
	fields := d.Fields()
	if len(fields) <= acctFieldInterestRate {
		return
	}

	accountType := accountTypeFromIndex(fields[acctFieldType].SelectedIndex)

	showCreditLimit := accountTypeShowsCreditLimit(accountType)
	showInterestRate := accountTypeShowsInterestRate(accountType)

	fields[acctFieldCreditLimit].Hidden = !showCreditLimit
	fields[acctFieldInterestRate].Hidden = !showInterestRate

	// Clear values of hidden fields so stale data isn't submitted
	if !showCreditLimit {
		fields[acctFieldCreditLimit].Value = ""
		fields[acctFieldCreditLimit].Error = ""
	}
	if !showInterestRate {
		fields[acctFieldInterestRate].Value = ""
		fields[acctFieldInterestRate].Error = ""
	}
}

// buildNewAccountDialog creates a dialog.Dialog for creating a new account.
func buildNewAccountDialog() *dialog.Dialog {
	d := dialog.NewDialog("New Account")

	// Name
	f := d.AddTextField("Name", "", "Account name", 0)
	f.Required = true

	// Type
	d.AddSelectField("Type", buildAccountTypeOptions(), 0)

	// Currency
	f = d.AddTextField("Currency", "USD", "ISO 4217", 5)
	f.Required = true

	// Opening balance
	f = d.AddTextField("Opening Balance", "0.00", "0.00", 12)
	f.Required = true

	// Opening date
	f = d.AddDateField("Opening Date", "")
	f.Required = true

	// Institution (optional)
	d.AddTextField("Institution", "", "Bank name (optional)", 0)

	// Account number (optional)
	d.AddTextField("Account #", "", "Account number (optional)", 0)

	// Notes (optional)
	d.AddTextField("Notes", "", "Optional notes", 0)

	// Credit limit (shown for credit cards)
	d.AddTextField("Credit Limit", "", "e.g. 5000.00", 12)

	// Interest rate (shown for most account types)
	d.AddTextField("Interest Rate", "", "APR, e.g. 5.25", 8)

	updateAccountFieldVisibility(d)
	d.SetVisible(true)
	return d
}

// buildEditAccountDialog creates a dialog.Dialog for editing an existing account.
func buildEditAccountDialog(acct *account.Account) *dialog.Dialog {
	d := dialog.NewDialog("Edit Account")

	// Name
	f := d.AddTextField("Name", acct.Name, "Account name", 0)
	f.Required = true

	// Type
	d.AddSelectField("Type", buildAccountTypeOptions(), accountTypeToIndex(acct.Type))

	// Currency
	f = d.AddTextField("Currency", acct.Currency, "ISO 4217", 5)
	f.Required = true

	// Opening balance
	f = d.AddTextField("Opening Balance", fmt.Sprintf("%.2f", acct.OpeningBalance.Float64()), "0.00", 12)
	f.Required = true

	// Opening date
	dateStr := acct.OpeningDate.Time().Format("01/02/2006")
	f = d.AddDateField("Opening Date", dateStr)
	f.Required = true

	// Institution
	institution := ""
	if acct.Institution.Valid {
		institution = acct.Institution.String
	}
	d.AddTextField("Institution", institution, "Bank name (optional)", 0)

	// Account number
	acctNum := ""
	if acct.AccountNumber.Valid {
		acctNum = acct.AccountNumber.String
	}
	d.AddTextField("Account #", acctNum, "Account number (optional)", 0)

	// Notes
	notes := ""
	if acct.Notes.Valid {
		notes = acct.Notes.String
	}
	d.AddTextField("Notes", notes, "Optional notes", 0)

	// Credit limit (shown for credit cards)
	creditLimit := ""
	if acct.CreditLimit.Valid {
		creditLimit = fmt.Sprintf("%.2f", acct.CreditLimit.Money.Float64())
	}
	d.AddTextField("Credit Limit", creditLimit, "e.g. 5000.00", 12)

	// Interest rate (shown for most account types)
	interestRate := ""
	if acct.InterestRate.Valid {
		interestRate = fmt.Sprintf("%.2f", acct.InterestRate.Money.Float64())
	}
	d.AddTextField("Interest Rate", interestRate, "APR, e.g. 5.25", 8)

	updateAccountFieldVisibility(d)
	d.SetVisible(true)
	return d
}

// loadNewAccountDialogData returns a command that prepares the new account dialog.
func (a *App) loadNewAccountDialogData() tea.Cmd {
	return func() tea.Msg {
		return accountDialogDataMsg{
			data: &accountDialogData{
				mode: accountDialogModeNew,
			},
		}
	}
}

// loadEditAccountDialogData returns a command that loads the selected account for editing.
func (a *App) loadEditAccountDialogData() tea.Cmd {
	accountID := a.sidebar.SelectedAccountID()
	return func() tea.Msg {
		if a.accountSvc == nil {
			return errMsg{err: fmt.Errorf("account service not available")}
		}

		account, err := a.accountSvc.GetByID(accountID)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load account: %w", err)}
		}

		return accountDialogDataMsg{
			data: &accountDialogData{
				mode:    accountDialogModeEdit,
				account: account,
			},
		}
	}
}

// closeAccountDialog clears the account dialog state.
func (a *App) closeAccountDialog() {
	a.acctDialog = nil
	a.acctDialogData = nil
}

// handleAccountDialogKey routes key events to the account dialog.
func (a *App) handleAccountDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.acctDialog == nil {
		return a, nil
	}

	action := a.acctDialog.HandleKey(msg)
	switch action {
	case dialog.DialogActionSubmit:
		return a.submitAccountDialog()
	case dialog.DialogActionCancel:
		a.closeAccountDialog()
		return a, nil
	}

	// Update field visibility when account type changes
	updateAccountFieldVisibility(a.acctDialog)

	return a, nil
}

// submitAccountDialog parses dialog fields, validates, and saves the account.
func (a *App) submitAccountDialog() (tea.Model, tea.Cmd) {
	if a.acctDialog == nil || a.acctDialogData == nil {
		return a, nil
	}

	fields := a.acctDialog.Fields()
	if len(fields) < 10 {
		return a, nil
	}

	a.acctDialog.ClearErrors()
	hasErrors := false

	// Name
	name := strings.TrimSpace(fields[acctFieldName].Value)
	if name == "" {
		fields[acctFieldName].Error = "Account name is required"
		hasErrors = true
	}

	// Type
	accountType := accountTypeFromIndex(fields[acctFieldType].SelectedIndex)

	// Currency
	currency := strings.TrimSpace(fields[acctFieldCurrency].Value)
	if currency == "" {
		fields[acctFieldCurrency].Error = "Currency is required"
		hasErrors = true
	} else {
		currency = strings.ToUpper(currency)
	}

	// Opening balance
	openingBalance, err := parseAmountInput(fields[acctFieldOpeningBalance].Value)
	if err != nil {
		fields[acctFieldOpeningBalance].Error = "Invalid amount"
		hasErrors = true
	}

	// Opening date
	openingDate, err := parseDateInput(fields[acctFieldOpeningDate].Value)
	if err != nil {
		fields[acctFieldOpeningDate].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	// Optional fields
	institution := strings.TrimSpace(fields[acctFieldInstitution].Value)
	accountNumber := strings.TrimSpace(fields[acctFieldAccountNumber].Value)
	notes := strings.TrimSpace(fields[acctFieldNotes].Value)

	// Parse credit limit if field is visible and provided
	var creditLimit types.NullableMoney
	if !fields[acctFieldCreditLimit].Hidden {
		creditLimitStr := strings.TrimSpace(fields[acctFieldCreditLimit].Value)
		if creditLimitStr != "" {
			cl, err := parseAmountInput(creditLimitStr)
			if err != nil {
				fields[acctFieldCreditLimit].Error = "Invalid amount"
				hasErrors = true
			} else {
				creditLimit = types.NullableMoney{Money: cl, Valid: true}
			}
		}
	}

	// Parse interest rate if field is visible and provided
	var interestRate types.NullableMoney
	if !fields[acctFieldInterestRate].Hidden {
		interestRateStr := strings.TrimSpace(fields[acctFieldInterestRate].Value)
		if interestRateStr != "" {
			ir, err := parseAmountInput(interestRateStr)
			if err != nil {
				fields[acctFieldInterestRate].Error = "Invalid amount"
				hasErrors = true
			} else {
				interestRate = types.NullableMoney{Money: ir, Valid: true}
			}
		}
	}

	if hasErrors {
		return a, nil
	}

	mode := a.acctDialogData.mode
	existingAccount := a.acctDialogData.account

	// Close dialog before async save for responsive UI
	a.closeAccountDialog()

	return a, func() tea.Msg {
		if a.accountSvc == nil || a.undoManager == nil {
			return errMsg{err: fmt.Errorf("account service not available")}
		}

		var acct *account.Account
		if mode == accountDialogModeEdit && existingAccount != nil {
			acct = existingAccount
			acct.Name = name
			acct.Type = accountType
			acct.Currency = currency
			acct.OpeningBalance = openingBalance
			acct.OpeningDate = openingDate
		} else {
			acct = account.NewAccount(name, accountType, currency, openingBalance, openingDate)
		}

		// Set optional fields
		acct.SetInstitution(institution)
		acct.SetAccountNumber(accountNumber)
		acct.SetNotes(notes)

		// Type-specific fields
		acct.CreditLimit = creditLimit
		acct.InterestRate = interestRate

		if mode == accountDialogModeEdit {
			cmd := undo.NewEditAccountCommand(a.accountSvc, acct)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to update account: %w", err)}
			}
		} else {
			cmd := undo.NewCreateAccountCommand(a.accountSvc, acct)
			if err := a.undoManager.Execute(cmd); err != nil {
				return errMsg{err: fmt.Errorf("failed to create account: %w", err)}
			}
		}

		return accountDialogSavedMsg{}
	}
}

// closeSelectedAccount closes the currently selected account.
func (a *App) closeSelectedAccount() tea.Cmd {
	accountID := a.sidebar.SelectedAccountID()
	return func() tea.Msg {
		if a.accountSvc == nil || a.undoManager == nil {
			return errMsg{err: fmt.Errorf("account service not available")}
		}

		cmd := undo.NewCloseAccountCommand(a.accountSvc, accountID)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to close account: %w", err)}
		}

		return accountClosedMsg{}
	}
}

// deleteSelectedAccount deletes the currently selected account.
func (a *App) deleteSelectedAccount() tea.Cmd {
	accountID := a.sidebar.SelectedAccountID()
	return func() tea.Msg {
		if a.accountSvc == nil || a.undoManager == nil {
			return errMsg{err: fmt.Errorf("account service not available")}
		}

		cmd := undo.NewDeleteAccountCommand(a.accountSvc, accountID)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to delete account: %w", err)}
		}

		return accountDeletedMsg{}
	}
}
