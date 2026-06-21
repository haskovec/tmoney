package security

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	securitydom "github.com/haskovec/tmoney/internal/security"
	"github.com/spf13/cobra"
)

// securityAddOptions are the inputs to `tmoney security add`.
type securityAddOptions struct {
	file       string
	ticker     string
	name       string
	isin       string
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
			"`--name` and `--type` are required; `--ticker` is optional (a security may " +
			"have a name but no ticker, e.g. a collective trust held in a 401k). " +
			"Other fields take sensible defaults.",
		Example: "  tmoney security add --ticker AAPL --name \"Apple Inc.\" --type stock --exchange NASDAQ\n" +
			"  tmoney security add --name \"MFS Mid Cap Value CT\" --type other --isin US0378331005",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runSecurityAdd(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.ticker, "ticker", "", "Ticker symbol (optional)")
	cmd.Flags().StringVar(&opts.name, "name", "", "Security name (required)")
	cmd.Flags().StringVar(&opts.isin, "isin", "", "ISIN identifier (optional, ISO 6166)")
	cmd.Flags().StringVar(&opts.secType, "type", "", "Security type: stock, etf, mutual_fund, other (required)")
	cmd.Flags().StringVar(&opts.assetClass, "asset-class", "", "Asset class (default unclassified)")
	cmd.Flags().StringVar(&opts.currency, "currency", "", "Currency code (default USD)")
	cmd.Flags().StringVar(&opts.exchange, "exchange", "", "Exchange (e.g. NASDAQ, NYSE)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

// runSecurityAdd creates a new security.
func runSecurityAdd(opts *securityAddOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	secType, err := securitydom.ParseType(opts.secType)
	if err != nil {
		return fmt.Errorf("invalid --type: %w", err)
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec := securitydom.NewSecurity(opts.ticker, opts.name, secType)

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

	if opts.isin != "" {
		sec.SetISIN(opts.isin)
	}

	if err := svc.Security.Create(sec); err != nil {
		return fmt.Errorf("failed to create security: %w", err)
	}

	fmt.Fprintln(w, "Security created successfully!")
	if sec.Ticker != "" {
		fmt.Fprintf(w, "  Ticker:      %s\n", sec.Ticker)
	}
	fmt.Fprintf(w, "  Name:        %s\n", sec.Name)
	if sec.ISIN != "" {
		fmt.Fprintf(w, "  ISIN:        %s\n", sec.ISIN)
	}
	fmt.Fprintf(w, "  Type:        %s\n", sec.SecurityType.DisplayName())
	fmt.Fprintf(w, "  Asset Class: %s\n", sec.AssetClass.DisplayName())
	fmt.Fprintf(w, "  Currency:    %s\n", sec.Currency)
	if sec.Exchange.Valid {
		fmt.Fprintf(w, "  Exchange:    %s\n", sec.Exchange.String)
	}

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
