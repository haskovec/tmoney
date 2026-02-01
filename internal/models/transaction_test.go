package models

import (
	"testing"
	"time"
)

func TestTransactionStatus(t *testing.T) {
	t.Run("AllTransactionStatuses returns all statuses", func(t *testing.T) {
		statuses := AllTransactionStatuses()
		expected := 3
		if len(statuses) != expected {
			t.Errorf("Expected %d transaction statuses, got %d", expected, len(statuses))
		}
	})

	t.Run("String returns correct value", func(t *testing.T) {
		tests := []struct {
			status   TransactionStatus
			expected string
		}{
			{TransactionStatusPending, "pending"},
			{TransactionStatusCleared, "cleared"},
			{TransactionStatusReconciled, "reconciled"},
		}
		for _, tc := range tests {
			if tc.status.String() != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, tc.status.String())
			}
		}
	})

	t.Run("IsValid returns true for valid statuses", func(t *testing.T) {
		validStatuses := []TransactionStatus{
			TransactionStatusPending,
			TransactionStatusCleared,
			TransactionStatusReconciled,
		}
		for _, ts := range validStatuses {
			if !ts.IsValid() {
				t.Errorf("IsValid should return true for %q", ts)
			}
		}
	})

	t.Run("IsValid returns false for invalid status", func(t *testing.T) {
		invalid := TransactionStatus("unknown")
		if invalid.IsValid() {
			t.Error("IsValid should return false for 'unknown'")
		}
	})

	t.Run("DisplayName returns human-readable names", func(t *testing.T) {
		tests := []struct {
			status   TransactionStatus
			expected string
		}{
			{TransactionStatusPending, "Pending"},
			{TransactionStatusCleared, "Cleared"},
			{TransactionStatusReconciled, "Reconciled"},
		}
		for _, tc := range tests {
			if tc.status.DisplayName() != tc.expected {
				t.Errorf("DisplayName for %q: expected %q, got %q",
					tc.status, tc.expected, tc.status.DisplayName())
			}
		}
	})

	t.Run("DisplayName returns raw string for unknown status", func(t *testing.T) {
		unknown := TransactionStatus("unknown")
		if unknown.DisplayName() != "unknown" {
			t.Errorf("Expected 'unknown', got %q", unknown.DisplayName())
		}
	})
}

func TestParseTransactionStatus(t *testing.T) {
	t.Run("Parses valid pending status", func(t *testing.T) {
		ts, err := ParseTransactionStatus("pending")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ts != TransactionStatusPending {
			t.Errorf("Expected TransactionStatusPending, got %q", ts)
		}
	})

	t.Run("Parses valid cleared status", func(t *testing.T) {
		ts, err := ParseTransactionStatus("cleared")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ts != TransactionStatusCleared {
			t.Errorf("Expected TransactionStatusCleared, got %q", ts)
		}
	})

	t.Run("Parses valid reconciled status", func(t *testing.T) {
		ts, err := ParseTransactionStatus("reconciled")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ts != TransactionStatusReconciled {
			t.Errorf("Expected TransactionStatusReconciled, got %q", ts)
		}
	})

	t.Run("Parses uppercase status", func(t *testing.T) {
		ts, err := ParseTransactionStatus("PENDING")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ts != TransactionStatusPending {
			t.Errorf("Expected TransactionStatusPending, got %q", ts)
		}
	})

	t.Run("Parses mixed case status", func(t *testing.T) {
		ts, err := ParseTransactionStatus("Cleared")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ts != TransactionStatusCleared {
			t.Errorf("Expected TransactionStatusCleared, got %q", ts)
		}
	})

	t.Run("Returns error for invalid status", func(t *testing.T) {
		_, err := ParseTransactionStatus("invalid")
		if err == nil {
			t.Error("Expected error for invalid transaction status")
		}
	})

	t.Run("Returns error for empty string", func(t *testing.T) {
		_, err := ParseTransactionStatus("")
		if err == nil {
			t.Error("Expected error for empty string")
		}
	})
}

func TestTransactionStatusScanValue(t *testing.T) {
	t.Run("Value returns string", func(t *testing.T) {
		ts := TransactionStatusPending
		v, err := ts.Value()
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if v != "pending" {
			t.Errorf("Expected 'pending', got %v", v)
		}
	})

	t.Run("Scan from string", func(t *testing.T) {
		var ts TransactionStatus
		err := ts.Scan("cleared")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ts != TransactionStatusCleared {
			t.Errorf("Expected TransactionStatusCleared, got %q", ts)
		}
	})

	t.Run("Scan from bytes", func(t *testing.T) {
		var ts TransactionStatus
		err := ts.Scan([]byte("reconciled"))
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ts != TransactionStatusReconciled {
			t.Errorf("Expected TransactionStatusReconciled, got %q", ts)
		}
	})

	t.Run("Scan from nil", func(t *testing.T) {
		var ts TransactionStatus
		err := ts.Scan(nil)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ts != "" {
			t.Errorf("Expected empty string, got %q", ts)
		}
	})

	t.Run("Scan from unsupported type returns error", func(t *testing.T) {
		var ts TransactionStatus
		err := ts.Scan(123)
		if err == nil {
			t.Error("Expected error for unsupported type")
		}
	})
}

func TestNewTransaction(t *testing.T) {
	t.Run("Creates transaction with required properties", func(t *testing.T) {
		accountID := NewID()
		date := NewDate(2024, time.January, 15)
		amount := MustNewMoney("-50.00")

		txn := NewTransaction(accountID, date, amount)

		if txn.ID.IsNil() {
			t.Error("NewTransaction should create non-nil ID")
		}
		if txn.AccountID != accountID {
			t.Errorf("Expected account ID %s, got %s", accountID.String(), txn.AccountID.String())
		}
		if !txn.Date.Equal(date) {
			t.Errorf("Expected date %s, got %s", date.String(), txn.Date.String())
		}
		if !txn.Amount.Equal(amount) {
			t.Errorf("Expected amount %s, got %s", amount.String(), txn.Amount.String())
		}
		if txn.Status != TransactionStatusPending {
			t.Errorf("Expected status 'pending', got %s", txn.Status.String())
		}
		if txn.PayeeID.Valid {
			t.Error("PayeeID should not be set")
		}
		if txn.CategoryID.Valid {
			t.Error("CategoryID should not be set")
		}
		if txn.Memo.Valid {
			t.Error("Memo should not be set")
		}
		if txn.CheckNumber.Valid {
			t.Error("CheckNumber should not be set")
		}
		if txn.TransferID.Valid {
			t.Error("TransferID should not be set")
		}
		if txn.TransferAccountID.Valid {
			t.Error("TransferAccountID should not be set")
		}
		if txn.CreatedAt.IsZero() {
			t.Error("CreatedAt should be set")
		}
		if txn.UpdatedAt.IsZero() {
			t.Error("UpdatedAt should be set")
		}
	})
}

func TestNewTransactionWithPayee(t *testing.T) {
	t.Run("Creates transaction with payee", func(t *testing.T) {
		accountID := NewID()
		payeeID := NewID()
		date := NewDate(2024, time.January, 15)
		amount := MustNewMoney("-50.00")

		txn := NewTransactionWithPayee(accountID, date, amount, payeeID)

		if !txn.PayeeID.Valid {
			t.Error("PayeeID should be set")
		}
		if txn.PayeeID.ID != payeeID {
			t.Errorf("Expected payee ID %s, got %s", payeeID.String(), txn.PayeeID.ID.String())
		}
	})
}

func TestNewTransactionFull(t *testing.T) {
	t.Run("Creates transaction with all properties", func(t *testing.T) {
		accountID := NewID()
		payeeID := NewID()
		categoryID := NewID()
		date := NewDate(2024, time.January, 15)
		amount := MustNewMoney("-50.00")
		memo := "Groceries"

		txn := NewTransactionFull(accountID, date, amount, payeeID, categoryID, memo)

		if txn.AccountID != accountID {
			t.Errorf("Expected account ID %s, got %s", accountID.String(), txn.AccountID.String())
		}
		if !txn.PayeeID.Valid || txn.PayeeID.ID != payeeID {
			t.Error("PayeeID should be set correctly")
		}
		if !txn.CategoryID.Valid || txn.CategoryID.ID != categoryID {
			t.Error("CategoryID should be set correctly")
		}
		if !txn.Memo.Valid || txn.Memo.String != memo {
			t.Errorf("Expected memo %q, got %q", memo, txn.Memo.String)
		}
	})

	t.Run("Handles nil payee and category", func(t *testing.T) {
		accountID := NewID()
		date := NewDate(2024, time.January, 15)
		amount := MustNewMoney("-50.00")

		txn := NewTransactionFull(accountID, date, amount, NilID, NilID, "")

		if txn.PayeeID.Valid {
			t.Error("PayeeID should not be set for NilID")
		}
		if txn.CategoryID.Valid {
			t.Error("CategoryID should not be set for NilID")
		}
		if txn.Memo.Valid {
			t.Error("Memo should not be set for empty string")
		}
	})
}

func TestTransactionPayee(t *testing.T) {
	t.Run("SetPayee sets payee", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
		payeeID := NewID()

		if txn.HasPayee() {
			t.Error("Transaction should start without payee")
		}

		txn.SetPayee(payeeID)

		if !txn.HasPayee() {
			t.Error("HasPayee should return true after setting")
		}
		if txn.PayeeID.ID != payeeID {
			t.Errorf("Expected payee ID %s, got %s", payeeID.String(), txn.PayeeID.ID.String())
		}
	})

	t.Run("ClearPayee removes payee", func(t *testing.T) {
		txn := NewTransactionWithPayee(NewID(), Today(), MustNewMoney("-50.00"), NewID())

		txn.ClearPayee()

		if txn.HasPayee() {
			t.Error("HasPayee should return false after clearing")
		}
	})
}

func TestTransactionCategory(t *testing.T) {
	t.Run("SetCategory sets category", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
		categoryID := NewID()

		if txn.HasCategory() {
			t.Error("Transaction should start without category")
		}

		txn.SetCategory(categoryID)

		if !txn.HasCategory() {
			t.Error("HasCategory should return true after setting")
		}
		if txn.CategoryID.ID != categoryID {
			t.Errorf("Expected category ID %s, got %s", categoryID.String(), txn.CategoryID.ID.String())
		}
	})

	t.Run("ClearCategory removes category", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
		txn.SetCategory(NewID())

		txn.ClearCategory()

		if txn.HasCategory() {
			t.Error("HasCategory should return false after clearing")
		}
	})
}

func TestTransactionMemo(t *testing.T) {
	t.Run("SetMemo sets memo", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))

		txn.SetMemo("Groceries")

		if !txn.Memo.Valid {
			t.Error("Memo should be valid after setting")
		}
		if txn.Memo.String != "Groceries" {
			t.Errorf("Expected memo 'Groceries', got %q", txn.Memo.String)
		}
	})

	t.Run("SetMemo with empty string clears memo", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
		txn.SetMemo("Some memo")

		txn.SetMemo("")

		if txn.Memo.Valid {
			t.Error("Memo should not be valid after setting empty string")
		}
	})

	t.Run("ClearMemo removes memo", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
		txn.SetMemo("Some memo")

		txn.ClearMemo()

		if txn.Memo.Valid {
			t.Error("Memo should not be valid after clearing")
		}
	})
}

func TestTransactionCheckNumber(t *testing.T) {
	t.Run("SetCheckNumber sets check number", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))

		txn.SetCheckNumber("1001")

		if !txn.CheckNumber.Valid {
			t.Error("CheckNumber should be valid after setting")
		}
		if txn.CheckNumber.String != "1001" {
			t.Errorf("Expected check number '1001', got %q", txn.CheckNumber.String)
		}
	})

	t.Run("SetCheckNumber with empty string clears check number", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
		txn.SetCheckNumber("1001")

		txn.SetCheckNumber("")

		if txn.CheckNumber.Valid {
			t.Error("CheckNumber should not be valid after setting empty string")
		}
	})

	t.Run("ClearCheckNumber removes check number", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
		txn.SetCheckNumber("1001")

		txn.ClearCheckNumber()

		if txn.CheckNumber.Valid {
			t.Error("CheckNumber should not be valid after clearing")
		}
	})
}

func TestTransactionStatus_Operations(t *testing.T) {
	t.Run("SetStatus sets status", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))

		txn.SetStatus(TransactionStatusCleared)

		if txn.Status != TransactionStatusCleared {
			t.Errorf("Expected status 'cleared', got %s", txn.Status.String())
		}
	})

	t.Run("Clear marks transaction as cleared", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))

		txn.Clear()

		if txn.Status != TransactionStatusCleared {
			t.Errorf("Expected status 'cleared', got %s", txn.Status.String())
		}
	})

	t.Run("Reconcile marks transaction as reconciled", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))

		txn.Reconcile()

		if txn.Status != TransactionStatusReconciled {
			t.Errorf("Expected status 'reconciled', got %s", txn.Status.String())
		}
	})

	t.Run("MarkPending marks transaction as pending", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
		txn.Clear()

		txn.MarkPending()

		if txn.Status != TransactionStatusPending {
			t.Errorf("Expected status 'pending', got %s", txn.Status.String())
		}
	})
}

func TestTransactionTransfer(t *testing.T) {
	t.Run("IsTransfer returns false for non-transfer", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
		if txn.IsTransfer() {
			t.Error("IsTransfer should return false for regular transaction")
		}
	})

	t.Run("SetTransfer links transaction to transfer", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
		transferID := NewID()
		transferAccountID := NewID()

		txn.SetTransfer(transferID, transferAccountID)

		if !txn.IsTransfer() {
			t.Error("IsTransfer should return true after SetTransfer")
		}
		if txn.TransferID.ID != transferID {
			t.Errorf("Expected transfer ID %s, got %s", transferID.String(), txn.TransferID.ID.String())
		}
		if txn.TransferAccountID.ID != transferAccountID {
			t.Errorf("Expected transfer account ID %s, got %s", transferAccountID.String(), txn.TransferAccountID.ID.String())
		}
	})

	t.Run("ClearTransfer removes transfer link", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
		txn.SetTransfer(NewID(), NewID())

		txn.ClearTransfer()

		if txn.IsTransfer() {
			t.Error("IsTransfer should return false after ClearTransfer")
		}
		if txn.TransferID.Valid {
			t.Error("TransferID should not be valid after clearing")
		}
		if txn.TransferAccountID.Valid {
			t.Error("TransferAccountID should not be valid after clearing")
		}
	})
}

func TestTransactionIncomeExpense(t *testing.T) {
	t.Run("IsIncome returns true for positive amount", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("100.00"))
		if !txn.IsIncome() {
			t.Error("IsIncome should return true for positive amount")
		}
		if txn.IsExpense() {
			t.Error("IsExpense should return false for positive amount")
		}
	})

	t.Run("IsExpense returns true for negative amount", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
		if !txn.IsExpense() {
			t.Error("IsExpense should return true for negative amount")
		}
		if txn.IsIncome() {
			t.Error("IsIncome should return false for negative amount")
		}
	})

	t.Run("Zero amount is neither income nor expense", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), ZeroMoney)
		if txn.IsIncome() {
			t.Error("IsIncome should return false for zero amount")
		}
		if txn.IsExpense() {
			t.Error("IsExpense should return false for zero amount")
		}
	})
}

func TestTransactionValidation(t *testing.T) {
	validTransaction := func() *Transaction {
		return NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
	}

	t.Run("Valid transaction passes validation", func(t *testing.T) {
		txn := validTransaction()
		errs := txn.Validate()
		if errs.HasErrors() {
			t.Errorf("Valid transaction should pass validation: %v", errs)
		}
	})

	t.Run("IsValid returns true for valid transaction", func(t *testing.T) {
		txn := validTransaction()
		if !txn.IsValid() {
			t.Error("IsValid should return true for valid transaction")
		}
	})

	t.Run("Nil account ID fails validation", func(t *testing.T) {
		txn := validTransaction()
		txn.AccountID = NilID
		errs := txn.Validate()
		if !errs.HasErrors() {
			t.Error("Nil account ID should fail validation")
		}
	})

	t.Run("Zero date fails validation", func(t *testing.T) {
		txn := validTransaction()
		txn.Date = ZeroDate
		errs := txn.Validate()
		if !errs.HasErrors() {
			t.Error("Zero date should fail validation")
		}
	})

	t.Run("Zero amount fails validation", func(t *testing.T) {
		txn := validTransaction()
		txn.Amount = ZeroMoney
		errs := txn.Validate()
		if !errs.HasErrors() {
			t.Error("Zero amount should fail validation")
		}
	})

	t.Run("Invalid status fails validation", func(t *testing.T) {
		txn := validTransaction()
		txn.Status = TransactionStatus("invalid")
		errs := txn.Validate()
		if !errs.HasErrors() {
			t.Error("Invalid status should fail validation")
		}
	})

	t.Run("Memo exceeding max length fails validation", func(t *testing.T) {
		txn := validTransaction()
		txn.Memo = NullableString{String: string(make([]byte, 1001)), Valid: true}
		errs := txn.Validate()
		if !errs.HasErrors() {
			t.Error("Memo exceeding 1000 chars should fail validation")
		}
	})

	t.Run("Check number exceeding max length fails validation", func(t *testing.T) {
		txn := validTransaction()
		txn.CheckNumber = NullableString{String: string(make([]byte, 51)), Valid: true}
		errs := txn.Validate()
		if !errs.HasErrors() {
			t.Error("Check number exceeding 50 chars should fail validation")
		}
	})

	t.Run("TransferID without TransferAccountID fails validation", func(t *testing.T) {
		txn := validTransaction()
		txn.TransferID = NullableID{ID: NewID(), Valid: true}
		errs := txn.Validate()
		if !errs.HasErrors() {
			t.Error("TransferID without TransferAccountID should fail validation")
		}
	})

	t.Run("TransferAccountID without TransferID fails validation", func(t *testing.T) {
		txn := validTransaction()
		txn.TransferAccountID = NullableID{ID: NewID(), Valid: true}
		errs := txn.Validate()
		if !errs.HasErrors() {
			t.Error("TransferAccountID without TransferID should fail validation")
		}
	})

	t.Run("Valid transfer passes validation", func(t *testing.T) {
		txn := validTransaction()
		txn.SetTransfer(NewID(), NewID())
		errs := txn.Validate()
		if errs.HasErrors() {
			t.Errorf("Valid transfer should pass validation: %v", errs)
		}
	})

	t.Run("All statuses pass validation", func(t *testing.T) {
		for _, status := range AllTransactionStatuses() {
			txn := validTransaction()
			txn.Status = status
			errs := txn.Validate()
			if errs.HasErrors() {
				t.Errorf("Status %q should pass validation: %v", status, errs)
			}
		}
	})

	t.Run("Multiple validation errors collected", func(t *testing.T) {
		txn := &Transaction{
			AccountID: NilID,
			Date:      ZeroDate,
			Amount:    ZeroMoney,
			Status:    TransactionStatus("bad"),
		}
		errs := txn.Validate()
		if len(errs) < 4 {
			t.Errorf("Expected at least 4 errors, got %d: %v", len(errs), errs)
		}
	})
}

func TestNewSplit(t *testing.T) {
	t.Run("Creates split with required properties", func(t *testing.T) {
		transactionID := NewID()
		categoryID := NewID()
		amount := MustNewMoney("-50.00")

		split := NewSplit(transactionID, categoryID, amount)

		if split.ID.IsNil() {
			t.Error("NewSplit should create non-nil ID")
		}
		if split.TransactionID != transactionID {
			t.Errorf("Expected transaction ID %s, got %s", transactionID.String(), split.TransactionID.String())
		}
		if split.CategoryID != categoryID {
			t.Errorf("Expected category ID %s, got %s", categoryID.String(), split.CategoryID.String())
		}
		if !split.Amount.Equal(amount) {
			t.Errorf("Expected amount %s, got %s", amount.String(), split.Amount.String())
		}
		if split.Memo.Valid {
			t.Error("Memo should not be set")
		}
		if split.CreatedAt.IsZero() {
			t.Error("CreatedAt should be set")
		}
		if split.UpdatedAt.IsZero() {
			t.Error("UpdatedAt should be set")
		}
	})
}

func TestNewSplitWithMemo(t *testing.T) {
	t.Run("Creates split with memo", func(t *testing.T) {
		transactionID := NewID()
		categoryID := NewID()
		amount := MustNewMoney("-30.00")
		memo := "Groceries portion"

		split := NewSplitWithMemo(transactionID, categoryID, amount, memo)

		if !split.Memo.Valid {
			t.Error("Memo should be set")
		}
		if split.Memo.String != memo {
			t.Errorf("Expected memo %q, got %q", memo, split.Memo.String)
		}
	})

	t.Run("Handles empty memo", func(t *testing.T) {
		split := NewSplitWithMemo(NewID(), NewID(), MustNewMoney("-30.00"), "")
		if split.Memo.Valid {
			t.Error("Memo should not be set for empty string")
		}
	})
}

func TestSplitMemo(t *testing.T) {
	t.Run("SetMemo sets memo", func(t *testing.T) {
		split := NewSplit(NewID(), NewID(), MustNewMoney("-30.00"))

		split.SetMemo("Test memo")

		if !split.Memo.Valid {
			t.Error("Memo should be valid after setting")
		}
		if split.Memo.String != "Test memo" {
			t.Errorf("Expected memo 'Test memo', got %q", split.Memo.String)
		}
	})

	t.Run("SetMemo with empty string clears memo", func(t *testing.T) {
		split := NewSplitWithMemo(NewID(), NewID(), MustNewMoney("-30.00"), "Some memo")

		split.SetMemo("")

		if split.Memo.Valid {
			t.Error("Memo should not be valid after setting empty string")
		}
	})

	t.Run("ClearMemo removes memo", func(t *testing.T) {
		split := NewSplitWithMemo(NewID(), NewID(), MustNewMoney("-30.00"), "Some memo")

		split.ClearMemo()

		if split.Memo.Valid {
			t.Error("Memo should not be valid after clearing")
		}
	})
}

func TestSplitValidation(t *testing.T) {
	validSplit := func() *Split {
		return NewSplit(NewID(), NewID(), MustNewMoney("-30.00"))
	}

	t.Run("Valid split passes validation", func(t *testing.T) {
		split := validSplit()
		errs := split.Validate()
		if errs.HasErrors() {
			t.Errorf("Valid split should pass validation: %v", errs)
		}
	})

	t.Run("IsValid returns true for valid split", func(t *testing.T) {
		split := validSplit()
		if !split.IsValid() {
			t.Error("IsValid should return true for valid split")
		}
	})

	t.Run("Nil transaction ID fails validation", func(t *testing.T) {
		split := validSplit()
		split.TransactionID = NilID
		errs := split.Validate()
		if !errs.HasErrors() {
			t.Error("Nil transaction ID should fail validation")
		}
	})

	t.Run("Nil category ID fails validation", func(t *testing.T) {
		split := validSplit()
		split.CategoryID = NilID
		errs := split.Validate()
		if !errs.HasErrors() {
			t.Error("Nil category ID should fail validation")
		}
	})

	t.Run("Zero amount fails validation", func(t *testing.T) {
		split := validSplit()
		split.Amount = ZeroMoney
		errs := split.Validate()
		if !errs.HasErrors() {
			t.Error("Zero amount should fail validation")
		}
	})

	t.Run("Memo exceeding max length fails validation", func(t *testing.T) {
		split := validSplit()
		split.Memo = NullableString{String: string(make([]byte, 501)), Valid: true}
		errs := split.Validate()
		if !errs.HasErrors() {
			t.Error("Memo exceeding 500 chars should fail validation")
		}
	})

	t.Run("Multiple validation errors collected", func(t *testing.T) {
		split := &Split{
			TransactionID: NilID,
			CategoryID:    NilID,
			Amount:        ZeroMoney,
		}
		errs := split.Validate()
		if len(errs) < 3 {
			t.Errorf("Expected at least 3 errors, got %d: %v", len(errs), errs)
		}
	})
}

func TestSplitCollection(t *testing.T) {
	t.Run("Total returns sum of split amounts", func(t *testing.T) {
		transactionID := NewID()
		splits := SplitCollection{
			NewSplit(transactionID, NewID(), MustNewMoney("-120.00")),
			NewSplit(transactionID, NewID(), MustNewMoney("-30.00")),
		}

		total := splits.Total()

		expected := MustNewMoney("-150.00")
		if !total.Equal(expected) {
			t.Errorf("Expected total %s, got %s", expected.String(), total.String())
		}
	})

	t.Run("Total returns zero for empty collection", func(t *testing.T) {
		splits := SplitCollection{}
		total := splits.Total()
		if !total.Equal(ZeroMoney) {
			t.Errorf("Expected zero total, got %s", total.String())
		}
	})

	t.Run("ValidateAgainstTransaction passes when totals match", func(t *testing.T) {
		transactionID := NewID()
		transactionAmount := MustNewMoney("-150.00")
		splits := SplitCollection{
			NewSplit(transactionID, NewID(), MustNewMoney("-120.00")),
			NewSplit(transactionID, NewID(), MustNewMoney("-30.00")),
		}

		errs := splits.ValidateAgainstTransaction(transactionAmount)

		if errs.HasErrors() {
			t.Errorf("ValidateAgainstTransaction should pass when totals match: %v", errs)
		}
	})

	t.Run("ValidateAgainstTransaction fails when totals don't match", func(t *testing.T) {
		transactionID := NewID()
		transactionAmount := MustNewMoney("-150.00")
		splits := SplitCollection{
			NewSplit(transactionID, NewID(), MustNewMoney("-100.00")),
			NewSplit(transactionID, NewID(), MustNewMoney("-30.00")),
		}

		errs := splits.ValidateAgainstTransaction(transactionAmount)

		if !errs.HasErrors() {
			t.Error("ValidateAgainstTransaction should fail when totals don't match")
		}
	})

	t.Run("ValidateAgainstTransaction passes for empty collection", func(t *testing.T) {
		splits := SplitCollection{}
		errs := splits.ValidateAgainstTransaction(MustNewMoney("-150.00"))
		if errs.HasErrors() {
			t.Errorf("ValidateAgainstTransaction should pass for empty collection: %v", errs)
		}
	})
}

func TestNewTransferPair(t *testing.T) {
	t.Run("Creates valid transfer pair", func(t *testing.T) {
		fromAccountID := NewID()
		toAccountID := NewID()
		date := NewDate(2024, time.January, 15)
		amount := MustNewMoney("500.00")

		pair := NewTransferPair(fromAccountID, toAccountID, date, amount)

		// From transaction
		if pair.FromTransaction.AccountID != fromAccountID {
			t.Error("From transaction should be in fromAccount")
		}
		if !pair.FromTransaction.Amount.IsNegative() {
			t.Error("From transaction amount should be negative")
		}
		if !pair.FromTransaction.IsTransfer() {
			t.Error("From transaction should be a transfer")
		}
		if pair.FromTransaction.TransferAccountID.ID != toAccountID {
			t.Error("From transaction transfer_account_id should be toAccountID")
		}

		// To transaction
		if pair.ToTransaction.AccountID != toAccountID {
			t.Error("To transaction should be in toAccount")
		}
		if !pair.ToTransaction.Amount.IsPositive() {
			t.Error("To transaction amount should be positive")
		}
		if !pair.ToTransaction.IsTransfer() {
			t.Error("To transaction should be a transfer")
		}
		if pair.ToTransaction.TransferAccountID.ID != fromAccountID {
			t.Error("To transaction transfer_account_id should be fromAccountID")
		}

		// Amounts should be equal and opposite
		if !pair.FromTransaction.Amount.Add(pair.ToTransaction.Amount).IsZero() {
			t.Error("Amounts should be equal and opposite")
		}

		// Transfer IDs should match
		if pair.FromTransaction.TransferID != pair.ToTransaction.TransferID {
			t.Error("Transfer IDs should match")
		}
	})
}

func TestTransferPairValidation(t *testing.T) {
	validTransferPair := func() *TransferPair {
		return NewTransferPair(NewID(), NewID(), Today(), MustNewMoney("500.00"))
	}

	t.Run("Valid transfer pair passes validation", func(t *testing.T) {
		pair := validTransferPair()
		errs := pair.Validate()
		if errs.HasErrors() {
			t.Errorf("Valid transfer pair should pass validation: %v", errs)
		}
	})

	t.Run("IsValid returns true for valid transfer pair", func(t *testing.T) {
		pair := validTransferPair()
		if !pair.IsValid() {
			t.Error("IsValid should return true for valid transfer pair")
		}
	})

	t.Run("Invalid from transaction fails validation", func(t *testing.T) {
		pair := validTransferPair()
		pair.FromTransaction.AccountID = NilID
		errs := pair.Validate()
		if !errs.HasErrors() {
			t.Error("Invalid from transaction should fail validation")
		}
	})

	t.Run("Invalid to transaction fails validation", func(t *testing.T) {
		pair := validTransferPair()
		pair.ToTransaction.AccountID = NilID
		errs := pair.Validate()
		if !errs.HasErrors() {
			t.Error("Invalid to transaction should fail validation")
		}
	})

	t.Run("Non-transfer from transaction fails validation", func(t *testing.T) {
		pair := validTransferPair()
		pair.FromTransaction.ClearTransfer()
		errs := pair.Validate()
		if !errs.HasErrors() {
			t.Error("Non-transfer from transaction should fail validation")
		}
	})

	t.Run("Mismatched amounts fails validation", func(t *testing.T) {
		pair := validTransferPair()
		pair.ToTransaction.Amount = MustNewMoney("600.00")
		errs := pair.Validate()
		if !errs.HasErrors() {
			t.Error("Mismatched amounts should fail validation")
		}
	})

	t.Run("Mismatched transfer IDs fails validation", func(t *testing.T) {
		pair := validTransferPair()
		pair.ToTransaction.TransferID = NullableID{ID: NewID(), Valid: true}
		errs := pair.Validate()
		if !errs.HasErrors() {
			t.Error("Mismatched transfer IDs should fail validation")
		}
	})

	t.Run("Mismatched transfer account IDs fails validation", func(t *testing.T) {
		pair := validTransferPair()
		pair.FromTransaction.TransferAccountID = NullableID{ID: NewID(), Valid: true}
		errs := pair.Validate()
		if !errs.HasErrors() {
			t.Error("Mismatched transfer account IDs should fail validation")
		}
	})
}

func TestTransactionUpdatesTimestamp(t *testing.T) {
	t.Run("SetPayee updates timestamp", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
		original := txn.UpdatedAt

		txn.SetPayee(NewID())

		if !txn.UpdatedAt.After(original) && !txn.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("SetPayee should update UpdatedAt")
		}
	})

	t.Run("SetCategory updates timestamp", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
		original := txn.UpdatedAt

		txn.SetCategory(NewID())

		if !txn.UpdatedAt.After(original) && !txn.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("SetCategory should update UpdatedAt")
		}
	})

	t.Run("SetMemo updates timestamp", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
		original := txn.UpdatedAt

		txn.SetMemo("Test")

		if !txn.UpdatedAt.After(original) && !txn.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("SetMemo should update UpdatedAt")
		}
	})

	t.Run("SetStatus updates timestamp", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
		original := txn.UpdatedAt

		txn.SetStatus(TransactionStatusCleared)

		if !txn.UpdatedAt.After(original) && !txn.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("SetStatus should update UpdatedAt")
		}
	})

	t.Run("SetTransfer updates timestamp", func(t *testing.T) {
		txn := NewTransaction(NewID(), Today(), MustNewMoney("-50.00"))
		original := txn.UpdatedAt

		txn.SetTransfer(NewID(), NewID())

		if !txn.UpdatedAt.After(original) && !txn.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("SetTransfer should update UpdatedAt")
		}
	})
}

func TestSplitUpdatesTimestamp(t *testing.T) {
	t.Run("SetMemo updates timestamp", func(t *testing.T) {
		split := NewSplit(NewID(), NewID(), MustNewMoney("-30.00"))
		original := split.UpdatedAt

		split.SetMemo("Test")

		if !split.UpdatedAt.After(original) && !split.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("SetMemo should update UpdatedAt")
		}
	})
}
