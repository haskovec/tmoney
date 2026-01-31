package models

import (
	"testing"
	"time"
)

func TestID(t *testing.T) {
	t.Run("NewID generates unique IDs", func(t *testing.T) {
		id1 := NewID()
		id2 := NewID()
		if id1 == id2 {
			t.Error("NewID should generate unique IDs")
		}
	})

	t.Run("NilID is zero value", func(t *testing.T) {
		if !NilID.IsNil() {
			t.Error("NilID.IsNil() should return true")
		}
	})

	t.Run("NewID is not nil", func(t *testing.T) {
		id := NewID()
		if id.IsNil() {
			t.Error("NewID should not generate nil ID")
		}
	})

	t.Run("ParseID parses valid UUID", func(t *testing.T) {
		original := NewID()
		parsed, err := ParseID(original.String())
		if err != nil {
			t.Fatalf("ParseID failed: %v", err)
		}
		if parsed != original {
			t.Error("Parsed ID should match original")
		}
	})

	t.Run("ParseID returns error for invalid string", func(t *testing.T) {
		_, err := ParseID("not-a-uuid")
		if err == nil {
			t.Error("ParseID should return error for invalid string")
		}
	})

	t.Run("MustParseID panics on invalid string", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("MustParseID should panic on invalid string")
			}
		}()
		MustParseID("not-a-uuid")
	})

	t.Run("String returns UUID format", func(t *testing.T) {
		id := NewID()
		s := id.String()
		if len(s) != 36 {
			t.Errorf("ID string should be 36 characters, got %d", len(s))
		}
	})

	t.Run("Scan handles string", func(t *testing.T) {
		original := NewID()
		var scanned ID
		err := scanned.Scan(original.String())
		if err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		if scanned != original {
			t.Error("Scanned ID should match original")
		}
	})

	t.Run("Scan handles nil", func(t *testing.T) {
		var scanned ID
		err := scanned.Scan(nil)
		if err != nil {
			t.Fatalf("Scan nil failed: %v", err)
		}
		if !scanned.IsNil() {
			t.Error("Scan nil should result in NilID")
		}
	})

	t.Run("Scan handles bytes", func(t *testing.T) {
		original := NewID()
		var scanned ID
		err := scanned.Scan([]byte(original.String()))
		if err != nil {
			t.Fatalf("Scan bytes failed: %v", err)
		}
		if scanned != original {
			t.Error("Scanned ID should match original")
		}
	})

	t.Run("Value returns string", func(t *testing.T) {
		id := NewID()
		val, err := id.Value()
		if err != nil {
			t.Fatalf("Value failed: %v", err)
		}
		if val.(string) != id.String() {
			t.Error("Value should return string representation")
		}
	})
}

func TestMoney(t *testing.T) {
	t.Run("NewMoney parses valid string", func(t *testing.T) {
		m, err := NewMoney("123.45")
		if err != nil {
			t.Fatalf("NewMoney failed: %v", err)
		}
		if m.String() != "123.45" {
			t.Errorf("Expected 123.45, got %s", m.String())
		}
	})

	t.Run("NewMoney returns error for invalid string", func(t *testing.T) {
		_, err := NewMoney("not-a-number")
		if err == nil {
			t.Error("NewMoney should return error for invalid string")
		}
	})

	t.Run("MustNewMoney panics on invalid string", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("MustNewMoney should panic on invalid string")
			}
		}()
		MustNewMoney("not-a-number")
	})

	t.Run("NewMoneyFromInt creates whole number", func(t *testing.T) {
		m := NewMoneyFromInt(100)
		if m.String() != "100" {
			t.Errorf("Expected 100, got %s", m.String())
		}
	})

	t.Run("ZeroMoney is zero", func(t *testing.T) {
		if !ZeroMoney.IsZero() {
			t.Error("ZeroMoney.IsZero() should return true")
		}
	})

	t.Run("IsNegative detects negative values", func(t *testing.T) {
		m := MustNewMoney("-50")
		if !m.IsNegative() {
			t.Error("IsNegative should return true for -50")
		}
		if m.IsPositive() {
			t.Error("IsPositive should return false for -50")
		}
	})

	t.Run("IsPositive detects positive values", func(t *testing.T) {
		m := MustNewMoney("50")
		if !m.IsPositive() {
			t.Error("IsPositive should return true for 50")
		}
		if m.IsNegative() {
			t.Error("IsNegative should return false for 50")
		}
	})

	t.Run("Add sums two values", func(t *testing.T) {
		a := MustNewMoney("100.50")
		b := MustNewMoney("50.25")
		sum := a.Add(b)
		if sum.String() != "150.75" {
			t.Errorf("Expected 150.75, got %s", sum.String())
		}
	})

	t.Run("Sub subtracts two values", func(t *testing.T) {
		a := MustNewMoney("100.50")
		b := MustNewMoney("50.25")
		diff := a.Sub(b)
		if diff.String() != "50.25" {
			t.Errorf("Expected 50.25, got %s", diff.String())
		}
	})

	t.Run("Neg negates value", func(t *testing.T) {
		m := MustNewMoney("50")
		neg := m.Neg()
		if neg.String() != "-50" {
			t.Errorf("Expected -50, got %s", neg.String())
		}
	})

	t.Run("Abs returns absolute value", func(t *testing.T) {
		m := MustNewMoney("-50")
		abs := m.Abs()
		if abs.String() != "50" {
			t.Errorf("Expected 50, got %s", abs.String())
		}
	})

	t.Run("Cmp compares values correctly", func(t *testing.T) {
		a := MustNewMoney("100")
		b := MustNewMoney("50")
		c := MustNewMoney("100")

		if a.Cmp(b) != 1 {
			t.Error("100 should be greater than 50")
		}
		if b.Cmp(a) != -1 {
			t.Error("50 should be less than 100")
		}
		if a.Cmp(c) != 0 {
			t.Error("100 should equal 100")
		}
	})

	t.Run("Equal checks equality", func(t *testing.T) {
		a := MustNewMoney("100")
		b := MustNewMoney("100")
		c := MustNewMoney("50")

		if !a.Equal(b) {
			t.Error("100 should equal 100")
		}
		if a.Equal(c) {
			t.Error("100 should not equal 50")
		}
	})

	t.Run("Scan handles string", func(t *testing.T) {
		var m Money
		err := m.Scan("123.45")
		if err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		if m.String() != "123.45" {
			t.Errorf("Expected 123.45, got %s", m.String())
		}
	})

	t.Run("Scan handles nil", func(t *testing.T) {
		var m Money
		err := m.Scan(nil)
		if err != nil {
			t.Fatalf("Scan nil failed: %v", err)
		}
		if !m.IsZero() {
			t.Error("Scan nil should result in ZeroMoney")
		}
	})

	t.Run("Scan handles float64", func(t *testing.T) {
		var m Money
		err := m.Scan(float64(123.45))
		if err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		expected := MustNewMoney("123.45")
		if m.Cmp(expected) != 0 {
			t.Errorf("Expected %s, got %s", expected.String(), m.String())
		}
	})

	t.Run("Value returns string", func(t *testing.T) {
		m := MustNewMoney("123.45")
		val, err := m.Value()
		if err != nil {
			t.Fatalf("Value failed: %v", err)
		}
		if val.(string) != "123.45" {
			t.Error("Value should return string representation")
		}
	})
}

func TestQuantity(t *testing.T) {
	t.Run("NewQuantity parses valid string", func(t *testing.T) {
		q, err := NewQuantity("12.34567890")
		if err != nil {
			t.Fatalf("NewQuantity failed: %v", err)
		}
		if q.String() != "12.3456789" {
			t.Errorf("Expected 12.3456789, got %s", q.String())
		}
	})

	t.Run("ZeroQuantity is zero", func(t *testing.T) {
		if !ZeroQuantity.IsZero() {
			t.Error("ZeroQuantity.IsZero() should return true")
		}
	})

	t.Run("Add sums quantities", func(t *testing.T) {
		a := MustNewQuantity("10.5")
		b := MustNewQuantity("5.25")
		sum := a.Add(b)
		if sum.String() != "15.75" {
			t.Errorf("Expected 15.75, got %s", sum.String())
		}
	})

	t.Run("Sub subtracts quantities", func(t *testing.T) {
		a := MustNewQuantity("10.5")
		b := MustNewQuantity("5.25")
		diff := a.Sub(b)
		if diff.String() != "5.25" {
			t.Errorf("Expected 5.25, got %s", diff.String())
		}
	})
}

func TestTimestamp(t *testing.T) {
	t.Run("Now returns current time", func(t *testing.T) {
		before := time.Now().UTC()
		ts := Now()
		after := time.Now().UTC()

		if ts.Time().Before(before) || ts.Time().After(after) {
			t.Error("Now() should return current time")
		}
	})

	t.Run("ZeroTimestamp is zero", func(t *testing.T) {
		if !ZeroTimestamp.IsZero() {
			t.Error("ZeroTimestamp.IsZero() should return true")
		}
	})

	t.Run("ParseTimestamp parses RFC3339", func(t *testing.T) {
		ts, err := ParseTimestamp("2024-01-15T10:30:00Z")
		if err != nil {
			t.Fatalf("ParseTimestamp failed: %v", err)
		}
		if ts.Time().Year() != 2024 || ts.Time().Month() != 1 || ts.Time().Day() != 15 {
			t.Error("Parsed timestamp has wrong date")
		}
	})

	t.Run("ParseTimestamp returns error for invalid format", func(t *testing.T) {
		_, err := ParseTimestamp("not-a-timestamp")
		if err == nil {
			t.Error("ParseTimestamp should return error for invalid format")
		}
	})

	t.Run("String returns RFC3339 format", func(t *testing.T) {
		ts := NewTimestamp(time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC))
		s := ts.String()
		if s != "2024-01-15T10:30:00Z" {
			t.Errorf("Expected 2024-01-15T10:30:00Z, got %s", s)
		}
	})

	t.Run("Before and After work correctly", func(t *testing.T) {
		earlier := NewTimestamp(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		later := NewTimestamp(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))

		if !earlier.Before(later) {
			t.Error("earlier.Before(later) should return true")
		}
		if !later.After(earlier) {
			t.Error("later.After(earlier) should return true")
		}
	})

	t.Run("Scan handles time.Time", func(t *testing.T) {
		input := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
		var ts Timestamp
		err := ts.Scan(input)
		if err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		if !ts.Time().Equal(input) {
			t.Error("Scanned timestamp should match input")
		}
	})

	t.Run("Scan handles nil", func(t *testing.T) {
		var ts Timestamp
		err := ts.Scan(nil)
		if err != nil {
			t.Fatalf("Scan nil failed: %v", err)
		}
		if !ts.IsZero() {
			t.Error("Scan nil should result in ZeroTimestamp")
		}
	})

	t.Run("Scan handles RFC3339 string", func(t *testing.T) {
		var ts Timestamp
		err := ts.Scan("2024-01-15T10:30:00Z")
		if err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		if ts.Time().Year() != 2024 {
			t.Error("Scanned timestamp has wrong year")
		}
	})

	t.Run("Scan handles DuckDB timestamp format", func(t *testing.T) {
		var ts Timestamp
		err := ts.Scan("2024-01-15 10:30:00")
		if err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		if ts.Time().Year() != 2024 {
			t.Error("Scanned timestamp has wrong year")
		}
	})
}

func TestDate(t *testing.T) {
	t.Run("Today returns current date", func(t *testing.T) {
		today := Today()
		now := time.Now()
		if today.Year() != now.Year() || today.Month() != now.Month() || today.Day() != now.Day() {
			t.Error("Today() should return current date")
		}
	})

	t.Run("NewDate creates correct date", func(t *testing.T) {
		d := NewDate(2024, 6, 15)
		if d.Year() != 2024 || d.Month() != 6 || d.Day() != 15 {
			t.Error("NewDate created wrong date")
		}
	})

	t.Run("ZeroDate is zero", func(t *testing.T) {
		if !ZeroDate.IsZero() {
			t.Error("ZeroDate.IsZero() should return true")
		}
	})

	t.Run("ParseDate parses YYYY-MM-DD", func(t *testing.T) {
		d, err := ParseDate("2024-06-15")
		if err != nil {
			t.Fatalf("ParseDate failed: %v", err)
		}
		if d.Year() != 2024 || d.Month() != 6 || d.Day() != 15 {
			t.Error("Parsed date has wrong values")
		}
	})

	t.Run("ParseDate returns error for invalid format", func(t *testing.T) {
		_, err := ParseDate("15/06/2024")
		if err == nil {
			t.Error("ParseDate should return error for invalid format")
		}
	})

	t.Run("MustParseDate panics on invalid string", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("MustParseDate should panic on invalid string")
			}
		}()
		MustParseDate("not-a-date")
	})

	t.Run("String returns YYYY-MM-DD format", func(t *testing.T) {
		d := NewDate(2024, 6, 15)
		if d.String() != "2024-06-15" {
			t.Errorf("Expected 2024-06-15, got %s", d.String())
		}
	})

	t.Run("Before and After work correctly", func(t *testing.T) {
		earlier := NewDate(2024, 1, 1)
		later := NewDate(2024, 12, 31)

		if !earlier.Before(later) {
			t.Error("earlier.Before(later) should return true")
		}
		if !later.After(earlier) {
			t.Error("later.After(earlier) should return true")
		}
	})

	t.Run("Equal checks equality", func(t *testing.T) {
		d1 := NewDate(2024, 6, 15)
		d2 := NewDate(2024, 6, 15)
		d3 := NewDate(2024, 6, 16)

		if !d1.Equal(d2) {
			t.Error("Same dates should be equal")
		}
		if d1.Equal(d3) {
			t.Error("Different dates should not be equal")
		}
	})

	t.Run("AddDays adds days", func(t *testing.T) {
		d := NewDate(2024, 6, 15)
		added := d.AddDays(10)
		if added.Day() != 25 {
			t.Errorf("Expected day 25, got %d", added.Day())
		}
	})

	t.Run("AddMonths adds months", func(t *testing.T) {
		d := NewDate(2024, 6, 15)
		added := d.AddMonths(3)
		if added.Month() != 9 {
			t.Errorf("Expected month 9, got %d", added.Month())
		}
	})

	t.Run("AddYears adds years", func(t *testing.T) {
		d := NewDate(2024, 6, 15)
		added := d.AddYears(2)
		if added.Year() != 2026 {
			t.Errorf("Expected year 2026, got %d", added.Year())
		}
	})

	t.Run("Scan handles time.Time", func(t *testing.T) {
		input := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
		var d Date
		err := d.Scan(input)
		if err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		if d.Year() != 2024 || d.Month() != 6 || d.Day() != 15 {
			t.Error("Scanned date has wrong values")
		}
	})

	t.Run("Scan handles nil", func(t *testing.T) {
		var d Date
		err := d.Scan(nil)
		if err != nil {
			t.Fatalf("Scan nil failed: %v", err)
		}
		if !d.IsZero() {
			t.Error("Scan nil should result in ZeroDate")
		}
	})

	t.Run("Scan handles string", func(t *testing.T) {
		var d Date
		err := d.Scan("2024-06-15")
		if err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		if d.Year() != 2024 || d.Month() != 6 || d.Day() != 15 {
			t.Error("Scanned date has wrong values")
		}
	})
}

func TestNullableTypes(t *testing.T) {
	t.Run("NullableID with valid value", func(t *testing.T) {
		id := NewID()
		n := NullableID{ID: id, Valid: true}
		val, err := n.Value()
		if err != nil {
			t.Fatalf("Value failed: %v", err)
		}
		if val == nil {
			t.Error("Value should not be nil for valid NullableID")
		}
	})

	t.Run("NullableID with null value", func(t *testing.T) {
		n := NullableID{Valid: false}
		val, err := n.Value()
		if err != nil {
			t.Fatalf("Value failed: %v", err)
		}
		if val != nil {
			t.Error("Value should be nil for invalid NullableID")
		}
	})

	t.Run("NullableID Scan nil", func(t *testing.T) {
		var n NullableID
		err := n.Scan(nil)
		if err != nil {
			t.Fatalf("Scan nil failed: %v", err)
		}
		if n.Valid {
			t.Error("Valid should be false after scanning nil")
		}
	})

	t.Run("NullableID Scan value", func(t *testing.T) {
		id := NewID()
		var n NullableID
		err := n.Scan(id.String())
		if err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		if !n.Valid {
			t.Error("Valid should be true after scanning value")
		}
		if n.ID != id {
			t.Error("ID should match scanned value")
		}
	})

	t.Run("NullableMoney with valid value", func(t *testing.T) {
		m := MustNewMoney("100.50")
		n := NullableMoney{Money: m, Valid: true}
		val, err := n.Value()
		if err != nil {
			t.Fatalf("Value failed: %v", err)
		}
		if val == nil {
			t.Error("Value should not be nil for valid NullableMoney")
		}
	})

	t.Run("NullableMoney with null value", func(t *testing.T) {
		n := NullableMoney{Valid: false}
		val, err := n.Value()
		if err != nil {
			t.Fatalf("Value failed: %v", err)
		}
		if val != nil {
			t.Error("Value should be nil for invalid NullableMoney")
		}
	})

	t.Run("NullableDate with valid value", func(t *testing.T) {
		d := NewDate(2024, 6, 15)
		n := NullableDate{Date: d, Valid: true}
		val, err := n.Value()
		if err != nil {
			t.Fatalf("Value failed: %v", err)
		}
		if val == nil {
			t.Error("Value should not be nil for valid NullableDate")
		}
	})

	t.Run("NullableDate Scan nil", func(t *testing.T) {
		var n NullableDate
		err := n.Scan(nil)
		if err != nil {
			t.Fatalf("Scan nil failed: %v", err)
		}
		if n.Valid {
			t.Error("Valid should be false after scanning nil")
		}
	})
}
