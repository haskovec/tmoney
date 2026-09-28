package app

import (
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// NewServices only wires the graph. The seeds and heals run in Prepare.
func TestNewServices_WritesNothingUntilPrepare(t *testing.T) {
	svc := NewServices(createTestDB(t))

	if _, err := svc.Category.GetByName("Income", nil); err == nil {
		t.Fatal("NewServices seeded the paycheck categories; it must write nothing")
	}

	if err := svc.Prepare(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if _, err := svc.Category.GetByName("Income", nil); err != nil {
		t.Errorf("Prepare did not seed the paycheck categories: %v", err)
	}
}

// DamagedLotFile returns a file with two brokerages: "Broken Brokerage",
// whose only lot has more shares consumed than it ever held (so its rebuild
// fails), and "Healthy Brokerage", whose stored position was desynced to 99
// shares (so a working heal resets it to 5).
func damagedLotFile(t *testing.T) (*db.DB, *Services, *account.Account, types.ID) {
	t.Helper()
	database := createTestDB(t)
	svc := NewServices(database)
	date := types.NewDate(2024, time.January, 15)
	sec := security.NewSecurity("FABR", "Fabrikam Inc.", security.TypeStock)
	if err := svc.Security.Create(sec); err != nil {
		t.Fatal(err)
	}
	mk := func(name string, trackLots bool) *account.Account {
		acct := account.NewAccount(name, account.TypeInvestment, "USD", types.ZeroMoney, date)
		acct.TrackLots = trackLots
		if err := svc.Account.Create(acct); err != nil {
			t.Fatal(err)
		}
		total := types.MustNewMoney("500.00")
		if _, err := svc.Investment.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("5"), &total, nil, types.ZeroMoney, ""); err != nil {
			t.Fatal(err)
		}
		return acct
	}
	broken := mk("Broken Brokerage", true)
	healthy := mk("Healthy Brokerage", false)

	lots, err := svc.LotRepo.ListByAccountAndSecurity(broken.ID, sec.ID, false)
	if err != nil || len(lots) != 1 {
		t.Fatalf("lots = %d, %v; want 1", len(lots), err)
	}
	if _, err := database.Conn().Exec(
		`INSERT INTO investment_transaction_lots (transaction_id, lot_id, shares) VALUES (?, ?, 10)`,
		lots[0].SourceTransactionID.String(), lots[0].ID.String(),
	); err != nil {
		t.Fatalf("damage the lot: %v", err)
	}
	if _, err := database.Conn().Exec(
		`UPDATE investment_positions SET shares = 99 WHERE CAST(account_id AS VARCHAR) = ?`, healthy.ID.String(),
	); err != nil {
		t.Fatalf("desync the position: %v", err)
	}
	return database, svc, healthy, sec.ID
}

// Prepare returns a heal failure instead of dropping it, and the failure of
// one account does not stop the others from healing.
func TestPrepare_ReportsHealFailureAndHealsTheRest(t *testing.T) {
	_, svc, healthy, secID := damagedLotFile(t)

	err := svc.Prepare()
	if err == nil {
		t.Fatal("Prepare() = nil, want the failed heal")
	}
	if !strings.Contains(err.Error(), `account "Broken Brokerage"`) {
		t.Errorf("Prepare() error = %v, want it to name the broken account", err)
	}

	pos, perr := svc.PositionRepo.GetByAccountAndSecurity(healthy.ID, secID)
	if perr != nil {
		t.Fatal(perr)
	}
	if !pos.Shares.Equal(types.MustNewQuantity("5")) {
		t.Errorf("healthy position = %s shares, want 5: the failure stopped the other heals", pos.Shares)
	}
	if _, cerr := svc.Category.GetByName("Income", nil); cerr != nil {
		t.Errorf("a failed heal stopped the category seed: %v", cerr)
	}
}
