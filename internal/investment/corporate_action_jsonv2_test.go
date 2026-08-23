package investment

import (
	v1 "encoding/json"
	v2 "encoding/json/v2"
	"testing"
)

// TestCorporateActionParamsSurviveJSONV2 pins the corporate-action parameter
// shapes to the same bytes under the v1 and the v2 encoder.
//
// These structs are persisted: ToJSON writes the corporate_actions.parameters
// TEXT column, and ParseSplitParams / ParseMergerParams / ParseSpinOffParams
// read it back to drive share and cost-basis multipliers. Go 1.27 already
// implements encoding/json on top of the v2 engine but keeps v1 semantics, so
// the two agree today only because every omitempty tag here sits on a type
// where they cannot diverge.
//
// The divergence to guard against is omitempty on a bool, number, or pointer:
// v1 asks whether the Go value is empty and drops false and 0, while v2 asks
// whether the encoded JSON is an empty JSON value and keeps them. Adding such a
// tag to any struct below would change the on-disk shape the moment this
// package moves to the v2 API. `omitzero` is the tag that means what v1 meant,
// and it behaves identically under both.
func TestCorporateActionParamsSurviveJSONV2(t *testing.T) {
	cases := []struct {
		name  string
		value any
	}{
		{"SplitParams zero", SplitParams{}},
		{"SplitParams forward 4:1", SplitParams{Numerator: 4, Denominator: 1}},
		{"SplitParams reverse 1:10", SplitParams{Numerator: 1, Denominator: 10}},
		{"MergerParams zero", MergerParams{}},
		{"MergerParams no cash", MergerParams{ExchangeRatio: 2.5}},
		{"MergerParams with cash", MergerParams{ExchangeRatio: 2.5, CashPerShare: 1.25}},
		{"SpinOffParams zero", SpinOffParams{}},
		{"SpinOffParams typical", SpinOffParams{ShareRatio: 0.25, ParentAllocationPct: 80}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, err := v1.Marshal(tc.value)
			if err != nil {
				t.Fatalf("v1 marshal: %v", err)
			}
			got, err := v2.Marshal(tc.value)
			if err != nil {
				t.Fatalf("v2 marshal: %v", err)
			}
			if string(got) != string(want) {
				t.Errorf("encoder disagreement\n  v1: %s\n  v2: %s", want, got)
			}
		})
	}
}
