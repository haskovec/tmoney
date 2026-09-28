package reconciliation

import (
	"errors"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// Reconcile compares a statement with the register ledger. An investment
// account keeps its cash and shares on the investment ledger, so its
// candidates are empty and a finished reconcile would claim a match it never
// checked. Start refuses it.
func TestService_StartReconciliation_RefusesInvestmentAccounts(t *testing.T) {
	for _, typ := range []account.Type{account.TypeInvestment, account.TypeHSAInvestment} {
		t.Run(string(typ), func(t *testing.T) {
			svc, _, accountRepo := createTestReconciliationService(t)
			acct := account.NewAccount("Northwind Brokerage", typ, "USD", types.ZeroMoney, types.NewDate(2024, 1, 1))
			if err := accountRepo.Create(acct); err != nil {
				t.Fatal(err)
			}

			_, err := svc.StartReconciliation(acct.ID, types.NewDate(2024, 2, 1), types.MustNewMoney("100.00"))
			var invErr *InvestmentAccountError
			if !errors.As(err, &invErr) {
				t.Fatalf("StartReconciliation() error = %v, want InvestmentAccountError", err)
			}
			if s, err := svc.GetActiveSession(acct.ID); err != nil || s != nil {
				t.Errorf("a session was created: %v, %v", s, err)
			}
		})
	}

	// HSA cash is a register account and still reconciles.
	for _, typ := range []account.Type{account.TypeChecking, account.TypeHSA} {
		t.Run(string(typ)+" still starts", func(t *testing.T) {
			svc, _, accountRepo := createTestReconciliationService(t)
			acct := account.NewAccount("Contoso Checking", typ, "USD", types.ZeroMoney, types.NewDate(2024, 1, 1))
			if err := accountRepo.Create(acct); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.StartReconciliation(acct.ID, types.NewDate(2024, 2, 1), types.ZeroMoney); err != nil {
				t.Fatalf("StartReconciliation() error = %v", err)
			}
		})
	}
}
