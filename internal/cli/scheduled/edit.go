package scheduled

import (
	"fmt"
	"io"
	"strings"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	scheduleddom "github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// scheduledEditOptions are the inputs to `tmoney scheduled edit`.
// The *Changed booleans record which editable flags were supplied so the
// command can apply delta semantics (only supplied flags take effect).
type scheduledEditOptions struct {
	file      string
	id        string
	amount    string
	payee     string
	category  string
	frequency string
	nextDate  string
	account   string
	memo      string
	autoPost  bool

	amountChanged    bool
	payeeChanged     bool
	categoryChanged  bool
	frequencyChanged bool
	nextDateChanged  bool
	accountChanged   bool
	memoChanged      bool
	autoPostChanged  bool
}

// newScheduledEditCmd registers `tmoney scheduled edit`. The database file
// is taken from the persistent `--file` / `-f` flag inherited from the root
// command. `--id` is required; at least one editable flag must be supplied
// and only supplied flags take effect (matching `transaction edit`).
func newScheduledEditCmd() *cobra.Command {
	opts := &scheduledEditOptions{}
	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit an existing scheduled transaction",
		Long: "Edit a single-line scheduled transaction identified by its UUID " +
			"(find it with `tmoney scheduled list --show-ids`). Only the supplied " +
			"flags take effect; pass an empty string to `--amount`, `--payee`, " +
			"`--category`, or `--memo` to clear that field (`--amount \"\"` makes the " +
			"schedule variable-amount). Multi-line (split/paycheck) templates are " +
			"edited in the TUI.",
		Example: "  tmoney scheduled edit --id <uuid> --amount -1600\n" +
			"  tmoney scheduled edit --id <uuid> --frequency weekly --auto-post",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.amountChanged = cmd.Flags().Changed("amount")
			opts.payeeChanged = cmd.Flags().Changed("payee")
			opts.categoryChanged = cmd.Flags().Changed("category")
			opts.frequencyChanged = cmd.Flags().Changed("frequency")
			opts.nextDateChanged = cmd.Flags().Changed("next-date")
			opts.accountChanged = cmd.Flags().Changed("account")
			opts.memoChanged = cmd.Flags().Changed("memo")
			opts.autoPostChanged = cmd.Flags().Changed("auto-post")
			return runScheduledEdit(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.id, "id", "", "UUID of the scheduled transaction to edit (required)")
	cmd.Flags().StringVar(&opts.amount, "amount", "", "New amount; pass an empty string to clear (variable-amount schedule)")
	cmd.Flags().StringVar(&opts.payee, "payee", "", "New payee name, auto-created if it doesn't exist (pass an empty string to clear)")
	cmd.Flags().StringVar(&opts.category, "category", "", "New category name, Parent or Parent:Subcategory (pass an empty string to clear)")
	cmd.Flags().StringVar(&opts.frequency, "frequency", "",
		"New frequency: daily, weekly, fortnightly, semimonthly, monthly, quarterly, yearly")
	cmd.Flags().StringVar(&opts.nextDate, "next-date", "", "New next occurrence date YYYY-MM-DD")
	cmd.Flags().StringVar(&opts.account, "account", "", "Move the schedule to a different account (by name)")
	cmd.Flags().StringVar(&opts.memo, "memo", "", "New memo (pass an empty string to clear)")
	cmd.Flags().BoolVar(&opts.autoPost, "auto-post", false, "Post automatically when due (--auto-post=false disables)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// runScheduledEdit executes `tmoney scheduled edit`: all field edits route
// through scheduled.Service.Update (the same path the TUI edit dialog uses).
func runScheduledEdit(opts *scheduledEditOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}
	if !opts.amountChanged && !opts.payeeChanged && !opts.categoryChanged &&
		!opts.frequencyChanged && !opts.nextDateChanged && !opts.accountChanged &&
		!opts.memoChanged && !opts.autoPostChanged {
		return fmt.Errorf("at least one editable flag is required (--amount, --payee, --category, --frequency, --next-date, --account, --memo, --auto-post)")
	}

	id, err := types.ParseID(opts.id)
	if err != nil {
		return fmt.Errorf("invalid --id: %w", err)
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	st, err := svc.Scheduled.GetByID(id)
	if err != nil {
		return fmt.Errorf("scheduled transaction %s not found", opts.id)
	}

	if len(st.Splits) > 0 {
		return fmt.Errorf("scheduled transaction is a multi-line (split/paycheck) template; multi-line templates are edited in the TUI")
	}

	if err := applyScheduledEdits(svc, st, opts); err != nil {
		return err
	}

	if err := svc.Scheduled.Update(st); err != nil {
		return fmt.Errorf("failed to update scheduled transaction: %w", err)
	}

	printScheduledSummary(w, svc, "Scheduled transaction updated successfully!", st)

	cmdutil.AutoBackupAfterModification(database)
	return nil
}

// applyScheduledEdits mutates st in place with the supplied delta flags.
// Payees auto-create (same as `scheduled add`); categories must exist. A
// transfer schedule refuses a payee (it has no payee) and stores its amount
// as the signed effect on the source account (negative), matching the TUI.
func applyScheduledEdits(svc *app.Services, st *scheduleddom.Transaction, opts *scheduledEditOptions) error {
	if opts.amountChanged {
		if opts.amount == "" {
			st.ClearAmount()
		} else {
			amount, err := types.NewMoney(opts.amount)
			if err != nil {
				return fmt.Errorf("invalid --amount: %w", err)
			}
			// A transfer schedule stores the amount as the negative signed
			// effect on the source account (the TUI enters a positive
			// magnitude and stores its negation).
			if st.IsTransfer() {
				amount = amount.Abs().Neg()
			}
			st.SetAmount(amount)
		}
	}

	if opts.payeeChanged {
		if st.IsTransfer() {
			return fmt.Errorf("scheduled transaction is a transfer and has no payee; edit its category or amount instead")
		}
		if opts.payee == "" {
			st.ClearPayee()
		} else {
			py, _, err := svc.Payee.GetOrCreate(opts.payee)
			if err != nil {
				return fmt.Errorf("failed to resolve payee: %w", err)
			}
			st.SetPayee(py.ID)
		}
	}

	if opts.categoryChanged {
		if opts.category == "" {
			st.ClearCategory()
		} else {
			cat, err := resolveScheduledCategory(svc, opts.category)
			if err != nil {
				return err
			}
			// A transfer schedule may only carry a non-system category label, and
			// only when its pair can store one at all.
			if st.IsTransfer() {
				if err := transaction.ValidateTransferCategory(cat); err != nil {
					return err
				}
				if err := refuseUnsupportedTransferCategory(svc, st); err != nil {
					return err
				}
			}
			st.SetCategory(cat.ID)
		}
	}

	if opts.frequencyChanged {
		frequency, err := scheduleddom.ParseFrequency(opts.frequency)
		if err != nil {
			validFreqs := []string{}
			for _, f := range scheduleddom.AllFrequencies() {
				validFreqs = append(validFreqs, string(f))
			}
			return fmt.Errorf("invalid --frequency %q: valid values are %s", opts.frequency, strings.Join(validFreqs, ", "))
		}
		st.Frequency = frequency
		st.Touch()
	}

	if opts.nextDateChanged {
		nextDate, err := types.ParseDate(opts.nextDate)
		if err != nil {
			return fmt.Errorf("invalid --next-date: %w", err)
		}
		st.NextDate = nextDate
		st.Touch()
	}

	if opts.accountChanged {
		acct, err := svc.Account.GetByName(opts.account)
		if err != nil {
			return fmt.Errorf("account %q not found", opts.account)
		}
		// A closed account is frozen: it takes no new transactions, so a
		// schedule may not be moved onto it (Service.Update does not re-run the
		// closed-account check that Create performs).
		if acct.IsClosed() {
			return fmt.Errorf("account %q is closed; reopen it first with `tmoney account reopen`", acct.Name)
		}
		// Moving a transfer schedule onto its own destination would make it a
		// self-transfer; refuse with a clear message before the service does.
		if st.IsTransfer() && st.TransferAccountID.ID == acct.ID {
			return fmt.Errorf("cannot move the schedule to its transfer destination (%s); that would be a self-transfer", acct.Name)
		}
		st.AccountID = acct.ID
		st.Touch()
	}

	if opts.memoChanged {
		st.SetMemo(opts.memo)
	}

	if opts.autoPostChanged {
		st.SetAutoPost(opts.autoPost)
	}

	return nil
}

// resolveScheduledCategory finds a category by name, first as a top-level
// category and then across all categories (so subcategory display names like
// "Food:Groceries" resolve). Shared by the scheduled edit and add paths.
func resolveScheduledCategory(svc *app.Services, name string) (*category.Category, error) {
	cat, err := svc.CategoryRepo.GetByName(name, nil)
	if err == nil {
		return cat, nil
	}
	categories, listErr := svc.CategoryRepo.List()
	if listErr != nil {
		return nil, fmt.Errorf("category %q not found", name)
	}
	for _, c := range categories {
		if c.Name == name {
			return c, nil
		}
	}
	return nil, fmt.Errorf("category %q not found", name)
}

// printScheduledSummary prints a confirmation block for the given scheduled
// transaction, resolving account/payee/category/transfer-destination names for
// display. Shared by the edit and delete commands.
func printScheduledSummary(w io.Writer, svc *app.Services, header string, st *scheduleddom.Transaction) {
	accountName := "Unknown"
	currency := "USD"
	if acct, err := svc.Account.GetByID(st.AccountID); err == nil {
		accountName = acct.Name
		currency = acct.Currency
	}

	fmt.Fprintln(w, header)
	fmt.Fprintf(w, "  Account:   %s\n", accountName)
	fmt.Fprintf(w, "  Frequency: %s\n", st.Frequency.DisplayName())
	fmt.Fprintf(w, "  Next Date: %s\n", st.NextDate.String())
	if st.HasAmount() {
		fmt.Fprintf(w, "  Amount:    %s\n", cmdutil.FormatMoney(st.Amount.Money, currency))
	} else {
		fmt.Fprintf(w, "  Amount:    Variable\n")
	}
	if st.IsTransfer() {
		destName := "Unknown"
		if dest, err := svc.Account.GetByID(st.TransferAccountID.ID); err == nil {
			destName = dest.Name
		}
		fmt.Fprintf(w, "  Transfer to: %s\n", destName)
	} else if st.HasPayee() {
		if py, err := svc.PayeeRepo.GetByID(st.PayeeID.ID); err == nil {
			fmt.Fprintf(w, "  Payee:     %s\n", py.Name)
		}
	}
	if st.HasCategory() {
		if cat, err := svc.CategoryRepo.GetByID(st.CategoryID.ID); err == nil {
			fmt.Fprintf(w, "  Category:  %s\n", cat.Name)
		}
	}
	if st.Memo.Valid {
		fmt.Fprintf(w, "  Memo:      %s\n", st.Memo.String)
	}
	if st.AutoPost {
		if st.PostLeadDays > 0 {
			fmt.Fprintf(w, "  Auto-post: Yes (%d days early)\n", st.PostLeadDays)
		} else {
			fmt.Fprintf(w, "  Auto-post: Yes\n")
		}
	}
}
