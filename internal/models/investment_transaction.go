package models

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

// InvestmentTransactionType represents the type of investment transaction.
type InvestmentTransactionType string

const (
	InvestmentTransactionTypeBuy              InvestmentTransactionType = "buy"
	InvestmentTransactionTypeSell             InvestmentTransactionType = "sell"
	InvestmentTransactionTypeDividend         InvestmentTransactionType = "dividend"
	InvestmentTransactionTypeReinvestDividend InvestmentTransactionType = "reinvest_dividend"
	InvestmentTransactionTypeFee              InvestmentTransactionType = "fee"
	InvestmentTransactionTypeFeeLiquidation   InvestmentTransactionType = "fee_liquidation"
	InvestmentTransactionTypeDeposit          InvestmentTransactionType = "deposit"
	InvestmentTransactionTypeWithdrawal       InvestmentTransactionType = "withdrawal"
	InvestmentTransactionTypeInterest         InvestmentTransactionType = "interest"
	InvestmentTransactionTypeTransferShares   InvestmentTransactionType = "transfer_shares"
	InvestmentTransactionTypeTransferCash     InvestmentTransactionType = "transfer_cash"
	InvestmentTransactionTypeExchange         InvestmentTransactionType = "exchange"
)

// AllInvestmentTransactionTypes returns all valid investment transaction types.
func AllInvestmentTransactionTypes() []InvestmentTransactionType {
	return []InvestmentTransactionType{
		InvestmentTransactionTypeBuy,
		InvestmentTransactionTypeSell,
		InvestmentTransactionTypeDividend,
		InvestmentTransactionTypeReinvestDividend,
		InvestmentTransactionTypeFee,
		InvestmentTransactionTypeFeeLiquidation,
		InvestmentTransactionTypeDeposit,
		InvestmentTransactionTypeWithdrawal,
		InvestmentTransactionTypeInterest,
		InvestmentTransactionTypeTransferShares,
		InvestmentTransactionTypeTransferCash,
		InvestmentTransactionTypeExchange,
	}
}

// String returns the string representation of the InvestmentTransactionType.
func (itt InvestmentTransactionType) String() string {
	return string(itt)
}

// IsValid returns true if the InvestmentTransactionType is a valid type.
func (itt InvestmentTransactionType) IsValid() bool {
	switch itt {
	case InvestmentTransactionTypeBuy, InvestmentTransactionTypeSell,
		InvestmentTransactionTypeDividend, InvestmentTransactionTypeReinvestDividend,
		InvestmentTransactionTypeFee, InvestmentTransactionTypeFeeLiquidation,
		InvestmentTransactionTypeDeposit, InvestmentTransactionTypeWithdrawal,
		InvestmentTransactionTypeInterest, InvestmentTransactionTypeTransferShares,
		InvestmentTransactionTypeTransferCash, InvestmentTransactionTypeExchange:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the investment transaction type.
func (itt InvestmentTransactionType) DisplayName() string {
	switch itt {
	case InvestmentTransactionTypeBuy:
		return "Buy"
	case InvestmentTransactionTypeSell:
		return "Sell"
	case InvestmentTransactionTypeDividend:
		return "Dividend"
	case InvestmentTransactionTypeReinvestDividend:
		return "Reinvest Dividend"
	case InvestmentTransactionTypeFee:
		return "Fee"
	case InvestmentTransactionTypeFeeLiquidation:
		return "Fee via Liquidation"
	case InvestmentTransactionTypeDeposit:
		return "Deposit"
	case InvestmentTransactionTypeWithdrawal:
		return "Withdrawal"
	case InvestmentTransactionTypeInterest:
		return "Interest"
	case InvestmentTransactionTypeTransferShares:
		return "Transfer Shares"
	case InvestmentTransactionTypeTransferCash:
		return "Transfer Cash"
	case InvestmentTransactionTypeExchange:
		return "Exchange"
	default:
		return string(itt)
	}
}

// RequiresSecurity returns true if this transaction type must reference a security.
func (itt InvestmentTransactionType) RequiresSecurity() bool {
	switch itt {
	case InvestmentTransactionTypeBuy, InvestmentTransactionTypeSell,
		InvestmentTransactionTypeDividend, InvestmentTransactionTypeReinvestDividend,
		InvestmentTransactionTypeFeeLiquidation, InvestmentTransactionTypeTransferShares,
		InvestmentTransactionTypeExchange:
		return true
	}
	return false
}

// RequiresShares returns true if this transaction type must have a shares value.
func (itt InvestmentTransactionType) RequiresShares() bool {
	switch itt {
	case InvestmentTransactionTypeBuy, InvestmentTransactionTypeSell,
		InvestmentTransactionTypeReinvestDividend, InvestmentTransactionTypeFeeLiquidation,
		InvestmentTransactionTypeTransferShares, InvestmentTransactionTypeExchange:
		return true
	}
	return false
}

// AffectsCash returns true if this transaction type affects the account cash position.
func (itt InvestmentTransactionType) AffectsCash() bool {
	switch itt {
	case InvestmentTransactionTypeBuy, InvestmentTransactionTypeSell,
		InvestmentTransactionTypeDividend, InvestmentTransactionTypeFee,
		InvestmentTransactionTypeDeposit, InvestmentTransactionTypeWithdrawal,
		InvestmentTransactionTypeInterest, InvestmentTransactionTypeTransferCash:
		return true
	}
	return false
}

// ParseInvestmentTransactionType parses a string into an InvestmentTransactionType.
func ParseInvestmentTransactionType(s string) (InvestmentTransactionType, error) {
	itt := InvestmentTransactionType(strings.ToLower(s))
	if !itt.IsValid() {
		return "", fmt.Errorf("invalid investment transaction type: %q", s)
	}
	return itt, nil
}

// Value implements the driver.Valuer interface for database storage.
func (itt InvestmentTransactionType) Value() (driver.Value, error) {
	return string(itt), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (itt *InvestmentTransactionType) Scan(value any) error {
	if value == nil {
		*itt = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*itt = InvestmentTransactionType(v)
	case []byte:
		*itt = InvestmentTransactionType(string(v))
	default:
		return fmt.Errorf("unsupported type for InvestmentTransactionType: %T", value)
	}
	return nil
}

// InvestmentTransactionStatus represents the status of an investment transaction.
type InvestmentTransactionStatus string

const (
	InvestmentTransactionStatusPending    InvestmentTransactionStatus = "pending"
	InvestmentTransactionStatusCleared    InvestmentTransactionStatus = "cleared"
	InvestmentTransactionStatusReconciled InvestmentTransactionStatus = "reconciled"
)

// AllInvestmentTransactionStatuses returns all valid investment transaction statuses.
func AllInvestmentTransactionStatuses() []InvestmentTransactionStatus {
	return []InvestmentTransactionStatus{
		InvestmentTransactionStatusPending,
		InvestmentTransactionStatusCleared,
		InvestmentTransactionStatusReconciled,
	}
}

// String returns the string representation of the InvestmentTransactionStatus.
func (s InvestmentTransactionStatus) String() string {
	return string(s)
}

// IsValid returns true if the InvestmentTransactionStatus is a valid status.
func (s InvestmentTransactionStatus) IsValid() bool {
	switch s {
	case InvestmentTransactionStatusPending, InvestmentTransactionStatusCleared, InvestmentTransactionStatusReconciled:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the investment transaction status.
func (s InvestmentTransactionStatus) DisplayName() string {
	switch s {
	case InvestmentTransactionStatusPending:
		return "Pending"
	case InvestmentTransactionStatusCleared:
		return "Cleared"
	case InvestmentTransactionStatusReconciled:
		return "Reconciled"
	default:
		return string(s)
	}
}

// ParseInvestmentTransactionStatus parses a string into an InvestmentTransactionStatus.
func ParseInvestmentTransactionStatus(str string) (InvestmentTransactionStatus, error) {
	s := InvestmentTransactionStatus(strings.ToLower(str))
	if !s.IsValid() {
		return "", fmt.Errorf("invalid investment transaction status: %q", str)
	}
	return s, nil
}

// Value implements the driver.Valuer interface for database storage.
func (s InvestmentTransactionStatus) Value() (driver.Value, error) {
	return string(s), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (s *InvestmentTransactionStatus) Scan(value any) error {
	if value == nil {
		*s = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*s = InvestmentTransactionStatus(v)
	case []byte:
		*s = InvestmentTransactionStatus(string(v))
	default:
		return fmt.Errorf("unsupported type for InvestmentTransactionStatus: %T", value)
	}
	return nil
}

// InvestmentTransaction represents a transaction in an investment account.
type InvestmentTransaction struct {
	BaseModel

	// Core properties (required)
	AccountID   ID                          `json:"account_id"`
	Date        Date                        `json:"date"`
	Type        InvestmentTransactionType   `json:"type"`
	TotalAmount Money                       `json:"total_amount"`
	Status      InvestmentTransactionStatus `json:"status"`

	// Optional properties
	SecurityID    NullableID       `json:"security_id"`
	Shares        NullableQuantity `json:"shares"`
	PricePerShare NullableMoney    `json:"price_per_share"`
	Commission    NullableMoney    `json:"commission"`
	Memo          NullableString   `json:"memo"`
}

// NewInvestmentTransaction creates a new InvestmentTransaction with required fields.
func NewInvestmentTransaction(accountID ID, date Date, txnType InvestmentTransactionType, totalAmount Money) *InvestmentTransaction {
	return &InvestmentTransaction{
		BaseModel:   NewBaseModel(),
		AccountID:   accountID,
		Date:        date,
		Type:        txnType,
		TotalAmount: totalAmount,
		Status:      InvestmentTransactionStatusPending,
	}
}

// NewInvestmentTransactionWithSecurity creates a new InvestmentTransaction with security and shares.
func NewInvestmentTransactionWithSecurity(accountID ID, date Date, txnType InvestmentTransactionType, totalAmount Money, securityID ID, shares Quantity) *InvestmentTransaction {
	t := NewInvestmentTransaction(accountID, date, txnType, totalAmount)
	t.SecurityID = NullableID{ID: securityID, Valid: true}
	t.Shares = NullableQuantity{Quantity: shares, Valid: true}
	return t
}

// SetSecurity sets the security for this transaction.
func (t *InvestmentTransaction) SetSecurity(securityID ID) {
	t.SecurityID = NullableID{ID: securityID, Valid: true}
	t.Touch()
}

// ClearSecurity removes the security from this transaction.
func (t *InvestmentTransaction) ClearSecurity() {
	t.SecurityID = NullableID{Valid: false}
	t.Touch()
}

// HasSecurity returns true if the transaction has a security set.
func (t *InvestmentTransaction) HasSecurity() bool {
	return t.SecurityID.Valid
}

// SetShares sets the shares for this transaction.
func (t *InvestmentTransaction) SetShares(shares Quantity) {
	t.Shares = NullableQuantity{Quantity: shares, Valid: true}
	t.Touch()
}

// ClearShares removes the shares from this transaction.
func (t *InvestmentTransaction) ClearShares() {
	t.Shares = NullableQuantity{Valid: false}
	t.Touch()
}

// HasShares returns true if the transaction has shares set.
func (t *InvestmentTransaction) HasShares() bool {
	return t.Shares.Valid
}

// SetPricePerShare sets the price per share for this transaction.
func (t *InvestmentTransaction) SetPricePerShare(price Money) {
	t.PricePerShare = NullableMoney{Money: price, Valid: true}
	t.Touch()
}

// ClearPricePerShare removes the price per share from this transaction.
func (t *InvestmentTransaction) ClearPricePerShare() {
	t.PricePerShare = NullableMoney{Valid: false}
	t.Touch()
}

// HasPricePerShare returns true if the transaction has a price per share set.
func (t *InvestmentTransaction) HasPricePerShare() bool {
	return t.PricePerShare.Valid
}

// SetCommission sets the commission for this transaction.
func (t *InvestmentTransaction) SetCommission(commission Money) {
	t.Commission = NullableMoney{Money: commission, Valid: true}
	t.Touch()
}

// ClearCommission removes the commission from this transaction.
func (t *InvestmentTransaction) ClearCommission() {
	t.Commission = NullableMoney{Valid: false}
	t.Touch()
}

// SetMemo sets the memo for this transaction.
func (t *InvestmentTransaction) SetMemo(memo string) {
	if memo == "" {
		t.Memo = NullableString{Valid: false}
	} else {
		t.Memo = NullableString{String: memo, Valid: true}
	}
	t.Touch()
}

// ClearMemo removes the memo from this transaction.
func (t *InvestmentTransaction) ClearMemo() {
	t.Memo = NullableString{Valid: false}
	t.Touch()
}

// SetStatus sets the transaction status.
func (t *InvestmentTransaction) SetStatus(status InvestmentTransactionStatus) {
	t.Status = status
	t.Touch()
}

// Clear marks the transaction as cleared.
func (t *InvestmentTransaction) Clear() {
	t.SetStatus(InvestmentTransactionStatusCleared)
}

// Reconcile marks the transaction as reconciled.
func (t *InvestmentTransaction) Reconcile() {
	t.SetStatus(InvestmentTransactionStatusReconciled)
}

// MarkPending marks the transaction as pending.
func (t *InvestmentTransaction) MarkPending() {
	t.SetStatus(InvestmentTransactionStatusPending)
}

// IsPending returns true if the transaction is pending.
func (t *InvestmentTransaction) IsPending() bool {
	return t.Status == InvestmentTransactionStatusPending
}

// IsCleared returns true if the transaction is cleared.
func (t *InvestmentTransaction) IsCleared() bool {
	return t.Status == InvestmentTransactionStatusCleared
}

// IsReconciled returns true if the transaction is reconciled.
func (t *InvestmentTransaction) IsReconciled() bool {
	return t.Status == InvestmentTransactionStatusReconciled
}

// Validate validates the investment transaction and returns any validation errors.
func (t *InvestmentTransaction) Validate() ValidationErrors {
	v := NewValidator()

	// Required fields
	v.RequiredID("account_id", t.AccountID)
	v.RequiredDate("date", t.Date)

	// Type must be valid
	if !t.Type.IsValid() {
		v.errors.Add("type", "must be a valid investment transaction type")
	}

	// Status must be valid
	if !t.Status.IsValid() {
		v.errors.Add("status", "must be a valid investment transaction status (pending, cleared, reconciled)")
	}

	// Security required for security-based types
	if t.Type.RequiresSecurity() && !t.SecurityID.Valid {
		v.errors.Add("security_id", "is required for "+t.Type.String()+" transactions")
	}

	// Shares required for share-based types
	if t.Type.RequiresShares() && !t.Shares.Valid {
		v.errors.Add("shares", "is required for "+t.Type.String()+" transactions")
	}

	// Commission must be non-negative if set
	if t.Commission.Valid && t.Commission.Money.IsNegative() {
		v.errors.Add("commission", "must not be negative")
	}

	// Memo max length
	if t.Memo.Valid {
		v.MaxLength("memo", t.Memo.String, 1000)
	}

	return v.Errors()
}

// IsValid returns true if the investment transaction passes validation.
func (t *InvestmentTransaction) IsValid() bool {
	return !t.Validate().HasErrors()
}
