package transfer

import (
	"errors"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
)

// TestClassifyKind_AllFourCombinations is the successor to
// transaction.TestChooseTransferDispatch_AllFourCombinations. It pins the same
// truth table on the classifier's new home.
func TestClassifyKind_AllFourCombinations(t *testing.T) {
	tests := []struct {
		name string
		from account.Type
		to   account.Type
		want Kind
	}{
		{"checking→savings", account.TypeChecking, account.TypeSavings, KindRegToReg},
		{"checking→credit card", account.TypeChecking, account.TypeCreditCard, KindRegToReg},
		{"brokerage→checking", account.TypeInvestment, account.TypeChecking, KindInvToReg},
		{"checking→brokerage", account.TypeChecking, account.TypeInvestment, KindRegToInv},
		{"brokerage→brokerage", account.TypeInvestment, account.TypeInvestment, KindInvToInv},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyKind(tc.from, tc.to); got != tc.want {
				t.Errorf("ClassifyKind(%q, %q) = %v, want %v", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

// TestClassifyKind_HSATreatedAsInvestment pins that HSA routes through the
// investment arms, matching account.Type.IsInvestmentType.
func TestClassifyKind_HSATreatedAsInvestment(t *testing.T) {
	if got := ClassifyKind(account.TypeHSA, account.TypeChecking); got != KindInvToReg {
		t.Errorf("HSA→checking = %v, want KindInvToReg", got)
	}
	if got := ClassifyKind(account.TypeChecking, account.TypeHSA); got != KindRegToInv {
		t.Errorf("checking→HSA = %v, want KindRegToInv", got)
	}
	if got := ClassifyKind(account.TypeHSA, account.TypeInvestment); got != KindInvToInv {
		t.Errorf("HSA→brokerage = %v, want KindInvToInv", got)
	}
}

// TestClassifyKind_UnknownTypeFallsThroughToRegToReg preserves the fallback
// behavior of the ChooseTransferDispatch this replaces: an unrecognized account
// type is not an investment type, so it lands in the regular arm rather than
// panicking or returning a zero-value surprise.
func TestClassifyKind_UnknownTypeFallsThroughToRegToReg(t *testing.T) {
	unknown := account.Type("not_a_real_type")
	if got := ClassifyKind(unknown, account.TypeChecking); got != KindRegToReg {
		t.Errorf("unknown→checking = %v, want KindRegToReg", got)
	}
	if got := ClassifyKind(account.TypeChecking, unknown); got != KindRegToReg {
		t.Errorf("checking→unknown = %v, want KindRegToReg", got)
	}
	if got := ClassifyKind(unknown, unknown); got != KindRegToReg {
		t.Errorf("unknown→unknown = %v, want KindRegToReg", got)
	}
}

// TestLedgerFor pins the table each account type's legs live in.
func TestLedgerFor(t *testing.T) {
	regular := []account.Type{account.TypeChecking, account.TypeSavings, account.TypeCreditCard}
	for _, at := range regular {
		if got := LedgerFor(at); got != LedgerRegular {
			t.Errorf("LedgerFor(%q) = %v, want LedgerRegular", at, got)
		}
	}
	for _, at := range []account.Type{account.TypeInvestment, account.TypeHSA} {
		if got := LedgerFor(at); got != LedgerInvestment {
			t.Errorf("LedgerFor(%q) = %v, want LedgerInvestment", at, got)
		}
	}
}

// TestKindStoresCategory pins the only kind-dependent rule in the design:
// inv↔inv has nowhere to put a category because both its legs live in
// investment_transactions, which has no category_id column.
func TestKindStoresCategory(t *testing.T) {
	for _, k := range []Kind{KindRegToReg, KindInvToReg, KindRegToInv} {
		if !k.StoresCategory() {
			t.Errorf("%v.StoresCategory() = false, want true", k)
		}
	}
	if KindInvToInv.StoresCategory() {
		t.Error("KindInvToInv.StoresCategory() = true, want false")
	}
}

// TestStatusRoundTrip pins the two directions of the ledger status mapping that
// have equivalents on both sides.
func TestStatusRoundTrip(t *testing.T) {
	cases := []struct {
		regular transaction.Status
		inv     investment.TransactionStatus
	}{
		{transaction.StatusUncleared, investment.TransactionStatusPending},
		{transaction.StatusCleared, investment.TransactionStatusCleared},
		{transaction.StatusReconciled, investment.TransactionStatusReconciled},
	}
	for _, tc := range cases {
		gotInv, err := StatusFromRegular(tc.regular)
		if err != nil {
			t.Fatalf("StatusFromRegular(%q) unexpected error: %v", tc.regular, err)
		}
		if gotInv != tc.inv {
			t.Errorf("StatusFromRegular(%q) = %q, want %q", tc.regular, gotInv, tc.inv)
		}
		if gotReg := StatusToRegular(tc.inv); gotReg != tc.regular {
			t.Errorf("StatusToRegular(%q) = %q, want %q", tc.inv, gotReg, tc.regular)
		}
	}
}

// TestStatusFromRegular_VoidIsUnrepresentable is the behavior change this
// package introduces over investment.statusFromRegular, which silently coerced
// void→pending — so a void regular leg round-tripped through an edit came back
// Uncleared. The loss is now a typed error instead of silent data change.
func TestStatusFromRegular_VoidIsUnrepresentable(t *testing.T) {
	_, err := StatusFromRegular(transaction.StatusVoid)
	if err == nil {
		t.Fatal("StatusFromRegular(void) returned nil error, want *UnrepresentableStatusError")
	}
	var target *UnrepresentableStatusError
	if !errors.As(err, &target) {
		t.Fatalf("StatusFromRegular(void) error = %T (%v), want *UnrepresentableStatusError", err, err)
	}
	if target.Status != transaction.StatusVoid {
		t.Errorf("UnrepresentableStatusError.Status = %q, want %q", target.Status, transaction.StatusVoid)
	}
}

// TestStatusFromRegular_InvalidStatus rejects a status that is not in the enum
// at all, rather than defaulting it into pending.
func TestStatusFromRegular_InvalidStatus(t *testing.T) {
	if _, err := StatusFromRegular(transaction.Status("banana")); err == nil {
		t.Fatal("StatusFromRegular(\"banana\") returned nil error, want a failure")
	}
}
