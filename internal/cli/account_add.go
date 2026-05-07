package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

func runAddAccount(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--add-account requires --file to specify a database")
	}
	if opts.acctName == "" {
		return fmt.Errorf("--add-account requires --name to specify an account name")
	}
	if opts.acctType == "" {
		return fmt.Errorf("--add-account requires --type to specify an account type")
	}

	// Parse account type
	acctType, err := account.ParseType(opts.acctType)
	if err != nil {
		validTypes := []string{}
		for _, t := range account.AllTypes() {
			validTypes = append(validTypes, string(t))
		}
		return fmt.Errorf("invalid --type %q: valid types are %s", opts.acctType, strings.Join(validTypes, ", "))
	}

	// Parse currency (default to USD)
	currency := "USD"
	if opts.acctCurrency != "" {
		currency = strings.ToUpper(opts.acctCurrency)
	}

	// Parse opening balance (default to 0)
	openingBalance := types.MustNewMoney("0")
	if opts.acctOpeningBal != "" {
		openingBalance, err = types.NewMoney(opts.acctOpeningBal)
		if err != nil {
			return fmt.Errorf("invalid --opening-balance: %w", err)
		}
	}

	// Parse opening date (default to today)
	openingDate := types.Today()
	if opts.acctOpeningDate != "" {
		openingDate, err = types.ParseDate(opts.acctOpeningDate)
		if err != nil {
			return fmt.Errorf("invalid --opening-date: %w", err)
		}
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Check if account name already exists
	if _, err := svc.Account.GetByName(opts.acctName); err == nil {
		return fmt.Errorf("account %q already exists", opts.acctName)
	}

	// Create account
	acct := account.NewAccount(opts.acctName, acctType, currency, openingBalance, openingDate)

	// Set optional fields
	if opts.acctInstitution != "" {
		acct.SetInstitution(opts.acctInstitution)
	}
	if opts.acctNumber != "" {
		acct.SetAccountNumber(opts.acctNumber)
	}
	if opts.acctNotes != "" {
		acct.SetNotes(opts.acctNotes)
	}

	// Handle type-specific fields
	if opts.acctCreditLimit != "" {
		if acctType != account.TypeCreditCard {
			return fmt.Errorf("--credit-limit is only valid for credit_card accounts")
		}
		creditLimit, err := types.NewMoney(opts.acctCreditLimit)
		if err != nil {
			return fmt.Errorf("invalid --credit-limit: %w", err)
		}
		acct.SetCreditLimit(creditLimit)
	}

	if opts.acctInterestRate != "" {
		if acctType != account.TypeLoan {
			return fmt.Errorf("--interest-rate is only valid for loan accounts")
		}
		interestRate, err := types.NewMoney(opts.acctInterestRate)
		if err != nil {
			return fmt.Errorf("invalid --interest-rate: %w", err)
		}
		acct.SetInterestRate(interestRate)
	}

	// Save account
	if err := svc.Account.Create(acct); err != nil {
		return fmt.Errorf("failed to create account: %w", err)
	}

	// Print confirmation
	fmt.Fprintln(w, "Account created successfully!")
	fmt.Fprintf(w, "  Name:            %s\n", acct.Name)
	fmt.Fprintf(w, "  Type:            %s\n", acct.Type.DisplayName())
	fmt.Fprintf(w, "  Currency:        %s\n", acct.Currency)
	fmt.Fprintf(w, "  Opening Balance: %s\n", formatMoney(acct.OpeningBalance, acct.Currency))
	fmt.Fprintf(w, "  Opening Date:    %s\n", acct.OpeningDate.String())
	if acct.Institution.Valid {
		fmt.Fprintf(w, "  Institution:     %s\n", acct.Institution.String)
	}
	if acct.AccountNumber.Valid {
		fmt.Fprintf(w, "  Account Number:  %s\n", acct.AccountNumber.String)
	}
	if acct.CreditLimit.Valid {
		fmt.Fprintf(w, "  Credit Limit:    %s\n", formatMoney(acct.CreditLimit.Money, acct.Currency))
	}
	if acct.InterestRate.Valid {
		fmt.Fprintf(w, "  Interest Rate:   %s%%\n", acct.InterestRate.Money.String())
	}
	if acct.Notes.Valid {
		fmt.Fprintf(w, "  Notes:           %s\n", acct.Notes.String)
	}

	autoBackupAfterModification(opts.file)
	return nil
}
