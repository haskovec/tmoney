package account

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

// accountBalanceOptions are the inputs to `tmoney account balance`.
type accountBalanceOptions struct {
	file string
}

// newAccountBalanceCmd registers `tmoney account balance`. It prints
// the balance for every active account and the resulting net worth.
// The database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command.
func newAccountBalanceCmd() *cobra.Command {
	opts := &accountBalanceOptions{}
	cmd := &cobra.Command{
		Use:          "balance",
		Short:        "Show balances for all active accounts",
		Long:         "Show the current balance of every active account along with overall net worth.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runAccountBalance(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runAccountBalance shows balances for all accounts with net worth.
func runAccountBalance(opts *accountBalanceOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	accounts, err := svc.Account.List(true)
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}

	balances, err := svc.Account.GetAllBalances()
	if err != nil {
		return fmt.Errorf("failed to get balances: %w", err)
	}

	printBalancesTable(w, accounts, balances)

	return nil
}
