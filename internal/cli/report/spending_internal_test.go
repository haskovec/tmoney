package report

import "testing"

// TestParseYearMonth is a white-box test: parseYearMonth is unexported, so this
// file stays in package report (internal) rather than the external
// package report_test that holds the command-level spending tests.
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
