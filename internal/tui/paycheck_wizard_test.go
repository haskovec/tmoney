package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

// TestPaycheckWizard_OpensWithEmptyForm covers MS-025: the paycheck
// wizard's static-form scaffolding. Invoking NewPaycheckWizard opens a
// dialog whose layout matches the spec — employer, frequency, gross
// pay, the five pre-tax deduction lines, the two post-tax deduction
// lines, and the net pay destinations section — with every input
// field starting empty (amounts blank; category/account selectors
// seeded to the wizard's defaults but not yet user-confirmed).
//
// Save logic, dynamic "+ Add line" affordances, and the
// computed-remainder rendering land in MS-027 / MS-028. MS-025 only
// needs the structural form to open and be visible.
func TestPaycheckWizard_OpensWithEmptyForm(t *testing.T) {
	// Build a representative category list that matches
	// category.PaycheckCategories. The wizard resolves its default
	// category selections by display name against the parallel
	// categoryOptions/categoryIDs slices the caller passes in (the
	// same shape produced by buildCategoryOptions).
	categoryOptions := []string{
		"(None)",
		"Income > Salary",
		"Insurance > Health",
		"Tax > Federal",
		"Tax > Medicare",
		"Tax > Social Security",
		"Tax > State",
	}
	salaryID := types.NewID()
	healthID := types.NewID()
	federalID := types.NewID()
	medicareID := types.NewID()
	socSecID := types.NewID()
	stateID := types.NewID()
	categoryIDs := []types.ID{
		types.NilID,
		salaryID,
		healthID,
		federalID,
		medicareID,
		socSecID,
		stateID,
	}

	checkingID := types.NewID()
	savingsID := types.NewID()
	retire401kID := types.NewID()
	hsaID := types.NewID()
	accounts := []*account.Account{
		{BaseModel: types.BaseModel{ID: checkingID}, Name: "Checking", Active: true, Type: account.TypeChecking},
		{BaseModel: types.BaseModel{ID: savingsID}, Name: "Savings", Active: true, Type: account.TypeSavings},
		{BaseModel: types.BaseModel{ID: retire401kID}, Name: "401k", Active: true, Type: account.TypeInvestment},
		{BaseModel: types.BaseModel{ID: hsaID}, Name: "HSA", Active: true, Type: account.TypeInvestment},
	}

	w := NewPaycheckWizard(categoryOptions, categoryIDs, accounts)
	if w == nil {
		t.Fatal("NewPaycheckWizard returned nil")
	}
	if !w.IsVisible() {
		t.Error("paycheck wizard should be visible after construction")
	}

	// Employer (payee) — text field, starts empty.
	if got := w.Employer().Value; got != "" {
		t.Errorf("employer field should start empty, got %q", got)
	}
	if want := FieldText; w.Employer().Type != want {
		t.Errorf("employer field type = %v, want %v", w.Employer().Type, want)
	}

	// Pay frequency — select field, default Biweekly per the spec mock.
	freqIdx := w.Frequency().SelectedIndex
	freqs := scheduled.AllFrequencies()
	if freqIdx < 0 || freqIdx >= len(freqs) || freqs[freqIdx] != scheduled.FrequencyBiweekly {
		t.Errorf("frequency default = %v, want Biweekly", freqs[freqIdx])
	}

	// Next payday — date field, present and editable. The wizard
	// seeds it with today's date so the schedule has a sane starting
	// point; the test only checks that the field exists and is a
	// FieldDate, not the literal value (which varies with the clock).
	if w.NextPayday().Type != FieldDate {
		t.Errorf("next payday field type = %v, want FieldDate", w.NextPayday().Type)
	}

	// Gross pay — amount text field starts empty; category select
	// defaults to "Income > Salary".
	if got := w.GrossAmount().Value; got != "" {
		t.Errorf("gross amount should start empty, got %q", got)
	}
	if got, want := w.GrossCategory().SelectedIndex, indexOf(categoryOptions, "Income > Salary"); got != want {
		t.Errorf("gross category default = %d, want %d (Income > Salary)", got, want)
	}

	// Pre-tax deduction lines: the spec lists five hardcoded rows in
	// this exact order (federal, state, social security, medicare,
	// 401(k) transfer). The wizard's scaffolding includes them all
	// with empty amounts.
	preTax := w.PreTaxLines()
	wantPreTaxLabels := []string{
		"Federal income tax",
		"State income tax",
		"Social Security",
		"Medicare",
		"401(k) contribution",
	}
	if len(preTax) != len(wantPreTaxLabels) {
		t.Fatalf("expected %d pre-tax lines, got %d", len(wantPreTaxLabels), len(preTax))
	}
	for i, want := range wantPreTaxLabels {
		if preTax[i].Label != want {
			t.Errorf("preTax[%d].Label = %q, want %q", i, preTax[i].Label, want)
		}
		if preTax[i].AmountField().Value != "" {
			t.Errorf("preTax[%d] amount should start empty, got %q", i, preTax[i].AmountField().Value)
		}
	}

	// The first four pre-tax lines are categorized (taxes); the
	// fifth (401(k)) is a transfer line per the spec.
	for i, line := range preTax[:4] {
		if line.IsTransfer() {
			t.Errorf("preTax[%d] (%s) should be categorized, not transfer", i, line.Label)
		}
	}
	if !preTax[4].IsTransfer() {
		t.Errorf("preTax[4] (401(k) contribution) should be a transfer line")
	}

	// Default category selections for the categorized lines match
	// the wizard's spec defaults.
	wantPreTaxCatDefaults := map[string]string{
		"Federal income tax": "Tax > Federal",
		"State income tax":   "Tax > State",
		"Social Security":    "Tax > Social Security",
		"Medicare":           "Tax > Medicare",
	}
	for i, line := range preTax[:4] {
		want := wantPreTaxCatDefaults[line.Label]
		got := categoryOptions[line.CategoryIndex()]
		if got != want {
			t.Errorf("preTax[%d] (%s) category default = %q, want %q",
				i, line.Label, got, want)
		}
	}

	// Post-tax deduction lines: the spec lists two hardcoded rows
	// (health insurance, HSA transfer).
	postTax := w.PostTaxLines()
	wantPostTaxLabels := []string{
		"Health insurance",
		"HSA contribution",
	}
	if len(postTax) != len(wantPostTaxLabels) {
		t.Fatalf("expected %d post-tax lines, got %d", len(wantPostTaxLabels), len(postTax))
	}
	for i, want := range wantPostTaxLabels {
		if postTax[i].Label != want {
			t.Errorf("postTax[%d].Label = %q, want %q", i, postTax[i].Label, want)
		}
		if postTax[i].AmountField().Value != "" {
			t.Errorf("postTax[%d] amount should start empty, got %q", i, postTax[i].AmountField().Value)
		}
	}
	if postTax[0].IsTransfer() {
		t.Errorf("postTax[0] (Health insurance) should be categorized, not transfer")
	}
	if !postTax[1].IsTransfer() {
		t.Errorf("postTax[1] (HSA contribution) should be a transfer line")
	}
	if got, want := categoryOptions[postTax[0].CategoryIndex()], "Insurance > Health"; got != want {
		t.Errorf("postTax[0] category default = %q, want %q", got, want)
	}

	// Net pay destinations: primary deposit account select +
	// additional transfers list (empty in MS-025).
	if w.PrimaryAccount().Type != FieldSelect {
		t.Errorf("primary account field type = %v, want FieldSelect", w.PrimaryAccount().Type)
	}
	// The primary account picker excludes inactive accounts but
	// includes every active one. Defaults to index 0 (Checking in
	// the test fixture).
	if got := w.PrimaryAccount().SelectedIndex; got != 0 {
		t.Errorf("primary account default = %d, want 0 (first active account)", got)
	}
	if got := len(w.PrimaryAccount().Options); got != 4 {
		t.Errorf("primary account options count = %d, want 4", got)
	}

	if got := len(w.AdditionalTransfers()); got != 0 {
		t.Errorf("additional transfers should start empty, got %d", got)
	}
}

// indexOf returns the index of needle in haystack, or -1 if not found.
func indexOf(haystack []string, needle string) int {
	for i, s := range haystack {
		if s == needle {
			return i
		}
	}
	return -1
}
