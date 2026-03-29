package reconciliation

import (
	"database/sql/driver"
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/types"
)

// SessionStatus represents the status of a reconciliation session.
type SessionStatus string

const (
	SessionStatusInProgress SessionStatus = "in_progress"
	SessionStatusCompleted  SessionStatus = "completed"
)

// String returns the string representation of the SessionStatus.
func (rs SessionStatus) String() string {
	return string(rs)
}

// IsValid returns true if the SessionStatus is a valid status.
func (rs SessionStatus) IsValid() bool {
	switch rs {
	case SessionStatusInProgress, SessionStatusCompleted:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the reconciliation session status.
func (rs SessionStatus) DisplayName() string {
	switch rs {
	case SessionStatusInProgress:
		return "In Progress"
	case SessionStatusCompleted:
		return "Completed"
	default:
		return string(rs)
	}
}

// ParseSessionStatus parses a string into a SessionStatus.
func ParseSessionStatus(s string) (SessionStatus, error) {
	rs := SessionStatus(strings.ToLower(s))
	if !rs.IsValid() {
		return "", fmt.Errorf("invalid reconciliation session status: %q", s)
	}
	return rs, nil
}

// Value implements the driver.Valuer interface for database storage.
func (rs SessionStatus) Value() (driver.Value, error) {
	return string(rs), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (rs *SessionStatus) Scan(value any) error {
	if value == nil {
		*rs = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*rs = SessionStatus(v)
	case []byte:
		*rs = SessionStatus(string(v))
	default:
		return fmt.Errorf("unsupported type for SessionStatus: %T", value)
	}
	return nil
}

// Session represents an active or completed reconciliation session for an account.
type Session struct {
	types.BaseModel

	// Core properties (required)
	AccountID        types.ID      `json:"account_id"`
	StatementDate    types.Date    `json:"statement_date"`
	StatementBalance types.Money   `json:"statement_balance"`
	Status           SessionStatus `json:"status"`

	// Optional properties
	CompletedAt types.NullableTimestamp `json:"completed_at"`
}

// NewSession creates a new Session with generated ID and timestamps.
func NewSession(accountID types.ID, statementDate types.Date, statementBalance types.Money) *Session {
	return &Session{
		BaseModel:        types.NewBaseModel(),
		AccountID:        accountID,
		StatementDate:    statementDate,
		StatementBalance: statementBalance,
		Status:           SessionStatusInProgress,
	}
}

// Complete marks the reconciliation session as completed.
func (rs *Session) Complete() {
	rs.Status = SessionStatusCompleted
	now := types.Now()
	rs.CompletedAt = types.NullableTimestamp{Timestamp: now, Valid: true}
	rs.Touch()
}

// IsInProgress returns true if the session is in progress.
func (rs *Session) IsInProgress() bool {
	return rs.Status == SessionStatusInProgress
}

// IsCompleted returns true if the session is completed.
func (rs *Session) IsCompleted() bool {
	return rs.Status == SessionStatusCompleted
}

// Validate validates the reconciliation session and returns any validation errors.
func (rs *Session) Validate() types.ValidationErrors {
	v := types.NewValidator()

	v.RequiredID("account_id", rs.AccountID)
	v.RequiredDate("statement_date", rs.StatementDate)

	if !rs.Status.IsValid() {
		v.AddError("status", "must be a valid reconciliation session status (in_progress or completed)")
	}

	return v.Errors()
}

// IsValid returns true if the reconciliation session passes validation.
func (rs *Session) IsValid() bool {
	return !rs.Validate().HasErrors()
}
