package integration

import (
	"errors"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/types"
)

// This file used to exercise transaction.Service's whole-transfer API —
// CreateTransfer, GetTransferPair, GetTransferCounterpart, UpdateTransfer{,Amount,
// Date,Status}, DeleteTransfer, IsTransfer. All of it was deleted in phase 5 of
// specs/design-unified-transfer.md and replaced by transfer.Service, whose own
// suite covers every verb across all four (From, To) ledger combinations far more
// thoroughly than these bank↔bank-only tests did.
//
// What is kept here is what internal/transfer's suite structurally CANNOT cover:
// it wires its services by hand, so it cannot catch a composition-root mistake.
// These tests go through app.NewServices, so they fail if Services.Transfer is
// unwired, wired to the wrong repositories, or wired in an order that leaves a
// collaborator nil.

// openTransferServices opens a scratch database and builds the real service graph.
func openTransferServices(t *testing.T) (*app.Services, *db.DB) {
	t.Helper()
	database := dbtest.New(t)
	return app.NewServices(database), database
}

func makeTransferAccount(t *testing.T, svc *app.Services, name string, at account.Type) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, at, "USD", types.MustNewMoney("1000.00"), types.NewDate(2024, 1, 1))
	if err := svc.AccountRepo.Create(acct); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return acct
}

// TestTransferWiring_AllFourShapesThroughTheCompositionRoot is the integration
// assertion that matters: every shape round-trips through the service graph that
// app.NewServices actually builds.
func TestTransferWiring_AllFourShapesThroughTheCompositionRoot(t *testing.T) {
	shapes := []struct {
		name     string
		fromType account.Type
		toType   account.Type
		wantKind transfer.Kind
	}{
		{"reg-to-reg", account.TypeChecking, account.TypeSavings, transfer.KindRegToReg},
		{"inv-to-reg", account.TypeInvestment, account.TypeChecking, transfer.KindInvToReg},
		{"reg-to-inv", account.TypeChecking, account.TypeInvestment, transfer.KindRegToInv},
		{"inv-to-inv", account.TypeInvestment, account.TypeHSA, transfer.KindInvToInv},
	}

	for _, sh := range shapes {
		t.Run(sh.name, func(t *testing.T) {
			svc, _ := openTransferServices(t)
			if svc.Transfer == nil {
				t.Fatal("Services.Transfer is nil; the composition root did not wire the transfer owner")
			}
			from := makeTransferAccount(t, svc, "From "+sh.name, sh.fromType)
			to := makeTransferAccount(t, svc, "To "+sh.name, sh.toType)

			amount := types.MustNewMoney("250.00")
			res, err := svc.Transfer.Create(transfer.Spec{
				FromAccountID: from.ID,
				ToAccountID:   to.ID,
				Date:          types.NewDate(2024, 6, 1),
				Amount:        amount,
				Memo:          "integration",
			})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if res.Kind != sh.wantKind {
				t.Errorf("Kind = %v, want %v", res.Kind, sh.wantKind)
			}

			// Reading back through the same graph resolves from either leg.
			for _, legID := range []types.ID{res.From.RowID, res.To.RowID} {
				got, err := svc.Transfer.Resolve(legID)
				if err != nil {
					t.Fatalf("Resolve(%s): %v", legID, err)
				}
				if got.TransferID != res.TransferID {
					t.Errorf("Resolve gave transfer %s, want %s", got.TransferID, res.TransferID)
				}
				if got.From.AccountID != from.ID || got.To.AccountID != to.ID {
					t.Errorf("direction = %s->%s, want %s->%s",
						got.From.AccountID, got.To.AccountID, from.ID, to.ID)
				}
				if !got.Amount.Equal(amount) {
					t.Errorf("Amount = %s, want %s", got.Amount, amount)
				}
			}

			// Edit and delete both work through the wired graph.
			if _, err := svc.Transfer.Update(res.TransferID, transfer.Edit{
				Date:   types.NewDate(2024, 6, 2),
				Amount: types.MustNewMoney("275.00"),
				Memo:   "edited",
			}); err != nil {
				t.Fatalf("Update: %v", err)
			}
			edited, err := svc.Transfer.Get(res.TransferID)
			if err != nil {
				t.Fatalf("Get after Update: %v", err)
			}
			if !edited.Amount.Equal(types.MustNewMoney("275.00")) {
				t.Errorf("edited amount = %s, want 275.00", edited.Amount)
			}
			if _, err := svc.Transfer.Delete(res.TransferID); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			if _, err := svc.Transfer.Get(res.TransferID); err == nil {
				t.Error("transfer still readable after Delete")
			}
		})
	}
}

// TestTransferWiring_InvestmentLegLandsInTheInvestmentLedger proves the two
// repositories are wired to the same database and that an investment leg really
// is written as a cash-affecting investment row — the failure mode that would
// otherwise show up only as a wrong balance.
func TestTransferWiring_InvestmentLegLandsInTheInvestmentLedger(t *testing.T) {
	svc, _ := openTransferServices(t)
	checking := makeTransferAccount(t, svc, "Checking", account.TypeChecking)
	brokerage := makeTransferAccount(t, svc, "Brokerage", account.TypeInvestment)

	res, err := svc.Transfer.Create(transfer.Spec{
		FromAccountID: checking.ID,
		ToAccountID:   brokerage.ID,
		Date:          types.NewDate(2024, 6, 1),
		Amount:        types.MustNewMoney("400.00"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	invRows, err := svc.InvestmentRepo.ListByAccount(brokerage.ID, investment.TransactionFilter{})
	if err != nil {
		t.Fatalf("list investment rows: %v", err)
	}
	if len(invRows) != 1 {
		t.Fatalf("expected 1 investment row, got %d", len(invRows))
	}
	if invRows[0].Type != investment.TransactionTypeTransferCash {
		t.Errorf("investment leg type = %q, want transfer_cash", invRows[0].Type)
	}
	if invRows[0].ID != res.To.RowID {
		t.Errorf("investment row %s is not the reported To leg %s", invRows[0].ID, res.To.RowID)
	}

	cash, err := svc.Investment.GetCashBalance(brokerage.ID)
	if err != nil {
		t.Fatalf("GetCashBalance: %v", err)
	}
	if !cash.Equal(types.MustNewMoney("400.00")) {
		t.Errorf("brokerage cash = %s, want 400.00", cash)
	}
}

// TestTransferWiring_PlainVerbsRefuseTransferLegs pins the phase-5 refusals at
// the integration level: the plain-transaction verbs write ONE row, so they must
// not touch a transfer leg whose counterpart may be in the other ledger.
func TestTransferWiring_PlainVerbsRefuseTransferLegs(t *testing.T) {
	svc, _ := openTransferServices(t)
	checking := makeTransferAccount(t, svc, "Checking", account.TypeChecking)
	savings := makeTransferAccount(t, svc, "Savings", account.TypeSavings)

	res, err := svc.Transfer.Create(transfer.Spec{
		FromAccountID: checking.ID,
		ToAccountID:   savings.ID,
		Date:          types.NewDate(2024, 6, 1),
		Amount:        types.MustNewMoney("90.00"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	leg, err := svc.TransactionRepo.GetByID(res.From.RowID)
	if err != nil {
		t.Fatalf("load leg: %v", err)
	}

	var target *transaction.IsTransferError

	if err := svc.Transaction.Delete(leg.ID); !errors.As(err, &target) {
		t.Errorf("Delete(transfer leg) = %T (%v), want *transaction.IsTransferError", err, err)
	}
	if err := svc.Transaction.VoidTransaction(leg.ID); !errors.As(err, &target) {
		t.Errorf("VoidTransaction(transfer leg) = %T (%v), want *transaction.IsTransferError", err, err)
	}
	edited := *leg
	edited.SetMemo("poke")
	if err := svc.Transaction.Update(&edited); !errors.As(err, &target) {
		t.Errorf("Update(transfer leg) = %T (%v), want *transaction.IsTransferError", err, err)
	}

	// The transfer is untouched by the refused attempts.
	still, err := svc.Transfer.Get(res.TransferID)
	if err != nil {
		t.Fatalf("Get after refusals: %v", err)
	}
	if !still.Amount.Equal(types.MustNewMoney("90.00")) {
		t.Errorf("amount = %s, want 90.00 (refused verbs must not have written)", still.Amount)
	}
}
