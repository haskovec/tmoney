package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/imexport"
	"github.com/spf13/cobra"
)

// importOptions are the inputs to `tmoney import <file>`.
type importOptions struct {
	file             string
	importFile       string
	account          string
	sourceAccount    string
	formatOverride   string
	confirm          bool
	skipDuplicates   bool
	updateDuplicates bool
}

// newImportCmd registers `tmoney import <file>`. The database file is
// taken from the persistent `--file` / `-f` flag inherited from the
// root command. `--account` is required; the format is auto-detected
// from the extension unless `--format` is supplied.
func newImportCmd() *cobra.Command {
	opts := &importOptions{}
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import transactions from CSV, QIF, or OFX/QFX",
		Long: "Import transactions from a CSV, QIF, or OFX/QFX file into a target account. " +
			"By default the command runs as a dry-run preview; pass --confirm to write changes. " +
			"For multi-account CSVs (e.g. Quicken Mac's Register Transactions export), " +
			"pass --source-account to choose which source account to import this pass.",
		Example: "  tmoney -f personal.tdb import statements.qif --account Checking\n" +
			"  tmoney -f personal.tdb import bank.csv --account Checking --confirm\n" +
			"  tmoney -f personal.tdb import register.csv --account \"BoA Checking\" \\\n" +
			"    --source-account \"Checking\" --confirm",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.importFile = args[0]
			return runImport(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Target account to import into (required)")
	cmd.Flags().StringVar(&opts.sourceAccount, "source-account", "", "Source account name when the file covers multiple accounts")
	cmd.Flags().StringVar(&opts.formatOverride, "format", "", "Override format detection (csv, qif, or ofx)")
	cmd.Flags().BoolVar(&opts.confirm, "confirm", false, "Execute the import (default is dry-run preview)")
	cmd.Flags().BoolVar(&opts.skipDuplicates, "skip-duplicates", false, "Skip rows that match existing transactions")
	cmd.Flags().BoolVar(&opts.updateDuplicates, "update-duplicates", false, "Update existing transactions when matched")
	_ = cmd.MarkFlagRequired("account")
	return cmd
}

// runImport handles `tmoney import <file>`.
func runImport(opts *importOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
	}
	if opts.skipDuplicates && opts.updateDuplicates {
		return fmt.Errorf("--skip-duplicates and --update-duplicates are mutually exclusive")
	}

	// Detect or override format
	var format imexport.Format
	if opts.formatOverride != "" {
		switch strings.ToLower(opts.formatOverride) {
		case "csv":
			format = imexport.FormatCSV
		case "qif":
			format = imexport.FormatQIF
		case "ofx", "qfx":
			format = imexport.FormatOFX
		default:
			return fmt.Errorf("unsupported --format value %q (must be csv, qif, or ofx)", opts.formatOverride)
		}
	} else {
		var err error
		format, err = imexport.DetectFormat(opts.importFile)
		if err != nil {
			return fmt.Errorf("cannot detect format: %w\nUse --format to specify the format explicitly", err)
		}
	}

	// Open the import file
	file, err := os.Open(opts.importFile)
	if err != nil {
		return fmt.Errorf("failed to open import file: %w", err)
	}
	defer file.Close()

	// Open database and services
	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Resolve the target account
	account, err := svc.Account.GetByName(opts.account)
	if err != nil {
		return fmt.Errorf("account %q not found: %w", opts.account, err)
	}
	if !account.Active {
		return fmt.Errorf("account %q is closed; cannot import into a closed account", opts.account)
	}

	// Determine duplicate handling
	dupHandling := imexport.DuplicateHandlingNone
	if opts.skipDuplicates {
		dupHandling = imexport.DuplicateHandlingSkip
	} else if opts.updateDuplicates {
		dupHandling = imexport.DuplicateHandlingUpdate
	}

	// Create import service with adapters
	importSvc := imexport.NewImportService(
		imexport.NewServiceCategoryResolver(svc.Category),
		imexport.NewServicePayeeResolver(svc.Payee),
		imexport.NewRepoTransactionStore(svc.TransactionRepo, svc.PayeeRepo),
		imexport.NewServiceTransactionCreator(svc.Transaction),
	)

	// Parse the file once, then check whether it contains rows for more
	// than one source account (Quicken Mac's "Register Transactions to
	// CSV" emits a single file covering every account). If so, the user
	// must pick which one to import via --source-account.
	parseResult, err := importSvc.Parse(file, format)
	if err != nil {
		return fmt.Errorf("import parse failed: %w", err)
	}
	sources := imexport.DistinctAccounts(parseResult)
	if len(sources) > 1 && opts.sourceAccount == "" {
		return fmt.Errorf("import file contains transactions for %d accounts: %s\n"+
			"Pass --source-account \"<name>\" to choose which one to import (run once per account)",
			len(sources), strings.Join(sources, ", "))
	}
	if opts.sourceAccount != "" {
		if len(sources) > 0 && !slices.Contains(sources, opts.sourceAccount) {
			return fmt.Errorf("source account %q not found in import file (available: %s)",
				opts.sourceAccount, strings.Join(sources, ", "))
		}
		parseResult = imexport.FilterByAccount(parseResult, opts.sourceAccount)
	}

	// Run preview from the (possibly filtered) records
	importOpts := imexport.ImportOptions{
		Format:            format,
		DuplicateHandling: dupHandling,
	}
	result, err := importSvc.PreviewRecords(parseResult, account.ID, importOpts)
	if err != nil {
		return fmt.Errorf("import preview failed: %w", err)
	}

	// If not confirming, show dry-run summary
	if !opts.confirm {
		printImportPreview(w, opts.importFile, opts.account, result)
		return nil
	}

	// Execute the import
	if err := importSvc.Execute(result, account.ID); err != nil {
		return fmt.Errorf("import execution failed: %w", err)
	}

	// Print execution summary
	printImportResult(w, opts.importFile, opts.account, result)

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}

// printImportPreview prints the dry-run import summary.
func printImportPreview(w io.Writer, importFile, accountName string, result *imexport.ImportResult) {
	fmt.Fprintf(w, "IMPORT PREVIEW: %s → %s\n", filepath.Base(importFile), accountName)
	fmt.Fprintln(w, strings.Repeat("=", 44))
	fmt.Fprintf(w, "Parsed: %d transactions\n", len(result.Rows))
	fmt.Fprintf(w, "  New:      %3d transactions (will be created)\n", result.NewCount())
	fmt.Fprintf(w, "  Matched:  %3d transactions (will be updated)\n", result.MatchCount())
	fmt.Fprintf(w, "  Review:   %3d transactions (low-confidence match)\n", result.ReviewCount())
	fmt.Fprintf(w, "  Skipped:  %3d transactions (duplicates)\n", result.SkipCount())

	if len(result.Rows) > 0 {
		fmt.Fprintf(w, "\nDate range: %s to %s\n", result.DateFrom.String(), result.DateTo.String())
		fmt.Fprintf(w, "Total amount: $%.2f\n", result.TotalAmount().Float64())
	}

	if len(result.Errors) > 0 {
		fmt.Fprintf(w, "\nWarnings:\n")
		for _, e := range result.Errors {
			fmt.Fprintf(w, "  - %s\n", e)
		}
	}

	fmt.Fprintln(w, "\nRun with --confirm to execute the import.")
}

// printImportResult prints the import execution summary.
func printImportResult(w io.Writer, importFile, accountName string, result *imexport.ImportResult) {
	fmt.Fprintf(w, "IMPORT COMPLETE: %s → %s\n", filepath.Base(importFile), accountName)
	fmt.Fprintln(w, strings.Repeat("=", 45))
	fmt.Fprintf(w, "Created:  %d new transactions\n", result.Created)
	fmt.Fprintf(w, "Updated:  %d existing transactions\n", result.Updated)
	fmt.Fprintf(w, "Skipped:  %d duplicates\n", result.Skipped)

	if len(result.Errors) > 0 {
		fmt.Fprintf(w, "\nErrors:\n")
		for _, e := range result.Errors {
			fmt.Fprintf(w, "  - %s\n", e)
		}
	}
}
