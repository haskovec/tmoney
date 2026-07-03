package loan

import (
	"fmt"
	"io"

	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

// loanListOptions are the inputs to `tmoney loan list`.
type loanListOptions struct {
	file string
}

// newLoanListCmd registers `tmoney loan list`. The database file is taken from
// the persistent `--file` / `-f` flag inherited from the root command.
func newLoanListCmd() *cobra.Command {
	opts := &loanListOptions{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List loans with balance, rate, payment, and payoff projection",
		Long: "List every loan account with its balance owed, APR, monthly P&I " +
			"payment, next payment date, payoff date, and interest remaining. Loans " +
			"with no loan-shaped payment schedule show dashes for the schedule-derived " +
			"columns; a loan that never pays off within 100 years shows 100y+.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runLoanList(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runLoanList lists loan accounts with their live amortization summary.
func runLoanList(opts *loanListOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	accounts, err := svc.Account.List(false)
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}

	var infos []*loanInfo
	for _, acct := range accounts {
		if acct.Type != accountdom.TypeLoan {
			continue
		}
		info, err := resolveLoanInfo(svc, acct)
		if err != nil {
			return fmt.Errorf("failed to resolve loan %q: %w", acct.Name, err)
		}
		infos = append(infos, info)
	}

	printLoanList(w, infos)
	return nil
}
