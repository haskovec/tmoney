package widget

import (
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

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
			got := PadRight(tt.input, tt.width)
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
			got := Truncate(tt.input, tt.maxLen)
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

	result := OverlayDropdown(layout, dropdown, 5, 1, 30)
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

func TestFitParts(t *testing.T) {
	parts := []string{"Cash: $1.00", "Total: $22.00", "G/L: $333.00"}
	drop := []int{0, 2} // Cash goes first, then G/L; Total stays
	tests := []struct {
		width int
		want  string
	}{
		{100, "Cash: $1.00  Total: $22.00  G/L: $333.00"},
		{40, "Cash: $1.00  Total: $22.00  G/L: $333.00"},
		{39, "Total: $22.00  G/L: $333.00"},
		{27, "Total: $22.00  G/L: $333.00"},
		{26, "Total: $22.00"},
		{13, "Total: $22.00"},
		{9, "Total: $…"},
		{0, ""},
	}
	for _, tt := range tests {
		if got := FitParts(parts, "  ", tt.width, drop); got != tt.want {
			t.Errorf("FitParts(width %d) = %q, want %q", tt.width, got, tt.want)
		}
	}

	if got := FitParts([]string{"a", "b"}, " ", 0, []int{1, 0}); got != "" {
		t.Errorf("FitParts with every part droppable = %q, want empty", got)
	}

	// Width counts cells, not bytes, and the styles of a whole part stay.
	styled := []string{lipgloss.NewStyle().Bold(true).Render("Café"), "東京"}
	if got := FitParts(styled, " ", 9, []int{1}); StripAnsi(got) != "Café 東京" {
		t.Errorf("FitParts(styled, 9) = %q, want both parts", got)
	}
}

// A drop index that is out of range is skipped, so a parts slice that later
// loses an entry cannot make a render panic.
func TestFitParts_SkipsBadDropIndex(t *testing.T) {
	if got := FitParts([]string{"aa", "bb"}, " ", 2, []int{5, -1, 1}); got != "aa" {
		t.Errorf("FitParts = %q, want %q", got, "aa")
	}
}

// One Backspace removes one character as the user sees it: a grapheme
// cluster, which can be several runes ("e" + U+0301, a skin tone, a ZWJ
// family, a flag). The result is always valid UTF-8.
func TestDropLastGrapheme(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"Acme", "Acm"},
		{"Café", "Caf"},
		{"Café", "Caf"},
		{"東京", "東"},
		{"👍🏽", ""},
		{"a👨‍👩‍👧", "a"},
		{"a🇺🇸🇨🇦", "a🇺🇸"},
		{"1️⃣", ""},
	}
	for _, tt := range tests {
		got := DropLastGrapheme(tt.in)
		if got != tt.want || !utf8.ValidString(got) {
			t.Errorf("DropLastGrapheme(%+q) = %+q, want %+q", tt.in, got, tt.want)
		}
	}
}

// Truncate counts cells and never splits a character.
func TestTruncate_CountsCells(t *testing.T) {
	tests := []struct {
		in    string
		width int
		want  string
	}{
		{"Café", 4, "Café"},         // 5 bytes, 4 cells: fits
		{"ÉÉÉÉÉÉÉÉ", 8, "ÉÉÉÉÉÉÉÉ"}, // 16 bytes, 8 cells: fits
		{"ÉÉÉÉÉÉÉÉ", 6, "ÉÉÉ..."},   // cut on a character, not a byte
		{"東京銀行普通", 7, "東京..."},      // 2-cell characters
	}
	for _, tt := range tests {
		got := Truncate(tt.in, tt.width)
		if got != tt.want || !utf8.ValidString(got) {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.in, tt.width, got, tt.want)
		}
	}
}
