package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

func TestHoldingsRepository_ListByAccount(t *testing.T) {
	t.Run("returns holdings for non-lot-tracking account", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		total := types.MustNewMoney("1000")
		_, err = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		holdingsRepo := NewHoldingsRepository(env.svc.db)
		holdings, err := holdingsRepo.ListByAccount(acct.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}

		if len(holdings) != 1 {
			t.Fatalf("Expected 1 holding, got %d", len(holdings))
		}

		h := holdings[0]
		if h.SecurityID != sec.ID {
			t.Errorf("Expected security ID %s, got %s", sec.ID, h.SecurityID)
		}
		if h.TotalShares.String() != "10" {
			t.Errorf("Expected total shares '10', got %q", h.TotalShares.String())
		}
		if h.TotalCostBasis.String() != "1000" {
			t.Errorf("Expected total cost basis '1000', got %q", h.TotalCostBasis.String())
		}
	})

	t.Run("returns holdings for lot-tracking account", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "LotBroker")
		sec := createSec(t, env.secRepo, "GOOG")
		date := types.NewDate(2024, time.March, 15)

		_, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("20000"), "")
		if err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		// Lot 1: 5 shares at $200
		total1 := types.MustNewMoney("1000")
		_, err = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("5"), &total1, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy lot 1 error = %v", err)
		}

		// Lot 2: 3 shares at $210
		date2 := types.NewDate(2024, time.March, 18)
		total2 := types.MustNewMoney("630")
		_, err = env.svc.Buy(acct.ID, sec.ID, date2, types.MustNewQuantity("3"), &total2, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy lot 2 error = %v", err)
		}

		holdingsRepo := NewHoldingsRepository(env.svc.db)
		holdings, err := holdingsRepo.ListByAccount(acct.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}

		if len(holdings) != 1 {
			t.Fatalf("Expected 1 holding, got %d", len(holdings))
		}

		h := holdings[0]
		// Total shares: 5 + 3 = 8
		if h.TotalShares.String() != "8" {
			t.Errorf("Expected total shares '8', got %q", h.TotalShares.String())
		}
		// Cost basis: 1000 + 630 = 1630
		if h.TotalCostBasis.String() != "1630" {
			t.Errorf("Expected total cost basis '1630', got %q", h.TotalCostBasis.String())
		}
	})

	t.Run("excludes zero-share positions", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "EmptyBroker")
		_ = createSec(t, env.secRepo, "ZERO")

		holdingsRepo := NewHoldingsRepository(env.svc.db)
		holdings, err := holdingsRepo.ListByAccount(acct.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}

		if len(holdings) != 0 {
			t.Errorf("Expected 0 holdings, got %d", len(holdings))
		}
	})

	t.Run("returns empty for non-investment account", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createCheckAccount(t, env.accountRepo, "Checking")

		holdingsRepo := NewHoldingsRepository(env.svc.db)
		holdings, err := holdingsRepo.ListByAccount(acct.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}

		if len(holdings) != 0 {
			t.Errorf("Expected 0 holdings for non-investment account, got %d", len(holdings))
		}
	})
}
