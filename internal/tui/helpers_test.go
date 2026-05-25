package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

func TestFormatDashboardMoney(t *testing.T) {
	tests := []struct {
		name     string
		money    types.Money
		expected string
	}{
		{"positive", types.MustNewMoney("1234.56"), "$1234.56"},
		{"negative", types.MustNewMoney("-50.00"), "-$50.00"},
		{"zero", types.MustNewMoney("0"), "$0.00"},
		{"large", types.MustNewMoney("99999.99"), "$99999.99"},
		{"small negative", types.MustNewMoney("-0.50"), "-$0.50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDashboardMoney(tt.money)
			if got != tt.expected {
				t.Errorf("formatDashboardMoney(%v) = %q, want %q", tt.money, got, tt.expected)
			}
		})
	}
}
