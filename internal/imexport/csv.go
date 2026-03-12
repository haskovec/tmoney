package imexport

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/haskovec/tmoney/internal/models"
)

// CSV column headers as defined in the spec.
const (
	csvColDate            = "Date"
	csvColAccount         = "Account"
	csvColPayee           = "Payee"
	csvColCategory        = "Category"
	csvColAmount          = "Amount"
	csvColMemo            = "Memo"
	csvColCheckNumber     = "Check Number"
	csvColStatus          = "Status"
	csvColTransferAccount = "Transfer Account"
)

// csvHeaders is the standard column order for export.
var csvHeaders = []string{
	csvColDate,
	csvColAccount,
	csvColPayee,
	csvColCategory,
	csvColAmount,
	csvColMemo,
	csvColCheckNumber,
	csvColStatus,
	csvColTransferAccount,
}

// columnMap maps header names to their column index.
type columnMap map[string]int

// buildColumnMap creates a case-insensitive mapping from header names to column indices.
func buildColumnMap(headers []string) columnMap {
	m := make(columnMap, len(headers))
	for i, h := range headers {
		m[strings.ToLower(strings.TrimSpace(h))] = i
	}
	return m
}

// get returns the value at the named column, or empty string if not present.
func (cm columnMap) get(row []string, name string) string {
	idx, ok := cm[strings.ToLower(name)]
	if !ok || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

// ParseCSV reads a CSV file from r and returns parsed import records.
// The CSV must have a header row. Columns are matched by name (case-insensitive).
// Unknown columns are ignored. Missing optional columns are allowed.
func ParseCSV(r io.Reader) (*ParseResult, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1 // Allow variable field count
	reader.LazyQuotes = true    // Be lenient with quoting

	result := &ParseResult{}

	// Read header row
	headers, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("empty CSV file: no header row")
		}
		return nil, fmt.Errorf("reading CSV header: %w", err)
	}

	cm := buildColumnMap(headers)

	// Verify at minimum Date and Amount columns exist
	if _, ok := cm[strings.ToLower(csvColDate)]; !ok {
		return nil, fmt.Errorf("CSV missing required column: %s", csvColDate)
	}
	if _, ok := cm[strings.ToLower(csvColAmount)]; !ok {
		return nil, fmt.Errorf("CSV missing required column: %s", csvColAmount)
	}

	lineNum := 1 // Header is line 1
	for {
		lineNum++
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Errors = append(result.Errors, ParseError{
				Line:    lineNum,
				Message: fmt.Sprintf("reading row: %v", err),
			})
			continue
		}

		record, parseErr := parseCSVRow(cm, row, lineNum)
		if parseErr != nil {
			result.Errors = append(result.Errors, *parseErr)
			continue
		}

		// Check if this row is a split continuation of the previous record.
		// Split rows share the same date, payee, and account as the parent,
		// but the parent has an empty category and subsequent rows provide
		// split categories and amounts.
		if len(result.Records) > 0 && isSplitContinuation(result.Records[len(result.Records)-1], *record) {
			parent := &result.Records[len(result.Records)-1]
			parent.Splits = append(parent.Splits, ImportSplit{
				Category: record.Category,
				Amount:   record.Amount,
				Memo:     record.Memo,
			})
			continue
		}

		result.Records = append(result.Records, *record)
	}

	return result, nil
}

// parseCSVRow parses a single CSV data row into an ImportRecord.
func parseCSVRow(cm columnMap, row []string, lineNum int) (*ImportRecord, *ParseError) {
	dateStr := cm.get(row, csvColDate)
	amountStr := cm.get(row, csvColAmount)

	// Date is required
	if dateStr == "" {
		return nil, &ParseError{Line: lineNum, Message: "missing date"}
	}

	date, err := models.ParseDate(dateStr)
	if err != nil {
		return nil, &ParseError{Line: lineNum, Message: fmt.Sprintf("invalid date %q: %v", dateStr, err)}
	}

	// Amount is required
	if amountStr == "" {
		return nil, &ParseError{Line: lineNum, Message: "missing amount"}
	}

	amount, err := models.NewMoney(amountStr)
	if err != nil {
		return nil, &ParseError{Line: lineNum, Message: fmt.Sprintf("invalid amount %q: %v", amountStr, err)}
	}

	return &ImportRecord{
		Date:            date,
		Account:         cm.get(row, csvColAccount),
		Payee:           cm.get(row, csvColPayee),
		Category:        cm.get(row, csvColCategory),
		Amount:          amount,
		Memo:            cm.get(row, csvColMemo),
		CheckNumber:     cm.get(row, csvColCheckNumber),
		Status:          cm.get(row, csvColStatus),
		TransferAccount: cm.get(row, csvColTransferAccount),
		SourceLine:      lineNum,
	}, nil
}

// isSplitContinuation returns true if record looks like a split continuation
// of prev: same date and payee, and prev has empty category (indicating it's
// a split parent).
func isSplitContinuation(prev ImportRecord, record ImportRecord) bool {
	if prev.Category != "" {
		return false
	}
	if !prev.Date.Equal(record.Date) {
		return false
	}
	if prev.Account != record.Account {
		return false
	}
	if prev.Payee != record.Payee {
		return false
	}
	return true
}

// WriteCSV writes export records to w in CSV format with a header row.
// Split transactions are written as multiple rows: the parent row with
// empty category, followed by one row per split with the split's category,
// amount, and memo. Parent fields (date, payee, account) are repeated.
func WriteCSV(w io.Writer, records []ExportRecord) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write header
	if err := writer.Write(csvHeaders); err != nil {
		return fmt.Errorf("writing CSV header: %w", err)
	}

	for _, rec := range records {
		if len(rec.Splits) > 0 {
			// Write parent row with empty category
			if err := writer.Write(exportRecordToRow(rec, "", rec.Amount, rec.Memo)); err != nil {
				return fmt.Errorf("writing CSV row: %w", err)
			}
			// Write split rows
			for _, split := range rec.Splits {
				if err := writer.Write(exportRecordToRow(rec, split.Category, split.Amount, split.Memo)); err != nil {
					return fmt.Errorf("writing CSV split row: %w", err)
				}
			}
		} else {
			if err := writer.Write(exportRecordToRow(rec, rec.Category, rec.Amount, rec.Memo)); err != nil {
				return fmt.Errorf("writing CSV row: %w", err)
			}
		}
	}

	return writer.Error()
}

// exportRecordToRow converts an ExportRecord to a CSV row with the given
// category, amount, and memo (to support split rows).
func exportRecordToRow(rec ExportRecord, category, amount, memo string) []string {
	return []string{
		rec.Date,
		rec.Account,
		rec.Payee,
		category,
		amount,
		memo,
		rec.CheckNumber,
		rec.Status,
		rec.TransferAccount,
	}
}
