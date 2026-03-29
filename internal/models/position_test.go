package models

import (
	"testing"
)

// SM-044: Position model and validation tests

func TestNewPosition(t *testing.T) {
	accountID := NewID()
	securityID := NewID()

	pos := NewPosition(accountID, securityID)

	if pos.ID.IsNil() {
		t.Error("expected non-nil ID")
	}
	if pos.AccountID != accountID {
		t.Error("expected matching AccountID")
	}
	if pos.SecurityID != securityID {
		t.Error("expected matching SecurityID")
	}
	if !pos.Shares.IsZero() {
		t.Errorf("expected zero shares, got %s", pos.Shares)
	}
	if !pos.AverageCostPerShare.IsZero() {
		t.Errorf("expected zero average cost, got %s", pos.AverageCostPerShare)
	}
}

func TestNewPositionWithShares(t *testing.T) {
	accountID := NewID()
	securityID := NewID()
	shares := MustNewQuantity("100")
	avgCost := MustNewMoney("50.00")

	pos := NewPositionWithShares(accountID, securityID, shares, avgCost)

	if !pos.Shares.Equal(shares) {
		t.Errorf("expected shares %s, got %s", shares, pos.Shares)
	}
	if !pos.AverageCostPerShare.Equal(avgCost) {
		t.Errorf("expected avg cost %s, got %s", avgCost, pos.AverageCostPerShare)
	}
}

func TestPositionCostBasis(t *testing.T) {
	tests := []struct {
		name     string
		shares   string
		avgCost  string
		expected string
	}{
		{"100 shares at $50", "100", "50.00", "5000.00"},
		{"0 shares", "0", "50.00", "0.00"},
		{"0.5 shares at $100", "0.5", "100.00", "50.00"},
		{"10 shares at $0.01", "10", "0.01", "0.10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := NewPositionWithShares(NewID(), NewID(), MustNewQuantity(tt.shares), MustNewMoney(tt.avgCost))
			result := pos.CostBasis()
			expected := MustNewMoney(tt.expected)
			if !result.Equal(expected) {
				t.Errorf("CostBasis() = %s, want %s", result, expected)
			}
		})
	}
}

func TestPositionMarketValue(t *testing.T) {
	tests := []struct {
		name         string
		shares       string
		currentPrice string
		expected     string
	}{
		{"100 shares at $60", "100", "60.00", "6000.00"},
		{"0 shares", "0", "60.00", "0.00"},
		{"0.5 shares at $200", "0.5", "200.00", "100.00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := NewPositionWithShares(NewID(), NewID(), MustNewQuantity(tt.shares), MustNewMoney("50.00"))
			result := pos.MarketValue(MustNewMoney(tt.currentPrice))
			expected := MustNewMoney(tt.expected)
			if !result.Equal(expected) {
				t.Errorf("MarketValue() = %s, want %s", result, expected)
			}
		})
	}
}

func TestPositionValidation(t *testing.T) {
	validPosition := func() Position {
		return NewPositionWithShares(NewID(), NewID(), MustNewQuantity("100"), MustNewMoney("50.00"))
	}

	t.Run("valid position", func(t *testing.T) {
		pos := validPosition()
		if !pos.IsValid() {
			t.Errorf("expected valid position, got errors: %v", pos.Validate())
		}
	})

	t.Run("missing account_id", func(t *testing.T) {
		pos := validPosition()
		pos.AccountID = NilID
		errs := pos.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for missing account_id")
		}
	})

	t.Run("missing security_id", func(t *testing.T) {
		pos := validPosition()
		pos.SecurityID = NilID
		errs := pos.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for missing security_id")
		}
	})

	t.Run("negative shares", func(t *testing.T) {
		pos := validPosition()
		pos.Shares = MustNewQuantity("-1")
		errs := pos.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for negative shares")
		}
	})

	t.Run("zero shares is valid", func(t *testing.T) {
		pos := NewPosition(NewID(), NewID())
		if !pos.IsValid() {
			t.Errorf("position with zero shares should be valid, got: %v", pos.Validate())
		}
	})

	t.Run("negative average_cost_per_share", func(t *testing.T) {
		pos := validPosition()
		pos.AverageCostPerShare = MustNewMoney("-10.00")
		errs := pos.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for negative average_cost_per_share")
		}
	})
}

// SM-045: Position average cost recalculation tests

func TestPositionAddShares(t *testing.T) {
	t.Run("add to empty position", func(t *testing.T) {
		pos := NewPosition(NewID(), NewID())

		err := pos.AddShares(MustNewQuantity("100"), MustNewMoney("50.00"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !pos.Shares.Equal(MustNewQuantity("100")) {
			t.Errorf("expected 100 shares, got %s", pos.Shares)
		}
		if !pos.AverageCostPerShare.Equal(MustNewMoney("50.00")) {
			t.Errorf("expected avg cost $50.00, got %s", pos.AverageCostPerShare)
		}
	})

	t.Run("add shares recalculates weighted average", func(t *testing.T) {
		pos := NewPositionWithShares(NewID(), NewID(), MustNewQuantity("100"), MustNewMoney("50.00"))

		// Add 100 shares at $60 → total cost = 5000 + 6000 = 11000, total shares = 200
		// average = 11000/200 = 55.00
		err := pos.AddShares(MustNewQuantity("100"), MustNewMoney("60.00"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !pos.Shares.Equal(MustNewQuantity("200")) {
			t.Errorf("expected 200 shares, got %s", pos.Shares)
		}
		if !pos.AverageCostPerShare.Equal(MustNewMoney("55.00")) {
			t.Errorf("expected avg cost $55.00, got %s", pos.AverageCostPerShare)
		}
	})

	t.Run("add zero shares returns error", func(t *testing.T) {
		pos := NewPosition(NewID(), NewID())

		err := pos.AddShares(ZeroQuantity, MustNewMoney("50.00"))
		if err == nil {
			t.Error("expected error when adding zero shares")
		}
	})

	t.Run("add negative shares returns error", func(t *testing.T) {
		pos := NewPosition(NewID(), NewID())

		err := pos.AddShares(MustNewQuantity("-10"), MustNewMoney("50.00"))
		if err == nil {
			t.Error("expected error when adding negative shares")
		}
	})

	t.Run("add shares with negative price returns error", func(t *testing.T) {
		pos := NewPosition(NewID(), NewID())

		err := pos.AddShares(MustNewQuantity("10"), MustNewMoney("-50.00"))
		if err == nil {
			t.Error("expected error when adding shares with negative price")
		}
	})
}

func TestPositionRemoveShares(t *testing.T) {
	t.Run("remove partial shares", func(t *testing.T) {
		pos := NewPositionWithShares(NewID(), NewID(), MustNewQuantity("100"), MustNewMoney("50.00"))

		err := pos.RemoveShares(MustNewQuantity("30"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !pos.Shares.Equal(MustNewQuantity("70")) {
			t.Errorf("expected 70 shares, got %s", pos.Shares)
		}
		// Average cost should NOT change on removal
		if !pos.AverageCostPerShare.Equal(MustNewMoney("50.00")) {
			t.Errorf("expected avg cost to remain $50.00, got %s", pos.AverageCostPerShare)
		}
	})

	t.Run("remove all shares", func(t *testing.T) {
		pos := NewPositionWithShares(NewID(), NewID(), MustNewQuantity("100"), MustNewMoney("50.00"))

		err := pos.RemoveShares(MustNewQuantity("100"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !pos.Shares.IsZero() {
			t.Errorf("expected 0 shares, got %s", pos.Shares)
		}
	})

	t.Run("remove more than held returns error", func(t *testing.T) {
		pos := NewPositionWithShares(NewID(), NewID(), MustNewQuantity("100"), MustNewMoney("50.00"))

		err := pos.RemoveShares(MustNewQuantity("150"))
		if err == nil {
			t.Error("expected error when removing more than held")
		}
	})

	t.Run("remove zero shares returns error", func(t *testing.T) {
		pos := NewPositionWithShares(NewID(), NewID(), MustNewQuantity("100"), MustNewMoney("50.00"))

		err := pos.RemoveShares(ZeroQuantity)
		if err == nil {
			t.Error("expected error when removing zero shares")
		}
	})

	t.Run("remove negative shares returns error", func(t *testing.T) {
		pos := NewPositionWithShares(NewID(), NewID(), MustNewQuantity("100"), MustNewMoney("50.00"))

		err := pos.RemoveShares(MustNewQuantity("-10"))
		if err == nil {
			t.Error("expected error when removing negative shares")
		}
	})

	t.Run("remove fractional shares", func(t *testing.T) {
		pos := NewPositionWithShares(NewID(), NewID(), MustNewQuantity("10.5"), MustNewMoney("100.00"))

		err := pos.RemoveShares(MustNewQuantity("0.5"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !pos.Shares.Equal(MustNewQuantity("10")) {
			t.Errorf("expected 10 shares, got %s", pos.Shares)
		}
	})
}
