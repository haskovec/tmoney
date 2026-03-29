package imexport

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

// ParseOFX reads an OFX/QFX file from r and returns parsed import records.
// OFX files use SGML-like markup. This parser handles both OFX 1.x (SGML)
// and OFX 2.x (XML) formats by extracting tag content line-by-line.
func ParseOFX(r io.Reader) (*ParseResult, error) {
	scanner := bufio.NewScanner(r)
	result := &ParseResult{}

	// Accumulate the full content for tag-based parsing
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading OFX file: %w", err)
	}

	content := strings.Join(lines, "\n")

	// Extract all STMTTRN blocks (statement transactions)
	transactions := extractOFXBlocks(content, "STMTTRN")
	if len(transactions) == 0 {
		return result, nil
	}

	for i, txnBlock := range transactions {
		record, parseErr := parseOFXTransaction(txnBlock, i+1)
		if parseErr != nil {
			result.Errors = append(result.Errors, *parseErr)
			continue
		}
		result.Records = append(result.Records, *record)
	}

	return result, nil
}

// parseOFXTransaction parses a single STMTTRN block into an ImportRecord.
func parseOFXTransaction(block string, index int) (*ImportRecord, *ParseError) {
	dtPosted := extractOFXValue(block, "DTPOSTED")
	trnAmt := extractOFXValue(block, "TRNAMT")
	name := extractOFXValue(block, "NAME")
	memo := extractOFXValue(block, "MEMO")
	fitID := extractOFXValue(block, "FITID")
	checkNum := extractOFXValue(block, "CHECKNUM")

	// Date is required
	if dtPosted == "" {
		return nil, &ParseError{
			Line:    index,
			Message: "missing DTPOSTED in transaction",
		}
	}

	date, err := parseOFXDate(dtPosted)
	if err != nil {
		return nil, &ParseError{
			Line:    index,
			Message: fmt.Sprintf("invalid date %q: %v", dtPosted, err),
		}
	}

	// Amount is required
	if trnAmt == "" {
		return nil, &ParseError{
			Line:    index,
			Message: "missing TRNAMT in transaction",
		}
	}

	amount, err := types.NewMoney(trnAmt)
	if err != nil {
		return nil, &ParseError{
			Line:    index,
			Message: fmt.Sprintf("invalid amount %q: %v", trnAmt, err),
		}
	}

	// Use NAME for payee; fall back to MEMO if NAME is empty
	payee := name
	payeeMemo := memo
	if payee == "" {
		payee = memo
		payeeMemo = ""
	}

	return &ImportRecord{
		Date:            date,
		Payee:           payee,
		Amount:          amount,
		Memo:            payeeMemo,
		CheckNumber:     checkNum,
		Status:          "U", // OFX imports are uncleared
		BankReferenceID: fitID,
		SourceLine:      index,
	}, nil
}

// extractOFXBlocks extracts all content blocks between <tag> and </tag>.
// For SGML-style OFX where closing tags may be absent, it extracts content
// between consecutive opening <tag> markers or until the parent closing tag.
func extractOFXBlocks(content, tag string) []string {
	openTag := "<" + tag + ">"
	closeTag := "</" + tag + ">"
	upper := strings.ToUpper(content)
	openTagUpper := strings.ToUpper(openTag)
	closeTagUpper := strings.ToUpper(closeTag)

	var blocks []string
	searchFrom := 0

	for {
		// Find opening tag (case-insensitive)
		startIdx := indexCaseInsensitive(upper, openTagUpper, searchFrom)
		if startIdx == -1 {
			break
		}

		blockStart := startIdx + len(openTag)

		// Look for closing tag first
		closeIdx := indexCaseInsensitive(upper, closeTagUpper, blockStart)

		// Also look for next opening tag (SGML style may not have closing tags)
		nextOpenIdx := indexCaseInsensitive(upper, openTagUpper, blockStart)

		var blockEnd int
		if closeIdx != -1 && (nextOpenIdx == -1 || closeIdx < nextOpenIdx) {
			blockEnd = closeIdx
			searchFrom = closeIdx + len(closeTag)
		} else if nextOpenIdx != -1 {
			blockEnd = nextOpenIdx
			searchFrom = nextOpenIdx
		} else {
			// Take everything to end
			blockEnd = len(content)
			searchFrom = blockEnd
		}

		blocks = append(blocks, content[blockStart:blockEnd])
	}

	return blocks
}

// extractOFXValue extracts the value of a simple OFX tag from a block.
// Handles both SGML-style (<TAG>value) and XML-style (<TAG>value</TAG>).
func extractOFXValue(block, tag string) string {
	openTag := "<" + tag + ">"
	closeTag := "</" + tag + ">"
	upper := strings.ToUpper(block)
	openTagUpper := strings.ToUpper(openTag)
	closeTagUpper := strings.ToUpper(closeTag)

	startIdx := indexCaseInsensitive(upper, openTagUpper, 0)
	if startIdx == -1 {
		return ""
	}

	valueStart := startIdx + len(openTag)

	// Check for XML-style closing tag
	closeIdx := indexCaseInsensitive(upper, closeTagUpper, valueStart)

	// Also check for next opening tag (end of SGML value)
	nextTagIdx := strings.Index(upper[valueStart:], "<")
	if nextTagIdx != -1 {
		nextTagIdx += valueStart
	}

	var valueEnd int
	if closeIdx != -1 && (nextTagIdx == -1 || closeIdx <= nextTagIdx) {
		valueEnd = closeIdx
	} else if nextTagIdx != -1 {
		valueEnd = nextTagIdx
	} else {
		// Value goes to end of block
		valueEnd = len(block)
	}

	value := strings.TrimSpace(block[valueStart:valueEnd])

	// Strip any newlines within the value
	value = strings.ReplaceAll(value, "\n", "")
	value = strings.ReplaceAll(value, "\r", "")

	return value
}

// indexCaseInsensitive finds the first occurrence of substr in s starting at fromIndex.
// Both s and substr should already be uppercased for case-insensitive matching.
func indexCaseInsensitive(s, substr string, fromIndex int) int {
	if fromIndex >= len(s) {
		return -1
	}
	idx := strings.Index(s[fromIndex:], substr)
	if idx == -1 {
		return -1
	}
	return fromIndex + idx
}

// parseOFXDate parses OFX date formats.
// OFX dates can be:
//   - YYYYMMDD
//   - YYYYMMDDHHMMSS
//   - YYYYMMDDHHMMSS.XXX (with milliseconds)
//   - YYYYMMDDHHMMSS.XXX[TZ:TZNAME] (with timezone)
//   - YYYYMMDD[TZ:TZNAME]
func parseOFXDate(s string) (types.Date, error) {
	s = strings.TrimSpace(s)

	// Strip timezone info in brackets: [GMT-5:EST]
	if bracketIdx := strings.Index(s, "["); bracketIdx != -1 {
		s = s[:bracketIdx]
	}

	// Strip fractional seconds
	if dotIdx := strings.Index(s, "."); dotIdx != -1 {
		s = s[:dotIdx]
	}

	s = strings.TrimSpace(s)

	// Try YYYYMMDDHHMMSS — truncate to date only
	if len(s) >= 14 {
		_, err := time.Parse("20060102150405", s[:14])
		if err == nil {
			// Parse date-only portion to get a clean date without time
			t, _ := time.Parse("20060102", s[:8])
			return types.Date(t), nil
		}
	}

	// Try YYYYMMDD
	if len(s) >= 8 {
		t, err := time.Parse("20060102", s[:8])
		if err == nil {
			return types.Date(t), nil
		}
	}

	return types.ZeroDate, fmt.Errorf("unable to parse OFX date %q", s)
}
