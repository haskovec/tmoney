package models

import (
	"testing"
)

// SM-042: Lot model and validation tests

func TestNewLot(t *testing.T) {
	accountID := NewID()
	securityID := NewID()
	shares := MustNewQuantity("100")
	costPerShare := MustNewMoney("50.00")
	purchaseDate := NewDate(2024, 3, 15)
	sourceTxnID := NewID()

	lot := NewLot(accountID, securityID, shares, costPerShare, purchaseDate, sourceTxnID)

	if lot.ID.IsNil() {
		t.Error("expected non-nil ID")
	}
	if lot.AccountID != accountID {
		t.Error("expected matching AccountID")
	}
	if lot.SecurityID != securityID {
		t.Error("expected matching SecurityID")
	}
	if !lot.Shares.Equal(shares) {
		t.Errorf("expected shares %s, got %s", shares, lot.Shares)
	}
	if !lot.OriginalShares.Equal(shares) {
		t.Errorf("expected original_shares %s, got %s", shares, lot.OriginalShares)
	}
	if !lot.CostPerShare.Equal(costPerShare) {
		t.Errorf("expected cost_per_share %s, got %s", costPerShare, lot.CostPerShare)
	}
	if lot.PurchaseDate != purchaseDate {
		t.Errorf("expected purchase_date %s, got %s", purchaseDate, lot.PurchaseDate)
	}
	if lot.SourceTransactionID != sourceTxnID {
		t.Errorf("expected source_transaction_id %s, got %s", sourceTxnID, lot.SourceTransactionID)
	}
	if lot.Closed {
		t.Error("expected closed to be false by default")
	}
}

func TestLotCostBasis(t *testing.T) {
	tests := []struct {
		name     string
		shares   string
		cost     string
		expected string
	}{
		{"100 shares at $50", "100", "50.00", "5000.00"},
		{"0.5 shares at $100", "0.5", "100.00", "50.00"},
		{"10 shares at $0.01", "10", "0.01", "0.10"},
		{"1 share at $1", "1", "1.00", "1.00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lot := NewLot(NewID(), NewID(), MustNewQuantity(tt.shares), MustNewMoney(tt.cost), NewDate(2024, 1, 1), NewID())
			result := lot.CostBasis()
			expected := MustNewMoney(tt.expected)
			if !result.Equal(expected) {
				t.Errorf("CostBasis() = %s, want %s", result, expected)
			}
		})
	}
}

func TestLotIsFullyClosed(t *testing.T) {
	lot := NewLot(NewID(), NewID(), MustNewQuantity("100"), MustNewMoney("50.00"), NewDate(2024, 1, 1), NewID())

	if lot.IsFullyClosed() {
		t.Error("new lot should not be fully closed")
	}

	// Simulate reducing to zero
	lot.Shares = ZeroQuantity
	lot.Closed = true

	if !lot.IsFullyClosed() {
		t.Error("lot with zero shares should be fully closed")
	}
}

func TestLotValidation(t *testing.T) {
	validLot := func() Lot {
		return NewLot(NewID(), NewID(), MustNewQuantity("100"), MustNewMoney("50.00"), NewDate(2024, 1, 1), NewID())
	}

	t.Run("valid lot", func(t *testing.T) {
		lot := validLot()
		if !lot.IsValid() {
			t.Errorf("expected valid lot, got errors: %v", lot.Validate())
		}
	})

	t.Run("missing account_id", func(t *testing.T) {
		lot := validLot()
		lot.AccountID = NilID
		errs := lot.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for missing account_id")
		}
	})

	t.Run("missing security_id", func(t *testing.T) {
		lot := validLot()
		lot.SecurityID = NilID
		errs := lot.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for missing security_id")
		}
	})

	t.Run("shares must be positive", func(t *testing.T) {
		lot := validLot()
		lot.Shares = ZeroQuantity
		errs := lot.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for zero shares")
		}

		lot.Shares = MustNewQuantity("-1")
		errs = lot.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for negative shares")
		}
	})

	t.Run("original_shares must be positive", func(t *testing.T) {
		lot := validLot()
		lot.OriginalShares = ZeroQuantity
		errs := lot.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for zero original_shares")
		}
	})

	t.Run("cost_per_share must be positive", func(t *testing.T) {
		lot := validLot()
		lot.CostPerShare = ZeroMoney
		errs := lot.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for zero cost_per_share")
		}

		lot.CostPerShare = MustNewMoney("-10.00")
		errs = lot.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for negative cost_per_share")
		}
	})

	t.Run("missing purchase_date", func(t *testing.T) {
		lot := validLot()
		lot.PurchaseDate = Date{}
		errs := lot.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for missing purchase_date")
		}
	})

	t.Run("missing source_transaction_id", func(t *testing.T) {
		lot := validLot()
		lot.SourceTransactionID = NilID
		errs := lot.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for missing source_transaction_id")
		}
	})

	t.Run("closed lot allows zero shares", func(t *testing.T) {
		lot := validLot()
		lot.Shares = ZeroQuantity
		lot.Closed = true
		errs := lot.Validate()
		if errs.HasErrors() {
			t.Errorf("closed lot with zero shares should be valid, got: %v", errs)
		}
	})
}

// SM-043: Lot reduce and close logic tests

func TestLotReduce(t *testing.T) {
	t.Run("reduce partial shares", func(t *testing.T) {
		lot := NewLot(NewID(), NewID(), MustNewQuantity("100"), MustNewMoney("50.00"), NewDate(2024, 1, 1), NewID())

		err := lot.Reduce(MustNewQuantity("30"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !lot.Shares.Equal(MustNewQuantity("70")) {
			t.Errorf("expected 70 shares, got %s", lot.Shares)
		}
		if lot.Closed {
			t.Error("lot should not be closed after partial reduce")
		}
	})

	t.Run("reduce to zero sets closed", func(t *testing.T) {
		lot := NewLot(NewID(), NewID(), MustNewQuantity("100"), MustNewMoney("50.00"), NewDate(2024, 1, 1), NewID())

		err := lot.Reduce(MustNewQuantity("100"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !lot.Shares.IsZero() {
			t.Errorf("expected 0 shares, got %s", lot.Shares)
		}
		if !lot.Closed {
			t.Error("lot should be closed when shares reach zero")
		}
	})

	t.Run("reduce more than available returns error", func(t *testing.T) {
		lot := NewLot(NewID(), NewID(), MustNewQuantity("100"), MustNewMoney("50.00"), NewDate(2024, 1, 1), NewID())

		err := lot.Reduce(MustNewQuantity("150"))
		if err == nil {
			t.Error("expected error when reducing more than available")
		}
	})

	t.Run("reduce negative amount returns error", func(t *testing.T) {
		lot := NewLot(NewID(), NewID(), MustNewQuantity("100"), MustNewMoney("50.00"), NewDate(2024, 1, 1), NewID())

		err := lot.Reduce(MustNewQuantity("-10"))
		if err == nil {
			t.Error("expected error when reducing by negative amount")
		}
	})

	t.Run("reduce zero amount returns error", func(t *testing.T) {
		lot := NewLot(NewID(), NewID(), MustNewQuantity("100"), MustNewMoney("50.00"), NewDate(2024, 1, 1), NewID())

		err := lot.Reduce(ZeroQuantity)
		if err == nil {
			t.Error("expected error when reducing by zero")
		}
	})

	t.Run("reduce already closed lot returns error", func(t *testing.T) {
		lot := NewLot(NewID(), NewID(), MustNewQuantity("100"), MustNewMoney("50.00"), NewDate(2024, 1, 1), NewID())
		lot.Shares = ZeroQuantity
		lot.Closed = true

		err := lot.Reduce(MustNewQuantity("10"))
		if err == nil {
			t.Error("expected error when reducing a closed lot")
		}
	})

	t.Run("multiple reduces", func(t *testing.T) {
		lot := NewLot(NewID(), NewID(), MustNewQuantity("100"), MustNewMoney("50.00"), NewDate(2024, 1, 1), NewID())

		if err := lot.Reduce(MustNewQuantity("30")); err != nil {
			t.Fatalf("unexpected error on first reduce: %v", err)
		}
		if err := lot.Reduce(MustNewQuantity("30")); err != nil {
			t.Fatalf("unexpected error on second reduce: %v", err)
		}
		if !lot.Shares.Equal(MustNewQuantity("40")) {
			t.Errorf("expected 40 shares after two reduces, got %s", lot.Shares)
		}

		if err := lot.Reduce(MustNewQuantity("40")); err != nil {
			t.Fatalf("unexpected error on final reduce: %v", err)
		}
		if !lot.Closed {
			t.Error("lot should be closed after reducing to zero")
		}
	})

	t.Run("reduce fractional shares", func(t *testing.T) {
		lot := NewLot(NewID(), NewID(), MustNewQuantity("10.5"), MustNewMoney("100.00"), NewDate(2024, 1, 1), NewID())

		err := lot.Reduce(MustNewQuantity("0.5"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !lot.Shares.Equal(MustNewQuantity("10")) {
			t.Errorf("expected 10 shares, got %s", lot.Shares)
		}
	})
}
