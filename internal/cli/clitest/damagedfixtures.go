package clitest

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// DamagedHealFile creates a file whose open-time repair fails: in "Broken
// Brokerage" (lot-tracked) the only lot has more shares consumed than it ever
// held, so rebuilding its positions fails. "Healthy Brokerage" holds 5 shares
// and rebuilds normally. It returns the file path, closed.
func DamagedHealFile(t *testing.T) string {
	t.Helper()
	database, path := dbtest.NewFile(t, "damaged.tdb")
	svc := app.NewServices(database)
	date := types.NewDate(2024, time.January, 15)
	sec := security.NewSecurity("FABR", "Fabrikam Inc.", security.TypeStock)
	if err := svc.Security.Create(sec); err != nil {
		t.Fatalf("DamagedHealFile: %v", err)
	}
	for _, spec := range []struct {
		name      string
		trackLots bool
	}{{"Broken Brokerage", true}, {"Healthy Brokerage", false}} {
		acct := account.NewAccount(spec.name, account.TypeInvestment, "USD", types.ZeroMoney, date)
		acct.TrackLots = spec.trackLots
		if err := svc.Account.Create(acct); err != nil {
			t.Fatalf("DamagedHealFile: %v", err)
		}
		total := types.MustNewMoney("500.00")
		if _, err := svc.Investment.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("5"), &total, nil, types.ZeroMoney, ""); err != nil {
			t.Fatalf("DamagedHealFile: %v", err)
		}
		if !spec.trackLots {
			continue
		}
		lots, err := svc.LotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if err != nil || len(lots) != 1 {
			t.Fatalf("DamagedHealFile: lots = %d, %v", len(lots), err)
		}
		if _, err := database.Conn().Exec(
			`INSERT INTO investment_transaction_lots (transaction_id, lot_id, shares) VALUES (?, ?, 10)`,
			lots[0].SourceTransactionID.String(), lots[0].ID.String(),
		); err != nil {
			t.Fatalf("DamagedHealFile: %v", err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatalf("DamagedHealFile: %v", err)
	}
	return path
}
