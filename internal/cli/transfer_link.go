package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/haskovec/tmoney/internal/transferlink"
)

// runLinkTransfers handles the --link-transfers command. By default it
// performs a dry-run preview of the candidate pairs that would be linked;
// passing --confirm executes the linking.
func runLinkTransfers(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--link-transfers requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	maxDays := opts.maxDateDiffDays
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

	autoBackupAfterModification(opts.file)
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
