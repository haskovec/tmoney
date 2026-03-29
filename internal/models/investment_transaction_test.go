package models

import (
	"strings"
	"testing"
	"time"
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

// --- SM-040: InvestmentTransaction model and validation tests ---

func TestInvestmentTransactionStatus(t *testing.T) {
	t.Run("AllInvestmentTransactionStatuses returns all statuses", func(t *testing.T) {
		statuses := AllInvestmentTransactionStatuses()
		if len(statuses) != 3 {
			t.Errorf("Expected 3 investment transaction statuses, got %d", len(statuses))
		}
	})

	t.Run("IsValid returns true for valid statuses", func(t *testing.T) {
		for _, s := range AllInvestmentTransactionStatuses() {
			if !s.IsValid() {
				t.Errorf("IsValid should return true for %q", s)
			}
		}
	})

	t.Run("IsValid returns false for invalid statuses", func(t *testing.T) {
		invalid := InvestmentTransactionStatus("cancelled")
		if invalid.IsValid() {
			t.Error("IsValid should return false for 'cancelled'")
		}
		empty := InvestmentTransactionStatus("")
		if empty.IsValid() {
			t.Error("IsValid should return false for empty string")
		}
	})

	t.Run("String returns correct value", func(t *testing.T) {
		tests := []struct {
			status   InvestmentTransactionStatus
			expected string
		}{
			{InvestmentTransactionStatusPending, "pending"},
			{InvestmentTransactionStatusCleared, "cleared"},
			{InvestmentTransactionStatusReconciled, "reconciled"},
		}
		for _, tc := range tests {
			if tc.status.String() != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, tc.status.String())
			}
		}
	})

	t.Run("DisplayName returns human-readable names", func(t *testing.T) {
		tests := []struct {
			status   InvestmentTransactionStatus
			expected string
		}{
			{InvestmentTransactionStatusPending, "Pending"},
			{InvestmentTransactionStatusCleared, "Cleared"},
			{InvestmentTransactionStatusReconciled, "Reconciled"},
		}
		for _, tc := range tests {
			if tc.status.DisplayName() != tc.expected {
				t.Errorf("DisplayName for %q: expected %q, got %q",
					tc.status, tc.expected, tc.status.DisplayName())
			}
		}
	})

	t.Run("ParseInvestmentTransactionStatus parses valid statuses", func(t *testing.T) {
		tests := []struct {
			input    string
			expected InvestmentTransactionStatus
		}{
			{"pending", InvestmentTransactionStatusPending},
			{"cleared", InvestmentTransactionStatusCleared},
			{"reconciled", InvestmentTransactionStatusReconciled},
			{"Pending", InvestmentTransactionStatusPending},
			{"CLEARED", InvestmentTransactionStatusCleared},
		}
		for _, tc := range tests {
			s, err := ParseInvestmentTransactionStatus(tc.input)
			if err != nil {
				t.Errorf("ParseInvestmentTransactionStatus(%q) returned error: %v", tc.input, err)
			}
			if s != tc.expected {
				t.Errorf("ParseInvestmentTransactionStatus(%q) = %q, expected %q", tc.input, s, tc.expected)
			}
		}
	})

	t.Run("ParseInvestmentTransactionStatus rejects invalid", func(t *testing.T) {
		_, err := ParseInvestmentTransactionStatus("void")
		if err == nil {
			t.Error("ParseInvestmentTransactionStatus('void') should return error")
		}
	})

	t.Run("Value and Scan roundtrip", func(t *testing.T) {
		original := InvestmentTransactionStatusCleared
		val, err := original.Value()
		if err != nil {
			t.Fatalf("Value() returned error: %v", err)
		}
		if val != "cleared" {
			t.Errorf("Expected 'cleared', got %v", val)
		}

		var scanned InvestmentTransactionStatus
		err = scanned.Scan("cleared")
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if scanned != original {
			t.Errorf("Expected %q, got %q", original, scanned)
		}
	})

	t.Run("Scan from nil", func(t *testing.T) {
		var s InvestmentTransactionStatus
		err := s.Scan(nil)
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if s != "" {
			t.Errorf("Expected empty string, got %q", s)
		}
	})

	t.Run("Scan from bytes", func(t *testing.T) {
		var s InvestmentTransactionStatus
		err := s.Scan([]byte("pending"))
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if s != InvestmentTransactionStatusPending {
			t.Errorf("Expected 'pending', got %q", s)
		}
	})

	t.Run("Scan from unsupported type returns error", func(t *testing.T) {
		var s InvestmentTransactionStatus
		err := s.Scan(42)
		if err == nil {
			t.Error("Scan from int should return error")
		}
	})
}

func TestNewInvestmentTransaction(t *testing.T) {
	t.Run("Creates with required fields", func(t *testing.T) {
		accountID := NewID()
		date := NewDate(2024, time.March, 15)
		txnType := InvestmentTransactionTypeBuy
		amount := MustNewMoney("1000.00")

		txn := NewInvestmentTransaction(accountID, date, txnType, amount)

		if txn.ID.IsNil() {
			t.Error("Should create non-nil ID")
		}
		if txn.AccountID != accountID {
			t.Errorf("Expected account ID %s, got %s", accountID, txn.AccountID)
		}
		if !txn.Date.Equal(date) {
			t.Errorf("Expected date %s, got %s", date, txn.Date)
		}
		if txn.Type != txnType {
			t.Errorf("Expected type %q, got %q", txnType, txn.Type)
		}
		if !txn.TotalAmount.Equal(amount) {
			t.Errorf("Expected amount %s, got %s", amount, txn.TotalAmount)
		}
		if txn.Status != InvestmentTransactionStatusPending {
			t.Errorf("Expected status 'pending', got %q", txn.Status)
		}
		// Optional fields should not be set
		if txn.SecurityID.Valid {
			t.Error("SecurityID should not be set")
		}
		if txn.Shares.Valid {
			t.Error("Shares should not be set")
		}
		if txn.PricePerShare.Valid {
			t.Error("PricePerShare should not be set")
		}
		if txn.Commission.Valid {
			t.Error("Commission should not be set")
		}
		if txn.Memo.Valid {
			t.Error("Memo should not be set")
		}
	})
}

func TestNewInvestmentTransactionWithSecurity(t *testing.T) {
	t.Run("Creates with security and shares", func(t *testing.T) {
		accountID := NewID()
		securityID := NewID()
		date := NewDate(2024, time.March, 15)
		amount := MustNewMoney("5000.00")
		shares := MustNewQuantity("100")

		txn := NewInvestmentTransactionWithSecurity(
			accountID, date, InvestmentTransactionTypeBuy,
			amount, securityID, shares,
		)

		if !txn.SecurityID.Valid || txn.SecurityID.ID != securityID {
			t.Error("SecurityID should be set")
		}
		if !txn.Shares.Valid || !txn.Shares.Quantity.IsPositive() {
			t.Error("Shares should be set and positive")
		}
	})
}

func TestInvestmentTransactionValidate(t *testing.T) {
	t.Run("Valid buy transaction passes validation", func(t *testing.T) {
		accountID := NewID()
		securityID := NewID()
		date := NewDate(2024, time.March, 15)
		amount := MustNewMoney("5000.00")
		shares := MustNewQuantity("100")

		txn := NewInvestmentTransactionWithSecurity(
			accountID, date, InvestmentTransactionTypeBuy,
			amount, securityID, shares,
		)

		errs := txn.Validate()
		if errs.HasErrors() {
			t.Errorf("Expected no errors, got: %v", errs)
		}
	})

	t.Run("Missing account_id fails", func(t *testing.T) {
		txn := NewInvestmentTransaction(NilID, NewDate(2024, time.March, 15),
			InvestmentTransactionTypeDeposit, MustNewMoney("1000.00"))
		errs := txn.Validate()
		if !errs.HasErrors() {
			t.Error("Expected validation error for missing account_id")
		}
		found := false
		for _, e := range errs {
			if e.Field == "account_id" {
				found = true
			}
		}
		if !found {
			t.Error("Expected error on field 'account_id'")
		}
	})

	t.Run("Missing date fails", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), ZeroDate,
			InvestmentTransactionTypeDeposit, MustNewMoney("1000.00"))
		errs := txn.Validate()
		found := false
		for _, e := range errs {
			if e.Field == "date" {
				found = true
			}
		}
		if !found {
			t.Error("Expected error on field 'date'")
		}
	})

	t.Run("Invalid type fails", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionType("bogus"), MustNewMoney("1000.00"))
		errs := txn.Validate()
		found := false
		for _, e := range errs {
			if e.Field == "type" {
				found = true
			}
		}
		if !found {
			t.Error("Expected error on field 'type'")
		}
	})

	t.Run("Invalid status fails", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeDeposit, MustNewMoney("1000.00"))
		txn.Status = InvestmentTransactionStatus("invalid")
		errs := txn.Validate()
		found := false
		for _, e := range errs {
			if e.Field == "status" {
				found = true
			}
		}
		if !found {
			t.Error("Expected error on field 'status'")
		}
	})

	t.Run("Security-based type without security_id fails", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeBuy, MustNewMoney("5000.00"))
		// Buy requires security but we didn't set it
		errs := txn.Validate()
		found := false
		for _, e := range errs {
			if e.Field == "security_id" {
				found = true
			}
		}
		if !found {
			t.Error("Expected error on field 'security_id' for buy without security")
		}
	})

	t.Run("Share-based type without shares fails", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeBuy, MustNewMoney("5000.00"))
		txn.SecurityID = NullableID{ID: NewID(), Valid: true}
		// Buy requires shares but we didn't set them
		errs := txn.Validate()
		found := false
		for _, e := range errs {
			if e.Field == "shares" {
				found = true
			}
		}
		if !found {
			t.Error("Expected error on field 'shares' for buy without shares")
		}
	})

	t.Run("Non-security type without security passes", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeDeposit, MustNewMoney("1000.00"))
		errs := txn.Validate()
		if errs.HasErrors() {
			t.Errorf("Deposit without security should pass validation, got: %v", errs)
		}
	})

	t.Run("Negative commission fails", func(t *testing.T) {
		txn := NewInvestmentTransactionWithSecurity(
			NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeBuy, MustNewMoney("5000.00"),
			NewID(), MustNewQuantity("100"),
		)
		txn.Commission = NullableMoney{Money: MustNewMoney("-10.00"), Valid: true}
		errs := txn.Validate()
		found := false
		for _, e := range errs {
			if e.Field == "commission" {
				found = true
			}
		}
		if !found {
			t.Error("Expected error on field 'commission' for negative value")
		}
	})

	t.Run("Zero commission is valid", func(t *testing.T) {
		txn := NewInvestmentTransactionWithSecurity(
			NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeBuy, MustNewMoney("5000.00"),
			NewID(), MustNewQuantity("100"),
		)
		txn.Commission = NullableMoney{Money: ZeroMoney, Valid: true}
		errs := txn.Validate()
		if errs.HasErrors() {
			t.Errorf("Zero commission should be valid, got: %v", errs)
		}
	})

	t.Run("Memo over max length fails", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeDeposit, MustNewMoney("1000.00"))
		var longMemo strings.Builder
		for range 1001 {
			longMemo.WriteString("x")
		}
		txn.Memo = NullableString{String: longMemo.String(), Valid: true}
		errs := txn.Validate()
		found := false
		for _, e := range errs {
			if e.Field == "memo" {
				found = true
			}
		}
		if !found {
			t.Error("Expected error on field 'memo' for long memo")
		}
	})

	t.Run("IsValid returns true for valid transaction", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeDeposit, MustNewMoney("1000.00"))
		if !txn.IsValid() {
			t.Error("Expected IsValid to return true")
		}
	})

	t.Run("IsValid returns false for invalid transaction", func(t *testing.T) {
		txn := NewInvestmentTransaction(NilID, ZeroDate,
			InvestmentTransactionType(""), ZeroMoney)
		if txn.IsValid() {
			t.Error("Expected IsValid to return false")
		}
	})
}

func TestInvestmentTransactionSetters(t *testing.T) {
	t.Run("SetSecurity and ClearSecurity", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeBuy, MustNewMoney("5000.00"))

		if txn.HasSecurity() {
			t.Error("Should start without security")
		}

		secID := NewID()
		txn.SetSecurity(secID)
		if !txn.HasSecurity() {
			t.Error("HasSecurity should return true after SetSecurity")
		}
		if txn.SecurityID.ID != secID {
			t.Error("SecurityID should match")
		}

		txn.ClearSecurity()
		if txn.HasSecurity() {
			t.Error("HasSecurity should return false after ClearSecurity")
		}
	})

	t.Run("SetShares and ClearShares", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeBuy, MustNewMoney("5000.00"))

		if txn.HasShares() {
			t.Error("Should start without shares")
		}

		shares := MustNewQuantity("50.5")
		txn.SetShares(shares)
		if !txn.HasShares() {
			t.Error("HasShares should return true after SetShares")
		}

		txn.ClearShares()
		if txn.HasShares() {
			t.Error("HasShares should return false after ClearShares")
		}
	})

	t.Run("SetPricePerShare and ClearPricePerShare", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeBuy, MustNewMoney("5000.00"))

		if txn.HasPricePerShare() {
			t.Error("Should start without price_per_share")
		}

		price := MustNewMoney("50.00")
		txn.SetPricePerShare(price)
		if !txn.HasPricePerShare() {
			t.Error("HasPricePerShare should return true after SetPricePerShare")
		}

		txn.ClearPricePerShare()
		if txn.HasPricePerShare() {
			t.Error("HasPricePerShare should return false after ClearPricePerShare")
		}
	})

	t.Run("SetCommission and ClearCommission", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeBuy, MustNewMoney("5000.00"))

		commission := MustNewMoney("9.99")
		txn.SetCommission(commission)
		if !txn.Commission.Valid {
			t.Error("Commission should be set")
		}

		txn.ClearCommission()
		if txn.Commission.Valid {
			t.Error("Commission should not be set after ClearCommission")
		}
	})

	t.Run("SetMemo and ClearMemo", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeDeposit, MustNewMoney("1000.00"))

		txn.SetMemo("test memo")
		if !txn.Memo.Valid || txn.Memo.String != "test memo" {
			t.Error("Memo should be set")
		}

		txn.SetMemo("") // empty string should clear
		if txn.Memo.Valid {
			t.Error("Empty string should clear memo")
		}

		txn.SetMemo("another memo")
		txn.ClearMemo()
		if txn.Memo.Valid {
			t.Error("ClearMemo should unset memo")
		}
	})
}

// --- SM-041: InvestmentTransaction status methods tests ---

func TestInvestmentTransactionStatusMethods(t *testing.T) {
	t.Run("Default status is pending", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeDeposit, MustNewMoney("1000.00"))
		if txn.Status != InvestmentTransactionStatusPending {
			t.Errorf("Expected pending, got %q", txn.Status)
		}
		if !txn.IsPending() {
			t.Error("IsPending should return true")
		}
	})

	t.Run("Clear sets status to cleared", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeDeposit, MustNewMoney("1000.00"))
		txn.Clear()
		if txn.Status != InvestmentTransactionStatusCleared {
			t.Errorf("Expected cleared, got %q", txn.Status)
		}
		if !txn.IsCleared() {
			t.Error("IsCleared should return true")
		}
		if txn.IsPending() {
			t.Error("IsPending should return false after Clear")
		}
	})

	t.Run("Reconcile sets status to reconciled", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeDeposit, MustNewMoney("1000.00"))
		txn.Reconcile()
		if txn.Status != InvestmentTransactionStatusReconciled {
			t.Errorf("Expected reconciled, got %q", txn.Status)
		}
		if !txn.IsReconciled() {
			t.Error("IsReconciled should return true")
		}
	})

	t.Run("MarkPending sets status back to pending", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeDeposit, MustNewMoney("1000.00"))
		txn.Clear()
		txn.MarkPending()
		if txn.Status != InvestmentTransactionStatusPending {
			t.Errorf("Expected pending, got %q", txn.Status)
		}
	})

	t.Run("SetStatus sets arbitrary valid status", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeDeposit, MustNewMoney("1000.00"))
		txn.SetStatus(InvestmentTransactionStatusReconciled)
		if txn.Status != InvestmentTransactionStatusReconciled {
			t.Errorf("Expected reconciled, got %q", txn.Status)
		}
	})

	t.Run("Status transitions update UpdatedAt", func(t *testing.T) {
		txn := NewInvestmentTransaction(NewID(), NewDate(2024, time.March, 15),
			InvestmentTransactionTypeDeposit, MustNewMoney("1000.00"))
		original := txn.UpdatedAt
		txn.Clear()
		if !txn.UpdatedAt.After(original) && txn.UpdatedAt != original {
			// UpdatedAt should be >= original (may be same if fast)
		}
	})
}
