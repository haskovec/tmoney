package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// paycheckWizardFixture returns a representative category list +
// account list used across wizard tests.
type paycheckWizardFixture struct {
	categoryOptions []string
	categoryIDs     []types.ID
	salaryID        types.ID
	healthID        types.ID
	federalID       types.ID
	ssID            types.ID
	medicareID      types.ID

	accounts     []*account.Account
	checkingID   types.ID
	savingsID    types.ID
	retire401kID types.ID
}

func newPaycheckWizardFixture() *paycheckWizardFixture {
	salaryID := types.NewID()
	healthID := types.NewID()
	federalID := types.NewID()
	ssID := types.NewID()
	medicareID := types.NewID()
	checkingID := types.NewID()
	savingsID := types.NewID()
	retire401kID := types.NewID()

	return &paycheckWizardFixture{
		categoryOptions: []string{
			"(None)",
			"Income > Salary",
			"Insurance > Health",
			"Tax > Federal",
			"Tax > Social Security",
			"Tax > Medicare",
		},
		categoryIDs: []types.ID{
			types.NilID,
			salaryID,
			healthID,
			federalID,
			ssID,
			medicareID,
		},
		salaryID:   salaryID,
		healthID:   healthID,
		federalID:  federalID,
		ssID:       ssID,
		medicareID: medicareID,
		accounts: []*account.Account{
			{BaseModel: types.BaseModel{ID: checkingID}, Name: "Checking", Active: true, Type: account.TypeChecking},
			{BaseModel: types.BaseModel{ID: savingsID}, Name: "Savings", Active: true, Type: account.TypeSavings},
			{BaseModel: types.BaseModel{ID: retire401kID}, Name: "401k", Active: true, Type: account.TypeInvestment},
		},
		checkingID:   checkingID,
		savingsID:    savingsID,
		retire401kID: retire401kID,
	}
}

// indexOf is a small helper used in several tests.
func indexOf(haystack []string, needle string) int {
	for i, s := range haystack {
		if s == needle {
			return i
		}
	}
	return -1
}

// TestPaycheckWizard_V2Layout_OpensWithSpecPrePopulation asserts the v2
// wizard opens with five sections pre-populated per
// specs/multiline-splits-and-paycheck.md "Pre-populated rows" table:
// Earnings has one Income:Salary row, Pre-tax is empty, Taxes has
// three rows (Federal/Social Security/Medicare), Post-tax is empty,
// and Net Pay Destinations has no additional transfers (the primary
// deposit picker lives in the header).
func TestPaycheckWizard_V2Layout_OpensWithSpecPrePopulation(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)
	if w == nil {
		t.Fatal("NewPaycheckWizard returned nil")
	}
	if !w.IsVisible() {
		t.Error("wizard should be visible after construction")
	}

	// Header field defaults (carried over from v1).
	if got := w.Employer().Value; got != "" {
		t.Errorf("employer should start empty, got %q", got)
	}
	if w.Employer().Type != dialog.FieldText {
		t.Errorf("employer should be dialog.FieldText, got %v", w.Employer().Type)
	}
	if got, want := w.Frequency().SelectedIndex, defaultPaycheckFrequencyIndex; got != want {
		t.Errorf("frequency default = %d, want %d (Fortnightly)", got, want)
	}
	if opt := paycheckFrequencyForIndex(w.Frequency().SelectedIndex); opt.frequency != scheduled.FrequencyFortnightly {
		t.Errorf("default frequency option = %v, want fortnightly", opt.frequency)
	}
	if w.NextPayday().Type != dialog.FieldDate {
		t.Errorf("next payday should be dialog.FieldDate, got %v", w.NextPayday().Type)
	}
	if w.NextPayday().Value == "" {
		t.Error("next payday should be seeded with today's date")
	}
	if w.DepositAccount().Type != dialog.FieldSelect {
		t.Errorf("deposit account should be dialog.FieldSelect, got %v", w.DepositAccount().Type)
	}
	if got := w.DepositAccount().Options[w.DepositAccount().SelectedIndex]; got != "Checking" {
		t.Errorf("deposit default = %q, want Checking", got)
	}

	// Earnings: 1 row pre-populated with Income > Salary, empty amount.
	earnings := w.EarningsLines()
	if got := len(earnings); got != 1 {
		t.Fatalf("EarningsLines count = %d, want 1", got)
	}
	if got, want := selectedLineOption(earnings[0]), "Income > Salary"; got != want {
		t.Errorf("Earnings row[0] category = %q, want %q", got, want)
	}
	if earnings[0].AmountField().Value != "" {
		t.Errorf("Earnings row[0] amount = %q, want empty",
			earnings[0].AmountField().Value)
	}
	if earnings[0].Section != PaycheckEarnings {
		t.Errorf("Earnings row[0] Section = %v, want PaycheckEarnings",
			earnings[0].Section)
	}

	// Pre-tax: 0 rows (added via [+ Add pre-tax line]).
	if got := len(w.PreTaxLines()); got != 0 {
		t.Errorf("PreTaxLines count = %d, want 0", got)
	}

	// Taxes: 3 rows pre-populated in order Federal, Social Security,
	// Medicare; all with empty amounts.
	tax := w.TaxLines()
	if got := len(tax); got != 3 {
		t.Fatalf("TaxLines count = %d, want 3", got)
	}
	wantTaxCats := []string{"Tax > Federal", "Tax > Social Security", "Tax > Medicare"}
	for i, want := range wantTaxCats {
		if got := selectedLineOption(tax[i]); got != want {
			t.Errorf("Tax row[%d] category = %q, want %q", i, got, want)
		}
		if tax[i].AmountField().Value != "" {
			t.Errorf("Tax row[%d] amount = %q, want empty",
				i, tax[i].AmountField().Value)
		}
		if tax[i].Section != PaycheckTax {
			t.Errorf("Tax row[%d] Section = %v, want PaycheckTax",
				i, tax[i].Section)
		}
	}

	// Post-tax: 0 rows (added via [+ Add post-tax line]).
	if got := len(w.PostTaxLines()); got != 0 {
		t.Errorf("PostTaxLines count = %d, want 0", got)
	}

	// Net Pay Destinations: 0 additional transfers. The primary
	// deposit picker is in the header (w.DepositAccount()).
	if got := len(w.AdditionalTransfers()); got != 0 {
		t.Errorf("AdditionalTransfers count = %d, want 0", got)
	}
}

// selectedLineOption returns the display string currently selected in
// a paycheck line's category-or-transfer picker, or "" if unset.
func selectedLineOption(line *PaycheckLine) string {
	f := line.SelectField()
	if f == nil || f.SelectedIndex < 0 || f.SelectedIndex >= len(f.Options) {
		return ""
	}
	return f.Options[f.SelectedIndex]
}

// TestPaycheckWizard_AddRow_AppendsToSection asserts AddRow appends an
// empty row to the requested section (on top of any v2-pre-populated
// rows) and that the line knows its section.
func TestPaycheckWizard_AddRow_AppendsToSection(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	earningsBefore := len(w.EarningsLines())
	preBefore := len(w.PreTaxLines())
	taxBefore := len(w.TaxLines())
	postBefore := len(w.PostTaxLines())

	earn := w.AddRow(PaycheckEarnings)
	pre := w.AddRow(PaycheckPreTax)
	tax := w.AddRow(PaycheckTax)
	post := w.AddRow(PaycheckPostTax)

	if earn.Section != PaycheckEarnings {
		t.Errorf("earn.Section = %v, want PaycheckEarnings", earn.Section)
	}
	if pre.Section != PaycheckPreTax {
		t.Errorf("pre.Section = %v, want PaycheckPreTax", pre.Section)
	}
	if tax.Section != PaycheckTax {
		t.Errorf("tax.Section = %v, want PaycheckTax", tax.Section)
	}
	if post.Section != PaycheckPostTax {
		t.Errorf("post.Section = %v, want PaycheckPostTax", post.Section)
	}

	if got, want := len(w.EarningsLines()), earningsBefore+1; got != want {
		t.Errorf("earnings count = %d, want %d", got, want)
	}
	if got, want := len(w.PreTaxLines()), preBefore+1; got != want {
		t.Errorf("pre-tax count = %d, want %d", got, want)
	}
	if got, want := len(w.TaxLines()), taxBefore+1; got != want {
		t.Errorf("tax count = %d, want %d", got, want)
	}
	if got, want := len(w.PostTaxLines()), postBefore+1; got != want {
		t.Errorf("post-tax count = %d, want %d", got, want)
	}

	// Newly-added rows default to empty amount and the (None) category.
	if pre.AmountField().Value != "" {
		t.Errorf("new row amount should be empty, got %q", pre.AmountField().Value)
	}
	if pre.SelectField().SelectedIndex != 0 {
		t.Errorf("new row select should default to (None), got %d", pre.SelectField().SelectedIndex)
	}
	if pre.IsTransfer() {
		t.Error("new row should not be a transfer-line by default")
	}
}

// TestPaycheckWizard_AddLine_AppendsRowToSection asserts that each
// section's `[+ Add …]` helper appends one row to that section with
// an empty amount field and the section's appropriate default:
//
//   - Earnings        → categorized, defaulted to Income > Salary
//   - Pre-tax / Tax / Post-tax → categorized, defaulted to (None)
//   - Net Pay Destinations    → transfer-line, defaulted to a
//     non-deposit account
//
// The five helpers (`AddEarningsLine`, `AddPreTaxLine`, `AddTaxLine`,
// `AddPostTaxLine`, `AddAdditionalTransfer`) are also the dispatch
// targets used when the user activates a `[+ Add …]` button in the
// rendered wizard.
func TestPaycheckWizard_AddLine_AppendsRowToSection(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	earnBefore := len(w.EarningsLines())
	preBefore := len(w.PreTaxLines())
	taxBefore := len(w.TaxLines())
	postBefore := len(w.PostTaxLines())
	xferBefore := len(w.AdditionalTransfers())

	earn := w.AddEarningsLine()
	pre := w.AddPreTaxLine()
	tax := w.AddTaxLine()
	post := w.AddPostTaxLine()
	xfer := w.AddAdditionalTransfer()

	if earn == nil || pre == nil || tax == nil || post == nil || xfer == nil {
		t.Fatalf("Add* helper returned nil: earn=%v pre=%v tax=%v post=%v xfer=%v",
			earn, pre, tax, post, xfer)
	}

	// Each section's row count grows by exactly one.
	if got, want := len(w.EarningsLines()), earnBefore+1; got != want {
		t.Errorf("EarningsLines count = %d, want %d", got, want)
	}
	if got, want := len(w.PreTaxLines()), preBefore+1; got != want {
		t.Errorf("PreTaxLines count = %d, want %d", got, want)
	}
	if got, want := len(w.TaxLines()), taxBefore+1; got != want {
		t.Errorf("TaxLines count = %d, want %d", got, want)
	}
	if got, want := len(w.PostTaxLines()), postBefore+1; got != want {
		t.Errorf("PostTaxLines count = %d, want %d", got, want)
	}
	if got, want := len(w.AdditionalTransfers()), xferBefore+1; got != want {
		t.Errorf("AdditionalTransfers count = %d, want %d", got, want)
	}

	// Section assignment on each new line.
	if earn.Section != PaycheckEarnings {
		t.Errorf("earn.Section = %v, want PaycheckEarnings", earn.Section)
	}
	if pre.Section != PaycheckPreTax {
		t.Errorf("pre.Section = %v, want PaycheckPreTax", pre.Section)
	}
	if tax.Section != PaycheckTax {
		t.Errorf("tax.Section = %v, want PaycheckTax", tax.Section)
	}
	if post.Section != PaycheckPostTax {
		t.Errorf("post.Section = %v, want PaycheckPostTax", post.Section)
	}
	if xfer.Section != PaycheckNetPayDestination {
		t.Errorf("xfer.Section = %v, want PaycheckNetPayDestination", xfer.Section)
	}

	// All five rows start with an empty amount.
	for name, line := range map[string]*PaycheckLine{
		"earnings": earn, "pre-tax": pre, "tax": tax, "post-tax": post, "transfer": xfer,
	} {
		if line.AmountField().Value != "" {
			t.Errorf("%s row amount = %q, want empty", name, line.AmountField().Value)
		}
	}

	// Earnings line is categorized and pre-selected with Income > Salary.
	if earn.IsTransfer() {
		t.Error("AddEarningsLine should produce a categorized row, not a transfer-line")
	}
	if got, want := selectedLineOption(earn), "Income > Salary"; got != want {
		t.Errorf("AddEarningsLine default category = %q, want %q", got, want)
	}

	// Pre-tax / Tax / Post-tax start categorized at (None) so the user picks.
	for name, line := range map[string]*PaycheckLine{
		"pre-tax": pre, "tax": tax, "post-tax": post,
	} {
		if line.IsTransfer() {
			t.Errorf("%s row should default to categorized, not transfer-line", name)
		}
		if got := line.SelectField().SelectedIndex; got != 0 {
			t.Errorf("%s row default category index = %d, want 0 ((None))", name, got)
		}
	}

	// Net Pay Destinations row defaults to a transfer targeting some
	// account other than the deposit account (which is the schedule's
	// parent — a self-transfer would be rejected on save).
	if !xfer.IsTransfer() {
		t.Error("AddAdditionalTransfer should produce a transfer-line")
	}
	depositIdx := w.DepositAccount().SelectedIndex
	if xfer.AccountIndex() == depositIdx {
		t.Errorf("AddAdditionalTransfer should not default to the deposit account (idx %d)", depositIdx)
	}
}

// TestPaycheckWizard_RemoveRow_RemovesByPointer asserts RemoveRow
// drops the row from its section. Uses Pre-tax which starts empty in
// v2 so the assertion is independent of pre-populated row counts.
func TestPaycheckWizard_RemoveRow_RemovesByPointer(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	a := w.AddRow(PaycheckPreTax)
	b := w.AddRow(PaycheckPreTax)

	w.RemoveRow(a)
	if got := len(w.PreTaxLines()); got != 1 {
		t.Fatalf("pre-tax count after remove = %d, want 1", got)
	}
	if w.PreTaxLines()[0] != b {
		t.Error("remaining row should be b")
	}
}

// TestPaycheckWizard_SetAccountIndex_FlagsTransfer asserts that
// pointing a row's select at an account flips it to transfer-line
// mode.
func TestPaycheckWizard_SetAccountIndex_FlagsTransfer(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	line := w.AddRow(PaycheckPostTax)
	if line.IsTransfer() {
		t.Fatal("new row should start categorized")
	}

	// 401k is at account index 2 (Checking 0, Savings 1, 401k 2).
	line.SetAccountIndex(2)
	if !line.IsTransfer() {
		t.Error("after SetAccountIndex line should be a transfer-line")
	}
	if got := line.AccountIndex(); got != 2 {
		t.Errorf("AccountIndex = %d, want 2", got)
	}

	// Switching back to a category index flips it back.
	line.SetCategoryIndex(1)
	if line.IsTransfer() {
		t.Error("after SetCategoryIndex line should be categorized again")
	}
	if got := line.CategoryIndex(); got != 1 {
		t.Errorf("CategoryIndex = %d, want 1", got)
	}
}

// TestPaycheckWizard_BuildSplits_PreservesSignedAmounts asserts the
// signed sum equals the parent amount and each row is persisted
// with the user's typed sign.
func TestPaycheckWizard_BuildSplits_PreservesSignedAmounts(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	gross := w.AddRow(PaycheckPreTax)
	gross.SetCategoryIndex(indexOf(w.PreTaxLines()[0].SelectField().Options, "Income > Salary"))
	gross.AmountField().Value = "5000"

	fed := w.AddRow(PaycheckTax)
	fed.SetCategoryIndex(indexOf(fed.SelectField().Options, "Tax > Federal"))
	fed.AmountField().Value = "-800"

	ss := w.AddRow(PaycheckTax)
	ss.SetCategoryIndex(indexOf(ss.SelectField().Options, "Tax > Social Security"))
	ss.AmountField().Value = "-310"

	transfer := w.AddRow(PaycheckPostTax)
	// 401k is account index 2.
	transfer.SetAccountIndex(2)
	transfer.AmountField().Value = "-500"

	parent, splits, err := w.BuildSplits()
	if err != nil {
		t.Fatalf("BuildSplits: %v", err)
	}
	if got, want := len(splits), 4; got != want {
		t.Fatalf("split count = %d, want %d", got, want)
	}
	wantParent := types.MustNewMoney("3390")
	if !parent.Equal(wantParent) {
		t.Errorf("parent = %s, want %s", parent.String(), wantParent.String())
	}

	wantBySource := map[string]string{
		"salary":   "5000",
		"federal":  "-800",
		"ss":       "-310",
		"transfer": "-500",
	}
	got := map[string]string{}
	for _, sp := range splits {
		switch {
		case sp.CategoryID.Valid && sp.CategoryID.ID == fx.salaryID:
			got["salary"] = sp.Amount.String()
		case sp.CategoryID.Valid && sp.CategoryID.ID == fx.federalID:
			got["federal"] = sp.Amount.String()
		case sp.CategoryID.Valid && sp.CategoryID.ID == fx.ssID:
			got["ss"] = sp.Amount.String()
		case sp.TransferAccountID.Valid && sp.TransferAccountID.ID == fx.retire401kID:
			got["transfer"] = sp.Amount.String()
		}
	}
	for k, want := range wantBySource {
		if g := got[k]; g != want {
			t.Errorf("%s amount = %q, want %q", k, g, want)
		}
	}
}

// TestPaycheckWizard_BuildSplits_PersistsNotesAsMemo asserts the
// per-row Notes field is written to Split.Memo (and that empty notes
// stay NULL).
func TestPaycheckWizard_BuildSplits_PersistsNotesAsMemo(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	gross := w.AddRow(PaycheckPreTax)
	gross.SetCategoryIndex(indexOf(gross.SelectField().Options, "Income > Salary"))
	gross.AmountField().Value = "5000"
	gross.NotesField().Value = "Base pay"

	fed := w.AddRow(PaycheckTax)
	fed.SetCategoryIndex(indexOf(fed.SelectField().Options, "Tax > Federal"))
	fed.AmountField().Value = "-800"
	// Note: no notes set on this row — Split.Memo should stay NULL.

	_, splits, err := w.BuildSplits()
	if err != nil {
		t.Fatalf("BuildSplits: %v", err)
	}
	if len(splits) != 2 {
		t.Fatalf("split count = %d, want 2", len(splits))
	}

	var sawSalary, sawFederal bool
	for _, sp := range splits {
		if sp.CategoryID.Valid && sp.CategoryID.ID == fx.salaryID {
			sawSalary = true
			if !sp.Memo.Valid || sp.Memo.String != "Base pay" {
				t.Errorf("salary row Memo = %+v, want valid=true value=%q", sp.Memo, "Base pay")
			}
		}
		if sp.CategoryID.Valid && sp.CategoryID.ID == fx.federalID {
			sawFederal = true
			if sp.Memo.Valid {
				t.Errorf("federal row Memo unexpectedly set: %q", sp.Memo.String)
			}
		}
	}
	if !sawSalary || !sawFederal {
		t.Fatalf("missing expected split rows: salary=%v federal=%v", sawSalary, sawFederal)
	}
}

// TestPaycheckWizard_BuildSplits_SkipsEmptyRows asserts an empty
// amount row is silently skipped instead of producing a zero-amount
// split.
func TestPaycheckWizard_BuildSplits_SkipsEmptyRows(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	a := w.AddRow(PaycheckPreTax)
	a.SetCategoryIndex(indexOf(a.SelectField().Options, "Income > Salary"))
	a.AmountField().Value = "5000"

	_ = w.AddRow(PaycheckTax) // empty row, no category picked

	_, splits, err := w.BuildSplits()
	if err != nil {
		t.Fatalf("BuildSplits: %v", err)
	}
	if got := len(splits); got != 1 {
		t.Errorf("split count = %d, want 1 (the empty row should be skipped)", got)
	}
}

// TestPaycheckWizard_BuildSplits_TagsEachLine asserts that BuildSplits
// stamps the matching `paycheck_section` enum string on every returned
// split, so the saved schedule can be round-tripped back into the wizard
// by reading the tag (PW2-008). Section assignment comes from the row's
// Section, not its category — a tax-section row with an unusual category
// still tags as `tax`.
func TestPaycheckWizard_BuildSplits_TagsEachLine(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	// Wipe the v2 pre-population so the test owns every row.
	for s := PaycheckEarnings; s <= PaycheckNetPayDestination; s++ {
		w.sections[s] = nil
	}

	earn := w.AddRow(PaycheckEarnings)
	earn.SetCategoryIndex(indexOf(earn.SelectField().Options, "Income > Salary"))
	earn.AmountField().Value = "5000"

	pre := w.AddRow(PaycheckPreTax)
	// 401k is account index 2 in the fixture.
	pre.SetAccountIndex(2)
	pre.AmountField().Value = "-300"

	tax := w.AddRow(PaycheckTax)
	tax.SetCategoryIndex(indexOf(tax.SelectField().Options, "Tax > Federal"))
	tax.AmountField().Value = "-800"

	post := w.AddRow(PaycheckPostTax)
	post.SetCategoryIndex(indexOf(post.SelectField().Options, "Insurance > Health"))
	post.AmountField().Value = "-150"

	xfer := w.AddRow(PaycheckNetPayDestination)
	// Savings is account index 1 (Checking is the deposit account at 0).
	xfer.SetAccountIndex(1)
	xfer.AmountField().Value = "-500"

	_, splits, err := w.BuildSplits()
	if err != nil {
		t.Fatalf("BuildSplits: %v", err)
	}
	if got, want := len(splits), 5; got != want {
		t.Fatalf("split count = %d, want %d", got, want)
	}

	// Splits are returned in section order — assert each position
	// carries the expected tag string.
	wantTags := []string{"earnings", "pre_tax", "tax", "post_tax", "net_pay_destination"}
	for i, want := range wantTags {
		sp := splits[i]
		if !sp.PaycheckSection.Valid {
			t.Errorf("splits[%d] PaycheckSection should be Valid (tag %q)", i, want)
			continue
		}
		if sp.PaycheckSection.String != want {
			t.Errorf("splits[%d] PaycheckSection = %q, want %q", i, sp.PaycheckSection.String, want)
		}
	}
}

// TestPaycheckWizard_BuildSplits_ElidesZeroRows asserts that a row with
// an empty amount and a row whose amount parses to zero are both
// silently elided, even when their categories are valid. The wizard
// opens with three pre-populated tax rows; filling in only two of them
// should produce two splits, not three.
func TestPaycheckWizard_BuildSplits_ElidesZeroRows(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	// Earnings is pre-populated with one Income > Salary row — fill it
	// in so BuildSplits doesn't fail on "add at least one row".
	earnings := w.EarningsLines()
	if len(earnings) != 1 {
		t.Fatalf("Earnings pre-population count = %d, want 1", len(earnings))
	}
	earnings[0].AmountField().Value = "5000"

	tax := w.TaxLines()
	if len(tax) != 3 {
		t.Fatalf("Tax pre-population count = %d, want 3", len(tax))
	}
	// Federal: real amount.
	tax[0].AmountField().Value = "-800"
	// Social Security: empty (elided).
	// Medicare: explicit zero (also elided).
	tax[2].AmountField().Value = "0"

	_, splits, err := w.BuildSplits()
	if err != nil {
		t.Fatalf("BuildSplits: %v", err)
	}
	if got, want := len(splits), 2; got != want {
		t.Fatalf("split count = %d, want %d (Earnings + Federal only)", got, want)
	}

	// The two emitted splits are Earnings (Salary, +5000) and Tax
	// (Federal, -800). No Social Security or Medicare rows leak through.
	for _, sp := range splits {
		if !sp.PaycheckSection.Valid {
			t.Errorf("split tag missing on amount=%s", sp.Amount.String())
		}
	}
	if sp := splits[0]; sp.Amount.String() != "5000" || sp.PaycheckSection.String != "earnings" {
		t.Errorf("splits[0] = (%s, %q), want (5000, earnings)", sp.Amount.String(), sp.PaycheckSection.String)
	}
	if sp := splits[1]; sp.Amount.String() != "-800" || sp.PaycheckSection.String != "tax" {
		t.Errorf("splits[1] = (%s, %q), want (-800, tax)", sp.Amount.String(), sp.PaycheckSection.String)
	}
}

// TestPaycheckWizard_BuildSplits_BalanceInvariant asserts the
// parent_amount returned by BuildSplits equals the signed sum of all
// returned splits, across mixed sections.
func TestPaycheckWizard_BuildSplits_BalanceInvariant(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	for s := PaycheckEarnings; s <= PaycheckNetPayDestination; s++ {
		w.sections[s] = nil
	}

	earn := w.AddRow(PaycheckEarnings)
	earn.SetCategoryIndex(indexOf(earn.SelectField().Options, "Income > Salary"))
	earn.AmountField().Value = "5234.17"

	pre := w.AddRow(PaycheckPreTax)
	pre.SetAccountIndex(2)
	pre.AmountField().Value = "-275.50"

	fed := w.AddRow(PaycheckTax)
	fed.SetCategoryIndex(indexOf(fed.SelectField().Options, "Tax > Federal"))
	fed.AmountField().Value = "-812.04"

	med := w.AddRow(PaycheckTax)
	med.SetCategoryIndex(indexOf(med.SelectField().Options, "Tax > Medicare"))
	med.AmountField().Value = "-75.90"

	post := w.AddRow(PaycheckPostTax)
	post.SetCategoryIndex(indexOf(post.SelectField().Options, "Insurance > Health"))
	post.AmountField().Value = "-152.33"

	xfer := w.AddRow(PaycheckNetPayDestination)
	xfer.SetAccountIndex(1)
	xfer.AmountField().Value = "-400.00"

	parent, splits, err := w.BuildSplits()
	if err != nil {
		t.Fatalf("BuildSplits: %v", err)
	}

	sum := types.ZeroMoney
	for _, sp := range splits {
		sum = sum.Add(sp.Amount)
	}
	if !parent.Equal(sum) {
		t.Errorf("parent (%s) != signed sum of splits (%s)", parent.String(), sum.String())
	}
}

// TestPaycheckWizard_Save_CreatesMultiLineSchedule drives the
// wizard end-to-end against a real DB and confirms the saved
// schedule mirrors the user's input.
func TestPaycheckWizard_Save_CreatesMultiLineSchedule(t *testing.T) {
	database := dbtest.New(t)

	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	payeeRepo := payee.NewRepository(database)
	schedRepo := scheduled.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitTxnRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)

	txnSvc := transaction.NewService(txnRepo, splitTxnRepo, transferRepo, payeeRepo, accountRepo, database)
	schedSvc := scheduled.NewService(schedRepo, txnRepo, txnSvc, database, accountRepo)
	accountSvc := account.NewService(accountRepo, database)
	payeeSvc := payee.NewService(payeeRepo, database)
	categorySvc := category.NewService(categoryRepo, database)

	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(checking); err != nil {
		t.Fatalf("create checking: %v", err)
	}
	retire := account.NewAccount("401k", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(retire); err != nil {
		t.Fatalf("create 401k: %v", err)
	}
	if err := categorySvc.EnsurePaycheckCategories(); err != nil {
		t.Fatalf("EnsurePaycheckCategories: %v", err)
	}

	accounts, err := accountSvc.List(true)
	if err != nil {
		t.Fatalf("List accounts: %v", err)
	}
	cats, err := categorySvc.List()
	if err != nil {
		t.Fatalf("List categories: %v", err)
	}
	categoryOptions, categoryIDs := buildCategoryOptions(cats)

	app := &App{
		currentView:     ViewDashboard,
		keys:            defaultKeyMap(),
		menubar:         widget.NewMenuBar(),
		statusbar:       widget.NewStatusBar(),
		sidebar:         NewSidebar(),
		accountSvc:      accountSvc,
		payeeSvc:        payeeSvc,
		categorySvc:     categorySvc,
		scheduledTxnSvc: schedSvc,
		transactionSvc:  txnSvc,
		undoManager:     undo.NewManager(),
	}
	app.paycheckWizard = NewPaycheckWizard(categoryOptions, categoryIDs, accounts)
	w := app.paycheckWizard

	// Set header fields.
	w.Employer().Value = "Acme Corp"
	w.NextPayday().Value = "05/15/2026"
	// Pick semi-monthly (15th & last day) — option index 3.
	w.Frequency().SelectedIndex = 3
	// Resolve deposit account by name to be robust to List ordering.
	checkingAcctIdx := indexOf(w.DepositAccount().Options, "Checking")
	if checkingAcctIdx < 0 {
		t.Fatalf("Checking missing from account picker: %v", w.DepositAccount().Options)
	}
	w.DepositAccount().SelectedIndex = checkingAcctIdx

	salaryIdx := indexOf(categoryOptions, "Income > Salary")
	federalIdx := indexOf(categoryOptions, "Tax > Federal")
	healthIdx := indexOf(categoryOptions, "Insurance > Health")
	retireAcctIdx := indexOf(w.DepositAccount().Options, "401k")
	if salaryIdx <= 0 || federalIdx <= 0 || healthIdx <= 0 || retireAcctIdx < 0 {
		t.Fatalf("category/account indices unresolved: salary=%d federal=%d health=%d 401k=%d",
			salaryIdx, federalIdx, healthIdx, retireAcctIdx)
	}

	gross := w.AddRow(PaycheckPreTax)
	gross.SetCategoryIndex(salaryIdx)
	gross.AmountField().Value = "5000"

	fed := w.AddRow(PaycheckTax)
	fed.SetCategoryIndex(federalIdx)
	fed.AmountField().Value = "-800"

	health := w.AddRow(PaycheckPostTax)
	health.SetCategoryIndex(healthIdx)
	health.AmountField().Value = "-150"

	transfer := w.AddRow(PaycheckPostTax)
	transfer.SetAccountIndex(retireAcctIdx)
	transfer.AmountField().Value = "-500"

	model, cmd := app.submitPaycheckWizard()
	app2 := model.(*App)
	if app2.paycheckWizard != nil {
		t.Errorf("wizard should be cleared after a successful save; errorMsg=%q", app2.paycheckWizard.errorMsg)
	}
	if cmd == nil {
		t.Fatal("submitPaycheckWizard should return a non-nil command")
	}
	if msg := cmd(); msg != nil {
		if e, ok := msg.(errMsg); ok {
			t.Fatalf("save command returned error: %v", e.err)
		}
		if _, ok := msg.(scheduledDialogSavedMsg); !ok {
			t.Fatalf("save command returned unexpected message type: %T", msg)
		}
	}

	schedules, err := schedSvc.List()
	if err != nil {
		t.Fatalf("List schedules: %v", err)
	}
	if len(schedules) != 1 {
		t.Fatalf("got %d schedules, want 1", len(schedules))
	}
	sched := schedules[0]

	if sched.AccountID != checking.ID {
		t.Errorf("AccountID = %v, want Checking %v", sched.AccountID, checking.ID)
	}
	if sched.Frequency != scheduled.FrequencySemiMonthly {
		t.Errorf("Frequency = %s, want semimonthly", sched.Frequency)
	}
	if !sched.DayOfMonth.Valid || sched.DayOfMonth.Int64 != 15 {
		t.Errorf("DayOfMonth = %+v, want 15", sched.DayOfMonth)
	}
	if !sched.SecondaryDayOfMonth.Valid || sched.SecondaryDayOfMonth.Int64 != -1 {
		t.Errorf("SecondaryDayOfMonth = %+v, want -1", sched.SecondaryDayOfMonth)
	}
	if sched.HasCategory() {
		t.Error("multi-line schedule should clear the scalar category")
	}
	wantNet := types.MustNewMoney("3550") // 5000 - 800 - 150 - 500
	if !sched.Amount.Money.Equal(wantNet) {
		t.Errorf("parent amount = %s, want %s", sched.Amount.Money.String(), wantNet.String())
	}
	if len(sched.Splits) != 4 {
		t.Fatalf("got %d splits, want 4", len(sched.Splits))
	}
	if !sched.Splits.Total().Equal(wantNet) {
		t.Errorf("signed sum of children = %s, want %s",
			sched.Splits.Total().String(), wantNet.String())
	}

	// Verify each saved split mirrors the wizard input.
	var sawSalary, sawFederal, sawHealth, sawTransfer bool
	for _, sp := range sched.Splits {
		switch {
		case sp.CategoryID.Valid && categoryByID(cats, sp.CategoryID.ID) == "Salary":
			sawSalary = true
			if got, want := sp.Amount, types.MustNewMoney("5000"); !got.Equal(want) {
				t.Errorf("salary amount = %s, want %s", got, want)
			}
		case sp.CategoryID.Valid && categoryByID(cats, sp.CategoryID.ID) == "Federal":
			sawFederal = true
			if got, want := sp.Amount, types.MustNewMoney("-800"); !got.Equal(want) {
				t.Errorf("federal amount = %s, want %s", got, want)
			}
		case sp.CategoryID.Valid && categoryByID(cats, sp.CategoryID.ID) == "Health":
			sawHealth = true
			if got, want := sp.Amount, types.MustNewMoney("-150"); !got.Equal(want) {
				t.Errorf("health amount = %s, want %s", got, want)
			}
		case sp.TransferAccountID.Valid && sp.TransferAccountID.ID == retire.ID:
			sawTransfer = true
			if got, want := sp.Amount, types.MustNewMoney("-500"); !got.Equal(want) {
				t.Errorf("transfer amount = %s, want %s", got, want)
			}
		}
	}
	if !sawSalary || !sawFederal || !sawHealth || !sawTransfer {
		t.Errorf("missing splits: salary=%v fed=%v health=%v transfer=%v",
			sawSalary, sawFederal, sawHealth, sawTransfer)
	}
}

// categoryByID returns the leaf category name (subcategory) for a given ID,
// or empty string when not found. Helper for paycheck-save tests.
func categoryByID(cats []*category.Category, id types.ID) string {
	for _, c := range cats {
		if c.ID == id {
			return c.Name
		}
	}
	return ""
}

// taggedSplit builds a categorized scheduled.Split tagged with the
// given paycheck_section enum string. Helper for the v2
// looksLikePaycheck tests.
func taggedSplit(categoryID types.ID, amount, tag string) *scheduled.Split {
	return &scheduled.Split{
		BaseModel:       types.NewBaseModel(),
		Amount:          types.MustNewMoney(amount),
		CategoryID:      types.NullableID{ID: categoryID, Valid: true},
		PaycheckSection: types.NullableString{String: tag, Valid: true},
	}
}

// TestLooksLikePaycheck_V2_RequiresTagsAndEarnings asserts the v2
// heuristic: a schedule looks like a paycheck iff it is multi-line,
// every split carries a non-NULL paycheck_section tag, and at least
// one split is tagged "earnings".
func TestLooksLikePaycheck_V2_RequiresTagsAndEarnings(t *testing.T) {
	fx := newPaycheckWizardFixture()
	st := scheduled.NewTransaction(fx.checkingID, scheduled.FrequencyFortnightly, types.Today())
	st.SetAmount(types.MustNewMoney("3890"))
	st.ClearCategory()
	st.Splits = scheduled.SplitCollection{
		taggedSplit(fx.salaryID, "5000", "earnings"),
		taggedSplit(fx.federalID, "-800", "tax"),
		taggedSplit(fx.ssID, "-310", "tax"),
	}
	if !looksLikePaycheck(st) {
		t.Errorf("fully-tagged paycheck with earnings line should look like a paycheck")
	}

	// Single-line schedule: not multi-line, so not a paycheck even with a
	// valid earnings tag.
	single := scheduled.NewTransaction(fx.checkingID, scheduled.FrequencyMonthly, types.Today())
	single.SetAmount(types.MustNewMoney("5000"))
	if looksLikePaycheck(single) {
		t.Errorf("single-line schedule should not look like a paycheck")
	}
}

// TestLooksLikePaycheck_V2_NullTagHidesAffordance asserts that a
// schedule whose splits are otherwise paycheck-shaped but include at
// least one untagged (NULL paycheck_section) line returns false —
// the affordance only surfaces when every line was produced by the
// wizard. The NULL state is the "treat as generic multi-line split"
// signal per migration 020.
func TestLooksLikePaycheck_V2_NullTagHidesAffordance(t *testing.T) {
	fx := newPaycheckWizardFixture()
	st := scheduled.NewTransaction(fx.checkingID, scheduled.FrequencyFortnightly, types.Today())
	st.SetAmount(types.MustNewMoney("3890"))
	st.ClearCategory()
	// First split has NULL paycheck_section (e.g. added via the generic
	// multi-line split dialog); the other two carry tags.
	st.Splits = scheduled.SplitCollection{
		{
			BaseModel:  types.NewBaseModel(),
			Amount:     types.MustNewMoney("5000"),
			CategoryID: types.NullableID{ID: fx.salaryID, Valid: true},
		},
		taggedSplit(fx.federalID, "-800", "tax"),
		taggedSplit(fx.ssID, "-310", "tax"),
	}
	if looksLikePaycheck(st) {
		t.Errorf("schedule with one NULL-tagged split should not look like a paycheck")
	}
}

// TestLooksLikePaycheck_V2_NoEarningsTag_Returns_False asserts that a
// fully-tagged multi-line schedule with no `earnings`-tagged line
// returns false: a paycheck must have at least one earnings row to be
// reopenable in the wizard.
func TestLooksLikePaycheck_V2_NoEarningsTag_Returns_False(t *testing.T) {
	fx := newPaycheckWizardFixture()
	st := scheduled.NewTransaction(fx.checkingID, scheduled.FrequencyFortnightly, types.Today())
	st.SetAmount(types.MustNewMoney("-1110"))
	st.ClearCategory()
	// Tagged splits but no `earnings` line — e.g. a deductions-only
	// schedule that happened to be tagged for some other reason.
	st.Splits = scheduled.SplitCollection{
		taggedSplit(fx.federalID, "-800", "tax"),
		taggedSplit(fx.ssID, "-310", "tax"),
	}
	if looksLikePaycheck(st) {
		t.Errorf("fully-tagged schedule without an earnings line should not look like a paycheck")
	}
}

// TestNewPaycheckWizardFromSchedule_V2_GroupsByTag asserts the v2
// pre-fill walks the schedule's splits and routes each row into the
// wizard section named by its paycheck_section tag — independent of
// storage order. The fixture deliberately stores the splits in a
// shuffled (non-section) order so the test proves routing comes from
// the tag, not the position.
func TestNewPaycheckWizardFromSchedule_V2_GroupsByTag(t *testing.T) {
	fx := newPaycheckWizardFixture()

	st := scheduled.NewTransaction(fx.checkingID, scheduled.FrequencyFortnightly, types.MustParseDate("2026-03-15"))
	st.SetAmount(types.MustNewMoney("3090"))
	st.ClearCategory()
	// Stored order is deliberately mixed: net_pay_destination, post_tax,
	// post_tax, pre_tax, tax, tax, earnings.
	st.Splits = scheduled.SplitCollection{
		{
			BaseModel:         types.NewBaseModel(),
			Amount:            types.MustNewMoney("-300"),
			TransferAccountID: types.NullableID{ID: fx.savingsID, Valid: true},
			PaycheckSection:   types.NullableString{String: "net_pay_destination", Valid: true},
		},
		{
			BaseModel:       types.NewBaseModel(),
			Amount:          types.MustNewMoney("-150"),
			CategoryID:      types.NullableID{ID: fx.healthID, Valid: true},
			PaycheckSection: types.NullableString{String: "post_tax", Valid: true},
		},
		{
			BaseModel:       types.NewBaseModel(),
			Amount:          types.MustNewMoney("-50"),
			CategoryID:      types.NullableID{ID: fx.healthID, Valid: true},
			PaycheckSection: types.NullableString{String: "post_tax", Valid: true},
		},
		{
			BaseModel:         types.NewBaseModel(),
			Amount:            types.MustNewMoney("-500"),
			TransferAccountID: types.NullableID{ID: fx.retire401kID, Valid: true},
			PaycheckSection:   types.NullableString{String: "pre_tax", Valid: true},
		},
		{
			BaseModel:       types.NewBaseModel(),
			Amount:          types.MustNewMoney("-800"),
			CategoryID:      types.NullableID{ID: fx.federalID, Valid: true},
			PaycheckSection: types.NullableString{String: "tax", Valid: true},
		},
		{
			BaseModel:       types.NewBaseModel(),
			Amount:          types.MustNewMoney("-310"),
			CategoryID:      types.NullableID{ID: fx.ssID, Valid: true},
			PaycheckSection: types.NullableString{String: "tax", Valid: true},
		},
		{
			BaseModel:       types.NewBaseModel(),
			Amount:          types.MustNewMoney("5000"),
			CategoryID:      types.NullableID{ID: fx.salaryID, Valid: true},
			PaycheckSection: types.NullableString{String: "earnings", Valid: true},
		},
	}

	w := NewPaycheckWizardFromSchedule(st, fx.accounts, nil, fx.categoryOptions, fx.categoryIDs)
	if w == nil {
		t.Fatal("NewPaycheckWizardFromSchedule returned nil")
	}

	if got := len(w.EarningsLines()); got != 1 {
		t.Errorf("EarningsLines count = %d, want 1", got)
	}
	if got := len(w.PreTaxLines()); got != 1 {
		t.Errorf("PreTaxLines count = %d, want 1", got)
	}
	if got := len(w.TaxLines()); got != 2 {
		t.Errorf("TaxLines count = %d, want 2", got)
	}
	if got := len(w.PostTaxLines()); got != 2 {
		t.Errorf("PostTaxLines count = %d, want 2", got)
	}
	if got := len(w.AdditionalTransfers()); got != 1 {
		t.Errorf("AdditionalTransfers count = %d, want 1", got)
	}

	// Earnings row: categorized Salary at the raw stored signed amount.
	earn := w.EarningsLines()
	if len(earn) > 0 {
		if earn[0].IsTransfer() {
			t.Error("earnings row should be categorized, not a transfer-line")
		}
		if got, want := selectedLineOption(earn[0]), "Income > Salary"; got != want {
			t.Errorf("earnings[0] category = %q, want %q", got, want)
		}
		if got, want := earn[0].AmountField().Value, "5000"; got != want {
			t.Errorf("earnings[0] amount = %q, want %q", got, want)
		}
	}

	// Pre-tax row: the 401k transfer-line tagged pre_tax.
	pre := w.PreTaxLines()
	if len(pre) > 0 {
		if !pre[0].IsTransfer() {
			t.Error("pre-tax row should be a transfer-line (401k)")
		}
		if got, want := pre[0].AmountField().Value, "-500"; got != want {
			t.Errorf("pre-tax[0] amount = %q, want %q", got, want)
		}
	}

	// Tax rows preserve storage order — Federal first (storage index 4),
	// Social Security second (storage index 5).
	tax := w.TaxLines()
	if len(tax) == 2 {
		if got, want := selectedLineOption(tax[0]), "Tax > Federal"; got != want {
			t.Errorf("tax[0] category = %q, want %q", got, want)
		}
		if got, want := selectedLineOption(tax[1]), "Tax > Social Security"; got != want {
			t.Errorf("tax[1] category = %q, want %q", got, want)
		}
	}

	// Net Pay Destinations row: the Savings transfer tagged net_pay_destination.
	xfer := w.AdditionalTransfers()
	if len(xfer) > 0 {
		if !xfer[0].IsTransfer() {
			t.Error("net pay destination row should be a transfer-line (Savings)")
		}
		if got, want := xfer[0].AmountField().Value, "-300"; got != want {
			t.Errorf("net_pay_destination[0] amount = %q, want %q", got, want)
		}
	}
}

// TestNewPaycheckWizardFromSchedule_V2_MultipleEarningsLines asserts
// that a paycheck with two earnings-tagged lines (e.g. Salary +5000
// plus Imputed LTD +44.03) opens with both rows in the Earnings
// section, in storage order.
func TestNewPaycheckWizardFromSchedule_V2_MultipleEarningsLines(t *testing.T) {
	fx := newPaycheckWizardFixture()
	imputedID := types.NewID()
	categoryOptions := append([]string{}, fx.categoryOptions...)
	categoryOptions = append(categoryOptions, "Income > Imputed LTD")
	categoryIDs := append([]types.ID{}, fx.categoryIDs...)
	categoryIDs = append(categoryIDs, imputedID)

	st := scheduled.NewTransaction(fx.checkingID, scheduled.FrequencyFortnightly, types.MustParseDate("2026-03-15"))
	st.SetAmount(types.MustNewMoney("5044.03"))
	st.ClearCategory()
	st.Splits = scheduled.SplitCollection{
		{
			BaseModel:       types.NewBaseModel(),
			Amount:          types.MustNewMoney("5000"),
			CategoryID:      types.NullableID{ID: fx.salaryID, Valid: true},
			PaycheckSection: types.NullableString{String: "earnings", Valid: true},
		},
		{
			BaseModel:       types.NewBaseModel(),
			Amount:          types.MustNewMoney("44.03"),
			CategoryID:      types.NullableID{ID: imputedID, Valid: true},
			PaycheckSection: types.NullableString{String: "earnings", Valid: true},
		},
	}

	w := NewPaycheckWizardFromSchedule(st, fx.accounts, nil, categoryOptions, categoryIDs)
	if w == nil {
		t.Fatal("NewPaycheckWizardFromSchedule returned nil")
	}

	earn := w.EarningsLines()
	if got := len(earn); got != 2 {
		t.Fatalf("EarningsLines count = %d, want 2", got)
	}
	// Storage order: salary first, imputed second.
	if got, want := selectedLineOption(earn[0]), "Income > Salary"; got != want {
		t.Errorf("earnings[0] category = %q, want %q", got, want)
	}
	if got, want := earn[0].AmountField().Value, "5000"; got != want {
		t.Errorf("earnings[0] amount = %q, want %q", got, want)
	}
	if got, want := selectedLineOption(earn[1]), "Income > Imputed LTD"; got != want {
		t.Errorf("earnings[1] category = %q, want %q", got, want)
	}
	if got, want := earn[1].AmountField().Value, "44.03"; got != want {
		t.Errorf("earnings[1] amount = %q, want %q", got, want)
	}

	// The other sections stay empty (no defaults leak through when the
	// schedule's splits are the only content).
	if got := len(w.PreTaxLines()); got != 0 {
		t.Errorf("PreTaxLines count = %d, want 0", got)
	}
	if got := len(w.TaxLines()); got != 0 {
		t.Errorf("TaxLines count = %d, want 0", got)
	}
	if got := len(w.PostTaxLines()); got != 0 {
		t.Errorf("PostTaxLines count = %d, want 0", got)
	}
	if got := len(w.AdditionalTransfers()); got != 0 {
		t.Errorf("AdditionalTransfers count = %d, want 0", got)
	}
}

// CC-005 — Inline category creation from the paycheck wizard.

// TestPaycheckWizard_CombinedOptionsIncludesAddNewSentinel pins the layout
// invariant the AddNew flow depends on: combinedOptions ends with the
// [+ Add new category…] sentinel, sitting one past the last transfer entry.
func TestPaycheckWizard_CombinedOptionsIncludesAddNewSentinel(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)
	if w == nil {
		t.Fatal("NewPaycheckWizard returned nil")
	}

	if got := w.combinedOptions[len(w.combinedOptions)-1]; got != paycheckAddNewSentinelLabel {
		t.Errorf("last combinedOptions entry = %q, want %q", got, paycheckAddNewSentinelLabel)
	}

	wantLen := len(w.categoryOptions) + len(w.accountOptions) + 1
	if len(w.combinedOptions) != wantLen {
		t.Errorf("len(combinedOptions) = %d, want %d (cats + accts + 1 AddNew)",
			len(w.combinedOptions), wantLen)
	}

	// Each pre-populated line (Earnings + 3 Taxes) inherits combinedOptions.
	for _, line := range w.EarningsLines() {
		if got := line.SelectField().Options[len(line.SelectField().Options)-1]; got != paycheckAddNewSentinelLabel {
			t.Errorf("earnings line Options last entry = %q, want sentinel", got)
		}
	}
	for _, line := range w.TaxLines() {
		if got := line.SelectField().Options[len(line.SelectField().Options)-1]; got != paycheckAddNewSentinelLabel {
			t.Errorf("tax line Options last entry = %q, want sentinel", got)
		}
	}
}

// TestPaycheckWizard_IsAddNew_TrueForLastIndex pins the accessor's contract:
// IsAddNew reports true when SelectedIndex points at the last entry of Options
// (the AddNew sentinel), and IsTransfer reports false there (the AddNew row
// sits past the transfer block, but it is not a transfer).
func TestPaycheckWizard_IsAddNew_TrueForLastIndex(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	line := w.AddPreTaxLine()
	if line == nil {
		t.Fatal("AddPreTaxLine returned nil")
	}

	// Default: (None) at index 0 — not AddNew, not Transfer.
	if line.IsAddNew() {
		t.Error("IsAddNew should be false at index 0 (None)")
	}
	if line.IsTransfer() {
		t.Error("IsTransfer should be false at index 0 (None)")
	}

	// Park on a transfer entry — IsTransfer true, IsAddNew false.
	transferIdx := len(w.categoryOptions) // first transfer entry
	line.SelectField().SelectedIndex = transferIdx
	if !line.IsTransfer() {
		t.Error("IsTransfer should be true on a transfer index")
	}
	if line.IsAddNew() {
		t.Error("IsAddNew should be false on a transfer index")
	}

	// Park on the AddNew sentinel — IsAddNew true, IsTransfer false.
	line.SelectField().SelectedIndex = len(line.SelectField().Options) - 1
	if !line.IsAddNew() {
		t.Error("IsAddNew should be true at len(Options)-1")
	}
	if line.IsTransfer() {
		t.Error("IsTransfer should be false on the AddNew sentinel")
	}
}

// TestPaycheckWizard_EnterOnAddNew_ReturnsDialogActionAddNew exercises
// HandleKey end-to-end: with focus on a line's select field parked on the
// AddNew sentinel, Enter returns dialog.DialogActionAddNew so the parent App can
// divert into the create-category sub-dialog.
func TestPaycheckWizard_EnterOnAddNew_ReturnsDialogActionAddNew(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	line := w.AddPreTaxLine()
	if line == nil {
		t.Fatal("AddPreTaxLine returned nil")
	}
	line.SelectField().SelectedIndex = len(line.SelectField().Options) - 1

	// Walk focusables to land focus on this line's select field.
	focused := false
	for i, target := range w.collectFocusables() {
		if target.kind == wizardFocusField && target.field == line.SelectField() {
			w.focusIndex = i
			focused = true
			break
		}
	}
	if !focused {
		t.Fatal("could not find focus index for the AddNew-parked line")
	}

	action := w.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != dialog.DialogActionAddNew {
		t.Errorf("HandleKey(Enter) = %v, want dialog.DialogActionAddNew", action)
	}
}

// TestPaycheckWizard_EnterOnRealCategory_AdvancesFocus pins the regression
// that ordinary category selections (not on AddNew) still advance focus on
// Enter — the AddNew detection must not change non-sentinel behavior.
func TestPaycheckWizard_EnterOnRealCategory_AdvancesFocus(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)

	line := w.AddPreTaxLine()
	if line == nil {
		t.Fatal("AddPreTaxLine returned nil")
	}
	line.SelectField().SelectedIndex = 1 // a real category

	startIdx := -1
	for i, target := range w.collectFocusables() {
		if target.kind == wizardFocusField && target.field == line.SelectField() {
			w.focusIndex = i
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		t.Fatal("could not find focus index for the line's select field")
	}

	action := w.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != dialog.DialogActionNone {
		t.Errorf("HandleKey(Enter) = %v, want dialog.DialogActionNone", action)
	}
	if w.focusIndex != startIdx+1 {
		t.Errorf("focusIndex = %d, want %d (advanced one)", w.focusIndex, startIdx+1)
	}
}

// newAppForPaycheckAddNew builds an *App whose paycheck wizard has a pre-tax
// row parked on the AddNew sentinel and is otherwise pre-populated, so we
// can assert that field values survive the open-cancel and open-submit
// round-trips. categorySvc may be nil for tests that don't drive submit.
func newAppForPaycheckAddNew(t *testing.T, categorySvc *category.Service, cats []*category.Category) (*App, *PaycheckLine) {
	t.Helper()
	fx := newPaycheckWizardFixture()
	categoryOptions, categoryIDs := fx.categoryOptions, fx.categoryIDs
	if len(cats) > 0 {
		categoryOptions, categoryIDs = buildCategoryOptions(cats)
	}

	w := NewPaycheckWizard(categoryOptions, categoryIDs, fx.accounts)
	// Seed a few scalar values we expect to see preserved across the divert.
	w.Employer().Value = "Acme"
	w.Memo().Value = "Biweekly"

	// Add one transfer line in Net Pay Destinations so we can pin that its
	// SelectedIndex shifts by the category-count delta after the rebuild.
	transferLine := w.AddRow(PaycheckNetPayDestination)
	transferLine.SetAccountIndex(0)

	// Park a pre-tax line on the AddNew sentinel.
	line := w.AddPreTaxLine()
	line.SelectField().SelectedIndex = len(line.SelectField().Options) - 1
	if !line.IsAddNew() {
		t.Fatalf("test setup: line should be parked on AddNew sentinel")
	}

	// Move focus onto that line's select field.
	for i, target := range w.collectFocusables() {
		if target.kind == wizardFocusField && target.field == line.SelectField() {
			w.focusIndex = i
			break
		}
	}

	app := &App{
		keys:           defaultKeyMap(),
		menubar:        widget.NewMenuBar(),
		statusbar:      widget.NewStatusBar(),
		sidebar:        NewSidebar(),
		categorySvc:    categorySvc,
		paycheckWizard: w,
	}
	return app, line
}

func TestApp_PaycheckWizard_AddNew_OpensCreateCategoryDialog(t *testing.T) {
	app, line := newAppForPaycheckAddNew(t, nil, nil)

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handlePaycheckWizardKey(enter)
	updated := model.(*App)

	if updated.createCatDialog == nil || !updated.createCatDialog.IsVisible() {
		t.Fatal("createCatDialog should be visible after Enter on AddNew sentinel")
	}
	if updated.createCatSource != createCatSourcePaycheckWizard {
		t.Errorf("createCatSource = %d, want createCatSourcePaycheckWizard (%d)",
			updated.createCatSource, createCatSourcePaycheckWizard)
	}
	if updated.createCatPaycheckLine != line {
		t.Error("createCatPaycheckLine should reference the originating line")
	}
	if updated.paycheckWizard == nil {
		t.Fatal("paycheckWizard should be kept (hidden) so its state survives the divert")
	}
	if updated.paycheckWizard.IsVisible() {
		t.Error("paycheckWizard should be hidden while createCatDialog is shown")
	}
	if updated.createCatDialog.Title() != "New Category" {
		t.Errorf("createCatDialog title = %q, want %q",
			updated.createCatDialog.Title(), "New Category")
	}
}

func TestApp_PaycheckWizard_AddNew_CancelRestoresState(t *testing.T) {
	app, _ := newAppForPaycheckAddNew(t, nil, nil)

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handlePaycheckWizardKey(enter)
	app = model.(*App)
	if app.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}

	esc := tea.KeyPressMsg{Code: tea.KeyEsc}
	model, _ = app.handleCreateCatDialogKey(esc)
	app = model.(*App)

	if app.createCatDialog != nil {
		t.Error("createCatDialog should be cleared after cancel")
	}
	if app.createCatSource != createCatSourceNone {
		t.Errorf("createCatSource = %d, want None after cancel", app.createCatSource)
	}
	if app.createCatPaycheckLine != nil {
		t.Error("createCatPaycheckLine should be cleared after cancel")
	}
	if app.paycheckWizard == nil || !app.paycheckWizard.IsVisible() {
		t.Fatal("paycheckWizard should be restored to visible after cancel")
	}
	if got := app.paycheckWizard.Employer().Value; got != "Acme" {
		t.Errorf("Employer preserved? got %q, want %q", got, "Acme")
	}
	if got := app.paycheckWizard.Memo().Value; got != "Biweekly" {
		t.Errorf("Memo preserved? got %q, want %q", got, "Biweekly")
	}
}

// TestApp_PaycheckWizard_AddNew_AppliesToOriginatingLine drives the
// open → submit flow end-to-end against a real category service: pre-tax
// row activates AddNew, user submits a new category, and the line points
// at the freshly-created category afterwards. The companion test below
// pins the transfer-line index shift.
func TestApp_PaycheckWizard_AddNew_AppliesToOriginatingLine(t *testing.T) {
	database := dbtest.New(t)

	repo := category.NewRepository(database)
	svc := category.NewService(repo, database)
	if err := svc.SeedDefaultCategories(); err != nil {
		t.Fatalf("SeedDefaultCategories: %v", err)
	}
	cats, err := svc.List()
	if err != nil {
		t.Fatalf("svc.List: %v", err)
	}

	app, line := newAppForPaycheckAddNew(t, svc, cats)

	// Open sub-dialog.
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handlePaycheckWizardKey(enter)
	app = model.(*App)
	if app.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}

	// Fill: Name=CommuterPass, Parent=(top-level), Type=Expense.
	cFields := app.createCatDialog.Fields()
	cFields[0].Value = "CommuterPass"
	cFields[1].SelectedIndex = 0
	cFields[2].SelectedIndex = 0

	model, cmd := app.submitCreateCatDialog()
	app = model.(*App)
	if cmd == nil {
		t.Fatal("submit should produce a tea.Cmd")
	}
	msg := cmd()
	model, _ = app.Update(msg)
	app = model.(*App)

	// Persisted.
	got, err := svc.List()
	if err != nil {
		t.Fatalf("svc.List after submit: %v", err)
	}
	var found *category.Category
	for _, c := range got {
		if c.Name == "CommuterPass" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("'CommuterPass' should be persisted after submit")
	}

	if app.createCatDialog != nil {
		t.Error("createCatDialog should be cleared after submit")
	}
	if app.createCatSource != createCatSourceNone {
		t.Errorf("createCatSource = %d, want None after submit", app.createCatSource)
	}
	if app.createCatPaycheckLine != nil {
		t.Error("createCatPaycheckLine should be cleared after submit")
	}
	if app.paycheckWizard == nil || !app.paycheckWizard.IsVisible() {
		t.Fatal("paycheckWizard should be visible again after submit")
	}

	// Originating line points at the new category.
	w := app.paycheckWizard
	if line.IsAddNew() {
		t.Error("originating line should no longer be on the AddNew sentinel")
	}
	if line.IsTransfer() {
		t.Error("originating line should be in category mode, not transfer mode")
	}
	idx := line.SelectField().SelectedIndex
	if idx < 0 || idx >= len(w.categoryOptions) {
		t.Fatalf("originating line idx %d out of category range", idx)
	}
	if w.categoryOptions[idx] != "CommuterPass" {
		t.Errorf("originating line category = %q, want %q",
			w.categoryOptions[idx], "CommuterPass")
	}
	if w.categoryIDs[idx] != found.ID {
		t.Errorf("originating line categoryID = %s, want %s",
			w.categoryIDs[idx], found.ID)
	}
}

// TestPaycheckWizard_AddNew_PreservesTransferLineSelections pins the
// footgun: after the rebuild, transfer-mode lines' SelectedIndex must
// shift by the category-count delta so the same account stays selected.
func TestPaycheckWizard_AddNew_PreservesTransferLineSelections(t *testing.T) {
	database := dbtest.New(t)

	repo := category.NewRepository(database)
	svc := category.NewService(repo, database)
	if err := svc.SeedDefaultCategories(); err != nil {
		t.Fatalf("SeedDefaultCategories: %v", err)
	}
	cats, err := svc.List()
	if err != nil {
		t.Fatalf("svc.List: %v", err)
	}

	app, _ := newAppForPaycheckAddNew(t, svc, cats)
	w := app.paycheckWizard

	// Find the transfer-mode line and the account name it points at, so
	// we can re-verify identity after the rebuild.
	var transferLine *PaycheckLine
	for _, ln := range w.AdditionalTransfers() {
		if ln.IsTransfer() {
			transferLine = ln
			break
		}
	}
	if transferLine == nil {
		t.Fatal("test setup: expected at least one transfer-mode line")
	}
	wantAcctIdx := transferLine.AccountIndex()
	wantAcctName := w.accountOptions[wantAcctIdx]

	// Drive the open + submit flow.
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handlePaycheckWizardKey(enter)
	app = model.(*App)
	if app.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}
	cFields := app.createCatDialog.Fields()
	cFields[0].Value = "Cleaning" // alphabetically inserts in the middle
	cFields[1].SelectedIndex = 0
	cFields[2].SelectedIndex = 0

	model, cmd := app.submitCreateCatDialog()
	app = model.(*App)
	if cmd == nil {
		t.Fatal("submit should produce a tea.Cmd")
	}
	model, _ = app.Update(cmd())
	app = model.(*App)

	w = app.paycheckWizard
	// Transfer line identity is preserved across the rebuild: still in
	// transfer mode, still pointing at the same account by name.
	if !transferLine.IsTransfer() {
		t.Fatal("transfer line should still be in transfer mode after rebuild")
	}
	if transferLine.IsAddNew() {
		t.Error("transfer line drifted onto AddNew sentinel")
	}
	gotAcctIdx := transferLine.AccountIndex()
	if gotAcctIdx < 0 || gotAcctIdx >= len(w.accountOptions) {
		t.Fatalf("transfer line AccountIndex = %d out of range", gotAcctIdx)
	}
	if got := w.accountOptions[gotAcctIdx]; got != wantAcctName {
		t.Errorf("transfer line account = %q, want %q (preserved across rebuild)",
			got, wantAcctName)
	}

	// Sanity: pre-populated category lines (Earnings → Income > Salary,
	// Taxes → Federal/SS/Medicare) still reference their original
	// categories by name after the alphabetical insert shifts indices.
	salaryLines := w.EarningsLines()
	if len(salaryLines) > 0 {
		idx := salaryLines[0].SelectField().SelectedIndex
		if idx < 0 || idx >= len(w.categoryOptions) {
			t.Fatalf("salary line idx %d out of category range", idx)
		}
		if got := w.categoryOptions[idx]; got != "Income > Salary" {
			t.Errorf("salary line category = %q, want 'Income > Salary' (preserved by ID)", got)
		}
	}
}

// TestPaycheckFrequencyOptions_IncludesYearly asserts the picker
// offers a Yearly cadence (annual bonuses) and that a yearly schedule
// round-trips through Edit-as-paycheck back to that picker entry.
func TestPaycheckFrequencyOptions_IncludesYearly(t *testing.T) {
	yearlyIdx := -1
	for i, opt := range paycheckFrequencyOptions {
		if opt.frequency == scheduled.FrequencyYearly {
			yearlyIdx = i
		}
	}
	if yearlyIdx == -1 {
		t.Fatal("paycheckFrequencyOptions should include a Yearly entry")
	}
	opt := paycheckFrequencyForIndex(yearlyIdx)
	if opt.dayOfMonth != 0 || opt.secondaryDayOfMonth != 0 {
		t.Errorf("Yearly option should not set day-of-month fields, got %d/%d",
			opt.dayOfMonth, opt.secondaryDayOfMonth)
	}

	// Fortnightly must stay the default (index stability for existing users).
	if paycheckFrequencyOptions[defaultPaycheckFrequencyIndex].frequency != scheduled.FrequencyFortnightly {
		t.Errorf("default picker entry should remain Fortnightly")
	}

	// Round-trip: Edit-as-paycheck on a yearly schedule selects Yearly.
	fx := newPaycheckWizardFixture()
	st := scheduled.NewTransaction(fx.checkingID, scheduled.FrequencyYearly, types.Today())
	if got := paycheckFrequencyIndexFor(st); got != yearlyIdx {
		t.Errorf("paycheckFrequencyIndexFor(yearly) = %d, want %d", got, yearlyIdx)
	}
}
