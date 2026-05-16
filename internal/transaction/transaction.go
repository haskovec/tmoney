package transaction

import (
	"database/sql/driver"
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/types"
)

// Status represents the status of a transaction.
type Status string

const (
	StatusUncleared  Status = "uncleared"
	StatusCleared    Status = "cleared"
	StatusReconciled Status = "reconciled"
	StatusVoid       Status = "void"
)

// AllStatuses returns all valid transaction statuses.
func AllStatuses() []Status {
	return []Status{
		StatusUncleared,
		StatusCleared,
		StatusReconciled,
		StatusVoid,
	}
}

// String returns the string representation of the Status.
func (ts Status) String() string {
	return string(ts)
}

// IsValid returns true if the Status is a valid status.
func (ts Status) IsValid() bool {
	switch ts {
	case StatusUncleared, StatusCleared, StatusReconciled, StatusVoid:
		return true
	}
	return false
}

// Code returns the single-letter status code for CLI display.
func (ts Status) Code() string {
	switch ts {
	case StatusUncleared:
		return "U"
	case StatusCleared:
		return "C"
	case StatusReconciled:
		return "R"
	case StatusVoid:
		return "V"
	default:
		return "?"
	}
}

// DisplayName returns a human-readable name for the transaction status.
func (ts Status) DisplayName() string {
	switch ts {
	case StatusUncleared:
		return "Uncleared"
	case StatusCleared:
		return "Cleared"
	case StatusReconciled:
		return "Reconciled"
	case StatusVoid:
		return "Void"
	default:
		return string(ts)
	}
}

// ParseStatus parses a string into a Status.
func ParseStatus(s string) (Status, error) {
	ts := Status(strings.ToLower(s))
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid transaction status: %q", s)
	}
	return ts, nil
}

// Value implements the driver.Valuer interface for database storage.
func (ts Status) Value() (driver.Value, error) {
	return string(ts), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (ts *Status) Scan(value any) error {
	if value == nil {
		*ts = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*ts = Status(v)
	case []byte:
		*ts = Status(string(v))
	default:
		return fmt.Errorf("unsupported type for Status: %T", value)
	}
	return nil
}

// Transaction represents a financial transaction in an account.
type Transaction struct {
	types.BaseModel

	// Core properties (required)
	AccountID types.ID    `json:"account_id"`
	Date      types.Date  `json:"date"`
	Amount    types.Money `json:"amount"`
	Status    Status      `json:"status"`

	// Optional properties
	PayeeID     types.NullableID     `json:"payee_id"`
	CategoryID  types.NullableID     `json:"category_id"`
	Memo        types.NullableString `json:"memo"`
	CheckNumber types.NullableString `json:"check_number"`

	// Transfer properties (for linked transfers between accounts)
	TransferID        types.NullableID `json:"transfer_id"`
	TransferAccountID types.NullableID `json:"transfer_account_id"`

	// Bank reference ID for imported transactions (e.g., OFX FITID)
	BankReferenceID types.NullableString `json:"bank_reference_id"`
}

// NewTransaction creates a new Transaction with generated ID and timestamps.
func NewTransaction(accountID types.ID, date types.Date, amount types.Money) *Transaction {
	return &Transaction{
		BaseModel: types.NewBaseModel(),
		AccountID: accountID,
		Date:      date,
		Amount:    amount,
		Status:    StatusUncleared,
	}
}

// NewTransactionWithPayee creates a new Transaction with a payee.
func NewTransactionWithPayee(accountID types.ID, date types.Date, amount types.Money, payeeID types.ID) *Transaction {
	t := NewTransaction(accountID, date, amount)
	t.PayeeID = types.NullableID{ID: payeeID, Valid: true}
	return t
}

// NewTransactionFull creates a new Transaction with all common properties.
func NewTransactionFull(accountID types.ID, date types.Date, amount types.Money, payeeID, categoryID types.ID, memo string) *Transaction {
	t := NewTransaction(accountID, date, amount)
	if !payeeID.IsNil() {
		t.PayeeID = types.NullableID{ID: payeeID, Valid: true}
	}
	if !categoryID.IsNil() {
		t.CategoryID = types.NullableID{ID: categoryID, Valid: true}
	}
	if memo != "" {
		t.Memo = types.NullableString{String: memo, Valid: true}
	}
	return t
}

// SetPayee sets the payee for this transaction.
func (t *Transaction) SetPayee(payeeID types.ID) {
	t.PayeeID = types.NullableID{ID: payeeID, Valid: true}
	t.Touch()
}

// ClearPayee removes the payee from this transaction.
func (t *Transaction) ClearPayee() {
	t.PayeeID = types.NullableID{Valid: false}
	t.Touch()
}

// HasPayee returns true if the transaction has a payee set.
func (t *Transaction) HasPayee() bool {
	return t.PayeeID.Valid
}

// SetCategory sets the category for this transaction.
func (t *Transaction) SetCategory(categoryID types.ID) {
	t.CategoryID = types.NullableID{ID: categoryID, Valid: true}
	t.Touch()
}

// ClearCategory removes the category from this transaction.
func (t *Transaction) ClearCategory() {
	t.CategoryID = types.NullableID{Valid: false}
	t.Touch()
}

// HasCategory returns true if the transaction has a category set.
func (t *Transaction) HasCategory() bool {
	return t.CategoryID.Valid
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

// SetCheckNumber sets the check number for this transaction.
func (t *Transaction) SetCheckNumber(number string) {
	if number == "" {
		t.CheckNumber = types.NullableString{Valid: false}
	} else {
		t.CheckNumber = types.NullableString{String: number, Valid: true}
	}
	t.Touch()
}

// ClearCheckNumber removes the check number from this transaction.
func (t *Transaction) ClearCheckNumber() {
	t.CheckNumber = types.NullableString{Valid: false}
	t.Touch()
}

// SetStatus sets the transaction status.
func (t *Transaction) SetStatus(status Status) {
	t.Status = status
	t.Touch()
}

// Clear marks the transaction as cleared.
func (t *Transaction) Clear() {
	t.SetStatus(StatusCleared)
}

// Reconcile marks the transaction as reconciled.
func (t *Transaction) Reconcile() {
	t.SetStatus(StatusReconciled)
}

// MarkUncleared marks the transaction as uncleared.
func (t *Transaction) MarkUncleared() {
	t.SetStatus(StatusUncleared)
}

// Void marks the transaction as void.
func (t *Transaction) Void() {
	t.SetStatus(StatusVoid)
}

// IsVoid returns true if the transaction is voided.
func (t *Transaction) IsVoid() bool {
	return t.Status == StatusVoid
}

// IsReconciled returns true if the transaction is reconciled.
func (t *Transaction) IsReconciled() bool {
	return t.Status == StatusReconciled
}

// IsTransfer returns true if this transaction is part of a transfer.
func (t *Transaction) IsTransfer() bool {
	return t.TransferID.Valid && t.TransferAccountID.Valid
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

// SetBankReferenceID sets the bank reference ID (e.g., OFX FITID) for this transaction.
func (t *Transaction) SetBankReferenceID(refID string) {
	if refID == "" {
		t.BankReferenceID = types.NullableString{Valid: false}
	} else {
		t.BankReferenceID = types.NullableString{String: refID, Valid: true}
	}
	t.Touch()
}

// HasBankReferenceID returns true if the transaction has a bank reference ID.
func (t *Transaction) HasBankReferenceID() bool {
	return t.BankReferenceID.Valid && t.BankReferenceID.String != ""
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
func (t *Transaction) Validate() types.ValidationErrors {
	v := types.NewValidator()

	// Required fields
	v.RequiredID("account_id", t.AccountID)
	v.RequiredDate("date", t.Date)

	// Amount cannot be zero (unless void)
	if t.Amount.IsZero() && t.Status != StatusVoid {
		v.AddError("amount", "cannot be zero")
	}

	// Status validation
	if !t.Status.IsValid() {
		v.AddError("status", "must be a valid transaction status (uncleared, cleared, reconciled, or void)")
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
		v.AddError("transfer", "transfer_id and transfer_account_id must both be set or neither")
	}

	return v.Errors()
}

// IsValid returns true if the transaction passes validation.
func (t *Transaction) IsValid() bool {
	return !t.Validate().HasErrors()
}

// Split represents one line of a split transaction.
//
// A line is either categorized (CategoryID set, transfer fields zero) or a
// transfer-line that moves cash to another account (TransferAccountID +
// TransferID set, CategoryID = NilID). The two shapes are mutually
// exclusive and enforced at the database via CHECK constraints (see
// migration 014). Service-layer validation of the new shape lives in later
// tasks; this struct just exposes the fields.
type Split struct {
	types.BaseModel

	// Core properties
	TransactionID types.ID    `json:"transaction_id"`
	CategoryID    types.ID    `json:"category_id"`
	Amount        types.Money `json:"amount"`

	// Transfer-line properties (set together iff the line is a transfer)
	TransferAccountID types.NullableID `json:"transfer_account_id"`
	TransferID        types.NullableID `json:"transfer_id"`

	// Optional properties
	Memo types.NullableString `json:"memo"`
}

// NewSplit creates a new Split with generated ID and timestamps.
func NewSplit(transactionID, categoryID types.ID, amount types.Money) *Split {
	return &Split{
		BaseModel:     types.NewBaseModel(),
		TransactionID: transactionID,
		CategoryID:    categoryID,
		Amount:        amount,
	}
}

// NewSplitWithMemo creates a new Split with a memo.
func NewSplitWithMemo(transactionID, categoryID types.ID, amount types.Money, memo string) *Split {
	s := NewSplit(transactionID, categoryID, amount)
	if memo != "" {
		s.Memo = types.NullableString{String: memo, Valid: true}
	}
	return s
}

// SetMemo sets the memo for this split.
func (s *Split) SetMemo(memo string) {
	if memo == "" {
		s.Memo = types.NullableString{Valid: false}
	} else {
		s.Memo = types.NullableString{String: memo, Valid: true}
	}
	s.Touch()
}

// ClearMemo removes the memo from this split.
func (s *Split) ClearMemo() {
	s.Memo = types.NullableString{Valid: false}
	s.Touch()
}

// Validate validates the split and returns any validation errors.
func (s *Split) Validate() types.ValidationErrors {
	v := types.NewValidator()

	// Required fields
	v.RequiredID("transaction_id", s.TransactionID)
	v.RequiredID("category_id", s.CategoryID)

	// Amount cannot be zero
	if s.Amount.IsZero() {
		v.AddError("amount", "cannot be zero")
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
func (sc SplitCollection) Total() types.Money {
	total := types.ZeroMoney
	for _, s := range sc {
		total = total.Add(s.Amount)
	}
	return total
}

// ValidateAgainstTransaction validates that the splits' signed sum equals the
// transaction amount. Line signs are independent of the parent's sign: a
// paycheck transaction with parent +100 and lines +200 (gross) / -100 (tax) is
// valid, as is a legacy same-sign split with parent -100 and lines -70 / -30.
func (sc SplitCollection) ValidateAgainstTransaction(transactionAmount types.Money) types.ValidationErrors {
	var errors types.ValidationErrors

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
func NewTransferPair(fromAccountID, toAccountID types.ID, date types.Date, amount types.Money) *TransferPair {
	transferID := types.NewID()

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
func (tp *TransferPair) Validate() types.ValidationErrors {
	var errors types.ValidationErrors

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

// SearchCriteria defines filters for searching transactions.
type SearchCriteria struct {
	// PayeeName filters by partial payee name match (case-insensitive).
	PayeeName string
	// Memo filters by partial memo match (case-insensitive).
	Memo string
	// CategoryName filters by partial category name match (case-insensitive).
	CategoryName string
	// StartDate filters transactions on or after this date.
	StartDate *types.Date
	// EndDate filters transactions on or before this date.
	EndDate *types.Date
	// AccountID filters by specific account.
	AccountID *types.ID
	// MinAmount filters transactions with amount >= this value.
	MinAmount *types.Money
	// MaxAmount filters transactions with amount <= this value.
	MaxAmount *types.Money
}

// HasFilters returns true if any search criteria is set.
func (c *SearchCriteria) HasFilters() bool {
	return c.PayeeName != "" ||
		c.Memo != "" ||
		c.CategoryName != "" ||
		c.StartDate != nil ||
		c.EndDate != nil ||
		c.AccountID != nil ||
		c.MinAmount != nil ||
		c.MaxAmount != nil
}
