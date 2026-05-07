package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// accountShowOptions are the inputs to `tmoney account show <name>`.
type accountShowOptions struct {
	file string
	name string
}

// newAccountShowCmd registers `tmoney account show <name>`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command; the single positional argument is
// the account name.
func newAccountShowCmd() *cobra.Command {
	opts := &accountShowOptions{}
	cmd := &cobra.Command{
		Use:          "show <name>",
		Short:        "Show details for a single account",
		Long:         "Show full details and current balance for the account with the given name.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.name = args[0]
			return runAccountShow(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runAccountShow shows detailed information for a specific account.
func runAccountShow(opts *accountShowOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	acct, err := svc.Account.GetByName(opts.name)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.name)
	}

	bal, err := svc.Account.GetBalance(acct.ID)
	if err != nil {
		return fmt.Errorf("failed to get balance: %w", err)
	}

	printAccountDetails(w, acct, bal)

	return nil
}
