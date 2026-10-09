package report

import (
	"testing"
	"unicode/utf8"

	"github.com/haskovec/tmoney/internal/types"
)

func TestBar(t *testing.T) {
	m := types.MustNewMoney
	tests := []struct {
		name           string
		value, largest string
		width          int
		want           string
	}{
		{"largest is full", "400", "400", 4, "████"},
		{"half", "200", "400", 4, "██  "},
		{"one eighth", "50", "400", 1, "▏"},
		{"seven eighths", "350", "400", 1, "▉"},
		{"rounds down to the eighth", "399", "400", 1, "▉"},
		{"cell and a part", "300", "400", 2, "█▌"},
		{"too small to draw", "1", "400", 4, "    "},
		{"zero value", "0", "400", 3, "   "},
		{"negative value", "-50", "400", 3, "   "},
		{"zero largest", "50", "0", 3, "   "},
		{"over largest is capped", "800", "400", 2, "██"},
		{"no width", "400", "400", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Bar(m(tt.value), m(tt.largest), tt.width)
			if got != tt.want {
				t.Errorf("Bar(%s, %s, %d) = %q, want %q", tt.value, tt.largest, tt.width, got, tt.want)
			}
			if n := utf8.RuneCountInString(got); n != max(tt.width, 0) {
				t.Errorf("Bar is %d cells, want %d", n, tt.width)
			}
		})
	}
}
