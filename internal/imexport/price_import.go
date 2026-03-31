package imexport

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/haskovec/tmoney/internal/types"
)

// Price CSV column headers.
const (
	priceCSVColDate   = "Date"
	priceCSVColTicker = "Ticker"
	priceCSVColPrice  = "Price"
)

// PriceRecord represents a single parsed row from a price CSV file.
type PriceRecord struct {
	Date       types.Date
	Ticker     string
	Price      types.Money
	SourceLine int
}

// PriceParseResult holds the outcome of parsing a price CSV file.
type PriceParseResult struct {
	Records []PriceRecord
	Errors  []ParseError
}

// HasErrors returns true if there were any parse errors.
func (r *PriceParseResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// ParsePriceCSV reads a price CSV file and returns parsed price records.
// Expected columns: Date, Ticker, Price. The header row is required and
// matched case-insensitively. Unknown columns are ignored.
func ParsePriceCSV(r io.Reader) (*PriceParseResult, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	result := &PriceParseResult{}

	// Read header row
	headers, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("empty CSV file: no header row")
		}
		return nil, fmt.Errorf("reading CSV header: %w", err)
	}

	cm := buildColumnMap(headers)

	// Verify required columns exist
	if _, ok := cm[strings.ToLower(priceCSVColDate)]; !ok {
		return nil, fmt.Errorf("CSV missing required column: %s", priceCSVColDate)
	}
	if _, ok := cm[strings.ToLower(priceCSVColTicker)]; !ok {
		return nil, fmt.Errorf("CSV missing required column: %s", priceCSVColTicker)
	}
	if _, ok := cm[strings.ToLower(priceCSVColPrice)]; !ok {
		return nil, fmt.Errorf("CSV missing required column: %s", priceCSVColPrice)
	}

	lineNum := 1 // Header is line 1
	for {
		lineNum++
		row, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			result.Errors = append(result.Errors, ParseError{
				Line:    lineNum,
				Message: fmt.Sprintf("reading row: %v", readErr),
			})
			continue
		}

		record, parseErr := parsePriceCSVRow(cm, row, lineNum)
		if parseErr != nil {
			result.Errors = append(result.Errors, *parseErr)
			continue
		}

		result.Records = append(result.Records, *record)
	}

	return result, nil
}

// parsePriceCSVRow parses a single price CSV data row into a PriceRecord.
func parsePriceCSVRow(cm columnMap, row []string, lineNum int) (*PriceRecord, *ParseError) {
	dateStr := cm.get(row, priceCSVColDate)
	tickerStr := cm.get(row, priceCSVColTicker)
	priceStr := cm.get(row, priceCSVColPrice)

	// Date is required
	if dateStr == "" {
		return nil, &ParseError{Line: lineNum, Message: "missing date"}
	}

	date, err := types.ParseDate(dateStr)
	if err != nil {
		return nil, &ParseError{Line: lineNum, Message: fmt.Sprintf("invalid date %q: %v", dateStr, err)}
	}

	// Ticker is required
	if tickerStr == "" {
		return nil, &ParseError{Line: lineNum, Message: "missing ticker"}
	}

	// Price is required
	if priceStr == "" {
		return nil, &ParseError{Line: lineNum, Message: "missing price"}
	}

	amount, err := types.NewMoney(priceStr)
	if err != nil {
		return nil, &ParseError{Line: lineNum, Message: fmt.Sprintf("invalid price %q: %v", priceStr, err)}
	}

	if amount.IsZero() || !amount.IsPositive() {
		return nil, &ParseError{Line: lineNum, Message: fmt.Sprintf("price must be positive, got %q", priceStr)}
	}

	return &PriceRecord{
		Date:       date,
		Ticker:     strings.ToUpper(strings.TrimSpace(tickerStr)),
		Price:      amount,
		SourceLine: lineNum,
	}, nil
}
