package scheduled

import (
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

func TestSplitCollectionClone(t *testing.T) {
	orig := SplitCollection{
		NewCategorizedSplit(types.NewID(), types.NewID(), types.MustNewMoney("-10")),
		NewCategorizedSplit(types.NewID(), types.NewID(), types.MustNewMoney("-20")),
	}

	clone := orig.Clone()
	if len(clone) != len(orig) {
		t.Fatalf("len = %d, want %d", len(clone), len(orig))
	}
	for i := range orig {
		if clone[i] == orig[i] {
			t.Errorf("element %d shares a pointer with the original", i)
		}
		if !clone[i].Amount.Equal(orig[i].Amount) {
			t.Errorf("element %d amount = %s, want %s", i, clone[i].Amount, orig[i].Amount)
		}
	}

	// Mutating the original must not reach the clone.
	orig[0].Amount = types.MustNewMoney("-99")
	if clone[0].Amount.Equal(orig[0].Amount) {
		t.Error("clone changed when the original was mutated")
	}

	if SplitCollection(nil).Clone() != nil {
		t.Error("Clone of nil should be nil")
	}
}
