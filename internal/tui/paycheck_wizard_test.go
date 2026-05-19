package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

// paycheckWizardFixture returns a representative category list (matching
// category.PaycheckCategories) and four active accounts shared between the
// MS-025 / MS-027 wizard tests. Categories are returned in the
// parallel-slice shape that buildCategoryOptions produces (the leading
// "(None)" entry is at index 0).
type paycheckWizardFixture struct {
	categoryOptions []string
	categoryIDs     []types.ID
	salaryID        types.ID
	healthID        types.ID
	federalID       types.ID
	medicareID      types.ID
	socSecID        types.ID
	stateID         types.ID

	accounts     []*account.Account
	checkingID   types.ID
	savingsID    types.ID
	retire401kID types.ID
	hsaID        types.ID
}

func newPaycheckWizardFixture() *paycheckWizardFixture {
	salaryID := types.NewID()
	healthID := types.NewID()
	federalID := types.NewID()
	medicareID := types.NewID()
	socSecID := types.NewID()
	stateID := types.NewID()
	checkingID := types.NewID()
	savingsID := types.NewID()
	retire401kID := types.NewID()
	hsaID := types.NewID()

	return &paycheckWizardFixture{
		categoryOptions: []string{
			"(None)",
			"Income > Salary",
			"Insurance > Health",
			"Tax > Federal",
			"Tax > Medicare",
			"Tax > Social Security",
			"Tax > State",
		},
		categoryIDs: []types.ID{
			types.NilID,
			salaryID,
			healthID,
			federalID,
			medicareID,
			socSecID,
			stateID,
		},
		salaryID:   salaryID,
		healthID:   healthID,
		federalID:  federalID,
		medicareID: medicareID,
		socSecID:   socSecID,
		stateID:    stateID,
		accounts: []*account.Account{
			{BaseModel: types.BaseModel{ID: checkingID}, Name: "Checking", Active: true, Type: account.TypeChecking},
			{BaseModel: types.BaseModel{ID: savingsID}, Name: "Savings", Active: true, Type: account.TypeSavings},
			{BaseModel: types.BaseModel{ID: retire401kID}, Name: "401k", Active: true, Type: account.TypeInvestment},
			{BaseModel: types.BaseModel{ID: hsaID}, Name: "HSA", Active: true, Type: account.TypeInvestment},
		},
		checkingID:   checkingID,
		savingsID:    savingsID,
		retire401kID: retire401kID,
		hsaID:        hsaID,
	}
}

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
	fx := newPaycheckWizardFixture()
	categoryOptions := fx.categoryOptions

	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)
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

// TestPaycheckWizard_Save_ComputesRemainder covers MS-027: when the user
// hits Save, the wizard assembles its form state into a list of signed
// schedule splits and computes the parent net amount (the "primary deposit
// account" remainder) as:
//
//	remainder = gross − sum(pre-tax + post-tax deductions) − sum(additional transfers)
//
// The remainder is derived from the signed sum of the assembled splits, so
// the resulting schedule satisfies the data-model invariant
// (parent.amount == signed_sum(line.amounts), see
// specs/multiline-splits-and-paycheck.md).
//
// Empty amount fields are skipped (no zero-amount split row).
func TestPaycheckWizard_Save_ComputesRemainder(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	// Gross 5000 (categorized as Income > Salary by the wizard default).
	w.GrossAmount().Value = "5000"

	// Pre-tax categorized deductions: 800 + 100 + 310 + 72.50 = 1282.50.
	// The fifth pre-tax line (401(k) transfer) is left empty and must be
	// skipped — empty rows don't produce zero-amount splits.
	preTax := w.PreTaxLines()
	preTax[0].AmountField().Value = "800"   // Federal
	preTax[1].AmountField().Value = "100"   // State
	preTax[2].AmountField().Value = "310"   // Social Security
	preTax[3].AmountField().Value = "72.50" // Medicare

	// Post-tax categorized deduction: 150. HSA transfer-line left empty.
	postTax := w.PostTaxLines()
	postTax[0].AmountField().Value = "150" // Health insurance

	// Total deductions so far: 1282.50 + 150.00 = 1432.50.

	// One additional transfer of 500 to Savings. The wizard's account
	// picker (built via buildSplitTransferAccountOptions) places Savings
	// at index 1 (after Checking at index 0 in the fixture).
	addl := w.AddAdditionalTransfer("Savings", indexOf(w.PrimaryAccount().Options, "Savings"))
	if addl == nil {
		t.Fatal("AddAdditionalTransfer returned nil")
	}
	addl.AmountField().Value = "500"

	// Remainder = 5000 - 1432.50 - 500 = 3067.50.
	parentAmount, splits, err := w.BuildSplits()
	if err != nil {
		t.Fatalf("BuildSplits returned error: %v", err)
	}

	wantParent := types.MustNewMoney("3067.50")
	if !parentAmount.Equal(wantParent) {
		t.Errorf("parent amount = %s, want %s", parentAmount.String(), wantParent.String())
	}

	// Signed sum of splits must equal the parent amount (data-model
	// invariant). This is the source of truth for the "primary deposit
	// account" net of +3067.50.
	total := types.ZeroMoney
	for _, s := range splits {
		total = total.Add(s.Amount)
	}
	if !total.Equal(wantParent) {
		t.Errorf("signed sum of splits = %s, want %s", total.String(), wantParent.String())
	}

	// Expected splits: gross (+5000) + 4 pre-tax categorized + 1 post-tax
	// categorized + 1 additional transfer = 7 rows. Empty 401k and HSA
	// rows are skipped.
	if got := len(splits); got != 7 {
		t.Fatalf("expected 7 splits, got %d", got)
	}

	// Gross is the first split: +5000, categorized as salary.
	gross := splits[0]
	if !gross.Amount.Equal(types.MustNewMoney("5000")) {
		t.Errorf("gross split amount = %s, want 5000", gross.Amount.String())
	}
	if !gross.CategoryID.Valid || gross.CategoryID.ID != fx.salaryID {
		t.Errorf("gross split should be categorized as salary, got CategoryID=%v Valid=%v",
			gross.CategoryID.ID, gross.CategoryID.Valid)
	}
	if gross.TransferAccountID.Valid {
		t.Errorf("gross split should not be a transfer-line")
	}

	// Every deduction / additional-transfer line stored as a negative
	// signed amount (user types positive magnitudes; wizard flips sign).
	for i, s := range splits[1:] {
		if !s.Amount.IsNegative() {
			t.Errorf("split[%d] amount = %s, want negative", i+1, s.Amount.String())
		}
	}

	// Locate the additional-transfer to Savings; its amount must be
	// stored as -500 and it must be a transfer-line, not a categorized
	// line (TransferAccountID set, CategoryID empty).
	var savingsTransfer *scheduled.Split
	for _, s := range splits {
		if s.TransferAccountID.Valid && s.TransferAccountID.ID == fx.savingsID {
			savingsTransfer = s
			break
		}
	}
	if savingsTransfer == nil {
		t.Fatal("expected an additional-transfer split to Savings, found none")
	}
	if !savingsTransfer.Amount.Equal(types.MustNewMoney("-500")) {
		t.Errorf("Savings transfer amount = %s, want -500", savingsTransfer.Amount.String())
	}
	if savingsTransfer.CategoryID.Valid {
		t.Errorf("Savings transfer should not carry a categoryID")
	}

	// Sanity-check the categorized deduction lines: each tax line's
	// signed amount matches the user's input (negated) and the line's
	// category resolves to the expected ID from the fixture.
	wantDeductionByCategory := map[types.ID]types.Money{
		fx.federalID:  types.MustNewMoney("-800"),
		fx.stateID:    types.MustNewMoney("-100"),
		fx.socSecID:   types.MustNewMoney("-310"),
		fx.medicareID: types.MustNewMoney("-72.50"),
		fx.healthID:   types.MustNewMoney("-150"),
	}
	for _, s := range splits {
		if !s.CategoryID.Valid || s.CategoryID.ID == fx.salaryID {
			continue
		}
		want, ok := wantDeductionByCategory[s.CategoryID.ID]
		if !ok {
			t.Errorf("unexpected categorized split for CategoryID=%v amount=%s",
				s.CategoryID.ID, s.Amount.String())
			continue
		}
		if !s.Amount.Equal(want) {
			t.Errorf("split for CategoryID=%v amount = %s, want %s",
				s.CategoryID.ID, s.Amount.String(), want.String())
		}
		delete(wantDeductionByCategory, s.CategoryID.ID)
	}
	if len(wantDeductionByCategory) != 0 {
		for catID := range wantDeductionByCategory {
			t.Errorf("missing categorized split for CategoryID=%v", catID)
		}
	}
}
