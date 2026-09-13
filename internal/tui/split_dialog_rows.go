package tui

import (
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
)

// The row model: the running remainder, adding and removing rows, validation,
// and the transaction.Split rows the editor hands back on save.

// remaining calculates totalAmount minus the sum of entered split amounts.
func (sd *SplitDialog) remaining() types.Money {
	sum := types.ZeroMoney
	for _, row := range sd.rows {
		amt := strings.TrimSpace(row.amountField.Value)
		if amt == "" {
			continue
		}
		m, err := parseAmountInput(amt)
		if err != nil {
			continue
		}
		sum = sum.Add(m)
	}
	return sd.totalAmount.Sub(sum)
}

// addRow appends a new empty split row.
func (sd *SplitDialog) addRow() {
	sd.rows = append(sd.rows, splitRow{
		categoryIndex: 0,
		amountField: dialog.Field{
			Type:        dialog.FieldText,
			Placeholder: "0.00",
			Width:       12,
		},
		memoField: dialog.Field{
			Type:        dialog.FieldText,
			Placeholder: "Memo",
		},
	})
}

// removeRow removes the row at the given index.
func (sd *SplitDialog) removeRow(index int) {
	if len(sd.rows) <= 1 || index < 0 || index >= len(sd.rows) {
		return
	}
	sd.rows = append(sd.rows[:index], sd.rows[index+1:]...)
	// Adjust focused row if needed
	if sd.rowIndex >= len(sd.rows) {
		sd.rowIndex = len(sd.rows) - 1
	}
}

// validate checks that all splits are valid and sum to the total.
func (sd *SplitDialog) validate() error {
	if len(sd.rows) == 0 {
		return fmt.Errorf("at least one split is required")
	}

	for i, row := range sd.rows {
		if row.transferMode {
			if row.accountIndex < 0 || row.accountIndex >= len(sd.transferAccountIDs) {
				return fmt.Errorf("split %d: pick a destination account for the transfer", i+1)
			}
		} else {
			// Category must be selected (index > 0 means not "(None)")
			if row.categoryIndex <= 0 {
				return fmt.Errorf("split %d: category is required", i+1)
			}
			// The AddNew sentinel is an action row, not a saveable
			// selection — landing here without activating it (Enter on the
			// Category field) is treated as "no category picked".
			if sd.isAddNewSentinel(row.categoryIndex) {
				return fmt.Errorf("split %d: category is required", i+1)
			}
			// The Transfer sentinel without a configured picker is not a
			// savable selection on its own. Once SetTransferTargets has
			// been called, hitting the sentinel transitions the row into
			// transfer mode (handled above).
			if sd.isTransferSentinel(row.categoryIndex) {
				return fmt.Errorf("split %d: pick a destination account for the transfer", i+1)
			}
		}

		// Amount must be present and valid
		amt := strings.TrimSpace(row.amountField.Value)
		if amt == "" {
			return fmt.Errorf("split %d: amount is required", i+1)
		}
		if _, err := parseAmountInput(amt); err != nil {
			return fmt.Errorf("split %d: invalid amount", i+1)
		}
	}

	// Check sum matches total
	rem := sd.remaining()
	if !rem.IsZero() {
		return fmt.Errorf("splits must sum to %s (remaining: %s)",
			formatDashboardMoney(sd.totalAmount), formatDashboardMoney(rem))
	}

	return nil
}

// buildSplits produces Split structs from the current rows.
//
// Transfer-line rows produce a Split with TransferAccountID set to the picked
// account; TransferID is left empty because the service layer mints it when
// the parent transaction is created (see MS-006). A seeded transfer line's
// category is carried through (row.seedTransferCategoryID) so an existing
// categorized transfer survives an edit round-trip; a freshly-created transfer
// row has no category (NilID).
func (sd *SplitDialog) buildSplits() ([]*transaction.Split, error) {
	if err := sd.validate(); err != nil {
		return nil, err
	}

	var splits []*transaction.Split
	for _, row := range sd.rows {
		amount, _ := parseAmountInput(row.amountField.Value)
		memo := strings.TrimSpace(row.memoField.Value)

		var split *transaction.Split
		if row.transferMode {
			split = &transaction.Split{
				BaseModel:     types.NewBaseModel(),
				TransactionID: types.NilID,
				CategoryID:    row.seedTransferCategoryID,
				Amount:        amount,
				TransferAccountID: types.NullableID{
					ID:    sd.transferAccountIDs[row.accountIndex],
					Valid: true,
				},
			}
			if memo != "" {
				split.SetMemo(memo)
			}
		} else {
			categoryID := sd.categoryIDs[row.categoryIndex]
			if memo != "" {
				split = transaction.NewSplitWithMemo(types.NilID, categoryID, amount, memo)
			} else {
				split = transaction.NewSplit(types.NilID, categoryID, amount)
			}
		}
		// Carry a seeded line's paycheck_section through unchanged; a row
		// added in this dialog leaves it NULL.
		split.PaycheckSection = row.seedPaycheckSection
		splits = append(splits, split)
	}

	return splits, nil
}
