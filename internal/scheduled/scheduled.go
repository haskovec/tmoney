package scheduled

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

// Frequency represents how often a scheduled transaction repeats.
type Frequency string

const (
	FrequencyDaily       Frequency = "daily"
	FrequencyWeekly      Frequency = "weekly"
	FrequencyFortnightly Frequency = "fortnightly"
	FrequencySemiMonthly Frequency = "semimonthly"
	FrequencyMonthly     Frequency = "monthly"
	FrequencyQuarterly   Frequency = "quarterly"
	FrequencyYearly      Frequency = "yearly"
)

// AllFrequencies returns all valid frequencies.
func AllFrequencies() []Frequency {
	return []Frequency{
		FrequencyDaily,
		FrequencyWeekly,
		FrequencyFortnightly,
		FrequencySemiMonthly,
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
	case FrequencyDaily, FrequencyWeekly, FrequencyFortnightly, FrequencySemiMonthly,
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
	case FrequencyFortnightly:
		return "Fortnightly"
	case FrequencySemiMonthly:
		return "Semi-Monthly"
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

// Transaction represents a recurring or future transaction.
type Transaction struct {
	types.BaseModel

	// Core properties (required)
	AccountID types.ID   `json:"account_id"`
	Frequency Frequency  `json:"frequency"`
	StartDate types.Date `json:"start_date"`
	NextDate  types.Date `json:"next_date"`

	// Schedule properties
	Interval             int                `json:"interval"`               // Every N periods (default: 1)
	EndDate              types.NullableDate `json:"end_date"`               // When schedule ends (null = indefinite)
	Occurrences          types.NullableInt  `json:"occurrences"`            // Total number of times to repeat
	OccurrencesRemaining types.NullableInt  `json:"occurrences_remaining"`  // Countdown for fixed occurrences
	DayOfMonth           types.NullableInt  `json:"day_of_month"`           // Specific day (1-31, or -1 for last day)
	SecondaryDayOfMonth  types.NullableInt  `json:"secondary_day_of_month"` // Second day for semi-monthly cadence (1-31, or -1 for last day)
	DayOfWeek            types.NullableInt  `json:"day_of_week"`            // Day of week (0=Sunday, 6=Saturday)

	// Transaction template properties
	PayeeID    types.NullableID     `json:"payee_id"`
	CategoryID types.NullableID     `json:"category_id"`
	Amount     types.NullableMoney  `json:"amount"` // Null if variable amount
	Memo       types.NullableString `json:"memo"`

	// TransferAccountID, when set, marks this as a single-line transfer
	// schedule: account_id is the source ("From") and TransferAccountID is
	// the destination ("To"). Mutually exclusive with CategoryID and with
	// multi-line Splits. Posting creates a clean linked transfer pair.
	TransferAccountID types.NullableID `json:"transfer_account_id"`

	// Variable amount estimation
	AmountEstimateCount types.NullableInt `json:"amount_estimate_count"` // Use average of last N transactions

	// Auto-post properties
	AutoPost     bool `json:"auto_post"`      // Whether to automatically post when due
	PostLeadDays int  `json:"post_lead_days"` // Days before due date to post (0, 3, or 7)

	// Multi-line template children. Empty for legacy single-line schedules.
	// Populated by the repository when loading; persisted via SplitRepository.
	Splits SplitCollection `json:"splits,omitempty"`
}

// NewTransaction creates a new Transaction with required properties.
func NewTransaction(accountID types.ID, frequency Frequency, startDate types.Date) *Transaction {
	return &Transaction{
		BaseModel: types.NewBaseModel(),
		AccountID: accountID,
		Frequency: frequency,
		StartDate: startDate,
		NextDate:  startDate,
		Interval:  1,
	}
}

// NewTransactionWithAmount creates a new Transaction with an amount.
func NewTransactionWithAmount(accountID types.ID, frequency Frequency, startDate types.Date, amount types.Money) *Transaction {
	st := NewTransaction(accountID, frequency, startDate)
	st.Amount = types.NullableMoney{Money: amount, Valid: true}
	return st
}

// NewTransactionFull creates a new Transaction with all common properties.
func NewTransactionFull(
	accountID types.ID,
	frequency Frequency,
	startDate types.Date,
	amount types.Money,
	payeeID, categoryID types.ID,
	memo string,
) *Transaction {
	st := NewTransactionWithAmount(accountID, frequency, startDate, amount)
	if !payeeID.IsNil() {
		st.PayeeID = types.NullableID{ID: payeeID, Valid: true}
	}
	if !categoryID.IsNil() {
		st.CategoryID = types.NullableID{ID: categoryID, Valid: true}
	}
	if memo != "" {
		st.Memo = types.NullableString{String: memo, Valid: true}
	}
	return st
}

// SetPayee sets the payee for this scheduled transaction.
func (st *Transaction) SetPayee(payeeID types.ID) {
	st.PayeeID = types.NullableID{ID: payeeID, Valid: true}
	st.Touch()
}

// ClearPayee removes the payee from this scheduled transaction.
func (st *Transaction) ClearPayee() {
	st.PayeeID = types.NullableID{Valid: false}
	st.Touch()
}

// HasPayee returns true if the scheduled transaction has a payee set.
func (st *Transaction) HasPayee() bool {
	return st.PayeeID.Valid
}

// SetCategory sets the category for this scheduled transaction.
func (st *Transaction) SetCategory(categoryID types.ID) {
	st.CategoryID = types.NullableID{ID: categoryID, Valid: true}
	st.Touch()
}

// ClearCategory removes the category from this scheduled transaction.
func (st *Transaction) ClearCategory() {
	st.CategoryID = types.NullableID{Valid: false}
	st.Touch()
}

// HasCategory returns true if the scheduled transaction has a category set.
func (st *Transaction) HasCategory() bool {
	return st.CategoryID.Valid
}

// SetTransfer marks this as a single-line transfer schedule whose destination
// ("To") is transferAccountID. The source ("From") is the schedule's own
// AccountID. Clears any category, since the two shapes are mutually exclusive.
func (st *Transaction) SetTransfer(transferAccountID types.ID) {
	st.TransferAccountID = types.NullableID{ID: transferAccountID, Valid: true}
	st.CategoryID = types.NullableID{Valid: false}
	st.Touch()
}

// ClearTransfer removes the transfer destination.
func (st *Transaction) ClearTransfer() {
	st.TransferAccountID = types.NullableID{Valid: false}
	st.Touch()
}

// IsTransfer returns true if this is a single-line transfer schedule.
func (st *Transaction) IsTransfer() bool {
	return st.TransferAccountID.Valid
}

// SetAmount sets the amount for this scheduled transaction.
func (st *Transaction) SetAmount(amount types.Money) {
	st.Amount = types.NullableMoney{Money: amount, Valid: true}
	st.Touch()
}

// ClearAmount removes the amount (marks as variable).
func (st *Transaction) ClearAmount() {
	st.Amount = types.NullableMoney{Valid: false}
	st.Touch()
}

// HasAmount returns true if the scheduled transaction has a fixed amount.
func (st *Transaction) HasAmount() bool {
	return st.Amount.Valid
}

// IsVariableAmount returns true if the amount is variable (not fixed).
func (st *Transaction) IsVariableAmount() bool {
	return !st.Amount.Valid
}

// SetMemo sets the memo for this scheduled transaction.
func (st *Transaction) SetMemo(memo string) {
	if memo == "" {
		st.Memo = types.NullableString{Valid: false}
	} else {
		st.Memo = types.NullableString{String: memo, Valid: true}
	}
	st.Touch()
}

// ClearMemo removes the memo from this scheduled transaction.
func (st *Transaction) ClearMemo() {
	st.Memo = types.NullableString{Valid: false}
	st.Touch()
}

// SetEndDate sets an end date for the schedule.
func (st *Transaction) SetEndDate(endDate types.Date) {
	st.EndDate = types.NullableDate{Date: endDate, Valid: true}
	// Clear occurrences if end date is set
	st.Occurrences = types.NullableInt{Valid: false}
	st.OccurrencesRemaining = types.NullableInt{Valid: false}
	st.Touch()
}

// ClearEndDate removes the end date (makes schedule indefinite).
func (st *Transaction) ClearEndDate() {
	st.EndDate = types.NullableDate{Valid: false}
	st.Touch()
}

// SetOccurrences sets a fixed number of occurrences.
func (st *Transaction) SetOccurrences(count int64) {
	st.Occurrences = types.NullableInt{Int64: count, Valid: true}
	st.OccurrencesRemaining = types.NullableInt{Int64: count, Valid: true}
	// Clear end date if occurrences is set
	st.EndDate = types.NullableDate{Valid: false}
	st.Touch()
}

// ClearOccurrences removes the occurrence limit (makes schedule indefinite).
func (st *Transaction) ClearOccurrences() {
	st.Occurrences = types.NullableInt{Valid: false}
	st.OccurrencesRemaining = types.NullableInt{Valid: false}
	st.Touch()
}

// SetDayOfMonth sets the specific day of month for monthly/quarterly/yearly schedules.
// Use -1 for last day of month.
func (st *Transaction) SetDayOfMonth(day int) {
	st.DayOfMonth = types.NullableInt{Int64: int64(day), Valid: true}
	st.Touch()
}

// ClearDayOfMonth removes the specific day of month.
func (st *Transaction) ClearDayOfMonth() {
	st.DayOfMonth = types.NullableInt{Valid: false}
	st.Touch()
}

// SetDayOfWeek sets the specific day of week for weekly schedules.
// 0 = Sunday, 6 = Saturday.
func (st *Transaction) SetDayOfWeek(day int) {
	st.DayOfWeek = types.NullableInt{Int64: int64(day), Valid: true}
	st.Touch()
}

// ClearDayOfWeek removes the specific day of week.
func (st *Transaction) ClearDayOfWeek() {
	st.DayOfWeek = types.NullableInt{Valid: false}
	st.Touch()
}

// SetAmountEstimateCount sets the number of past transactions to average for estimates.
func (st *Transaction) SetAmountEstimateCount(count int) {
	st.AmountEstimateCount = types.NullableInt{Int64: int64(count), Valid: true}
	st.Touch()
}

// ClearAmountEstimateCount removes the amount estimation.
func (st *Transaction) ClearAmountEstimateCount() {
	st.AmountEstimateCount = types.NullableInt{Valid: false}
	st.Touch()
}

// SetAutoPost enables or disables auto-posting for this scheduled transaction.
func (st *Transaction) SetAutoPost(autoPost bool) {
	st.AutoPost = autoPost
	st.Touch()
}

// SetPostLeadDays sets the number of days before the due date to auto-post.
// Valid values are 0, 3, or 7.
func (st *Transaction) SetPostLeadDays(days int) {
	st.PostLeadDays = days
	st.Touch()
}

// IsAutoPost returns true if the scheduled transaction is set to auto-post.
func (st *Transaction) IsAutoPost() bool {
	return st.AutoPost
}

// SetInterval sets the interval between occurrences.
func (st *Transaction) SetInterval(interval int) {
	if interval < 1 {
		interval = 1
	}
	st.Interval = interval
	st.Touch()
}

// IsIndefinite returns true if the schedule has no end date or occurrence limit.
func (st *Transaction) IsIndefinite() bool {
	return !st.EndDate.Valid && !st.Occurrences.Valid
}

// IsDue returns true if the next date is today or in the past.
func (st *Transaction) IsDue() bool {
	today := types.Today()
	return !st.NextDate.After(today)
}

// IsCompleted returns true if the schedule has finished all occurrences.
func (st *Transaction) IsCompleted() bool {
	if st.OccurrencesRemaining.Valid && st.OccurrencesRemaining.Int64 <= 0 {
		return true
	}
	if st.EndDate.Valid && st.NextDate.After(st.EndDate.Date) {
		return true
	}
	return false
}

// MarkCompleted forces the schedule into the terminal "completed" state that a
// naturally-exhausted fixed-duration schedule reaches: occurrences_remaining =
// 0, with occurrences backfilled to a positive value so validation
// (occurrences > 0; remaining ≤ occurrences) still holds on an otherwise
// indefinite schedule. Any end_date is cleared so it cannot collide with the
// occurrence limit (they are mutually exclusive in validation). This is the
// mechanism the loan-payoff completion uses — deliberately not the end_date
// trick, which strands NextDate == EndDate and can violate the
// end_date > start_date rule on a first-occurrence payoff.
func (st *Transaction) MarkCompleted() {
	occurrences := int64(1)
	if st.Occurrences.Valid && st.Occurrences.Int64 > occurrences {
		occurrences = st.Occurrences.Int64
	}
	st.Occurrences = types.NullableInt{Int64: occurrences, Valid: true}
	st.OccurrencesRemaining = types.NullableInt{Int64: 0, Valid: true}
	st.EndDate = types.NullableDate{Valid: false}
	st.Touch()
}

// CalculateNextDate calculates the next occurrence date after the current next_date.
func (st *Transaction) CalculateNextDate() types.Date {
	return calculateNextDate(st.NextDate, st.Frequency, st.Interval, st.DayOfMonth, st.SecondaryDayOfMonth)
}

// AdvanceSchedule advances the schedule to the next occurrence.
// Returns false if the schedule is completed.
func (st *Transaction) AdvanceSchedule() bool {
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
func calculateNextDate(current types.Date, freq Frequency, interval int, dayOfMonth, secondaryDayOfMonth types.NullableInt) types.Date {
	if interval < 1 {
		interval = 1
	}

	currentTime := current.Time()

	switch freq {
	case FrequencyDaily:
		return types.Date(currentTime.AddDate(0, 0, interval))

	case FrequencyWeekly:
		return types.Date(currentTime.AddDate(0, 0, interval*7))

	case FrequencyFortnightly:
		return types.Date(currentTime.AddDate(0, 0, 14))

	case FrequencySemiMonthly:
		return addSemiMonthly(currentTime, dayOfMonth, secondaryDayOfMonth)

	case FrequencyMonthly:
		return addMonthsWithDayHandling(currentTime, interval, dayOfMonth)

	case FrequencyQuarterly:
		return addMonthsWithDayHandling(currentTime, 3, dayOfMonth)

	case FrequencyYearly:
		return types.Date(currentTime.AddDate(interval, 0, 0))

	default:
		// Fallback: add interval days
		return types.Date(currentTime.AddDate(0, 0, interval))
	}
}

// addSemiMonthly advances a semi-monthly cadence to the next pay date.
// dayOfMonth and secondaryDayOfMonth are the two pay days (1-31, or -1
// for "last day of month"). If only one is set, falls back to monthly.
//
// Algorithm: of the two pay days, find the next chronological one after
// the current date. If the current date is on or after both this month's
// pay days, roll to the earlier day of next month.
func addSemiMonthly(current time.Time, dayOfMonth, secondaryDayOfMonth types.NullableInt) types.Date {
	if !dayOfMonth.Valid {
		// Defensive: schedule was marked semi-monthly but no day stored.
		// Advance by half a month as a sane fallback.
		return types.Date(current.AddDate(0, 0, 15))
	}
	d1 := int(dayOfMonth.Int64)
	if !secondaryDayOfMonth.Valid {
		// Treat as monthly.
		return addMonthsWithDayHandling(current, 1, dayOfMonth)
	}
	d2 := int(secondaryDayOfMonth.Int64)

	// Resolve each pay day to an actual day-of-month for `current`'s month
	// and next month. -1 means "last day of month".
	year := current.Year()
	month := current.Month()
	lastDayThis := lastDayOf(year, month)
	nextMonth := month + 1
	nextYear := year
	if nextMonth > 12 {
		nextMonth -= 12
		nextYear++
	}
	lastDayNext := lastDayOf(nextYear, nextMonth)

	resolveDay := func(d int, lastDay int) int {
		if d == -1 || d > lastDay {
			return lastDay
		}
		if d < 1 {
			return 1
		}
		return d
	}

	thisD1 := resolveDay(d1, lastDayThis)
	thisD2 := resolveDay(d2, lastDayThis)
	nextD1 := resolveDay(d1, lastDayNext)
	nextD2 := resolveDay(d2, lastDayNext)

	// Build candidate dates in this month and next month, then pick the
	// soonest candidate strictly after `current`.
	type candidate struct {
		y int
		m time.Month
		d int
	}
	cands := []candidate{
		{year, month, thisD1},
		{year, month, thisD2},
		{nextYear, nextMonth, nextD1},
		{nextYear, nextMonth, nextD2},
	}
	currentY, currentM, currentD := current.Date()
	var best *candidate
	for i := range cands {
		c := cands[i]
		if c.y < currentY {
			continue
		}
		if c.y == currentY && c.m < currentM {
			continue
		}
		if c.y == currentY && c.m == currentM && c.d <= currentD {
			continue
		}
		if best == nil || c.y < best.y ||
			(c.y == best.y && c.m < best.m) ||
			(c.y == best.y && c.m == best.m && c.d < best.d) {
			best = &c
		}
	}
	if best == nil {
		// Defensive: shouldn't happen, but advance by 15 days.
		return types.Date(current.AddDate(0, 0, 15))
	}
	return types.NewDate(best.y, best.m, best.d)
}

// lastDayOf returns the last day number of the given month/year.
func lastDayOf(year int, month time.Month) int {
	// Day 0 of next month = last day of this month.
	t := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC)
	return t.Day()
}

// addMonthsWithDayHandling adds months while handling day-of-month edge cases.
func addMonthsWithDayHandling(t time.Time, months int, dayOfMonth types.NullableInt) types.Date {
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
		return types.Date(t.AddDate(0, months, 0))
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
func lastDayOfMonth(t time.Time) types.Date {
	// Go to first day of next month, then subtract one day
	firstOfNextMonth := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	lastDay := firstOfNextMonth.AddDate(0, 0, -1)
	return types.Date(lastDay)
}

// adjustDayOfMonth adjusts the day to fit within the month.
// If day > days in month, uses last day of month.
func adjustDayOfMonth(t time.Time, day int) types.Date {
	year := t.Year()
	month := t.Month()

	// Find last day of target month
	lastDayTime := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC)
	lastDay := lastDayTime.Day()

	if day > lastDay {
		day = lastDay
	}

	return types.Date(time.Date(year, month, day, 0, 0, 0, 0, time.UTC))
}

// Validate validates the scheduled transaction and returns any validation errors.
func (st *Transaction) Validate() types.ValidationErrors {
	v := types.NewValidator()

	// Required fields
	v.RequiredID("account_id", st.AccountID)
	v.RequiredDate("start_date", st.StartDate)
	v.RequiredDate("next_date", st.NextDate)

	// Frequency validation
	if !st.Frequency.IsValid() {
		v.AddError("frequency", "must be a valid frequency (daily, weekly, fortnightly, semimonthly, monthly, quarterly, or yearly)")
	}

	// Interval must be positive
	if st.Interval < 1 {
		v.AddError("interval", "must be at least 1")
	}

	// end_date must be after start_date if set
	if st.EndDate.Valid && !st.EndDate.Date.IsZero() {
		if !st.EndDate.Date.After(st.StartDate) {
			v.AddError("end_date", "must be after start_date")
		}
	}

	// occurrences must be positive if set
	if st.Occurrences.Valid && st.Occurrences.Int64 <= 0 {
		v.AddError("occurrences", "must be positive")
	}

	// Cannot have both end_date and occurrences
	if st.EndDate.Valid && st.Occurrences.Valid {
		v.AddError("duration", "cannot have both end_date and occurrences; use one or the other")
	}

	// day_of_month validation: 1-31 or -1
	if st.DayOfMonth.Valid {
		dom := st.DayOfMonth.Int64
		if dom < -1 || dom == 0 || dom > 31 {
			v.AddError("day_of_month", "must be 1-31 or -1 for last day of month")
		}
	}

	// day_of_week validation: 0-6
	if st.DayOfWeek.Valid {
		dow := st.DayOfWeek.Int64
		if dow < 0 || dow > 6 {
			v.AddError("day_of_week", "must be 0-6 (Sunday=0, Saturday=6)")
		}
	}

	// amount_estimate_count must be positive if set
	if st.AmountEstimateCount.Valid && st.AmountEstimateCount.Int64 <= 0 {
		v.AddError("amount_estimate_count", "must be positive")
	}

	// post_lead_days must be 0, 3, or 7
	if st.PostLeadDays != 0 && st.PostLeadDays != 3 && st.PostLeadDays != 7 {
		v.AddError("post_lead_days", "must be 0, 3, or 7")
	}

	// occurrences_remaining cannot exceed occurrences
	if st.Occurrences.Valid && st.OccurrencesRemaining.Valid {
		if st.OccurrencesRemaining.Int64 > st.Occurrences.Int64 {
			v.AddError("occurrences_remaining", "cannot exceed occurrences")
		}
	}

	// Optional field length limits
	if st.Memo.Valid {
		v.MaxLength("memo", st.Memo.String, 1000)
	}

	// Single-line transfer invariants.
	if st.TransferAccountID.Valid {
		// Mutually exclusive with a scalar category and with multi-line splits.
		if st.CategoryID.Valid {
			v.AddError("transfer_account_id", "a transfer schedule cannot also set a category")
		}
		if len(st.Splits) > 0 {
			v.AddError("transfer_account_id", "a transfer schedule cannot also have split lines")
		}
		// Cannot transfer to the source account.
		if st.TransferAccountID.ID == st.AccountID {
			v.AddError("transfer_account_id", "cannot transfer to the same account")
		}
		// A transfer always carries a fixed (estimated) amount.
		if !st.Amount.Valid {
			v.AddError("amount", "a transfer schedule requires an amount")
		}
	}

	return v.Errors()
}

// IsValid returns true if the scheduled transaction passes validation.
func (st *Transaction) IsValid() bool {
	return !st.Validate().HasErrors()
}
