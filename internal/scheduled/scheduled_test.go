package scheduled

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

func TestFrequency(t *testing.T) {
	t.Run("AllFrequencies returns all frequencies", func(t *testing.T) {
		frequencies := AllFrequencies()
		expected := 7
		if len(frequencies) != expected {
			t.Errorf("Expected %d frequencies, got %d", expected, len(frequencies))
		}
	})

	t.Run("String returns correct value", func(t *testing.T) {
		tests := []struct {
			freq     Frequency
			expected string
		}{
			{FrequencyDaily, "daily"},
			{FrequencyWeekly, "weekly"},
			{FrequencyFortnightly, "fortnightly"},
			{FrequencySemiMonthly, "semimonthly"},
			{FrequencyMonthly, "monthly"},
			{FrequencyQuarterly, "quarterly"},
			{FrequencyYearly, "yearly"},
		}
		for _, tc := range tests {
			if tc.freq.String() != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, tc.freq.String())
			}
		}
	})

	t.Run("IsValid returns true for valid frequencies", func(t *testing.T) {
		for _, f := range AllFrequencies() {
			if !f.IsValid() {
				t.Errorf("IsValid should return true for %q", f)
			}
		}
	})

	t.Run("IsValid returns false for invalid frequency", func(t *testing.T) {
		invalid := Frequency("every_second")
		if invalid.IsValid() {
			t.Error("IsValid should return false for invalid frequency")
		}
	})

	t.Run("DisplayName returns human-readable names", func(t *testing.T) {
		tests := []struct {
			freq     Frequency
			expected string
		}{
			{FrequencyDaily, "Daily"},
			{FrequencyWeekly, "Weekly"},
			{FrequencyFortnightly, "Fortnightly"},
			{FrequencySemiMonthly, "Semi-Monthly"},
			{FrequencyMonthly, "Monthly"},
			{FrequencyQuarterly, "Quarterly"},
			{FrequencyYearly, "Yearly"},
		}
		for _, tc := range tests {
			if tc.freq.DisplayName() != tc.expected {
				t.Errorf("DisplayName for %q: expected %q, got %q",
					tc.freq, tc.expected, tc.freq.DisplayName())
			}
		}
	})

	t.Run("DisplayName returns raw string for unknown frequency", func(t *testing.T) {
		unknown := Frequency("unknown")
		if unknown.DisplayName() != "unknown" {
			t.Errorf("Expected 'unknown', got %q", unknown.DisplayName())
		}
	})
}

func TestParseFrequency(t *testing.T) {
	t.Run("Parses valid frequencies", func(t *testing.T) {
		tests := []struct {
			input    string
			expected Frequency
		}{
			{"daily", FrequencyDaily},
			{"weekly", FrequencyWeekly},
			{"fortnightly", FrequencyFortnightly},
			{"semimonthly", FrequencySemiMonthly},
			{"monthly", FrequencyMonthly},
			{"quarterly", FrequencyQuarterly},
			{"yearly", FrequencyYearly},
		}
		for _, tc := range tests {
			f, err := ParseFrequency(tc.input)
			if err != nil {
				t.Errorf("Unexpected error for %q: %v", tc.input, err)
			}
			if f != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, f)
			}
		}
	})

	t.Run("Parses uppercase frequency", func(t *testing.T) {
		f, err := ParseFrequency("MONTHLY")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if f != FrequencyMonthly {
			t.Errorf("Expected FrequencyMonthly, got %q", f)
		}
	})

	t.Run("Parses mixed case frequency", func(t *testing.T) {
		f, err := ParseFrequency("Weekly")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if f != FrequencyWeekly {
			t.Errorf("Expected FrequencyWeekly, got %q", f)
		}
	})

	t.Run("Returns error for invalid frequency", func(t *testing.T) {
		_, err := ParseFrequency("invalid")
		if err == nil {
			t.Error("Expected error for invalid frequency")
		}
	})

	t.Run("Returns error for empty string", func(t *testing.T) {
		_, err := ParseFrequency("")
		if err == nil {
			t.Error("Expected error for empty string")
		}
	})
}

func TestFrequencyScanValue(t *testing.T) {
	t.Run("Value returns string", func(t *testing.T) {
		f := FrequencyMonthly
		v, err := f.Value()
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if v != "monthly" {
			t.Errorf("Expected 'monthly', got %v", v)
		}
	})

	t.Run("Scan from string", func(t *testing.T) {
		var f Frequency
		err := f.Scan("weekly")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if f != FrequencyWeekly {
			t.Errorf("Expected FrequencyWeekly, got %q", f)
		}
	})

	t.Run("Scan from bytes", func(t *testing.T) {
		var f Frequency
		err := f.Scan([]byte("daily"))
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if f != FrequencyDaily {
			t.Errorf("Expected FrequencyDaily, got %q", f)
		}
	})

	t.Run("Scan from nil", func(t *testing.T) {
		var f Frequency
		err := f.Scan(nil)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if f != "" {
			t.Errorf("Expected empty string, got %q", f)
		}
	})

	t.Run("Scan from unsupported type returns error", func(t *testing.T) {
		var f Frequency
		err := f.Scan(123)
		if err == nil {
			t.Error("Expected error for unsupported type")
		}
	})
}

func TestNewTransaction(t *testing.T) {
	t.Run("Creates scheduled transaction with required properties", func(t *testing.T) {
		accountID := types.NewID()
		startDate := types.NewDate(2024, time.January, 15)

		st := NewTransaction(accountID, FrequencyMonthly, startDate)

		if st.ID.IsNil() {
			t.Error("NewTransaction should create non-nil ID")
		}
		if st.AccountID != accountID {
			t.Errorf("Expected account ID %s, got %s", accountID.String(), st.AccountID.String())
		}
		if st.Frequency != FrequencyMonthly {
			t.Errorf("Expected frequency 'monthly', got %s", st.Frequency.String())
		}
		if !st.StartDate.Equal(startDate) {
			t.Errorf("Expected start date %s, got %s", startDate.String(), st.StartDate.String())
		}
		if !st.NextDate.Equal(startDate) {
			t.Errorf("Expected next date to equal start date, got %s", st.NextDate.String())
		}
		if st.Interval != 1 {
			t.Errorf("Expected interval 1, got %d", st.Interval)
		}
		if st.Amount.Valid {
			t.Error("Amount should not be set")
		}
		if st.PayeeID.Valid {
			t.Error("PayeeID should not be set")
		}
		if st.CategoryID.Valid {
			t.Error("CategoryID should not be set")
		}
		if st.Memo.Valid {
			t.Error("Memo should not be set")
		}
		if st.EndDate.Valid {
			t.Error("EndDate should not be set")
		}
		if st.Occurrences.Valid {
			t.Error("Occurrences should not be set")
		}
		if st.AutoPost {
			t.Error("AutoPost should default to false")
		}
		if st.PostLeadDays != 0 {
			t.Errorf("PostLeadDays should default to 0, got %d", st.PostLeadDays)
		}
		if st.CreatedAt.IsZero() {
			t.Error("CreatedAt should be set")
		}
		if st.UpdatedAt.IsZero() {
			t.Error("UpdatedAt should be set")
		}
	})
}

func TestNewTransactionWithAmount(t *testing.T) {
	t.Run("Creates scheduled transaction with amount", func(t *testing.T) {
		accountID := types.NewID()
		startDate := types.NewDate(2024, time.January, 15)
		amount := types.MustNewMoney("-1500.00")

		st := NewTransactionWithAmount(accountID, FrequencyMonthly, startDate, amount)

		if !st.Amount.Valid {
			t.Error("Amount should be set")
		}
		if !st.Amount.Money.Equal(amount) {
			t.Errorf("Expected amount %s, got %s", amount.String(), st.Amount.Money.String())
		}
	})
}

func TestNewTransactionFull(t *testing.T) {
	t.Run("Creates scheduled transaction with all properties", func(t *testing.T) {
		accountID := types.NewID()
		payeeID := types.NewID()
		categoryID := types.NewID()
		startDate := types.NewDate(2024, time.January, 15)
		amount := types.MustNewMoney("-1500.00")
		memo := "Monthly rent"

		st := NewTransactionFull(accountID, FrequencyMonthly, startDate, amount, payeeID, categoryID, memo)

		if st.AccountID != accountID {
			t.Errorf("Expected account ID %s, got %s", accountID.String(), st.AccountID.String())
		}
		if !st.PayeeID.Valid || st.PayeeID.ID != payeeID {
			t.Error("PayeeID should be set correctly")
		}
		if !st.CategoryID.Valid || st.CategoryID.ID != categoryID {
			t.Error("CategoryID should be set correctly")
		}
		if !st.Amount.Valid || !st.Amount.Money.Equal(amount) {
			t.Errorf("Expected amount %s", amount.String())
		}
		if !st.Memo.Valid || st.Memo.String != memo {
			t.Errorf("Expected memo %q, got %q", memo, st.Memo.String)
		}
	})

	t.Run("Handles nil payee and category", func(t *testing.T) {
		accountID := types.NewID()
		startDate := types.NewDate(2024, time.January, 15)
		amount := types.MustNewMoney("-50.00")

		st := NewTransactionFull(accountID, FrequencyMonthly, startDate, amount, types.NilID, types.NilID, "")

		if st.PayeeID.Valid {
			t.Error("PayeeID should not be set for NilID")
		}
		if st.CategoryID.Valid {
			t.Error("CategoryID should not be set for NilID")
		}
		if st.Memo.Valid {
			t.Error("Memo should not be set for empty string")
		}
	})
}

func TestTransactionPayee(t *testing.T) {
	t.Run("SetPayee sets payee", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		payeeID := types.NewID()

		if st.HasPayee() {
			t.Error("Scheduled transaction should start without payee")
		}

		st.SetPayee(payeeID)

		if !st.HasPayee() {
			t.Error("HasPayee should return true after setting")
		}
		if st.PayeeID.ID != payeeID {
			t.Errorf("Expected payee ID %s, got %s", payeeID.String(), st.PayeeID.ID.String())
		}
	})

	t.Run("ClearPayee removes payee", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		st.SetPayee(types.NewID())

		st.ClearPayee()

		if st.HasPayee() {
			t.Error("HasPayee should return false after clearing")
		}
	})
}

func TestTransactionCategory(t *testing.T) {
	t.Run("SetCategory sets category", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		categoryID := types.NewID()

		if st.HasCategory() {
			t.Error("Scheduled transaction should start without category")
		}

		st.SetCategory(categoryID)

		if !st.HasCategory() {
			t.Error("HasCategory should return true after setting")
		}
		if st.CategoryID.ID != categoryID {
			t.Errorf("Expected category ID %s, got %s", categoryID.String(), st.CategoryID.ID.String())
		}
	})

	t.Run("ClearCategory removes category", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		st.SetCategory(types.NewID())

		st.ClearCategory()

		if st.HasCategory() {
			t.Error("HasCategory should return false after clearing")
		}
	})
}

func TestTransactionAmount(t *testing.T) {
	t.Run("SetAmount sets amount", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		amount := types.MustNewMoney("-100.00")

		if st.HasAmount() {
			t.Error("Scheduled transaction should start without amount")
		}
		if !st.IsVariableAmount() {
			t.Error("IsVariableAmount should return true when no amount set")
		}

		st.SetAmount(amount)

		if !st.HasAmount() {
			t.Error("HasAmount should return true after setting")
		}
		if st.IsVariableAmount() {
			t.Error("IsVariableAmount should return false after setting amount")
		}
		if !st.Amount.Money.Equal(amount) {
			t.Errorf("Expected amount %s, got %s", amount.String(), st.Amount.Money.String())
		}
	})

	t.Run("ClearAmount marks as variable", func(t *testing.T) {
		st := NewTransactionWithAmount(types.NewID(), FrequencyMonthly, types.Today(), types.MustNewMoney("-100.00"))

		st.ClearAmount()

		if st.HasAmount() {
			t.Error("HasAmount should return false after clearing")
		}
		if !st.IsVariableAmount() {
			t.Error("IsVariableAmount should return true after clearing")
		}
	})
}

func TestTransactionMemo(t *testing.T) {
	t.Run("SetMemo sets memo", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())

		st.SetMemo("Monthly rent")

		if !st.Memo.Valid {
			t.Error("Memo should be valid after setting")
		}
		if st.Memo.String != "Monthly rent" {
			t.Errorf("Expected memo 'Monthly rent', got %q", st.Memo.String)
		}
	})

	t.Run("SetMemo with empty string clears memo", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		st.SetMemo("Some memo")

		st.SetMemo("")

		if st.Memo.Valid {
			t.Error("Memo should not be valid after setting empty string")
		}
	})

	t.Run("ClearMemo removes memo", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		st.SetMemo("Some memo")

		st.ClearMemo()

		if st.Memo.Valid {
			t.Error("Memo should not be valid after clearing")
		}
	})
}

func TestTransactionEndDate(t *testing.T) {
	t.Run("SetEndDate sets end date and clears occurrences", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.NewDate(2024, time.January, 1))
		st.SetOccurrences(12)

		endDate := types.NewDate(2024, time.December, 31)
		st.SetEndDate(endDate)

		if !st.EndDate.Valid {
			t.Error("EndDate should be valid after setting")
		}
		if !st.EndDate.Date.Equal(endDate) {
			t.Errorf("Expected end date %s, got %s", endDate.String(), st.EndDate.Date.String())
		}
		if st.Occurrences.Valid {
			t.Error("Occurrences should be cleared when end date is set")
		}
	})

	t.Run("ClearEndDate removes end date", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		st.SetEndDate(types.NewDate(2025, time.December, 31))

		st.ClearEndDate()

		if st.EndDate.Valid {
			t.Error("EndDate should not be valid after clearing")
		}
	})
}

func TestTransactionOccurrences(t *testing.T) {
	t.Run("SetOccurrences sets occurrences and remaining", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.NewDate(2024, time.January, 1))
		st.SetEndDate(types.NewDate(2024, time.December, 31))

		st.SetOccurrences(60)

		if !st.Occurrences.Valid || st.Occurrences.Int64 != 60 {
			t.Errorf("Expected occurrences 60, got %d", st.Occurrences.Int64)
		}
		if !st.OccurrencesRemaining.Valid || st.OccurrencesRemaining.Int64 != 60 {
			t.Errorf("Expected occurrences_remaining 60, got %d", st.OccurrencesRemaining.Int64)
		}
		if st.EndDate.Valid {
			t.Error("EndDate should be cleared when occurrences is set")
		}
	})

	t.Run("ClearOccurrences removes occurrences", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		st.SetOccurrences(12)

		st.ClearOccurrences()

		if st.Occurrences.Valid {
			t.Error("Occurrences should not be valid after clearing")
		}
		if st.OccurrencesRemaining.Valid {
			t.Error("OccurrencesRemaining should not be valid after clearing")
		}
	})
}

func TestTransactionDaySettings(t *testing.T) {
	t.Run("SetDayOfMonth sets day of month", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())

		st.SetDayOfMonth(15)

		if !st.DayOfMonth.Valid || st.DayOfMonth.Int64 != 15 {
			t.Errorf("Expected day_of_month 15, got %d", st.DayOfMonth.Int64)
		}
	})

	t.Run("SetDayOfMonth with -1 for last day", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())

		st.SetDayOfMonth(-1)

		if !st.DayOfMonth.Valid || st.DayOfMonth.Int64 != -1 {
			t.Errorf("Expected day_of_month -1, got %d", st.DayOfMonth.Int64)
		}
	})

	t.Run("ClearDayOfMonth removes day of month", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		st.SetDayOfMonth(15)

		st.ClearDayOfMonth()

		if st.DayOfMonth.Valid {
			t.Error("DayOfMonth should not be valid after clearing")
		}
	})
}

func TestTransactionAmountEstimate(t *testing.T) {
	t.Run("SetAmountEstimateCount sets count", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())

		st.SetAmountEstimateCount(3)

		if !st.AmountEstimateCount.Valid || st.AmountEstimateCount.Int64 != 3 {
			t.Errorf("Expected amount_estimate_count 3, got %d", st.AmountEstimateCount.Int64)
		}
	})

	t.Run("ClearAmountEstimateCount removes count", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		st.SetAmountEstimateCount(3)

		st.ClearAmountEstimateCount()

		if st.AmountEstimateCount.Valid {
			t.Error("AmountEstimateCount should not be valid after clearing")
		}
	})
}

func TestTransactionAutoPost(t *testing.T) {
	t.Run("SetAutoPost enables auto-post", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())

		if st.IsAutoPost() {
			t.Error("Scheduled transaction should start with auto-post disabled")
		}

		st.SetAutoPost(true)

		if !st.IsAutoPost() {
			t.Error("IsAutoPost should return true after enabling")
		}
		if !st.AutoPost {
			t.Error("AutoPost should be true after enabling")
		}
	})

	t.Run("SetAutoPost disables auto-post", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		st.SetAutoPost(true)

		st.SetAutoPost(false)

		if st.IsAutoPost() {
			t.Error("IsAutoPost should return false after disabling")
		}
	})

	t.Run("SetPostLeadDays sets lead days", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())

		st.SetPostLeadDays(3)

		if st.PostLeadDays != 3 {
			t.Errorf("Expected post_lead_days 3, got %d", st.PostLeadDays)
		}
	})

	t.Run("SetPostLeadDays with 7 days", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())

		st.SetPostLeadDays(7)

		if st.PostLeadDays != 7 {
			t.Errorf("Expected post_lead_days 7, got %d", st.PostLeadDays)
		}
	})

	t.Run("SetAutoPost updates timestamp", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		original := st.UpdatedAt

		st.SetAutoPost(true)

		if !st.UpdatedAt.After(original) && !st.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("SetAutoPost should update UpdatedAt")
		}
	})

	t.Run("SetPostLeadDays updates timestamp", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		original := st.UpdatedAt

		st.SetPostLeadDays(3)

		if !st.UpdatedAt.After(original) && !st.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("SetPostLeadDays should update UpdatedAt")
		}
	})
}

func TestTransactionInterval(t *testing.T) {
	t.Run("SetInterval sets interval", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())

		st.SetInterval(2)

		if st.Interval != 2 {
			t.Errorf("Expected interval 2, got %d", st.Interval)
		}
	})

	t.Run("SetInterval with zero or negative sets to 1", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		st.SetInterval(5)

		st.SetInterval(0)
		if st.Interval != 1 {
			t.Errorf("Expected interval 1 for zero input, got %d", st.Interval)
		}

		st.SetInterval(-3)
		if st.Interval != 1 {
			t.Errorf("Expected interval 1 for negative input, got %d", st.Interval)
		}
	})
}

func TestTransactionState(t *testing.T) {
	t.Run("IsIndefinite returns true when no end date or occurrences", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())

		if !st.IsIndefinite() {
			t.Error("IsIndefinite should return true when no end date or occurrences")
		}
	})

	t.Run("IsIndefinite returns false when end date is set", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		st.SetEndDate(types.NewDate(2025, time.December, 31))

		if st.IsIndefinite() {
			t.Error("IsIndefinite should return false when end date is set")
		}
	})

	t.Run("IsIndefinite returns false when occurrences is set", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		st.SetOccurrences(12)

		if st.IsIndefinite() {
			t.Error("IsIndefinite should return false when occurrences is set")
		}
	})

	t.Run("IsDue returns true when next date is today or past", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())

		if !st.IsDue() {
			t.Error("IsDue should return true when next date is today")
		}

		st.NextDate = types.Today().AddDays(-1)
		if !st.IsDue() {
			t.Error("IsDue should return true when next date is in the past")
		}
	})

	t.Run("IsDue returns false when next date is in the future", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today().AddDays(7))

		if st.IsDue() {
			t.Error("IsDue should return false when next date is in the future")
		}
	})

	t.Run("IsCompleted returns true when occurrences remaining is zero", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		st.SetOccurrences(1)
		st.OccurrencesRemaining.Int64 = 0

		if !st.IsCompleted() {
			t.Error("IsCompleted should return true when occurrences remaining is zero")
		}
	})

	t.Run("IsCompleted returns true when next date is past end date", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.NewDate(2024, time.January, 1))
		st.SetEndDate(types.NewDate(2024, time.December, 31))
		st.NextDate = types.NewDate(2025, time.January, 1)

		if !st.IsCompleted() {
			t.Error("IsCompleted should return true when next date is past end date")
		}
	})

	t.Run("IsCompleted returns false for active indefinite schedule", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())

		if st.IsCompleted() {
			t.Error("IsCompleted should return false for active indefinite schedule")
		}
	})
}

func TestCalculateNextDate(t *testing.T) {
	t.Run("Daily adds interval days", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyDaily, types.NewDate(2024, time.January, 1))
		st.Interval = 1

		next := st.CalculateNextDate()

		expected := types.NewDate(2024, time.January, 2)
		if !next.Equal(expected) {
			t.Errorf("Expected %s, got %s", expected.String(), next.String())
		}
	})

	t.Run("Daily with interval 3", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyDaily, types.NewDate(2024, time.January, 1))
		st.Interval = 3

		next := st.CalculateNextDate()

		expected := types.NewDate(2024, time.January, 4)
		if !next.Equal(expected) {
			t.Errorf("Expected %s, got %s", expected.String(), next.String())
		}
	})

	t.Run("Weekly adds 7 days", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyWeekly, types.NewDate(2024, time.January, 1))

		next := st.CalculateNextDate()

		expected := types.NewDate(2024, time.January, 8)
		if !next.Equal(expected) {
			t.Errorf("Expected %s, got %s", expected.String(), next.String())
		}
	})

	t.Run("Weekly with interval 2", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyWeekly, types.NewDate(2024, time.January, 1))
		st.Interval = 2

		next := st.CalculateNextDate()

		expected := types.NewDate(2024, time.January, 15)
		if !next.Equal(expected) {
			t.Errorf("Expected %s, got %s", expected.String(), next.String())
		}
	})

	t.Run("Fortnightly adds 14 days", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyFortnightly, types.NewDate(2024, time.January, 5))

		next := st.CalculateNextDate()

		expected := types.NewDate(2024, time.January, 19)
		if !next.Equal(expected) {
			t.Errorf("Expected %s, got %s", expected.String(), next.String())
		}
	})

	t.Run("Monthly adds 1 month", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.NewDate(2024, time.January, 15))

		next := st.CalculateNextDate()

		expected := types.NewDate(2024, time.February, 15)
		if !next.Equal(expected) {
			t.Errorf("Expected %s, got %s", expected.String(), next.String())
		}
	})

	t.Run("Monthly with interval 2", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.NewDate(2024, time.January, 15))
		st.Interval = 2

		next := st.CalculateNextDate()

		expected := types.NewDate(2024, time.March, 15)
		if !next.Equal(expected) {
			t.Errorf("Expected %s, got %s", expected.String(), next.String())
		}
	})

	t.Run("Monthly handles month-end adjustment", func(t *testing.T) {
		// Jan 31 -> Feb should become Feb 29 (leap year) or Feb 28
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.NewDate(2024, time.January, 31))
		st.SetDayOfMonth(31)

		next := st.CalculateNextDate()

		// 2024 is a leap year
		expected := types.NewDate(2024, time.February, 29)
		if !next.Equal(expected) {
			t.Errorf("Expected %s, got %s", expected.String(), next.String())
		}
	})

	t.Run("Monthly with day_of_month -1 for last day", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.NewDate(2024, time.January, 31))
		st.SetDayOfMonth(-1)

		next := st.CalculateNextDate()

		// Last day of February 2024 (leap year)
		expected := types.NewDate(2024, time.February, 29)
		if !next.Equal(expected) {
			t.Errorf("Expected %s, got %s", expected.String(), next.String())
		}
	})

	t.Run("Quarterly adds 3 months", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyQuarterly, types.NewDate(2024, time.January, 15))

		next := st.CalculateNextDate()

		expected := types.NewDate(2024, time.April, 15)
		if !next.Equal(expected) {
			t.Errorf("Expected %s, got %s", expected.String(), next.String())
		}
	})

	t.Run("Yearly adds 1 year", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyYearly, types.NewDate(2024, time.January, 15))

		next := st.CalculateNextDate()

		expected := types.NewDate(2025, time.January, 15)
		if !next.Equal(expected) {
			t.Errorf("Expected %s, got %s", expected.String(), next.String())
		}
	})

	t.Run("Yearly with interval 2", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyYearly, types.NewDate(2024, time.January, 15))
		st.Interval = 2

		next := st.CalculateNextDate()

		expected := types.NewDate(2026, time.January, 15)
		if !next.Equal(expected) {
			t.Errorf("Expected %s, got %s", expected.String(), next.String())
		}
	})

	t.Run("Leap year Feb 29 to next year", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyYearly, types.NewDate(2024, time.February, 29))

		next := st.CalculateNextDate()

		// Go's AddDate handles leap year: Feb 29 + 1 year = Mar 1 (or Feb 28 depending on implementation)
		// The standard behavior is March 1
		expected := types.NewDate(2025, time.March, 1)
		if !next.Equal(expected) {
			t.Errorf("Expected %s, got %s", expected.String(), next.String())
		}
	})

	t.Run("Semi-monthly: 15th and last day", func(t *testing.T) {
		// From the 15th, next pay date should be the last day of the
		// same month.
		st := NewTransaction(types.NewID(), FrequencySemiMonthly, types.NewDate(2026, time.March, 15))
		st.DayOfMonth = types.NullableInt{Int64: 15, Valid: true}
		st.SecondaryDayOfMonth = types.NullableInt{Int64: -1, Valid: true}

		next := st.CalculateNextDate()
		if expected := types.NewDate(2026, time.March, 31); !next.Equal(expected) {
			t.Errorf("from 15th: got %s, want %s", next.String(), expected.String())
		}

		// From the last day, next should roll to the 15th of next month.
		st.NextDate = types.NewDate(2026, time.March, 31)
		next = st.CalculateNextDate()
		if expected := types.NewDate(2026, time.April, 15); !next.Equal(expected) {
			t.Errorf("from last day: got %s, want %s", next.String(), expected.String())
		}

		// February (28-day) — from 15th, advances to Feb 28.
		st.NextDate = types.NewDate(2026, time.February, 15)
		next = st.CalculateNextDate()
		if expected := types.NewDate(2026, time.February, 28); !next.Equal(expected) {
			t.Errorf("Feb from 15th: got %s, want %s", next.String(), expected.String())
		}
	})

	t.Run("Semi-monthly: 1st and 15th", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencySemiMonthly, types.NewDate(2026, time.January, 1))
		st.DayOfMonth = types.NullableInt{Int64: 1, Valid: true}
		st.SecondaryDayOfMonth = types.NullableInt{Int64: 15, Valid: true}

		next := st.CalculateNextDate()
		if expected := types.NewDate(2026, time.January, 15); !next.Equal(expected) {
			t.Errorf("from 1st: got %s, want %s", next.String(), expected.String())
		}

		st.NextDate = types.NewDate(2026, time.January, 15)
		next = st.CalculateNextDate()
		if expected := types.NewDate(2026, time.February, 1); !next.Equal(expected) {
			t.Errorf("from 15th: got %s, want %s", next.String(), expected.String())
		}
	})
}

func TestAdvanceSchedule(t *testing.T) {
	t.Run("Advances next date for indefinite schedule", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.NewDate(2024, time.January, 15))

		result := st.AdvanceSchedule()

		if !result {
			t.Error("AdvanceSchedule should return true for indefinite schedule")
		}
		expected := types.NewDate(2024, time.February, 15)
		if !st.NextDate.Equal(expected) {
			t.Errorf("Expected next date %s, got %s", expected.String(), st.NextDate.String())
		}
	})

	t.Run("Decrements occurrences remaining", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.NewDate(2024, time.January, 15))
		st.SetOccurrences(3)

		result := st.AdvanceSchedule()

		if !result {
			t.Error("AdvanceSchedule should return true when occurrences remaining > 1")
		}
		if st.OccurrencesRemaining.Int64 != 2 {
			t.Errorf("Expected occurrences_remaining 2, got %d", st.OccurrencesRemaining.Int64)
		}
	})

	t.Run("Returns false when occurrences exhausted", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.NewDate(2024, time.January, 15))
		st.SetOccurrences(1)

		result := st.AdvanceSchedule()

		if result {
			t.Error("AdvanceSchedule should return false when occurrences exhausted")
		}
		if st.OccurrencesRemaining.Int64 != 0 {
			t.Errorf("Expected occurrences_remaining 0, got %d", st.OccurrencesRemaining.Int64)
		}
	})

	t.Run("Returns false when next date would be past end date", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.NewDate(2024, time.December, 15))
		st.SetEndDate(types.NewDate(2024, time.December, 31))

		result := st.AdvanceSchedule()

		if result {
			t.Error("AdvanceSchedule should return false when next date would be past end date")
		}
	})

	t.Run("Multiple advances work correctly", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyWeekly, types.NewDate(2024, time.January, 1))

		st.AdvanceSchedule()
		st.AdvanceSchedule()
		st.AdvanceSchedule()

		expected := types.NewDate(2024, time.January, 22)
		if !st.NextDate.Equal(expected) {
			t.Errorf("Expected next date %s after 3 advances, got %s", expected.String(), st.NextDate.String())
		}
	})
}

func TestTransactionValidation(t *testing.T) {
	validTransaction := func() *Transaction {
		return NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
	}

	t.Run("Valid scheduled transaction passes validation", func(t *testing.T) {
		st := validTransaction()
		errs := st.Validate()
		if errs.HasErrors() {
			t.Errorf("Valid scheduled transaction should pass validation: %v", errs)
		}
	})

	t.Run("IsValid returns true for valid scheduled transaction", func(t *testing.T) {
		st := validTransaction()
		if !st.IsValid() {
			t.Error("IsValid should return true for valid scheduled transaction")
		}
	})

	t.Run("Nil account ID fails validation", func(t *testing.T) {
		st := validTransaction()
		st.AccountID = types.NilID
		errs := st.Validate()
		if !errs.HasErrors() {
			t.Error("Nil account ID should fail validation")
		}
	})

	t.Run("Zero start date fails validation", func(t *testing.T) {
		st := validTransaction()
		st.StartDate = types.ZeroDate
		errs := st.Validate()
		if !errs.HasErrors() {
			t.Error("Zero start date should fail validation")
		}
	})

	t.Run("Zero next date fails validation", func(t *testing.T) {
		st := validTransaction()
		st.NextDate = types.ZeroDate
		errs := st.Validate()
		if !errs.HasErrors() {
			t.Error("Zero next date should fail validation")
		}
	})

	t.Run("Invalid frequency fails validation", func(t *testing.T) {
		st := validTransaction()
		st.Frequency = Frequency("invalid")
		errs := st.Validate()
		if !errs.HasErrors() {
			t.Error("Invalid frequency should fail validation")
		}
	})

	t.Run("Zero interval fails validation", func(t *testing.T) {
		st := validTransaction()
		st.Interval = 0
		errs := st.Validate()
		if !errs.HasErrors() {
			t.Error("Zero interval should fail validation")
		}
	})

	t.Run("Negative interval fails validation", func(t *testing.T) {
		st := validTransaction()
		st.Interval = -1
		errs := st.Validate()
		if !errs.HasErrors() {
			t.Error("Negative interval should fail validation")
		}
	})

	t.Run("End date before start date fails validation", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.NewDate(2024, time.June, 15))
		st.EndDate = types.NullableDate{Date: types.NewDate(2024, time.January, 1), Valid: true}
		errs := st.Validate()
		if !errs.HasErrors() {
			t.Error("End date before start date should fail validation")
		}
	})

	t.Run("End date equal to start date fails validation", func(t *testing.T) {
		startDate := types.NewDate(2024, time.June, 15)
		st := NewTransaction(types.NewID(), FrequencyMonthly, startDate)
		st.EndDate = types.NullableDate{Date: startDate, Valid: true}
		errs := st.Validate()
		if !errs.HasErrors() {
			t.Error("End date equal to start date should fail validation")
		}
	})

	t.Run("Zero occurrences fails validation", func(t *testing.T) {
		st := validTransaction()
		st.Occurrences = types.NullableInt{Int64: 0, Valid: true}
		errs := st.Validate()
		if !errs.HasErrors() {
			t.Error("Zero occurrences should fail validation")
		}
	})

	t.Run("Negative occurrences fails validation", func(t *testing.T) {
		st := validTransaction()
		st.Occurrences = types.NullableInt{Int64: -5, Valid: true}
		errs := st.Validate()
		if !errs.HasErrors() {
			t.Error("Negative occurrences should fail validation")
		}
	})

	t.Run("Both end date and occurrences fails validation", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.NewDate(2024, time.January, 1))
		st.EndDate = types.NullableDate{Date: types.NewDate(2025, time.December, 31), Valid: true}
		st.Occurrences = types.NullableInt{Int64: 12, Valid: true}
		errs := st.Validate()
		if !errs.HasErrors() {
			t.Error("Having both end date and occurrences should fail validation")
		}
	})

	t.Run("Invalid day_of_month fails validation", func(t *testing.T) {
		testCases := []int64{0, 32, -2, -100}
		for _, dom := range testCases {
			st := validTransaction()
			st.DayOfMonth = types.NullableInt{Int64: dom, Valid: true}
			errs := st.Validate()
			if !errs.HasErrors() {
				t.Errorf("day_of_month %d should fail validation", dom)
			}
		}
	})

	t.Run("Valid day_of_month passes validation", func(t *testing.T) {
		validDays := []int64{1, 15, 28, 31, -1}
		for _, dom := range validDays {
			st := validTransaction()
			st.DayOfMonth = types.NullableInt{Int64: dom, Valid: true}
			errs := st.Validate()
			if errs.HasErrors() {
				t.Errorf("day_of_month %d should pass validation: %v", dom, errs)
			}
		}
	})


	t.Run("Zero amount_estimate_count fails validation", func(t *testing.T) {
		st := validTransaction()
		st.AmountEstimateCount = types.NullableInt{Int64: 0, Valid: true}
		errs := st.Validate()
		if !errs.HasErrors() {
			t.Error("Zero amount_estimate_count should fail validation")
		}
	})

	t.Run("Negative amount_estimate_count fails validation", func(t *testing.T) {
		st := validTransaction()
		st.AmountEstimateCount = types.NullableInt{Int64: -3, Valid: true}
		errs := st.Validate()
		if !errs.HasErrors() {
			t.Error("Negative amount_estimate_count should fail validation")
		}
	})

	t.Run("Occurrences remaining exceeding occurrences fails validation", func(t *testing.T) {
		st := validTransaction()
		st.Occurrences = types.NullableInt{Int64: 10, Valid: true}
		st.OccurrencesRemaining = types.NullableInt{Int64: 15, Valid: true}
		errs := st.Validate()
		if !errs.HasErrors() {
			t.Error("Occurrences remaining exceeding occurrences should fail validation")
		}
	})

	t.Run("Memo exceeding max length fails validation", func(t *testing.T) {
		st := validTransaction()
		st.Memo = types.NullableString{String: string(make([]byte, 1001)), Valid: true}
		errs := st.Validate()
		if !errs.HasErrors() {
			t.Error("Memo exceeding 1000 chars should fail validation")
		}
	})

	t.Run("Invalid post_lead_days fails validation", func(t *testing.T) {
		testCases := []int{1, 2, 4, 5, 6, 10, -1}
		for _, days := range testCases {
			st := validTransaction()
			st.PostLeadDays = days
			errs := st.Validate()
			if !errs.HasErrors() {
				t.Errorf("post_lead_days %d should fail validation", days)
			}
		}
	})

	t.Run("Valid post_lead_days passes validation", func(t *testing.T) {
		validDays := []int{0, 3, 7}
		for _, days := range validDays {
			st := validTransaction()
			st.PostLeadDays = days
			errs := st.Validate()
			if errs.HasErrors() {
				t.Errorf("post_lead_days %d should pass validation: %v", days, errs)
			}
		}
	})

	t.Run("AutoPost with valid post_lead_days passes validation", func(t *testing.T) {
		st := validTransaction()
		st.AutoPost = true
		st.PostLeadDays = 3
		errs := st.Validate()
		if errs.HasErrors() {
			t.Errorf("AutoPost with post_lead_days 3 should pass validation: %v", errs)
		}
	})

	t.Run("All frequencies pass validation", func(t *testing.T) {
		for _, freq := range AllFrequencies() {
			st := NewTransaction(types.NewID(), freq, types.Today())
			errs := st.Validate()
			if errs.HasErrors() {
				t.Errorf("Frequency %q should pass validation: %v", freq, errs)
			}
		}
	})

	t.Run("Multiple validation errors collected", func(t *testing.T) {
		st := &Transaction{
			AccountID: types.NilID,
			Frequency: Frequency("bad"),
			StartDate: types.ZeroDate,
			NextDate:  types.ZeroDate,
			Interval:  0,
		}
		errs := st.Validate()
		if len(errs) < 5 {
			t.Errorf("Expected at least 5 errors, got %d: %v", len(errs), errs)
		}
	})
}

func TestTransactionUpdatesTimestamp(t *testing.T) {
	t.Run("SetPayee updates timestamp", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		original := st.UpdatedAt

		st.SetPayee(types.NewID())

		if !st.UpdatedAt.After(original) && !st.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("SetPayee should update UpdatedAt")
		}
	})

	t.Run("SetCategory updates timestamp", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		original := st.UpdatedAt

		st.SetCategory(types.NewID())

		if !st.UpdatedAt.After(original) && !st.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("SetCategory should update UpdatedAt")
		}
	})

	t.Run("SetAmount updates timestamp", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		original := st.UpdatedAt

		st.SetAmount(types.MustNewMoney("-100.00"))

		if !st.UpdatedAt.After(original) && !st.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("SetAmount should update UpdatedAt")
		}
	})

	t.Run("SetMemo updates timestamp", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		original := st.UpdatedAt

		st.SetMemo("Test")

		if !st.UpdatedAt.After(original) && !st.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("SetMemo should update UpdatedAt")
		}
	})

	t.Run("SetEndDate updates timestamp", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		original := st.UpdatedAt

		st.SetEndDate(types.NewDate(2025, time.December, 31))

		if !st.UpdatedAt.After(original) && !st.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("SetEndDate should update UpdatedAt")
		}
	})

	t.Run("SetOccurrences updates timestamp", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		original := st.UpdatedAt

		st.SetOccurrences(12)

		if !st.UpdatedAt.After(original) && !st.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("SetOccurrences should update UpdatedAt")
		}
	})

	t.Run("AdvanceSchedule updates timestamp", func(t *testing.T) {
		st := NewTransaction(types.NewID(), FrequencyMonthly, types.Today())
		original := st.UpdatedAt

		st.AdvanceSchedule()

		if !st.UpdatedAt.After(original) && !st.UpdatedAt.Time().Equal(original.Time()) {
			t.Error("AdvanceSchedule should update UpdatedAt")
		}
	})
}
