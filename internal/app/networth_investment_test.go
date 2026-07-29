package app

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// TestNetWorth_InvestmentValuation_DoesNotBlock reproduces the TUI dashboard
// hang caused by SetMaxOpenConns(1): NetWorth iterates the account-balance
// rows (holding a pooled connection) and calls the REAL investment valuer
// inside that loop for investment accounts — the valuer's own queries need a
// second connection. With a one-connection pool the inner query waited
// forever; the report package's tests never saw it because they use a mock
// valuer. The watchdog fails fast instead of tripping the 10m test timeout.
func TestNetWorth_InvestmentValuation_DoesNotBlock(t *testing.T) {
	database := createTestDB(t)
	svc := NewServices(database)

	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD",
		types.MustNewMoney("0.00"), types.NewDate(2024, 1, 1))
	if err := svc.Account.Create(acct); err != nil {
		t.Fatalf("Create account: %v", err)
	}

	sec := security.NewSecurity("VTI", "Vanguard Total Market", security.TypeETF)
	if err := svc.Security.Create(sec); err != nil {
		t.Fatalf("Create security: %v", err)
	}

	if _, err := svc.Investment.Deposit(acct.ID, types.NewDate(2024, 1, 2),
		types.MustNewMoney("1000.00"), "seed cash"); err != nil {
		t.Fatalf("Deposit: %v", err)
	}

	total := types.MustNewMoney("500.00")
	if _, err := svc.Investment.Buy(acct.ID, sec.ID, types.NewDate(2024, 1, 3),
		types.MustNewQuantity("5"), &total, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := svc.Report.NetWorthReport()
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("NetWorthReport() error = %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("NetWorthReport() blocked: investment valuation inside the balance-row loop starved the connection pool")
	}
}
