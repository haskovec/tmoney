package transaction

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/category"
)

// SystemCategoryTransferError is returned when a system category (Transfer,
// Value Adjustment) is assigned as a transfer's label. System categories are
// reserved and may never label a transfer.
type SystemCategoryTransferError struct {
	Name string
}

func (e *SystemCategoryTransferError) Error() string {
	return fmt.Sprintf("category %q is a system category and cannot label a transfer", e.Name)
}

// ValidateTransferCategory reports whether cat may label a transfer.
//
// A transfer category is always optional — a nil cat (no category assigned) is
// valid — and must be a non-system category: the system categories (Transfer,
// Value Adjustment) are reserved and may never label a transfer. Any path that
// assigns a category to a transfer — whole-transaction transfers, scheduled
// transfers, transfer-line splits, and the loan wizard's principal line — is
// expected to resolve the category and call this before persisting; those
// assignment paths land in later phases. Category existence is enforced
// separately (by the category_id FK and the split repositories'
// verifyReferences); this guard adds only the non-system rule.
func ValidateTransferCategory(cat *category.Category) error {
	if cat == nil || !cat.IsSystem {
		return nil
	}
	return &SystemCategoryTransferError{Name: cat.Name}
}
