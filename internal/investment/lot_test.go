package investment

import (
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

// SM-042: Lot model and validation tests

func TestNewLot(t *testing.T) {
	accountID := types.NewID()
	securityID := types.NewID()
	shares := types.MustNewQuantity("100")
	costPerShare := types.MustNewMoney("50.00")
	purchaseDate := types.NewDate(2024, 3, 15)
	sourceTxnID := types.NewID()

	lot := NewLot(accountID, securityID, shares, costPerShare, purchaseDate, sourceTxnID)

	if lot.ID.IsNil() {
		t.Error("expected non-nil ID")
	}
	if lot.AccountID != accountID {
		t.Error("expected matching AccountID")
	}
	if !lot.Shares.Equal(shares) {
		t.Errorf("expected shares %s, got %s", shares, lot.Shares)
	}
	if !lot.OriginalShares.Equal(shares) {
		t.Errorf("expected original_shares %s, got %s", shares, lot.OriginalShares)
	}
	if lot.Closed {
		t.Error("expected closed to be false by default")
	}
}

func TestLotCostBasis(t *testing.T) {
	lot := NewLot(types.NewID(), types.NewID(), types.MustNewQuantity("100"), types.MustNewMoney("50.00"), types.NewDate(2024, 1, 1), types.NewID())
	result := lot.CostBasis()
	expected := types.MustNewMoney("5000.00")
	if !result.Equal(expected) {
		t.Errorf("CostBasis() = %s, want %s", result, expected)
	}
}

func TestLotReduce(t *testing.T) {
	t.Run("reduce partial shares", func(t *testing.T) {
		lot := NewLot(types.NewID(), types.NewID(), types.MustNewQuantity("100"), types.MustNewMoney("50.00"), types.NewDate(2024, 1, 1), types.NewID())

		err := lot.Reduce(types.MustNewQuantity("30"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !lot.Shares.Equal(types.MustNewQuantity("70")) {
			t.Errorf("expected 70 shares, got %s", lot.Shares)
		}
		if lot.Closed {
			t.Error("lot should not be closed after partial reduce")
		}
	})

	t.Run("reduce to zero sets closed", func(t *testing.T) {
		lot := NewLot(types.NewID(), types.NewID(), types.MustNewQuantity("100"), types.MustNewMoney("50.00"), types.NewDate(2024, 1, 1), types.NewID())

		err := lot.Reduce(types.MustNewQuantity("100"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !lot.Closed {
			t.Error("lot should be closed when shares reach zero")
		}
	})

	t.Run("reduce more than available returns error", func(t *testing.T) {
		lot := NewLot(types.NewID(), types.NewID(), types.MustNewQuantity("100"), types.MustNewMoney("50.00"), types.NewDate(2024, 1, 1), types.NewID())

		err := lot.Reduce(types.MustNewQuantity("150"))
		if err == nil {
			t.Error("expected error when reducing more than available")
		}
	})

	t.Run("reduce already closed lot returns error", func(t *testing.T) {
		lot := NewLot(types.NewID(), types.NewID(), types.MustNewQuantity("100"), types.MustNewMoney("50.00"), types.NewDate(2024, 1, 1), types.NewID())
		lot.Shares = types.ZeroQuantity
		lot.Closed = true

		err := lot.Reduce(types.MustNewQuantity("10"))
		if err == nil {
			t.Error("expected error when reducing a closed lot")
		}
	})
}

func TestLotValidation(t *testing.T) {
	validLot := func() Lot {
		return NewLot(types.NewID(), types.NewID(), types.MustNewQuantity("100"), types.MustNewMoney("50.00"), types.NewDate(2024, 1, 1), types.NewID())
	}

	t.Run("valid lot", func(t *testing.T) {
		lot := validLot()
		if !lot.IsValid() {
			t.Errorf("expected valid lot, got errors: %v", lot.Validate())
		}
	})

	t.Run("missing account_id", func(t *testing.T) {
		lot := validLot()
		lot.AccountID = types.NilID
		if !lot.Validate().HasErrors() {
			t.Error("expected validation error for missing account_id")
		}
	})

	t.Run("closed lot allows zero shares", func(t *testing.T) {
		lot := validLot()
		lot.Shares = types.ZeroQuantity
		lot.Closed = true
		errs := lot.Validate()
		if errs.HasErrors() {
			t.Errorf("closed lot with zero shares should be valid, got: %v", errs)
		}
	})
}
