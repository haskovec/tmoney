package transfer

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/transferlink"
	"github.com/spf13/cobra"
)

// transferLinkOptions are the inputs to `tmoney transfer link`.
type transferLinkOptions struct {
	file    string
	confirm bool
	maxDays int
}

// newTransferLinkCmd registers `tmoney transfer link`. The database file
// is taken from the persistent `--file` / `-f` flag inherited from the
// root command. Default behavior is a dry-run preview; `--confirm`
// executes the linking.
func newTransferLinkCmd() *cobra.Command {
	opts := &transferLinkOptions{}
	cmd := &cobra.Command{
		Use:   "link",
		Short: "Link unlinked transfer pairs across accounts",
		Long: "Scan for pairs of unlinked transactions across accounts " +
			"whose amounts cancel and whose dates are within --max-days, " +
			"and join the matched pairs into proper transfers. By default " +
			"prints a dry-run preview; pass --confirm to apply the changes.",
		Example: "  tmoney transfer link --file personal.tdb\n" +
			"  tmoney transfer link --file personal.tdb --confirm\n" +
			"  tmoney transfer link --file personal.tdb --max-days 3 --confirm",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runTransferLink(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&opts.confirm, "confirm", false, "Execute the linking (default is dry-run preview)")
	cmd.Flags().IntVar(&opts.maxDays, "max-days", transferlink.DefaultMaxDateDiffDays,
		"Maximum days between the two postings of a candidate pair")
	return cmd
}

// runTransferLink performs a dry-run preview of candidate transfer pairs
// and, when opts.confirm is set, links the clean pairs.
func runTransferLink(opts *transferLinkOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}
	if opts.maxDays < 0 {
		return fmt.Errorf("--max-days must be a non-negative integer, got %d", opts.maxDays)
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	maxDays := opts.maxDays
	if maxDays == 0 {
		maxDays = transferlink.DefaultMaxDateDiffDays
	}

	result, err := svc.TransferLink.FindUnlinked(maxDays)
	if err != nil {
		return fmt.Errorf("scan for transfer candidates failed: %w", err)
	}

	if !opts.confirm {
		printLinkTransferPreview(w, result, maxDays)
		return nil
	}

	linked, errs := svc.TransferLink.Link(result.Clean)
	fmt.Fprintf(w, "LINK TRANSFERS COMPLETE\n")
	fmt.Fprintf(w, "%s\n", strings.Repeat("=", 40))
	fmt.Fprintf(w, "Linked:    %d pairs\n", linked)
	fmt.Fprintf(w, "Ambiguous: %d pairs (left untouched — review by hand)\n", len(result.Ambiguous))
	if len(errs) > 0 {
		fmt.Fprintf(w, "\nErrors:\n")
		for _, e := range errs {
			fmt.Fprintf(w, "  - %s\n", e)
		}
		return fmt.Errorf("%d link errors", len(errs))
	}

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}

// printLinkTransferPreview renders a dry-run summary of FindUnlinked.
func printLinkTransferPreview(w io.Writer, result *transferlink.Result, maxDays int) {
	fmt.Fprintf(w, "LINK TRANSFERS PREVIEW (window: %d days)\n", maxDays)
	fmt.Fprintf(w, "%s\n", strings.Repeat("=", 40))
	fmt.Fprintf(w, "Scanned:   %d eligible transactions\n", result.Scanned)
	fmt.Fprintf(w, "Clean:     %d pairs (will be linked)\n", len(result.Clean))
	fmt.Fprintf(w, "Ambiguous: %d pairs (need manual review)\n\n", len(result.Ambiguous))

	if len(result.Clean) > 0 {
		fmt.Fprintln(w, "Clean pairs:")
		writeCandidateTable(w, result.Clean)
	}
	if len(result.Ambiguous) > 0 {
		fmt.Fprintln(w, "\nAmbiguous pairs:")
		writeCandidateTable(w, result.Ambiguous)
	}

	if len(result.Clean) > 0 {
		fmt.Fprintf(w, "\nRun with --confirm to link the %d clean pairs.\n", len(result.Clean))
	} else {
		fmt.Fprintln(w, "\nNothing to link.")
	}
}

func writeCandidateTable(w io.Writer, cs []*transferlink.Candidate) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  From date\tFrom account\tAmount\tTo date\tTo account\tΔ days")
	for _, c := range cs {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%d\n",
			c.From.Date.String(),
			c.FromAccount,
			c.From.Amount.String(),
			c.To.Date.String(),
			c.ToAccount,
			c.DateDiffDays,
		)
	}
	tw.Flush()
}
