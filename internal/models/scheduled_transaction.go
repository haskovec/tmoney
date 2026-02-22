package models

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"
)

// Frequency represents how often a scheduled transaction repeats.
type Frequency string

const (
	FrequencyDaily     Frequency = "daily"
	FrequencyWeekly    Frequency = "weekly"
	FrequencyBiweekly  Frequency = "biweekly"
	FrequencyMonthly   Frequency = "monthly"
	FrequencyQuarterly Frequency = "quarterly"
	FrequencyYearly    Frequency = "yearly"
)

// AllFrequencies returns all valid frequencies.
func AllFrequencies() []Frequency {
	return []Frequency{
		FrequencyDaily,
		FrequencyWeekly,
		FrequencyBiweekly,
		FrequencyMonthly,
		FrequencyQuarterly,
		FrequencyYearly,
	}
}

// String returns the string representation of the Frequency.
func (f Frequency) String() string {
	return string(f)
}

// IsValid returns true if the Frequency is a valid frequency.
func (f Frequency) IsValid() bool {
	switch f {
	case FrequencyDaily, FrequencyWeekly, FrequencyBiweekly,
		FrequencyMonthly, FrequencyQuarterly, FrequencyYearly:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the frequency.
func (f Frequency) DisplayName() string {
	switch f {
	case FrequencyDaily:
		return "Daily"
	case FrequencyWeekly:
		return "Weekly"
	case FrequencyBiweekly:
		return "Biweekly"
	case FrequencyMonthly:
		return "Monthly"
	case FrequencyQuarterly:
		return "Quarterly"
	case FrequencyYearly:
		return "Yearly"
	default:
		return string(f)
	}
}

// ParseFrequency parses a string into a Frequency.
func ParseFrequency(s string) (Frequency, error) {
	f := Frequency(strings.ToLower(s))
	if !f.IsValid() {
		return "", fmt.Errorf("invalid frequency: %q", s)
	}
	return f, nil
}

// Value implements the driver.Valuer interface for database storage.
func (f Frequency) Value() (driver.Value, error) {
	return string(f), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (f *Frequency) Scan(value any) error {
	if value == nil {
		*f = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*f = Frequency(v)
	case []byte:
		*f = Frequency(string(v))
	default:
		return fmt.Errorf("unsupported type for Frequency: %T", value)
	}
	return nil
}

// ScheduledTransaction represents a recurring or future transaction.
type ScheduledTransaction struct {
	BaseModel

	// Core properties (required)
	AccountID ID        `json:"account_id"`
	Frequency Frequency `json:"frequency"`
	StartDate Date      `json:"start_date"`
	NextDate  Date      `json:"next_date"`

	// Schedule properties
	Interval             int          `json:"interval"`              // Every N periods (default: 1)
	EndDate              NullableDate `json:"end_date"`              // When schedule ends (null = indefinite)
	Occurrences          NullableInt  `json:"occurrences"`           // Total number of times to repeat
	OccurrencesRemaining NullableInt  `json:"occurrences_remaining"` // Countdown for fixed occurrences
	DayOfMonth           NullableInt  `json:"day_of_month"`          // Specific day (1-31, or -1 for last day)
	DayOfWeek            NullableInt  `json:"day_of_week"`           // Day of week (0=Sunday, 6=Saturday)

	// Transaction template properties
	PayeeID    NullableID     `json:"payee_id"`
	CategoryID NullableID     `json:"category_id"`
	Amount     NullableMoney  `json:"amount"` // Null if variable amount
	Memo       NullableString `json:"memo"`

	// Variable amount estimation
	AmountEstimateCount NullableInt `json:"amount_estimate_count"` // Use average of last N transactions
}

// NewScheduledTransaction creates a new ScheduledTransaction with required properties.
func NewScheduledTransaction(accountID ID, frequency Frequency, startDate Date) *ScheduledTransaction {
	return &ScheduledTransaction{
		BaseModel: NewBaseModel(),
		AccountID: accountID,
		Frequency: frequency,
		StartDate: startDate,
		NextDate:  startDate,
		Interval:  1,
	}
}

// NewScheduledTransactionWithAmount creates a new ScheduledTransaction with an amount.
func NewScheduledTransactionWithAmount(accountID ID, frequency Frequency, startDate Date, amount Money) *ScheduledTransaction {
	st := NewScheduledTransaction(accountID, frequency, startDate)
	st.Amount = NullableMoney{Money: amount, Valid: true}
	return st
}

// NewScheduledTransactionFull creates a new ScheduledTransaction with all common properties.
func NewScheduledTransactionFull(
	accountID ID,
	frequency Frequency,
	startDate Date,
	amount Money,
	payeeID, categoryID ID,
	memo string,
) *ScheduledTransaction {
	st := NewScheduledTransactionWithAmount(accountID, frequency, startDate, amount)
	if !payeeID.IsNil() {
		st.PayeeID = NullableID{ID: payeeID, Valid: true}
	}
	if !categoryID.IsNil() {
		st.CategoryID = NullableID{ID: categoryID, Valid: true}
	}
	if memo != "" {
		st.Memo = NullableString{String: memo, Valid: true}
	}
	return st
}

// SetPayee sets the payee for this scheduled transaction.
func (st *ScheduledTransaction) SetPayee(payeeID ID) {
	st.PayeeID = NullableID{ID: payeeID, Valid: true}
	st.Touch()
}

// ClearPayee removes the payee from this scheduled transaction.
func (st *ScheduledTransaction) ClearPayee() {
	st.PayeeID = NullableID{Valid: false}
	st.Touch()
}

// HasPayee returns true if the scheduled transaction has a payee set.
func (st *ScheduledTransaction) HasPayee() bool {
	return st.PayeeID.Valid
}

// SetCategory sets the category for this scheduled transaction.
func (st *ScheduledTransaction) SetCategory(categoryID ID) {
	st.CategoryID = NullableID{ID: categoryID, Valid: true}
	st.Touch()
}

// ClearCategory removes the category from this scheduled transaction.
func (st *ScheduledTransaction) ClearCategory() {
	st.CategoryID = NullableID{Valid: false}
	st.Touch()
}

// HasCategory returns true if the scheduled transaction has a category set.
func (st *ScheduledTransaction) HasCategory() bool {
	return st.CategoryID.Valid
}

// SetAmount sets the amount for this scheduled transaction.
func (st *ScheduledTransaction) SetAmount(amount Money) {
	st.Amount = NullableMoney{Money: amount, Valid: true}
	st.Touch()
}

// ClearAmount removes the amount (marks as variable).
func (st *ScheduledTransaction) ClearAmount() {
	st.Amount = NullableMoney{Valid: false}
	st.Touch()
}

// HasAmount returns true if the scheduled transaction has a fixed amount.
func (st *ScheduledTransaction) HasAmount() bool {
	return st.Amount.Valid
}

// IsVariableAmount returns true if the amount is variable (not fixed).
func (st *ScheduledTransaction) IsVariableAmount() bool {
	return !st.Amount.Valid
}

// SetMemo sets the memo for this scheduled transaction.
func (st *ScheduledTransaction) SetMemo(memo string) {
	if memo == "" {
		st.Memo = NullableString{Valid: false}
	} else {
		st.Memo = NullableString{String: memo, Valid: true}
	}
	st.Touch()
}

// ClearMemo removes the memo from this scheduled transaction.
func (st *ScheduledTransaction) ClearMemo() {
	st.Memo = NullableString{Valid: false}
	st.Touch()
}

// SetEndDate sets an end date for the schedule.
func (st *ScheduledTransaction) SetEndDate(endDate Date) {
	st.EndDate = NullableDate{Date: endDate, Valid: true}
	// Clear occurrences if end date is set
	st.Occurrences = NullableInt{Valid: false}
	st.OccurrencesRemaining = NullableInt{Valid: false}
	st.Touch()
}

// ClearEndDate removes the end date (makes schedule indefinite).
func (st *ScheduledTransaction) ClearEndDate() {
	st.EndDate = NullableDate{Valid: false}
	st.Touch()
}

// SetOccurrences sets a fixed number of occurrences.
func (st *ScheduledTransaction) SetOccurrences(count int64) {
	st.Occurrences = NullableInt{Int64: count, Valid: true}
	st.OccurrencesRemaining = NullableInt{Int64: count, Valid: true}
	// Clear end date if occurrences is set
	st.EndDate = NullableDate{Valid: false}
	st.Touch()
}

// ClearOccurrences removes the occurrence limit (makes schedule indefinite).
func (st *ScheduledTransaction) ClearOccurrences() {
	st.Occurrences = NullableInt{Valid: false}
	st.OccurrencesRemaining = NullableInt{Valid: false}
	st.Touch()
}

// SetDayOfMonth sets the specific day of month for monthly/quarterly/yearly schedules.
// Use -1 for last day of month.
func (st *ScheduledTransaction) SetDayOfMonth(day int) {
	st.DayOfMonth = NullableInt{Int64: int64(day), Valid: true}
	st.Touch()
}

// ClearDayOfMonth removes the specific day of month.
func (st *ScheduledTransaction) ClearDayOfMonth() {
	st.DayOfMonth = NullableInt{Valid: false}
	st.Touch()
}

// SetDayOfWeek sets the specific day of week for weekly schedules.
// 0 = Sunday, 6 = Saturday.
func (st *ScheduledTransaction) SetDayOfWeek(day int) {
	st.DayOfWeek = NullableInt{Int64: int64(day), Valid: true}
	st.Touch()
}

// ClearDayOfWeek removes the specific day of week.
func (st *ScheduledTransaction) ClearDayOfWeek() {
	st.DayOfWeek = NullableInt{Valid: false}
	st.Touch()
}

// SetAmountEstimateCount sets the number of past transactions to average for estimates.
func (st *ScheduledTransaction) SetAmountEstimateCount(count int) {
	st.AmountEstimateCount = NullableInt{Int64: int64(count), Valid: true}
	st.Touch()
}

// ClearAmountEstimateCount removes the amount estimation.
func (st *ScheduledTransaction) ClearAmountEstimateCount() {
	st.AmountEstimateCount = NullableInt{Valid: false}
	st.Touch()
}

// SetInterval sets the interval between occurrences.
func (st *ScheduledTransaction) SetInterval(interval int) {
	if interval < 1 {
		interval = 1
	}
	st.Interval = interval
	st.Touch()
}

// IsIndefinite returns true if the schedule has no end date or occurrence limit.
func (st *ScheduledTransaction) IsIndefinite() bool {
	return !st.EndDate.Valid && !st.Occurrences.Valid
}

// IsDue returns true if the next date is today or in the past.
func (st *ScheduledTransaction) IsDue() bool {
	today := Today()
	return !st.NextDate.After(today)
}

// IsCompleted returns true if the schedule has finished all occurrences.
func (st *ScheduledTransaction) IsCompleted() bool {
	if st.OccurrencesRemaining.Valid && st.OccurrencesRemaining.Int64 <= 0 {
		return true
	}
	if st.EndDate.Valid && st.NextDate.After(st.EndDate.Date) {
		return true
	}
	return false
}

// CalculateNextDate calculates the next occurrence date after the current next_date.
func (st *ScheduledTransaction) CalculateNextDate() Date {
	return calculateNextDate(st.NextDate, st.Frequency, st.Interval, st.DayOfMonth, st.DayOfWeek)
}

// AdvanceSchedule advances the schedule to the next occurrence.
// Returns false if the schedule is completed.
func (st *ScheduledTransaction) AdvanceSchedule() bool {
	// Decrement remaining occurrences if applicable
	if st.OccurrencesRemaining.Valid {
		st.OccurrencesRemaining.Int64--
		if st.OccurrencesRemaining.Int64 <= 0 {
			st.Touch()
			return false
		}
	}

	// Calculate next date
	nextDate := st.CalculateNextDate()

	// Check if past end date
	if st.EndDate.Valid && nextDate.After(st.EndDate.Date) {
		st.Touch()
		return false
	}

	st.NextDate = nextDate
	st.Touch()
	return true
}

// calculateNextDate calculates the next occurrence date based on frequency and settings.
func calculateNextDate(current Date, freq Frequency, interval int, dayOfMonth, dayOfWeek NullableInt) Date {
	if interval < 1 {
		interval = 1
	}

	currentTime := current.Time()

	switch freq {
	case FrequencyDaily:
		return Date(currentTime.AddDate(0, 0, interval))

	case FrequencyWeekly:
		return Date(currentTime.AddDate(0, 0, interval*7))

	case FrequencyBiweekly:
		return Date(currentTime.AddDate(0, 0, 14))

	case FrequencyMonthly:
		return addMonthsWithDayHandling(currentTime, interval, dayOfMonth)

	case FrequencyQuarterly:
		return addMonthsWithDayHandling(currentTime, 3, dayOfMonth)

	case FrequencyYearly:
		return Date(currentTime.AddDate(interval, 0, 0))

	default:
		// Fallback: add interval days
		return Date(currentTime.AddDate(0, 0, interval))
	}
}

// addMonthsWithDayHandling adds months while handling day-of-month edge cases.
func addMonthsWithDayHandling(t time.Time, months int, dayOfMonth NullableInt) Date {
	// Calculate target year and month
	targetYear := t.Year()
	targetMonth := int(t.Month()) + months

	// Normalize month overflow
	for targetMonth > 12 {
		targetMonth -= 12
		targetYear++
	}

	if !dayOfMonth.Valid {
		// No specific day set - use Go's AddDate and let it handle the day
		return Date(t.AddDate(0, months, 0))
	}

	day := int(dayOfMonth.Int64)

	// Handle -1 (last day of month)
	if day == -1 {
		return lastDayOfMonth(time.Date(targetYear, time.Month(targetMonth), 1, 0, 0, 0, 0, time.UTC))
	}

	// Handle specific day with month-end adjustment
	return adjustDayOfMonth(time.Date(targetYear, time.Month(targetMonth), 1, 0, 0, 0, 0, time.UTC), day)
}

// lastDayOfMonth returns the last day of the month for the given time.
func lastDayOfMonth(t time.Time) Date {
	// Go to first day of next month, then subtract one day
	firstOfNextMonth := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	lastDay := firstOfNextMonth.AddDate(0, 0, -1)
	return Date(lastDay)
}

// adjustDayOfMonth adjusts the day to fit within the month.
// If day > days in month, uses last day of month.
func adjustDayOfMonth(t time.Time, day int) Date {
	year := t.Year()
	month := t.Month()

	// Find last day of target month
	lastDayTime := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC)
	lastDay := lastDayTime.Day()

	if day > lastDay {
		day = lastDay
	}

	return Date(time.Date(year, month, day, 0, 0, 0, 0, time.UTC))
}

// Validate validates the scheduled transaction and returns any validation errors.
func (st *ScheduledTransaction) Validate() ValidationErrors {
	v := NewValidator()

	// Required fields
	v.RequiredID("account_id", st.AccountID)
	v.RequiredDate("start_date", st.StartDate)
	v.RequiredDate("next_date", st.NextDate)

	// Frequency validation
	if !st.Frequency.IsValid() {
		v.errors.Add("frequency", "must be a valid frequency (daily, weekly, biweekly, monthly, quarterly, or yearly)")
	}

	// Interval must be positive
	if st.Interval < 1 {
		v.errors.Add("interval", "must be at least 1")
	}

	// end_date must be after start_date if set
	if st.EndDate.Valid && !st.EndDate.Date.IsZero() {
		if !st.EndDate.Date.After(st.StartDate) {
			v.errors.Add("end_date", "must be after start_date")
		}
	}

	// occurrences must be positive if set
	if st.Occurrences.Valid && st.Occurrences.Int64 <= 0 {
		v.errors.Add("occurrences", "must be positive")
	}

	// Cannot have both end_date and occurrences
	if st.EndDate.Valid && st.Occurrences.Valid {
		v.errors.Add("duration", "cannot have both end_date and occurrences; use one or the other")
	}

	// day_of_month validation: 1-31 or -1
	if st.DayOfMonth.Valid {
		dom := st.DayOfMonth.Int64
		if dom < -1 || dom == 0 || dom > 31 {
			v.errors.Add("day_of_month", "must be 1-31 or -1 for last day of month")
		}
	}

	// day_of_week validation: 0-6
	if st.DayOfWeek.Valid {
		dow := st.DayOfWeek.Int64
		if dow < 0 || dow > 6 {
			v.errors.Add("day_of_week", "must be 0-6 (Sunday=0, Saturday=6)")
		}
	}

	// amount_estimate_count must be positive if set
	if st.AmountEstimateCount.Valid && st.AmountEstimateCount.Int64 <= 0 {
		v.errors.Add("amount_estimate_count", "must be positive")
	}

	// occurrences_remaining cannot exceed occurrences
	if st.Occurrences.Valid && st.OccurrencesRemaining.Valid {
		if st.OccurrencesRemaining.Int64 > st.Occurrences.Int64 {
			v.errors.Add("occurrences_remaining", "cannot exceed occurrences")
		}
	}

	// Optional field length limits
	if st.Memo.Valid {
		v.MaxLength("memo", st.Memo.String, 1000)
	}

	return v.Errors()
}

// IsValid returns true if the scheduled transaction passes validation.
func (st *ScheduledTransaction) IsValid() bool {
	return !st.Validate().HasErrors()
}
