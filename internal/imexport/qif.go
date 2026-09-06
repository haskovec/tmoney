package imexport

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

// QIF account type headers mapped from TMoney account types.
const (
	qifTypeBank  = "Bank"
	qifTypeCCard = "CCard"
	qifTypeInvst = "Invst"
	qifTypeCash  = "Cash"
	qifTypeOthL  = "Oth L"
	qifTypeOthA  = "Oth A"
)

// qifDateFormats lists the date formats to try when parsing QIF dates.
var qifDateFormats = []string{
	"01/02/2006", // MM/DD/YYYY (most common)
	"1/2/2006",   // M/D/YYYY
	"01/02/06",   // MM/DD/YY
	"1/2/06",     // M/D/YY
	"01-02-2006", // MM-DD-YYYY
	"1-2-2006",   // M-D-YYYY
	"01/02'2006", // MM/DD'YYYY (Quicken variant)
	"1/2'2006",   // M/D'YYYY
	"2006-01-02", // YYYY-MM-DD (ISO)
	"01/02'06",   // MM/DD'YY
	"1/2'06",     // M/D'YY
}

// QIF field code prefixes.
const (
	qifFieldTypeHeader = "!Type:"
	qifFieldDate       = "D"
	qifFieldAmount     = "T"
	qifFieldPayee      = "P"
	qifFieldCategory   = "L"
	qifFieldMemo       = "M"
	qifFieldCleared    = "C"
	qifFieldCheckNum   = "N"
	qifFieldSplitCat   = "S"
	qifFieldSplitAmt   = "$"
	qifFieldSplitMemo  = "E"
	qifRecordEnd       = "^"
)

// parseQIFDate tries multiple date formats to parse a QIF date string.
func parseQIFDate(s string) (types.Date, error) {
	s = strings.TrimSpace(s)
	for _, format := range qifDateFormats {
		t, err := time.Parse(format, s)
		if err == nil {
			return types.Date(t), nil
		}
	}
	return types.ZeroDate, fmt.Errorf("unable to parse QIF date %q", s)
}

// formatQIFDate formats a date string from YYYY-MM-DD to MM/DD/YYYY for QIF output.
func formatQIFDate(isoDate string) string {
	t, err := time.Parse("2006-01-02", isoDate)
	if err != nil {
		return isoDate
	}
	return t.Format("01/02/2006")
}

// qifStatusToImport converts a QIF `C` field to an import status code, following
// Quicken: `*`, `c` or `C` is cleared; `X` or `R` is reconciled.
func qifStatusToImport(s string) string {
	switch strings.TrimSpace(s) {
	case "*", "c", "C":
		return "C" // Cleared
	case "X", "x", "R", "r":
		return "R" // Reconciled
	default:
		return "U" // Uncleared
	}
}

// exportStatusToQIF converts an export status code to a QIF `C` field value.
// See qifStatusToImport for the convention.
func exportStatusToQIF(s string) string {
	switch s {
	case "C":
		return "*"
	case "R":
		return "X"
	default:
		return ""
	}
}

// ParseQIF reads a QIF file from r and returns parsed import records.
// QIF records are delimited by ^ characters. Field codes are single-character
// prefixes on each line (D=date, T=amount, P=payee, etc.).
func ParseQIF(r io.Reader) (*ParseResult, error) {
	scanner := bufio.NewScanner(r)
	result := &ParseResult{}

	lineNum := 0
	recordStartLine := 0

	// Current record fields being accumulated
	var (
		dateStr      string
		amountStr    string
		payee        string
		category     string
		memo         string
		cleared      string
		checkNum     string
		splits       []ImportSplit
		curSplitCat  string
		curSplitAmt  string
		curSplitMemo string
		inRecord     bool
	)

	flushSplit := func() {
		if curSplitCat != "" || curSplitAmt != "" {
			amt, err := types.NewMoney(curSplitAmt)
			if err != nil {
				amt = types.MustNewMoney("0")
			}
			splits = append(splits, ImportSplit{
				Category: curSplitCat,
				Amount:   amt,
				Memo:     curSplitMemo,
			})
			curSplitCat = ""
			curSplitAmt = ""
			curSplitMemo = ""
		}
	}

	resetRecord := func() {
		dateStr = ""
		amountStr = ""
		payee = ""
		category = ""
		memo = ""
		cleared = ""
		checkNum = ""
		splits = nil
		curSplitCat = ""
		curSplitAmt = ""
		curSplitMemo = ""
		inRecord = false
	}

	finishRecord := func() {
		if !inRecord {
			return
		}

		// Flush any pending split
		flushSplit()

		// Date is required
		if dateStr == "" {
			result.Errors = append(result.Errors, ParseError{
				Line:    recordStartLine,
				Message: "missing date",
			})
			resetRecord()
			return
		}

		date, err := parseQIFDate(dateStr)
		if err != nil {
			result.Errors = append(result.Errors, ParseError{
				Line:    recordStartLine,
				Message: fmt.Sprintf("invalid date %q: %v", dateStr, err),
			})
			resetRecord()
			return
		}

		// Amount is required
		if amountStr == "" {
			result.Errors = append(result.Errors, ParseError{
				Line:    recordStartLine,
				Message: "missing amount",
			})
			resetRecord()
			return
		}

		amount, err := types.NewMoney(amountStr)
		if err != nil {
			result.Errors = append(result.Errors, ParseError{
				Line:    recordStartLine,
				Message: fmt.Sprintf("invalid amount %q: %v", amountStr, err),
			})
			resetRecord()
			return
		}

		// Detect transfers: category like [Account Name]
		transferAccount := ""
		actualCategory := category
		if strings.HasPrefix(category, "[") && strings.HasSuffix(category, "]") {
			transferAccount = category[1 : len(category)-1]
			actualCategory = ""
		}

		status := qifStatusToImport(cleared)

		rec := ImportRecord{
			Date:            date,
			Payee:           payee,
			Category:        actualCategory,
			Amount:          amount,
			Memo:            memo,
			CheckNumber:     checkNum,
			Status:          status,
			TransferAccount: transferAccount,
			SourceLine:      recordStartLine,
		}

		if len(splits) > 0 {
			rec.Category = "" // Parent has empty category for splits
			rec.Splits = splits
		}

		result.Records = append(result.Records, rec)
		resetRecord()
	}

	for scanner.Scan() {
		lineNum++
		line := strings.TrimRight(scanner.Text(), "\r")

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Skip type headers
		if strings.HasPrefix(line, qifFieldTypeHeader) {
			continue
		}

		// End of record
		if line == qifRecordEnd {
			finishRecord()
			continue
		}

		if !inRecord {
			inRecord = true
			recordStartLine = lineNum
		}

		if len(line) < 1 {
			continue
		}

		code := string(line[0])
		value := line[1:]

		switch code {
		case qifFieldDate:
			dateStr = strings.TrimSpace(value)
		case qifFieldAmount:
			// Strip commas from amounts (e.g., "1,500.00" -> "1500.00")
			amountStr = strings.ReplaceAll(strings.TrimSpace(value), ",", "")
		case qifFieldPayee:
			payee = strings.TrimSpace(value)
		case qifFieldCategory:
			category = strings.TrimSpace(value)
		case qifFieldMemo:
			memo = strings.TrimSpace(value)
		case qifFieldCleared:
			cleared = strings.TrimSpace(value)
		case qifFieldCheckNum:
			checkNum = strings.TrimSpace(value)
		case qifFieldSplitCat:
			// Flush previous split if any
			flushSplit()
			curSplitCat = strings.TrimSpace(value)
		case qifFieldSplitAmt:
			curSplitAmt = strings.ReplaceAll(strings.TrimSpace(value), ",", "")
		case qifFieldSplitMemo:
			curSplitMemo = strings.TrimSpace(value)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading QIF file: %w", err)
	}

	// Handle final record without trailing ^
	finishRecord()

	return result, nil
}

// WriteQIF writes export records to w in QIF format.
// Each record is written with the appropriate field codes and terminated by ^.
func WriteQIF(w io.Writer, records []ExportRecord, accountType string) error {
	bw := bufio.NewWriter(w)

	// Write type header
	qifType := accountTypeToQIF(accountType)
	if _, err := fmt.Fprintf(bw, "!Type:%s\n", qifType); err != nil {
		return fmt.Errorf("writing QIF type header: %w", err)
	}

	for _, rec := range records {
		// Date (required)
		if _, err := fmt.Fprintf(bw, "D%s\n", formatQIFDate(rec.Date)); err != nil {
			return fmt.Errorf("writing QIF record: %w", err)
		}

		// Amount (required)
		if _, err := fmt.Fprintf(bw, "T%s\n", rec.Amount); err != nil {
			return fmt.Errorf("writing QIF record: %w", err)
		}

		// Payee
		if rec.Payee != "" {
			if _, err := fmt.Fprintf(bw, "P%s\n", rec.Payee); err != nil {
				return fmt.Errorf("writing QIF record: %w", err)
			}
		}

		// Category or transfer
		cat := rec.Category
		if rec.TransferAccount != "" {
			cat = "[" + rec.TransferAccount + "]"
		}
		if cat != "" {
			if _, err := fmt.Fprintf(bw, "L%s\n", cat); err != nil {
				return fmt.Errorf("writing QIF record: %w", err)
			}
		}

		// Memo
		if rec.Memo != "" {
			if _, err := fmt.Fprintf(bw, "M%s\n", rec.Memo); err != nil {
				return fmt.Errorf("writing QIF record: %w", err)
			}
		}

		// Cleared status
		qifStatus := exportStatusToQIF(rec.Status)
		if qifStatus != "" {
			if _, err := fmt.Fprintf(bw, "C%s\n", qifStatus); err != nil {
				return fmt.Errorf("writing QIF record: %w", err)
			}
		}

		// Check number
		if rec.CheckNumber != "" {
			if _, err := fmt.Fprintf(bw, "N%s\n", rec.CheckNumber); err != nil {
				return fmt.Errorf("writing QIF record: %w", err)
			}
		}

		// Splits
		for _, split := range rec.Splits {
			if _, err := fmt.Fprintf(bw, "S%s\n", split.Category); err != nil {
				return fmt.Errorf("writing QIF split: %w", err)
			}
			if _, err := fmt.Fprintf(bw, "$%s\n", split.Amount); err != nil {
				return fmt.Errorf("writing QIF split: %w", err)
			}
			if split.Memo != "" {
				if _, err := fmt.Fprintf(bw, "E%s\n", split.Memo); err != nil {
					return fmt.Errorf("writing QIF split: %w", err)
				}
			}
		}

		// Record separator
		if _, err := fmt.Fprintln(bw, "^"); err != nil {
			return fmt.Errorf("writing QIF record separator: %w", err)
		}
	}

	return bw.Flush()
}

// accountTypeToQIF maps TMoney account types to QIF type headers.
func accountTypeToQIF(accountType string) string {
	switch strings.ToLower(accountType) {
	case "credit_card":
		return qifTypeCCard
	case "investment":
		return qifTypeInvst
	case "cash":
		return qifTypeCash
	case "loan":
		return qifTypeOthL
	case "asset":
		return qifTypeOthA
	default:
		// checking, savings, and anything else default to Bank
		return qifTypeBank
	}
}
