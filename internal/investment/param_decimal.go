package investment

import (
	"fmt"
	"strings"

	"github.com/alpacahq/alpacadecimal"
)

// ParamDecimal is an exact decimal inside a corporate action's stored
// parameters.
//
// It is written as a bare JSON number with the decimal's own digits, so the
// stored shape is the one the old float64 fields wrote: old rows, new rows and
// older binaries all read the same file. It reads a JSON number or a quoted
// string, both exactly — the decimal library parses the raw token, so a stored
// 0.1 never passes through binary floating point on the way in.
type ParamDecimal struct {
	d alpacadecimal.Decimal
}

// ParseParamDecimal parses typed input such as "0.10" or "1.1".
func ParseParamDecimal(s string) (ParamDecimal, error) {
	d, err := alpacadecimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		return ParamDecimal{}, fmt.Errorf("invalid number %q", s)
	}
	return ParamDecimal{d: d}, nil
}

// Decimal returns the value for arithmetic.
func (p ParamDecimal) Decimal() alpacadecimal.Decimal { return p.d }

// IsZero reports whether the value is zero. It is also what `omitzero` asks.
func (p ParamDecimal) IsZero() bool { return p.d.IsZero() }

// String returns the exact digits, with no trailing zeros.
func (p ParamDecimal) String() string { return p.d.String() }

// MarshalJSON writes the decimal as a bare JSON number.
func (p ParamDecimal) MarshalJSON() ([]byte, error) {
	return []byte(p.d.String()), nil
}

// UnmarshalJSON reads a JSON number or a quoted decimal string exactly.
func (p *ParamDecimal) UnmarshalJSON(b []byte) error {
	return p.d.UnmarshalJSON(b)
}
