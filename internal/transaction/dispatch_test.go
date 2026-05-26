package transaction

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
)

// TestChooseTransferDispatch_AllFourCombinations exercises every
// (account.Type, account.Type) combination of interest and asserts the
// correct TransferDispatchKind. HSA on either side must route via the
// investment branches — otherwise an HSA→checking sweep would silently
// take the reg/reg path and mint a malformed regular-table row in the
// HSA's ledger.
func TestChooseTransferDispatch_AllFourCombinations(t *testing.T) {
	// Representative regular and investment types. The dispatcher only
	// looks at IsInvestmentType(), so one of each side is enough plus
	// dedicated HSA coverage below.
	reg := []account.Type{
		account.TypeChecking,
		account.TypeSavings,
		account.TypeCreditCard,
		account.TypeCash,
		account.TypeLoan,
		account.TypeAsset,
	}
	inv := []account.Type{
		account.TypeInvestment,
		account.TypeHSA,
	}

	cases := []struct {
		name string
		from account.Type
		to   account.Type
		want TransferDispatchKind
	}{}

	for _, f := range reg {
		for _, tt := range reg {
			cases = append(cases, struct {
				name string
				from account.Type
				to   account.Type
				want TransferDispatchKind
			}{
				name: string(f) + "->" + string(tt) + "=RegToReg",
				from: f,
				to:   tt,
				want: DispatchRegToReg,
			})
		}
		for _, tt := range inv {
			cases = append(cases, struct {
				name string
				from account.Type
				to   account.Type
				want TransferDispatchKind
			}{
				name: string(f) + "->" + string(tt) + "=RegToInv",
				from: f,
				to:   tt,
				want: DispatchRegToInv,
			})
		}
	}
	for _, f := range inv {
		for _, tt := range reg {
			cases = append(cases, struct {
				name string
				from account.Type
				to   account.Type
				want TransferDispatchKind
			}{
				name: string(f) + "->" + string(tt) + "=InvToReg",
				from: f,
				to:   tt,
				want: DispatchInvToReg,
			})
		}
		for _, tt := range inv {
			cases = append(cases, struct {
				name string
				from account.Type
				to   account.Type
				want TransferDispatchKind
			}{
				name: string(f) + "->" + string(tt) + "=InvToInv",
				from: f,
				to:   tt,
				want: DispatchInvToInv,
			})
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ChooseTransferDispatch(tc.from, tc.to)
			if got != tc.want {
				t.Errorf("ChooseTransferDispatch(%q, %q) = %v, want %v",
					tc.from, tc.to, got, tc.want)
			}
		})
	}
}

// TestChooseTransferDispatch_UnknownTypeFallsThroughToRegToReg covers the
// dispatcher's tolerance for an unknown (zero-value) account.Type, which
// happens when callers look up a type from a stale account-id and get the
// empty string back. The dispatcher must dispatch to RegToReg in that
// case so the call falls through to the regular service-layer guards
// instead of silently routing to an investment path.
func TestChooseTransferDispatch_UnknownTypeFallsThroughToRegToReg(t *testing.T) {
	var unknown account.Type
	if got := ChooseTransferDispatch(unknown, account.TypeChecking); got != DispatchRegToReg {
		t.Errorf("unknown→checking dispatch = %v, want RegToReg", got)
	}
	if got := ChooseTransferDispatch(account.TypeChecking, unknown); got != DispatchRegToReg {
		t.Errorf("checking→unknown dispatch = %v, want RegToReg", got)
	}
	if got := ChooseTransferDispatch(unknown, unknown); got != DispatchRegToReg {
		t.Errorf("unknown→unknown dispatch = %v, want RegToReg", got)
	}
}
