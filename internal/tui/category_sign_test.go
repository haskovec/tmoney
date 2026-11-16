package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
)

func TestSignedAmountText(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		loaded      string
		sign        categorySign
		want        string
		wantChanged bool
	}{
		// Expense
		{"expense fills empty", "", "", categorySignNegative, "-", true},
		{"expense fills blank", "  ", "", categorySignNegative, "-", true},
		{"expense keeps lone minus", "-", "", categorySignNegative, "-", false},
		{"expense negates typed positive", "52.10", "", categorySignNegative, "-52.10", true},
		{"expense negates typed dollar amount", "$52.10", "", categorySignNegative, "-$52.10", true},
		{"expense trims before negating", " 52.10 ", "", categorySignNegative, "-52.10", true},
		{"expense keeps negative", "-52.10", "", categorySignNegative, "-52.10", false},
		{"expense keeps zero", "0", "", categorySignNegative, "0", false},
		{"expense keeps loaded refund", "20.00", "20.00", categorySignNegative, "20.00", false},
		{"expense negates edited loaded amount", "25.00", "20.00", categorySignNegative, "-25.00", true},
		{"expense fills emptied loaded amount", "", "20.00", categorySignNegative, "-", true},
		{"expense leaves unparseable text", "5x", "", categorySignNegative, "5x", false},
		{"expense turns plus into minus", "+52.10", "", categorySignNegative, "-52.10", true},
		{"expense turns dollar plus into minus", "$+52.10", "", categorySignNegative, "$-52.10", true},
		{"expense leaves double sign", "--52.10", "", categorySignNegative, "--52.10", false},
		{"expense negates exponent at the front", "1e+2", "", categorySignNegative, "-1e+2", true},
		{"expense turns dollar plus exponent", "$+1e+2", "", categorySignNegative, "$-1e+2", true},

		// Income
		{"income leaves empty", "", "", categorySignPositive, "", false},
		{"income clears lone minus", "-", "", categorySignPositive, "", true},
		{"income clears padded lone minus", " - ", "", categorySignPositive, "", true},
		{"income negates typed negative", "-52.10", "", categorySignPositive, "52.10", true},
		{"income negates minus dollar", "-$52.10", "", categorySignPositive, "$52.10", true},
		{"income negates dollar minus", "$-52.10", "", categorySignPositive, "$52.10", true},
		{"income keeps positive", "52.10", "", categorySignPositive, "52.10", false},
		{"income keeps loaded negative", "-120.00", "-120.00", categorySignPositive, "-120.00", false},
		{"income leaves double sign", "--52.10", "", categorySignPositive, "--52.10", false},
		{"income negates exponent at the front", "-1e-2", "", categorySignPositive, "1e-2", true},

		// None / system
		{"none leaves empty", "", "", categorySignNone, "", false},
		{"none clears lone minus", "-", "", categorySignNone, "", true},
		{"none keeps negative", "-52.10", "", categorySignNone, "-52.10", false},
		{"none keeps positive", "52.10", "", categorySignNone, "52.10", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := signedAmountText(tt.value, tt.loaded, tt.sign)
			if got != tt.want || changed != tt.wantChanged {
				t.Errorf("signedAmountText(%q, %q, %v) = (%q, %v), want (%q, %v)",
					tt.value, tt.loaded, tt.sign, got, changed, tt.want, tt.wantChanged)
			}
		})
	}
}

func TestCategorySignOf(t *testing.T) {
	groceries := category.NewCategory("Groceries", category.TypeExpense)
	salary := category.NewCategory("Salary", category.TypeIncome)
	valueAdj := category.NewCategory(category.ValueAdjustmentCategoryName, category.TypeExpense)
	valueAdj.IsSystem = true
	cats := []*category.Category{groceries, salary, valueAdj, nil}

	tests := []struct {
		name string
		id   types.ID
		want categorySign
	}{
		{"expense", groceries.ID, categorySignNegative},
		{"income", salary.ID, categorySignPositive},
		{"system expense", valueAdj.ID, categorySignNone},
		{"nil id", types.NilID, categorySignNone},
		{"unknown id", types.NewID(), categorySignNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := categorySignOf(cats, tt.id); got != tt.want {
				t.Errorf("categorySignOf = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCategorySignAt(t *testing.T) {
	groceries := category.NewCategory("Groceries", category.TypeExpense)
	cats := []*category.Category{groceries}
	ids := []types.ID{types.NilID, groceries.ID}

	if got := categorySignAt(cats, ids, 1); got != categorySignNegative {
		t.Errorf("index 1 = %v, want negative", got)
	}
	for _, idx := range []int{0, -1, 2} {
		if got := categorySignAt(cats, ids, idx); got != categorySignNone {
			t.Errorf("index %d = %v, want none", idx, got)
		}
	}
}

func TestApplyCategorySignMovesCursorToEndAndClearsError(t *testing.T) {
	f := &dialog.Field{Type: dialog.FieldText, Error: "Invalid amount"}
	applyCategorySign(f, "", categorySignNegative)
	if f.Value != "-" || f.CursorPos() != 1 {
		t.Fatalf("field = %q cursor %d, want \"-\" cursor 1", f.Value, f.CursorPos())
	}
	if f.Error != "" {
		t.Errorf("Error = %q after the rule changed the value, want it cleared", f.Error)
	}
	for _, r := range "52.10" {
		f.InsertChar(r)
	}
	if f.Value != "-52.10" {
		t.Errorf("typed digits after the fill = %q, want -52.10", f.Value)
	}

	// No change keeps the error: it is still about the text in the field.
	f = &dialog.Field{Type: dialog.FieldText, Value: "5x", Error: "Invalid amount"}
	applyCategorySign(f, "", categorySignNegative)
	if f.Error != "Invalid amount" {
		t.Errorf("Error = %q after no change, want it kept", f.Error)
	}
}

func TestLeadingSignIndex(t *testing.T) {
	for s, want := range map[string]int{
		"-50": 0, "+50": 0, "$-50": 1, "$+50": 1, "50": -1, "$50": -1, "1e-2": -1, "": -1, "$": -1,
	} {
		if got := leadingSignIndex(s); got != want {
			t.Errorf("leadingSignIndex(%q) = %d, want %d", s, got, want)
		}
	}
}
