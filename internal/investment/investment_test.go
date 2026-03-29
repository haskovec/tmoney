package investment

import (
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

// --- SM-039: TransactionType enum tests ---

func TestTransactionType(t *testing.T) {
	t.Run("AllTransactionTypes returns all types", func(t *testing.T) {
		txnTypes := AllTransactionTypes()
		expected := 12
		if len(txnTypes) != expected {
			t.Errorf("Expected %d investment transaction types, got %d", expected, len(txnTypes))
		}
	})

	t.Run("String returns correct value", func(t *testing.T) {
		tests := []struct {
			txnType  TransactionType
			expected string
		}{
			{TransactionTypeBuy, "buy"},
			{TransactionTypeSell, "sell"},
			{TransactionTypeDividend, "dividend"},
			{TransactionTypeReinvestDividend, "reinvest_dividend"},
			{TransactionTypeFee, "fee"},
			{TransactionTypeFeeLiquidation, "fee_liquidation"},
			{TransactionTypeDeposit, "deposit"},
			{TransactionTypeWithdrawal, "withdrawal"},
			{TransactionTypeInterest, "interest"},
			{TransactionTypeTransferShares, "transfer_shares"},
			{TransactionTypeTransferCash, "transfer_cash"},
			{TransactionTypeExchange, "exchange"},
		}
		for _, tc := range tests {
			if tc.txnType.String() != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, tc.txnType.String())
			}
		}
	})

	t.Run("IsValid returns true for all valid types", func(t *testing.T) {
		for _, itt := range AllTransactionTypes() {
			if !itt.IsValid() {
				t.Errorf("IsValid should return true for %q", itt)
			}
		}
	})

	t.Run("IsValid returns false for invalid types", func(t *testing.T) {
		invalid := TransactionType("refund")
		if invalid.IsValid() {
			t.Error("IsValid should return false for 'refund'")
		}

		empty := TransactionType("")
		if empty.IsValid() {
			t.Error("IsValid should return false for empty string")
		}
	})

	t.Run("DisplayName returns human-readable names", func(t *testing.T) {
		tests := []struct {
			txnType  TransactionType
			expected string
		}{
			{TransactionTypeBuy, "Buy"},
			{TransactionTypeSell, "Sell"},
			{TransactionTypeDividend, "Dividend"},
			{TransactionTypeReinvestDividend, "Reinvest Dividend"},
			{TransactionTypeFee, "Fee"},
			{TransactionTypeFeeLiquidation, "Fee via Liquidation"},
			{TransactionTypeDeposit, "Deposit"},
			{TransactionTypeWithdrawal, "Withdrawal"},
			{TransactionTypeInterest, "Interest"},
			{TransactionTypeTransferShares, "Transfer Shares"},
			{TransactionTypeTransferCash, "Transfer Cash"},
			{TransactionTypeExchange, "Exchange"},
		}
		for _, tc := range tests {
			if tc.txnType.DisplayName() != tc.expected {
				t.Errorf("DisplayName for %q: expected %q, got %q",
					tc.txnType, tc.expected, tc.txnType.DisplayName())
			}
		}
	})

	t.Run("DisplayName returns raw string for unknown type", func(t *testing.T) {
		unknown := TransactionType("unknown")
		if unknown.DisplayName() != "unknown" {
			t.Errorf("Expected 'unknown', got %q", unknown.DisplayName())
		}
	})
}

func TestTransactionTypeRequiresSecurity(t *testing.T) {
	requiresSecurity := []TransactionType{
		TransactionTypeBuy, TransactionTypeSell,
		TransactionTypeDividend, TransactionTypeReinvestDividend,
		TransactionTypeFeeLiquidation, TransactionTypeTransferShares,
		TransactionTypeExchange,
	}
	doesNotRequireSecurity := []TransactionType{
		TransactionTypeFee, TransactionTypeDeposit,
		TransactionTypeWithdrawal, TransactionTypeInterest,
		TransactionTypeTransferCash,
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

func TestTransactionTypeRequiresShares(t *testing.T) {
	requiresShares := []TransactionType{
		TransactionTypeBuy, TransactionTypeSell,
		TransactionTypeReinvestDividend, TransactionTypeFeeLiquidation,
		TransactionTypeTransferShares, TransactionTypeExchange,
	}
	doesNotRequireShares := []TransactionType{
		TransactionTypeDividend, TransactionTypeFee,
		TransactionTypeDeposit, TransactionTypeWithdrawal,
		TransactionTypeInterest, TransactionTypeTransferCash,
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

func TestTransactionTypeAffectsCash(t *testing.T) {
	affectsCash := []TransactionType{
		TransactionTypeBuy, TransactionTypeSell,
		TransactionTypeDividend, TransactionTypeFee,
		TransactionTypeDeposit, TransactionTypeWithdrawal,
		TransactionTypeInterest, TransactionTypeTransferCash,
	}
	doesNotAffectCash := []TransactionType{
		TransactionTypeReinvestDividend, TransactionTypeFeeLiquidation,
		TransactionTypeTransferShares, TransactionTypeExchange,
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

func TestParseTransactionType(t *testing.T) {
	t.Run("Parses valid types", func(t *testing.T) {
		tests := []struct {
			input    string
			expected TransactionType
		}{
			{"buy", TransactionTypeBuy},
			{"sell", TransactionTypeSell},
			{"dividend", TransactionTypeDividend},
			{"reinvest_dividend", TransactionTypeReinvestDividend},
			{"fee", TransactionTypeFee},
			{"fee_liquidation", TransactionTypeFeeLiquidation},
			{"deposit", TransactionTypeDeposit},
			{"withdrawal", TransactionTypeWithdrawal},
			{"interest", TransactionTypeInterest},
			{"transfer_shares", TransactionTypeTransferShares},
			{"transfer_cash", TransactionTypeTransferCash},
			{"exchange", TransactionTypeExchange},
		}
		for _, tc := range tests {
			itt, err := ParseTransactionType(tc.input)
			if err != nil {
				t.Errorf("ParseTransactionType(%q) returned error: %v", tc.input, err)
			}
			if itt != tc.expected {
				t.Errorf("ParseTransactionType(%q) = %q, expected %q", tc.input, itt, tc.expected)
			}
		}
	})

	t.Run("Parses case-insensitive", func(t *testing.T) {
		tests := []struct {
			input    string
			expected TransactionType
		}{
			{"Buy", TransactionTypeBuy},
			{"SELL", TransactionTypeSell},
			{"Dividend", TransactionTypeDividend},
			{"REINVEST_DIVIDEND", TransactionTypeReinvestDividend},
		}
		for _, tc := range tests {
			itt, err := ParseTransactionType(tc.input)
			if err != nil {
				t.Errorf("ParseTransactionType(%q) returned error: %v", tc.input, err)
			}
			if itt != tc.expected {
				t.Errorf("ParseTransactionType(%q) = %q, expected %q", tc.input, itt, tc.expected)
			}
		}
	})

	t.Run("Rejects invalid types", func(t *testing.T) {
		invalidTypes := []string{"refund", "transfer", "bond_coupon", ""}
		for _, s := range invalidTypes {
			_, err := ParseTransactionType(s)
			if err == nil {
				t.Errorf("ParseTransactionType(%q) should return error", s)
			}
		}
	})
}

func TestTransactionTypeScanValue(t *testing.T) {
	t.Run("Value returns string", func(t *testing.T) {
		val, err := TransactionTypeBuy.Value()
		if err != nil {
			t.Fatalf("Value() returned error: %v", err)
		}
		if val != "buy" {
			t.Errorf("Expected 'buy', got %v", val)
		}
	})

	t.Run("Scan from string", func(t *testing.T) {
		var itt TransactionType
		err := itt.Scan("sell")
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if itt != TransactionTypeSell {
			t.Errorf("Expected TransactionTypeSell, got %q", itt)
		}
	})

	t.Run("Scan from bytes", func(t *testing.T) {
		var itt TransactionType
		err := itt.Scan([]byte("dividend"))
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if itt != TransactionTypeDividend {
			t.Errorf("Expected TransactionTypeDividend, got %q", itt)
		}
	})

	t.Run("Scan from nil", func(t *testing.T) {
		var itt TransactionType
		err := itt.Scan(nil)
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		if itt != "" {
			t.Errorf("Expected empty string, got %q", itt)
		}
	})

	t.Run("Scan from unsupported type returns error", func(t *testing.T) {
		var itt TransactionType
		err := itt.Scan(123)
		if err == nil {
			t.Error("Scan from int should return error")
		}
	})
}

// --- SM-040: Transaction model and validation tests ---

func TestNewTransaction(t *testing.T) {
	t.Run("Creates with required fields", func(t *testing.T) {
		accountID := types.NewID()
		date := types.NewDate(2024, time.March, 15)
		txnType := TransactionTypeBuy
		amount := types.MustNewMoney("1000.00")

		txn := NewTransaction(accountID, date, txnType, amount)

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
		if txn.Status != TransactionStatusPending {
			t.Errorf("Expected status 'pending', got %q", txn.Status)
		}
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

func TestNewTransactionWithSecurity(t *testing.T) {
	t.Run("Creates with security and shares", func(t *testing.T) {
		accountID := types.NewID()
		securityID := types.NewID()
		date := types.NewDate(2024, time.March, 15)
		amount := types.MustNewMoney("5000.00")
		shares := types.MustNewQuantity("100")

		txn := NewTransactionWithSecurity(
			accountID, date, TransactionTypeBuy,
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

func TestTransactionValidate(t *testing.T) {
	t.Run("Valid buy transaction passes validation", func(t *testing.T) {
		txn := NewTransactionWithSecurity(
			types.NewID(), types.NewDate(2024, time.March, 15),
			TransactionTypeBuy, types.MustNewMoney("5000.00"),
			types.NewID(), types.MustNewQuantity("100"),
		)

		errs := txn.Validate()
		if errs.HasErrors() {
			t.Errorf("Expected no errors, got: %v", errs)
		}
	})

	t.Run("Missing account_id fails", func(t *testing.T) {
		txn := NewTransaction(types.NilID, types.NewDate(2024, time.March, 15),
			TransactionTypeDeposit, types.MustNewMoney("1000.00"))
		errs := txn.Validate()
		if !errs.HasErrors() {
			t.Error("Expected validation error for missing account_id")
		}
	})

	t.Run("Security-based type without security_id fails", func(t *testing.T) {
		txn := NewTransaction(types.NewID(), types.NewDate(2024, time.March, 15),
			TransactionTypeBuy, types.MustNewMoney("5000.00"))
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

	t.Run("Negative commission fails", func(t *testing.T) {
		txn := NewTransactionWithSecurity(
			types.NewID(), types.NewDate(2024, time.March, 15),
			TransactionTypeBuy, types.MustNewMoney("5000.00"),
			types.NewID(), types.MustNewQuantity("100"),
		)
		txn.Commission = types.NullableMoney{Money: types.MustNewMoney("-10.00"), Valid: true}
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

	t.Run("Memo over max length fails", func(t *testing.T) {
		txn := NewTransaction(types.NewID(), types.NewDate(2024, time.March, 15),
			TransactionTypeDeposit, types.MustNewMoney("1000.00"))
		var longMemo strings.Builder
		for range 1001 {
			longMemo.WriteString("x")
		}
		txn.Memo = types.NullableString{String: longMemo.String(), Valid: true}
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
}

func TestTransactionSetters(t *testing.T) {
	t.Run("SetSecurity and ClearSecurity", func(t *testing.T) {
		txn := NewTransaction(types.NewID(), types.NewDate(2024, time.March, 15),
			TransactionTypeBuy, types.MustNewMoney("5000.00"))

		if txn.HasSecurity() {
			t.Error("Should start without security")
		}

		secID := types.NewID()
		txn.SetSecurity(secID)
		if !txn.HasSecurity() {
			t.Error("HasSecurity should return true after SetSecurity")
		}

		txn.ClearSecurity()
		if txn.HasSecurity() {
			t.Error("HasSecurity should return false after ClearSecurity")
		}
	})

	t.Run("SetMemo and ClearMemo", func(t *testing.T) {
		txn := NewTransaction(types.NewID(), types.NewDate(2024, time.March, 15),
			TransactionTypeDeposit, types.MustNewMoney("1000.00"))

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

func TestTransactionStatusMethods(t *testing.T) {
	t.Run("Default status is pending", func(t *testing.T) {
		txn := NewTransaction(types.NewID(), types.NewDate(2024, time.March, 15),
			TransactionTypeDeposit, types.MustNewMoney("1000.00"))
		if txn.Status != TransactionStatusPending {
			t.Errorf("Expected pending, got %q", txn.Status)
		}
		if !txn.IsPending() {
			t.Error("IsPending should return true")
		}
	})

	t.Run("Clear sets status to cleared", func(t *testing.T) {
		txn := NewTransaction(types.NewID(), types.NewDate(2024, time.March, 15),
			TransactionTypeDeposit, types.MustNewMoney("1000.00"))
		txn.Clear()
		if !txn.IsCleared() {
			t.Error("IsCleared should return true")
		}
	})

	t.Run("Reconcile sets status to reconciled", func(t *testing.T) {
		txn := NewTransaction(types.NewID(), types.NewDate(2024, time.March, 15),
			TransactionTypeDeposit, types.MustNewMoney("1000.00"))
		txn.Reconcile()
		if !txn.IsReconciled() {
			t.Error("IsReconciled should return true")
		}
	})
}
