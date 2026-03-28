package models

import (
	"testing"
)

// --- SM-039: InvestmentTransactionType enum tests ---

func TestInvestmentTransactionType(t *testing.T) {
	t.Run("AllInvestmentTransactionTypes returns all types", func(t *testing.T) {
		types := AllInvestmentTransactionTypes()
		expected := 12
		if len(types) != expected {
			t.Errorf("Expected %d investment transaction types, got %d", expected, len(types))
		}
	})

	t.Run("String returns correct value", func(t *testing.T) {
		tests := []struct {
			txnType  InvestmentTransactionType
			expected string
		}{
			{InvestmentTransactionTypeBuy, "buy"},
			{InvestmentTransactionTypeSell, "sell"},
			{InvestmentTransactionTypeDividend, "dividend"},
			{InvestmentTransactionTypeReinvestDividend, "reinvest_dividend"},
			{InvestmentTransactionTypeFee, "fee"},
			{InvestmentTransactionTypeFeeLiquidation, "fee_liquidation"},
			{InvestmentTransactionTypeDeposit, "deposit"},
			{InvestmentTransactionTypeWithdrawal, "withdrawal"},
			{InvestmentTransactionTypeInterest, "interest"},
			{InvestmentTransactionTypeTransferShares, "transfer_shares"},
			{InvestmentTransactionTypeTransferCash, "transfer_cash"},
			{InvestmentTransactionTypeExchange, "exchange"},
		}
		for _, tc := range tests {
			if tc.txnType.String() != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, tc.txnType.String())
			}
		}
	})

	t.Run("IsValid returns true for all valid types", func(t *testing.T) {
		for _, itt := range AllInvestmentTransactionTypes() {
			if !itt.IsValid() {
				t.Errorf("IsValid should return true for %q", itt)
			}
		}
	})

	t.Run("IsValid returns false for invalid types", func(t *testing.T) {
		invalid := InvestmentTransactionType("refund")
		if invalid.IsValid() {
			t.Error("IsValid should return false for 'refund'")
		}

		empty := InvestmentTransactionType("")
		if empty.IsValid() {
			t.Error("IsValid should return false for empty string")
		}
	})

	t.Run("DisplayName returns human-readable names", func(t *testing.T) {
		tests := []struct {
			txnType  InvestmentTransactionType
			expected string
		}{
			{InvestmentTransactionTypeBuy, "Buy"},
			{InvestmentTransactionTypeSell, "Sell"},
			{InvestmentTransactionTypeDividend, "Dividend"},
			{InvestmentTransactionTypeReinvestDividend, "Reinvest Dividend"},
			{InvestmentTransactionTypeFee, "Fee"},
			{InvestmentTransactionTypeFeeLiquidation, "Fee via Liquidation"},
			{InvestmentTransactionTypeDeposit, "Deposit"},
			{InvestmentTransactionTypeWithdrawal, "Withdrawal"},
			{InvestmentTransactionTypeInterest, "Interest"},
			{InvestmentTransactionTypeTransferShares, "Transfer Shares"},
			{InvestmentTransactionTypeTransferCash, "Transfer Cash"},
			{InvestmentTransactionTypeExchange, "Exchange"},
		}
		for _, tc := range tests {
			if tc.txnType.DisplayName() != tc.expected {
				t.Errorf("DisplayName for %q: expected %q, got %q",
					tc.txnType, tc.expected, tc.txnType.DisplayName())
			}
		}
	})

	t.Run("DisplayName returns raw string for unknown type", func(t *testing.T) {
		unknown := InvestmentTransactionType("unknown")
		if unknown.DisplayName() != "unknown" {
			t.Errorf("Expected 'unknown', got %q", unknown.DisplayName())
		}
	})
}

func TestInvestmentTransactionTypeRequiresSecurity(t *testing.T) {
	requiresSecurity := []InvestmentTransactionType{
		InvestmentTransactionTypeBuy,
		InvestmentTransactionTypeSell,
		InvestmentTransactionTypeDividend,
		InvestmentTransactionTypeReinvestDividend,
		InvestmentTransactionTypeFeeLiquidation,
		InvestmentTransactionTypeTransferShares,
		InvestmentTransactionTypeExchange,
	}
	doesNotRequireSecurity := []InvestmentTransactionType{
		InvestmentTransactionTypeFee,
		InvestmentTransactionTypeDeposit,
		InvestmentTransactionTypeWithdrawal,
		InvestmentTransactionTypeInterest,
		InvestmentTransactionTypeTransferCash,
	}

	for _, itt := range requiresSecurity {
		if !itt.RequiresSecurity() {
			t.Errorf("RequiresSecurity should return true for %q", itt)
		}
	}
	for _, itt := range doesNotRequireSecurity {
		if itt.RequiresSecurity() {
			t.Errorf("RequiresSecurity should return false for %q", itt)
		}
	}
}

func TestInvestmentTransactionTypeRequiresShares(t *testing.T) {
	requiresShares := []InvestmentTransactionType{
		InvestmentTransactionTypeBuy,
		InvestmentTransactionTypeSell,
		InvestmentTransactionTypeReinvestDividend,
		InvestmentTransactionTypeFeeLiquidation,
		InvestmentTransactionTypeTransferShares,
		InvestmentTransactionTypeExchange,
	}
	doesNotRequireShares := []InvestmentTransactionType{
		InvestmentTransactionTypeDividend,
		InvestmentTransactionTypeFee,
		InvestmentTransactionTypeDeposit,
		InvestmentTransactionTypeWithdrawal,
		InvestmentTransactionTypeInterest,
		InvestmentTransactionTypeTransferCash,
	}

	for _, itt := range requiresShares {
		if !itt.RequiresShares() {
			t.Errorf("RequiresShares should return true for %q", itt)
		}
	}
	for _, itt := range doesNotRequireShares {
		if itt.RequiresShares() {
			t.Errorf("RequiresShares should return false for %q", itt)
		}
	}
}

func TestInvestmentTransactionTypeAffectsCash(t *testing.T) {
	affectsCash := []InvestmentTransactionType{
		InvestmentTransactionTypeBuy,
		InvestmentTransactionTypeSell,
		InvestmentTransactionTypeDividend,
		InvestmentTransactionTypeFee,
		InvestmentTransactionTypeDeposit,
		InvestmentTransactionTypeWithdrawal,
		InvestmentTransactionTypeInterest,
		InvestmentTransactionTypeTransferCash,
	}
	doesNotAffectCash := []InvestmentTransactionType{
		InvestmentTransactionTypeReinvestDividend,
		InvestmentTransactionTypeFeeLiquidation,
		InvestmentTransactionTypeTransferShares,
		InvestmentTransactionTypeExchange,
	}

	for _, itt := range affectsCash {
		if !itt.AffectsCash() {
			t.Errorf("AffectsCash should return true for %q", itt)
		}
	}
	for _, itt := range doesNotAffectCash {
		if itt.AffectsCash() {
			t.Errorf("AffectsCash should return false for %q", itt)
		}
	}
}

func TestParseInvestmentTransactionType(t *testing.T) {
	t.Run("Parses valid types", func(t *testing.T) {
		tests := []struct {
			input    string
			expected InvestmentTransactionType
		}{
			{"buy", InvestmentTransactionTypeBuy},
			{"sell", InvestmentTransactionTypeSell},
			{"dividend", InvestmentTransactionTypeDividend},
			{"reinvest_dividend", InvestmentTransactionTypeReinvestDividend},
			{"fee", InvestmentTransactionTypeFee},
			{"fee_liquidation", InvestmentTransactionTypeFeeLiquidation},
			{"deposit", InvestmentTransactionTypeDeposit},
			{"withdrawal", InvestmentTransactionTypeWithdrawal},
			{"interest", InvestmentTransactionTypeInterest},
			{"transfer_shares", InvestmentTransactionTypeTransferShares},
			{"transfer_cash", InvestmentTransactionTypeTransferCash},
			{"exchange", InvestmentTransactionTypeExchange},
		}
		for _, tc := range tests {
			itt, err := ParseInvestmentTransactionType(tc.input)
			if err != nil {
				t.Errorf("ParseInvestmentTransactionType(%q) returned error: %v", tc.input, err)
			}
			if itt != tc.expected {
				t.Errorf("ParseInvestmentTransactionType(%q) = %q, expected %q", tc.input, itt, tc.expected)
			}
		}
	})

	t.Run("Parses case-insensitive", func(t *testing.T) {
		tests := []struct {
			input    string
			expected InvestmentTransactionType
		}{
			{"Buy", InvestmentTransactionTypeBuy},
			{"SELL", InvestmentTransactionTypeSell},
			{"Dividend", InvestmentTransactionTypeDividend},
			{"REINVEST_DIVIDEND", InvestmentTransactionTypeReinvestDividend},
		}
		for _, tc := range tests {
			itt, err := ParseInvestmentTransactionType(tc.input)
			if err != nil {
				t.Errorf("ParseInvestmentTransactionType(%q) returned error: %v", tc.input, err)
			}
			if itt != tc.expected {
				t.Errorf("ParseInvestmentTransactionType(%q) = %q, expected %q", tc.input, itt, tc.expected)
			}
		}
	})

	t.Run("Rejects invalid types", func(t *testing.T) {
		invalidTypes := []string{"refund", "transfer", "bond_coupon", ""}
		for _, s := range invalidTypes {
			_, err := ParseInvestmentTransactionType(s)
			if err == nil {
				t.Errorf("ParseInvestmentTransactionType(%q) should return error", s)
			}
		}
	})
}

func TestInvestmentTransactionTypeScanValue(t *testing.T) {
	t.Run("Value returns string", func(t *testing.T) {
		val, err := InvestmentTransactionTypeBuy.Value()
		if err != nil {
			t.Fatalf("Value() returned error: %v", err)
		}
		if val != "buy" {
			t.Errorf("Expected 'buy', got %v", val)
		}
	})

	t.Run("Scan from string", func(t *testing.T) {
		var itt InvestmentTransactionType
		err := itt.Scan("sell")
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if itt != InvestmentTransactionTypeSell {
			t.Errorf("Expected InvestmentTransactionTypeSell, got %q", itt)
		}
	})

	t.Run("Scan from bytes", func(t *testing.T) {
		var itt InvestmentTransactionType
		err := itt.Scan([]byte("dividend"))
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if itt != InvestmentTransactionTypeDividend {
			t.Errorf("Expected InvestmentTransactionTypeDividend, got %q", itt)
		}
	})

	t.Run("Scan from nil", func(t *testing.T) {
		var itt InvestmentTransactionType
		err := itt.Scan(nil)
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if itt != "" {
			t.Errorf("Expected empty string, got %q", itt)
		}
	})

	t.Run("Scan from unsupported type returns error", func(t *testing.T) {
		var itt InvestmentTransactionType
		err := itt.Scan(123)
		if err == nil {
			t.Error("Scan from int should return error")
		}
	})
}
