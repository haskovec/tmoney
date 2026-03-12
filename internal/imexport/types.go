package imexport

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/models"
)

// ImportRecord represents a single parsed transaction from an import file.
// This is a format-agnostic intermediate representation used between parsing
// and the import engine.
type ImportRecord struct {
	Date            models.Date
	Account         string
	Payee           string
	Category        string
	Amount          models.Money
	Memo            string
	CheckNumber     string
	Status          string
	TransferAccount string

	// Splits for split transactions (parent record has empty Category)
	Splits []ImportSplit

	// BankReferenceID for OFX FITID matching
	BankReferenceID string

	// SourceLine tracks the line number in the source file for error reporting
	SourceLine int
}

// ImportSplit represents a split line within a split transaction.
type ImportSplit struct {
	Category string
	Amount   models.Money
	Memo     string
}

// IsSplit returns true if this record has split lines.
func (r *ImportRecord) IsSplit() bool {
	return len(r.Splits) > 0
}

// ExportRecord represents a single transaction to be written to an export file.
// This is populated from the database before format-specific writing.
type ExportRecord struct {
	Date            string
	Account         string
	Payee           string
	Category        string
	Amount          string
	Memo            string
	CheckNumber     string
	Status          string
	TransferAccount string

	// Splits for split transactions
	Splits []ExportSplit
}

// ExportSplit represents a split line for export.
type ExportSplit struct {
	Category string
	Amount   string
	Memo     string
}

// ParseError represents an error encountered while parsing an import file.
type ParseError struct {
	Line    int
	Message string
}

// Error implements the error interface.
func (e *ParseError) Error() string {
	return fmt.Sprintf("line %d: %s", e.Line, e.Message)
}

// ParseResult holds the outcome of parsing an import file.
type ParseResult struct {
	Records []ImportRecord
	Errors  []ParseError
}

// HasErrors returns true if there were any parse errors.
func (r *ParseResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// Format represents a supported import/export file format.
type Format string

const (
	FormatCSV Format = "csv"
	FormatQIF Format = "qif"
	FormatOFX Format = "ofx"
)

// DetectFormat returns the format based on file extension.
func DetectFormat(filename string) (Format, error) {
	switch {
	case hasExtension(filename, ".csv"):
		return FormatCSV, nil
	case hasExtension(filename, ".qif"):
		return FormatQIF, nil
	case hasExtension(filename, ".ofx"), hasExtension(filename, ".qfx"):
		return FormatOFX, nil
	default:
		return "", fmt.Errorf("unable to detect format from file extension: %s", filename)
	}
}

func hasExtension(filename, ext string) bool {
	if len(filename) < len(ext) {
		return false
	}
	suffix := filename[len(filename)-len(ext):]
	// Case-insensitive comparison
	for i := range suffix {
		c := suffix[i]
		e := ext[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if e >= 'A' && e <= 'Z' {
			e += 'a' - 'A'
		}
		if c != e {
			return false
		}
	}
	return true
}
