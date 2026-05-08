package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/security"
	"github.com/spf13/cobra"
)

// securityListOptions are the inputs to `tmoney security list`.
type securityListOptions struct {
	file          string
	includeHidden bool
	secType       string
	assetClass    string
}

// newSecurityListCmd registers `tmoney security list`. The database
// file is taken from the persistent `--file` / `-f` flag inherited
// from the root command.
func newSecurityListCmd() *cobra.Command {
	opts := &securityListOptions{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List securities",
		Long: "List securities tracked by TMoney. By default, hidden " +
			"securities are excluded; pass `--include-hidden` to show " +
			"them. Filter by `--type` or `--asset-class`.",
		Example: "  tmoney security list\n" +
			"  tmoney security list --include-hidden\n" +
			"  tmoney security list --type etf",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runSecurityList(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&opts.includeHidden, "include-hidden", false, "Include hidden securities in the listing")
	cmd.Flags().StringVar(&opts.secType, "type", "", "Filter by type (stock, etf, mutual_fund, other)")
	cmd.Flags().StringVar(&opts.assetClass, "asset-class", "", "Filter by asset class")
	return cmd
}

// runSecurityList lists securities from the database.
func runSecurityList(opts *securityListOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	filter := security.Filter{}
	if !opts.includeHidden {
		excludeHidden := true
		filter.ExcludeHidden = &excludeHidden
	}
	if opts.secType != "" {
		secType, err := security.ParseType(opts.secType)
		if err != nil {
			return fmt.Errorf("invalid --type: %w", err)
		}
		filter.SecurityType = &secType
	}
	if opts.assetClass != "" {
		ac, err := security.ParseAssetClass(opts.assetClass)
		if err != nil {
			return fmt.Errorf("invalid --asset-class: %w", err)
		}
		filter.AssetClass = &ac
	}

	securities, err := svc.Security.List(filter)
	if err != nil {
		return fmt.Errorf("failed to list securities: %w", err)
	}

	printSecuritiesTable(w, securities)
	return nil
}
