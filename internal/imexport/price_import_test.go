package imexport

import (
	"strings"
	"testing"
)

// =============================================================================
// SM-115: ParsePriceCSV
// =============================================================================

func TestParsePriceCSV(t *testing.T) {
	t.Run("parses valid CSV with date, ticker, price", func(t *testing.T) {
		input := "Date,Ticker,Price\n2024-03-15,AAPL,150.00\n2024-03-15,GOOG,140.50\n2024-03-16,AAPL,152.25\n"
		result, err := ParsePriceCSV(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParsePriceCSV() error = %v", err)
		}
		if len(result.Errors) != 0 {
			t.Errorf("Expected 0 errors, got %d: %v", len(result.Errors), result.Errors)
		}
		if len(result.Records) != 3 {
			t.Fatalf("Expected 3 records, got %d", len(result.Records))
		}

		// Check first record
		rec := result.Records[0]
		if rec.Ticker != "AAPL" {
			t.Errorf("Expected ticker AAPL, got %q", rec.Ticker)
		}
		if rec.Price.String() != "150" {
			t.Errorf("Expected price '150', got %q", rec.Price.String())
		}
		if rec.Date.Time().Format("2006-01-02") != "2024-03-15" {
			t.Errorf("Expected date 2024-03-15, got %q", rec.Date.Time().Format("2006-01-02"))
		}
		if rec.SourceLine != 2 {
			t.Errorf("Expected source line 2, got %d", rec.SourceLine)
		}

		// Check second record
		rec2 := result.Records[1]
		if rec2.Ticker != "GOOG" {
			t.Errorf("Expected ticker GOOG, got %q", rec2.Ticker)
		}
		if rec2.Price.String() != "140.5" {
			t.Errorf("Expected price '140.5', got %q", rec2.Price.String())
		}
	})

	t.Run("rejects empty file (no header)", func(t *testing.T) {
		input := ""
		_, err := ParsePriceCSV(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for empty file")
		}
		if !strings.Contains(err.Error(), "no header row") {
			t.Errorf("Expected 'no header row' error, got: %v", err)
		}
	})

	t.Run("rejects missing Date header", func(t *testing.T) {
		input := "Ticker,Price\nAAPL,150.00\n"
		_, err := ParsePriceCSV(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for missing Date header")
		}
		if !strings.Contains(err.Error(), "Date") {
			t.Errorf("Expected error mentioning Date, got: %v", err)
		}
	})

	t.Run("rejects missing Ticker header", func(t *testing.T) {
		input := "Date,Price\n2024-03-15,150.00\n"
		_, err := ParsePriceCSV(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for missing Ticker header")
		}
		if !strings.Contains(err.Error(), "Ticker") {
			t.Errorf("Expected error mentioning Ticker, got: %v", err)
		}
	})

	t.Run("rejects missing Price header", func(t *testing.T) {
		input := "Date,Ticker\n2024-03-15,AAPL\n"
		_, err := ParsePriceCSV(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for missing Price header")
		}
		if !strings.Contains(err.Error(), "Price") {
			t.Errorf("Expected error mentioning Price, got: %v", err)
		}
	})

	t.Run("reports invalid date format with line number", func(t *testing.T) {
		input := "Date,Ticker,Price\n2024-03-15,AAPL,150.00\nnot-a-date,GOOG,140.00\n"
		result, err := ParsePriceCSV(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParsePriceCSV() error = %v", err)
		}
		if len(result.Records) != 1 {
			t.Errorf("Expected 1 valid record, got %d", len(result.Records))
		}
		if len(result.Errors) != 1 {
			t.Fatalf("Expected 1 error, got %d", len(result.Errors))
		}
		if result.Errors[0].Line != 3 {
			t.Errorf("Expected error on line 3, got line %d", result.Errors[0].Line)
		}
		if !strings.Contains(result.Errors[0].Message, "invalid date") {
			t.Errorf("Expected 'invalid date' error, got: %s", result.Errors[0].Message)
		}
	})

	t.Run("reports invalid price with line number", func(t *testing.T) {
		input := "Date,Ticker,Price\n2024-03-15,AAPL,abc\n"
		result, err := ParsePriceCSV(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParsePriceCSV() error = %v", err)
		}
		if len(result.Records) != 0 {
			t.Errorf("Expected 0 records, got %d", len(result.Records))
		}
		if len(result.Errors) != 1 {
			t.Fatalf("Expected 1 error, got %d", len(result.Errors))
		}
		if result.Errors[0].Line != 2 {
			t.Errorf("Expected error on line 2, got line %d", result.Errors[0].Line)
		}
		if !strings.Contains(result.Errors[0].Message, "invalid price") {
			t.Errorf("Expected 'invalid price' error, got: %s", result.Errors[0].Message)
		}
	})

	t.Run("rejects non-positive price", func(t *testing.T) {
		input := "Date,Ticker,Price\n2024-03-15,AAPL,0\n2024-03-15,GOOG,-10.00\n"
		result, err := ParsePriceCSV(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParsePriceCSV() error = %v", err)
		}
		if len(result.Records) != 0 {
			t.Errorf("Expected 0 valid records, got %d", len(result.Records))
		}
		if len(result.Errors) != 2 {
			t.Fatalf("Expected 2 errors, got %d", len(result.Errors))
		}
		for _, e := range result.Errors {
			if !strings.Contains(e.Message, "price must be positive") {
				t.Errorf("Expected 'price must be positive' error, got: %s", e.Message)
			}
		}
	})

	t.Run("reports missing ticker with line number", func(t *testing.T) {
		input := "Date,Ticker,Price\n2024-03-15,,150.00\n"
		result, err := ParsePriceCSV(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParsePriceCSV() error = %v", err)
		}
		if len(result.Errors) != 1 {
			t.Fatalf("Expected 1 error, got %d", len(result.Errors))
		}
		if result.Errors[0].Line != 2 {
			t.Errorf("Expected error on line 2, got line %d", result.Errors[0].Line)
		}
		if !strings.Contains(result.Errors[0].Message, "missing ticker") {
			t.Errorf("Expected 'missing ticker' error, got: %s", result.Errors[0].Message)
		}
	})

	t.Run("case-insensitive headers", func(t *testing.T) {
		input := "date,ticker,price\n2024-03-15,AAPL,150.00\n"
		result, err := ParsePriceCSV(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParsePriceCSV() error = %v", err)
		}
		if len(result.Records) != 1 {
			t.Fatalf("Expected 1 record, got %d", len(result.Records))
		}
		if result.Records[0].Ticker != "AAPL" {
			t.Errorf("Expected AAPL, got %q", result.Records[0].Ticker)
		}
	})

	t.Run("normalizes ticker to uppercase", func(t *testing.T) {
		input := "Date,Ticker,Price\n2024-03-15,aapl,150.00\n"
		result, err := ParsePriceCSV(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParsePriceCSV() error = %v", err)
		}
		if len(result.Records) != 1 {
			t.Fatalf("Expected 1 record, got %d", len(result.Records))
		}
		if result.Records[0].Ticker != "AAPL" {
			t.Errorf("Expected AAPL, got %q", result.Records[0].Ticker)
		}
	})

	t.Run("ignores extra columns", func(t *testing.T) {
		input := "Date,Ticker,Price,Volume,Notes\n2024-03-15,AAPL,150.00,1000000,test\n"
		result, err := ParsePriceCSV(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParsePriceCSV() error = %v", err)
		}
		if len(result.Records) != 1 {
			t.Fatalf("Expected 1 record, got %d", len(result.Records))
		}
	})

	t.Run("multiple errors reported with correct line numbers", func(t *testing.T) {
		input := "Date,Ticker,Price\n2024-03-15,AAPL,150.00\nbad-date,GOOG,140.00\n2024-03-16,,155.00\n2024-03-17,MSFT,abc\n"
		result, err := ParsePriceCSV(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParsePriceCSV() error = %v", err)
		}
		if len(result.Records) != 1 {
			t.Errorf("Expected 1 valid record, got %d", len(result.Records))
		}
		if len(result.Errors) != 3 {
			t.Fatalf("Expected 3 errors, got %d", len(result.Errors))
		}
		expectedLines := []int{3, 4, 5}
		for i, e := range result.Errors {
			if e.Line != expectedLines[i] {
				t.Errorf("Error %d: expected line %d, got %d", i, expectedLines[i], e.Line)
			}
		}
	})

	t.Run("header only file returns empty result", func(t *testing.T) {
		input := "Date,Ticker,Price\n"
		result, err := ParsePriceCSV(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParsePriceCSV() error = %v", err)
		}
		if len(result.Records) != 0 {
			t.Errorf("Expected 0 records, got %d", len(result.Records))
		}
		if len(result.Errors) != 0 {
			t.Errorf("Expected 0 errors, got %d", len(result.Errors))
		}
	})
}
