package investment

import (
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

// SM-044: Position model and validation tests

func TestNewPosition(t *testing.T) {
	accountID := types.NewID()
	securityID := types.NewID()

	pos := NewPosition(accountID, securityID)

	if pos.ID.IsNil() {
		t.Error("expected non-nil ID")
	}
	if pos.AccountID != accountID {
		t.Error("expected matching AccountID")
	}
	if !pos.Shares.IsZero() {
		t.Errorf("expected zero shares, got %s", pos.Shares)
	}
	if !pos.AverageCostPerShare.IsZero() {
		t.Errorf("expected zero average cost, got %s", pos.AverageCostPerShare)
	}
}

func TestPositionCostBasis(t *testing.T) {
	pos := NewPositionWithShares(types.NewID(), types.NewID(), types.MustNewQuantity("100"), types.MustNewMoney("50.00"))
	result := pos.CostBasis()
	expected := types.MustNewMoney("5000.00")
	if !result.Equal(expected) {
		t.Errorf("CostBasis() = %s, want %s", result, expected)
	}
}

func TestPositionMarketValue(t *testing.T) {
	pos := NewPositionWithShares(types.NewID(), types.NewID(), types.MustNewQuantity("100"), types.MustNewMoney("50.00"))
	result := pos.MarketValue(types.MustNewMoney("60.00"))
	expected := types.MustNewMoney("6000.00")
	if !result.Equal(expected) {
		t.Errorf("MarketValue() = %s, want %s", result, expected)
	}
}

func TestPositionAddShares(t *testing.T) {
	t.Run("add to empty position", func(t *testing.T) {
		pos := NewPosition(types.NewID(), types.NewID())

		err := pos.AddShares(types.MustNewQuantity("100"), types.MustNewMoney("50.00"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !pos.Shares.Equal(types.MustNewQuantity("100")) {
			t.Errorf("expected 100 shares, got %s", pos.Shares)
		}
		if !pos.AverageCostPerShare.Equal(types.MustNewMoney("50.00")) {
			t.Errorf("expected avg cost $50.00, got %s", pos.AverageCostPerShare)
		}
	})

	t.Run("add shares recalculates weighted average", func(t *testing.T) {
		pos := NewPositionWithShares(types.NewID(), types.NewID(), types.MustNewQuantity("100"), types.MustNewMoney("50.00"))

		err := pos.AddShares(types.MustNewQuantity("100"), types.MustNewMoney("60.00"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !pos.Shares.Equal(types.MustNewQuantity("200")) {
			t.Errorf("expected 200 shares, got %s", pos.Shares)
		}
		if !pos.AverageCostPerShare.Equal(types.MustNewMoney("55.00")) {
			t.Errorf("expected avg cost $55.00, got %s", pos.AverageCostPerShare)
		}
	})

	t.Run("add zero shares returns error", func(t *testing.T) {
		pos := NewPosition(types.NewID(), types.NewID())
		err := pos.AddShares(types.ZeroQuantity, types.MustNewMoney("50.00"))
		if err == nil {
			t.Error("expected error when adding zero shares")
		}
	})
}

func TestPositionRemoveShares(t *testing.T) {
	t.Run("remove partial shares", func(t *testing.T) {
		pos := NewPositionWithShares(types.NewID(), types.NewID(), types.MustNewQuantity("100"), types.MustNewMoney("50.00"))

		err := pos.RemoveShares(types.MustNewQuantity("30"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !pos.Shares.Equal(types.MustNewQuantity("70")) {
			t.Errorf("expected 70 shares, got %s", pos.Shares)
		}
		if !pos.AverageCostPerShare.Equal(types.MustNewMoney("50.00")) {
			t.Errorf("expected avg cost to remain $50.00, got %s", pos.AverageCostPerShare)
		}
	})

	t.Run("remove more than held returns error", func(t *testing.T) {
		pos := NewPositionWithShares(types.NewID(), types.NewID(), types.MustNewQuantity("100"), types.MustNewMoney("50.00"))
		err := pos.RemoveShares(types.MustNewQuantity("150"))
		if err == nil {
			t.Error("expected error when removing more than held")
		}
	})
}

func TestPositionValidation(t *testing.T) {
	t.Run("valid position", func(t *testing.T) {
		pos := NewPositionWithShares(types.NewID(), types.NewID(), types.MustNewQuantity("100"), types.MustNewMoney("50.00"))
		if !pos.IsValid() {
			t.Errorf("expected valid position, got errors: %v", pos.Validate())
		}
	})

	t.Run("missing account_id", func(t *testing.T) {
		pos := NewPositionWithShares(types.NilID, types.NewID(), types.MustNewQuantity("100"), types.MustNewMoney("50.00"))
		if !pos.Validate().HasErrors() {
			t.Error("expected validation error for missing account_id")
		}
	})

	t.Run("zero shares is valid", func(t *testing.T) {
		pos := NewPosition(types.NewID(), types.NewID())
		if !pos.IsValid() {
			t.Errorf("position with zero shares should be valid, got: %v", pos.Validate())
		}
	})
}
