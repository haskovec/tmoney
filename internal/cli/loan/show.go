package loan

import (
	"fmt"
	"io"

	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

// loanShowOptions are the inputs to `tmoney loan show <name>`.
type loanShowOptions struct {
	file  string
	name  string
	limit int
	all   bool
}

// newLoanShowCmd registers `tmoney loan show <name>`. The database file is taken
// from the persistent `--file` / `-f` flag inherited from the root command; the
// single positional argument is the loan account name.
func newLoanShowCmd() *cobra.Command {
	opts := &loanShowOptions{}
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show a loan's details and amortization projection",
		Long: "Show a loan account's balance, APR, P&I payment, escrow, payoff " +
			"date, and interest remaining, followed by its remaining-payment " +
			"amortization table. --limit caps the number of projection rows " +
			"(default 12); --all shows every remaining payment.",
		Example: "  tmoney loan show Mortgage\n" +
			"  tmoney loan show Mortgage --limit 24\n" +
			"  tmoney loan show Mortgage --all",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.name = args[0]
			return runLoanShow(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().IntVar(&opts.limit, "limit", 12, "Number of projection rows to show (ignored with --all)")
	cmd.Flags().BoolVar(&opts.all, "all", false, "Show all remaining payments")
	return cmd
}

// runLoanShow shows a single loan's details and amortization projection.
func runLoanShow(opts *loanShowOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	acct, err := svc.Account.GetByName(opts.name)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.name)
	}
	if acct.Type != accountdom.TypeLoan {
		return fmt.Errorf("account %q is not a loan account (type %s)", opts.name, acct.Type)
	}

	info, err := resolveLoanInfo(svc, acct)
	if err != nil {
		return fmt.Errorf("failed to resolve loan %q: %w", opts.name, err)
	}

	printLoanShow(w, info, opts.limit, opts.all)
	return nil
}
