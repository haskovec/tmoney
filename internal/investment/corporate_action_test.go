package investment

import (
	"encoding/json"
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

// --- SM-140: CorporateActionType enum ---

func TestActionType_IsValid(t *testing.T) {
	valid := []ActionType{
		ActionTypeSplit,
		ActionTypeReverseSplit,
		ActionTypeMerger,
		ActionTypeSpinOff,
	}
	for _, at := range valid {
		if !at.IsValid() {
			t.Errorf("expected %q to be valid", at)
		}
	}

	invalid := []ActionType{"invalid", "stock_split", ""}
	for _, at := range invalid {
		if at.IsValid() {
			t.Errorf("expected %q to be invalid", at)
		}
	}
}

func TestActionType_DisplayName(t *testing.T) {
	tests := []struct {
		at   ActionType
		want string
	}{
		{ActionTypeSplit, "Stock Split"},
		{ActionTypeReverseSplit, "Reverse Split"},
		{ActionTypeMerger, "Merger"},
		{ActionTypeSpinOff, "Spin-Off"},
	}
	for _, tt := range tests {
		if got := tt.at.DisplayName(); got != tt.want {
			t.Errorf("ActionType(%q).DisplayName() = %q, want %q", tt.at, got, tt.want)
		}
	}
}

func TestActionType_RequiresTargetSecurity(t *testing.T) {
	tests := []struct {
		at   ActionType
		want bool
	}{
		{ActionTypeSplit, false},
		{ActionTypeReverseSplit, false},
		{ActionTypeMerger, true},
		{ActionTypeSpinOff, true},
	}
	for _, tt := range tests {
		if got := tt.at.RequiresTargetSecurity(); got != tt.want {
			t.Errorf("ActionType(%q).RequiresTargetSecurity() = %v, want %v", tt.at, got, tt.want)
		}
	}
}

func TestActionType_String(t *testing.T) {
	if got := ActionTypeSplit.String(); got != "split" {
		t.Errorf("ActionTypeSplit.String() = %q, want %q", got, "split")
	}
}

func TestAllActionTypes(t *testing.T) {
	all := AllActionTypes()
	if len(all) != 4 {
		t.Errorf("AllActionTypes() returned %d types, want 4", len(all))
	}
}

func TestParseActionType(t *testing.T) {
	tests := []struct {
		input   string
		want    ActionType
		wantErr bool
	}{
		{"split", ActionTypeSplit, false},
		{"SPLIT", ActionTypeSplit, false},
		{"reverse_split", ActionTypeReverseSplit, false},
		{"merger", ActionTypeMerger, false},
		{"spin_off", ActionTypeSpinOff, false},
		{"invalid", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		got, err := ParseActionType(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseActionType(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseActionType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestActionType_ValueScan(t *testing.T) {
	at := ActionTypeMerger
	val, err := at.Value()
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}
	if val != "merger" {
		t.Errorf("Value() = %v, want %q", val, "merger")
	}

	var scanned ActionType
	err = scanned.Scan("spin_off")
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if scanned != ActionTypeSpinOff {
		t.Errorf("Scan() = %q, want %q", scanned, ActionTypeSpinOff)
	}

	err = scanned.Scan([]byte("split"))
	if err != nil {
		t.Fatalf("Scan([]byte) error = %v", err)
	}
	if scanned != ActionTypeSplit {
		t.Errorf("Scan([]byte) = %q, want %q", scanned, ActionTypeSplit)
	}

	err = scanned.Scan(nil)
	if err != nil {
		t.Fatalf("Scan(nil) error = %v", err)
	}
	if scanned != "" {
		t.Errorf("Scan(nil) = %q, want empty", scanned)
	}

	err = scanned.Scan(123)
	if err == nil {
		t.Error("Scan(int) expected error")
	}
}

// --- SM-141: CorporateAction model ---

func TestCorporateAction_Validate(t *testing.T) {
	secID := types.NewID()
	targetID := types.NewID()
	date := types.NewDate(2024, 8, 1)

	t.Run("valid split action", func(t *testing.T) {
		ca := NewCorporateAction(ActionTypeSplit, secID, date, `{"numerator":4,"denominator":1}`)
		errs := ca.Validate()
		if errs.HasErrors() {
			t.Errorf("expected no errors, got: %v", errs)
		}
	})

	t.Run("valid merger action with target", func(t *testing.T) {
		ca := NewCorporateAction(ActionTypeMerger, secID, date, `{"exchange_ratio":2.5}`)
		ca.SetTargetSecurity(targetID)
		errs := ca.Validate()
		if errs.HasErrors() {
			t.Errorf("expected no errors, got: %v", errs)
		}
	})

	t.Run("valid spin-off action with target", func(t *testing.T) {
		ca := NewCorporateAction(ActionTypeSpinOff, secID, date, `{"share_ratio":0.25,"parent_allocation_pct":80}`)
		ca.SetTargetSecurity(targetID)
		errs := ca.Validate()
		if errs.HasErrors() {
			t.Errorf("expected no errors, got: %v", errs)
		}
	})

	t.Run("missing security_id", func(t *testing.T) {
		ca := NewCorporateAction(ActionTypeSplit, types.ID{}, date, `{"numerator":4,"denominator":1}`)
		errs := ca.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for missing security_id")
		}
	})

	t.Run("missing action_date", func(t *testing.T) {
		ca := NewCorporateAction(ActionTypeSplit, secID, types.Date{}, `{"numerator":4,"denominator":1}`)
		errs := ca.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for missing action_date")
		}
	})

	t.Run("missing parameters", func(t *testing.T) {
		ca := NewCorporateAction(ActionTypeSplit, secID, date, "")
		errs := ca.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for missing parameters")
		}
	})

	t.Run("invalid action_type", func(t *testing.T) {
		ca := NewCorporateAction(ActionType("bogus"), secID, date, "{}")
		errs := ca.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for invalid action_type")
		}
	})

	t.Run("merger without target_security_id", func(t *testing.T) {
		ca := NewCorporateAction(ActionTypeMerger, secID, date, `{"exchange_ratio":2.5}`)
		errs := ca.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for merger missing target_security_id")
		}
		found := false
		for _, e := range errs {
			if e.Field == "target_security_id" {
				found = true
			}
		}
		if !found {
			t.Error("expected target_security_id error")
		}
	})

	t.Run("spin_off without target_security_id", func(t *testing.T) {
		ca := NewCorporateAction(ActionTypeSpinOff, secID, date, `{"share_ratio":0.25}`)
		errs := ca.Validate()
		if !errs.HasErrors() {
			t.Error("expected validation error for spin_off missing target_security_id")
		}
	})

	t.Run("split does not require target_security_id", func(t *testing.T) {
		ca := NewCorporateAction(ActionTypeSplit, secID, date, `{"numerator":4,"denominator":1}`)
		// No target security set — should still be valid
		errs := ca.Validate()
		if errs.HasErrors() {
			t.Errorf("split should not require target_security_id, got: %v", errs)
		}
	})
}

func TestCorporateAction_IsValid(t *testing.T) {
	secID := types.NewID()
	date := types.NewDate(2024, 8, 1)

	ca := NewCorporateAction(ActionTypeSplit, secID, date, `{"numerator":4,"denominator":1}`)
	if !ca.IsValid() {
		t.Error("expected IsValid() to return true for valid action")
	}

	bad := NewCorporateAction(ActionType(""), types.ID{}, types.Date{}, "")
	if bad.IsValid() {
		t.Error("expected IsValid() to return false for invalid action")
	}
}

func TestNewCorporateAction(t *testing.T) {
	secID := types.NewID()
	date := types.NewDate(2024, 8, 1)
	params := `{"numerator":4,"denominator":1}`

	ca := NewCorporateAction(ActionTypeSplit, secID, date, params)

	if ca.ID.IsNil() {
		t.Error("expected non-nil ID")
	}
	if ca.ActionType != ActionTypeSplit {
		t.Errorf("expected ActionType %q, got %q", ActionTypeSplit, ca.ActionType)
	}
	if ca.SecurityID != secID {
		t.Error("expected matching SecurityID")
	}
	if ca.ActionDate != date {
		t.Error("expected matching ActionDate")
	}
	if ca.Parameters != params {
		t.Errorf("expected parameters %q, got %q", params, ca.Parameters)
	}
	if ca.TargetSecurityID.Valid {
		t.Error("expected TargetSecurityID to not be set")
	}
}

func TestCorporateAction_SetTargetSecurity(t *testing.T) {
	secID := types.NewID()
	targetID := types.NewID()
	date := types.NewDate(2024, 8, 1)

	ca := NewCorporateAction(ActionTypeMerger, secID, date, `{}`)
	ca.SetTargetSecurity(targetID)

	if !ca.TargetSecurityID.Valid {
		t.Error("expected TargetSecurityID to be valid")
	}
	if ca.TargetSecurityID.ID != targetID {
		t.Error("expected TargetSecurityID to match")
	}
}

// --- SM-142: SplitParams ---

func TestSplitParams_Ratio(t *testing.T) {
	tests := []struct {
		name string
		sp   SplitParams
		want float64
	}{
		{"4:1 split", SplitParams{Numerator: 4, Denominator: 1}, 4.0},
		{"2:1 split", SplitParams{Numerator: 2, Denominator: 1}, 2.0},
		{"1:10 reverse", SplitParams{Numerator: 1, Denominator: 10}, 0.1},
		{"3:2 split", SplitParams{Numerator: 3, Denominator: 2}, 1.5},
		{"zero denominator", SplitParams{Numerator: 4, Denominator: 0}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.sp.Ratio()
			if got != tt.want {
				t.Errorf("Ratio() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSplitParams_RatioString(t *testing.T) {
	sp := SplitParams{Numerator: 4, Denominator: 1}
	if got := sp.RatioString(); got != "4:1" {
		t.Errorf("RatioString() = %q, want %q", got, "4:1")
	}
}

func TestSplitParams_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		sp := SplitParams{Numerator: 4, Denominator: 1}
		errs := sp.Validate()
		if errs.HasErrors() {
			t.Errorf("expected no errors, got: %v", errs)
		}
	})

	t.Run("zero numerator", func(t *testing.T) {
		sp := SplitParams{Numerator: 0, Denominator: 1}
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("expected error for zero numerator")
		}
	})

	t.Run("negative numerator", func(t *testing.T) {
		sp := SplitParams{Numerator: -1, Denominator: 1}
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("expected error for negative numerator")
		}
	})

	t.Run("zero denominator", func(t *testing.T) {
		sp := SplitParams{Numerator: 4, Denominator: 0}
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("expected error for zero denominator")
		}
	})

	t.Run("negative denominator", func(t *testing.T) {
		sp := SplitParams{Numerator: 4, Denominator: -1}
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("expected error for negative denominator")
		}
	})

	t.Run("both positive", func(t *testing.T) {
		sp := SplitParams{Numerator: 1, Denominator: 10}
		errs := sp.Validate()
		if errs.HasErrors() {
			t.Errorf("expected no errors for reverse split, got: %v", errs)
		}
	})
}

func TestSplitParams_JSON(t *testing.T) {
	sp := SplitParams{Numerator: 4, Denominator: 1}

	jsonStr, err := sp.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	parsed, err := ParseSplitParams(jsonStr)
	if err != nil {
		t.Fatalf("ParseSplitParams() error = %v", err)
	}

	if parsed.Numerator != sp.Numerator || parsed.Denominator != sp.Denominator {
		t.Errorf("round-trip failed: got %+v, want %+v", parsed, sp)
	}
}

func TestSplitParams_JSONStructure(t *testing.T) {
	sp := SplitParams{Numerator: 4, Denominator: 1}
	jsonStr, _ := sp.ToJSON()

	var raw map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if raw["numerator"] != float64(4) {
		t.Errorf("expected numerator=4, got %v", raw["numerator"])
	}
	if raw["denominator"] != float64(1) {
		t.Errorf("expected denominator=1, got %v", raw["denominator"])
	}
}

func TestParseSplitParams_InvalidJSON(t *testing.T) {
	_, err := ParseSplitParams("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseSplitRatio(t *testing.T) {
	tests := []struct {
		input   string
		wantN   int
		wantD   int
		wantErr bool
	}{
		{"4:1", 4, 1, false},
		{"1:10", 1, 10, false},
		{"3:2", 3, 2, false},
		{"invalid", 0, 0, true},
		{"4", 0, 0, true},
		{"a:b", 0, 0, true},
		{"4:b", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			sp, err := ParseSplitRatio(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSplitRatio(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if sp.Numerator != tt.wantN || sp.Denominator != tt.wantD {
					t.Errorf("ParseSplitRatio(%q) = %d:%d, want %d:%d", tt.input, sp.Numerator, sp.Denominator, tt.wantN, tt.wantD)
				}
			}
		})
	}
}

// --- SM-143: MergerParams ---

func TestMergerParams_Validate(t *testing.T) {
	t.Run("valid with cash", func(t *testing.T) {
		mp := MergerParams{ExchangeRatio: 2.5, CashPerShare: 5.00}
		errs := mp.Validate()
		if errs.HasErrors() {
			t.Errorf("expected no errors, got: %v", errs)
		}
	})

	t.Run("valid without cash", func(t *testing.T) {
		mp := MergerParams{ExchangeRatio: 1.0}
		errs := mp.Validate()
		if errs.HasErrors() {
			t.Errorf("expected no errors, got: %v", errs)
		}
	})

	t.Run("zero exchange_ratio", func(t *testing.T) {
		mp := MergerParams{ExchangeRatio: 0}
		errs := mp.Validate()
		if !errs.HasErrors() {
			t.Error("expected error for zero exchange_ratio")
		}
	})

	t.Run("negative exchange_ratio", func(t *testing.T) {
		mp := MergerParams{ExchangeRatio: -1.0}
		errs := mp.Validate()
		if !errs.HasErrors() {
			t.Error("expected error for negative exchange_ratio")
		}
	})

	t.Run("negative cash_per_share", func(t *testing.T) {
		mp := MergerParams{ExchangeRatio: 2.0, CashPerShare: -1.0}
		errs := mp.Validate()
		if !errs.HasErrors() {
			t.Error("expected error for negative cash_per_share")
		}
	})
}

func TestMergerParams_HasCashConsideration(t *testing.T) {
	mp := MergerParams{ExchangeRatio: 2.0, CashPerShare: 5.00}
	if !mp.HasCashConsideration() {
		t.Error("expected HasCashConsideration() = true")
	}

	mp2 := MergerParams{ExchangeRatio: 2.0}
	if mp2.HasCashConsideration() {
		t.Error("expected HasCashConsideration() = false for zero cash")
	}
}

func TestMergerParams_JSON(t *testing.T) {
	mp := MergerParams{ExchangeRatio: 2.5, CashPerShare: 5.00}

	jsonStr, err := mp.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	parsed, err := ParseMergerParams(jsonStr)
	if err != nil {
		t.Fatalf("ParseMergerParams() error = %v", err)
	}

	if parsed.ExchangeRatio != mp.ExchangeRatio || parsed.CashPerShare != mp.CashPerShare {
		t.Errorf("round-trip failed: got %+v, want %+v", parsed, mp)
	}
}

func TestMergerParams_JSON_OmitEmptyCash(t *testing.T) {
	mp := MergerParams{ExchangeRatio: 2.0}
	jsonStr, _ := mp.ToJSON()

	var raw map[string]any
	json.Unmarshal([]byte(jsonStr), &raw)

	if _, ok := raw["cash_per_share"]; ok {
		t.Error("expected cash_per_share to be omitted when zero")
	}
}

func TestParseMergerParams_InvalidJSON(t *testing.T) {
	_, err := ParseMergerParams("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- SM-144: SpinOffParams ---

func TestSpinOffParams_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		sp := SpinOffParams{ShareRatio: 0.25, ParentAllocationPct: 80}
		errs := sp.Validate()
		if errs.HasErrors() {
			t.Errorf("expected no errors, got: %v", errs)
		}
	})

	t.Run("zero share_ratio", func(t *testing.T) {
		sp := SpinOffParams{ShareRatio: 0, ParentAllocationPct: 80}
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("expected error for zero share_ratio")
		}
	})

	t.Run("negative share_ratio", func(t *testing.T) {
		sp := SpinOffParams{ShareRatio: -0.5, ParentAllocationPct: 80}
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("expected error for negative share_ratio")
		}
	})

	t.Run("zero parent_allocation_pct", func(t *testing.T) {
		sp := SpinOffParams{ShareRatio: 0.25, ParentAllocationPct: 0}
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("expected error for zero parent_allocation_pct")
		}
	})

	t.Run("100 parent_allocation_pct", func(t *testing.T) {
		sp := SpinOffParams{ShareRatio: 0.25, ParentAllocationPct: 100}
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("expected error for 100% parent_allocation_pct")
		}
	})

	t.Run("over 100 parent_allocation_pct", func(t *testing.T) {
		sp := SpinOffParams{ShareRatio: 0.25, ParentAllocationPct: 101}
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("expected error for >100 parent_allocation_pct")
		}
	})

	t.Run("negative parent_allocation_pct", func(t *testing.T) {
		sp := SpinOffParams{ShareRatio: 0.25, ParentAllocationPct: -10}
		errs := sp.Validate()
		if !errs.HasErrors() {
			t.Error("expected error for negative parent_allocation_pct")
		}
	})
}

func TestSpinOffParams_SpinOffAllocationPct(t *testing.T) {
	sp := SpinOffParams{ShareRatio: 0.25, ParentAllocationPct: 80}
	if got := sp.SpinOffAllocationPct(); got != 20 {
		t.Errorf("SpinOffAllocationPct() = %v, want 20", got)
	}
}

func TestSpinOffParams_JSON(t *testing.T) {
	sp := SpinOffParams{ShareRatio: 0.25, ParentAllocationPct: 80}

	jsonStr, err := sp.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	parsed, err := ParseSpinOffParams(jsonStr)
	if err != nil {
		t.Fatalf("ParseSpinOffParams() error = %v", err)
	}

	if parsed.ShareRatio != sp.ShareRatio || parsed.ParentAllocationPct != sp.ParentAllocationPct {
		t.Errorf("round-trip failed: got %+v, want %+v", parsed, sp)
	}
}

func TestSpinOffParams_JSONStructure(t *testing.T) {
	sp := SpinOffParams{ShareRatio: 0.25, ParentAllocationPct: 80}
	jsonStr, _ := sp.ToJSON()

	var raw map[string]any
	json.Unmarshal([]byte(jsonStr), &raw)

	if raw["share_ratio"] != 0.25 {
		t.Errorf("expected share_ratio=0.25, got %v", raw["share_ratio"])
	}
	if raw["parent_allocation_pct"] != float64(80) {
		t.Errorf("expected parent_allocation_pct=80, got %v", raw["parent_allocation_pct"])
	}
}

func TestParseSpinOffParams_InvalidJSON(t *testing.T) {
	_, err := ParseSpinOffParams("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
