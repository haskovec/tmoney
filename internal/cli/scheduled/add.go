package scheduled

import (
	"fmt"
	"io"
	"strings"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	scheduleddom "github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// scheduledAddOptions are the inputs to `tmoney scheduled add`.
type scheduledAddOptions struct {
	file        string
	account     string
	frequency   string
	amount      string
	payee       string
	category    string
	date        string
	memo        string
	day         int
	occurrences int64
	endDate     string
	autoPost    bool
	leadDays    int
}

// newScheduledAddCmd registers `tmoney scheduled add`. The database file
// is taken from the persistent `--file` / `-f` flag inherited from the
// root command. `--account` and `--frequency` are required.
func newScheduledAddCmd() *cobra.Command {
	opts := &scheduledAddOptions{}
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a new scheduled transaction",
		Long: "Create a new scheduled transaction on an account. " +
			"`--account` and `--frequency` are required; `--amount` is optional " +
			"(omit it to create a variable-amount schedule).",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runScheduledAdd(cmd, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Account name (required)")
	cmd.Flags().StringVar(&opts.frequency, "frequency", "",
		"Frequency: daily, weekly, fortnightly, semimonthly, monthly, quarterly, yearly (required)")
	cmd.Flags().StringVar(&opts.amount, "amount", "",
		"Scheduled amount; omit for a variable-amount schedule")
	cmd.Flags().StringVar(&opts.payee, "payee", "", "Payee name (auto-created if it doesn't exist)")
	cmd.Flags().StringVar(&opts.category, "category", "", "Category name (Parent or Parent:Subcategory)")
	cmd.Flags().StringVar(&opts.date, "date", "", "Start date YYYY-MM-DD (default today)")
	cmd.Flags().StringVar(&opts.memo, "memo", "", "Free-form memo")
	cmd.Flags().IntVar(&opts.day, "day", 0, "Day of month (1-31, or -1 for last day of month)")
	cmd.Flags().Int64Var(&opts.occurrences, "occurrences", 0, "Number of occurrences (omit for indefinite)")
	cmd.Flags().StringVar(&opts.endDate, "end-date", "", "End date YYYY-MM-DD (omit for indefinite)")
	cmd.Flags().BoolVar(&opts.autoPost, "auto-post", false, "Post automatically when due")
	cmd.Flags().IntVar(&opts.leadDays, "lead-days", 0, "Auto-post lead days: 0, 3, or 7 (requires --auto-post)")
	_ = cmd.MarkFlagRequired("account")
	_ = cmd.MarkFlagRequired("frequency")
	return cmd
}

// runScheduledAdd creates a new scheduled transaction.
func runScheduledAdd(cmd *cobra.Command, opts *scheduledAddOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	frequency, err := scheduleddom.ParseFrequency(opts.frequency)
	if err != nil {
		validFreqs := []string{}
		for _, f := range scheduleddom.AllFrequencies() {
			validFreqs = append(validFreqs, string(f))
		}
		return fmt.Errorf("invalid --frequency %q: valid values are %s", opts.frequency, strings.Join(validFreqs, ", "))
	}

	var startDate types.Date
	if opts.date != "" {
		startDate, err = types.ParseDate(opts.date)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
	} else {
		startDate = types.Today()
	}

	if cmd.Flags().Changed("lead-days") {
		if !opts.autoPost {
			return fmt.Errorf("--lead-days requires --auto-post")
		}
		if opts.leadDays != 0 && opts.leadDays != 3 && opts.leadDays != 7 {
			return fmt.Errorf("--lead-days must be 0, 3, or 7")
		}
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	acct, err := svc.Account.GetByName(opts.account)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.account)
	}

	st := scheduleddom.NewTransaction(acct.ID, frequency, startDate)

	if opts.amount != "" {
		amount, err := types.NewMoney(opts.amount)
		if err != nil {
			return fmt.Errorf("invalid --amount: %w", err)
		}
		st.SetAmount(amount)
	}

	var payeeName string
	if opts.payee != "" {
		py, _, err := svc.Payee.GetOrCreate(opts.payee)
		if err != nil {
			return fmt.Errorf("failed to resolve payee: %w", err)
		}
		st.SetPayee(py.ID)
		payeeName = py.Name
	}

	var categoryName string
	if opts.category != "" {
		cat, err := svc.CategoryRepo.GetByName(opts.category, nil)
		if err != nil {
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
		st.SetCategory(cat.ID)
		categoryName = cat.Name
	}

	if opts.memo != "" {
		st.SetMemo(opts.memo)
	}

	if cmd.Flags().Changed("day") {
		st.SetDayOfMonth(opts.day)
	}

	if cmd.Flags().Changed("occurrences") {
		st.SetOccurrences(opts.occurrences)
	}

	if opts.endDate != "" {
		endDate, err := types.ParseDate(opts.endDate)
		if err != nil {
			return fmt.Errorf("invalid --end-date: %w", err)
		}
		st.SetEndDate(endDate)
	}

	if opts.autoPost {
		st.SetAutoPost(true)
	}

	if cmd.Flags().Changed("lead-days") {
		st.SetPostLeadDays(opts.leadDays)
	}

	if err := svc.Scheduled.Create(st); err != nil {
		return fmt.Errorf("failed to create scheduled transaction: %w", err)
	}

	fmt.Fprintln(w, "Scheduled transaction created successfully!")
	fmt.Fprintf(w, "  Account:   %s\n", acct.Name)
	fmt.Fprintf(w, "  Frequency: %s\n", frequency.DisplayName())
	fmt.Fprintf(w, "  Next Date: %s\n", st.NextDate.String())
	if st.HasAmount() {
		fmt.Fprintf(w, "  Amount:    %s\n", cmdutil.FormatMoney(st.Amount.Money, acct.Currency))
	} else {
		fmt.Fprintf(w, "  Amount:    Variable\n")
	}
	if payeeName != "" {
		fmt.Fprintf(w, "  Payee:     %s\n", payeeName)
	}
	if categoryName != "" {
		fmt.Fprintf(w, "  Category:  %s\n", categoryName)
	}
	if opts.memo != "" {
		fmt.Fprintf(w, "  Memo:      %s\n", opts.memo)
	}
	if st.AutoPost {
		if st.PostLeadDays > 0 {
			fmt.Fprintf(w, "  Auto-post: Yes (%d days early)\n", st.PostLeadDays)
		} else {
			fmt.Fprintf(w, "  Auto-post: Yes\n")
		}
	}

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
