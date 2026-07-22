package investment

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	investmentdom "github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// investmentActionsOptions are the inputs to `tmoney investment actions`.
type investmentActionsOptions struct {
	file       string
	ticker     string
	isin       string
	name       string
	actionType string
	showIDs    bool
}

// newInvestmentActionsCmd registers `tmoney investment actions`, the
// read-only CLI counterpart of the TUI's corporate-action history view.
// The database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command.
func newInvestmentActionsCmd() *cobra.Command {
	opts := &investmentActionsOptions{}
	cmd := &cobra.Command{
		Use:   "actions",
		Short: "List recorded corporate actions (splits, mergers, spin-offs)",
		Long: "List recorded corporate actions (stock splits, reverse splits, " +
			"mergers, and spin-offs), newest first. Optionally filter to a single " +
			"security with --ticker / --isin / --name, or to one action type with " +
			"--type. Pass --show-ids to print each action's UUID. This is a " +
			"read-only view; use `investment split`, `investment merge`, and " +
			"`investment spin-off` to record actions.",
		Example: "  tmoney investment actions\n" +
			"  tmoney investment actions --ticker AAPL\n" +
			"  tmoney investment actions --type merger --show-ids",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runInvestmentActions(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.ticker, "ticker", "", "Filter to one security by ticker (or use --isin / --name)")
	cmd.Flags().StringVar(&opts.actionType, "type", "", "Filter by action type (split, reverse_split, merger, spin_off)")
	cmd.Flags().BoolVar(&opts.showIDs, "show-ids", false, "Prefix each row with the action's UUID")
	cmdutil.AddSecuritySelectorFlags(cmd, &opts.isin, &opts.name)
	return cmd
}

// runInvestmentActions lists recorded corporate actions.
func runInvestmentActions(opts *investmentActionsOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	var typeFilter *investmentdom.ActionType
	if opts.actionType != "" {
		at, err := investmentdom.ParseActionType(opts.actionType)
		if err != nil {
			return fmt.Errorf("invalid --type %q (valid types: split, reverse_split, merger, spin_off)", opts.actionType)
		}
		typeFilter = &at
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	var actions []*investmentdom.CorporateAction
	if opts.ticker != "" || opts.isin != "" || opts.name != "" {
		sec, err := svc.Security.Resolve(opts.ticker, opts.isin, opts.name)
		if err != nil {
			return err
		}
		actions, err = svc.CorporateAction.ListBySecurity(sec.ID)
		if err != nil {
			return fmt.Errorf("failed to list corporate actions: %w", err)
		}
	} else {
		actions, err = svc.CorporateAction.ListAll()
		if err != nil {
			return fmt.Errorf("failed to list corporate actions: %w", err)
		}
	}

	if typeFilter != nil {
		filtered := actions[:0:0]
		for _, ca := range actions {
			if ca.ActionType == *typeFilter {
				filtered = append(filtered, ca)
			}
		}
		actions = filtered
	}

	securities, err := svc.SecurityRepo.List(security.Filter{})
	if err != nil {
		return fmt.Errorf("failed to list securities: %w", err)
	}
	labels := make(map[types.ID]string, len(securities))
	for _, sec := range securities {
		labels[sec.ID] = securityLabel(sec)
	}

	fmt.Fprintln(w, "CORPORATE ACTIONS")
	fmt.Fprintln(w, strings.Repeat("=", len("CORPORATE ACTIONS")))

	if len(actions) == 0 {
		fmt.Fprintln(w, "No corporate actions found.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if opts.showIDs {
		fmt.Fprintln(tw, "ID\tDate\tTicker\tType\tDetails")
	} else {
		fmt.Fprintln(tw, "Date\tTicker\tType\tDetails")
	}
	for _, ca := range actions {
		row := fmt.Sprintf("%s\t%s\t%s\t%s",
			ca.ActionDate.String(),
			actionSecurityLabel(ca.SecurityID, labels),
			ca.ActionType.DisplayName(),
			formatActionDetails(ca, labels),
		)
		if opts.showIDs {
			row = ca.ID.String() + "\t" + row
		}
		fmt.Fprintln(tw, row)
	}
	return tw.Flush()
}

// actionSecurityLabel returns the ticker-or-name label for a security ID,
// or "???" when it cannot be resolved.
func actionSecurityLabel(id types.ID, labels map[types.ID]string) string {
	if label, ok := labels[id]; ok {
		return label
	}
	return "???"
}

// formatActionDetails formats a corporate action's parameters into a
// human-readable string, mirroring the TUI's history view. It falls back to
// the raw parameter JSON on a parse error.
func formatActionDetails(ca *investmentdom.CorporateAction, labels map[types.ID]string) string {
	switch ca.ActionType {
	case investmentdom.ActionTypeSplit, investmentdom.ActionTypeReverseSplit:
		params, err := investmentdom.ParseSplitParams(ca.Parameters)
		if err != nil {
			return ca.Parameters
		}
		return fmt.Sprintf("Ratio %s", params.RatioString())

	case investmentdom.ActionTypeMerger:
		params, err := investmentdom.ParseMergerParams(ca.Parameters)
		if err != nil {
			return ca.Parameters
		}
		target := actionTargetLabel(ca.TargetSecurityID, labels)
		if params.HasCashConsideration() {
			return fmt.Sprintf("→ %s, ratio %.2f, cash $%.2f/sh", target, params.ExchangeRatio, params.CashPerShare)
		}
		return fmt.Sprintf("→ %s, ratio %.2f", target, params.ExchangeRatio)

	case investmentdom.ActionTypeSpinOff:
		params, err := investmentdom.ParseSpinOffParams(ca.Parameters)
		if err != nil {
			return ca.Parameters
		}
		target := actionTargetLabel(ca.TargetSecurityID, labels)
		return fmt.Sprintf("→ %s, ratio %.2f, parent %.1f%%", target, params.ShareRatio, params.ParentAllocationPct)
	}
	return ca.Parameters
}

// actionTargetLabel resolves a nullable target-security ID to its label,
// returning "???" when absent or unresolvable.
func actionTargetLabel(id types.NullableID, labels map[types.ID]string) string {
	if !id.Valid {
		return "???"
	}
	return actionSecurityLabel(id.ID, labels)
}
