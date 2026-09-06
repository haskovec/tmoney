package transaction

import (
	"fmt"
	"io"
	"strings"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/db"
	transactiondom "github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// transferSplitPrefix marks a --split line as a transfer line: the text
// after the prefix is the target account name.
const transferSplitPrefix = "Transfer:"

// transactionAddOptions are the inputs to `tmoney transaction add`.
type transactionAddOptions struct {
	file     string
	account  string
	amount   string
	payee    string
	category string
	date     string
	memo     string
	splits   []string

	amountChanged bool
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
		Long: "Create a new transaction on an account. `--account` is required. " +
			"Without `--split`, `--amount` is required and the transaction is a " +
			"single line. Repeat `--split \"Category=amount[:memo]\"` to create a " +
			"split transaction whose lines carry the categories; a line named " +
			"`Transfer:<Account>=amount` is a transfer line to another account. " +
			"With `--split`, `--category` is refused and `--amount` is optional " +
			"(if given the lines must sum to it, otherwise it is derived from the " +
			"line sum). Split-line editing is done in the TUI.",
		Example: "  tmoney transaction add --account Checking --amount -50 --category Food\n" +
			"  tmoney transaction add --account Checking --split \"Food:Groceries=-40\" --split \"Household=-10:paper towels\"\n" +
			"  tmoney transaction add --account Checking --amount -100 --split \"Food=-60\" --split \"Transfer:Savings=-40\"",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.amountChanged = cmd.Flags().Changed("amount")
			return runTransactionAdd(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Account name (required)")
	cmd.Flags().StringVar(&opts.amount, "amount", "", "Transaction amount; negative for expenses (required unless --split is used)")
	cmd.Flags().StringVar(&opts.payee, "payee", "", "Payee name (auto-created if it doesn't exist)")
	cmd.Flags().StringVar(&opts.category, "category", "", "Category name (Parent or Parent:Subcategory); not allowed with --split")
	cmd.Flags().StringVar(&opts.date, "date", "", "Transaction date YYYY-MM-DD (default today)")
	cmd.Flags().StringVar(&opts.memo, "memo", "", "Free-form memo")
	cmd.Flags().StringArrayVar(&opts.splits, "split", nil, "Split line \"Category=amount[:memo]\" or \"Transfer:<Account>=amount[:memo]\"; repeatable")
	_ = cmd.MarkFlagRequired("account")
	return cmd
}

// runTransactionAdd creates a new transaction, either a single-line
// transaction or, when one or more `--split` flags are given, a split
// transaction created via CreateWithSplits.
func runTransactionAdd(opts *transactionAddOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	hasSplits := len(opts.splits) > 0

	// --amount is required without --split (MarkFlagRequired can't be
	// conditional, so enforce it here). --category is refused with --split.
	if !hasSplits && !opts.amountChanged {
		return fmt.Errorf("required flag(s) \"amount\" not set")
	}
	if hasSplits && opts.category != "" {
		return fmt.Errorf("--category is not allowed with --split; put the categories on the split lines")
	}

	// Parse the single-line amount up front so a bad value is reported
	// before we open the database. The split path parses --amount later
	// (only to compare against the line sum).
	var singleAmount types.Money
	var err error
	if !hasSplits {
		singleAmount, err = types.NewMoney(opts.amount)
		if err != nil {
			return fmt.Errorf("invalid --amount: %w", err)
		}
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

	if hasSplits {
		return addWithSplits(opts, w, database, svc, acct, date, payeeID, payeeName, payeeCreated)
	}

	amount := singleAmount

	// Handle category
	var categoryID types.NullableID
	var categoryName string
	if opts.category != "" {
		cat, err := resolveCategoryByName(svc, opts.category)
		if err != nil {
			return err
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

	cmdutil.AutoBackupAfterModification(database)
	return nil
}

// parsedSplit is a resolved --split line: the built Split plus the amount,
// memo, and a display label used in the confirmation summary.
type parsedSplit struct {
	split  *transactiondom.Split
	amount types.Money
	memo   string
	label  string // display label for the confirmation summary
}

// parseSplitFlag parses one `--split` value of the form
// "Name=amount[:memo]" into its name, amount string, and memo parts. The
// name runs to the first `=`; the amount runs to the first `:` after it;
// anything after that first `:` is the memo (which may itself contain `:`).
func parseSplitFlag(raw string) (name, amountStr, memo string, err error) {
	before, after, ok := strings.Cut(raw, "=")
	if !ok {
		return "", "", "", fmt.Errorf("invalid --split %q: expected \"Name=amount[:memo]\"", raw)
	}
	name = strings.TrimSpace(before)
	rest := after
	if before, after, ok := strings.Cut(rest, ":"); ok {
		amountStr = strings.TrimSpace(before)
		memo = strings.TrimSpace(after)
	} else {
		amountStr = strings.TrimSpace(rest)
	}
	if name == "" {
		return "", "", "", fmt.Errorf("invalid --split %q: empty name", raw)
	}
	return name, amountStr, memo, nil
}

// resolveSplits parses and resolves every --split flag into Splits ready
// for CreateWithSplits, and returns the signed sum of the line amounts.
func resolveSplits(opts *transactionAddOptions, svc *app.Services, acct *account.Account) ([]parsedSplit, types.Money, error) {
	parsed := make([]parsedSplit, 0, len(opts.splits))
	total := types.ZeroMoney
	for _, raw := range opts.splits {
		name, amountStr, memo, err := parseSplitFlag(raw)
		if err != nil {
			return nil, types.ZeroMoney, err
		}
		amount, err := types.NewMoney(amountStr)
		if err != nil {
			return nil, types.ZeroMoney, fmt.Errorf("invalid amount in --split %q: %w", raw, err)
		}
		if amount.IsZero() {
			return nil, types.ZeroMoney, fmt.Errorf("invalid --split %q: amount cannot be zero", raw)
		}

		var sp *transactiondom.Split
		var label string
		if target, ok := strings.CutPrefix(name, transferSplitPrefix); ok {
			target = strings.TrimSpace(target)
			if target == "" {
				return nil, types.ZeroMoney, fmt.Errorf("invalid --split %q: transfer line needs a target account", raw)
			}
			if target == acct.Name {
				return nil, types.ZeroMoney, fmt.Errorf("invalid --split %q: cannot transfer to the transaction's own account %q", raw, acct.Name)
			}
			targetAcct, err := svc.Account.GetByName(target)
			if err != nil {
				return nil, types.ZeroMoney, fmt.Errorf("transfer target account %q not found", target)
			}
			sp = transactiondom.NewSplitWithMemo(types.NilID, types.NilID, amount, memo)
			sp.TransferAccountID = types.NullableID{ID: targetAcct.ID, Valid: true}
			label = fmt.Sprintf("Transfer to %s", targetAcct.Name)
		} else {
			cat, err := resolveCategoryByName(svc, name)
			if err != nil {
				return nil, types.ZeroMoney, err
			}
			sp = transactiondom.NewSplitWithMemo(types.NilID, cat.ID, amount, memo)
			label = cat.Name
		}

		parsed = append(parsed, parsedSplit{split: sp, amount: amount, memo: memo, label: label})
		total = total.Add(amount)
	}
	return parsed, total, nil
}

// addWithSplits builds and persists a split transaction via
// CreateWithSplits, then prints a summary listing each line.
func addWithSplits(opts *transactionAddOptions, w io.Writer, database *db.DB, svc *app.Services, acct *account.Account, date types.Date, payeeID types.NullableID, payeeName string, payeeCreated bool) error {
	parsed, lineSum, err := resolveSplits(opts, svc, acct)
	if err != nil {
		return err
	}

	// Determine the parent amount: derive from the line sum, or, when
	// --amount is given, require the lines to sum to it.
	amount := lineSum
	if opts.amountChanged {
		amount, err = types.NewMoney(opts.amount)
		if err != nil {
			return fmt.Errorf("invalid --amount: %w", err)
		}
		if !lineSum.Equal(amount) {
			return fmt.Errorf("split lines sum to %s but --amount is %s; they must match", lineSum.String(), amount.String())
		}
	}

	txn := transactiondom.NewTransaction(acct.ID, date, amount)
	if payeeID.Valid {
		txn.SetPayee(payeeID.ID)
	}
	if opts.memo != "" {
		txn.SetMemo(opts.memo)
	}

	splits := make([]*transactiondom.Split, len(parsed))
	for i, p := range parsed {
		p.split.TransactionID = txn.ID
		splits[i] = p.split
	}

	if err := svc.Transaction.CreateWithSplits(txn, splits); err != nil {
		return fmt.Errorf("failed to create split transaction: %w", err)
	}

	fmt.Fprintln(w, "Split transaction created successfully!")
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
	if opts.memo != "" {
		fmt.Fprintf(w, "  Memo:     %s\n", opts.memo)
	}
	fmt.Fprintf(w, "  Splits:   %d line(s)\n", len(parsed))
	for _, p := range parsed {
		line := fmt.Sprintf("    - %s: %s", p.label, cmdutil.FormatMoney(p.amount, acct.Currency))
		if p.memo != "" {
			line += fmt.Sprintf(" (%s)", p.memo)
		}
		fmt.Fprintln(w, line)
	}

	cmdutil.AutoBackupAfterModification(database)
	return nil
}
