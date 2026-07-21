package scheduled

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	scheduleddom "github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// scheduledListOptions are the inputs to `tmoney scheduled list`.
type scheduledListOptions struct {
	file    string
	account string
	due     bool
	showIDs bool
}

// newScheduledListCmd registers `tmoney scheduled list`. The database
// file is taken from the persistent `--file` / `-f` flag inherited from
// the root command. `--due` restricts the listing to scheduled
// transactions whose next occurrence is on or before today.
func newScheduledListCmd() *cobra.Command {
	opts := &scheduledListOptions{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List scheduled transactions",
		Long: "List scheduled transactions on the database. Pass --due to " +
			"limit the listing to occurrences that are due today or earlier, " +
			"or --account to filter by account name.",
		Example: "  tmoney scheduled list --file personal.tdb\n" +
			"  tmoney scheduled list --file personal.tdb --due\n" +
			"  tmoney scheduled list --file personal.tdb --account Checking",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runScheduledList(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Filter by account name")
	cmd.Flags().BoolVar(&opts.due, "due", false, "Only show scheduled transactions due today or earlier")
	cmd.Flags().BoolVar(&opts.showIDs, "show-ids", false, "Show each scheduled transaction's full UUID (for use with `scheduled edit`/`scheduled delete`)")
	return cmd
}

// runScheduledList lists scheduled transactions.
func runScheduledList(opts *scheduledListOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	var scheduledTxns []*scheduleddom.Transaction
	if opts.due {
		scheduledTxns, err = svc.Scheduled.ListDue()
	} else {
		scheduledTxns, err = svc.Scheduled.List()
	}
	if err != nil {
		return fmt.Errorf("failed to list scheduled transactions: %w", err)
	}

	if opts.account != "" {
		acct, err := svc.Account.GetByName(opts.account)
		if err != nil {
			return fmt.Errorf("account %q not found", opts.account)
		}

		var filtered []*scheduleddom.Transaction
		for _, st := range scheduledTxns {
			if st.AccountID == acct.ID {
				filtered = append(filtered, st)
			}
		}
		scheduledTxns = filtered
	}

	payeeNames := make(map[types.ID]string)
	categoryNames := make(map[types.ID]string)
	accountNames := make(map[types.ID]string)
	accountCurrencies := make(map[types.ID]string)

	payees, _ := svc.PayeeRepo.List()
	for _, p := range payees {
		payeeNames[p.ID] = p.Name
	}

	categories, _ := svc.CategoryRepo.List()
	for _, c := range categories {
		categoryNames[c.ID] = c.Name
	}

	accounts, _ := svc.AccountRepo.List(false)
	for _, a := range accounts {
		accountNames[a.ID] = a.Name
		accountCurrencies[a.ID] = a.Currency
	}

	printScheduledTransactionsTable(w, scheduledTxns, opts.due, opts.showIDs, accountNames, accountCurrencies, payeeNames, categoryNames)

	return nil
}
