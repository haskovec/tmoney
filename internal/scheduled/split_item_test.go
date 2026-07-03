package scheduled

import (
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

// TestScheduledSplitValidate pins the migration-029 relaxation on the
// scheduled-split model: a line must carry a category, a transfer target, or
// both, and a categorized transfer (both set) is now valid.
func TestScheduledSplitValidate(t *testing.T) {
	schedID := types.NewID()
	amt := types.MustNewMoney("-100.00")

	t.Run("categorized line passes", func(t *testing.T) {
		s := NewCategorizedSplit(schedID, types.NewID(), amt)
		if errs := s.Validate(); errs.HasErrors() {
			t.Errorf("categorized scheduled split should validate: %v", errs)
		}
	})

	t.Run("transfer line passes", func(t *testing.T) {
		s := NewTransferSplit(schedID, types.NewID(), amt)
		if errs := s.Validate(); errs.HasErrors() {
			t.Errorf("transfer scheduled split should validate: %v", errs)
		}
	})

	t.Run("categorized transfer (both set) passes post-029", func(t *testing.T) {
		s := NewTransferSplit(schedID, types.NewID(), amt)
		s.CategoryID = types.NullableID{ID: types.NewID(), Valid: true}
		if errs := s.Validate(); errs.HasErrors() {
			t.Errorf("a categorized transfer scheduled split should validate: %v", errs)
		}
	})

	t.Run("neither category nor transfer fails", func(t *testing.T) {
		s := &Split{
			BaseModel:              types.NewBaseModel(),
			ScheduledTransactionID: schedID,
			Amount:                 amt,
		}
		if errs := s.Validate(); !errs.HasErrors() {
			t.Error("a split with neither category nor transfer should fail validation")
		}
	})

	t.Run("zero amount fails", func(t *testing.T) {
		s := NewCategorizedSplit(schedID, types.NewID(), types.ZeroMoney)
		if errs := s.Validate(); !errs.HasErrors() {
			t.Error("zero amount should fail validation")
		}
	})
}
