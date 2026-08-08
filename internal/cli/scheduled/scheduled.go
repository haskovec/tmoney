package scheduled

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/app"
	scheduleddom "github.com/haskovec/tmoney/internal/scheduled"
	xfer "github.com/haskovec/tmoney/internal/transfer"
	"github.com/spf13/cobra"
)

// NewCmd returns the `scheduled` parent command. Child verbs are
// registered as they migrate from the legacy `--flag` dispatcher.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scheduled",
		Short: "Manage TMoney scheduled transactions",
		Long: "Subcommands for adding, listing, posting, and skipping " +
			"scheduled transactions on an account.",
		Example: "  tmoney scheduled add --account Checking --frequency monthly --amount -1500 --payee Landlord",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		SilenceUsage: true,
	}
	cmd.AddCommand(newScheduledAddCmd())
	cmd.AddCommand(newScheduledDeleteCmd())
	cmd.AddCommand(newScheduledEditCmd())
	cmd.AddCommand(newScheduledListCmd())
	cmd.AddCommand(newScheduledPostCmd())
	cmd.AddCommand(newScheduledSkipCmd())
	return cmd
}

// refuseUnsupportedTransferCategory rejects a category on a transfer schedule
// whose two endpoints cannot store one — an investment-to-investment pair, whose
// legs both live in investment_transactions.
//
// It CALLS the domain predicate rather than restating the rule, matching
// cli/transfer/add.go, so there is one implementation of "which pairs can hold a
// category". Such a schedule is not merely mislabelled: it can never post, and
// in an auto-post batch its refusal used to abort every other schedule too.
func refuseUnsupportedTransferCategory(svc *app.Services, st *scheduleddom.Transaction) error {
	if !st.IsTransfer() {
		return nil
	}
	from, err := svc.AccountRepo.GetByID(st.AccountID)
	if err != nil {
		return fmt.Errorf("failed to load the source account: %w", err)
	}
	to, err := svc.AccountRepo.GetByID(st.TransferAccountID.ID)
	if err != nil {
		return fmt.Errorf("failed to load the transfer destination account: %w", err)
	}
	if !xfer.ClassifyKind(from.Type, to.Type).StoresCategory() {
		return fmt.Errorf("--category is not supported for investment-to-investment transfers")
	}
	return nil
}
