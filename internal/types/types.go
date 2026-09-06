package types

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"time"
	"uuid"

	"github.com/alpacahq/alpacadecimal"
	"github.com/duckdb/duckdb-go/v2"
)

// ID represents a unique identifier for model entities.
// It wraps uuid.UUID for consistent handling across the application.
type ID uuid.UUID

// NilID is the zero value ID (all zeros).
var NilID = ID(uuid.Nil())

// NewID generates a new time-ordered UUID v7.
// UUID v7 provides better database index locality than random UUIDs.
func NewID() ID {
	return ID(uuid.NewV7())
}

// ParseID parses a string into an ID.
func ParseID(s string) (ID, error) {
	u, err := uuid.Parse(s)
	if err != nil {
		return NilID, fmt.Errorf("invalid ID format: %w", err)
	}
	return ID(u), nil
}

// MustParseID parses a string into an ID, panicking on error.
// Use only in tests or with known-good values.
func MustParseID(s string) ID {
	id, err := ParseID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// String returns the string representation of the ID.
func (id ID) String() string {
	return uuid.UUID(id).String()
}

// IsNil returns true if the ID is the zero value.
func (id ID) IsNil() bool {
	return uuid.UUID(id) == uuid.Nil()
}

// Value implements the driver.Valuer interface for database storage.
func (id ID) Value() (driver.Value, error) {
	return uuid.UUID(id).String(), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (id *ID) Scan(value any) error {
	if value == nil {
		*id = NilID
		return nil
	}

	switch v := value.(type) {
	case string:
		parsed, err := uuid.Parse(v)
		if err != nil {
			return fmt.Errorf("failed to parse ID from string: %w", err)
		}
		*id = ID(parsed)
	case []byte:
		// DuckDB returns UUIDs as raw 16-byte binary
		if len(v) == 16 {
			*id = ID(uuid.UUID(v))
		} else {
			// Try parsing as string representation
			parsed, err := uuid.Parse(string(v))
			if err != nil {
				return fmt.Errorf("failed to parse ID from bytes: %w", err)
			}
			*id = ID(parsed)
		}
	default:
		return fmt.Errorf("unsupported type for ID: %T", value)
	}
	return nil
}

// Money represents a monetary value with fixed-point decimal precision.
// Uses alpacadecimal for accurate financial calculations.
type Money struct {
	value alpacadecimal.Decimal
}

// ZeroMoney is the zero value for Money.
var ZeroMoney = Money{value: alpacadecimal.NewFromInt(0)}

// NewMoney creates a Money value from a string representation.
func NewMoney(s string) (Money, error) {
	d, err := alpacadecimal.NewFromString(s)
	if err != nil {
		return ZeroMoney, fmt.Errorf("invalid money format: %w", err)
	}
	return Money{value: d}, nil
}

// MustNewMoney creates a Money value from a string, panicking on error.
// Use only in tests or with known-good values.
func MustNewMoney(s string) Money {
	m, err := NewMoney(s)
	if err != nil {
		panic(err)
	}
	return m
}

// NewMoneyFromInt creates a Money value from an integer (whole units).
func NewMoneyFromInt(i int64) Money {
	return Money{value: alpacadecimal.NewFromInt(i)}
}

// NewMoneyFromFloat creates a Money value from a float64.
// Note: May have precision issues; prefer NewMoney with string for exact values.
func NewMoneyFromFloat(f float64) Money {
	return Money{value: alpacadecimal.NewFromFloat(f)}
}

// NewMoneyFromDecimal creates a Money value from a raw decimal, preserving its
// full precision. It is the inverse of Money.Decimal and lets callers that do
// intermediate decimal arithmetic (e.g. the loan amortization engine) round-trip
// back into a Money.
func NewMoneyFromDecimal(d alpacadecimal.Decimal) Money {
	return Money{value: d}
}

// String returns the string representation of the Money value.
func (m Money) String() string {
	return m.value.String()
}

// Float64 returns the float64 representation (may lose precision).
func (m Money) Float64() float64 {
	f, _ := m.value.Float64()
	return f
}

// Decimal returns the underlying decimal value at full precision. Use it when a
// calculation needs to multiply/divide a Money by a non-integer factor (Money's
// own arithmetic only exposes integer division); pair it with NewMoneyFromDecimal
// to return to a Money.
func (m Money) Decimal() alpacadecimal.Decimal {
	return m.value
}

// IsZero returns true if the Money value is zero.
func (m Money) IsZero() bool {
	return m.value.IsZero()
}

// IsNegative returns true if the Money value is negative.
func (m Money) IsNegative() bool {
	return m.value.IsNegative()
}

// IsPositive returns true if the Money value is positive (greater than zero).
func (m Money) IsPositive() bool {
	return m.value.IsPositive()
}

// Add returns the sum of two Money values.
func (m Money) Add(other Money) Money {
	return Money{value: m.value.Add(other.value)}
}

// Sub returns the difference of two Money values.
func (m Money) Sub(other Money) Money {
	return Money{value: m.value.Sub(other.value)}
}

// Mul returns the product of Money and a multiplier.
func (m Money) Mul(multiplier alpacadecimal.Decimal) Money {
	return Money{value: m.value.Mul(multiplier)}
}

// Div returns the quotient of Money and a divisor. A zero divisor panics, like
// integer division: silently returning zero money turned a caller's bug (an
// empty average, a missing count) into a wrong amount on the ledger.
func (m Money) Div(divisor int64) Money {
	if divisor == 0 {
		panic("types.Money.Div: division by zero")
	}
	return Money{value: m.value.Div(alpacadecimal.NewFromInt(divisor))}
}

// Neg returns the negated Money value.
func (m Money) Neg() Money {
	return Money{value: m.value.Neg()}
}

// Abs returns the absolute value.
func (m Money) Abs() Money {
	return Money{value: m.value.Abs()}
}

// Cmp compares two Money values: -1 if m < other, 0 if equal, 1 if m > other.
func (m Money) Cmp(other Money) int {
	return m.value.Cmp(other.value)
}

// Equal returns true if two Money values are equal.
func (m Money) Equal(other Money) bool {
	return m.value.Equal(other.value)
}

// Value implements the driver.Valuer interface for database storage.
func (m Money) Value() (driver.Value, error) {
	return m.value.String(), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (m *Money) Scan(value any) error {
	if value == nil {
		*m = ZeroMoney
		return nil
	}

	switch v := value.(type) {
	case string:
		d, err := alpacadecimal.NewFromString(v)
		if err != nil {
			return fmt.Errorf("failed to parse Money from string: %w", err)
		}
		m.value = d
	case []byte:
		d, err := alpacadecimal.NewFromString(string(v))
		if err != nil {
			return fmt.Errorf("failed to parse Money from bytes: %w", err)
		}
		m.value = d
	case float64:
		m.value = alpacadecimal.NewFromFloat(v)
	case int64:
		m.value = alpacadecimal.NewFromInt(v)
	case duckdb.Decimal:
		d, err := alpacadecimal.NewFromString(v.String())
		if err != nil {
			return fmt.Errorf("failed to parse Money from duckdb.Decimal: %w", err)
		}
		m.value = d
	case *duckdb.Decimal:
		d, err := alpacadecimal.NewFromString(v.String())
		if err != nil {
			return fmt.Errorf("failed to parse Money from *duckdb.Decimal: %w", err)
		}
		m.value = d
	default:
		return fmt.Errorf("unsupported type for Money: %T", value)
	}
	return nil
}

// Quantity represents a quantity value (like shares) with higher precision.
// Uses 8 decimal places for fractional shares support.
type Quantity struct {
	value alpacadecimal.Decimal
}

// ZeroQuantity is the zero value for Quantity.
var ZeroQuantity = Quantity{value: alpacadecimal.NewFromInt(0)}

// NewQuantity creates a Quantity value from a string representation.
func NewQuantity(s string) (Quantity, error) {
	d, err := alpacadecimal.NewFromString(s)
	if err != nil {
		return ZeroQuantity, fmt.Errorf("invalid quantity format: %w", err)
	}
	return Quantity{value: d}, nil
}

// MustNewQuantity creates a Quantity value from a string, panicking on error.
func MustNewQuantity(s string) Quantity {
	q, err := NewQuantity(s)
	if err != nil {
		panic(err)
	}
	return q
}

// NewQuantityFromFloat creates a Quantity from a float64.
func NewQuantityFromFloat(f float64) Quantity {
	return Quantity{value: alpacadecimal.NewFromFloat(f)}
}

// String returns the string representation of the Quantity.
func (q Quantity) String() string {
	return q.value.String()
}

// Float64 returns the float64 representation (may lose precision).
func (q Quantity) Float64() float64 {
	f, _ := q.value.Float64()
	return f
}

// IsZero returns true if the Quantity is zero.
func (q Quantity) IsZero() bool {
	return q.value.IsZero()
}

// IsPositive returns true if the Quantity is greater than zero.
func (q Quantity) IsPositive() bool {
	return q.value.GreaterThan(alpacadecimal.NewFromInt(0))
}

// IsNegative returns true if the Quantity is less than zero.
func (q Quantity) IsNegative() bool {
	return q.value.LessThan(alpacadecimal.NewFromInt(0))
}

// Add returns the sum of two Quantities.
func (q Quantity) Add(other Quantity) Quantity {
	return Quantity{value: q.value.Add(other.value)}
}

// Sub returns the difference of two Quantities.
func (q Quantity) Sub(other Quantity) Quantity {
	return Quantity{value: q.value.Sub(other.value)}
}

// Mul returns the product of Quantity and a multiplier.
func (q Quantity) Mul(multiplier alpacadecimal.Decimal) Quantity {
	return Quantity{value: q.value.Mul(multiplier)}
}

// Cmp compares two Quantity values: -1 if q < other, 0 if equal, 1 if q > other.
func (q Quantity) Cmp(other Quantity) int {
	return q.value.Cmp(other.value)
}

// Equal returns true if two Quantity values are equal.
func (q Quantity) Equal(other Quantity) bool {
	return q.value.Equal(other.value)
}

// Floor returns the largest integer value less than or equal to this quantity.
func (q Quantity) Floor() Quantity {
	return Quantity{value: q.value.Floor()}
}

// Decimal returns the underlying decimal value.
func (q Quantity) Decimal() alpacadecimal.Decimal {
	return q.value
}

// Value implements the driver.Valuer interface for database storage.
func (q Quantity) Value() (driver.Value, error) {
	return q.value.String(), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (q *Quantity) Scan(value any) error {
	if value == nil {
		*q = ZeroQuantity
		return nil
	}

	switch v := value.(type) {
	case string:
		d, err := alpacadecimal.NewFromString(v)
		if err != nil {
			return fmt.Errorf("failed to parse Quantity from string: %w", err)
		}
		q.value = d
	case []byte:
		d, err := alpacadecimal.NewFromString(string(v))
		if err != nil {
			return fmt.Errorf("failed to parse Quantity from bytes: %w", err)
		}
		q.value = d
	case float64:
		q.value = alpacadecimal.NewFromFloat(v)
	case duckdb.Decimal:
		d, err := alpacadecimal.NewFromString(v.String())
		if err != nil {
			return fmt.Errorf("failed to parse Quantity from duckdb.Decimal: %w", err)
		}
		q.value = d
	case *duckdb.Decimal:
		d, err := alpacadecimal.NewFromString(v.String())
		if err != nil {
			return fmt.Errorf("failed to parse Quantity from *duckdb.Decimal: %w", err)
		}
		q.value = d
	default:
		return fmt.Errorf("unsupported type for Quantity: %T", value)
	}
	return nil
}

// Timestamp represents a point in time, stored in UTC.
type Timestamp time.Time

// ZeroTimestamp is the zero value for Timestamp.
var ZeroTimestamp = Timestamp(time.Time{})

// Now returns the current time as a Timestamp.
func Now() Timestamp {
	return Timestamp(time.Now().UTC())
}

// NewTimestamp creates a Timestamp from a time.Time.
func NewTimestamp(t time.Time) Timestamp {
	return Timestamp(t.UTC())
}

// ParseTimestamp parses an ISO 8601 timestamp string.
func ParseTimestamp(s string) (Timestamp, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return ZeroTimestamp, fmt.Errorf("invalid timestamp format: %w", err)
	}
	return Timestamp(t.UTC()), nil
}

// Time returns the underlying time.Time value.
func (ts Timestamp) Time() time.Time {
	return time.Time(ts)
}

// String returns the ISO 8601 representation of the Timestamp.
func (ts Timestamp) String() string {
	return time.Time(ts).Format(time.RFC3339)
}

// IsZero returns true if the Timestamp is the zero value.
func (ts Timestamp) IsZero() bool {
	return time.Time(ts).IsZero()
}

// Before reports whether ts is before other.
func (ts Timestamp) Before(other Timestamp) bool {
	return time.Time(ts).Before(time.Time(other))
}

// After reports whether ts is after other.
func (ts Timestamp) After(other Timestamp) bool {
	return time.Time(ts).After(time.Time(other))
}

// Value implements the driver.Valuer interface for database storage.
func (ts Timestamp) Value() (driver.Value, error) {
	return time.Time(ts), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (ts *Timestamp) Scan(value any) error {
	if value == nil {
		*ts = ZeroTimestamp
		return nil
	}

	switch v := value.(type) {
	case time.Time:
		*ts = Timestamp(v.UTC())
	case string:
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			// Try parsing without timezone (DuckDB format)
			t, err = time.Parse("2006-01-02 15:04:05", v)
			if err != nil {
				return fmt.Errorf("failed to parse Timestamp from string: %w", err)
			}
		}
		*ts = Timestamp(t.UTC())
	case []byte:
		t, err := time.Parse(time.RFC3339, string(v))
		if err != nil {
			t, err = time.Parse("2006-01-02 15:04:05", string(v))
			if err != nil {
				return fmt.Errorf("failed to parse Timestamp from bytes: %w", err)
			}
		}
		*ts = Timestamp(t.UTC())
	default:
		return fmt.Errorf("unsupported type for Timestamp: %T", value)
	}
	return nil
}

// Date represents a calendar date without time information.
type Date time.Time

// ZeroDate is the zero value for Date.
var ZeroDate = Date(time.Time{})

// Today returns the current date.
func Today() Date {
	now := time.Now()
	return Date(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC))
}

// NewDate creates a Date from year, month, day.
func NewDate(year int, month time.Month, day int) Date {
	return Date(time.Date(year, month, day, 0, 0, 0, 0, time.UTC))
}

// ParseDate parses a date string in YYYY-MM-DD format.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return ZeroDate, fmt.Errorf("invalid date format (expected YYYY-MM-DD): %w", err)
	}
	return Date(t), nil
}

// MustParseDate parses a date string, panicking on error.
func MustParseDate(s string) Date {
	d, err := ParseDate(s)
	if err != nil {
		panic(err)
	}
	return d
}

// Time returns the underlying time.Time value.
func (d Date) Time() time.Time {
	return time.Time(d)
}

// String returns the YYYY-MM-DD representation of the Date.
func (d Date) String() string {
	return time.Time(d).Format("2006-01-02")
}

// IsZero returns true if the Date is the zero value.
func (d Date) IsZero() bool {
	return time.Time(d).IsZero()
}

// Before reports whether d is before other.
func (d Date) Before(other Date) bool {
	return time.Time(d).Before(time.Time(other))
}

// After reports whether d is after other.
func (d Date) After(other Date) bool {
	return time.Time(d).After(time.Time(other))
}

// Equal reports whether d equals other.
func (d Date) Equal(other Date) bool {
	return time.Time(d).Equal(time.Time(other))
}

// AddDays returns the Date that is n days after d.
func (d Date) AddDays(n int) Date {
	return Date(time.Time(d).AddDate(0, 0, n))
}

// AddMonths returns the Date that is n months after d.
func (d Date) AddMonths(n int) Date {
	return Date(time.Time(d).AddDate(0, n, 0))
}

// AddYears returns the Date that is n years after d.
func (d Date) AddYears(n int) Date {
	return Date(time.Time(d).AddDate(n, 0, 0))
}

// Year returns the year of the Date.
func (d Date) Year() int {
	return time.Time(d).Year()
}

// Month returns the month of the Date.
func (d Date) Month() time.Month {
	return time.Time(d).Month()
}

// Day returns the day of the month.
func (d Date) Day() int {
	return time.Time(d).Day()
}

// Value implements the driver.Valuer interface for database storage.
func (d Date) Value() (driver.Value, error) {
	return time.Time(d), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (d *Date) Scan(value any) error {
	if value == nil {
		*d = ZeroDate
		return nil
	}

	switch v := value.(type) {
	case time.Time:
		*d = Date(time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, time.UTC))
	case string:
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return fmt.Errorf("failed to parse Date from string: %w", err)
		}
		*d = Date(t)
	case []byte:
		t, err := time.Parse("2006-01-02", string(v))
		if err != nil {
			return fmt.Errorf("failed to parse Date from bytes: %w", err)
		}
		*d = Date(t)
	default:
		return fmt.Errorf("unsupported type for Date: %T", value)
	}
	return nil
}

// NullableTimestamp is a Timestamp that can be null in the database.
type NullableTimestamp struct {
	Timestamp Timestamp
	Valid     bool
}

// Value implements the driver.Valuer interface.
func (n NullableTimestamp) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return n.Timestamp.Value()
}

// Scan implements the sql.Scanner interface.
func (n *NullableTimestamp) Scan(value any) error {
	if value == nil {
		n.Timestamp = ZeroTimestamp
		n.Valid = false
		return nil
	}
	n.Valid = true
	return n.Timestamp.Scan(value)
}

// NullableID is an ID that can be null in the database.
type NullableID struct {
	ID    ID
	Valid bool
}

// Value implements the driver.Valuer interface.
func (n NullableID) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return n.ID.Value()
}

// Scan implements the sql.Scanner interface.
func (n *NullableID) Scan(value any) error {
	if value == nil {
		n.ID = NilID
		n.Valid = false
		return nil
	}
	n.Valid = true
	return n.ID.Scan(value)
}

// NullableMoney is a Money value that can be null in the database.
type NullableMoney struct {
	Money Money
	Valid bool
}

// Value implements the driver.Valuer interface.
func (n NullableMoney) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return n.Money.Value()
}

// Scan implements the sql.Scanner interface.
func (n *NullableMoney) Scan(value any) error {
	if value == nil {
		n.Money = ZeroMoney
		n.Valid = false
		return nil
	}
	n.Valid = true
	return n.Money.Scan(value)
}

// NullableString wraps sql.NullString for consistency.
type NullableString = sql.NullString

// NullableInt wraps sql.NullInt64 for consistency.
type NullableInt = sql.NullInt64

// NullableDate is a Date that can be null in the database.
type NullableDate struct {
	Date  Date
	Valid bool
}

// Value implements the driver.Valuer interface.
func (n NullableDate) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return n.Date.Value()
}

// Scan implements the sql.Scanner interface.
func (n *NullableDate) Scan(value any) error {
	if value == nil {
		n.Date = ZeroDate
		n.Valid = false
		return nil
	}
	n.Valid = true
	return n.Date.Scan(value)
}

// NullableQuantity is a Quantity that can be null in the database.
type NullableQuantity struct {
	Quantity Quantity
	Valid    bool
}

// Value implements the driver.Valuer interface.
func (n NullableQuantity) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return n.Quantity.Value()
}

// Scan implements the sql.Scanner interface.
func (n *NullableQuantity) Scan(value any) error {
	if value == nil {
		n.Quantity = ZeroQuantity
		n.Valid = false
		return nil
	}
	n.Valid = true
	return n.Quantity.Scan(value)
}
