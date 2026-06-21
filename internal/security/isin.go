package security

import "strings"

// NormalizeISIN trims surrounding whitespace and upper-cases an ISIN. An ISIN
// is canonically upper-case; we normalize before storing, validating, and
// comparing so that "us0378331005" and "US0378331005" are treated as one value.
func NormalizeISIN(isin string) string {
	return strings.ToUpper(strings.TrimSpace(isin))
}

// IsValidISIN reports whether s is a structurally valid ISIN (ISO 6166) with a
// correct check digit. The input is normalized first, so casing/whitespace do
// not matter. An empty string is NOT a valid ISIN — callers treat "" as
// "no ISIN recorded" and should skip this check for the empty case.
//
// Structure: 2-letter country code + 9 alphanumeric (the NSIN) + 1 check digit.
// The check digit uses the "modulus 10 double-add-double" (Luhn) scheme over
// the digit expansion of the first 11 characters (letters expand A=10..Z=35).
func IsValidISIN(s string) bool {
	s = NormalizeISIN(s)
	if len(s) != 12 {
		return false
	}
	// First two characters: country code (letters).
	if !isUpperLetter(s[0]) || !isUpperLetter(s[1]) {
		return false
	}
	// Characters 3..11 (index 2..10): alphanumeric body.
	for i := 2; i < 11; i++ {
		if !isUpperLetter(s[i]) && !isDigit(s[i]) {
			return false
		}
	}
	// Final character: numeric check digit.
	if !isDigit(s[11]) {
		return false
	}
	want := int(s[11] - '0')
	return isinCheckDigit(s[:11]) == want
}

// isinCheckDigit computes the ISO 6166 check digit for the first 11 characters
// of an ISIN (the country code + NSIN, without the trailing check digit).
// Returns -1 if body contains an unexpected character (callers validate the
// alphabet first, so this is defensive).
func isinCheckDigit(body string) int {
	// Expand letters to two digits (A=10..Z=35), digits to one digit.
	digits := make([]int, 0, len(body)*2)
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case isDigit(c):
			digits = append(digits, int(c-'0'))
		case isUpperLetter(c):
			v := int(c-'A') + 10
			digits = append(digits, v/10, v%10)
		default:
			return -1
		}
	}

	// Luhn: starting from the rightmost expanded digit (position 1), double the
	// digits in odd positions; reduce results > 9 by subtracting 9 (== summing
	// their two decimal digits). The check digit slot is the next position to
	// the right, so the rightmost existing digit is one that gets doubled.
	sum := 0
	for i := 0; i < len(digits); i++ {
		d := digits[len(digits)-1-i]
		if i%2 == 0 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return (10 - (sum % 10)) % 10
}

func isUpperLetter(c byte) bool { return c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool       { return c >= '0' && c <= '9' }
