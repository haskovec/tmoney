package imexport

import (
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

func TestParseOFX_BasicTransactions(t *testing.T) {
	input := `OFXHEADER:100
DATA:OFXSGML
<OFX>
<SIGNONMSGSRSV1>
<SONRS>
<STATUS><CODE>0<SEVERITY>INFO</STATUS>
<DTSERVER>20240131120000
<LANGUAGE>ENG
</SONRS>
</SIGNONMSGSRSV1>
<BANKMSGSRSV1>
<STMTTRNRS>
<TRNUID>1001
<STATUS><CODE>0<SEVERITY>INFO</STATUS>
<STMTRS>
<CURDEF>USD
<BANKACCTFROM>
<BANKID>123456789
<ACCTID>987654321
<ACCTTYPE>CHECKING
</BANKACCTFROM>
<BANKTRANLIST>
<DTSTART>20240101120000
<DTEND>20240131120000
<STMTTRN>
<TRNTYPE>DEBIT
<DTPOSTED>20240115120000
<TRNAMT>-125.43
<FITID>2024011501
<NAME>KROGER #1234
<MEMO>GROCERY PURCHASE
</STMTTRN>
<STMTTRN>
<TRNTYPE>CREDIT
<DTPOSTED>20240116120000
<TRNAMT>3500.00
<FITID>2024011601
<NAME>EMPLOYER INC PAYROLL
</STMTTRN>
<STMTTRN>
<TRNTYPE>CHECK
<DTPOSTED>20240117120000
<TRNAMT>-95.00
<FITID>2024011701
<NAME>ELECTRIC CO
<CHECKNUM>1234
</STMTTRN>
</BANKTRANLIST>
</STMTRS>
</STMTTRNRS>
</BANKMSGSRSV1>
</OFX>`

	result, err := ParseOFX(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseOFX() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseOFX() had errors: %v", result.Errors)
	}

	if len(result.Records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(result.Records))
	}

	// Verify first record (debit)
	rec := result.Records[0]
	expectedDate, _ := types.ParseDate("2024-01-15")
	if !rec.Date.Equal(expectedDate) {
		t.Errorf("record 0 date = %v, want %v", rec.Date, expectedDate)
	}
	expectedAmount := types.MustNewMoney("-125.43")
	if !rec.Amount.Equal(expectedAmount) {
		t.Errorf("record 0 amount = %v, want %v", rec.Amount, expectedAmount)
	}
	if rec.Payee != "KROGER #1234" {
		t.Errorf("record 0 payee = %q, want %q", rec.Payee, "KROGER #1234")
	}
	if rec.Memo != "GROCERY PURCHASE" {
		t.Errorf("record 0 memo = %q, want %q", rec.Memo, "GROCERY PURCHASE")
	}
	if rec.BankReferenceID != "2024011501" {
		t.Errorf("record 0 FITID = %q, want %q", rec.BankReferenceID, "2024011501")
	}
	if rec.Status != "U" {
		t.Errorf("record 0 status = %q, want %q", rec.Status, "U")
	}

	// Verify second record (credit)
	rec = result.Records[1]
	expectedDate, _ = types.ParseDate("2024-01-16")
	if !rec.Date.Equal(expectedDate) {
		t.Errorf("record 1 date = %v, want %v", rec.Date, expectedDate)
	}
	expectedAmount = types.MustNewMoney("3500.00")
	if !rec.Amount.Equal(expectedAmount) {
		t.Errorf("record 1 amount = %v, want %v", rec.Amount, expectedAmount)
	}
	if rec.Payee != "EMPLOYER INC PAYROLL" {
		t.Errorf("record 1 payee = %q, want %q", rec.Payee, "EMPLOYER INC PAYROLL")
	}
	if rec.BankReferenceID != "2024011601" {
		t.Errorf("record 1 FITID = %q, want %q", rec.BankReferenceID, "2024011601")
	}

	// Verify third record (check)
	rec = result.Records[2]
	if rec.CheckNumber != "1234" {
		t.Errorf("record 2 check number = %q, want %q", rec.CheckNumber, "1234")
	}
	if rec.BankReferenceID != "2024011701" {
		t.Errorf("record 2 FITID = %q, want %q", rec.BankReferenceID, "2024011701")
	}
}

func TestParseOFX_XMLStyle(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<?OFX OFXHEADER="200" VERSION="220"?>
<OFX>
<SIGNONMSGSRSV1>
<SONRS>
<STATUS><CODE>0</CODE><SEVERITY>INFO</SEVERITY></STATUS>
<DTSERVER>20240131120000</DTSERVER>
<LANGUAGE>ENG</LANGUAGE>
</SONRS>
</SIGNONMSGSRSV1>
<BANKMSGSRSV1>
<STMTTRNRS>
<STMTRS>
<BANKTRANLIST>
<STMTTRN>
<TRNTYPE>DEBIT</TRNTYPE>
<DTPOSTED>20240115</DTPOSTED>
<TRNAMT>-42.50</TRNAMT>
<FITID>ABC123</FITID>
<NAME>SHELL OIL</NAME>
</STMTTRN>
</BANKTRANLIST>
</STMTRS>
</STMTTRNRS>
</BANKMSGSRSV1>
</OFX>`

	result, err := ParseOFX(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseOFX() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseOFX() had errors: %v", result.Errors)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	rec := result.Records[0]
	expectedDate, _ := types.ParseDate("2024-01-15")
	if !rec.Date.Equal(expectedDate) {
		t.Errorf("date = %v, want %v", rec.Date, expectedDate)
	}
	expectedAmount := types.MustNewMoney("-42.50")
	if !rec.Amount.Equal(expectedAmount) {
		t.Errorf("amount = %v, want %v", rec.Amount, expectedAmount)
	}
	if rec.Payee != "SHELL OIL" {
		t.Errorf("payee = %q, want %q", rec.Payee, "SHELL OIL")
	}
	if rec.BankReferenceID != "ABC123" {
		t.Errorf("FITID = %q, want %q", rec.BankReferenceID, "ABC123")
	}
}

func TestParseOFX_DateFormats(t *testing.T) {
	tests := []struct {
		name     string
		dateStr  string
		wantDate string
	}{
		{
			name:     "YYYYMMDD",
			dateStr:  "20240115",
			wantDate: "2024-01-15",
		},
		{
			name:     "YYYYMMDDHHMMSS",
			dateStr:  "20240115120000",
			wantDate: "2024-01-15",
		},
		{
			name:     "with timezone",
			dateStr:  "20240115120000[-5:EST]",
			wantDate: "2024-01-15",
		},
		{
			name:     "with GMT timezone",
			dateStr:  "20240115120000[0:GMT]",
			wantDate: "2024-01-15",
		},
		{
			name:     "with fractional seconds",
			dateStr:  "20240115120000.000",
			wantDate: "2024-01-15",
		},
		{
			name:     "with fractional seconds and timezone",
			dateStr:  "20240115120000.000[-5:EST]",
			wantDate: "2024-01-15",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS><BANKTRANLIST>
<STMTTRN>
<DTPOSTED>` + tt.dateStr + `
<TRNAMT>-10.00
<FITID>TEST1
</STMTTRN>
</BANKTRANLIST></STMTRS></STMTTRNRS></BANKMSGSRSV1></OFX>`

			result, err := ParseOFX(strings.NewReader(input))
			if err != nil {
				t.Fatalf("ParseOFX() error = %v", err)
			}

			if result.HasErrors() {
				t.Fatalf("ParseOFX() had errors: %v", result.Errors)
			}

			if len(result.Records) != 1 {
				t.Fatalf("expected 1 record, got %d", len(result.Records))
			}

			expectedDate, _ := types.ParseDate(tt.wantDate)
			if !result.Records[0].Date.Equal(expectedDate) {
				t.Errorf("date = %v, want %v", result.Records[0].Date, expectedDate)
			}
		})
	}
}

func TestParseOFX_PayeeFallbackToMemo(t *testing.T) {
	input := `<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS><BANKTRANLIST>
<STMTTRN>
<DTPOSTED>20240115
<TRNAMT>-25.00
<FITID>TEST1
<MEMO>PAYMENT TO STORE
</STMTTRN>
</BANKTRANLIST></STMTRS></STMTTRNRS></BANKMSGSRSV1></OFX>`

	result, err := ParseOFX(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseOFX() error = %v", err)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	rec := result.Records[0]
	if rec.Payee != "PAYMENT TO STORE" {
		t.Errorf("payee = %q, want %q (should fall back to MEMO)", rec.Payee, "PAYMENT TO STORE")
	}
	if rec.Memo != "" {
		t.Errorf("memo = %q, want empty (MEMO used as payee fallback)", rec.Memo)
	}
}

func TestParseOFX_NameAndMemo(t *testing.T) {
	input := `<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS><BANKTRANLIST>
<STMTTRN>
<DTPOSTED>20240115
<TRNAMT>-25.00
<FITID>TEST1
<NAME>COFFEE SHOP
<MEMO>Purchase at register
</STMTTRN>
</BANKTRANLIST></STMTRS></STMTTRNRS></BANKMSGSRSV1></OFX>`

	result, err := ParseOFX(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseOFX() error = %v", err)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	rec := result.Records[0]
	if rec.Payee != "COFFEE SHOP" {
		t.Errorf("payee = %q, want %q", rec.Payee, "COFFEE SHOP")
	}
	if rec.Memo != "Purchase at register" {
		t.Errorf("memo = %q, want %q", rec.Memo, "Purchase at register")
	}
}

func TestParseOFX_EmptyFile(t *testing.T) {
	result, err := ParseOFX(strings.NewReader(""))
	if err != nil {
		t.Fatalf("ParseOFX() error = %v", err)
	}

	if len(result.Records) != 0 {
		t.Errorf("expected 0 records, got %d", len(result.Records))
	}
}

func TestParseOFX_NoTransactions(t *testing.T) {
	input := `OFXHEADER:100
DATA:OFXSGML
<OFX>
<SIGNONMSGSRSV1>
<SONRS>
<STATUS><CODE>0<SEVERITY>INFO</STATUS>
</SONRS>
</SIGNONMSGSRSV1>
</OFX>`

	result, err := ParseOFX(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseOFX() error = %v", err)
	}

	if len(result.Records) != 0 {
		t.Errorf("expected 0 records, got %d", len(result.Records))
	}
}

func TestParseOFX_MissingDate(t *testing.T) {
	input := `<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS><BANKTRANLIST>
<STMTTRN>
<TRNAMT>-10.00
<FITID>TEST1
<NAME>Test Payee
</STMTTRN>
</BANKTRANLIST></STMTRS></STMTTRNRS></BANKMSGSRSV1></OFX>`

	result, err := ParseOFX(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseOFX() error = %v", err)
	}

	if len(result.Records) != 0 {
		t.Errorf("expected 0 records, got %d", len(result.Records))
	}

	if !result.HasErrors() {
		t.Error("expected parse errors for missing date")
	}

	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}

	if !strings.Contains(result.Errors[0].Message, "missing DTPOSTED") {
		t.Errorf("error message = %q, want to contain %q", result.Errors[0].Message, "missing DTPOSTED")
	}
}

func TestParseOFX_MissingAmount(t *testing.T) {
	input := `<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS><BANKTRANLIST>
<STMTTRN>
<DTPOSTED>20240115
<FITID>TEST1
<NAME>Test Payee
</STMTTRN>
</BANKTRANLIST></STMTRS></STMTTRNRS></BANKMSGSRSV1></OFX>`

	result, err := ParseOFX(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseOFX() error = %v", err)
	}

	if len(result.Records) != 0 {
		t.Errorf("expected 0 records, got %d", len(result.Records))
	}

	if !result.HasErrors() {
		t.Error("expected parse errors for missing amount")
	}

	if !strings.Contains(result.Errors[0].Message, "missing TRNAMT") {
		t.Errorf("error message = %q, want to contain %q", result.Errors[0].Message, "missing TRNAMT")
	}
}

func TestParseOFX_InvalidDate(t *testing.T) {
	input := `<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS><BANKTRANLIST>
<STMTTRN>
<DTPOSTED>NOTADATE
<TRNAMT>-10.00
<FITID>TEST1
</STMTTRN>
</BANKTRANLIST></STMTRS></STMTTRNRS></BANKMSGSRSV1></OFX>`

	result, err := ParseOFX(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseOFX() error = %v", err)
	}

	if len(result.Records) != 0 {
		t.Errorf("expected 0 records, got %d", len(result.Records))
	}

	if !result.HasErrors() {
		t.Error("expected parse errors for invalid date")
	}

	if !strings.Contains(result.Errors[0].Message, "invalid date") {
		t.Errorf("error message = %q, want to contain %q", result.Errors[0].Message, "invalid date")
	}
}

func TestParseOFX_InvalidAmount(t *testing.T) {
	input := `<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS><BANKTRANLIST>
<STMTTRN>
<DTPOSTED>20240115
<TRNAMT>NOTAMOUNT
<FITID>TEST1
</STMTTRN>
</BANKTRANLIST></STMTRS></STMTTRNRS></BANKMSGSRSV1></OFX>`

	result, err := ParseOFX(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseOFX() error = %v", err)
	}

	if len(result.Records) != 0 {
		t.Errorf("expected 0 records, got %d", len(result.Records))
	}

	if !result.HasErrors() {
		t.Error("expected parse errors for invalid amount")
	}

	if !strings.Contains(result.Errors[0].Message, "invalid amount") {
		t.Errorf("error message = %q, want to contain %q", result.Errors[0].Message, "invalid amount")
	}
}

func TestParseOFX_MixedValidAndInvalid(t *testing.T) {
	input := `<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS><BANKTRANLIST>
<STMTTRN>
<DTPOSTED>20240115
<TRNAMT>-42.50
<FITID>GOOD1
<NAME>Valid Transaction
</STMTTRN>
<STMTTRN>
<TRNAMT>-10.00
<FITID>BAD1
<NAME>Missing Date
</STMTTRN>
<STMTTRN>
<DTPOSTED>20240117
<TRNAMT>-15.00
<FITID>GOOD2
<NAME>Another Valid
</STMTTRN>
</BANKTRANLIST></STMTRS></STMTTRNRS></BANKMSGSRSV1></OFX>`

	result, err := ParseOFX(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseOFX() error = %v", err)
	}

	if len(result.Records) != 2 {
		t.Fatalf("expected 2 valid records, got %d", len(result.Records))
	}

	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}

	if result.Records[0].Payee != "Valid Transaction" {
		t.Errorf("record 0 payee = %q, want %q", result.Records[0].Payee, "Valid Transaction")
	}
	if result.Records[1].Payee != "Another Valid" {
		t.Errorf("record 1 payee = %q, want %q", result.Records[1].Payee, "Another Valid")
	}
}

func TestParseOFX_CreditCardStatement(t *testing.T) {
	input := `OFXHEADER:100
DATA:OFXSGML
<OFX>
<SIGNONMSGSRSV1>
<SONRS>
<STATUS><CODE>0<SEVERITY>INFO</STATUS>
</SONRS>
</SIGNONMSGSRSV1>
<CREDITCARDMSGSRSV1>
<CCSTMTTRNRS>
<CCSTMTRS>
<CURDEF>USD
<CCACCTFROM>
<ACCTID>1234567890123456
</CCACCTFROM>
<BANKTRANLIST>
<STMTTRN>
<TRNTYPE>DEBIT
<DTPOSTED>20240115
<TRNAMT>-55.00
<FITID>CC20240115001
<NAME>RESTAURANT
</STMTTRN>
</BANKTRANLIST>
</CCSTMTRS>
</CCSTMTTRNRS>
</CREDITCARDMSGSRSV1>
</OFX>`

	result, err := ParseOFX(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseOFX() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseOFX() had errors: %v", result.Errors)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	rec := result.Records[0]
	if rec.Payee != "RESTAURANT" {
		t.Errorf("payee = %q, want %q", rec.Payee, "RESTAURANT")
	}
	expectedAmount := types.MustNewMoney("-55.00")
	if !rec.Amount.Equal(expectedAmount) {
		t.Errorf("amount = %v, want %v", rec.Amount, expectedAmount)
	}
}

func TestParseOFX_CaseInsensitiveTags(t *testing.T) {
	input := `<ofx><bankmsgsrsv1><stmttrnrs><stmtrs><banktranlist>
<stmttrn>
<dtposted>20240115
<trnamt>-10.00
<fitid>TEST1
<name>Lowercase Tags
</stmttrn>
</banktranlist></stmtrs></stmttrnrs></bankmsgsrsv1></ofx>`

	result, err := ParseOFX(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseOFX() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseOFX() had errors: %v", result.Errors)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	if result.Records[0].Payee != "Lowercase Tags" {
		t.Errorf("payee = %q, want %q", result.Records[0].Payee, "Lowercase Tags")
	}
}

func TestParseOFX_NoPayeeOrMemo(t *testing.T) {
	input := `<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS><BANKTRANLIST>
<STMTTRN>
<DTPOSTED>20240115
<TRNAMT>-10.00
<FITID>TEST1
</STMTTRN>
</BANKTRANLIST></STMTRS></STMTTRNRS></BANKMSGSRSV1></OFX>`

	result, err := ParseOFX(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseOFX() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseOFX() had errors: %v", result.Errors)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	rec := result.Records[0]
	if rec.Payee != "" {
		t.Errorf("payee = %q, want empty", rec.Payee)
	}
	if rec.Memo != "" {
		t.Errorf("memo = %q, want empty", rec.Memo)
	}
}

func TestParseOFX_NoFITID(t *testing.T) {
	input := `<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS><BANKTRANLIST>
<STMTTRN>
<DTPOSTED>20240115
<TRNAMT>-10.00
<NAME>No FITID
</STMTTRN>
</BANKTRANLIST></STMTRS></STMTTRNRS></BANKMSGSRSV1></OFX>`

	result, err := ParseOFX(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseOFX() error = %v", err)
	}

	if result.HasErrors() {
		t.Fatalf("ParseOFX() had errors: %v", result.Errors)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	if result.Records[0].BankReferenceID != "" {
		t.Errorf("bank reference ID = %q, want empty", result.Records[0].BankReferenceID)
	}
}

func TestParseOFX_MultipleTransactionsOrdering(t *testing.T) {
	input := `<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS><BANKTRANLIST>
<STMTTRN>
<DTPOSTED>20240101
<TRNAMT>-10.00
<FITID>FIRST
<NAME>First
</STMTTRN>
<STMTTRN>
<DTPOSTED>20240102
<TRNAMT>-20.00
<FITID>SECOND
<NAME>Second
</STMTTRN>
<STMTTRN>
<DTPOSTED>20240103
<TRNAMT>-30.00
<FITID>THIRD
<NAME>Third
</STMTTRN>
</BANKTRANLIST></STMTRS></STMTTRNRS></BANKMSGSRSV1></OFX>`

	result, err := ParseOFX(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseOFX() error = %v", err)
	}

	if len(result.Records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(result.Records))
	}

	// Verify ordering is preserved
	names := []string{"First", "Second", "Third"}
	for i, name := range names {
		if result.Records[i].Payee != name {
			t.Errorf("record %d payee = %q, want %q", i, result.Records[i].Payee, name)
		}
	}
}

func TestParseOFXDate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"YYYYMMDD", "20240115", "2024-01-15", false},
		{"YYYYMMDDHHMMSS", "20240115120000", "2024-01-15", false},
		{"with timezone", "20240115120000[-5:EST]", "2024-01-15", false},
		{"with GMT", "20240115120000[0:GMT]", "2024-01-15", false},
		{"with fractional", "20240115120000.000", "2024-01-15", false},
		{"with frac and tz", "20240115120000.000[-5:EST]", "2024-01-15", false},
		{"whitespace", "  20240115  ", "2024-01-15", false},
		{"short", "2024", "", true},
		{"empty", "", "", true},
		{"invalid", "NOTADATE", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOFXDate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseOFXDate(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				want, _ := types.ParseDate(tt.want)
				if !got.Equal(want) {
					t.Errorf("parseOFXDate(%q) = %v, want %v", tt.input, got, want)
				}
			}
		})
	}
}

func TestExtractOFXValue(t *testing.T) {
	tests := []struct {
		name  string
		block string
		tag   string
		want  string
	}{
		{
			name:  "SGML style",
			block: "<NAME>Test Payee\n<MEMO>Some memo",
			tag:   "NAME",
			want:  "Test Payee",
		},
		{
			name:  "XML style",
			block: "<NAME>Test Payee</NAME>",
			tag:   "NAME",
			want:  "Test Payee",
		},
		{
			name:  "missing tag",
			block: "<NAME>Test Payee",
			tag:   "MEMO",
			want:  "",
		},
		{
			name:  "case insensitive",
			block: "<name>Test Payee</name>",
			tag:   "NAME",
			want:  "Test Payee",
		},
		{
			name:  "with whitespace",
			block: "<NAME>  Test Payee  </NAME>",
			tag:   "NAME",
			want:  "Test Payee",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractOFXValue(tt.block, tt.tag)
			if got != tt.want {
				t.Errorf("extractOFXValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractOFXBlocks(t *testing.T) {
	t.Run("with closing tags", func(t *testing.T) {
		content := "<STMTTRN><NAME>First</NAME></STMTTRN><STMTTRN><NAME>Second</NAME></STMTTRN>"
		blocks := extractOFXBlocks(content, "STMTTRN")
		if len(blocks) != 2 {
			t.Fatalf("expected 2 blocks, got %d", len(blocks))
		}
	})

	t.Run("SGML without closing tags", func(t *testing.T) {
		content := "<STMTTRN>\n<NAME>First\n<STMTTRN>\n<NAME>Second\n</BANKTRANLIST>"
		blocks := extractOFXBlocks(content, "STMTTRN")
		if len(blocks) != 2 {
			t.Fatalf("expected 2 blocks, got %d", len(blocks))
		}
	})

	t.Run("no blocks", func(t *testing.T) {
		content := "<OFX><BANKMSGSRSV1></BANKMSGSRSV1></OFX>"
		blocks := extractOFXBlocks(content, "STMTTRN")
		if len(blocks) != 0 {
			t.Fatalf("expected 0 blocks, got %d", len(blocks))
		}
	})
}

func TestParseOFX_QFXExtension(t *testing.T) {
	// QFX is the same format as OFX (Quicken's variant)
	// Verify format detection handles .qfx
	format, err := DetectFormat("statement.qfx")
	if err != nil {
		t.Fatalf("DetectFormat() error = %v", err)
	}
	if format != FormatOFX {
		t.Errorf("format = %q, want %q", format, FormatOFX)
	}
}
