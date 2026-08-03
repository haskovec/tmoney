package transfer

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// shapeCase names one of the four (From, To) ledger combinations by the fixture
// accounts that produce it, so every verb can be driven across all four.
type shapeCase struct {
	name       string
	fromOf     func(h *harness) *account.Account
	toOf       func(h *harness) *account.Account
	wantKind   Kind
	fromLedger Ledger
	toLedger   Ledger
	// categorizable is false for inv↔inv, whose legs both live in
	// investment_transactions and so have no category column.
	categorizable bool
}

func allShapes() []shapeCase {
	return []shapeCase{
		{
			name:          "reg→reg",
			fromOf:        func(h *harness) *account.Account { return h.checking },
			toOf:          func(h *harness) *account.Account { return h.savings },
			wantKind:      KindRegToReg,
			fromLedger:    LedgerRegular,
			toLedger:      LedgerRegular,
			categorizable: true,
		},
		{
			name:          "inv→reg",
			fromOf:        func(h *harness) *account.Account { return h.brokerage },
			toOf:          func(h *harness) *account.Account { return h.checking },
			wantKind:      KindInvToReg,
			fromLedger:    LedgerInvestment,
			toLedger:      LedgerRegular,
			categorizable: true,
		},
		{
			name:          "reg→inv",
			fromOf:        func(h *harness) *account.Account { return h.checking },
			toOf:          func(h *harness) *account.Account { return h.brokerage },
			wantKind:      KindRegToInv,
			fromLedger:    LedgerRegular,
			toLedger:      LedgerInvestment,
			categorizable: true,
		},
		{
			name:          "inv→inv",
			fromOf:        func(h *harness) *account.Account { return h.brokerage },
			toOf:          func(h *harness) *account.Account { return h.ira },
			wantKind:      KindInvToInv,
			fromLedger:    LedgerInvestment,
			toLedger:      LedgerInvestment,
			categorizable: false,
		},
	}
}

func (sc shapeCase) spec(h *harness, amount string) Spec {
	return Spec{
		FromAccountID: sc.fromOf(h).ID,
		ToAccountID:   sc.toOf(h).ID,
		Date:          testDate(),
		Amount:        types.MustNewMoney(amount),
		Memo:          "matrix " + sc.name,
	}
}

// =============================================================================
// Create
// =============================================================================

// TestCreate_AllFourShapes is the core claim of the design: one Create, with no
// dispatch switch, produces all four transfer shapes correctly.
func TestCreate_AllFourShapes(t *testing.T) {
	for _, sc := range allShapes() {
		t.Run(sc.name, func(t *testing.T) {
			h := newHarness(t)
			res, err := h.svc.Create(sc.spec(h, "400.00"))
			if err != nil {
				t.Fatalf("Create(%s) error = %v", sc.name, err)
			}

			if res.Kind != sc.wantKind {
				t.Errorf("Kind = %v, want %v", res.Kind, sc.wantKind)
			}
			if res.From.Ledger != sc.fromLedger {
				t.Errorf("From.Ledger = %v, want %v", res.From.Ledger, sc.fromLedger)
			}
			if res.To.Ledger != sc.toLedger {
				t.Errorf("To.Ledger = %v, want %v", res.To.Ledger, sc.toLedger)
			}
			if res.Before != nil {
				t.Error("Before should be nil for Create")
			}

			// Read it back through the read model and check the invariants.
			got, err := h.svc.Get(res.TransferID)
			if err != nil {
				t.Fatalf("Get after Create: %v", err)
			}
			assertTransfer(t, got, sc.wantKind, sc.fromOf(h).ID, sc.toOf(h).ID, "400.00")
			if got.Memo != "matrix "+sc.name {
				t.Errorf("Memo = %q, want %q", got.Memo, "matrix "+sc.name)
			}
			if got.Status != transaction.StatusUncleared {
				t.Errorf("Status = %q, want %q (the zero-value default)", got.Status, transaction.StatusUncleared)
			}
		})
	}
}

// TestCreate_InvestmentLegsAreTypedTransferCash pins the load-bearing detail
// that fails silently: GetCashBalance sums TotalAmount only over rows whose
// Type.AffectsCash(), so an investment leg written with the wrong transaction
// type stops counting as money with no error anywhere.
func TestCreate_InvestmentLegsAreTypedTransferCash(t *testing.T) {
	for _, sc := range allShapes() {
		if sc.fromLedger != LedgerInvestment && sc.toLedger != LedgerInvestment {
			continue
		}
		t.Run(sc.name, func(t *testing.T) {
			h := newHarness(t)
			res, err := h.svc.Create(sc.spec(h, "250.00"))
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			for _, ref := range []LegRef{res.From, res.To} {
				if ref.Ledger != LedgerInvestment {
					continue
				}
				row, err := h.invRepo.GetByID(ref.RowID)
				if err != nil {
					t.Fatalf("load investment leg: %v", err)
				}
				if row.Type != investment.TransactionTypeTransferCash {
					t.Errorf("investment leg type = %q, want %q", row.Type, investment.TransactionTypeTransferCash)
				}
				if !row.Type.AffectsCash() {
					t.Errorf("investment leg type %q does not affect cash; the leg would vanish from the cash balance", row.Type)
				}
			}
		})
	}
}

// TestCreate_MovesCashBalanceOnBothSides is the end-to-end money assertion: the
// two legs must actually move the two accounts' balances by equal and opposite
// amounts.
func TestCreate_MovesCashBalanceOnBothSides(t *testing.T) {
	h := newHarness(t)
	amount := types.MustNewMoney("321.00")

	beforeBrokerage, err := h.invSvc.GetCashBalance(h.brokerage.ID)
	if err != nil {
		t.Fatalf("GetCashBalance(brokerage) before: %v", err)
	}
	beforeIRA, err := h.invSvc.GetCashBalance(h.ira.ID)
	if err != nil {
		t.Fatalf("GetCashBalance(ira) before: %v", err)
	}

	if _, err := h.svc.Create(Spec{
		FromAccountID: h.brokerage.ID,
		ToAccountID:   h.ira.ID,
		Date:          testDate(),
		Amount:        amount,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	afterBrokerage, err := h.invSvc.GetCashBalance(h.brokerage.ID)
	if err != nil {
		t.Fatalf("GetCashBalance(brokerage) after: %v", err)
	}
	afterIRA, err := h.invSvc.GetCashBalance(h.ira.ID)
	if err != nil {
		t.Fatalf("GetCashBalance(ira) after: %v", err)
	}

	if want := beforeBrokerage.Sub(amount); !afterBrokerage.Equal(want) {
		t.Errorf("brokerage cash = %s, want %s (fell by %s)", afterBrokerage, want, amount)
	}
	if want := beforeIRA.Add(amount); !afterIRA.Equal(want) {
		t.Errorf("IRA cash = %s, want %s (rose by %s)", afterIRA, want, amount)
	}
}

// TestCreate_CategoryLandsOnRegularLegsOnly pins where a category can live.
func TestCreate_CategoryLandsOnRegularLegsOnly(t *testing.T) {
	for _, sc := range allShapes() {
		if !sc.categorizable {
			continue
		}
		t.Run(sc.name, func(t *testing.T) {
			h := newHarness(t)
			cat := h.newCategory("Transfers Out", category.TypeExpense)

			spec := sc.spec(h, "150.00")
			spec.CategoryID = types.NullableID{ID: cat.ID, Valid: true}
			res, err := h.svc.Create(spec)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			got, err := h.svc.Get(res.TransferID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if !got.CategoryID.Valid || got.CategoryID.ID != cat.ID {
				t.Errorf("CategoryID = %+v, want %s", got.CategoryID, cat.ID)
			}
			for _, leg := range got.Legs() {
				if leg.Ledger == LedgerInvestment && leg.CategoryID.Valid {
					t.Error("an investment leg reported a category; that table has no category column")
				}
			}
		})
	}
}

// =============================================================================
// Guard matrix
// =============================================================================

func TestCreate_GuardMatrix(t *testing.T) {
	t.Run("non-positive amount", func(t *testing.T) {
		h := newHarness(t)
		for _, amt := range []string{"0.00", "-5.00"} {
			spec := Spec{FromAccountID: h.checking.ID, ToAccountID: h.savings.ID, Date: testDate(), Amount: types.MustNewMoney(amt)}
			_, err := h.svc.Create(spec)
			var target *InvalidAmountError
			if !errors.As(err, &target) {
				t.Errorf("Create(amount=%s) error = %T (%v), want *InvalidAmountError", amt, err, err)
			}
		}
	})

	t.Run("same account", func(t *testing.T) {
		h := newHarness(t)
		spec := Spec{FromAccountID: h.checking.ID, ToAccountID: h.checking.ID, Date: testDate(), Amount: types.MustNewMoney("10.00")}
		_, err := h.svc.Create(spec)
		var target *SameAccountError
		if !errors.As(err, &target) {
			t.Fatalf("error = %T (%v), want *SameAccountError", err, err)
		}
	})

	t.Run("missing account", func(t *testing.T) {
		h := newHarness(t)
		spec := Spec{FromAccountID: types.NewID(), ToAccountID: h.savings.ID, Date: testDate(), Amount: types.MustNewMoney("10.00")}
		if _, err := h.svc.Create(spec); err == nil {
			t.Fatal("expected an error for a missing from-account")
		}
	})

	// A closed account on EITHER side is refused, for every shape. Note that
	// inv↔inv already had this via requireInvestmentAccount, so unifying the
	// guard is not a silent behavior change for that path.
	t.Run("closed account either side", func(t *testing.T) {
		for _, sc := range allShapes() {
			for _, side := range []string{"from", "to"} {
				t.Run(sc.name+"/"+side, func(t *testing.T) {
					h := newHarness(t)
					victim := sc.fromOf(h)
					if side == "to" {
						victim = sc.toOf(h)
					}
					victim.Active = false
					if err := h.accountRepo.Update(victim); err != nil {
						t.Fatalf("close account: %v", err)
					}

					_, err := h.svc.Create(sc.spec(h, "10.00"))
					var target *account.AccountClosedError
					if !errors.As(err, &target) {
						t.Fatalf("error = %T (%v), want *account.AccountClosedError", err, err)
					}
				})
			}
		}
	})

	t.Run("date before opening date either side", func(t *testing.T) {
		for _, sc := range allShapes() {
			t.Run(sc.name, func(t *testing.T) {
				h := newHarness(t)
				spec := sc.spec(h, "10.00")
				spec.Date = types.NewDate(1999, time.January, 1) // before every fixture's opening date
				if _, err := h.svc.Create(spec); err == nil {
					t.Fatal("expected an opening-date error")
				}
			})
		}
	})

	t.Run("missing category", func(t *testing.T) {
		h := newHarness(t)
		spec := sc0(h, "10.00")
		spec.CategoryID = types.NullableID{ID: types.NewID(), Valid: true}
		if _, err := h.svc.Create(spec); err == nil {
			t.Fatal("expected a not-found error for an unknown category")
		}
	})

	// NEW ENFORCEMENT for the three investment paths: internal/investment does
	// no category validation at all and its comments delegate it to the caller.
	t.Run("system category refused", func(t *testing.T) {
		for _, sc := range allShapes() {
			if !sc.categorizable {
				continue
			}
			t.Run(sc.name, func(t *testing.T) {
				h := newHarness(t)
				sys := category.NewCategory("Transfer", category.TypeExpense)
				sys.IsSystem = true
				if err := h.categoryRepo.Create(sys); err != nil {
					t.Fatalf("create system category: %v", err)
				}

				spec := sc.spec(h, "10.00")
				spec.CategoryID = types.NullableID{ID: sys.ID, Valid: true}
				_, err := h.svc.Create(spec)
				var target *transaction.SystemCategoryTransferError
				if !errors.As(err, &target) {
					t.Fatalf("error = %T (%v), want *transaction.SystemCategoryTransferError", err, err)
				}
			})
		}
	})

	// The domain home for a refusal that lives in five presentation sites today
	// and nowhere in the domain.
	t.Run("category on inv-to-inv refused", func(t *testing.T) {
		h := newHarness(t)
		cat := h.newCategory("Rollover", category.TypeExpense)
		spec := Spec{
			FromAccountID: h.brokerage.ID, ToAccountID: h.ira.ID,
			Date: testDate(), Amount: types.MustNewMoney("10.00"),
			CategoryID: types.NullableID{ID: cat.ID, Valid: true},
		}
		_, err := h.svc.Create(spec)
		var target *CategoryNotSupportedError
		if !errors.As(err, &target) {
			t.Fatalf("error = %T (%v), want *CategoryNotSupportedError", err, err)
		}
		if target.Kind != KindInvToInv {
			t.Errorf("Kind = %v, want KindInvToInv", target.Kind)
		}
	})

	t.Run("invalid and void status refused", func(t *testing.T) {
		h := newHarness(t)
		spec := sc0(h, "10.00")
		spec.Status = transaction.Status("banana")
		if _, err := h.svc.Create(spec); err == nil {
			t.Error("expected an error for an invalid status")
		}
		spec.Status = transaction.StatusVoid
		if _, err := h.svc.Create(spec); err == nil {
			t.Error("expected an error for creating directly into void")
		}
	})
}

// sc0 is the reg↔reg spec, used by guard cases that are shape-independent.
func sc0(h *harness, amount string) Spec { return allShapes()[0].spec(h, amount) }

// =============================================================================
// Update / Reverse
// =============================================================================

// TestUpdate_AllFourShapes_EditsInPlace pins the identity-preserving property.
// investment.UpdateTransferCash deletes both legs and recreates them under a
// brand-new transfer_id, orphaning anything that referenced the old one.
func TestUpdate_AllFourShapes_EditsInPlace(t *testing.T) {
	for _, sc := range allShapes() {
		t.Run(sc.name, func(t *testing.T) {
			h := newHarness(t)
			created, err := h.svc.Create(sc.spec(h, "100.00"))
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			newDate := types.NewDate(2024, time.July, 4)
			res, err := h.svc.Update(created.TransferID, Edit{
				Date:   newDate,
				Amount: types.MustNewMoney("175.50"),
				Memo:   "edited",
				Status: transaction.StatusCleared,
			})
			if err != nil {
				t.Fatalf("Update: %v", err)
			}

			// Identity preserved: same transfer_id, same row IDs.
			if res.TransferID != created.TransferID {
				t.Errorf("transfer_id changed: %s -> %s", created.TransferID, res.TransferID)
			}
			if res.From.RowID != created.From.RowID || res.To.RowID != created.To.RowID {
				t.Errorf("row IDs changed: (%s,%s) -> (%s,%s)",
					created.From.RowID, created.To.RowID, res.From.RowID, res.To.RowID)
			}
			if res.Before == nil {
				t.Fatal("Before is nil; Update must carry the pre-edit state")
			}
			if !res.Before.Amount.Equal(types.MustNewMoney("100.00")) {
				t.Errorf("Before.Amount = %s, want 100.00", res.Before.Amount)
			}

			got, err := h.svc.Get(created.TransferID)
			if err != nil {
				t.Fatalf("Get after Update: %v", err)
			}
			assertTransfer(t, got, sc.wantKind, sc.fromOf(h).ID, sc.toOf(h).ID, "175.50")
			if got.Date != newDate {
				t.Errorf("Date = %v, want %v", got.Date, newDate)
			}
			if got.Memo != "edited" {
				t.Errorf("Memo = %q, want %q", got.Memo, "edited")
			}
			if got.Status != transaction.StatusCleared {
				t.Errorf("Status = %q, want cleared", got.Status)
			}
		})
	}
}

// TestReverse_AllFourShapes flips direction while keeping row identity.
func TestReverse_AllFourShapes(t *testing.T) {
	for _, sc := range allShapes() {
		t.Run(sc.name, func(t *testing.T) {
			h := newHarness(t)
			created, err := h.svc.Create(sc.spec(h, "60.00"))
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			res, err := h.svc.Reverse(created.TransferID)
			if err != nil {
				t.Fatalf("Reverse: %v", err)
			}
			if res.TransferID != created.TransferID {
				t.Errorf("transfer_id changed by Reverse")
			}

			got, err := h.svc.Get(created.TransferID)
			if err != nil {
				t.Fatalf("Get after Reverse: %v", err)
			}
			// Direction flipped: the original To account is now the From side.
			assertTransfer(t, got, ClassifyKind(sc.toOf(h).Type, sc.fromOf(h).Type),
				sc.toOf(h).ID, sc.fromOf(h).ID, "60.00")

			// The same two rows still hold the transfer.
			rows := map[types.ID]bool{created.From.RowID: true, created.To.RowID: true}
			for _, leg := range got.Legs() {
				if !rows[leg.RowID] {
					t.Errorf("Reverse replaced row %s instead of updating in place", leg.RowID)
				}
			}
		})
	}
}

// =============================================================================
// Status
// =============================================================================

func TestSetStatus_SetsBothLegs(t *testing.T) {
	for _, sc := range allShapes() {
		t.Run(sc.name, func(t *testing.T) {
			h := newHarness(t)
			created, err := h.svc.Create(sc.spec(h, "20.00"))
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if _, err := h.svc.SetStatus(created.TransferID, transaction.StatusCleared); err != nil {
				t.Fatalf("SetStatus: %v", err)
			}
			got, err := h.svc.Get(created.TransferID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			for _, leg := range got.Legs() {
				if leg.Status != transaction.StatusCleared {
					t.Errorf("leg %s status = %q, want cleared", leg.RowID, leg.Status)
				}
			}
		})
	}
}

// TestSetLegStatus_TouchesOneLegOnly is what the register's cleared toggle needs:
// clearing your side of a transfer says your bank posted it, independent of the
// other account.
func TestSetLegStatus_TouchesOneLegOnly(t *testing.T) {
	for _, sc := range allShapes() {
		t.Run(sc.name, func(t *testing.T) {
			h := newHarness(t)
			created, err := h.svc.Create(sc.spec(h, "30.00"))
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			if _, err := h.svc.SetLegStatus(created.From.RowID, transaction.StatusCleared); err != nil {
				t.Fatalf("SetLegStatus: %v", err)
			}

			got, err := h.svc.Get(created.TransferID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			for _, leg := range got.Legs() {
				want := transaction.StatusUncleared
				if leg.RowID == created.From.RowID {
					want = transaction.StatusCleared
				}
				if leg.Status != want {
					t.Errorf("leg %s status = %q, want %q", leg.RowID, leg.Status, want)
				}
			}
		})
	}
}

// =============================================================================
// Void / Restore
// =============================================================================

func TestVoid_RegToReg_ZeroesBothLegsAndRestoreBringsThemBack(t *testing.T) {
	h := newHarness(t)
	created, err := h.svc.Create(sc0(h, "500.00"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Snapshot before the void, RowID-addressed.
	before, err := h.svc.Get(created.TransferID)
	if err != nil {
		t.Fatalf("Get before void: %v", err)
	}
	snaps := make([]RestoreLeg, 0, 2)
	for _, leg := range before.Legs() {
		snaps = append(snaps, RestoreLeg{
			RowID:  leg.RowID,
			Amount: leg.Amount,
			Memo:   nullableMemo(leg.Memo),
			Status: leg.Status,
		})
	}

	if _, err := h.svc.Void(created.TransferID); err != nil {
		t.Fatalf("Void: %v", err)
	}

	voided, err := h.svc.Get(created.TransferID)
	if err != nil {
		t.Fatalf("Get after void: %v", err)
	}
	for _, leg := range voided.Legs() {
		if !leg.Amount.IsZero() {
			t.Errorf("voided leg %s amount = %s, want 0", leg.RowID, leg.Amount)
		}
		if leg.Status != transaction.StatusVoid {
			t.Errorf("voided leg %s status = %q, want void", leg.RowID, leg.Status)
		}
	}

	if _, err := h.svc.Restore(created.TransferID, snaps); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	restored, err := h.svc.Get(created.TransferID)
	if err != nil {
		t.Fatalf("Get after restore: %v", err)
	}
	assertTransfer(t, restored, KindRegToReg, h.checking.ID, h.savings.ID, "500.00")
	if restored.Status != transaction.StatusUncleared {
		t.Errorf("restored status = %q, want uncleared", restored.Status)
	}
}

// TestVoid_InvestmentInvolvingIsRefusedByName replaces the misleading
// "expected 2 transactions for transfer, found 1" a user gets today.
func TestVoid_InvestmentInvolvingIsRefusedByName(t *testing.T) {
	for _, sc := range allShapes() {
		if sc.wantKind == KindRegToReg {
			continue
		}
		t.Run(sc.name, func(t *testing.T) {
			h := newHarness(t)
			created, err := h.svc.Create(sc.spec(h, "40.00"))
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			_, err = h.svc.Void(created.TransferID)
			var target *VoidNotSupportedError
			if !errors.As(err, &target) {
				t.Fatalf("error = %T (%v), want *VoidNotSupportedError", err, err)
			}
		})
	}
}

// =============================================================================
// Delete / Recreate
// =============================================================================

func TestDelete_AllFourShapes(t *testing.T) {
	for _, sc := range allShapes() {
		t.Run(sc.name, func(t *testing.T) {
			h := newHarness(t)
			created, err := h.svc.Create(sc.spec(h, "80.00"))
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if _, err := h.svc.Delete(created.TransferID); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			if _, err := h.svc.Get(created.TransferID); err == nil {
				t.Fatal("Get after Delete returned no error; both legs should be gone")
			}
		})
	}
}

// TestRecreate_ReusesTheTransferID is what undo needs: the recreated transfer
// must be the same transfer, so a second undo step still addresses it.
func TestRecreate_ReusesTheTransferID(t *testing.T) {
	for _, sc := range allShapes() {
		t.Run(sc.name, func(t *testing.T) {
			h := newHarness(t)
			spec := sc.spec(h, "90.00")
			created, err := h.svc.Create(spec)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			transferID := created.TransferID
			if _, err := h.svc.Delete(transferID); err != nil {
				t.Fatalf("Delete: %v", err)
			}

			res, err := h.svc.Recreate(transferID, spec)
			if err != nil {
				t.Fatalf("Recreate: %v", err)
			}
			if res.TransferID != transferID {
				t.Errorf("Recreate minted a new transfer_id %s, want %s", res.TransferID, transferID)
			}
			got, err := h.svc.Get(transferID)
			if err != nil {
				t.Fatalf("Get after Recreate: %v", err)
			}
			assertTransfer(t, got, sc.wantKind, sc.fromOf(h).ID, sc.toOf(h).ID, "90.00")
		})
	}
}

// =============================================================================
// LinkExisting
// =============================================================================

// TestLinkExisting_LinksTwoUnrelatedRows covers transferlink's write.
func TestLinkExisting_LinksTwoUnrelatedRows(t *testing.T) {
	h := newHarness(t)
	out := transaction.NewTransaction(h.checking.ID, testDate(), types.MustNewMoney("-200.00"))
	if err := h.txnRepo.Create(out); err != nil {
		t.Fatalf("create out row: %v", err)
	}
	in := transaction.NewTransaction(h.savings.ID, testDate(), types.MustNewMoney("200.00"))
	if err := h.txnRepo.Create(in); err != nil {
		t.Fatalf("create in row: %v", err)
	}

	transferID, err := h.svc.LinkExisting(out.ID, in.ID, types.NullableID{})
	if err != nil {
		t.Fatalf("LinkExisting: %v", err)
	}

	got, err := h.svc.Get(transferID)
	if err != nil {
		t.Fatalf("Get after LinkExisting: %v", err)
	}
	assertTransfer(t, got, KindRegToReg, h.checking.ID, h.savings.ID, "200.00")
}

// TestLinkExisting_KeepsTodaysPermissiveSemantics pins the deliberate decision
// NOT to adopt Update's guards: transferlink links reconciled rows and rows
// dated before their account's opening date today, and must keep doing so.
// Tightening that here would be a silent capability reduction.
func TestLinkExisting_KeepsTodaysPermissiveSemantics(t *testing.T) {
	h := newHarness(t)
	out := transaction.NewTransaction(h.checking.ID, testDate(), types.MustNewMoney("-75.00"))
	out.Status = transaction.StatusReconciled
	if err := h.txnRepo.Create(out); err != nil {
		t.Fatalf("create reconciled out row: %v", err)
	}
	in := transaction.NewTransaction(h.savings.ID, testDate(), types.MustNewMoney("75.00"))
	in.Status = transaction.StatusReconciled
	if err := h.txnRepo.Create(in); err != nil {
		t.Fatalf("create reconciled in row: %v", err)
	}

	if _, err := h.svc.LinkExisting(out.ID, in.ID, types.NullableID{}); err != nil {
		t.Fatalf("LinkExisting refused reconciled rows: %v (transferlink links them today)", err)
	}
}

func TestLinkExisting_RefusesSameAccountAndSelf(t *testing.T) {
	h := newHarness(t)
	a := transaction.NewTransaction(h.checking.ID, testDate(), types.MustNewMoney("-10.00"))
	if err := h.txnRepo.Create(a); err != nil {
		t.Fatalf("create a: %v", err)
	}
	b := transaction.NewTransaction(h.checking.ID, testDate(), types.MustNewMoney("10.00"))
	if err := h.txnRepo.Create(b); err != nil {
		t.Fatalf("create b: %v", err)
	}

	if _, err := h.svc.LinkExisting(a.ID, a.ID, types.NullableID{}); err == nil {
		t.Error("linking a row to itself should fail")
	}
	_, err := h.svc.LinkExisting(a.ID, b.ID, types.NullableID{})
	var target *SameAccountError
	if !errors.As(err, &target) {
		t.Errorf("same-account link error = %T (%v), want *SameAccountError", err, err)
	}
}

// =============================================================================
// Refusals shared by every verb
// =============================================================================

// TestVerbs_RefuseSplitLines pins that every mutating verb refuses a transfer
// line inside a multi-line split, naming the parent. Reads still succeed.
func TestVerbs_RefuseSplitLines(t *testing.T) {
	h := newHarness(t)
	transferID, counterpartLeg, parentID := h.seedSplitTransferLine()

	assertSplitRefusal := func(t *testing.T, name string, err error) {
		t.Helper()
		var target *SplitLineError
		if !errors.As(err, &target) {
			t.Errorf("%s error = %T (%v), want *SplitLineError", name, err, err)
			return
		}
		if target.ParentID != parentID {
			t.Errorf("%s: ParentID = %s, want %s", name, target.ParentID, parentID)
		}
	}

	_, err := h.svc.Update(transferID, Edit{Date: testDate(), Amount: types.MustNewMoney("50.00")})
	assertSplitRefusal(t, "Update", err)

	_, err = h.svc.Reverse(transferID)
	assertSplitRefusal(t, "Reverse", err)

	_, err = h.svc.Delete(transferID)
	assertSplitRefusal(t, "Delete", err)

	_, err = h.svc.Void(transferID)
	assertSplitRefusal(t, "Void", err)

	_, err = h.svc.SetStatus(transferID, transaction.StatusCleared)
	assertSplitRefusal(t, "SetStatus", err)

	_, err = h.svc.SetLegStatus(counterpartLeg, transaction.StatusCleared)
	assertSplitRefusal(t, "SetLegStatus", err)
}

// TestVerbs_RefuseShareTransfers closes the corruption hole: today
// investment.UpdateTransferCash never checks the row type, so a cash edit handed
// a share transfer's (or a buy's) ID deletes rows via repo.Delete WITHOUT
// calling reverseTxnEffects, silently corrupting lots and positions.
func TestVerbs_RefuseShareTransfers(t *testing.T) {
	h := newHarness(t)
	transferID, srcLeg, _ := h.seedShareTransferPair()

	assertShareRefusal := func(t *testing.T, name string, err error) {
		t.Helper()
		var target *ShareTransferError
		if !errors.As(err, &target) {
			t.Errorf("%s error = %T (%v), want *ShareTransferError", name, err, err)
		}
	}

	_, err := h.svc.Update(transferID, Edit{Date: testDate(), Amount: types.MustNewMoney("50.00")})
	assertShareRefusal(t, "Update", err)

	_, err = h.svc.Reverse(transferID)
	assertShareRefusal(t, "Reverse", err)

	_, err = h.svc.Delete(transferID)
	assertShareRefusal(t, "Delete", err)

	_, err = h.svc.SetStatus(transferID, transaction.StatusCleared)
	assertShareRefusal(t, "SetStatus", err)

	_, err = h.svc.SetLegStatus(srcLeg, transaction.StatusCleared)
	assertShareRefusal(t, "SetLegStatus", err)
}

// TestVerbs_RefuseReconciledLegs is NEW ENFORCEMENT for every
// investment-involving shape: grep IsReconciled in investment_service.go returns
// exactly one hit, inside FindTransferCashCounterpart, so UpdateTransferCash and
// DeleteTransaction have no reconciled guard and the TUI can silently edit and
// delete reconciled investment transfers.
//
// Both legs are checked, so a HALF-reconciled pair is refused — the case that
// matters, since reconciliation reconciles one leg at a time.
func TestVerbs_RefuseReconciledLegs(t *testing.T) {
	for _, sc := range allShapes() {
		t.Run(sc.name, func(t *testing.T) {
			h := newHarness(t)
			created, err := h.svc.Create(sc.spec(h, "55.00"))
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			// Reconcile ONE leg only.
			if _, err := h.svc.SetLegStatus(created.From.RowID, transaction.StatusReconciled); err != nil {
				t.Fatalf("SetLegStatus(reconciled): %v", err)
			}

			for name, err := range map[string]error{
				"Update":  errOf(h.svc.Update(created.TransferID, Edit{Date: testDate(), Amount: types.MustNewMoney("1.00")})),
				"Reverse": errOf(h.svc.Reverse(created.TransferID)),
				"Delete":  errOf(h.svc.Delete(created.TransferID)),
			} {
				var target *ReconciledLegError
				if !errors.As(err, &target) {
					t.Errorf("%s error = %T (%v), want *ReconciledLegError", name, err, err)
				}
			}
		})
	}
}

func errOf(_ *Result, err error) error { return err }

// =============================================================================
// Atomicity
// =============================================================================

// failingQueryer wraps a real db.Queryer and errors on the Nth Exec, so a test
// can prove a mid-flow failure rolls the whole flow back rather than leaving one
// leg behind. Same shape as transaction_service_tx_test.go's.
type failingQueryer struct {
	inner  db.Queryer
	failOn int
	execN  int
}

func (f *failingQueryer) Exec(query string, args ...any) (sql.Result, error) {
	f.execN++
	if f.failOn != 0 && f.execN == f.failOn {
		return nil, fmt.Errorf("injected fault on exec #%d", f.execN)
	}
	return f.inner.Exec(query, args...)
}

func (f *failingQueryer) Query(query string, args ...any) (*sql.Rows, error) {
	return f.inner.Query(query, args...)
}

func (f *failingQueryer) QueryRow(query string, args ...any) *sql.Row {
	return f.inner.QueryRow(query, args...)
}

// TestCreate_FaultRollsBackBothLegs runs the injection across all four shapes.
// It proves not only that WithTx works — that is proven once in internal/db —
// but that no write ESCAPED the transaction, which is the regression that would
// silently reintroduce a half-written transfer.
func TestCreate_FaultRollsBackBothLegs(t *testing.T) {
	for _, sc := range allShapes() {
		t.Run(sc.name, func(t *testing.T) {
			h := newHarness(t)
			spec := sc.spec(h, "123.00")

			err := h.db.WithTx(func(tx db.Queryer) error {
				// Fail the second Exec: the from leg lands, the to leg errors,
				// so the whole transaction must roll back.
				fw := &failingQueryer{inner: tx, failOn: 2}
				_, e := h.svc.InTx(fw).Create(spec)
				return e
			})
			if err == nil {
				t.Fatal("expected the injected fault to surface")
			}

			// Neither ledger may hold a leg.
			regRows, err := h.txnRepo.ListByAccount(sc.fromOf(h).ID)
			if err != nil {
				t.Fatalf("list regular rows: %v", err)
			}
			for _, r := range regRows {
				if r.TransferID.Valid {
					t.Errorf("orphaned regular transfer leg survived rollback: %s", r.ID)
				}
			}
			for _, acctID := range []types.ID{sc.fromOf(h).ID, sc.toOf(h).ID} {
				invRows, err := h.invRepo.ListByAccount(acctID, investment.TransactionFilter{})
				if err != nil {
					t.Fatalf("list investment rows: %v", err)
				}
				for _, r := range invRows {
					if r.TransferID.Valid {
						t.Errorf("orphaned investment transfer leg survived rollback: %s", r.ID)
					}
				}
			}
		})
	}
}

// =============================================================================
// Differential: the new path against the one it replaces
// =============================================================================

// TestCreate_MatchesLegacyCreateTransfer is the differential test the design
// calls for. Both implementations write reg↔reg pairs until phase 5, and they
// must produce identical rows MODULO the freshly minted id, transfer_id and
// timestamps, which can never match across two invocations.
func TestCreate_MatchesLegacyCreateTransfer(t *testing.T) {
	h := newHarness(t)
	cat := h.newCategory("Bills", category.TypeExpense)
	catID := types.NullableID{ID: cat.ID, Valid: true}
	amount := types.MustNewMoney("212.34")

	legacyPair, err := h.txnSvc.CreateTransfer(h.checking.ID, h.savings.ID, testDate(), amount, "same memo", catID)
	if err != nil {
		t.Fatalf("legacy CreateTransfer: %v", err)
	}

	res, err := h.svc.Create(Spec{
		FromAccountID: h.checking.ID,
		ToAccountID:   h.savings.ID,
		Date:          testDate(),
		Amount:        amount,
		Memo:          "same memo",
		CategoryID:    catID,
	})
	if err != nil {
		t.Fatalf("transfer.Create: %v", err)
	}

	newFrom, err := h.txnRepo.GetByID(res.From.RowID)
	if err != nil {
		t.Fatalf("load new from-leg: %v", err)
	}
	newTo, err := h.txnRepo.GetByID(res.To.RowID)
	if err != nil {
		t.Fatalf("load new to-leg: %v", err)
	}

	compare := func(side string, legacy, fresh *transaction.Transaction) {
		t.Helper()
		if legacy.AccountID != fresh.AccountID {
			t.Errorf("%s account_id: legacy %s, new %s", side, legacy.AccountID, fresh.AccountID)
		}
		if !legacy.Amount.Equal(fresh.Amount) {
			t.Errorf("%s amount: legacy %s, new %s", side, legacy.Amount, fresh.Amount)
		}
		if legacy.Date != fresh.Date {
			t.Errorf("%s date: legacy %v, new %v", side, legacy.Date, fresh.Date)
		}
		if legacy.Status != fresh.Status {
			t.Errorf("%s status: legacy %q, new %q", side, legacy.Status, fresh.Status)
		}
		if legacy.Memo != fresh.Memo {
			t.Errorf("%s memo: legacy %+v, new %+v", side, legacy.Memo, fresh.Memo)
		}
		if legacy.CategoryID != fresh.CategoryID {
			t.Errorf("%s category_id: legacy %+v, new %+v", side, legacy.CategoryID, fresh.CategoryID)
		}
		if legacy.TransferAccountID != fresh.TransferAccountID {
			t.Errorf("%s transfer_account_id: legacy %+v, new %+v", side, legacy.TransferAccountID, fresh.TransferAccountID)
		}
		if legacy.PayeeID != fresh.PayeeID {
			t.Errorf("%s payee_id: legacy %+v, new %+v", side, legacy.PayeeID, fresh.PayeeID)
		}
		if legacy.CheckNumber != fresh.CheckNumber {
			t.Errorf("%s check_number: legacy %+v, new %+v", side, legacy.CheckNumber, fresh.CheckNumber)
		}
	}

	compare("from", legacyPair.FromTransaction, newFrom)
	compare("to", legacyPair.ToTransaction, newTo)
}
