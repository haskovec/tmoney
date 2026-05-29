package investment

import (
	"database/sql/driver"
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/types"
)

// TransactionType represents the type of investment transaction.
type TransactionType string

const (
	TransactionTypeBuy              TransactionType = "buy"
	TransactionTypeSell             TransactionType = "sell"
	TransactionTypeDividend         TransactionType = "dividend"
	TransactionTypeReinvestDividend TransactionType = "reinvest_dividend"
	TransactionTypeFee              TransactionType = "fee"
	TransactionTypeFeeLiquidation   TransactionType = "fee_liquidation"
	TransactionTypeDeposit          TransactionType = "deposit"
	TransactionTypeWithdrawal       TransactionType = "withdrawal"
	TransactionTypeInterest         TransactionType = "interest"
	TransactionTypeTransferShares   TransactionType = "transfer_shares"
	TransactionTypeTransferCash     TransactionType = "transfer_cash"
	TransactionTypeExchange         TransactionType = "exchange"
)

// AllTransactionTypes returns all valid investment transaction types.
func AllTransactionTypes() []TransactionType {
	return []TransactionType{
		TransactionTypeBuy,
		TransactionTypeSell,
		TransactionTypeDividend,
		TransactionTypeReinvestDividend,
		TransactionTypeFee,
		TransactionTypeFeeLiquidation,
		TransactionTypeDeposit,
		TransactionTypeWithdrawal,
		TransactionTypeInterest,
		TransactionTypeTransferShares,
		TransactionTypeTransferCash,
		TransactionTypeExchange,
	}
}

// String returns the string representation of the TransactionType.
func (itt TransactionType) String() string {
	return string(itt)
}

// IsValid returns true if the TransactionType is a valid type.
func (itt TransactionType) IsValid() bool {
	switch itt {
	case TransactionTypeBuy, TransactionTypeSell,
		TransactionTypeDividend, TransactionTypeReinvestDividend,
		TransactionTypeFee, TransactionTypeFeeLiquidation,
		TransactionTypeDeposit, TransactionTypeWithdrawal,
		TransactionTypeInterest, TransactionTypeTransferShares,
		TransactionTypeTransferCash, TransactionTypeExchange:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the investment transaction type.
func (itt TransactionType) DisplayName() string {
	switch itt {
	case TransactionTypeBuy:
		return "Buy"
	case TransactionTypeSell:
		return "Sell"
	case TransactionTypeDividend:
		return "Dividend"
	case TransactionTypeReinvestDividend:
		return "Reinvest Dividend"
	case TransactionTypeFee:
		return "Fee"
	case TransactionTypeFeeLiquidation:
		return "Fee via Liquidation"
	case TransactionTypeDeposit:
		return "Deposit"
	case TransactionTypeWithdrawal:
		return "Withdrawal"
	case TransactionTypeInterest:
		return "Interest"
	case TransactionTypeTransferShares:
		return "Transfer Shares"
	case TransactionTypeTransferCash:
		return "Transfer Cash"
	case TransactionTypeExchange:
		return "Exchange"
	default:
		return string(itt)
	}
}

// RequiresSecurity returns true if this transaction type must reference a security.
func (itt TransactionType) RequiresSecurity() bool {
	switch itt {
	case TransactionTypeBuy, TransactionTypeSell,
		TransactionTypeDividend, TransactionTypeReinvestDividend,
		TransactionTypeFeeLiquidation, TransactionTypeTransferShares,
		TransactionTypeExchange:
		return true
	}
	return false
}

// RequiresShares returns true if this transaction type must have a shares value.
func (itt TransactionType) RequiresShares() bool {
	switch itt {
	case TransactionTypeBuy, TransactionTypeSell,
		TransactionTypeReinvestDividend, TransactionTypeFeeLiquidation,
		TransactionTypeTransferShares, TransactionTypeExchange:
		return true
	}
	return false
}

// mutatesPositionOrPrice reports whether posting this type would create a
// future-dated price row (buy/sell/reinvest auto-create one) or a future
// share/lot change — the only reason an investment transaction is restricted
// to non-future dates. Pure cash operations (deposit, withdrawal, fee,
// interest, transfer_cash, and dividend — a payment linked to a security but
// involving no share price or count change) carry no such hazard and may be
// dated in the future, mirroring bank transactions (e.g. posting a scheduled
// paycheck whose transfer legs fund a 401k or HSA a day early).
func (itt TransactionType) mutatesPositionOrPrice() bool {
	return itt.RequiresShares()
}

// AffectsCash returns true if this transaction type affects the account cash position.
func (itt TransactionType) AffectsCash() bool {
	switch itt {
	case TransactionTypeBuy, TransactionTypeSell,
		TransactionTypeDividend, TransactionTypeFee,
		TransactionTypeDeposit, TransactionTypeWithdrawal,
		TransactionTypeInterest, TransactionTypeTransferCash:
		return true
	}
	return false
}

// ParseTransactionType parses a string into a TransactionType.
func ParseTransactionType(s string) (TransactionType, error) {
	itt := TransactionType(strings.ToLower(s))
	if !itt.IsValid() {
		return "", fmt.Errorf("invalid investment transaction type: %q", s)
	}
	return itt, nil
}

// Value implements the driver.Valuer interface for database storage.
func (itt TransactionType) Value() (driver.Value, error) {
	return string(itt), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (itt *TransactionType) Scan(value any) error {
	if value == nil {
		*itt = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*itt = TransactionType(v)
	case []byte:
		*itt = TransactionType(string(v))
	default:
		return fmt.Errorf("unsupported type for TransactionType: %T", value)
	}
	return nil
}

// TransactionStatus represents the status of an investment transaction.
type TransactionStatus string

const (
	TransactionStatusPending    TransactionStatus = "pending"
	TransactionStatusCleared    TransactionStatus = "cleared"
	TransactionStatusReconciled TransactionStatus = "reconciled"
)

// AllTransactionStatuses returns all valid investment transaction statuses.
func AllTransactionStatuses() []TransactionStatus {
	return []TransactionStatus{
		TransactionStatusPending,
		TransactionStatusCleared,
		TransactionStatusReconciled,
	}
}

// String returns the string representation of the TransactionStatus.
func (s TransactionStatus) String() string {
	return string(s)
}

// IsValid returns true if the TransactionStatus is a valid status.
func (s TransactionStatus) IsValid() bool {
	switch s {
	case TransactionStatusPending, TransactionStatusCleared, TransactionStatusReconciled:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the investment transaction status.
func (s TransactionStatus) DisplayName() string {
	switch s {
	case TransactionStatusPending:
		return "Pending"
	case TransactionStatusCleared:
		return "Cleared"
	case TransactionStatusReconciled:
		return "Reconciled"
	default:
		return string(s)
	}
}

// ParseTransactionStatus parses a string into a TransactionStatus.
func ParseTransactionStatus(str string) (TransactionStatus, error) {
	s := TransactionStatus(strings.ToLower(str))
	if !s.IsValid() {
		return "", fmt.Errorf("invalid investment transaction status: %q", str)
	}
	return s, nil
}

// Value implements the driver.Valuer interface for database storage.
func (s TransactionStatus) Value() (driver.Value, error) {
	return string(s), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (s *TransactionStatus) Scan(value any) error {
	if value == nil {
		*s = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*s = TransactionStatus(v)
	case []byte:
		*s = TransactionStatus(string(v))
	default:
		return fmt.Errorf("unsupported type for TransactionStatus: %T", value)
	}
	return nil
}

// Transaction represents a transaction in an investment account.
type Transaction struct {
	types.BaseModel

	// Core properties (required)
	AccountID   types.ID          `json:"account_id"`
	Date        types.Date        `json:"date"`
	Type        TransactionType   `json:"type"`
	TotalAmount types.Money       `json:"total_amount"`
	Status      TransactionStatus `json:"status"`

	// Optional properties
	SecurityID    types.NullableID       `json:"security_id"`
	Shares        types.NullableQuantity `json:"shares"`
	PricePerShare types.NullableMoney    `json:"price_per_share"`
	Commission    types.NullableMoney    `json:"commission"`
	Memo          types.NullableString   `json:"memo"`

	// Transfer properties (for linked transfers between investment and regular accounts)
	TransferID        types.NullableID `json:"transfer_id"`
	TransferAccountID types.NullableID `json:"transfer_account_id"`
}

// NewTransaction creates a new Transaction with required fields.
func NewTransaction(accountID types.ID, date types.Date, txnType TransactionType, totalAmount types.Money) *Transaction {
	return &Transaction{
		BaseModel:   types.NewBaseModel(),
		AccountID:   accountID,
		Date:        date,
		Type:        txnType,
		TotalAmount: totalAmount,
		Status:      TransactionStatusPending,
	}
}

// NewTransactionWithSecurity creates a new Transaction with security and shares.
func NewTransactionWithSecurity(accountID types.ID, date types.Date, txnType TransactionType, totalAmount types.Money, securityID types.ID, shares types.Quantity) *Transaction {
	t := NewTransaction(accountID, date, txnType, totalAmount)
	t.SecurityID = types.NullableID{ID: securityID, Valid: true}
	t.Shares = types.NullableQuantity{Quantity: shares, Valid: true}
	return t
}

// SetSecurity sets the security for this transaction.
func (t *Transaction) SetSecurity(securityID types.ID) {
	t.SecurityID = types.NullableID{ID: securityID, Valid: true}
	t.Touch()
}

// ClearSecurity removes the security from this transaction.
func (t *Transaction) ClearSecurity() {
	t.SecurityID = types.NullableID{Valid: false}
	t.Touch()
}

// HasSecurity returns true if the transaction has a security set.
func (t *Transaction) HasSecurity() bool {
	return t.SecurityID.Valid
}

// SetShares sets the shares for this transaction.
func (t *Transaction) SetShares(shares types.Quantity) {
	t.Shares = types.NullableQuantity{Quantity: shares, Valid: true}
	t.Touch()
}

// ClearShares removes the shares from this transaction.
func (t *Transaction) ClearShares() {
	t.Shares = types.NullableQuantity{Valid: false}
	t.Touch()
}

// HasShares returns true if the transaction has shares set.
func (t *Transaction) HasShares() bool {
	return t.Shares.Valid
}

// SetPricePerShare sets the price per share for this transaction.
func (t *Transaction) SetPricePerShare(price types.Money) {
	t.PricePerShare = types.NullableMoney{Money: price, Valid: true}
	t.Touch()
}

// ClearPricePerShare removes the price per share from this transaction.
func (t *Transaction) ClearPricePerShare() {
	t.PricePerShare = types.NullableMoney{Valid: false}
	t.Touch()
}

// HasPricePerShare returns true if the transaction has a price per share set.
func (t *Transaction) HasPricePerShare() bool {
	return t.PricePerShare.Valid
}

// SetCommission sets the commission for this transaction.
func (t *Transaction) SetCommission(commission types.Money) {
	t.Commission = types.NullableMoney{Money: commission, Valid: true}
	t.Touch()
}

// ClearCommission removes the commission from this transaction.
func (t *Transaction) ClearCommission() {
	t.Commission = types.NullableMoney{Valid: false}
	t.Touch()
}

// SetTransfer links this transaction to another as a transfer.
func (t *Transaction) SetTransfer(transferID, transferAccountID types.ID) {
	t.TransferID = types.NullableID{ID: transferID, Valid: true}
	t.TransferAccountID = types.NullableID{ID: transferAccountID, Valid: true}
	t.Touch()
}

// ClearTransfer removes the transfer link from this transaction.
func (t *Transaction) ClearTransfer() {
	t.TransferID = types.NullableID{Valid: false}
	t.TransferAccountID = types.NullableID{Valid: false}
	t.Touch()
}

// IsTransfer returns true if this transaction is part of a transfer.
func (t *Transaction) IsTransfer() bool {
	return t.TransferID.Valid && t.TransferAccountID.Valid
}

// SetMemo sets the memo for this transaction.
func (t *Transaction) SetMemo(memo string) {
	if memo == "" {
		t.Memo = types.NullableString{Valid: false}
	} else {
		t.Memo = types.NullableString{String: memo, Valid: true}
	}
	t.Touch()
}

// ClearMemo removes the memo from this transaction.
func (t *Transaction) ClearMemo() {
	t.Memo = types.NullableString{Valid: false}
	t.Touch()
}

// SetStatus sets the transaction status.
func (t *Transaction) SetStatus(status TransactionStatus) {
	t.Status = status
	t.Touch()
}

// Clear marks the transaction as cleared.
func (t *Transaction) Clear() {
	t.SetStatus(TransactionStatusCleared)
}

// Reconcile marks the transaction as reconciled.
func (t *Transaction) Reconcile() {
	t.SetStatus(TransactionStatusReconciled)
}

// MarkPending marks the transaction as pending.
func (t *Transaction) MarkPending() {
	t.SetStatus(TransactionStatusPending)
}

// IsPending returns true if the transaction is pending.
func (t *Transaction) IsPending() bool {
	return t.Status == TransactionStatusPending
}

// IsCleared returns true if the transaction is cleared.
func (t *Transaction) IsCleared() bool {
	return t.Status == TransactionStatusCleared
}

// IsReconciled returns true if the transaction is reconciled.
func (t *Transaction) IsReconciled() bool {
	return t.Status == TransactionStatusReconciled
}

// Validate validates the investment transaction and returns any validation errors.
func (t *Transaction) Validate() types.ValidationErrors {
	v := types.NewValidator()

	// Required fields
	v.RequiredID("account_id", t.AccountID)
	v.RequiredDate("date", t.Date)
	// Only position/price-bearing types are restricted to non-future dates;
	// cash operations (incl. dividend) may be scheduled forward. See
	// mutatesPositionOrPrice.
	if t.Type.mutatesPositionOrPrice() {
		v.NotFutureDate("date", t.Date)
	}

	// Type must be valid
	if !t.Type.IsValid() {
		v.AddError("type", "must be a valid investment transaction type")
	}

	// Status must be valid
	if !t.Status.IsValid() {
		v.AddError("status", "must be a valid investment transaction status (pending, cleared, reconciled)")
	}

	// Security required for security-based types
	if t.Type.RequiresSecurity() && !t.SecurityID.Valid {
		v.AddError("security_id", "is required for "+t.Type.String()+" transactions")
	}

	// Shares required for share-based types
	if t.Type.RequiresShares() && !t.Shares.Valid {
		v.AddError("shares", "is required for "+t.Type.String()+" transactions")
	}

	// Commission must be non-negative if set
	if t.Commission.Valid && t.Commission.Money.IsNegative() {
		v.AddError("commission", "must not be negative")
	}

	// Memo max length
	if t.Memo.Valid {
		v.MaxLength("memo", t.Memo.String, 1000)
	}

	return v.Errors()
}

// IsValid returns true if the investment transaction passes validation.
func (t *Transaction) IsValid() bool {
	return !t.Validate().HasErrors()
}
