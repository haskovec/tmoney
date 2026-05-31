package security

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	securitydom "github.com/haskovec/tmoney/internal/security"
	"github.com/spf13/cobra"
)

// securityEditOptions are the inputs to `tmoney security edit`.
type securityEditOptions struct {
	file       string
	lookup     string // positional ticker (the security to edit)
	newTicker  string // --ticker (rename)
	name       string
	secType    string
	assetClass string
	currency   string
	exchange   string
}

// newSecurityEditCmd registers `tmoney security edit <ticker>`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. The positional `<ticker>` selects
// the security to edit; only fields whose flag is supplied are
// updated. Pass `--ticker` to rename the security to a new ticker.
func newSecurityEditCmd() *cobra.Command {
	opts := &securityEditOptions{}
	cmd := &cobra.Command{
		Use:   "edit <ticker>",
		Short: "Edit fields of an existing security",
		Long: "Edit fields on an existing security identified by ticker. " +
			"Only flags that are supplied take effect; other fields are left as-is. " +
			"Pass `--ticker` to rename the security to a new symbol.",
		Example: "  tmoney security edit AAPL --name \"Apple Corporation\"\n" +
			"  tmoney security edit AAPL --ticker AAPL2\n" +
			"  tmoney security edit VTI --asset-class total_market",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.lookup = args[0]
			return runSecurityEdit(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.newTicker, "ticker", "", "New ticker symbol (rename)")
	cmd.Flags().StringVar(&opts.name, "name", "", "New security name")
	cmd.Flags().StringVar(&opts.secType, "type", "", "New security type: stock, etf, mutual_fund, other")
	cmd.Flags().StringVar(&opts.assetClass, "asset-class", "", "New asset class")
	cmd.Flags().StringVar(&opts.currency, "currency", "", "New currency code")
	cmd.Flags().StringVar(&opts.exchange, "exchange", "", "New exchange")
	return cmd
}

// runSecurityEdit edits an existing security.
func runSecurityEdit(opts *securityEditOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.lookup, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.lookup)
	}

	if opts.newTicker != "" {
		sec.Ticker = opts.newTicker
	}
	if opts.name != "" {
		sec.Name = opts.name
	}
	if opts.secType != "" {
		secType, err := securitydom.ParseType(opts.secType)
		if err != nil {
			return fmt.Errorf("invalid --type: %w", err)
		}
		sec.SecurityType = secType
	}
	if opts.assetClass != "" {
		ac, err := securitydom.ParseAssetClass(opts.assetClass)
		if err != nil {
			return fmt.Errorf("invalid --asset-class: %w", err)
		}
		sec.AssetClass = ac
	}
	if opts.currency != "" {
		sec.Currency = opts.currency
	}
	if opts.exchange != "" {
		sec.SetExchange(opts.exchange)
	}

	if err := svc.Security.Update(sec); err != nil {
		return fmt.Errorf("failed to update security: %w", err)
	}

	fmt.Fprintln(w, "Security updated successfully!")
	fmt.Fprintf(w, "  Ticker:      %s\n", sec.Ticker)
	fmt.Fprintf(w, "  Name:        %s\n", sec.Name)
	fmt.Fprintf(w, "  Type:        %s\n", sec.SecurityType.DisplayName())
	fmt.Fprintf(w, "  Asset Class: %s\n", sec.AssetClass.DisplayName())
	fmt.Fprintf(w, "  Currency:    %s\n", sec.Currency)

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
