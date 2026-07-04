package scheduled

import (
	"errors"
	"testing"

	"github.com/haskovec/tmoney/internal/loan"
	"github.com/haskovec/tmoney/internal/types"
)

// findLoanSection returns the first split tagged with the given loan_section,
// or nil.
func findLoanSection(splits []*Split, section string) *Split {
	for _, sp := range splits {
		if sp.LoanSection.Valid && sp.LoanSection.String == section {
			return sp
		}
	}
	return nil
}

func TestBuildLoanSchedule_AssemblesMonthlyLoanShape(t *testing.T) {
	loanID := types.NewID()
	fundingID := types.NewID()
	payeeID := types.NewID()
	interestCat := types.NewID()
	nextDate := types.NewDate(2026, 8, 15)

	// 380,000 @ 6.5% / 360mo → payment 2401.86; interest 2058.33, principal 343.53.
	st, final, err := BuildLoanSchedule(fundingID, nextDate, payeeID, true, LoanSnapshotInput{
		LoanAccountID: loanID,
		APR:           types.MustNewMoney("6.5"),
		Owed:          types.MustNewMoney("380000"),
		PIPayment:     types.MustNewMoney("2401.86"),
		InterestCatID: interestCat,
	})
	if err != nil {
		t.Fatalf("BuildLoanSchedule: %v", err)
	}
	if final {
		t.Errorf("final = true, want false for a full-term loan")
	}

	if st.AccountID != fundingID {
		t.Errorf("schedule AccountID = %v, want funding account %v", st.AccountID, fundingID)
	}
	if st.Frequency != FrequencyMonthly {
		t.Errorf("frequency = %v, want monthly", st.Frequency)
	}
	if !st.DayOfMonth.Valid || st.DayOfMonth.Int64 != 15 {
		t.Errorf("day-of-month = %v, want 15", st.DayOfMonth)
	}
	if !st.NextDate.Equal(nextDate) {
		t.Errorf("next date = %s, want %s", st.NextDate, nextDate)
	}
	if !st.HasPayee() || st.PayeeID.ID != payeeID {
		t.Errorf("payee = %v, want %v", st.PayeeID, payeeID)
	}
	if !st.IsAutoPost() {
		t.Error("auto-post = false, want true")
	}
	if st.HasCategory() {
		t.Error("parent category should be cleared on a multi-line loan schedule")
	}

	// Parent amount is the negative funding-account draft; lines sum to it.
	if want := types.MustNewMoney("-2401.86"); !st.Amount.Money.Equal(want) {
		t.Errorf("parent amount = %s, want %s", st.Amount.Money, want)
	}
	sum := types.ZeroMoney
	for _, sp := range st.Splits {
		sum = sum.Add(sp.Amount)
		if !sp.LoanSection.Valid {
			t.Errorf("split %v is missing a loan_section tag", sp.Amount)
		}
	}
	if !sum.Equal(st.Amount.Money) {
		t.Errorf("split sum %s != parent %s", sum, st.Amount.Money)
	}

	if findLoanSection([]*Split(st.Splits), LoanSectionInterest) == nil {
		t.Error("no interest line in assembled schedule")
	}
	if p := findLoanSection([]*Split(st.Splits), LoanSectionPrincipal); p == nil {
		t.Error("no principal line in assembled schedule")
	} else if !p.TransferAccountID.Valid || p.TransferAccountID.ID != loanID {
		t.Errorf("principal transfer target = %v, want loan %v", p.TransferAccountID, loanID)
	}
}

func TestBuildLoanSchedule_PropagatesSnapshotError(t *testing.T) {
	// P&I payment below the first month's interest → negative amortization.
	_, _, err := BuildLoanSchedule(types.NewID(), types.NewDate(2026, 8, 1), types.NilID, false, LoanSnapshotInput{
		LoanAccountID: types.NewID(),
		APR:           types.MustNewMoney("6.5"),
		Owed:          types.MustNewMoney("380000"),
		PIPayment:     types.MustNewMoney("100"),
		InterestCatID: types.NewID(),
	})
	if !errors.Is(err, loan.ErrNegativeAmortization) {
		t.Fatalf("err = %v, want ErrNegativeAmortization", err)
	}
}

func TestBuildLoanSnapshot_PrincipalCategory(t *testing.T) {
	loanID := types.NewID()
	interestCat := types.NewID()
	principalCat := types.NewID()

	_, splits, _, err := BuildLoanSnapshot(LoanSnapshotInput{
		LoanAccountID:  loanID,
		APR:            types.MustNewMoney("6.5"),
		Owed:           types.MustNewMoney("380000"),
		PIPayment:      types.MustNewMoney("2401.86"),
		InterestCatID:  interestCat,
		PrincipalCatID: principalCat,
	})
	if err != nil {
		t.Fatalf("BuildLoanSnapshot: %v", err)
	}
	p := findLoanSection(splits, LoanSectionPrincipal)
	if p == nil {
		t.Fatal("no principal line")
	}
	// The principal line is a categorized transfer: transfer target AND category.
	if !p.TransferAccountID.Valid || p.TransferAccountID.ID != loanID {
		t.Errorf("principal transfer target = %v, want %v", p.TransferAccountID, loanID)
	}
	if !p.CategoryID.Valid || p.CategoryID.ID != principalCat {
		t.Errorf("principal category = %v, want %v", p.CategoryID, principalCat)
	}
	// A categorized transfer satisfies the relaxed split rule (at least one of
	// category / transfer). LoanSection tag is preserved.
	if !p.LoanSection.Valid || p.LoanSection.String != LoanSectionPrincipal {
		t.Errorf("principal loan_section = %v, want %q", p.LoanSection, LoanSectionPrincipal)
	}
}

func TestBuildLoanSnapshot_PrincipalCategoryOmittedWhenNil(t *testing.T) {
	// PrincipalCatID left nil → a bare (uncategorized) transfer line, the
	// old shape. Interest category is unaffected.
	_, splits, _, err := BuildLoanSnapshot(LoanSnapshotInput{
		LoanAccountID: types.NewID(),
		APR:           types.MustNewMoney("6.5"),
		Owed:          types.MustNewMoney("380000"),
		PIPayment:     types.MustNewMoney("2401.86"),
		InterestCatID: types.NewID(),
	})
	if err != nil {
		t.Fatalf("BuildLoanSnapshot: %v", err)
	}
	p := findLoanSection(splits, LoanSectionPrincipal)
	if p == nil {
		t.Fatal("no principal line")
	}
	if p.CategoryID.Valid {
		t.Errorf("principal category = %v, want unset (bare transfer)", p.CategoryID)
	}
}

// TestBuildLoanSnapshot_ZeroRatePrincipalStillLabeled pins that a 0% loan (no
// interest line) still carries the principal category — the spec's "0% loans:
// no interest line, principal line still labeled".
func TestBuildLoanSnapshot_ZeroRatePrincipalStillLabeled(t *testing.T) {
	principalCat := types.NewID()
	_, splits, _, err := BuildLoanSnapshot(LoanSnapshotInput{
		LoanAccountID:  types.NewID(),
		APR:            types.ZeroMoney,
		Owed:           types.MustNewMoney("32000"),
		PIPayment:      types.MustNewMoney("533.34"),
		PrincipalCatID: principalCat,
	})
	if err != nil {
		t.Fatalf("BuildLoanSnapshot: %v", err)
	}
	if findLoanSection(splits, LoanSectionInterest) != nil {
		t.Error("0% loan should have no interest line")
	}
	p := findLoanSection(splits, LoanSectionPrincipal)
	if p == nil || !p.CategoryID.Valid || p.CategoryID.ID != principalCat {
		t.Errorf("0%% loan principal category = %v, want %v", p, principalCat)
	}
}

func TestBuildLoanSnapshot_Standard(t *testing.T) {
	loanID := types.NewID()
	interestCat := types.NewID()

	// 380,000 @ 6.5% / 360mo → payment 2401.86.
	// interest = round(380000 * 6.5 / 1200) = round(2058.3333) = 2058.33
	// principal = 2401.86 - 2058.33 = 343.53
	parent, splits, final, err := BuildLoanSnapshot(LoanSnapshotInput{
		LoanAccountID: loanID,
		APR:           types.MustNewMoney("6.5"),
		Owed:          types.MustNewMoney("380000"),
		PIPayment:     types.MustNewMoney("2401.86"),
		InterestCatID: interestCat,
	})
	if err != nil {
		t.Fatalf("BuildLoanSnapshot: %v", err)
	}
	if final {
		t.Errorf("final = true, want false for a full-term loan")
	}
	if len(splits) != 2 {
		t.Fatalf("got %d splits, want 2 (interest, principal)", len(splits))
	}

	interest := findLoanSection(splits, LoanSectionInterest)
	if interest == nil {
		t.Fatal("no interest line")
	}
	if want := types.MustNewMoney("-2058.33"); !interest.Amount.Equal(want) {
		t.Errorf("interest amount = %s, want %s", interest.Amount, want)
	}
	if !interest.CategoryID.Valid || interest.CategoryID.ID != interestCat {
		t.Errorf("interest line category = %v, want %v", interest.CategoryID, interestCat)
	}
	if interest.TransferAccountID.Valid {
		t.Error("interest line should be categorized, not a transfer")
	}

	principal := findLoanSection(splits, LoanSectionPrincipal)
	if principal == nil {
		t.Fatal("no principal line")
	}
	if want := types.MustNewMoney("-343.53"); !principal.Amount.Equal(want) {
		t.Errorf("principal amount = %s, want %s", principal.Amount, want)
	}
	if !principal.TransferAccountID.Valid || principal.TransferAccountID.ID != loanID {
		t.Errorf("principal line transfer target = %v, want loan %v", principal.TransferAccountID, loanID)
	}
	if principal.CategoryID.Valid {
		t.Error("principal line must have no category (transfer only)")
	}

	// Parent must equal the signed sum of the lines, and equal -(P&I).
	if got := SplitCollection(splits).Total(); !got.Equal(parent) {
		t.Errorf("split total %s != parent %s", got, parent)
	}
	if want := types.MustNewMoney("-2401.86"); !parent.Equal(want) {
		t.Errorf("parent = %s, want %s", parent, want)
	}
}

func TestBuildLoanSnapshot_WithEscrow(t *testing.T) {
	loanID := types.NewID()
	interestCat := types.NewID()
	taxCat := types.NewID()
	insCat := types.NewID()

	parent, splits, _, err := BuildLoanSnapshot(LoanSnapshotInput{
		LoanAccountID: loanID,
		APR:           types.MustNewMoney("6.5"),
		Owed:          types.MustNewMoney("380000"),
		PIPayment:     types.MustNewMoney("2401.86"),
		InterestCatID: interestCat,
		Escrow: []LoanEscrowLine{
			{CategoryID: taxCat, Amount: types.MustNewMoney("650")},
			{CategoryID: insCat, Amount: types.MustNewMoney("120"), Memo: "homeowners"},
		},
	})
	if err != nil {
		t.Fatalf("BuildLoanSnapshot: %v", err)
	}
	if len(splits) != 4 {
		t.Fatalf("got %d splits, want 4 (interest, principal, 2 escrow)", len(splits))
	}
	// Escrow is a fixed pass-through; it never enters the split math, so
	// interest/principal are unchanged from the no-escrow case. Parent = draft.
	if want := types.MustNewMoney("-3171.86"); !parent.Equal(want) {
		t.Errorf("parent = %s, want %s (P&I 2401.86 + escrow 770)", parent, want)
	}
	if got := SplitCollection(splits).Total(); !got.Equal(parent) {
		t.Errorf("split total %s != parent %s", got, parent)
	}
	// Escrow lines carry the escrow tag and negative signs; memo preserved.
	var escrowCount int
	for _, sp := range splits {
		if sp.LoanSection.Valid && sp.LoanSection.String == LoanSectionEscrow {
			escrowCount++
			if sp.Amount.IsPositive() {
				t.Errorf("escrow line amount %s should be negative", sp.Amount)
			}
		}
	}
	if escrowCount != 2 {
		t.Errorf("got %d escrow lines, want 2", escrowCount)
	}
}

func TestBuildLoanSnapshot_ZeroRateOmitsInterest(t *testing.T) {
	loanID := types.NewID()
	// 0% loan: no interest line, InterestCatID may be nil.
	parent, splits, final, err := BuildLoanSnapshot(LoanSnapshotInput{
		LoanAccountID: loanID,
		APR:           types.MustNewMoney("0"),
		Owed:          types.MustNewMoney("32000"),
		PIPayment:     types.MustNewMoney("533.34"), // ceil(32000/60)
	})
	if err != nil {
		t.Fatalf("BuildLoanSnapshot: %v", err)
	}
	if final {
		t.Errorf("final = true, want false")
	}
	if len(splits) != 1 {
		t.Fatalf("got %d splits, want 1 (principal only; no interest line at 0%%)", len(splits))
	}
	if findLoanSection(splits, LoanSectionInterest) != nil {
		t.Error("0% loan must have no interest line")
	}
	principal := findLoanSection(splits, LoanSectionPrincipal)
	if principal == nil {
		t.Fatal("no principal line")
	}
	if want := types.MustNewMoney("-533.34"); !principal.Amount.Equal(want) {
		t.Errorf("principal = %s, want %s (full payment to principal at 0%%)", principal.Amount, want)
	}
	if want := types.MustNewMoney("-533.34"); !parent.Equal(want) {
		t.Errorf("parent = %s, want %s", parent, want)
	}
}

func TestBuildLoanSnapshot_FinalPaymentClamp(t *testing.T) {
	loanID := types.NewID()
	interestCat := types.NewID()
	// One payment left: owed 100, big P&I → principal clamps to owed.
	// interest = round(100 * 6.5 / 1200) = round(0.5417) = 0.54
	parent, splits, final, err := BuildLoanSnapshot(LoanSnapshotInput{
		LoanAccountID: loanID,
		APR:           types.MustNewMoney("6.5"),
		Owed:          types.MustNewMoney("100"),
		PIPayment:     types.MustNewMoney("2401.86"),
		InterestCatID: interestCat,
	})
	if err != nil {
		t.Fatalf("BuildLoanSnapshot: %v", err)
	}
	if !final {
		t.Errorf("final = false, want true (clamped last payment)")
	}
	principal := findLoanSection(splits, LoanSectionPrincipal)
	if want := types.MustNewMoney("-100"); !principal.Amount.Equal(want) {
		t.Errorf("clamped principal = %s, want %s", principal.Amount, want)
	}
	// Parent shrinks to interest + clamped principal.
	if want := types.MustNewMoney("-100.54"); !parent.Equal(want) {
		t.Errorf("clamped parent = %s, want %s", parent, want)
	}
	if got := SplitCollection(splits).Total(); !got.Equal(parent) {
		t.Errorf("split total %s != parent %s (clamped lines must still sum)", got, parent)
	}
}

func TestBuildLoanSnapshot_NegativeAmortization(t *testing.T) {
	_, _, _, err := BuildLoanSnapshot(LoanSnapshotInput{
		LoanAccountID: types.NewID(),
		APR:           types.MustNewMoney("6.5"),
		Owed:          types.MustNewMoney("380000"),
		PIPayment:     types.MustNewMoney("100"), // < month's interest
		InterestCatID: types.NewID(),
	})
	if !errors.Is(err, loan.ErrNegativeAmortization) {
		t.Fatalf("err = %v, want ErrNegativeAmortization", err)
	}
}

func TestBuildLoanSnapshot_MissingInterestCategory(t *testing.T) {
	_, _, _, err := BuildLoanSnapshot(LoanSnapshotInput{
		LoanAccountID: types.NewID(),
		APR:           types.MustNewMoney("6.5"),
		Owed:          types.MustNewMoney("380000"),
		PIPayment:     types.MustNewMoney("2401.86"),
		// InterestCatID left nil while interest > 0.
	})
	if err == nil {
		t.Fatal("expected error when interest accrues but no interest category given")
	}
}

func TestBuildLoanSnapshot_MissingLoanAccount(t *testing.T) {
	_, _, _, err := BuildLoanSnapshot(LoanSnapshotInput{
		APR:           types.MustNewMoney("6.5"),
		Owed:          types.MustNewMoney("380000"),
		PIPayment:     types.MustNewMoney("2401.86"),
		InterestCatID: types.NewID(),
	})
	if err == nil {
		t.Fatal("expected error when loan account ID is nil")
	}
}

func TestBuildLoanSnapshot_EscrowMissingCategory(t *testing.T) {
	_, _, _, err := BuildLoanSnapshot(LoanSnapshotInput{
		LoanAccountID: types.NewID(),
		APR:           types.MustNewMoney("0"),
		Owed:          types.MustNewMoney("32000"),
		PIPayment:     types.MustNewMoney("533.34"),
		Escrow:        []LoanEscrowLine{{Amount: types.MustNewMoney("50")}}, // no category
	})
	if err == nil {
		t.Fatal("expected error for escrow line with no category")
	}
}
