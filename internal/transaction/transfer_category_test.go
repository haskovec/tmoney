package transaction

import (
	"errors"
	"testing"

	"github.com/haskovec/tmoney/internal/category"
)

func TestValidateTransferCategory(t *testing.T) {
	t.Run("nil category is allowed (transfer with no label)", func(t *testing.T) {
		if err := ValidateTransferCategory(nil); err != nil {
			t.Errorf("nil category should be allowed, got %v", err)
		}
	})

	t.Run("non-system category is allowed", func(t *testing.T) {
		cat := category.NewCategory("Bills", category.TypeExpense)
		if err := ValidateTransferCategory(cat); err != nil {
			t.Errorf("non-system category should be allowed, got %v", err)
		}
	})

	t.Run("non-system income category is allowed", func(t *testing.T) {
		cat := category.NewCategory("Reimbursement", category.TypeIncome)
		if err := ValidateTransferCategory(cat); err != nil {
			t.Errorf("non-system income category should be allowed, got %v", err)
		}
	})

	t.Run("system Transfer category is rejected", func(t *testing.T) {
		cat := category.NewSystemCategory(category.TransferCategoryName, category.TypeExpense)
		err := ValidateTransferCategory(cat)
		if err == nil {
			t.Fatal("system Transfer category should be rejected")
		}
		var sysErr *SystemCategoryTransferError
		if !errors.As(err, &sysErr) {
			t.Errorf("expected *SystemCategoryTransferError, got %T (%v)", err, err)
		}
	})

	t.Run("system Value Adjustment category is rejected", func(t *testing.T) {
		cat := category.NewSystemCategory(category.ValueAdjustmentCategoryName, category.TypeExpense)
		if err := ValidateTransferCategory(cat); err == nil {
			t.Fatal("system Value Adjustment category should be rejected")
		}
	})
}
