package imexport

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/haskovec/tmoney/internal/types"
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

	// Rows that may fold into one split transaction are held back in `pending`
	// until the run ends, because the decision needs the whole run: see
	// splitCandidate.flush.
	var pending *splitCandidate

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

		if pending != nil && isSplitContinuation(pending.parent, *record) {
			pending.lines = append(pending.lines, *record)
			continue
		}

		if pending != nil {
			result.Records = pending.flush(result.Records)
			pending = nil
		}

		// A row with no category is the only thing that can open a split run.
		if record.Category == "" {
			pending = &splitCandidate{parent: *record}
			continue
		}
		result.Records = append(result.Records, *record)
	}

	if pending != nil {
		result.Records = pending.flush(result.Records)
	}

	return result, nil
}

// splitCandidate is a run of rows that has the SHAPE the exporter writes for a
// split transaction (WriteCSV): a parent row with a blank category, then one row
// per split line repeating the parent's date, account and payee.
type splitCandidate struct {
	parent ImportRecord
	lines  []ImportRecord
}

// flush appends the run to records, folded into one split transaction only if
// the split amounts sum to the parent's amount, else as independent records.
//
// The sum check is what stops two ordinary transactions from being merged. The
// shape alone is not enough evidence: two uncategorized ATM withdrawals on one
// day share date, account and payee, and so do a same-day uncategorized purchase
// and a categorized one from the same shop. Both were folded into a bogus split
// before, and the split then failed validation (or worse, passed with the
// wrong total). A genuine export always sums exactly, so a mismatch means these
// rows were never one transaction.
func (c *splitCandidate) flush(records []ImportRecord) []ImportRecord {
	if len(c.lines) == 0 {
		return append(records, c.parent)
	}

	total := types.ZeroMoney
	for _, line := range c.lines {
		total = total.Add(line.Amount)
	}
	if !total.Equal(c.parent.Amount) {
		records = append(records, c.parent)
		return append(records, c.lines...)
	}

	parent := c.parent
	parent.Splits = make([]ImportSplit, 0, len(c.lines))
	for _, line := range c.lines {
		parent.Splits = append(parent.Splits, ImportSplit{
			Category: line.Category,
			Amount:   line.Amount,
			Memo:     line.Memo,
		})
	}
	return append(records, parent)
}

// parseCSVRow parses a single CSV data row into an ImportRecord.
func parseCSVRow(cm columnMap, row []string, lineNum int) (*ImportRecord, *ParseError) {
	dateStr := cm.get(row, csvColDate)
	amountStr := cm.get(row, csvColAmount)

	// Date is required
	if dateStr == "" {
		return nil, &ParseError{Line: lineNum, Message: "missing date"}
	}

	date, err := types.ParseDate(dateStr)
	if err != nil {
		return nil, &ParseError{Line: lineNum, Message: fmt.Sprintf("invalid date %q: %v", dateStr, err)}
	}

	// Amount is required
	if amountStr == "" {
		return nil, &ParseError{Line: lineNum, Message: "missing amount"}
	}

	amount, err := types.NewMoney(amountStr)
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

// isSplitContinuation returns true if record has the shape of a split line
// under parent: the same date, account and payee, and a category of its own. A
// split line without a category is meaningless, so a second uncategorized row
// is a new transaction, not a continuation. The amount check that completes the
// decision runs over the whole run in splitCandidate.flush.
func isSplitContinuation(parent ImportRecord, record ImportRecord) bool {
	if record.Category == "" {
		return false
	}
	if !parent.Date.Equal(record.Date) {
		return false
	}
	if parent.Account != record.Account {
		return false
	}
	if parent.Payee != record.Payee {
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
