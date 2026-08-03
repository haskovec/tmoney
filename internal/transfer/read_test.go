package transfer

import (
	"errors"
	"testing"

	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// assertTransfer checks the transfer-level fields every shape must get right.
func assertTransfer(t *testing.T, got *Transfer, wantKind Kind, fromAcct, toAcct types.ID, amount string) {
	t.Helper()
	if got.Kind != wantKind {
		t.Errorf("Kind = %v, want %v", got.Kind, wantKind)
	}
	if got.From.AccountID != fromAcct {
		t.Errorf("From.AccountID = %s, want %s", got.From.AccountID, fromAcct)
	}
	if got.To.AccountID != toAcct {
		t.Errorf("To.AccountID = %s, want %s", got.To.AccountID, toAcct)
	}
	want := types.MustNewMoney(amount)
	if !got.Amount.Equal(want) {
		t.Errorf("Amount = %s, want %s", got.Amount, want)
	}
	// The sign convention: From negative, To positive.
	if !got.From.Amount.IsNegative() {
		t.Errorf("From.Amount = %s, want negative", got.From.Amount)
	}
	if got.To.Amount.IsNegative() {
		t.Errorf("To.Amount = %s, want positive", got.To.Amount)
	}
	// Each leg's transfer_account_id points at the OTHER leg's account.
	if got.From.OtherAccountID != got.To.AccountID {
		t.Errorf("From.OtherAccountID = %s, want %s (To's account)", got.From.OtherAccountID, got.To.AccountID)
	}
	if got.To.OtherAccountID != got.From.AccountID {
		t.Errorf("To.OtherAccountID = %s, want %s (From's account)", got.To.OtherAccountID, got.From.AccountID)
	}
	if got.Movement != MovementCash {
		t.Errorf("Movement = %v, want MovementCash", got.Movement)
	}
	if got.Shape != ShapeWhole {
		t.Errorf("Shape = %v, want ShapeWhole", got.Shape)
	}
}

// TestResolve_RegToReg_FromEitherLeg is the baseline: the shape the old
// TransferRepository.GetByTransferID could already see.
func TestResolve_RegToReg_FromEitherLeg(t *testing.T) {
	h := newHarness(t)
	_, fromLeg, toLeg := h.seedRegToReg("250.00", types.NullableID{})

	for name, legID := range map[string]types.ID{"from leg": fromLeg, "to leg": toLeg} {
		t.Run(name, func(t *testing.T) {
			got, err := h.svc.Resolve(legID)
			if err != nil {
				t.Fatalf("Resolve(%s) error = %v", name, err)
			}
			assertTransfer(t, got, KindRegToReg, h.checking.ID, h.savings.ID, "250.00")
			if got.From.Ledger != LedgerRegular || got.To.Ledger != LedgerRegular {
				t.Errorf("ledgers = (%v, %v), want both regular", got.From.Ledger, got.To.Ledger)
			}
			if got.Memo != "rent reserve" {
				t.Errorf("Memo = %q, want %q", got.Memo, "rent reserve")
			}
		})
	}
}

// TestResolve_InvToReg_FromEitherLeg is the shape the old repository was
// structurally blind to: it asserted exactly two rows on `transactions`, so an
// inv↔reg pair (one row per table) always failed with
// "expected 2 transactions for transfer, found 1".
func TestResolve_InvToReg_FromEitherLeg(t *testing.T) {
	h := newHarness(t)
	_, invLeg, regLeg := h.seedInvToReg("500.00", types.NullableID{})

	for name, legID := range map[string]types.ID{"investment leg": invLeg, "regular leg": regLeg} {
		t.Run(name, func(t *testing.T) {
			got, err := h.svc.Resolve(legID)
			if err != nil {
				t.Fatalf("Resolve(%s) error = %v", name, err)
			}
			assertTransfer(t, got, KindInvToReg, h.brokerage.ID, h.checking.ID, "500.00")
			if got.From.Ledger != LedgerInvestment {
				t.Errorf("From.Ledger = %v, want investment", got.From.Ledger)
			}
			if got.To.Ledger != LedgerRegular {
				t.Errorf("To.Ledger = %v, want regular", got.To.Ledger)
			}
			if !got.HasInvestmentLeg() {
				t.Error("HasInvestmentLeg() = false, want true")
			}
		})
	}
}

// TestResolve_RegToInv_FromEitherLeg covers the mirror direction, where the
// investment leg is the positive one.
func TestResolve_RegToInv_FromEitherLeg(t *testing.T) {
	h := newHarness(t)
	_, invLeg, regLeg := h.seedRegToInv("750.00", types.NullableID{})

	for name, legID := range map[string]types.ID{"investment leg": invLeg, "regular leg": regLeg} {
		t.Run(name, func(t *testing.T) {
			got, err := h.svc.Resolve(legID)
			if err != nil {
				t.Fatalf("Resolve(%s) error = %v", name, err)
			}
			assertTransfer(t, got, KindRegToInv, h.checking.ID, h.brokerage.ID, "750.00")
			if got.From.Ledger != LedgerRegular {
				t.Errorf("From.Ledger = %v, want regular", got.From.Ledger)
			}
			if got.To.Ledger != LedgerInvestment {
				t.Errorf("To.Ledger = %v, want investment", got.To.Ledger)
			}
		})
	}
}

// TestResolve_InvToInv_FromEitherLeg covers the pair with no regular-table leg
// at all — invisible to every regular-table-only resolver.
func TestResolve_InvToInv_FromEitherLeg(t *testing.T) {
	h := newHarness(t)
	_, srcLeg, dstLeg := h.seedInvToInv("1200.00")

	for name, legID := range map[string]types.ID{"source leg": srcLeg, "destination leg": dstLeg} {
		t.Run(name, func(t *testing.T) {
			got, err := h.svc.Resolve(legID)
			if err != nil {
				t.Fatalf("Resolve(%s) error = %v", name, err)
			}
			assertTransfer(t, got, KindInvToInv, h.brokerage.ID, h.ira.ID, "1200.00")
			if got.From.Ledger != LedgerInvestment || got.To.Ledger != LedgerInvestment {
				t.Errorf("ledgers = (%v, %v), want both investment", got.From.Ledger, got.To.Ledger)
			}
			if got.CategoryID.Valid {
				t.Error("CategoryID.Valid = true; an inv↔inv transfer has no category column to read from")
			}
		})
	}
}

// TestResolve_CategoryReadFromRegularLeg pins that the category is found no
// matter which leg was named — for an inv↔reg transfer it lives only on the
// regular leg, in the other table from the investment leg.
func TestResolve_CategoryReadFromRegularLeg(t *testing.T) {
	h := newHarness(t)
	cat := h.newCategory("Investment Transfer",
		// An expense category is a valid non-system transfer label.
		"expense")
	catID := types.NullableID{ID: cat.ID, Valid: true}
	_, invLeg, regLeg := h.seedInvToReg("300.00", catID)

	for name, legID := range map[string]types.ID{"investment leg": invLeg, "regular leg": regLeg} {
		t.Run(name, func(t *testing.T) {
			got, err := h.svc.Resolve(legID)
			if err != nil {
				t.Fatalf("Resolve(%s) error = %v", name, err)
			}
			if !got.CategoryID.Valid {
				t.Fatal("CategoryID.Valid = false; want the regular leg's category")
			}
			if got.CategoryID.ID != cat.ID {
				t.Errorf("CategoryID = %s, want %s", got.CategoryID.ID, cat.ID)
			}
		})
	}
}

// TestResolve_ShareTransferIsClassifiedAsMovementShares is the guard that
// closes a live corruption hole. A transfer_shares pair is inv↔inv by account
// type, and investment.Service.UpdateTransferCash never checks the row type, so
// today a cash edit handed a share transfer's ID deletes rows without reversing
// their lot effects. Reads must SEE that it is shares so the verbs can refuse.
func TestResolve_ShareTransferIsClassifiedAsMovementShares(t *testing.T) {
	h := newHarness(t)
	transferID, srcLeg, dstLeg := h.seedShareTransferPair()

	for name, legID := range map[string]types.ID{"source leg": srcLeg, "destination leg": dstLeg} {
		t.Run(name, func(t *testing.T) {
			got, err := h.svc.Resolve(legID)
			if err != nil {
				t.Fatalf("Resolve(%s) error = %v", name, err)
			}
			if got.Movement != MovementShares {
				t.Errorf("Movement = %v, want MovementShares", got.Movement)
			}
			if got.Kind != KindInvToInv {
				t.Errorf("Kind = %v, want KindInvToInv (it is inv↔inv by account type)", got.Kind)
			}
		})
	}

	byID, err := h.svc.Get(transferID)
	if err != nil {
		t.Fatalf("Get(share transfer) error = %v", err)
	}
	if byID.Movement != MovementShares {
		t.Errorf("Get().Movement = %v, want MovementShares", byID.Movement)
	}
}

// TestResolve_SplitTransferLineResolvesWithShapeSplitLine pins the design's
// deliberate asymmetry: a split line RESOLVES so callers can explain the
// refusal, rather than reporting "not found". Only the verbs refuse.
func TestResolve_SplitTransferLineResolvesWithShapeSplitLine(t *testing.T) {
	h := newHarness(t)
	transferID, counterpartLeg, parentID := h.seedSplitTransferLine()

	got, err := h.svc.Resolve(counterpartLeg)
	if err != nil {
		t.Fatalf("Resolve(split line counterpart) error = %v; reads must succeed for split lines", err)
	}
	if got.Shape != ShapeSplitLine {
		t.Errorf("Shape = %v, want ShapeSplitLine", got.Shape)
	}
	if got.ParentTransactionID != parentID {
		t.Errorf("ParentTransactionID = %s, want %s", got.ParentTransactionID, parentID)
	}
	if got.TransferID != transferID {
		t.Errorf("TransferID = %s, want %s", got.TransferID, transferID)
	}
}

// TestResolve_NonTransferRowReturnsIsNotTransferError pins that an ordinary
// transaction is distinguished from a missing one. transaction.IsNotTransferError
// has existed since before this package and had no production producer; this is
// it.
func TestResolve_NonTransferRowReturnsIsNotTransferError(t *testing.T) {
	h := newHarness(t)
	plain := transaction.NewTransaction(h.checking.ID, testDate(), types.MustNewMoney("-42.00"))
	if err := h.txnRepo.Create(plain); err != nil {
		t.Fatalf("create plain transaction: %v", err)
	}

	_, err := h.svc.Resolve(plain.ID)
	if err == nil {
		t.Fatal("Resolve(plain transaction) error = nil, want *transaction.IsNotTransferError")
	}
	var target *transaction.IsNotTransferError
	if !errors.As(err, &target) {
		t.Fatalf("Resolve(plain) error = %T (%v), want *transaction.IsNotTransferError", err, err)
	}
}

// TestResolve_MissingRowReturnsNotFound distinguishes absent from
// present-but-not-a-transfer.
func TestResolve_MissingRowReturnsNotFound(t *testing.T) {
	h := newHarness(t)

	_, err := h.svc.Resolve(types.NewID())
	if err == nil {
		t.Fatal("Resolve(random ID) error = nil, want *dberrors.NotFoundError")
	}
	var target *dberrors.NotFoundError
	if !errors.As(err, &target) {
		t.Fatalf("Resolve(random) error = %T (%v), want *dberrors.NotFoundError", err, err)
	}
}

// TestGet_OrphanedSingleLegReturnsMalformedPairError pins the replacement for
// the old "expected 2 transactions" error — with per-table counts, and counting
// across BOTH ledgers so it cannot misfire on a legitimate inv↔reg pair.
func TestGet_OrphanedSingleLegReturnsMalformedPairError(t *testing.T) {
	h := newHarness(t)
	transferID, fromLeg, toLeg := h.seedRegToReg("100.00", types.NullableID{})

	// Orphan the pair by deleting one leg behind the service's back.
	if err := h.txnRepo.Delete(toLeg); err != nil {
		t.Fatalf("delete to-leg: %v", err)
	}

	_, err := h.svc.Get(transferID)
	if err == nil {
		t.Fatal("Get(orphaned transfer) error = nil, want *MalformedPairError")
	}
	var target *MalformedPairError
	if !errors.As(err, &target) {
		t.Fatalf("Get(orphaned) error = %T (%v), want *MalformedPairError", err, err)
	}
	if target.RegularLegs != 1 || target.InvestmentLegs != 0 {
		t.Errorf("counts = (regular %d, investment %d), want (1, 0)", target.RegularLegs, target.InvestmentLegs)
	}

	// Resolving the surviving leg surfaces the same malformed-pair error.
	if _, err := h.svc.Resolve(fromLeg); !errors.As(err, &target) {
		t.Errorf("Resolve(surviving leg) error = %T (%v), want *MalformedPairError", err, err)
	}
}

// TestGet_UnknownTransferIDReturnsNotFound covers a transfer_id that names no
// legs at all, which is a different failure from a half-present pair.
func TestGet_UnknownTransferIDReturnsNotFound(t *testing.T) {
	h := newHarness(t)

	_, err := h.svc.Get(types.NewID())
	var target *dberrors.NotFoundError
	if !errors.As(err, &target) {
		t.Fatalf("Get(random transfer ID) error = %T (%v), want *dberrors.NotFoundError", err, err)
	}
}

// TestResolve_VoidedPairIsStable pins that a voided reg↔reg pair — both legs
// zeroed, so the sign carries no orientation — still resolves, and resolves to
// the SAME orientation every time. Which leg lands in From is arbitrary and
// documented as such; instability would be the actual bug, because it would
// make a voided transfer render differently on each refresh.
func TestResolve_VoidedPairIsStable(t *testing.T) {
	h := newHarness(t)
	transferID, fromLeg, _ := h.seedRegToReg("400.00", types.NullableID{})

	if err := h.txnSvc.VoidTransaction(fromLeg); err != nil {
		t.Fatalf("VoidTransaction: %v", err)
	}

	first, err := h.svc.Get(transferID)
	if err != nil {
		t.Fatalf("Get(voided transfer) error = %v", err)
	}
	for i := range 5 {
		again, err := h.svc.Get(transferID)
		if err != nil {
			t.Fatalf("Get(voided transfer) repeat %d error = %v", i, err)
		}
		if again.From.RowID != first.From.RowID || again.To.RowID != first.To.RowID {
			t.Fatalf("orientation unstable across calls: first=(%s,%s) repeat=(%s,%s)",
				first.From.RowID, first.To.RowID, again.From.RowID, again.To.RowID)
		}
	}
	// Both legs are still present and both belong to the two fixture accounts.
	accts := map[types.ID]bool{h.checking.ID: true, h.savings.ID: true}
	if !accts[first.From.AccountID] || !accts[first.To.AccountID] {
		t.Errorf("legs = (%s, %s), want the checking/savings pair", first.From.AccountID, first.To.AccountID)
	}
	if first.From.AccountID == first.To.AccountID {
		t.Error("both legs resolved to the same account")
	}
}

// TestLegForAccount pins the accessor the TUI uses to put the cursor back on
// whichever leg is in the register the user is looking at.
func TestLegForAccount(t *testing.T) {
	h := newHarness(t)
	_, invLeg, _ := h.seedInvToReg("125.00", types.NullableID{})

	got, err := h.svc.Resolve(invLeg)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	leg, ok := got.LegForAccount(h.brokerage.ID)
	if !ok {
		t.Fatal("LegForAccount(brokerage) not found")
	}
	if leg.Ledger != LedgerInvestment || leg.RowID != invLeg {
		t.Errorf("brokerage leg = (%v, %s), want (investment, %s)", leg.Ledger, leg.RowID, invLeg)
	}

	if _, ok := got.LegForAccount(h.ira.ID); ok {
		t.Error("LegForAccount(uninvolved account) returned a leg")
	}
}

// TestResolve_StatusNormalizedAcrossLedgers pins that an investment leg's
// "pending" reads back as the regular ledger's "uncleared" rather than leaking
// the investment enum into the read model.
func TestResolve_StatusNormalizedAcrossLedgers(t *testing.T) {
	h := newHarness(t)
	_, invLeg, _ := h.seedInvToInv("900.00")

	got, err := h.svc.Resolve(invLeg)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	for _, leg := range got.Legs() {
		if !leg.Status.IsValid() {
			t.Errorf("leg %s status = %q, which is not a valid transaction.Status", leg.RowID, leg.Status)
		}
		if leg.Status != transaction.StatusUncleared {
			t.Errorf("leg %s status = %q, want %q (investment 'pending' normalizes to it)",
				leg.RowID, leg.Status, transaction.StatusUncleared)
		}
	}
}
