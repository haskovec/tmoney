package models

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

// ReconciliationSessionStatus represents the status of a reconciliation session.
type ReconciliationSessionStatus string

const (
	ReconciliationStatusInProgress ReconciliationSessionStatus = "in_progress"
	ReconciliationStatusCompleted  ReconciliationSessionStatus = "completed"
)

// String returns the string representation of the ReconciliationSessionStatus.
func (rs ReconciliationSessionStatus) String() string {
	return string(rs)
}

// IsValid returns true if the ReconciliationSessionStatus is a valid status.
func (rs ReconciliationSessionStatus) IsValid() bool {
	switch rs {
	case ReconciliationStatusInProgress, ReconciliationStatusCompleted:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the reconciliation session status.
func (rs ReconciliationSessionStatus) DisplayName() string {
	switch rs {
	case ReconciliationStatusInProgress:
		return "In Progress"
	case ReconciliationStatusCompleted:
		return "Completed"
	default:
		return string(rs)
	}
}

// ParseReconciliationSessionStatus parses a string into a ReconciliationSessionStatus.
func ParseReconciliationSessionStatus(s string) (ReconciliationSessionStatus, error) {
	rs := ReconciliationSessionStatus(strings.ToLower(s))
	if !rs.IsValid() {
		return "", fmt.Errorf("invalid reconciliation session status: %q", s)
	}
	return rs, nil
}

// Value implements the driver.Valuer interface for database storage.
func (rs ReconciliationSessionStatus) Value() (driver.Value, error) {
	return string(rs), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (rs *ReconciliationSessionStatus) Scan(value any) error {
	if value == nil {
		*rs = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*rs = ReconciliationSessionStatus(v)
	case []byte:
		*rs = ReconciliationSessionStatus(string(v))
	default:
		return fmt.Errorf("unsupported type for ReconciliationSessionStatus: %T", value)
	}
	return nil
}

// ReconciliationSession represents an active or completed reconciliation session for an account.
type ReconciliationSession struct {
	BaseModel

	// Core properties (required)
	AccountID        ID                          `json:"account_id"`
	StatementDate    Date                        `json:"statement_date"`
	StatementBalance Money                       `json:"statement_balance"`
	Status           ReconciliationSessionStatus `json:"status"`

	// Optional properties
	CompletedAt NullableTimestamp `json:"completed_at"`
}

// NewReconciliationSession creates a new ReconciliationSession with generated ID and timestamps.
func NewReconciliationSession(accountID ID, statementDate Date, statementBalance Money) *ReconciliationSession {
	return &ReconciliationSession{
		BaseModel:        NewBaseModel(),
		AccountID:        accountID,
		StatementDate:    statementDate,
		StatementBalance: statementBalance,
		Status:           ReconciliationStatusInProgress,
	}
}

// Complete marks the reconciliation session as completed.
func (rs *ReconciliationSession) Complete() {
	rs.Status = ReconciliationStatusCompleted
	now := Now()
	rs.CompletedAt = NullableTimestamp{Timestamp: now, Valid: true}
	rs.Touch()
}

// IsInProgress returns true if the session is in progress.
func (rs *ReconciliationSession) IsInProgress() bool {
	return rs.Status == ReconciliationStatusInProgress
}

// IsCompleted returns true if the session is completed.
func (rs *ReconciliationSession) IsCompleted() bool {
	return rs.Status == ReconciliationStatusCompleted
}

// Validate validates the reconciliation session and returns any validation errors.
func (rs *ReconciliationSession) Validate() ValidationErrors {
	v := NewValidator()

	v.RequiredID("account_id", rs.AccountID)
	v.RequiredDate("statement_date", rs.StatementDate)

	if !rs.Status.IsValid() {
		v.errors.Add("status", "must be a valid reconciliation session status (in_progress or completed)")
	}

	return v.Errors()
}

// IsValid returns true if the reconciliation session passes validation.
func (rs *ReconciliationSession) IsValid() bool {
	return !rs.Validate().HasErrors()
}
