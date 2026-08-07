package investment

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/types"
)

// ActionType represents the type of corporate action.
type ActionType string

const (
	ActionTypeSplit        ActionType = "split"
	ActionTypeReverseSplit ActionType = "reverse_split"
	ActionTypeMerger       ActionType = "merger"
	ActionTypeSpinOff      ActionType = "spin_off"
)

// AllActionTypes returns all valid corporate action types.
func AllActionTypes() []ActionType {
	return []ActionType{
		ActionTypeSplit,
		ActionTypeReverseSplit,
		ActionTypeMerger,
		ActionTypeSpinOff,
	}
}

// String returns the string representation of the ActionType.
func (at ActionType) String() string {
	return string(at)
}

// IsValid returns true if the ActionType is a valid type.
func (at ActionType) IsValid() bool {
	switch at {
	case ActionTypeSplit, ActionTypeReverseSplit, ActionTypeMerger, ActionTypeSpinOff:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the corporate action type.
func (at ActionType) DisplayName() string {
	switch at {
	case ActionTypeSplit:
		return "Stock Split"
	case ActionTypeReverseSplit:
		return "Reverse Split"
	case ActionTypeMerger:
		return "Merger"
	case ActionTypeSpinOff:
		return "Spin-Off"
	default:
		return string(at)
	}
}

// RequiresTargetSecurity returns true if this action type needs a target security.
func (at ActionType) RequiresTargetSecurity() bool {
	switch at {
	case ActionTypeMerger, ActionTypeSpinOff:
		return true
	}
	return false
}

// ParseActionType parses a string into an ActionType.
func ParseActionType(s string) (ActionType, error) {
	at := ActionType(strings.ToLower(s))
	if !at.IsValid() {
		return "", fmt.Errorf("invalid corporate action type: %q", s)
	}
	return at, nil
}

// Value implements the driver.Valuer interface for database storage.
func (at ActionType) Value() (driver.Value, error) {
	return string(at), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (at *ActionType) Scan(value any) error {
	if value == nil {
		*at = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*at = ActionType(v)
	case []byte:
		*at = ActionType(string(v))
	default:
		return fmt.Errorf("unsupported type for ActionType: %T", value)
	}
	return nil
}

// CorporateAction represents a corporate action record.
type CorporateAction struct {
	ID               types.ID         `json:"id"`
	ActionType       ActionType       `json:"action_type"`
	SecurityID       types.ID         `json:"security_id"`
	TargetSecurityID types.NullableID `json:"target_security_id"`
	ActionDate       types.Date       `json:"action_date"`
	Parameters       string           `json:"parameters"`
	CreatedAt        types.Timestamp  `json:"created_at"`
}

// NewCorporateAction creates a new CorporateAction with required fields.
func NewCorporateAction(actionType ActionType, securityID types.ID, actionDate types.Date, parameters string) *CorporateAction {
	return &CorporateAction{
		ID:         types.NewID(),
		ActionType: actionType,
		SecurityID: securityID,
		ActionDate: actionDate,
		Parameters: parameters,
		CreatedAt:  types.Now(),
	}
}

// SetTargetSecurity sets the target security for merger/spin-off actions.
func (ca *CorporateAction) SetTargetSecurity(targetID types.ID) {
	ca.TargetSecurityID = types.NullableID{ID: targetID, Valid: true}
}

// Validate validates the corporate action and returns any validation errors.
func (ca *CorporateAction) Validate() types.ValidationErrors {
	v := types.NewValidator()

	v.RequiredID("security_id", ca.SecurityID)
	v.RequiredDate("action_date", ca.ActionDate)
	v.RequiredString("parameters", ca.Parameters)

	if !ca.ActionType.IsValid() {
		v.AddError("action_type", "must be a valid corporate action type")
	}

	// target_security_id required for merger and spin_off
	if ca.ActionType.RequiresTargetSecurity() && !ca.TargetSecurityID.Valid {
		v.AddError("target_security_id", "is required for "+ca.ActionType.String()+" actions")
	}

	return v.Errors()
}

// IsValid returns true if the corporate action passes validation.
func (ca *CorporateAction) IsValid() bool {
	return !ca.Validate().HasErrors()
}

// SplitParams holds the parameters for a stock split or reverse split.
type SplitParams struct {
	Numerator   int `json:"numerator"`
	Denominator int `json:"denominator"`
}

// Ratio returns the split ratio as a decimal (numerator / denominator).
func (sp SplitParams) Ratio() float64 {
	if sp.Denominator == 0 {
		return 0
	}
	return float64(sp.Numerator) / float64(sp.Denominator)
}

// RatioString returns the split ratio as "N:D" string.
func (sp SplitParams) RatioString() string {
	return fmt.Sprintf("%d:%d", sp.Numerator, sp.Denominator)
}

// Validate validates split parameters.
func (sp SplitParams) Validate() types.ValidationErrors {
	v := types.NewValidator()

	if sp.Numerator <= 0 {
		v.AddError("numerator", "must be positive")
	}
	if sp.Denominator <= 0 {
		v.AddError("denominator", "must be positive")
	}

	return v.Errors()
}

// ToJSON serializes SplitParams to a JSON string.
func (sp SplitParams) ToJSON() (string, error) {
	b, err := json.Marshal(sp)
	if err != nil {
		return "", fmt.Errorf("marshal split params: %w", err)
	}
	return string(b), nil
}

// ParseSplitParams deserializes SplitParams from a JSON string.
func ParseSplitParams(jsonStr string) (*SplitParams, error) {
	var sp SplitParams
	if err := json.Unmarshal([]byte(jsonStr), &sp); err != nil {
		return nil, fmt.Errorf("unmarshal split params: %w", err)
	}
	return &sp, nil
}

// ParseSplitRatio parses a ratio string like "4:1" into SplitParams.
func ParseSplitRatio(ratio string) (*SplitParams, error) {
	parts := strings.SplitN(ratio, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid split ratio format: %q (expected N:D)", ratio)
	}

	var sp SplitParams
	_, err := fmt.Sscanf(parts[0], "%d", &sp.Numerator)
	if err != nil {
		return nil, fmt.Errorf("invalid numerator in ratio: %q", parts[0])
	}
	_, err = fmt.Sscanf(parts[1], "%d", &sp.Denominator)
	if err != nil {
		return nil, fmt.Errorf("invalid denominator in ratio: %q", parts[1])
	}

	return &sp, nil
}

// MergerParams holds the parameters for a merger/acquisition.
type MergerParams struct {
	ExchangeRatio float64 `json:"exchange_ratio"`
	CashPerShare  float64 `json:"cash_per_share,omitempty"`
}

// Validate validates merger parameters.
func (mp MergerParams) Validate() types.ValidationErrors {
	v := types.NewValidator()

	if mp.ExchangeRatio <= 0 {
		v.AddError("exchange_ratio", "must be positive")
	}
	if mp.CashPerShare < 0 {
		v.AddError("cash_per_share", "must not be negative")
	}

	return v.Errors()
}

// HasCashConsideration returns true if there is a cash component.
func (mp MergerParams) HasCashConsideration() bool {
	return mp.CashPerShare > 0
}

// ToJSON serializes MergerParams to a JSON string.
func (mp MergerParams) ToJSON() (string, error) {
	b, err := json.Marshal(mp)
	if err != nil {
		return "", fmt.Errorf("marshal merger params: %w", err)
	}
	return string(b), nil
}

// ParseMergerParams deserializes MergerParams from a JSON string.
func ParseMergerParams(jsonStr string) (*MergerParams, error) {
	var mp MergerParams
	if err := json.Unmarshal([]byte(jsonStr), &mp); err != nil {
		return nil, fmt.Errorf("unmarshal merger params: %w", err)
	}
	return &mp, nil
}

// SpinOffParams holds the parameters for a spin-off.
type SpinOffParams struct {
	ShareRatio          float64 `json:"share_ratio"`
	ParentAllocationPct float64 `json:"parent_allocation_pct"`
}

// Validate validates spin-off parameters.
func (sp SpinOffParams) Validate() types.ValidationErrors {
	v := types.NewValidator()

	if sp.ShareRatio <= 0 {
		v.AddError("share_ratio", "must be positive")
	}
	if sp.ParentAllocationPct <= 0 || sp.ParentAllocationPct >= 100 {
		v.AddError("parent_allocation_pct", "must be between 0 and 100 (exclusive)")
	}

	return v.Errors()
}

// SpinOffAllocationPct returns the percentage allocated to the spin-off security.
func (sp SpinOffParams) SpinOffAllocationPct() float64 {
	return 100 - sp.ParentAllocationPct
}

// ToJSON serializes SpinOffParams to a JSON string.
func (sp SpinOffParams) ToJSON() (string, error) {
	b, err := json.Marshal(sp)
	if err != nil {
		return "", fmt.Errorf("marshal spin-off params: %w", err)
	}
	return string(b), nil
}

// ParseSpinOffParams deserializes SpinOffParams from a JSON string.
func ParseSpinOffParams(jsonStr string) (*SpinOffParams, error) {
	var sp SpinOffParams
	if err := json.Unmarshal([]byte(jsonStr), &sp); err != nil {
		return nil, fmt.Errorf("unmarshal spin-off params: %w", err)
	}
	return &sp, nil
}

// DownstreamEventsError is returned when a reversal is blocked by a
// later investment transaction on the affected security.
type DownstreamEventsError struct {
	ActionDate     types.Date
	BlockerTicker  string
	BlockerDate    types.Date
	BlockerTxnType string
}

func (e *DownstreamEventsError) Error() string {
	ticker := e.BlockerTicker
	if ticker == "" {
		ticker = "the affected security"
	}
	return fmt.Sprintf(
		"cannot reverse: %s has a %s transaction on %s (action dated %s). Remove or re-date later transactions first.",
		ticker, e.BlockerTxnType, e.BlockerDate.String(), e.ActionDate.String(),
	)
}

// UnsupportedReversalError is returned when the user tries to reverse
// a corporate-action type for which reversal is not yet implemented.
type UnsupportedReversalError struct {
	ActionType ActionType
}

func (e *UnsupportedReversalError) Error() string {
	return fmt.Sprintf(
		"reversing %s corporate actions is not yet supported — only splits and reverse splits can be undone in this version",
		e.ActionType.DisplayName(),
	)
}
