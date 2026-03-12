package imexport

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/models"
)

func TestParseQIF_BasicTransactions(t *testing.T) {
	input := `!Type:Bank
D01/15/2024
T-125.43
PKroger
LFood:Groceries
MWeekly groceries
CX
^
D01/16/2024
T3500.00
PEmployer Inc
LIncome:Salary
MJanuary paycheck
CX
^
D01/17/2024
T-95.00
PElectric Co
LBills:Utilities
MElectric bill
N1234
^
`

	result, err := ParseQIF(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseQIF() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseQIF() had errors: %v", result.Errors)
	}

	if len(result.Records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(result.Records))
	}

	// Verify first record
	rec := result.Records[0]
	if rec.Date.String() != "2024-01-15" {
		t.Errorf("record 0 date = %s, want 2024-01-15", rec.Date.String())
	}
	if rec.Payee != "Kroger" {
		t.Errorf("record 0 payee = %q, want %q", rec.Payee, "Kroger")
	}
	if rec.Category != "Food:Groceries" {
		t.Errorf("record 0 category = %q, want %q", rec.Category, "Food:Groceries")
	}
	expectedAmt := models.MustNewMoney("-125.43")
	if !rec.Amount.Equal(expectedAmt) {
		t.Errorf("record 0 amount = %s, want %s", rec.Amount.String(), expectedAmt.String())
	}
	if rec.Memo != "Weekly groceries" {
		t.Errorf("record 0 memo = %q, want %q", rec.Memo, "Weekly groceries")
	}
	if rec.Status != "C" {
		t.Errorf("record 0 status = %q, want %q", rec.Status, "C")
	}

	// Verify third record has check number and uncleared status
	rec2 := result.Records[2]
	if rec2.CheckNumber != "1234" {
		t.Errorf("record 2 check number = %q, want %q", rec2.CheckNumber, "1234")
	}
	if rec2.Status != "U" {
		t.Errorf("record 2 status = %q, want %q", rec2.Status, "U")
	}
}

func TestParseQIF_SplitTransactions(t *testing.T) {
	input := `!Type:Bank
D01/15/2024
T-150.00
PKroger
SFood:Groceries
$-120.00
EFood items
SHousehold:Cleaning
$-30.00
ECleaning supplies
^
`

	result, err := ParseQIF(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseQIF() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseQIF() had errors: %v", result.Errors)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
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
	expectedAmt := models.MustNewMoney("-120.00")
	if !rec.Splits[0].Amount.Equal(expectedAmt) {
		t.Errorf("split 0 amount = %s, want %s", rec.Splits[0].Amount.String(), expectedAmt.String())
	}
	if rec.Splits[0].Memo != "Food items" {
		t.Errorf("split 0 memo = %q, want %q", rec.Splits[0].Memo, "Food items")
	}

	if rec.Splits[1].Category != "Household:Cleaning" {
		t.Errorf("split 1 category = %q, want %q", rec.Splits[1].Category, "Household:Cleaning")
	}
	expectedAmt2 := models.MustNewMoney("-30.00")
	if !rec.Splits[1].Amount.Equal(expectedAmt2) {
		t.Errorf("split 1 amount = %s, want %s", rec.Splits[1].Amount.String(), expectedAmt2.String())
	}
}

func TestParseQIF_TransferCategory(t *testing.T) {
	input := `!Type:Bank
D01/15/2024
T-500.00
PTransfer
L[Savings]
CX
^
`

	result, err := ParseQIF(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseQIF() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseQIF() had errors: %v", result.Errors)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	rec := result.Records[0]
	if rec.TransferAccount != "Savings" {
		t.Errorf("transfer account = %q, want %q", rec.TransferAccount, "Savings")
	}
	if rec.Category != "" {
		t.Errorf("category should be empty for transfer, got %q", rec.Category)
	}
}

func TestParseQIF_ReconciledStatus(t *testing.T) {
	input := `!Type:Bank
D01/15/2024
T-50.00
PKroger
C*
^
`

	result, err := ParseQIF(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseQIF() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseQIF() had errors: %v", result.Errors)
	}

	if result.Records[0].Status != "R" {
		t.Errorf("status = %q, want %q", result.Records[0].Status, "R")
	}
}

func TestParseQIF_MultipleDateFormats(t *testing.T) {
	tests := []struct {
		name     string
		dateStr  string
		wantDate string
	}{
		{"MM/DD/YYYY", "01/15/2024", "2024-01-15"},
		{"M/D/YYYY", "1/5/2024", "2024-01-05"},
		{"MM/DD/YY", "01/15/24", "2024-01-15"},
		{"M/D/YY", "1/5/24", "2024-01-05"},
		{"YYYY-MM-DD", "2024-01-15", "2024-01-15"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := "!Type:Bank\nD" + tt.dateStr + "\nT-50.00\n^\n"
			result, err := ParseQIF(strings.NewReader(input))
			if err != nil {
				t.Fatalf("ParseQIF() error = %v", err)
			}
			if result.HasErrors() {
				t.Fatalf("ParseQIF() had errors: %v", result.Errors)
			}
			if len(result.Records) != 1 {
				t.Fatalf("expected 1 record, got %d", len(result.Records))
			}
			if result.Records[0].Date.String() != tt.wantDate {
				t.Errorf("date = %s, want %s", result.Records[0].Date.String(), tt.wantDate)
			}
		})
	}
}

func TestParseQIF_EmptyFile(t *testing.T) {
	result, err := ParseQIF(strings.NewReader(""))
	if err != nil {
		t.Fatalf("ParseQIF() error = %v", err)
	}

	if len(result.Records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(result.Records))
	}
}

func TestParseQIF_HeaderOnly(t *testing.T) {
	result, err := ParseQIF(strings.NewReader("!Type:Bank\n"))
	if err != nil {
		t.Fatalf("ParseQIF() error = %v", err)
	}

	if len(result.Records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(result.Records))
	}
}

func TestParseQIF_MissingDate(t *testing.T) {
	input := `!Type:Bank
T-50.00
PKroger
^
`

	result, err := ParseQIF(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseQIF() error = %v", err)
	}

	if !result.HasErrors() {
		t.Fatal("expected parse errors for missing date")
	}
	if len(result.Records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(result.Records))
	}
}

func TestParseQIF_MissingAmount(t *testing.T) {
	input := `!Type:Bank
D01/15/2024
PKroger
^
`

	result, err := ParseQIF(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseQIF() error = %v", err)
	}

	if !result.HasErrors() {
		t.Fatal("expected parse errors for missing amount")
	}
}

func TestParseQIF_InvalidDate(t *testing.T) {
	input := `!Type:Bank
Dnot-a-date
T-50.00
^
`

	result, err := ParseQIF(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseQIF() error = %v", err)
	}

	if !result.HasErrors() {
		t.Fatal("expected parse errors for invalid date")
	}
}

func TestParseQIF_InvalidAmount(t *testing.T) {
	input := `!Type:Bank
D01/15/2024
Tnot-a-number
^
`

	result, err := ParseQIF(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseQIF() error = %v", err)
	}

	if !result.HasErrors() {
		t.Fatal("expected parse errors for invalid amount")
	}
}

func TestParseQIF_MixedValidAndInvalidRecords(t *testing.T) {
	input := `!Type:Bank
D01/15/2024
T-50.00
PKroger
^
Dbad-date
T-30.00
PStore
^
D01/17/2024
T-75.00
PTarget
^
`

	result, err := ParseQIF(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseQIF() error = %v", err)
	}

	if len(result.Records) != 2 {
		t.Fatalf("expected 2 valid records, got %d", len(result.Records))
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
}

func TestParseQIF_SourceLineTracking(t *testing.T) {
	input := `!Type:Bank
D01/15/2024
T-50.00
PKroger
^
D01/16/2024
T-30.00
PTarget
^
`

	result, err := ParseQIF(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseQIF() error = %v", err)
	}

	if result.Records[0].SourceLine != 2 {
		t.Errorf("record 0 source line = %d, want 2", result.Records[0].SourceLine)
	}
	if result.Records[1].SourceLine != 6 {
		t.Errorf("record 1 source line = %d, want 6", result.Records[1].SourceLine)
	}
}

func TestParseQIF_AmountWithCommas(t *testing.T) {
	input := `!Type:Bank
D01/15/2024
T-1,500.00
PKroger
^
`

	result, err := ParseQIF(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseQIF() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseQIF() had errors: %v", result.Errors)
	}

	expectedAmt := models.MustNewMoney("-1500.00")
	if !result.Records[0].Amount.Equal(expectedAmt) {
		t.Errorf("amount = %s, want %s", result.Records[0].Amount.String(), expectedAmt.String())
	}
}

func TestParseQIF_NoTrailingSeparator(t *testing.T) {
	// Some QIF files omit the trailing ^ on the last record
	input := `!Type:Bank
D01/15/2024
T-50.00
PKroger`

	result, err := ParseQIF(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseQIF() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseQIF() had errors: %v", result.Errors)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	if result.Records[0].Payee != "Kroger" {
		t.Errorf("payee = %q, want %q", result.Records[0].Payee, "Kroger")
	}
}

func TestParseQIF_MultipleTypeHeaders(t *testing.T) {
	// QIF files may have multiple type headers (e.g., different account types)
	input := `!Type:Bank
D01/15/2024
T-50.00
PKroger
^
!Type:CCard
D01/16/2024
T-25.00
PStarbucks
^
`

	result, err := ParseQIF(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseQIF() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseQIF() had errors: %v", result.Errors)
	}

	if len(result.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(result.Records))
	}
}

func TestParseQIF_SplitWithoutMemo(t *testing.T) {
	input := `!Type:Bank
D01/15/2024
T-150.00
PKroger
SFood:Groceries
$-120.00
SHousehold:Cleaning
$-30.00
^
`

	result, err := ParseQIF(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseQIF() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseQIF() had errors: %v", result.Errors)
	}

	rec := result.Records[0]
	if len(rec.Splits) != 2 {
		t.Fatalf("expected 2 splits, got %d", len(rec.Splits))
	}
	if rec.Splits[0].Memo != "" {
		t.Errorf("split 0 memo should be empty, got %q", rec.Splits[0].Memo)
	}
}

// WriteQIF tests

func TestWriteQIF_BasicTransactions(t *testing.T) {
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
			Date:     "2024-01-16",
			Account:  "Checking",
			Payee:    "Employer Inc",
			Category: "Income:Salary",
			Amount:   "3500.00",
			Status:   "C",
		},
	}

	var buf bytes.Buffer
	err := WriteQIF(&buf, records, "checking")
	if err != nil {
		t.Fatalf("WriteQIF() error = %v", err)
	}

	output := buf.String()

	// Verify type header
	if !strings.HasPrefix(output, "!Type:Bank\n") {
		t.Errorf("expected output to start with !Type:Bank, got %q", output[:30])
	}

	// Verify contains expected fields
	if !strings.Contains(output, "D01/15/2024") {
		t.Error("expected output to contain D01/15/2024")
	}
	if !strings.Contains(output, "T-125.43") {
		t.Error("expected output to contain T-125.43")
	}
	if !strings.Contains(output, "PKroger") {
		t.Error("expected output to contain PKroger")
	}
	if !strings.Contains(output, "LFood:Groceries") {
		t.Error("expected output to contain LFood:Groceries")
	}
	if !strings.Contains(output, "MWeekly groceries") {
		t.Error("expected output to contain MWeekly groceries")
	}
	if !strings.Contains(output, "CX") {
		t.Error("expected output to contain CX (cleared status)")
	}

	// Records separated by ^
	if strings.Count(output, "^\n") != 2 {
		t.Errorf("expected 2 record separators, got %d", strings.Count(output, "^\n"))
	}
}

func TestWriteQIF_SplitTransactions(t *testing.T) {
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
	err := WriteQIF(&buf, records, "checking")
	if err != nil {
		t.Fatalf("WriteQIF() error = %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "SFood:Groceries") {
		t.Error("expected output to contain SFood:Groceries")
	}
	if !strings.Contains(output, "$-120.00") {
		t.Error("expected output to contain $-120.00")
	}
	if !strings.Contains(output, "EFood items") {
		t.Error("expected output to contain EFood items")
	}
	if !strings.Contains(output, "SHousehold:Cleaning") {
		t.Error("expected output to contain SHousehold:Cleaning")
	}
}

func TestWriteQIF_TransferAccount(t *testing.T) {
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
	err := WriteQIF(&buf, records, "checking")
	if err != nil {
		t.Fatalf("WriteQIF() error = %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "L[Savings]") {
		t.Errorf("expected output to contain L[Savings], got:\n%s", output)
	}
}

func TestWriteQIF_CheckNumber(t *testing.T) {
	records := []ExportRecord{
		{
			Date:        "2024-01-15",
			Account:     "Checking",
			Payee:       "Electric Co",
			Category:    "Bills:Utilities",
			Amount:      "-95.00",
			CheckNumber: "1234",
		},
	}

	var buf bytes.Buffer
	err := WriteQIF(&buf, records, "checking")
	if err != nil {
		t.Fatalf("WriteQIF() error = %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "N1234") {
		t.Error("expected output to contain N1234")
	}
}

func TestWriteQIF_ReconciledStatus(t *testing.T) {
	records := []ExportRecord{
		{
			Date:   "2024-01-15",
			Amount: "-50.00",
			Status: "R",
		},
	}

	var buf bytes.Buffer
	err := WriteQIF(&buf, records, "checking")
	if err != nil {
		t.Fatalf("WriteQIF() error = %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "C*") {
		t.Error("expected output to contain C* (reconciled status)")
	}
}

func TestWriteQIF_UnclearedStatusOmitted(t *testing.T) {
	records := []ExportRecord{
		{
			Date:   "2024-01-15",
			Amount: "-50.00",
			Status: "U",
		},
	}

	var buf bytes.Buffer
	err := WriteQIF(&buf, records, "checking")
	if err != nil {
		t.Fatalf("WriteQIF() error = %v", err)
	}

	output := buf.String()

	// Uncleared should not write a C line
	for line := range strings.SplitSeq(output, "\n") {
		if strings.HasPrefix(line, "C") {
			t.Errorf("uncleared status should not produce C line, got %q", line)
		}
	}
}

func TestWriteQIF_EmptyRecords(t *testing.T) {
	var buf bytes.Buffer
	err := WriteQIF(&buf, nil, "checking")
	if err != nil {
		t.Fatalf("WriteQIF() error = %v", err)
	}

	output := buf.String()
	if !strings.HasPrefix(output, "!Type:Bank\n") {
		t.Errorf("expected type header even with no records")
	}
}

func TestWriteQIF_AccountTypes(t *testing.T) {
	tests := []struct {
		accountType string
		wantHeader  string
	}{
		{"checking", "!Type:Bank"},
		{"savings", "!Type:Bank"},
		{"credit_card", "!Type:CCard"},
		{"investment", "!Type:Invst"},
		{"cash", "!Type:Cash"},
		{"loan", "!Type:Oth L"},
		{"asset", "!Type:Oth A"},
		{"unknown", "!Type:Bank"},
	}

	for _, tt := range tests {
		t.Run(tt.accountType, func(t *testing.T) {
			var buf bytes.Buffer
			err := WriteQIF(&buf, nil, tt.accountType)
			if err != nil {
				t.Fatalf("WriteQIF() error = %v", err)
			}

			output := buf.String()
			if !strings.HasPrefix(output, tt.wantHeader+"\n") {
				t.Errorf("expected %q header, got %q", tt.wantHeader, strings.Split(output, "\n")[0])
			}
		})
	}
}

func TestQIF_RoundTrip(t *testing.T) {
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
			Date:        "2024-01-17",
			Account:     "Checking",
			Payee:       "Electric Co",
			Category:    "Bills:Utilities",
			Amount:      "-95.00",
			Memo:        "Electric bill",
			CheckNumber: "1234",
			Status:      "R",
		},
	}

	// Write
	var buf bytes.Buffer
	err := WriteQIF(&buf, records, "checking")
	if err != nil {
		t.Fatalf("WriteQIF() error = %v", err)
	}

	// Parse back
	result, err := ParseQIF(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("round-trip ParseQIF() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("round-trip had errors: %v", result.Errors)
	}

	if len(result.Records) != 2 {
		t.Fatalf("round-trip expected 2 records, got %d", len(result.Records))
	}

	// Verify first record
	rec := result.Records[0]
	if rec.Date.String() != "2024-01-15" {
		t.Errorf("round-trip record 0 date = %s, want 2024-01-15", rec.Date.String())
	}
	if rec.Payee != "Kroger" {
		t.Errorf("round-trip record 0 payee = %q, want %q", rec.Payee, "Kroger")
	}
	if rec.Category != "Food:Groceries" {
		t.Errorf("round-trip record 0 category = %q, want %q", rec.Category, "Food:Groceries")
	}
	expectedAmt := models.MustNewMoney("-125.43")
	if !rec.Amount.Equal(expectedAmt) {
		t.Errorf("round-trip record 0 amount = %s, want %s", rec.Amount.String(), expectedAmt.String())
	}
	if rec.Status != "C" {
		t.Errorf("round-trip record 0 status = %q, want %q", rec.Status, "C")
	}

	// Verify second record
	rec2 := result.Records[1]
	if rec2.CheckNumber != "1234" {
		t.Errorf("round-trip record 1 check number = %q, want %q", rec2.CheckNumber, "1234")
	}
	if rec2.Status != "R" {
		t.Errorf("round-trip record 1 status = %q, want %q", rec2.Status, "R")
	}
}

func TestQIF_RoundTripSplits(t *testing.T) {
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

	// Write
	var buf bytes.Buffer
	err := WriteQIF(&buf, records, "checking")
	if err != nil {
		t.Fatalf("WriteQIF() error = %v", err)
	}

	// Parse back
	result, err := ParseQIF(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("round-trip ParseQIF() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("round-trip had errors: %v", result.Errors)
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
	if rec.Splits[0].Memo != "Food items" {
		t.Errorf("round-trip split 0 memo = %q, want %q", rec.Splits[0].Memo, "Food items")
	}
}

func TestQIF_RoundTripTransfer(t *testing.T) {
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

	// Write
	var buf bytes.Buffer
	err := WriteQIF(&buf, records, "checking")
	if err != nil {
		t.Fatalf("WriteQIF() error = %v", err)
	}

	// Parse back
	result, err := ParseQIF(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("round-trip ParseQIF() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("round-trip had errors: %v", result.Errors)
	}

	if result.Records[0].TransferAccount != "Savings" {
		t.Errorf("round-trip transfer account = %q, want %q", result.Records[0].TransferAccount, "Savings")
	}
	if result.Records[0].Category != "" {
		t.Errorf("round-trip category should be empty for transfer, got %q", result.Records[0].Category)
	}
}

func TestParseQIFDate(t *testing.T) {
	tests := []struct {
		input    string
		wantDate string
		wantErr  bool
	}{
		{"01/15/2024", "2024-01-15", false},
		{"1/5/2024", "2024-01-05", false},
		{"01/15/24", "2024-01-15", false},
		{"2024-01-15", "2024-01-15", false},
		{"not-a-date", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseQIFDate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseQIFDate(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.String() != tt.wantDate {
				t.Errorf("parseQIFDate(%q) = %s, want %s", tt.input, got.String(), tt.wantDate)
			}
		})
	}
}

func TestAccountTypeToQIF(t *testing.T) {
	tests := []struct {
		accountType string
		want        string
	}{
		{"checking", "Bank"},
		{"savings", "Bank"},
		{"credit_card", "CCard"},
		{"investment", "Invst"},
		{"cash", "Cash"},
		{"loan", "Oth L"},
		{"asset", "Oth A"},
		{"unknown", "Bank"},
		{"", "Bank"},
	}

	for _, tt := range tests {
		t.Run(tt.accountType, func(t *testing.T) {
			got := accountTypeToQIF(tt.accountType)
			if got != tt.want {
				t.Errorf("accountTypeToQIF(%q) = %q, want %q", tt.accountType, got, tt.want)
			}
		})
	}
}

func TestQIFStatusConversions(t *testing.T) {
	// qifStatusToImport
	if qifStatusToImport("X") != "C" {
		t.Error("X should map to C")
	}
	if qifStatusToImport("x") != "C" {
		t.Error("x should map to C")
	}
	if qifStatusToImport("*") != "R" {
		t.Error("* should map to R")
	}
	if qifStatusToImport("") != "U" {
		t.Error("empty should map to U")
	}

	// exportStatusToQIF
	if exportStatusToQIF("C") != "X" {
		t.Error("C should map to X")
	}
	if exportStatusToQIF("R") != "*" {
		t.Error("R should map to *")
	}
	if exportStatusToQIF("U") != "" {
		t.Error("U should map to empty")
	}
}
