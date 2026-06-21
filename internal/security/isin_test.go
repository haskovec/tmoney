package security

import "testing"

func TestIsValidISIN(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		// Known-good real ISINs (correct check digits).
		{"apple", "US0378331005", true},
		{"ibm", "US4592001014", true},
		{"bae_systems_gb", "GB0002634946", true},
		{"nestle_ch", "CH0038863350", true},
		{"bmw_de", "DE0005190003", true},
		{"toyota_jp", "JP3633400001", true},
		// Normalization: lower-case and surrounding whitespace are accepted.
		{"lowercase_normalized", "us0378331005", true},
		{"whitespace_trimmed", "  US0378331005  ", true},

		// Wrong check digit (Apple's body with the wrong final digit).
		{"bad_check_digit", "US0378331004", false},
		// Structural failures.
		{"empty", "", false},
		{"too_short", "US037833100", false},
		{"too_long", "US03783310055", false},
		{"country_not_letters", "1S0378331005", false},
		{"check_digit_not_numeric", "US037833100X", false},
		{"body_has_symbol", "US03783-1005", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidISIN(tc.in); got != tc.want {
				t.Errorf("IsValidISIN(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeISIN(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"  us0378331005 ", "US0378331005"},
		{"US0378331005", "US0378331005"},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range tests {
		if got := NormalizeISIN(tc.in); got != tc.want {
			t.Errorf("NormalizeISIN(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
