package investment

import "testing"

func TestFormatPerformancePct(t *testing.T) {
	annual, cumulative := 7.41, 15.9
	cases := []struct {
		name               string
		annual, cumulative *float64
		want               string
	}{
		{"annual wins over cumulative", &annual, &cumulative, "+7.41% (annual)"},
		{"cumulative under a year", nil, &cumulative, "+15.90% (cumulative)"},
		{"undefined", nil, nil, "—"},
	}
	for _, tc := range cases {
		if got := formatPerformancePct(tc.annual, tc.cumulative); got != tc.want {
			t.Errorf("%s: formatPerformancePct() = %q, want %q", tc.name, got, tc.want)
		}
	}
}
