package scheduled

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// scheduledDeleteOptions are the inputs to `tmoney scheduled delete <id>`.
type scheduledDeleteOptions struct {
	file string
	id   string
}

// newScheduledDeleteCmd registers `tmoney scheduled delete <id>`. The
// database file is taken from the persistent `--file` / `-f` flag inherited
// from the root command. The ID is positional, matching
// `transaction delete <id>`.
func newScheduledDeleteCmd() *cobra.Command {
	opts := &scheduledDeleteOptions{}
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a scheduled transaction by ID",
		Long: "Permanently delete a scheduled transaction template identified by " +
			"its UUID (find it with `tmoney scheduled list --show-ids`). This " +
			"removes the template only; any transactions already posted from it " +
			"remain untouched. Multi-line (split/paycheck) templates are deleted " +
			"here too (their child lines are removed with the template).",
		Example:      "  tmoney scheduled delete 0d9f7c2a-…",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.id = args[0]
			return runScheduledDelete(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runScheduledDelete deletes a scheduled transaction template by ID via
// scheduled.Service.Delete — the same path the TUI delete action uses.
func runScheduledDelete(opts *scheduledDeleteOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	id, err := types.ParseID(opts.id)
	if err != nil {
		return fmt.Errorf("invalid scheduled transaction ID: %w", err)
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	st, err := svc.Scheduled.GetByID(id)
	if err != nil {
		return fmt.Errorf("scheduled transaction %s not found", opts.id)
	}

	if err := svc.Scheduled.Delete(id); err != nil {
		return fmt.Errorf("failed to delete scheduled transaction: %w", err)
	}

	printScheduledSummary(w, svc, "Scheduled transaction deleted successfully!", st)

	cmdutil.AutoBackupAfterModification(database)
	return nil
}
