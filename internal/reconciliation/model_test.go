package reconciliation

import (
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

func TestSessionStatus(t *testing.T) {
	t.Run("String returns correct value", func(t *testing.T) {
		if SessionStatusInProgress.String() != "in_progress" {
			t.Errorf("Expected 'in_progress', got %q", SessionStatusInProgress.String())
		}
		if SessionStatusCompleted.String() != "completed" {
			t.Errorf("Expected 'completed', got %q", SessionStatusCompleted.String())
		}
	})

	t.Run("IsValid returns true for valid statuses", func(t *testing.T) {
		validStatuses := []SessionStatus{
			SessionStatusInProgress,
			SessionStatusCompleted,
		}
		for _, s := range validStatuses {
			if !s.IsValid() {
				t.Errorf("IsValid should return true for %q", s)
			}
		}
	})

	t.Run("IsValid returns false for invalid status", func(t *testing.T) {
		invalid := SessionStatus("unknown")
		if invalid.IsValid() {
			t.Error("IsValid should return false for 'unknown'")
		}
	})

	t.Run("DisplayName returns human-readable names", func(t *testing.T) {
		tests := []struct {
			status   SessionStatus
			expected string
		}{
			{SessionStatusInProgress, "In Progress"},
			{SessionStatusCompleted, "Completed"},
		}
		for _, tc := range tests {
			if tc.status.DisplayName() != tc.expected {
				t.Errorf("DisplayName for %q: expected %q, got %q",
					tc.status, tc.expected, tc.status.DisplayName())
			}
		}
	})
}

func TestParseSessionStatus(t *testing.T) {
	t.Run("Parses valid status", func(t *testing.T) {
		rs, err := ParseSessionStatus("in_progress")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if rs != SessionStatusInProgress {
			t.Errorf("Expected SessionStatusInProgress, got %q", rs)
		}
	})

	t.Run("Parses uppercase status", func(t *testing.T) {
		rs, err := ParseSessionStatus("COMPLETED")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if rs != SessionStatusCompleted {
			t.Errorf("Expected SessionStatusCompleted, got %q", rs)
		}
	})

	t.Run("Returns error for invalid status", func(t *testing.T) {
		_, err := ParseSessionStatus("invalid")
		if err == nil {
			t.Error("Expected error for invalid reconciliation session status")
		}
	})
}

func TestSessionStatusScanValue(t *testing.T) {
	t.Run("Value returns string", func(t *testing.T) {
		rs := SessionStatusInProgress
		v, err := rs.Value()
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if v != "in_progress" {
			t.Errorf("Expected 'in_progress', got %v", v)
		}
	})

	t.Run("Scan from string", func(t *testing.T) {
		var rs SessionStatus
		err := rs.Scan("completed")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if rs != SessionStatusCompleted {
			t.Errorf("Expected SessionStatusCompleted, got %q", rs)
		}
	})

	t.Run("Scan from bytes", func(t *testing.T) {
		var rs SessionStatus
		err := rs.Scan([]byte("in_progress"))
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if rs != SessionStatusInProgress {
			t.Errorf("Expected SessionStatusInProgress, got %q", rs)
		}
	})

	t.Run("Scan from nil", func(t *testing.T) {
		var rs SessionStatus
		err := rs.Scan(nil)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if rs != "" {
			t.Errorf("Expected empty string, got %q", rs)
		}
	})

	t.Run("Scan from unsupported type returns error", func(t *testing.T) {
		var rs SessionStatus
		err := rs.Scan(123)
		if err == nil {
			t.Error("Expected error for unsupported type")
		}
	})
}

func TestNewSession(t *testing.T) {
	t.Run("Creates session with required fields", func(t *testing.T) {
		accountID := types.NewID()
		date := types.NewDate(2024, 1, 31)
		balance := types.MustNewMoney("5234.56")

		session := NewSession(accountID, date, balance)

		if session.ID.IsNil() {
			t.Error("NewSession should create non-nil ID")
		}
		if session.AccountID != accountID {
			t.Errorf("Expected account ID %v, got %v", accountID, session.AccountID)
		}
		if !session.StatementDate.Equal(date) {
			t.Errorf("Expected statement date %v, got %v", date, session.StatementDate)
		}
		if !session.StatementBalance.Equal(balance) {
			t.Errorf("Expected statement balance %s, got %s", balance.String(), session.StatementBalance.String())
		}
		if session.Status != SessionStatusInProgress {
			t.Errorf("Expected status in_progress, got %q", session.Status)
		}
		if session.CompletedAt.Valid {
			t.Error("CompletedAt should not be set on new session")
		}
		if session.CreatedAt.IsZero() {
			t.Error("NewSession should set CreatedAt")
		}
		if session.UpdatedAt.IsZero() {
			t.Error("NewSession should set UpdatedAt")
		}
	})
}

func TestSessionComplete(t *testing.T) {
	t.Run("Complete marks session as completed", func(t *testing.T) {
		session := NewSession(types.NewID(), types.Today(), types.MustNewMoney("1000"))
		if !session.IsInProgress() {
			t.Error("New session should be in progress")
		}

		session.Complete()

		if !session.IsCompleted() {
			t.Error("Session should be completed after Complete()")
		}
		if session.IsInProgress() {
			t.Error("Session should not be in progress after Complete()")
		}
		if !session.CompletedAt.Valid {
			t.Error("CompletedAt should be set after Complete()")
		}
		if session.CompletedAt.Timestamp.IsZero() {
			t.Error("CompletedAt should have a non-zero timestamp")
		}
	})
}

func TestSessionValidation(t *testing.T) {
	validSession := func() *Session {
		return NewSession(
			types.NewID(),
			types.NewDate(2024, 1, 31),
			types.MustNewMoney("5234.56"),
		)
	}

	t.Run("Valid session passes validation", func(t *testing.T) {
		session := validSession()
		errs := session.Validate()
		if errs.HasErrors() {
			t.Errorf("Valid session should pass validation: %v", errs)
		}
	})

	t.Run("IsValid returns true for valid session", func(t *testing.T) {
		session := validSession()
		if !session.IsValid() {
			t.Error("IsValid should return true for valid session")
		}
	})

	t.Run("Nil account ID fails validation", func(t *testing.T) {
		session := validSession()
		session.AccountID = types.NilID
		errs := session.Validate()
		if !errs.HasErrors() {
			t.Error("Nil account ID should fail validation")
		}
	})

	t.Run("Zero statement date fails validation", func(t *testing.T) {
		session := validSession()
		session.StatementDate = types.ZeroDate
		errs := session.Validate()
		if !errs.HasErrors() {
			t.Error("Zero statement date should fail validation")
		}
	})

	t.Run("Invalid status fails validation", func(t *testing.T) {
		session := validSession()
		session.Status = SessionStatus("invalid")
		errs := session.Validate()
		if !errs.HasErrors() {
			t.Error("Invalid status should fail validation")
		}
	})

	t.Run("Zero statement balance passes validation", func(t *testing.T) {
		session := validSession()
		session.StatementBalance = types.ZeroMoney
		errs := session.Validate()
		if errs.HasErrors() {
			t.Errorf("Zero statement balance should pass validation: %v", errs)
		}
	})

	t.Run("Negative statement balance passes validation", func(t *testing.T) {
		session := validSession()
		session.StatementBalance = types.MustNewMoney("-500.00")
		errs := session.Validate()
		if errs.HasErrors() {
			t.Errorf("Negative statement balance should pass validation (e.g., credit cards): %v", errs)
		}
	})

	t.Run("Multiple validation errors collected", func(t *testing.T) {
		session := validSession()
		session.AccountID = types.NilID
		session.StatementDate = types.ZeroDate
		session.Status = SessionStatus("bad")
		errs := session.Validate()
		if len(errs) < 3 {
			t.Errorf("Expected at least 3 errors, got %d: %v", len(errs), errs)
		}
	})
}
