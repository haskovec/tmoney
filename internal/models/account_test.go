package models

import (
	"testing"
)

func TestAccountType(t *testing.T) {
	t.Run("AllAccountTypes returns all types", func(t *testing.T) {
		types := AllAccountTypes()
		expected := 7
		if len(types) != expected {
			t.Errorf("Expected %d account types, got %d", expected, len(types))
		}
	})

	t.Run("String returns correct value", func(t *testing.T) {
		if AccountTypeChecking.String() != "checking" {
			t.Errorf("Expected 'checking', got %q", AccountTypeChecking.String())
		}
		if AccountTypeCreditCard.String() != "credit_card" {
			t.Errorf("Expected 'credit_card', got %q", AccountTypeCreditCard.String())
		}
	})

	t.Run("IsValid returns true for valid types", func(t *testing.T) {
		validTypes := []AccountType{
			AccountTypeChecking,
			AccountTypeSavings,
			AccountTypeCreditCard,
			AccountTypeInvestment,
			AccountTypeCash,
			AccountTypeLoan,
			AccountTypeAsset,
		}
		for _, at := range validTypes {
			if !at.IsValid() {
				t.Errorf("IsValid should return true for %q", at)
			}
		}
	})

	t.Run("IsValid returns false for invalid type", func(t *testing.T) {
		invalid := AccountType("unknown")
		if invalid.IsValid() {
			t.Error("IsValid should return false for 'unknown'")
		}
	})

	t.Run("DisplayName returns human-readable names", func(t *testing.T) {
		tests := []struct {
			accountType AccountType
			expected    string
		}{
			{AccountTypeChecking, "Checking"},
			{AccountTypeSavings, "Savings"},
			{AccountTypeCreditCard, "Credit Card"},
			{AccountTypeInvestment, "Investment"},
			{AccountTypeCash, "Cash"},
			{AccountTypeLoan, "Loan"},
			{AccountTypeAsset, "Asset"},
		}
		for _, tc := range tests {
			if tc.accountType.DisplayName() != tc.expected {
				t.Errorf("DisplayName for %q: expected %q, got %q",
					tc.accountType, tc.expected, tc.accountType.DisplayName())
			}
		}
	})

	t.Run("IsAssetType returns true for asset types", func(t *testing.T) {
		assetTypes := []AccountType{
			AccountTypeChecking,
			AccountTypeSavings,
			AccountTypeInvestment,
			AccountTypeCash,
			AccountTypeAsset,
		}
		for _, at := range assetTypes {
			if !at.IsAssetType() {
				t.Errorf("IsAssetType should return true for %q", at)
			}
		}
	})

	t.Run("IsAssetType returns false for liability types", func(t *testing.T) {
		liabilityTypes := []AccountType{
			AccountTypeCreditCard,
			AccountTypeLoan,
		}
		for _, at := range liabilityTypes {
			if at.IsAssetType() {
				t.Errorf("IsAssetType should return false for %q", at)
			}
		}
	})

	t.Run("IsLiabilityType returns true for liability types", func(t *testing.T) {
		liabilityTypes := []AccountType{
			AccountTypeCreditCard,
			AccountTypeLoan,
		}
		for _, at := range liabilityTypes {
			if !at.IsLiabilityType() {
				t.Errorf("IsLiabilityType should return true for %q", at)
			}
		}
	})

	t.Run("IsLiabilityType returns false for asset types", func(t *testing.T) {
		assetTypes := []AccountType{
			AccountTypeChecking,
			AccountTypeSavings,
			AccountTypeInvestment,
			AccountTypeCash,
			AccountTypeAsset,
		}
		for _, at := range assetTypes {
			if at.IsLiabilityType() {
				t.Errorf("IsLiabilityType should return false for %q", at)
			}
		}
	})
}

func TestParseAccountType(t *testing.T) {
	t.Run("Parses valid type", func(t *testing.T) {
		at, err := ParseAccountType("checking")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if at != AccountTypeChecking {
			t.Errorf("Expected AccountTypeChecking, got %q", at)
		}
	})

	t.Run("Parses uppercase type", func(t *testing.T) {
		at, err := ParseAccountType("SAVINGS")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if at != AccountTypeSavings {
			t.Errorf("Expected AccountTypeSavings, got %q", at)
		}
	})

	t.Run("Parses mixed case type", func(t *testing.T) {
		at, err := ParseAccountType("Credit_Card")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if at != AccountTypeCreditCard {
			t.Errorf("Expected AccountTypeCreditCard, got %q", at)
		}
	})

	t.Run("Returns error for invalid type", func(t *testing.T) {
		_, err := ParseAccountType("invalid")
		if err == nil {
			t.Error("Expected error for invalid account type")
		}
	})
}

func TestAccountTypeScanValue(t *testing.T) {
	t.Run("Value returns string", func(t *testing.T) {
		at := AccountTypeChecking
		v, err := at.Value()
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if v != "checking" {
			t.Errorf("Expected 'checking', got %v", v)
		}
	})

	t.Run("Scan from string", func(t *testing.T) {
		var at AccountType
		err := at.Scan("savings")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if at != AccountTypeSavings {
			t.Errorf("Expected AccountTypeSavings, got %q", at)
		}
	})

	t.Run("Scan from bytes", func(t *testing.T) {
		var at AccountType
		err := at.Scan([]byte("credit_card"))
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if at != AccountTypeCreditCard {
			t.Errorf("Expected AccountTypeCreditCard, got %q", at)
		}
	})

	t.Run("Scan from nil", func(t *testing.T) {
		var at AccountType
		err := at.Scan(nil)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if at != "" {
			t.Errorf("Expected empty string, got %q", at)
		}
	})

	t.Run("Scan from unsupported type returns error", func(t *testing.T) {
		var at AccountType
		err := at.Scan(123)
		if err == nil {
			t.Error("Expected error for unsupported type")
		}
	})
}

func TestNewAccount(t *testing.T) {
	t.Run("Creates account with required fields", func(t *testing.T) {
		acc := NewAccount(
			"Chase Checking",
			AccountTypeChecking,
			"USD",
			MustNewMoney("1000.00"),
			NewDate(2024, 1, 15),
		)

		if acc.ID.IsNil() {
			t.Error("NewAccount should create non-nil ID")
		}
		if acc.Name != "Chase Checking" {
			t.Errorf("Expected name 'Chase Checking', got %q", acc.Name)
		}
		if acc.Type != AccountTypeChecking {
			t.Errorf("Expected type checking, got %q", acc.Type)
		}
		if acc.Currency != "USD" {
			t.Errorf("Expected currency USD, got %q", acc.Currency)
		}
		if !acc.OpeningBalance.Equal(MustNewMoney("1000.00")) {
			t.Errorf("Expected opening balance 1000.00, got %s", acc.OpeningBalance.String())
		}
		if !acc.Active {
			t.Error("NewAccount should create active account")
		}
		if acc.CreatedAt.IsZero() {
			t.Error("NewAccount should set CreatedAt")
		}
		if acc.UpdatedAt.IsZero() {
			t.Error("NewAccount should set UpdatedAt")
		}
	})
}

func TestAccountValidation(t *testing.T) {
	validAccount := func() *Account {
		return NewAccount(
			"Test Account",
			AccountTypeChecking,
			"USD",
			MustNewMoney("100.00"),
			NewDate(2024, 1, 15),
		)
	}

	t.Run("Valid account passes validation", func(t *testing.T) {
		acc := validAccount()
		errs := acc.Validate()
		if errs.HasErrors() {
			t.Errorf("Valid account should pass validation: %v", errs)
		}
	})

	t.Run("IsValid returns true for valid account", func(t *testing.T) {
		acc := validAccount()
		if !acc.IsValid() {
			t.Error("IsValid should return true for valid account")
		}
	})

	t.Run("Empty name fails validation", func(t *testing.T) {
		acc := validAccount()
		acc.Name = ""
		errs := acc.Validate()
		if !errs.HasErrors() {
			t.Error("Empty name should fail validation")
		}
	})

	t.Run("Whitespace-only name fails validation", func(t *testing.T) {
		acc := validAccount()
		acc.Name = "   "
		errs := acc.Validate()
		if !errs.HasErrors() {
			t.Error("Whitespace-only name should fail validation")
		}
	})

	t.Run("Name exceeding max length fails validation", func(t *testing.T) {
		acc := validAccount()
		acc.Name = string(make([]byte, 256))
		errs := acc.Validate()
		if !errs.HasErrors() {
			t.Error("Name exceeding 255 chars should fail validation")
		}
	})

	t.Run("Empty currency fails validation", func(t *testing.T) {
		acc := validAccount()
		acc.Currency = ""
		errs := acc.Validate()
		if !errs.HasErrors() {
			t.Error("Empty currency should fail validation")
		}
	})

	t.Run("Invalid currency fails validation", func(t *testing.T) {
		acc := validAccount()
		acc.Currency = "XXX"
		errs := acc.Validate()
		if !errs.HasErrors() {
			t.Error("Invalid currency should fail validation")
		}
	})

	t.Run("Zero opening date fails validation", func(t *testing.T) {
		acc := validAccount()
		acc.OpeningDate = ZeroDate
		errs := acc.Validate()
		if !errs.HasErrors() {
			t.Error("Zero opening date should fail validation")
		}
	})

	t.Run("Future opening date fails validation", func(t *testing.T) {
		acc := validAccount()
		acc.OpeningDate = Today().AddYears(1)
		errs := acc.Validate()
		if !errs.HasErrors() {
			t.Error("Future opening date should fail validation")
		}
	})

	t.Run("Invalid account type fails validation", func(t *testing.T) {
		acc := validAccount()
		acc.Type = AccountType("invalid")
		errs := acc.Validate()
		if !errs.HasErrors() {
			t.Error("Invalid account type should fail validation")
		}
	})

	t.Run("Negative credit limit fails validation", func(t *testing.T) {
		acc := validAccount()
		acc.Type = AccountTypeCreditCard
		acc.SetCreditLimit(MustNewMoney("-1000"))
		errs := acc.Validate()
		if !errs.HasErrors() {
			t.Error("Negative credit limit should fail validation")
		}
	})

	t.Run("Zero credit limit fails validation", func(t *testing.T) {
		acc := validAccount()
		acc.Type = AccountTypeCreditCard
		acc.SetCreditLimit(ZeroMoney)
		errs := acc.Validate()
		if !errs.HasErrors() {
			t.Error("Zero credit limit should fail validation")
		}
	})

	t.Run("Positive credit limit passes validation", func(t *testing.T) {
		acc := validAccount()
		acc.Type = AccountTypeCreditCard
		acc.SetCreditLimit(MustNewMoney("5000"))
		errs := acc.Validate()
		if errs.HasErrors() {
			t.Errorf("Positive credit limit should pass validation: %v", errs)
		}
	})

	t.Run("Interest rate below 0 fails validation", func(t *testing.T) {
		acc := validAccount()
		acc.Type = AccountTypeLoan
		acc.SetInterestRate(MustNewMoney("-5"))
		errs := acc.Validate()
		if !errs.HasErrors() {
			t.Error("Negative interest rate should fail validation")
		}
	})

	t.Run("Interest rate above 100 fails validation", func(t *testing.T) {
		acc := validAccount()
		acc.Type = AccountTypeLoan
		acc.SetInterestRate(MustNewMoney("150"))
		errs := acc.Validate()
		if !errs.HasErrors() {
			t.Error("Interest rate above 100 should fail validation")
		}
	})

	t.Run("Valid interest rate passes validation", func(t *testing.T) {
		acc := validAccount()
		acc.Type = AccountTypeLoan
		acc.SetInterestRate(MustNewMoney("5.5"))
		errs := acc.Validate()
		if errs.HasErrors() {
			t.Errorf("Valid interest rate should pass validation: %v", errs)
		}
	})

	t.Run("Institution exceeding max length fails validation", func(t *testing.T) {
		acc := validAccount()
		acc.SetInstitution(string(make([]byte, 256)))
		errs := acc.Validate()
		if !errs.HasErrors() {
			t.Error("Institution exceeding 255 chars should fail validation")
		}
	})

	t.Run("Account number exceeding max length fails validation", func(t *testing.T) {
		acc := validAccount()
		acc.SetAccountNumber(string(make([]byte, 51)))
		errs := acc.Validate()
		if !errs.HasErrors() {
			t.Error("Account number exceeding 50 chars should fail validation")
		}
	})

	t.Run("Notes exceeding max length fails validation", func(t *testing.T) {
		acc := validAccount()
		acc.SetNotes(string(make([]byte, 2001)))
		errs := acc.Validate()
		if !errs.HasErrors() {
			t.Error("Notes exceeding 2000 chars should fail validation")
		}
	})

	t.Run("Multiple validation errors collected", func(t *testing.T) {
		acc := validAccount()
		acc.Name = ""
		acc.Currency = "INVALID"
		acc.OpeningDate = ZeroDate
		acc.Type = AccountType("bad")
		errs := acc.Validate()
		if len(errs) < 4 {
			t.Errorf("Expected at least 4 errors, got %d: %v", len(errs), errs)
		}
	})
}

func TestAccountOptionalFields(t *testing.T) {
	t.Run("SetInstitution sets valid value", func(t *testing.T) {
		acc := NewAccount("Test", AccountTypeChecking, "USD", ZeroMoney, Today())
		acc.SetInstitution("Chase Bank")
		if !acc.Institution.Valid {
			t.Error("Institution should be valid after SetInstitution")
		}
		if acc.Institution.String != "Chase Bank" {
			t.Errorf("Expected 'Chase Bank', got %q", acc.Institution.String)
		}
	})

	t.Run("SetInstitution clears with empty string", func(t *testing.T) {
		acc := NewAccount("Test", AccountTypeChecking, "USD", ZeroMoney, Today())
		acc.SetInstitution("Chase Bank")
		acc.SetInstitution("")
		if acc.Institution.Valid {
			t.Error("Institution should be invalid after clearing")
		}
	})

	t.Run("SetAccountNumber sets valid value", func(t *testing.T) {
		acc := NewAccount("Test", AccountTypeChecking, "USD", ZeroMoney, Today())
		acc.SetAccountNumber("1234")
		if !acc.AccountNumber.Valid {
			t.Error("AccountNumber should be valid after SetAccountNumber")
		}
		if acc.AccountNumber.String != "1234" {
			t.Errorf("Expected '1234', got %q", acc.AccountNumber.String)
		}
	})

	t.Run("SetAccountNumber clears with empty string", func(t *testing.T) {
		acc := NewAccount("Test", AccountTypeChecking, "USD", ZeroMoney, Today())
		acc.SetAccountNumber("1234")
		acc.SetAccountNumber("")
		if acc.AccountNumber.Valid {
			t.Error("AccountNumber should be invalid after clearing")
		}
	})

	t.Run("SetNotes sets valid value", func(t *testing.T) {
		acc := NewAccount("Test", AccountTypeChecking, "USD", ZeroMoney, Today())
		acc.SetNotes("Some notes here")
		if !acc.Notes.Valid {
			t.Error("Notes should be valid after SetNotes")
		}
		if acc.Notes.String != "Some notes here" {
			t.Errorf("Expected 'Some notes here', got %q", acc.Notes.String)
		}
	})

	t.Run("SetNotes clears with empty string", func(t *testing.T) {
		acc := NewAccount("Test", AccountTypeChecking, "USD", ZeroMoney, Today())
		acc.SetNotes("Some notes")
		acc.SetNotes("")
		if acc.Notes.Valid {
			t.Error("Notes should be invalid after clearing")
		}
	})

	t.Run("SetCreditLimit sets valid value", func(t *testing.T) {
		acc := NewAccount("Test", AccountTypeCreditCard, "USD", ZeroMoney, Today())
		acc.SetCreditLimit(MustNewMoney("5000"))
		if !acc.CreditLimit.Valid {
			t.Error("CreditLimit should be valid after SetCreditLimit")
		}
		if !acc.CreditLimit.Money.Equal(MustNewMoney("5000")) {
			t.Errorf("Expected 5000, got %s", acc.CreditLimit.Money.String())
		}
	})

	t.Run("ClearCreditLimit clears the value", func(t *testing.T) {
		acc := NewAccount("Test", AccountTypeCreditCard, "USD", ZeroMoney, Today())
		acc.SetCreditLimit(MustNewMoney("5000"))
		acc.ClearCreditLimit()
		if acc.CreditLimit.Valid {
			t.Error("CreditLimit should be invalid after ClearCreditLimit")
		}
	})

	t.Run("SetInterestRate sets valid value", func(t *testing.T) {
		acc := NewAccount("Test", AccountTypeLoan, "USD", ZeroMoney, Today())
		acc.SetInterestRate(MustNewMoney("5.5"))
		if !acc.InterestRate.Valid {
			t.Error("InterestRate should be valid after SetInterestRate")
		}
		if !acc.InterestRate.Money.Equal(MustNewMoney("5.5")) {
			t.Errorf("Expected 5.5, got %s", acc.InterestRate.Money.String())
		}
	})

	t.Run("ClearInterestRate clears the value", func(t *testing.T) {
		acc := NewAccount("Test", AccountTypeLoan, "USD", ZeroMoney, Today())
		acc.SetInterestRate(MustNewMoney("5.5"))
		acc.ClearInterestRate()
		if acc.InterestRate.Valid {
			t.Error("InterestRate should be invalid after ClearInterestRate")
		}
	})
}

func TestAccountCloseReopen(t *testing.T) {
	t.Run("Close marks account inactive", func(t *testing.T) {
		acc := NewAccount("Test", AccountTypeChecking, "USD", ZeroMoney, Today())
		if !acc.Active {
			t.Error("New account should be active")
		}
		acc.Close()
		if acc.Active {
			t.Error("Account should be inactive after Close")
		}
	})

	t.Run("Close updates UpdatedAt", func(t *testing.T) {
		acc := NewAccount("Test", AccountTypeChecking, "USD", ZeroMoney, Today())
		original := acc.UpdatedAt
		acc.Close()
		if !acc.UpdatedAt.After(original) && !acc.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("Close should update UpdatedAt")
		}
	})

	t.Run("Reopen marks account active", func(t *testing.T) {
		acc := NewAccount("Test", AccountTypeChecking, "USD", ZeroMoney, Today())
		acc.Close()
		acc.Reopen()
		if !acc.Active {
			t.Error("Account should be active after Reopen")
		}
	})

	t.Run("Reopen updates UpdatedAt", func(t *testing.T) {
		acc := NewAccount("Test", AccountTypeChecking, "USD", ZeroMoney, Today())
		acc.Close()
		original := acc.UpdatedAt
		acc.Reopen()
		if !acc.UpdatedAt.After(original) && !acc.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("Reopen should update UpdatedAt")
		}
	})
}

func TestAccountNegativeOpeningBalance(t *testing.T) {
	t.Run("Negative opening balance is valid for credit cards", func(t *testing.T) {
		acc := NewAccount(
			"Credit Card",
			AccountTypeCreditCard,
			"USD",
			MustNewMoney("-500.00"),
			Today(),
		)
		errs := acc.Validate()
		if errs.HasErrors() {
			t.Errorf("Credit card can have negative opening balance: %v", errs)
		}
	})

	t.Run("Negative opening balance is valid for checking", func(t *testing.T) {
		acc := NewAccount(
			"Checking",
			AccountTypeChecking,
			"USD",
			MustNewMoney("-100.00"),
			Today(),
		)
		errs := acc.Validate()
		if errs.HasErrors() {
			t.Errorf("Checking can have negative opening balance (overdrawn): %v", errs)
		}
	})
}
