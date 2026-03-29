package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

func TestFormatMoney(t *testing.T) {
	tests := []struct {
		name     string
		money    types.Money
		currency string
		want     string
	}{
		{"positive USD", types.MustNewMoney("100.50"), "USD", "$100.50"},
		{"negative USD", types.MustNewMoney("-50.25"), "USD", "-$50.25"},
		{"zero USD", types.MustNewMoney("0"), "USD", "$0.00"},
		{"positive EUR", types.MustNewMoney("100.50"), "EUR", "€100.50"},
		{"negative EUR", types.MustNewMoney("-50.25"), "EUR", "-€50.25"},
		{"positive GBP", types.MustNewMoney("100.50"), "GBP", "£100.50"},
		{"other currency", types.MustNewMoney("100.50"), "JPY", "JPY 100.50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMoney(tt.money, tt.currency)
			if got != tt.want {
				t.Errorf("formatMoney(%v, %q) = %q, want %q", tt.money, tt.currency, got, tt.want)
			}
		})
	}
}

func TestPrintVersion(t *testing.T) {
	buf := &bytes.Buffer{}
	printVersion(buf)
	output := buf.String()

	if !strings.Contains(output, "tmoney version") {
		t.Error("version output should contain 'tmoney version'")
	}
	if !strings.Contains(output, "Build time") {
		t.Error("version output should contain 'Build time'")
	}
	if !strings.Contains(output, "Git commit") {
		t.Error("version output should contain 'Git commit'")
	}
}

func TestPrintHelp(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "TMoney") {
		t.Error("help output should contain 'TMoney'")
	}
	if !strings.Contains(output, "--file") {
		t.Error("help output should document --file flag")
	}
	if !strings.Contains(output, "--list-accounts") {
		t.Error("help output should document --list-accounts flag")
	}
	if !strings.Contains(output, "--include-closed") {
		t.Error("help output should document --include-closed flag")
	}
}

func TestPrintHelp_IncludesCreate(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--create") {
		t.Error("help output should document --create flag")
	}
	if !strings.Contains(output, "Create a new database file") {
		t.Error("help output should describe --create functionality")
	}
}

func TestPrintHelp_IncludesAccountAndBalance(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--account") {
		t.Error("help output should document --account flag")
	}
	if !strings.Contains(output, "--balance") {
		t.Error("help output should document --balance flag")
	}
}

func TestPrintHelp_IncludesTransactions(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--transactions") {
		t.Error("help output should document --transactions flag")
	}
	if !strings.Contains(output, "--limit") {
		t.Error("help output should document --limit flag")
	}
	if !strings.Contains(output, "--from") {
		t.Error("help output should document --from flag")
	}
	if !strings.Contains(output, "--to") {
		t.Error("help output should document --to flag")
	}
}

func TestPrintHelp_IncludesAddTransaction(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--add-transaction") {
		t.Error("help output should document --add-transaction flag")
	}
	if !strings.Contains(output, "--amount") {
		t.Error("help output should document --amount flag")
	}
	if !strings.Contains(output, "--payee") {
		t.Error("help output should document --payee flag")
	}
	if !strings.Contains(output, "--category") {
		t.Error("help output should document --category flag")
	}
	if !strings.Contains(output, "--date") {
		t.Error("help output should document --date flag")
	}
	if !strings.Contains(output, "--memo") {
		t.Error("help output should document --memo flag")
	}
}

func TestPrintHelp_IncludesAddAccount(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--add-account") {
		t.Error("help output should document --add-account flag")
	}
	if !strings.Contains(output, "--name") {
		t.Error("help output should document --name flag")
	}
	if !strings.Contains(output, "--type") {
		t.Error("help output should document --type flag")
	}
	if !strings.Contains(output, "--currency") {
		t.Error("help output should document --currency flag")
	}
	if !strings.Contains(output, "--opening-balance") {
		t.Error("help output should document --opening-balance flag")
	}
	if !strings.Contains(output, "--opening-date") {
		t.Error("help output should document --opening-date flag")
	}
	if !strings.Contains(output, "--institution") {
		t.Error("help output should document --institution flag")
	}
	if !strings.Contains(output, "--account-number") {
		t.Error("help output should document --account-number flag")
	}
	if !strings.Contains(output, "--notes") {
		t.Error("help output should document --notes flag")
	}
	if !strings.Contains(output, "--credit-limit") {
		t.Error("help output should document --credit-limit flag")
	}
	if !strings.Contains(output, "--interest-rate") {
		t.Error("help output should document --interest-rate flag")
	}
}

func TestPrintHelp_IncludesTransfer(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--transfer") {
		t.Error("help output should document --transfer flag")
	}
	if !strings.Contains(output, "Source account") || !strings.Contains(output, "--from") {
		t.Error("help output should document --from flag for transfers")
	}
	if !strings.Contains(output, "Destination account") || !strings.Contains(output, "--to") {
		t.Error("help output should document --to flag for transfers")
	}
}

func TestPrintHelp_IncludesSearch(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--search") {
		t.Error("help output should document --search flag")
	}
	if !strings.Contains(output, "--min") {
		t.Error("help output should document --min flag")
	}
	if !strings.Contains(output, "--max") {
		t.Error("help output should document --max flag")
	}
	if !strings.Contains(output, "Search transactions") {
		t.Error("help output should describe search functionality")
	}
}

func TestPrintHelp_IncludesScheduled(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--scheduled") {
		t.Error("help output should document --scheduled flag")
	}
	if !strings.Contains(output, "--due") {
		t.Error("help output should document --due flag")
	}
	if !strings.Contains(output, "--post-scheduled") {
		t.Error("help output should document --post-scheduled flag")
	}
	if !strings.Contains(output, "--skip-scheduled") {
		t.Error("help output should document --skip-scheduled flag")
	}
	if !strings.Contains(output, "Scheduled Transaction Commands") {
		t.Error("help output should have Scheduled Transaction Commands section")
	}
}

func TestPrintHelp_IncludesVoidAndStatus(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--void") {
		t.Error("help output should document --void flag")
	}
	if !strings.Contains(output, "--status") {
		t.Error("help output should document --status flag")
	}
	if !strings.Contains(output, "Void a transaction") {
		t.Error("help output should describe --void functionality")
	}
}

func TestPrintHelp_IncludesReports(t *testing.T) {
	buf := &bytes.Buffer{}
	printHelp(buf)
	output := buf.String()

	if !strings.Contains(output, "--report net-worth") {
		t.Error("help output should document --report net-worth")
	}
	if !strings.Contains(output, "--report spending") {
		t.Error("help output should document --report spending")
	}
	if !strings.Contains(output, "--as-of") {
		t.Error("help output should document --as-of flag")
	}
	if !strings.Contains(output, "--month") {
		t.Error("help output should document --month flag")
	}
	if !strings.Contains(output, "--year") {
		t.Error("help output should document --year flag")
	}
	if !strings.Contains(output, "Report Commands") {
		t.Error("help output should have Report Commands section")
	}
}

