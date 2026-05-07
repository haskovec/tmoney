package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// accountAddOptions are the inputs to `tmoney account add`.
type accountAddOptions struct {
	file          string
	name          string
	accountType   string
	currency      string
	openingBal    string
	openingDate   string
	institution   string
	accountNumber string
	notes         string
	creditLimit   string
	interestRate  string
}

// newAccountAddCmd registers `tmoney account add`. The database file
// is taken from the persistent `--file` / `-f` flag inherited from
// the root command. `--name` and `--type` are required.
func newAccountAddCmd() *cobra.Command {
	opts := &accountAddOptions{}
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a new account",
		Long: "Create a new account in the TMoney database. " +
			"`--name` and `--type` are required; other fields take sensible defaults.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runAccountAdd(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.name, "name", "", "Account name (required)")
	cmd.Flags().StringVar(&opts.accountType, "type", "", "Account type: checking, savings, credit_card, investment, cash, loan, asset (required)")
	cmd.Flags().StringVar(&opts.currency, "currency", "", "Currency code (default USD)")
	cmd.Flags().StringVar(&opts.openingBal, "opening-balance", "", "Opening balance (default 0)")
	cmd.Flags().StringVar(&opts.openingDate, "opening-date", "", "Opening date YYYY-MM-DD (default today)")
	cmd.Flags().StringVar(&opts.institution, "institution", "", "Institution name")
	cmd.Flags().StringVar(&opts.accountNumber, "account-number", "", "Account number")
	cmd.Flags().StringVar(&opts.notes, "notes", "", "Free-form notes")
	cmd.Flags().StringVar(&opts.creditLimit, "credit-limit", "", "Credit limit (credit_card accounts only)")
	cmd.Flags().StringVar(&opts.interestRate, "interest-rate", "", "Interest rate / APR (loan accounts only)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

// runAccountAdd creates a new account.
func runAccountAdd(opts *accountAddOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
	}

	// Parse account type
	acctType, err := account.ParseType(opts.accountType)
	if err != nil {
		validTypes := []string{}
		for _, t := range account.AllTypes() {
			validTypes = append(validTypes, string(t))
		}
		return fmt.Errorf("invalid --type %q: valid types are %s", opts.accountType, strings.Join(validTypes, ", "))
	}

	// Parse currency (default to USD)
	currency := "USD"
	if opts.currency != "" {
		currency = strings.ToUpper(opts.currency)
	}

	// Parse opening balance (default to 0)
	openingBalance := types.MustNewMoney("0")
	if opts.openingBal != "" {
		openingBalance, err = types.NewMoney(opts.openingBal)
		if err != nil {
			return fmt.Errorf("invalid --opening-balance: %w", err)
		}
	}

	// Parse opening date (default to today)
	openingDate := types.Today()
	if opts.openingDate != "" {
		openingDate, err = types.ParseDate(opts.openingDate)
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
	if _, err := svc.Account.GetByName(opts.name); err == nil {
		return fmt.Errorf("account %q already exists", opts.name)
	}

	// Create account
	acct := account.NewAccount(opts.name, acctType, currency, openingBalance, openingDate)

	// Set optional fields
	if opts.institution != "" {
		acct.SetInstitution(opts.institution)
	}
	if opts.accountNumber != "" {
		acct.SetAccountNumber(opts.accountNumber)
	}
	if opts.notes != "" {
		acct.SetNotes(opts.notes)
	}

	// Handle type-specific fields
	if opts.creditLimit != "" {
		if acctType != account.TypeCreditCard {
			return fmt.Errorf("--credit-limit is only valid for credit_card accounts")
		}
		creditLimit, err := types.NewMoney(opts.creditLimit)
		if err != nil {
			return fmt.Errorf("invalid --credit-limit: %w", err)
		}
		acct.SetCreditLimit(creditLimit)
	}

	if opts.interestRate != "" {
		if acctType != account.TypeLoan {
			return fmt.Errorf("--interest-rate is only valid for loan accounts")
		}
		interestRate, err := types.NewMoney(opts.interestRate)
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
