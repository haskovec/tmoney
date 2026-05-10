package tui

import (
	"strings"
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

func TestPadRight(t *testing.T) {
	tests := []struct {
		input    string
		width    int
		expected string
	}{
		{"abc", 6, "abc   "},
		{"abc", 3, "abc"},
		{"abc", 1, "abc"}, // already wider, no padding
		{"", 4, "    "},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := padRight(tt.input, tt.width)
			if got != tt.expected {
				t.Errorf("padRight(%q, %d) = %q, want %q", tt.input, tt.width, got, tt.expected)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is too long", 10, "this is..."},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"}, // maxLen <= 3, no ellipsis
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.expected {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expected)
			}
		})
	}
}

// TestOverlayDropdown_PreservesBackgroundANSI mirrors the OverlayCenter
// guard: the View → Theme submenu (and any other dropdown) lands on top
// of a colored desktop, and the prefix/suffix bands around the dropdown
// must keep the desktop SGR background. Otherwise dropdowns gain a
// black halo on Turbo Vision.
func TestOverlayDropdown_PreservesBackgroundANSI(t *testing.T) {
	const blueBg = "\x1b[44m"
	const reset = "\x1b[0m"
	bgRow := blueBg + strings.Repeat(" ", 30) + reset
	layout := strings.Join([]string{bgRow, bgRow, bgRow, bgRow}, "\n")

	dropdown := "MENU" // visible width 4

	result := overlayDropdown(layout, dropdown, 5, 1, 30)
	lines := strings.Split(result, "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(lines))
	}

	row := lines[1]
	if !strings.Contains(row, "MENU") {
		t.Fatalf("dropdown row should contain overlay, got %q", row)
	}
	if !strings.Contains(row, blueBg) {
		t.Errorf("dropdown row should preserve blue-bg ANSI on prefix/suffix, got %q", row)
	}
}
