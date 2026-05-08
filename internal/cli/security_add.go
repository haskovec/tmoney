package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/security"
	"github.com/spf13/cobra"
)

// securityAddOptions are the inputs to `tmoney security add`.
type securityAddOptions struct {
	file       string
	ticker     string
	name       string
	secType    string
	assetClass string
	currency   string
	exchange   string
}

// newSecurityAddCmd registers `tmoney security add`. The database file
// is taken from the persistent `--file` / `-f` flag inherited from the
// root command. `--ticker`, `--name`, and `--type` are required.
func newSecurityAddCmd() *cobra.Command {
	opts := &securityAddOptions{}
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a new security",
		Long: "Create a new security in the TMoney database. " +
			"`--ticker`, `--name`, and `--type` are required; other fields take sensible defaults.",
		Example:      "  tmoney security add --ticker AAPL --name \"Apple Inc.\" --type stock --exchange NASDAQ",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runSecurityAdd(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.ticker, "ticker", "", "Ticker symbol (required)")
	cmd.Flags().StringVar(&opts.name, "name", "", "Security name (required)")
	cmd.Flags().StringVar(&opts.secType, "type", "", "Security type: stock, etf, mutual_fund, other (required)")
	cmd.Flags().StringVar(&opts.assetClass, "asset-class", "", "Asset class (default unclassified)")
	cmd.Flags().StringVar(&opts.currency, "currency", "", "Currency code (default USD)")
	cmd.Flags().StringVar(&opts.exchange, "exchange", "", "Exchange (e.g. NASDAQ, NYSE)")
	_ = cmd.MarkFlagRequired("ticker")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

// runSecurityAdd creates a new security.
func runSecurityAdd(opts *securityAddOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
	}

	secType, err := security.ParseType(opts.secType)
	if err != nil {
		return fmt.Errorf("invalid --type: %w", err)
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec := security.NewSecurity(opts.ticker, opts.name, secType)

	if opts.assetClass != "" {
		ac, err := security.ParseAssetClass(opts.assetClass)
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

	if err := svc.Security.Create(sec); err != nil {
		return fmt.Errorf("failed to create security: %w", err)
	}

	fmt.Fprintln(w, "Security created successfully!")
	fmt.Fprintf(w, "  Ticker:      %s\n", sec.Ticker)
	fmt.Fprintf(w, "  Name:        %s\n", sec.Name)
	fmt.Fprintf(w, "  Type:        %s\n", sec.SecurityType.DisplayName())
	fmt.Fprintf(w, "  Asset Class: %s\n", sec.AssetClass.DisplayName())
	fmt.Fprintf(w, "  Currency:    %s\n", sec.Currency)
	if sec.Exchange.Valid {
		fmt.Fprintf(w, "  Exchange:    %s\n", sec.Exchange.String)
	}

	autoBackupAfterModification(opts.file)
	return nil
}
