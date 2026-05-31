package transaction

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	transactiondom "github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// transactionAddOptions are the inputs to `tmoney transaction add`.
type transactionAddOptions struct {
	file     string
	account  string
	amount   string
	payee    string
	category string
	date     string
	memo     string
}

// newTransactionAddCmd registers `tmoney transaction add`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. `--account` and `--amount` are
// required.
func newTransactionAddCmd() *cobra.Command {
	opts := &transactionAddOptions{}
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a new transaction",
		Long: "Create a new transaction on an account. " +
			"`--account` and `--amount` are required; other fields take sensible defaults.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runTransactionAdd(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Account name (required)")
	cmd.Flags().StringVar(&opts.amount, "amount", "", "Transaction amount; negative for expenses (required)")
	cmd.Flags().StringVar(&opts.payee, "payee", "", "Payee name (auto-created if it doesn't exist)")
	cmd.Flags().StringVar(&opts.category, "category", "", "Category name (Parent or Parent:Subcategory)")
	cmd.Flags().StringVar(&opts.date, "date", "", "Transaction date YYYY-MM-DD (default today)")
	cmd.Flags().StringVar(&opts.memo, "memo", "", "Free-form memo")
	_ = cmd.MarkFlagRequired("account")
	_ = cmd.MarkFlagRequired("amount")
	return cmd
}

// runTransactionAdd creates a new transaction.
func runTransactionAdd(opts *transactionAddOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	// Parse amount
	amount, err := types.NewMoney(opts.amount)
	if err != nil {
		return fmt.Errorf("invalid --amount: %w", err)
	}

	// Parse date (default to today)
	var date types.Date
	if opts.date != "" {
		date, err = types.ParseDate(opts.date)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
	} else {
		date = types.Today()
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Get account by name
	acct, err := svc.Account.GetByName(opts.account)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.account)
	}

	// Handle payee (auto-create if needed)
	var payeeID types.NullableID
	var payeeName string
	var payeeCreated bool
	if opts.payee != "" {
		py, created, err := svc.Payee.GetOrCreate(opts.payee)
		if err != nil {
			return fmt.Errorf("failed to resolve payee: %w", err)
		}
		payeeID = types.NullableID{Valid: true, ID: py.ID}
		payeeName = py.Name
		payeeCreated = created
	}

	// Handle category
	var categoryID types.NullableID
	var categoryName string
	if opts.category != "" {
		// First try top-level category, then search all categories
		cat, err := svc.CategoryRepo.GetByName(opts.category, nil)
		if err != nil {
			// Try finding it as a subcategory (search all categories)
			categories, listErr := svc.CategoryRepo.List()
			if listErr != nil {
				return fmt.Errorf("category %q not found", opts.category)
			}
			var found *category.Category
			for _, c := range categories {
				if c.Name == opts.category {
					found = c
					break
				}
			}
			if found == nil {
				return fmt.Errorf("category %q not found", opts.category)
			}
			cat = found
		}
		categoryID = types.NullableID{Valid: true, ID: cat.ID}
		categoryName = cat.Name
	}

	// Create transaction
	txn := transactiondom.NewTransaction(acct.ID, date, amount)
	if payeeID.Valid {
		txn.SetPayee(payeeID.ID)
	}
	if categoryID.Valid {
		txn.SetCategory(categoryID.ID)
	}
	if opts.memo != "" {
		txn.SetMemo(opts.memo)
	}

	// Save transaction
	if err := svc.Transaction.Create(txn); err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Print confirmation
	fmt.Fprintln(w, "Transaction created successfully!")
	fmt.Fprintf(w, "  Account:  %s\n", acct.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Amount:   %s\n", cmdutil.FormatMoney(amount, acct.Currency))
	if payeeName != "" {
		if payeeCreated {
			fmt.Fprintf(w, "  Payee:    %s (new)\n", payeeName)
		} else {
			fmt.Fprintf(w, "  Payee:    %s\n", payeeName)
		}
	}
	if categoryName != "" {
		fmt.Fprintf(w, "  Category: %s\n", categoryName)
	}
	if opts.memo != "" {
		fmt.Fprintf(w, "  Memo:     %s\n", opts.memo)
	}

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
