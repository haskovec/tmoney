package main

import (
	"strings"
	"testing"
)

func TestParseArgs_FileFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedFile string
	}{
		{"short flag with space", []string{"-f", "/path/to/file.tdb"}, "/path/to/file.tdb"},
		{"long flag with space", []string{"--file", "/path/to/file.tdb"}, "/path/to/file.tdb"},
		{"long flag with equals", []string{"--file=/path/to/file.tdb"}, "/path/to/file.tdb"},
		{"short flag with equals", []string{"-f=/path/to/file.tdb"}, "/path/to/file.tdb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.file != tt.expectedFile {
				t.Errorf("parseArgs(%v) file = %q, want %q", tt.args, opts.file, tt.expectedFile)
			}
		})
	}
}

func TestParseArgs_FileFlagMissingPath(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"short flag missing path", []string{"-f"}},
		{"long flag missing path", []string{"--file"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseArgs(tt.args)
			if err == nil {
				t.Errorf("parseArgs(%v) expected error for missing path", tt.args)
			}
		})
	}
}

func TestParseArgs_ListAccountsFlag(t *testing.T) {
	opts, _, err := parseArgs([]string{"--list-accounts"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if !opts.listAccounts {
		t.Error("parseArgs did not set listAccounts flag")
	}
}

func TestParseArgs_IncludeClosedFlag(t *testing.T) {
	opts, _, err := parseArgs([]string{"--include-closed"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if !opts.includeClosed {
		t.Error("parseArgs did not set includeClosed flag")
	}
}

func TestParseArgs_CombinedFlags(t *testing.T) {
	opts, remaining, err := parseArgs([]string{"--file", "test.tdb", "--list-accounts", "--include-closed"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if opts.file != "test.tdb" {
		t.Errorf("file = %q, want %q", opts.file, "test.tdb")
	}
	if !opts.listAccounts {
		t.Error("listAccounts flag not set")
	}
	if !opts.includeClosed {
		t.Error("includeClosed flag not set")
	}
	if len(remaining) != 0 {
		t.Errorf("remaining = %v, want empty", remaining)
	}
}

func TestParseArgs_RemainingArgs(t *testing.T) {
	opts, remaining, err := parseArgs([]string{"some-file.tdb", "extra-arg"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if len(remaining) != 2 {
		t.Errorf("remaining = %v, want 2 elements", remaining)
	}
	if opts.file != "" {
		t.Errorf("file should be empty for positional args in parseArgs, got %q", opts.file)
	}
}

func TestParseArgs_CreateFlag(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedCreate string
	}{
		{"long flag with space", []string{"--create", "/path/to/new.tdb"}, "/path/to/new.tdb"},
		{"long flag with equals", []string{"--create=/path/to/new.tdb"}, "/path/to/new.tdb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.createDB != tt.expectedCreate {
				t.Errorf("parseArgs(%v) createDB = %q, want %q", tt.args, opts.createDB, tt.expectedCreate)
			}
		})
	}
}

func TestParseArgs_CreateFlagMissingPath(t *testing.T) {
	_, _, err := parseArgs([]string{"--create"})
	if err == nil {
		t.Error("parseArgs(--create) without path should return error")
	}
	if !strings.Contains(err.Error(), "requires a path") {
		t.Errorf("error should mention path requirement, got: %v", err)
	}
}

func TestParseArgs_AccountFlag(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		expectedAccount string
	}{
		{"long flag with space", []string{"--account", "My Checking"}, "My Checking"},
		{"long flag with equals", []string{"--account=My Checking"}, "My Checking"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.accountName != tt.expectedAccount {
				t.Errorf("parseArgs(%v) accountName = %q, want %q", tt.args, opts.accountName, tt.expectedAccount)
			}
		})
	}
}

func TestParseArgs_AccountFlagMissingName(t *testing.T) {
	_, _, err := parseArgs([]string{"--account"})
	if err == nil {
		t.Error("parseArgs(--account) without name should return error")
	}
	if !strings.Contains(err.Error(), "requires") {
		t.Errorf("error should mention requirement, got: %v", err)
	}
}

func TestParseArgs_BalanceFlag(t *testing.T) {
	opts, _, err := parseArgs([]string{"--balance"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if !opts.showBalance {
		t.Error("parseArgs did not set showBalance flag")
	}
}

func TestParseArgs_TransactionsFlag(t *testing.T) {
	opts, _, err := parseArgs([]string{"--transactions"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if !opts.transactions {
		t.Error("parseArgs did not set transactions flag")
	}
}

func TestParseArgs_LimitFlag(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedLimit int
	}{
		{"long flag with space", []string{"--limit", "10"}, 10},
		{"long flag with equals", []string{"--limit=25"}, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.limit != tt.expectedLimit {
				t.Errorf("parseArgs(%v) limit = %d, want %d", tt.args, opts.limit, tt.expectedLimit)
			}
		})
	}
}

func TestParseArgs_LimitFlagInvalid(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"missing value", []string{"--limit"}},
		{"not a number", []string{"--limit", "abc"}},
		{"zero", []string{"--limit", "0"}},
		{"negative", []string{"--limit", "-5"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseArgs(tt.args)
			if err == nil {
				t.Errorf("parseArgs(%v) should return error", tt.args)
			}
		})
	}
}

func TestParseArgs_FromToFlags(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedFrom string
		expectedTo   string
	}{
		{"from with space", []string{"--from", "2024-01-01"}, "2024-01-01", ""},
		{"to with space", []string{"--to", "2024-12-31"}, "", "2024-12-31"},
		{"from with equals", []string{"--from=2024-01-01"}, "2024-01-01", ""},
		{"to with equals", []string{"--to=2024-12-31"}, "", "2024-12-31"},
		{"both flags", []string{"--from", "2024-01-01", "--to", "2024-12-31"}, "2024-01-01", "2024-12-31"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.fromDate != tt.expectedFrom {
				t.Errorf("parseArgs(%v) fromDate = %q, want %q", tt.args, opts.fromDate, tt.expectedFrom)
			}
			if opts.toDate != tt.expectedTo {
				t.Errorf("parseArgs(%v) toDate = %q, want %q", tt.args, opts.toDate, tt.expectedTo)
			}
		})
	}
}

func TestParseArgs_FromToMissingValue(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"from missing value", []string{"--from"}},
		{"to missing value", []string{"--to"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseArgs(tt.args)
			if err == nil {
				t.Errorf("parseArgs(%v) should return error", tt.args)
			}
		})
	}
}

func TestParseArgs_AddTransactionFlag(t *testing.T) {
	opts, _, err := parseArgs([]string{"--add-transaction"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if !opts.addTransaction {
		t.Error("parseArgs did not set addTransaction flag")
	}
}

func TestParseArgs_AmountFlag(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedAmount string
	}{
		{"long flag with space", []string{"--amount", "-50.00"}, "-50.00"},
		{"long flag with equals", []string{"--amount=-50.00"}, "-50.00"},
		{"positive amount", []string{"--amount", "100.50"}, "100.50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.txAmount != tt.expectedAmount {
				t.Errorf("parseArgs(%v) txAmount = %q, want %q", tt.args, opts.txAmount, tt.expectedAmount)
			}
		})
	}
}

func TestParseArgs_AmountFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--amount"})
	if err == nil {
		t.Error("parseArgs(--amount) without value should return error")
	}
	if !strings.Contains(err.Error(), "requires") {
		t.Errorf("error should mention requirement, got: %v", err)
	}
}

func TestParseArgs_PayeeFlag(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedPayee string
	}{
		{"long flag with space", []string{"--payee", "Coffee Shop"}, "Coffee Shop"},
		{"long flag with equals", []string{"--payee=Coffee Shop"}, "Coffee Shop"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.txPayee != tt.expectedPayee {
				t.Errorf("parseArgs(%v) txPayee = %q, want %q", tt.args, opts.txPayee, tt.expectedPayee)
			}
		})
	}
}

func TestParseArgs_PayeeFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--payee"})
	if err == nil {
		t.Error("parseArgs(--payee) without value should return error")
	}
}

func TestParseArgs_CategoryFlag(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		expectedCategory string
	}{
		{"long flag with space", []string{"--category", "Food"}, "Food"},
		{"long flag with equals", []string{"--category=Food"}, "Food"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.txCategory != tt.expectedCategory {
				t.Errorf("parseArgs(%v) txCategory = %q, want %q", tt.args, opts.txCategory, tt.expectedCategory)
			}
		})
	}
}

func TestParseArgs_CategoryFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--category"})
	if err == nil {
		t.Error("parseArgs(--category) without value should return error")
	}
}

func TestParseArgs_DateFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedDate string
	}{
		{"long flag with space", []string{"--date", "2024-01-15"}, "2024-01-15"},
		{"long flag with equals", []string{"--date=2024-01-15"}, "2024-01-15"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.txDate != tt.expectedDate {
				t.Errorf("parseArgs(%v) txDate = %q, want %q", tt.args, opts.txDate, tt.expectedDate)
			}
		})
	}
}

func TestParseArgs_DateFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--date"})
	if err == nil {
		t.Error("parseArgs(--date) without value should return error")
	}
}

func TestParseArgs_MemoFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedMemo string
	}{
		{"long flag with space", []string{"--memo", "Morning coffee"}, "Morning coffee"},
		{"long flag with equals", []string{"--memo=Morning coffee"}, "Morning coffee"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.txMemo != tt.expectedMemo {
				t.Errorf("parseArgs(%v) txMemo = %q, want %q", tt.args, opts.txMemo, tt.expectedMemo)
			}
		})
	}
}

func TestParseArgs_MemoFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--memo"})
	if err == nil {
		t.Error("parseArgs(--memo) without value should return error")
	}
}

func TestParseArgs_AddAccountFlag(t *testing.T) {
	opts, _, err := parseArgs([]string{"--add-account"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if !opts.addAccount {
		t.Error("parseArgs did not set addAccount flag")
	}
}

func TestParseArgs_NameFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedName string
	}{
		{"long flag with space", []string{"--name", "My Checking"}, "My Checking"},
		{"long flag with equals", []string{"--name=My Checking"}, "My Checking"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctName != tt.expectedName {
				t.Errorf("parseArgs(%v) acctName = %q, want %q", tt.args, opts.acctName, tt.expectedName)
			}
		})
	}
}

func TestParseArgs_NameFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--name"})
	if err == nil {
		t.Error("parseArgs(--name) without value should return error")
	}
}

func TestParseArgs_TypeFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedType string
	}{
		{"long flag with space", []string{"--type", "checking"}, "checking"},
		{"long flag with equals", []string{"--type=savings"}, "savings"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctType != tt.expectedType {
				t.Errorf("parseArgs(%v) acctType = %q, want %q", tt.args, opts.acctType, tt.expectedType)
			}
		})
	}
}

func TestParseArgs_TypeFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--type"})
	if err == nil {
		t.Error("parseArgs(--type) without value should return error")
	}
}

func TestParseArgs_CurrencyFlag(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		expectedCurrency string
	}{
		{"long flag with space", []string{"--currency", "EUR"}, "EUR"},
		{"long flag with equals", []string{"--currency=GBP"}, "GBP"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctCurrency != tt.expectedCurrency {
				t.Errorf("parseArgs(%v) acctCurrency = %q, want %q", tt.args, opts.acctCurrency, tt.expectedCurrency)
			}
		})
	}
}

func TestParseArgs_CurrencyFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--currency"})
	if err == nil {
		t.Error("parseArgs(--currency) without value should return error")
	}
}

func TestParseArgs_OpeningBalanceFlag(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		expectedBalance string
	}{
		{"long flag with space", []string{"--opening-balance", "1000.00"}, "1000.00"},
		{"long flag with equals", []string{"--opening-balance=500.50"}, "500.50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctOpeningBal != tt.expectedBalance {
				t.Errorf("parseArgs(%v) acctOpeningBal = %q, want %q", tt.args, opts.acctOpeningBal, tt.expectedBalance)
			}
		})
	}
}

func TestParseArgs_OpeningBalanceFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--opening-balance"})
	if err == nil {
		t.Error("parseArgs(--opening-balance) without value should return error")
	}
}

func TestParseArgs_OpeningDateFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedDate string
	}{
		{"long flag with space", []string{"--opening-date", "2024-01-15"}, "2024-01-15"},
		{"long flag with equals", []string{"--opening-date=2024-06-01"}, "2024-06-01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctOpeningDate != tt.expectedDate {
				t.Errorf("parseArgs(%v) acctOpeningDate = %q, want %q", tt.args, opts.acctOpeningDate, tt.expectedDate)
			}
		})
	}
}

func TestParseArgs_OpeningDateFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--opening-date"})
	if err == nil {
		t.Error("parseArgs(--opening-date) without value should return error")
	}
}

func TestParseArgs_InstitutionFlag(t *testing.T) {
	tests := []struct {
		name                string
		args                []string
		expectedInstitution string
	}{
		{"long flag with space", []string{"--institution", "Chase Bank"}, "Chase Bank"},
		{"long flag with equals", []string{"--institution=Wells Fargo"}, "Wells Fargo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctInstitution != tt.expectedInstitution {
				t.Errorf("parseArgs(%v) acctInstitution = %q, want %q", tt.args, opts.acctInstitution, tt.expectedInstitution)
			}
		})
	}
}

func TestParseArgs_InstitutionFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--institution"})
	if err == nil {
		t.Error("parseArgs(--institution) without value should return error")
	}
}

func TestParseArgs_AccountNumberFlag(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedNumber string
	}{
		{"long flag with space", []string{"--account-number", "1234567890"}, "1234567890"},
		{"long flag with equals", []string{"--account-number=9876543210"}, "9876543210"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctNumber != tt.expectedNumber {
				t.Errorf("parseArgs(%v) acctNumber = %q, want %q", tt.args, opts.acctNumber, tt.expectedNumber)
			}
		})
	}
}

func TestParseArgs_AccountNumberFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--account-number"})
	if err == nil {
		t.Error("parseArgs(--account-number) without value should return error")
	}
}

func TestParseArgs_NotesFlag(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedNotes string
	}{
		{"long flag with space", []string{"--notes", "Primary checking account"}, "Primary checking account"},
		{"long flag with equals", []string{"--notes=For emergencies only"}, "For emergencies only"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctNotes != tt.expectedNotes {
				t.Errorf("parseArgs(%v) acctNotes = %q, want %q", tt.args, opts.acctNotes, tt.expectedNotes)
			}
		})
	}
}

func TestParseArgs_NotesFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--notes"})
	if err == nil {
		t.Error("parseArgs(--notes) without value should return error")
	}
}

func TestParseArgs_CreditLimitFlag(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedLimit string
	}{
		{"long flag with space", []string{"--credit-limit", "5000.00"}, "5000.00"},
		{"long flag with equals", []string{"--credit-limit=10000"}, "10000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctCreditLimit != tt.expectedLimit {
				t.Errorf("parseArgs(%v) acctCreditLimit = %q, want %q", tt.args, opts.acctCreditLimit, tt.expectedLimit)
			}
		})
	}
}

func TestParseArgs_CreditLimitFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--credit-limit"})
	if err == nil {
		t.Error("parseArgs(--credit-limit) without value should return error")
	}
}

func TestParseArgs_InterestRateFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedRate string
	}{
		{"long flag with space", []string{"--interest-rate", "5.5"}, "5.5"},
		{"long flag with equals", []string{"--interest-rate=7.25"}, "7.25"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.acctInterestRate != tt.expectedRate {
				t.Errorf("parseArgs(%v) acctInterestRate = %q, want %q", tt.args, opts.acctInterestRate, tt.expectedRate)
			}
		})
	}
}

func TestParseArgs_InterestRateFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--interest-rate"})
	if err == nil {
		t.Error("parseArgs(--interest-rate) without value should return error")
	}
}

func TestParseArgs_TransferFlag(t *testing.T) {
	opts, _, err := parseArgs([]string{"--transfer"})
	if err != nil {
		t.Errorf("parseArgs returned error: %v", err)
		return
	}
	if !opts.transfer {
		t.Error("parseArgs did not set transfer flag")
	}
}

func TestParseArgs_TransferWithFromTo(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantFrom    string
		wantTo      string
		wantTransfer bool
	}{
		{
			name:        "transfer with from and to",
			args:        []string{"--transfer", "--from", "Checking", "--to", "Savings"},
			wantFrom:    "Checking",
			wantTo:      "Savings",
			wantTransfer: true,
		},
		{
			name:        "transfer with equals syntax",
			args:        []string{"--transfer", "--from=Checking", "--to=Savings"},
			wantFrom:    "Checking",
			wantTo:      "Savings",
			wantTransfer: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.transfer != tt.wantTransfer {
				t.Errorf("transfer = %v, want %v", opts.transfer, tt.wantTransfer)
			}
			if opts.fromAccount != tt.wantFrom {
				t.Errorf("fromAccount = %q, want %q", opts.fromAccount, tt.wantFrom)
			}
			if opts.toAccount != tt.wantTo {
				t.Errorf("toAccount = %q, want %q", opts.toAccount, tt.wantTo)
			}
		})
	}
}

func TestParseArgs_SearchFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedTerm string
	}{
		{"long flag with space", []string{"--search", "amazon"}, "amazon"},
		{"long flag with equals", []string{"--search=amazon"}, "amazon"},
		{"term with spaces", []string{"--search", "coffee shop"}, "coffee shop"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.searchTerm != tt.expectedTerm {
				t.Errorf("parseArgs(%v) searchTerm = %q, want %q", tt.args, opts.searchTerm, tt.expectedTerm)
			}
		})
	}
}

func TestParseArgs_SearchFlagMissingTerm(t *testing.T) {
	_, _, err := parseArgs([]string{"--search"})
	if err == nil {
		t.Error("parseArgs(--search) without term should return error")
	}
	if !strings.Contains(err.Error(), "requires") {
		t.Errorf("error should mention requirement, got: %v", err)
	}
}

func TestParseArgs_MinMaxFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectedMin string
		expectedMax string
	}{
		{"min flag with space", []string{"--min", "10.00"}, "10.00", ""},
		{"min flag with equals", []string{"--min=10.00"}, "10.00", ""},
		{"max flag with space", []string{"--max", "100.00"}, "", "100.00"},
		{"max flag with equals", []string{"--max=100.00"}, "", "100.00"},
		{"both flags", []string{"--min", "10.00", "--max", "100.00"}, "10.00", "100.00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.minAmount != tt.expectedMin {
				t.Errorf("parseArgs(%v) minAmount = %q, want %q", tt.args, opts.minAmount, tt.expectedMin)
			}
			if opts.maxAmount != tt.expectedMax {
				t.Errorf("parseArgs(%v) maxAmount = %q, want %q", tt.args, opts.maxAmount, tt.expectedMax)
			}
		})
	}
}

func TestParseArgs_MinFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--min"})
	if err == nil {
		t.Error("parseArgs(--min) without value should return error")
	}
	if !strings.Contains(err.Error(), "requires") {
		t.Errorf("error should mention requirement, got: %v", err)
	}
}

func TestParseArgs_MaxFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--max"})
	if err == nil {
		t.Error("parseArgs(--max) without value should return error")
	}
	if !strings.Contains(err.Error(), "requires") {
		t.Errorf("error should mention requirement, got: %v", err)
	}
}

func TestParseArgs_ScheduledFlags(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		expectedScheduled bool
		expectedDue       bool
	}{
		{"scheduled flag", []string{"--scheduled"}, true, false},
		{"scheduled with due", []string{"--scheduled", "--due"}, true, true},
		{"due without scheduled", []string{"--due"}, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.scheduled != tt.expectedScheduled {
				t.Errorf("parseArgs(%v) scheduled = %v, want %v", tt.args, opts.scheduled, tt.expectedScheduled)
			}
			if opts.scheduledDue != tt.expectedDue {
				t.Errorf("parseArgs(%v) scheduledDue = %v, want %v", tt.args, opts.scheduledDue, tt.expectedDue)
			}
		})
	}
}

func TestParseArgs_PostScheduledFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{"with space", []string{"--post-scheduled", "abc123"}, "abc123"},
		{"with equals", []string{"--post-scheduled=abc123"}, "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.postScheduled != tt.expected {
				t.Errorf("parseArgs(%v) postScheduled = %q, want %q", tt.args, opts.postScheduled, tt.expected)
			}
		})
	}
}

func TestParseArgs_SkipScheduledFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{"with space", []string{"--skip-scheduled", "abc123"}, "abc123"},
		{"with equals", []string{"--skip-scheduled=abc123"}, "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.skipScheduled != tt.expected {
				t.Errorf("parseArgs(%v) skipScheduled = %q, want %q", tt.args, opts.skipScheduled, tt.expected)
			}
		})
	}
}

func TestParseArgs_PostScheduledMissingID(t *testing.T) {
	_, _, err := parseArgs([]string{"--post-scheduled"})
	if err == nil {
		t.Error("parseArgs(--post-scheduled) without ID should return error")
	}
	if !strings.Contains(err.Error(), "requires a scheduled transaction ID") {
		t.Errorf("error should mention ID requirement, got: %v", err)
	}
}

func TestParseArgs_SkipScheduledMissingID(t *testing.T) {
	_, _, err := parseArgs([]string{"--skip-scheduled"})
	if err == nil {
		t.Error("parseArgs(--skip-scheduled) without ID should return error")
	}
	if !strings.Contains(err.Error(), "requires a scheduled transaction ID") {
		t.Errorf("error should mention ID requirement, got: %v", err)
	}
}

func TestParseArgs_ReportFlag(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantReport bool
		wantType   string
	}{
		{"report flag only", []string{"--report", "net-worth"}, true, "net-worth"},
		{"report spending", []string{"--report", "spending"}, true, "spending"},
		{"report with equals", []string{"--report=net-worth"}, true, "net-worth"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.report != tt.wantReport {
				t.Errorf("report = %v, want %v", opts.report, tt.wantReport)
			}
			if opts.reportType != tt.wantType {
				t.Errorf("reportType = %q, want %q", opts.reportType, tt.wantType)
			}
		})
	}
}

func TestParseArgs_ReportMonthFlag(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantMonth string
	}{
		{"month with space", []string{"--month", "2024-01"}, "2024-01"},
		{"month with equals", []string{"--month=2024-06"}, "2024-06"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.reportMonth != tt.wantMonth {
				t.Errorf("reportMonth = %q, want %q", opts.reportMonth, tt.wantMonth)
			}
		})
	}
}

func TestParseArgs_ReportYearFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantYear int
	}{
		{"year with space", []string{"--year", "2024"}, 2024},
		{"year with equals", []string{"--year=2023"}, 2023},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.reportYear != tt.wantYear {
				t.Errorf("reportYear = %d, want %d", opts.reportYear, tt.wantYear)
			}
		})
	}
}

func TestParseArgs_ReportAsOfFlag(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantVal string
	}{
		{"as-of with space", []string{"--as-of", "2024-01-15"}, "2024-01-15"},
		{"as-of with equals", []string{"--as-of=2023-12-31"}, "2023-12-31"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.reportAsOf != tt.wantVal {
				t.Errorf("reportAsOf = %q, want %q", opts.reportAsOf, tt.wantVal)
			}
		})
	}
}

func TestParseArgs_ReportMonthMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--month"})
	if err == nil {
		t.Error("--month without value should return error")
	}
}

func TestParseArgs_ReportYearMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--year"})
	if err == nil {
		t.Error("--year without value should return error")
	}
}

func TestParseArgs_ReportYearInvalidValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--year", "abc"})
	if err == nil {
		t.Error("--year with non-numeric value should return error")
	}
}

func TestParseArgs_ReportAsOfMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--as-of"})
	if err == nil {
		t.Error("--as-of without value should return error")
	}
}

func TestParseArgs_VoidFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{"with space", []string{"--void", "abc123"}, "abc123"},
		{"with equals", []string{"--void=abc123"}, "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.voidTxn != tt.expected {
				t.Errorf("parseArgs(%v) voidTxn = %q, want %q", tt.args, opts.voidTxn, tt.expected)
			}
		})
	}
}

func TestParseArgs_VoidFlagMissingID(t *testing.T) {
	_, _, err := parseArgs([]string{"--void"})
	if err == nil {
		t.Error("parseArgs(--void) without ID should return error")
	}
	if !strings.Contains(err.Error(), "requires a transaction ID") {
		t.Errorf("error should mention transaction ID requirement, got: %v", err)
	}
}

func TestParseArgs_StatusFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{"uncleared with space", []string{"--status", "uncleared"}, "uncleared"},
		{"cleared with space", []string{"--status", "cleared"}, "cleared"},
		{"reconciled with space", []string{"--status", "reconciled"}, "reconciled"},
		{"void with space", []string{"--status", "void"}, "void"},
		{"with equals", []string{"--status=cleared"}, "cleared"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseArgs(tt.args)
			if err != nil {
				t.Errorf("parseArgs(%v) returned error: %v", tt.args, err)
				return
			}
			if opts.txStatus != tt.expected {
				t.Errorf("parseArgs(%v) txStatus = %q, want %q", tt.args, opts.txStatus, tt.expected)
			}
		})
	}
}

func TestParseArgs_StatusFlagMissingValue(t *testing.T) {
	_, _, err := parseArgs([]string{"--status"})
	if err == nil {
		t.Error("parseArgs(--status) without value should return error")
	}
	if !strings.Contains(err.Error(), "requires a status value") {
		t.Errorf("error should mention status value requirement, got: %v", err)
	}
}

func TestParseYearMonth(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantYear  int
		wantMonth int
		wantErr   bool
	}{
		{"valid January", "2024-01", 2024, 1, false},
		{"valid December", "2024-12", 2024, 12, false},
		{"invalid format", "2024/01", 0, 0, true},
		{"missing month", "2024", 0, 0, true},
		{"invalid year", "abcd-01", 0, 0, true},
		{"invalid month", "2024-ab", 0, 0, true},
		{"month too low", "2024-00", 0, 0, true},
		{"month too high", "2024-13", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			year, month, err := parseYearMonth(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseYearMonth(%q) expected error", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("parseYearMonth(%q) unexpected error: %v", tt.input, err)
				return
			}
			if year != tt.wantYear {
				t.Errorf("parseYearMonth(%q) year = %d, want %d", tt.input, year, tt.wantYear)
			}
			if month != tt.wantMonth {
				t.Errorf("parseYearMonth(%q) month = %d, want %d", tt.input, month, tt.wantMonth)
			}
		})
	}
}

