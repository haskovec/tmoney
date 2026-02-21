package models

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

// TransactionStatus represents the status of a transaction.
type TransactionStatus string

const (
	TransactionStatusUncleared  TransactionStatus = "uncleared"
	TransactionStatusCleared    TransactionStatus = "cleared"
	TransactionStatusReconciled TransactionStatus = "reconciled"
	TransactionStatusVoid       TransactionStatus = "void"
)

// AllTransactionStatuses returns all valid transaction statuses.
func AllTransactionStatuses() []TransactionStatus {
	return []TransactionStatus{
		TransactionStatusUncleared,
		TransactionStatusCleared,
		TransactionStatusReconciled,
		TransactionStatusVoid,
	}
}

// String returns the string representation of the TransactionStatus.
func (ts TransactionStatus) String() string {
	return string(ts)
}

// IsValid returns true if the TransactionStatus is a valid status.
func (ts TransactionStatus) IsValid() bool {
	switch ts {
	case TransactionStatusUncleared, TransactionStatusCleared, TransactionStatusReconciled, TransactionStatusVoid:
		return true
	}
	return false
}

// Code returns the single-letter status code for CLI display.
func (ts TransactionStatus) Code() string {
	switch ts {
	case TransactionStatusUncleared:
		return "U"
	case TransactionStatusCleared:
		return "C"
	case TransactionStatusReconciled:
		return "R"
	case TransactionStatusVoid:
		return "V"
	default:
		return "?"
	}
}

// DisplayName returns a human-readable name for the transaction status.
func (ts TransactionStatus) DisplayName() string {
	switch ts {
	case TransactionStatusUncleared:
		return "Uncleared"
	case TransactionStatusCleared:
		return "Cleared"
	case TransactionStatusReconciled:
		return "Reconciled"
	case TransactionStatusVoid:
		return "Void"
	default:
		return string(ts)
	}
}

// ParseTransactionStatus parses a string into a TransactionStatus.
func ParseTransactionStatus(s string) (TransactionStatus, error) {
	ts := TransactionStatus(strings.ToLower(s))
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid transaction status: %q", s)
	}
	return ts, nil
}

// Value implements the driver.Valuer interface for database storage.
func (ts TransactionStatus) Value() (driver.Value, error) {
	return string(ts), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (ts *TransactionStatus) Scan(value interface{}) error {
	if value == nil {
		*ts = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*ts = TransactionStatus(v)
	case []byte:
		*ts = TransactionStatus(string(v))
	default:
		return fmt.Errorf("unsupported type for TransactionStatus: %T", value)
	}
	return nil
}

// Transaction represents a financial transaction in an account.
type Transaction struct {
	BaseModel

	// Core properties (required)
	AccountID ID                `json:"account_id"`
	Date      Date              `json:"date"`
	Amount    Money             `json:"amount"`
	Status    TransactionStatus `json:"status"`

	// Optional properties
	PayeeID     NullableID     `json:"payee_id,omitempty"`
	CategoryID  NullableID     `json:"category_id,omitempty"`
	Memo        NullableString `json:"memo,omitempty"`
	CheckNumber NullableString `json:"check_number,omitempty"`

	// Transfer properties (for linked transfers between accounts)
	TransferID        NullableID `json:"transfer_id,omitempty"`
	TransferAccountID NullableID `json:"transfer_account_id,omitempty"`
}

// NewTransaction creates a new Transaction with generated ID and timestamps.
func NewTransaction(accountID ID, date Date, amount Money) *Transaction {
	return &Transaction{
		BaseModel: NewBaseModel(),
		AccountID: accountID,
		Date:      date,
		Amount:    amount,
		Status:    TransactionStatusUncleared,
	}
}

// NewTransactionWithPayee creates a new Transaction with a payee.
func NewTransactionWithPayee(accountID ID, date Date, amount Money, payeeID ID) *Transaction {
	t := NewTransaction(accountID, date, amount)
	t.PayeeID = NullableID{ID: payeeID, Valid: true}
	return t
}

// NewTransactionFull creates a new Transaction with all common properties.
func NewTransactionFull(accountID ID, date Date, amount Money, payeeID, categoryID ID, memo string) *Transaction {
	t := NewTransaction(accountID, date, amount)
	if !payeeID.IsNil() {
		t.PayeeID = NullableID{ID: payeeID, Valid: true}
	}
	if !categoryID.IsNil() {
		t.CategoryID = NullableID{ID: categoryID, Valid: true}
	}
	if memo != "" {
		t.Memo = NullableString{String: memo, Valid: true}
	}
	return t
}

// SetPayee sets the payee for this transaction.
func (t *Transaction) SetPayee(payeeID ID) {
	t.PayeeID = NullableID{ID: payeeID, Valid: true}
	t.Touch()
}

// ClearPayee removes the payee from this transaction.
func (t *Transaction) ClearPayee() {
	t.PayeeID = NullableID{Valid: false}
	t.Touch()
}

// HasPayee returns true if the transaction has a payee set.
func (t *Transaction) HasPayee() bool {
	return t.PayeeID.Valid
}

// SetCategory sets the category for this transaction.
func (t *Transaction) SetCategory(categoryID ID) {
	t.CategoryID = NullableID{ID: categoryID, Valid: true}
	t.Touch()
}

// ClearCategory removes the category from this transaction.
func (t *Transaction) ClearCategory() {
	t.CategoryID = NullableID{Valid: false}
	t.Touch()
}

// HasCategory returns true if the transaction has a category set.
func (t *Transaction) HasCategory() bool {
	return t.CategoryID.Valid
}

// SetMemo sets the memo for this transaction.
func (t *Transaction) SetMemo(memo string) {
	if memo == "" {
		t.Memo = NullableString{Valid: false}
	} else {
		t.Memo = NullableString{String: memo, Valid: true}
	}
	t.Touch()
}

// ClearMemo removes the memo from this transaction.
func (t *Transaction) ClearMemo() {
	t.Memo = NullableString{Valid: false}
	t.Touch()
}

// SetCheckNumber sets the check number for this transaction.
func (t *Transaction) SetCheckNumber(number string) {
	if number == "" {
		t.CheckNumber = NullableString{Valid: false}
	} else {
		t.CheckNumber = NullableString{String: number, Valid: true}
	}
	t.Touch()
}

// ClearCheckNumber removes the check number from this transaction.
func (t *Transaction) ClearCheckNumber() {
	t.CheckNumber = NullableString{Valid: false}
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

// MarkUncleared marks the transaction as uncleared.
func (t *Transaction) MarkUncleared() {
	t.SetStatus(TransactionStatusUncleared)
}

// Void marks the transaction as void.
func (t *Transaction) Void() {
	t.SetStatus(TransactionStatusVoid)
}

// IsVoid returns true if the transaction is voided.
func (t *Transaction) IsVoid() bool {
	return t.Status == TransactionStatusVoid
}

// IsReconciled returns true if the transaction is reconciled.
func (t *Transaction) IsReconciled() bool {
	return t.Status == TransactionStatusReconciled
}

// IsTransfer returns true if this transaction is part of a transfer.
func (t *Transaction) IsTransfer() bool {
	return t.TransferID.Valid && t.TransferAccountID.Valid
}

// SetTransfer links this transaction to another as a transfer.
func (t *Transaction) SetTransfer(transferID, transferAccountID ID) {
	t.TransferID = NullableID{ID: transferID, Valid: true}
	t.TransferAccountID = NullableID{ID: transferAccountID, Valid: true}
	t.Touch()
}

// ClearTransfer removes the transfer link from this transaction.
func (t *Transaction) ClearTransfer() {
	t.TransferID = NullableID{Valid: false}
	t.TransferAccountID = NullableID{Valid: false}
	t.Touch()
}

// IsIncome returns true if the transaction is an income (positive amount).
func (t *Transaction) IsIncome() bool {
	return t.Amount.IsPositive()
}

// IsExpense returns true if the transaction is an expense (negative amount).
func (t *Transaction) IsExpense() bool {
	return t.Amount.IsNegative()
}

// Validate validates the transaction and returns any validation errors.
func (t *Transaction) Validate() ValidationErrors {
	v := NewValidator()

	// Required fields
	v.RequiredID("account_id", t.AccountID)
	v.RequiredDate("date", t.Date)

	// Amount cannot be zero (unless void)
	if t.Amount.IsZero() && t.Status != TransactionStatusVoid {
		v.errors.Add("amount", "cannot be zero")
	}

	// Status validation
	if !t.Status.IsValid() {
		v.errors.Add("status", "must be a valid transaction status (uncleared, cleared, reconciled, or void)")
	}

	// Optional field length limits
	if t.Memo.Valid {
		v.MaxLength("memo", t.Memo.String, 1000)
	}
	if t.CheckNumber.Valid {
		v.MaxLength("check_number", t.CheckNumber.String, 50)
	}

	// Transfer consistency: both transfer fields must be set or neither
	if t.TransferID.Valid != t.TransferAccountID.Valid {
		v.errors.Add("transfer", "transfer_id and transfer_account_id must both be set or neither")
	}

	return v.Errors()
}

// IsValid returns true if the transaction passes validation.
func (t *Transaction) IsValid() bool {
	return !t.Validate().HasErrors()
}

// Split represents a portion of a split transaction assigned to a category.
type Split struct {
	BaseModel

	// Core properties (required)
	TransactionID ID    `json:"transaction_id"`
	CategoryID    ID    `json:"category_id"`
	Amount        Money `json:"amount"`

	// Optional properties
	Memo NullableString `json:"memo,omitempty"`
}

// NewSplit creates a new Split with generated ID and timestamps.
func NewSplit(transactionID, categoryID ID, amount Money) *Split {
	return &Split{
		BaseModel:     NewBaseModel(),
		TransactionID: transactionID,
		CategoryID:    categoryID,
		Amount:        amount,
	}
}

// NewSplitWithMemo creates a new Split with a memo.
func NewSplitWithMemo(transactionID, categoryID ID, amount Money, memo string) *Split {
	s := NewSplit(transactionID, categoryID, amount)
	if memo != "" {
		s.Memo = NullableString{String: memo, Valid: true}
	}
	return s
}

// SetMemo sets the memo for this split.
func (s *Split) SetMemo(memo string) {
	if memo == "" {
		s.Memo = NullableString{Valid: false}
	} else {
		s.Memo = NullableString{String: memo, Valid: true}
	}
	s.Touch()
}

// ClearMemo removes the memo from this split.
func (s *Split) ClearMemo() {
	s.Memo = NullableString{Valid: false}
	s.Touch()
}

// Validate validates the split and returns any validation errors.
func (s *Split) Validate() ValidationErrors {
	v := NewValidator()

	// Required fields
	v.RequiredID("transaction_id", s.TransactionID)
	v.RequiredID("category_id", s.CategoryID)

	// Amount cannot be zero
	if s.Amount.IsZero() {
		v.errors.Add("amount", "cannot be zero")
	}

	// Optional field length limits
	if s.Memo.Valid {
		v.MaxLength("memo", s.Memo.String, 500)
	}

	return v.Errors()
}

// IsValid returns true if the split passes validation.
func (s *Split) IsValid() bool {
	return !s.Validate().HasErrors()
}

// SplitCollection represents a collection of splits for a transaction.
type SplitCollection []*Split

// Total returns the sum of all split amounts.
func (sc SplitCollection) Total() Money {
	total := ZeroMoney
	for _, s := range sc {
		total = total.Add(s.Amount)
	}
	return total
}

// ValidateAgainstTransaction validates that the splits total matches the transaction amount.
func (sc SplitCollection) ValidateAgainstTransaction(transactionAmount Money) ValidationErrors {
	var errors ValidationErrors

	if len(sc) == 0 {
		return errors
	}

	total := sc.Total()
	if !total.Equal(transactionAmount) {
		errors.Add("splits", fmt.Sprintf("split total (%s) does not equal transaction amount (%s)",
			total.String(), transactionAmount.String()))
	}

	return errors
}

// TransferPair represents a pair of linked transfer transactions.
type TransferPair struct {
	FromTransaction *Transaction
	ToTransaction   *Transaction
}

// NewTransferPair creates a linked pair of transfer transactions.
// The fromAccount sends money to the toAccount.
func NewTransferPair(fromAccountID, toAccountID ID, date Date, amount Money) *TransferPair {
	transferID := NewID()

	// From side: negative amount (money leaving)
	from := NewTransaction(fromAccountID, date, amount.Neg())
	from.SetTransfer(transferID, toAccountID)

	// To side: positive amount (money arriving)
	to := NewTransaction(toAccountID, date, amount.Abs())
	to.SetTransfer(transferID, fromAccountID)

	return &TransferPair{
		FromTransaction: from,
		ToTransaction:   to,
	}
}

// Validate validates both transactions in the transfer pair.
func (tp *TransferPair) Validate() ValidationErrors {
	var errors ValidationErrors

	// Validate individual transactions
	fromErrors := tp.FromTransaction.Validate()
	for _, e := range fromErrors {
		errors.Add("from_transaction."+e.Field, e.Message)
	}

	toErrors := tp.ToTransaction.Validate()
	for _, e := range toErrors {
		errors.Add("to_transaction."+e.Field, e.Message)
	}

	// Validate transfer consistency
	if !tp.FromTransaction.IsTransfer() {
		errors.Add("from_transaction", "must be a transfer")
	}
	if !tp.ToTransaction.IsTransfer() {
		errors.Add("to_transaction", "must be a transfer")
	}

	// Amounts should be equal and opposite
	if !tp.FromTransaction.Amount.Add(tp.ToTransaction.Amount).IsZero() {
		errors.Add("transfer", "amounts must be equal and opposite")
	}

	// Transfer IDs should match
	if tp.FromTransaction.TransferID != tp.ToTransaction.TransferID {
		errors.Add("transfer", "transfer IDs must match")
	}

	// Cross-reference account IDs
	if tp.FromTransaction.TransferAccountID.ID != tp.ToTransaction.AccountID {
		errors.Add("transfer", "from transaction transfer_account_id must match to transaction account_id")
	}
	if tp.ToTransaction.TransferAccountID.ID != tp.FromTransaction.AccountID {
		errors.Add("transfer", "to transaction transfer_account_id must match from transaction account_id")
	}

	// Accounts must be different
	if tp.FromTransaction.AccountID == tp.ToTransaction.AccountID {
		errors.Add("transfer", "from and to accounts must be different")
	}

	return errors
}

// IsValid returns true if the transfer pair passes validation.
func (tp *TransferPair) IsValid() bool {
	return !tp.Validate().HasErrors()
}
