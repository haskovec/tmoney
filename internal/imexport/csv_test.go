package imexport

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

func TestParseCSV_BasicTransactions(t *testing.T) {
	input := `Date,Account,Payee,Category,Amount,Memo,Check Number,Status,Transfer Account
2024-01-15,Checking,Kroger,Food:Groceries,-125.43,Weekly groceries,,C,
2024-01-16,Checking,Employer Inc,Income:Salary,3500.00,January paycheck,,C,
2024-01-17,Checking,Electric Co,Bills:Utilities,-95.00,Electric bill,1234,U,
`

	result, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseCSV() had errors: %v", result.Errors)
	}

	if len(result.Records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(result.Records))
	}

	// Verify first record
	rec := result.Records[0]
	if rec.Date.String() != "2024-01-15" {
		t.Errorf("record 0 date = %s, want 2024-01-15", rec.Date.String())
	}
	if rec.Account != "Checking" {
		t.Errorf("record 0 account = %q, want %q", rec.Account, "Checking")
	}
	if rec.Payee != "Kroger" {
		t.Errorf("record 0 payee = %q, want %q", rec.Payee, "Kroger")
	}
	if rec.Category != "Food:Groceries" {
		t.Errorf("record 0 category = %q, want %q", rec.Category, "Food:Groceries")
	}
	expectedAmt := types.MustNewMoney("-125.43")
	if !rec.Amount.Equal(expectedAmt) {
		t.Errorf("record 0 amount = %s, want %s", rec.Amount.String(), expectedAmt.String())
	}
	if rec.Memo != "Weekly groceries" {
		t.Errorf("record 0 memo = %q, want %q", rec.Memo, "Weekly groceries")
	}
	if rec.Status != "C" {
		t.Errorf("record 0 status = %q, want %q", rec.Status, "C")
	}

	// Verify third record has check number
	rec2 := result.Records[2]
	if rec2.CheckNumber != "1234" {
		t.Errorf("record 2 check number = %q, want %q", rec2.CheckNumber, "1234")
	}
	if rec2.Status != "U" {
		t.Errorf("record 2 status = %q, want %q", rec2.Status, "U")
	}
}

func TestParseCSV_SplitTransactions(t *testing.T) {
	input := `Date,Account,Payee,Category,Amount,Memo,Check Number,Status,Transfer Account
2024-01-15,Checking,Kroger,,-150.00,,,C,
2024-01-15,Checking,Kroger,Food:Groceries,-120.00,Food items,,C,
2024-01-15,Checking,Kroger,Household:Cleaning,-30.00,Cleaning supplies,,C,
`

	result, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseCSV() had errors: %v", result.Errors)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record (parent with splits), got %d", len(result.Records))
	}

	rec := result.Records[0]
	if !rec.IsSplit() {
		t.Fatal("expected record to be a split transaction")
	}
	if len(rec.Splits) != 2 {
		t.Fatalf("expected 2 splits, got %d", len(rec.Splits))
	}

	if rec.Splits[0].Category != "Food:Groceries" {
		t.Errorf("split 0 category = %q, want %q", rec.Splits[0].Category, "Food:Groceries")
	}
	expectedAmt := types.MustNewMoney("-120.00")
	if !rec.Splits[0].Amount.Equal(expectedAmt) {
		t.Errorf("split 0 amount = %s, want %s", rec.Splits[0].Amount.String(), expectedAmt.String())
	}
	if rec.Splits[0].Memo != "Food items" {
		t.Errorf("split 0 memo = %q, want %q", rec.Splits[0].Memo, "Food items")
	}

	if rec.Splits[1].Category != "Household:Cleaning" {
		t.Errorf("split 1 category = %q, want %q", rec.Splits[1].Category, "Household:Cleaning")
	}
}

func TestParseCSV_CaseInsensitiveHeaders(t *testing.T) {
	input := `date,account,payee,category,amount,memo,check number,status,transfer account
2024-01-15,Checking,Kroger,Food,-50.00,,,,
`

	result, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseCSV() had errors: %v", result.Errors)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	if result.Records[0].Payee != "Kroger" {
		t.Errorf("payee = %q, want %q", result.Records[0].Payee, "Kroger")
	}
}

func TestParseCSV_MissingOptionalColumns(t *testing.T) {
	// Only required columns (Date, Amount) plus Payee
	input := `Date,Amount,Payee
2024-01-15,-50.00,Kroger
2024-01-16,3500.00,Employer
`

	result, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseCSV() had errors: %v", result.Errors)
	}

	if len(result.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(result.Records))
	}

	// Optional fields should be empty
	rec := result.Records[0]
	if rec.Account != "" {
		t.Errorf("account should be empty, got %q", rec.Account)
	}
	if rec.Category != "" {
		t.Errorf("category should be empty, got %q", rec.Category)
	}
	if rec.Memo != "" {
		t.Errorf("memo should be empty, got %q", rec.Memo)
	}
}

func TestParseCSV_UnknownColumnsIgnored(t *testing.T) {
	input := `Date,Amount,Payee,CustomField,AnotherOne
2024-01-15,-50.00,Kroger,foo,bar
`

	result, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseCSV() had errors: %v", result.Errors)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
}

func TestParseCSV_EmptyFile(t *testing.T) {
	_, err := ParseCSV(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty CSV file")
	}
}

func TestParseCSV_HeaderOnly(t *testing.T) {
	input := "Date,Amount,Payee\n"

	result, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}

	if len(result.Records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(result.Records))
	}
}

func TestParseCSV_MissingRequiredDateColumn(t *testing.T) {
	input := "Amount,Payee\n-50.00,Kroger\n"

	_, err := ParseCSV(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing Date column")
	}
}

func TestParseCSV_MissingRequiredAmountColumn(t *testing.T) {
	input := "Date,Payee\n2024-01-15,Kroger\n"

	_, err := ParseCSV(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing Amount column")
	}
}

func TestParseCSV_InvalidDate(t *testing.T) {
	input := `Date,Amount
not-a-date,-50.00
`

	result, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}

	if !result.HasErrors() {
		t.Fatal("expected parse errors for invalid date")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
	if result.Errors[0].Line != 2 {
		t.Errorf("error line = %d, want 2", result.Errors[0].Line)
	}
}

func TestParseCSV_InvalidAmount(t *testing.T) {
	input := `Date,Amount
2024-01-15,not-a-number
`

	result, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}

	if !result.HasErrors() {
		t.Fatal("expected parse errors for invalid amount")
	}
}

func TestParseCSV_MixedValidAndInvalidRows(t *testing.T) {
	input := `Date,Amount,Payee
2024-01-15,-50.00,Kroger
bad-date,-30.00,Store
2024-01-17,-75.00,Target
`

	result, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}

	if len(result.Records) != 2 {
		t.Fatalf("expected 2 valid records, got %d", len(result.Records))
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
}

func TestParseCSV_TransferAccount(t *testing.T) {
	input := `Date,Account,Payee,Category,Amount,Memo,Check Number,Status,Transfer Account
2024-01-15,Checking,Transfer,,-500.00,,,C,Savings
`

	result, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseCSV() had errors: %v", result.Errors)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	if result.Records[0].TransferAccount != "Savings" {
		t.Errorf("transfer account = %q, want %q", result.Records[0].TransferAccount, "Savings")
	}
}

func TestParseCSV_QuotedFields(t *testing.T) {
	input := `Date,Amount,Payee,Memo
2024-01-15,-50.00,"Kroger Store #123","Groceries, cleaning supplies"
`

	result, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseCSV() had errors: %v", result.Errors)
	}

	rec := result.Records[0]
	if rec.Payee != "Kroger Store #123" {
		t.Errorf("payee = %q, want %q", rec.Payee, "Kroger Store #123")
	}
	if rec.Memo != "Groceries, cleaning supplies" {
		t.Errorf("memo = %q, want %q", rec.Memo, "Groceries, cleaning supplies")
	}
}

func TestParseCSV_WhitespaceInHeaders(t *testing.T) {
	input := ` Date , Amount , Payee
2024-01-15,-50.00,Kroger
`

	result, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseCSV() had errors: %v", result.Errors)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
}

func TestParseCSV_SourceLineTracking(t *testing.T) {
	input := `Date,Amount,Payee
2024-01-15,-50.00,Kroger
2024-01-16,-30.00,Target
`

	result, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}

	if result.Records[0].SourceLine != 2 {
		t.Errorf("record 0 source line = %d, want 2", result.Records[0].SourceLine)
	}
	if result.Records[1].SourceLine != 3 {
		t.Errorf("record 1 source line = %d, want 3", result.Records[1].SourceLine)
	}
}

// WriteCSV tests

func TestWriteCSV_BasicTransactions(t *testing.T) {
	records := []ExportRecord{
		{
			Date:     "2024-01-15",
			Account:  "Checking",
			Payee:    "Kroger",
			Category: "Food:Groceries",
			Amount:   "-125.43",
			Memo:     "Weekly groceries",
			Status:   "C",
		},
		{
			Date:    "2024-01-16",
			Account: "Checking",
			Payee:   "Employer Inc",
			Category: "Income:Salary",
			Amount:  "3500.00",
			Status:  "C",
		},
	}

	var buf bytes.Buffer
	err := WriteCSV(&buf, records)
	if err != nil {
		t.Fatalf("WriteCSV() error = %v", err)
	}

	output := buf.String()

	// Verify header row
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 { // header + 2 data rows
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	expectedHeader := "Date,Account,Payee,Category,Amount,Memo,Check Number,Status,Transfer Account"
	if lines[0] != expectedHeader {
		t.Errorf("header = %q, want %q", lines[0], expectedHeader)
	}

	// Round-trip: parse the output back
	result, err := ParseCSV(strings.NewReader(output))
	if err != nil {
		t.Fatalf("round-trip ParseCSV() error = %v", err)
	}

	if len(result.Records) != 2 {
		t.Fatalf("round-trip expected 2 records, got %d", len(result.Records))
	}

	if result.Records[0].Payee != "Kroger" {
		t.Errorf("round-trip record 0 payee = %q, want %q", result.Records[0].Payee, "Kroger")
	}

	expectedAmt := types.MustNewMoney("-125.43")
	if !result.Records[0].Amount.Equal(expectedAmt) {
		t.Errorf("round-trip record 0 amount = %s, want %s", result.Records[0].Amount.String(), expectedAmt.String())
	}
}

func TestWriteCSV_SplitTransactions(t *testing.T) {
	records := []ExportRecord{
		{
			Date:    "2024-01-15",
			Account: "Checking",
			Payee:   "Kroger",
			Amount:  "-150.00",
			Status:  "C",
			Splits: []ExportSplit{
				{Category: "Food:Groceries", Amount: "-120.00", Memo: "Food items"},
				{Category: "Household:Cleaning", Amount: "-30.00", Memo: "Cleaning supplies"},
			},
		},
	}

	var buf bytes.Buffer
	err := WriteCSV(&buf, records)
	if err != nil {
		t.Fatalf("WriteCSV() error = %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	// header + parent row + 2 split rows = 4
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d: %s", len(lines), output)
	}

	// Round-trip: parse back and verify split detection
	result, err := ParseCSV(strings.NewReader(output))
	if err != nil {
		t.Fatalf("round-trip ParseCSV() error = %v", err)
	}

	if len(result.Records) != 1 {
		t.Fatalf("round-trip expected 1 record, got %d", len(result.Records))
	}

	rec := result.Records[0]
	if !rec.IsSplit() {
		t.Fatal("round-trip: expected split transaction")
	}
	if len(rec.Splits) != 2 {
		t.Fatalf("round-trip: expected 2 splits, got %d", len(rec.Splits))
	}
	if rec.Splits[0].Category != "Food:Groceries" {
		t.Errorf("round-trip split 0 category = %q, want %q", rec.Splits[0].Category, "Food:Groceries")
	}
}

func TestWriteCSV_EmptyRecords(t *testing.T) {
	var buf bytes.Buffer
	err := WriteCSV(&buf, nil)
	if err != nil {
		t.Fatalf("WriteCSV() error = %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 { // Just header
		t.Fatalf("expected 1 line (header only), got %d", len(lines))
	}
}

func TestWriteCSV_SpecialCharacters(t *testing.T) {
	records := []ExportRecord{
		{
			Date:    "2024-01-15",
			Account: "Checking",
			Payee:   `Store "The Best"`,
			Amount:  "-50.00",
			Memo:    "Has, commas, and\nnewlines",
		},
	}

	var buf bytes.Buffer
	err := WriteCSV(&buf, records)
	if err != nil {
		t.Fatalf("WriteCSV() error = %v", err)
	}

	// Round-trip
	result, err := ParseCSV(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("round-trip ParseCSV() error = %v", err)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	if result.Records[0].Payee != `Store "The Best"` {
		t.Errorf("payee = %q, want %q", result.Records[0].Payee, `Store "The Best"`)
	}
}

func TestWriteCSV_TransferAccount(t *testing.T) {
	records := []ExportRecord{
		{
			Date:            "2024-01-15",
			Account:         "Checking",
			Payee:           "Transfer",
			Amount:          "-500.00",
			Status:          "C",
			TransferAccount: "Savings",
		},
	}

	var buf bytes.Buffer
	err := WriteCSV(&buf, records)
	if err != nil {
		t.Fatalf("WriteCSV() error = %v", err)
	}

	result, err := ParseCSV(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("round-trip ParseCSV() error = %v", err)
	}

	if result.Records[0].TransferAccount != "Savings" {
		t.Errorf("transfer account = %q, want %q", result.Records[0].TransferAccount, "Savings")
	}
}

// DetectFormat tests

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		filename string
		want     Format
		wantErr  bool
	}{
		{"transactions.csv", FormatCSV, false},
		{"TRANSACTIONS.CSV", FormatCSV, false},
		{"data.qif", FormatQIF, false},
		{"bank.ofx", FormatOFX, false},
		{"bank.qfx", FormatOFX, false},
		{"bank.QFX", FormatOFX, false},
		{"file.txt", "", true},
		{"noext", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got, err := DetectFormat(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("DetectFormat(%q) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("DetectFormat(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestParseError_Error(t *testing.T) {
	pe := &ParseError{Line: 5, Message: "bad value"}
	expected := "line 5: bad value"
	if pe.Error() != expected {
		t.Errorf("ParseError.Error() = %q, want %q", pe.Error(), expected)
	}
}

func TestParseCSV_MissingDate(t *testing.T) {
	input := `Date,Amount
,50.00
`

	result, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}

	if !result.HasErrors() {
		t.Fatal("expected parse errors for missing date value")
	}
}

func TestParseCSV_MissingAmount(t *testing.T) {
	input := `Date,Amount
2024-01-15,
`

	result, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}

	if !result.HasErrors() {
		t.Fatal("expected parse errors for missing amount value")
	}
}
