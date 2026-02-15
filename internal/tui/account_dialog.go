package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haskovec/tmoney/internal/models"
)

// accountDialogMode indicates whether the dialog is creating or editing.
type accountDialogMode int

const (
	accountDialogModeNew  accountDialogMode = iota
	accountDialogModeEdit
)

// accountDialogData holds the loaded data needed for the account dialog.
type accountDialogData struct {
	mode    accountDialogMode
	account *models.Account // non-nil when editing
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
	types := models.AllAccountTypes()
	options := make([]string, len(types))
	for i, t := range types {
		options[i] = t.DisplayName()
	}
	return options
}

// accountTypeFromIndex returns the AccountType for a given select index.
func accountTypeFromIndex(index int) models.AccountType {
	types := models.AllAccountTypes()
	if index < 0 || index >= len(types) {
		return models.AccountTypeChecking
	}
	return types[index]
}

// accountTypeToIndex returns the select index for a given AccountType.
func accountTypeToIndex(at models.AccountType) int {
	types := models.AllAccountTypes()
	for i, t := range types {
		if t == at {
			return i
		}
	}
	return 0
}

// accountTypeShowsCreditLimit returns true if the account type should show the credit limit field.
func accountTypeShowsCreditLimit(at models.AccountType) bool {
	return at == models.AccountTypeCreditCard
}

// accountTypeShowsInterestRate returns true if the account type should show the interest rate field.
func accountTypeShowsInterestRate(at models.AccountType) bool {
	switch at {
	case models.AccountTypeChecking, models.AccountTypeSavings, models.AccountTypeCreditCard,
		models.AccountTypeInvestment, models.AccountTypeLoan:
		return true
	default:
		return false
	}
}

// updateAccountFieldVisibility updates the Hidden state of credit limit and interest rate
// fields based on the currently selected account type.
func updateAccountFieldVisibility(d *Dialog) {
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

// buildNewAccountDialog creates a Dialog for creating a new account.
func buildNewAccountDialog() *Dialog {
	d := NewDialog("New Account")

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
	today := time.Now().Format("01/02/2006")
	f = d.AddTextField("Opening Date", today, "MM/DD/YYYY", 10)
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

// buildEditAccountDialog creates a Dialog for editing an existing account.
func buildEditAccountDialog(account *models.Account) *Dialog {
	d := NewDialog("Edit Account")

	// Name
	f := d.AddTextField("Name", account.Name, "Account name", 0)
	f.Required = true

	// Type
	d.AddSelectField("Type", buildAccountTypeOptions(), accountTypeToIndex(account.Type))

	// Currency
	f = d.AddTextField("Currency", account.Currency, "ISO 4217", 5)
	f.Required = true

	// Opening balance
	f = d.AddTextField("Opening Balance", fmt.Sprintf("%.2f", account.OpeningBalance.Float64()), "0.00", 12)
	f.Required = true

	// Opening date
	dateStr := account.OpeningDate.Time().Format("01/02/2006")
	f = d.AddTextField("Opening Date", dateStr, "MM/DD/YYYY", 10)
	f.Required = true

	// Institution
	institution := ""
	if account.Institution.Valid {
		institution = account.Institution.String
	}
	d.AddTextField("Institution", institution, "Bank name (optional)", 0)

	// Account number
	acctNum := ""
	if account.AccountNumber.Valid {
		acctNum = account.AccountNumber.String
	}
	d.AddTextField("Account #", acctNum, "Account number (optional)", 0)

	// Notes
	notes := ""
	if account.Notes.Valid {
		notes = account.Notes.String
	}
	d.AddTextField("Notes", notes, "Optional notes", 0)

	// Credit limit (shown for credit cards)
	creditLimit := ""
	if account.CreditLimit.Valid {
		creditLimit = fmt.Sprintf("%.2f", account.CreditLimit.Money.Float64())
	}
	d.AddTextField("Credit Limit", creditLimit, "e.g. 5000.00", 12)

	// Interest rate (shown for most account types)
	interestRate := ""
	if account.InterestRate.Valid {
		interestRate = fmt.Sprintf("%.2f", account.InterestRate.Money.Float64())
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
func (a *App) handleAccountDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.acctDialog == nil {
		return a, nil
	}

	action := a.acctDialog.HandleKey(msg)
	switch action {
	case DialogActionSubmit:
		return a.submitAccountDialog()
	case DialogActionCancel:
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
	var creditLimit models.NullableMoney
	if !fields[acctFieldCreditLimit].Hidden {
		creditLimitStr := strings.TrimSpace(fields[acctFieldCreditLimit].Value)
		if creditLimitStr != "" {
			cl, err := parseAmountInput(creditLimitStr)
			if err != nil {
				fields[acctFieldCreditLimit].Error = "Invalid amount"
				hasErrors = true
			} else {
				creditLimit = models.NullableMoney{Money: cl, Valid: true}
			}
		}
	}

	// Parse interest rate if field is visible and provided
	var interestRate models.NullableMoney
	if !fields[acctFieldInterestRate].Hidden {
		interestRateStr := strings.TrimSpace(fields[acctFieldInterestRate].Value)
		if interestRateStr != "" {
			ir, err := parseAmountInput(interestRateStr)
			if err != nil {
				fields[acctFieldInterestRate].Error = "Invalid amount"
				hasErrors = true
			} else {
				interestRate = models.NullableMoney{Money: ir, Valid: true}
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
		if a.accountSvc == nil {
			return errMsg{err: fmt.Errorf("account service not available")}
		}

		var account *models.Account
		if mode == accountDialogModeEdit && existingAccount != nil {
			account = existingAccount
			account.Name = name
			account.Type = accountType
			account.Currency = currency
			account.OpeningBalance = openingBalance
			account.OpeningDate = openingDate
		} else {
			account = models.NewAccount(name, accountType, currency, openingBalance, openingDate)
		}

		// Set optional fields
		account.SetInstitution(institution)
		account.SetAccountNumber(accountNumber)
		account.SetNotes(notes)

		// Type-specific fields
		account.CreditLimit = creditLimit
		account.InterestRate = interestRate

		if mode == accountDialogModeEdit {
			if err := a.accountSvc.Update(account); err != nil {
				return errMsg{err: fmt.Errorf("failed to update account: %w", err)}
			}
		} else {
			if err := a.accountSvc.Create(account); err != nil {
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
		if a.accountSvc == nil {
			return errMsg{err: fmt.Errorf("account service not available")}
		}

		if err := a.accountSvc.Close(accountID); err != nil {
			return errMsg{err: fmt.Errorf("failed to close account: %w", err)}
		}

		return accountClosedMsg{}
	}
}

// deleteSelectedAccount deletes the currently selected account.
func (a *App) deleteSelectedAccount() tea.Cmd {
	accountID := a.sidebar.SelectedAccountID()
	return func() tea.Msg {
		if a.accountSvc == nil {
			return errMsg{err: fmt.Errorf("account service not available")}
		}

		if err := a.accountSvc.Delete(accountID); err != nil {
			return errMsg{err: fmt.Errorf("failed to delete account: %w", err)}
		}

		return accountDeletedMsg{}
	}
}
