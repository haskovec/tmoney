package account

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

// accountListOptions are the inputs to `tmoney account list`.
type accountListOptions struct {
	file          string
	includeClosed bool
}

// newAccountListCmd registers `tmoney account list`. The database
// file is taken from the persistent `--file` / `-f` flag inherited
// from the root command.
func newAccountListCmd() *cobra.Command {
	opts := &accountListOptions{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List accounts",
		Long: "List accounts in the TMoney database. By default only " +
			"active accounts are shown; pass `--include-closed` to " +
			"include closed accounts.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runAccountList(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&opts.includeClosed, "include-closed", false, "Include closed accounts in the listing")
	return cmd
}

// runAccountList lists accounts from the database.
func runAccountList(opts *accountListOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	accounts, err := svc.Account.List(!opts.includeClosed)
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}

	balances, err := svc.Account.GetAllBalances()
	if err != nil {
		return fmt.Errorf("failed to get balances: %w", err)
	}

	printAccountsTable(w, accounts, balances)

	return nil
}
