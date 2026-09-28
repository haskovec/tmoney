package cmdutil

import (
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
		{"negative other currency", types.MustNewMoney("-30"), "JPY", "JPY -30.00"},
		// Rounded in decimal, half away from zero; float64 printed these a cent low.
		{"half cent rounds up", types.MustNewMoney("1.005"), "USD", "$1.01"},
		{"half cent rounds up, EUR", types.MustNewMoney("10.225"), "EUR", "€10.23"},
		{"negative half cent", types.MustNewMoney("-1.005"), "USD", "-$1.01"},
		{"rounds to zero, no sign", types.MustNewMoney("-0.004"), "USD", "$0.00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatMoney(tt.money, tt.currency)
			if got != tt.want {
				t.Errorf("FormatMoney(%v, %q) = %q, want %q", tt.money, tt.currency, got, tt.want)
			}
		})
	}
}
