package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

// SplitLot is a per-lot repair: it scales a single lot (shares, original_shares,
// cost) and recomputes the account position. It is sound only when the security
// has a recorded split (so position heal skips it and the scale survives), which
// is the scenario these tests set up: a global split, then a late-entered lot.
func TestCorporateActionService_SplitLot(t *testing.T) {
	env := createCATestEnv(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	preDate := types.NewDate(2020, time.January, 10)
	splitDate := types.NewDate(2020, time.August, 31)
	missedDate := types.NewDate(2020, time.February, 1)

	if _, err := env.invSvc.Deposit(acct.ID, preDate, types.MustNewMoney("100000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	// Original buy, then a global 2:1 split (records the split on the security).
	a := types.MustNewMoney("1000.00") // 10 @ $100 → 20 @ $50 after split
	if _, err := env.invSvc.Buy(acct.ID, sec.ID, preDate, types.MustNewQuantity("10"), &a, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() A error = %v", err)
	}
	if _, err := env.caSvc.Split(sec.ID, splitDate, SplitParams{Numerator: 2, Denominator: 1}); err != nil {
		t.Fatalf("Split() error = %v", err)
	}

	// A missed buy entered raw after the split: a 10-share lot the split missed.
	b := types.MustNewMoney("900.00") // 10 @ $90 (pre-split price)
	txnB, err := env.invSvc.Buy(acct.ID, sec.ID, missedDate, types.MustNewQuantity("10"), &b, nil, types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("Buy() B error = %v", err)
	}
	lotB, err := env.lotRepo.GetBySourceTransaction(txnB.ID)
	if err != nil {
		t.Fatalf("GetBySourceTransaction() error = %v", err)
	}

	updated, err := env.caSvc.SplitLot(lotB.ID, SplitParams{Numerator: 2, Denominator: 1})
	if err != nil {
		t.Fatalf("SplitLot() error = %v", err)
	}
	if !updated.Shares.Equal(types.MustNewQuantity("20")) {
		t.Errorf("shares = %s, want 20 (2:1)", updated.Shares.String())
	}
	if !updated.OriginalShares.Equal(types.MustNewQuantity("20")) {
		t.Errorf("original_shares = %s, want 20", updated.OriginalShares.String())
	}
	if !updated.CostPerShare.Equal(types.MustNewMoney("45")) {
		t.Errorf("cost/share = %s, want 45 (90/2)", updated.CostPerShare.String())
	}

	pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("GetByAccountAndSecurity() error = %v", err)
	}
	if !pos.Shares.Equal(types.MustNewQuantity("40")) {
		t.Errorf("position shares = %s, want 40 (20 + 20)", pos.Shares.String())
	}
}

// SplitLot refuses a lot that has already been sold against, since scaling it
// without scaling the dependent junction records would corrupt realized gain.
func TestCorporateActionService_SplitLot_RefusesConsumedLot(t *testing.T) {
	env := createCATestEnv(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2020, time.January, 10)
	splitDate := types.NewDate(2020, time.August, 31)

	if _, err := env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("100000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	total := types.MustNewMoney("1000.00")
	txn, err := env.invSvc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	if _, err := env.caSvc.Split(sec.ID, splitDate, SplitParams{Numerator: 2, Denominator: 1}); err != nil {
		t.Fatalf("Split() error = %v", err)
	}
	lot, err := env.lotRepo.GetBySourceTransaction(txn.ID) // now 20 shares
	if err != nil {
		t.Fatalf("GetBySourceTransaction() error = %v", err)
	}

	sellDate := types.NewDate(2021, time.February, 1)
	price := types.MustNewMoney("120")
	if _, err := env.invSvc.Sell(acct.ID, sec.ID, sellDate, types.MustNewQuantity("5"), nil, &price, types.ZeroMoney, "",
		[]SellLotAllocation{{LotID: lot.ID, Shares: types.MustNewQuantity("5")}}); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	if _, err := env.caSvc.SplitLot(lot.ID, SplitParams{Numerator: 2, Denominator: 1}); err == nil {
		t.Fatalf("SplitLot() on a consumed lot = nil error, want refusal")
	}
}

// SplitLot refuses when the security has no recorded split, because a per-lot
// scale would be reverted by the next position heal (ledger replay).
func TestCorporateActionService_SplitLot_RefusesWithoutSplitAction(t *testing.T) {
	env := createCATestEnv(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2020, time.January, 10)

	if _, err := env.invSvc.Deposit(acct.ID, date, types.MustNewMoney("100000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	total := types.MustNewMoney("1000.00")
	txn, err := env.invSvc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	lot, err := env.lotRepo.GetBySourceTransaction(txn.ID)
	if err != nil {
		t.Fatalf("GetBySourceTransaction() error = %v", err)
	}

	if _, err := env.caSvc.SplitLot(lot.ID, SplitParams{Numerator: 2, Denominator: 1}); err == nil {
		t.Fatalf("SplitLot() without a recorded split = nil error, want refusal")
	}
}

// CatchUpSplitsForLot brings a back-dated buy's raw lot into line with splits
// that already ran — the engine behind `buy --catch-up-splits`.
func TestCorporateActionService_CatchUpSplitsForLot(t *testing.T) {
	env := createCATestEnv(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	preDate := types.NewDate(2020, time.January, 10)
	splitDate := types.NewDate(2020, time.August, 31)
	missedDate := types.NewDate(2020, time.February, 1) // also pre-split, entered late

	if _, err := env.invSvc.Deposit(acct.ID, preDate, types.MustNewMoney("100000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	// Original buy, then a 2:1 split scales it to 20 @ $50.
	a := types.MustNewMoney("1000.00") // 10 @ $100
	if _, err := env.invSvc.Buy(acct.ID, sec.ID, preDate, types.MustNewQuantity("10"), &a, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() A error = %v", err)
	}
	if _, err := env.caSvc.Split(sec.ID, splitDate, SplitParams{Numerator: 2, Denominator: 1}); err != nil {
		t.Fatalf("Split() error = %v", err)
	}

	// The missed buy, entered after the split was recorded: a raw 10-share lot.
	b := types.MustNewMoney("900.00") // 10 @ $90 (pre-split price)
	txnB, err := env.invSvc.Buy(acct.ID, sec.ID, missedDate, types.MustNewQuantity("10"), &b, nil, types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("Buy() B error = %v", err)
	}
	lotB, err := env.lotRepo.GetBySourceTransaction(txnB.ID)
	if err != nil {
		t.Fatalf("GetBySourceTransaction() error = %v", err)
	}
	if !lotB.Shares.Equal(types.MustNewQuantity("10")) {
		t.Fatalf("pre-catch-up lot B shares = %s, want 10 (raw, unscaled)", lotB.Shares.String())
	}

	applied, err := env.caSvc.CatchUpSplitsForLot(lotB.ID)
	if err != nil {
		t.Fatalf("CatchUpSplitsForLot() error = %v", err)
	}
	if applied != 1 {
		t.Errorf("splits applied = %d, want 1", applied)
	}

	lotB, _ = env.lotRepo.GetBySourceTransaction(txnB.ID)
	if !lotB.Shares.Equal(types.MustNewQuantity("20")) {
		t.Errorf("lot B shares after catch-up = %s, want 20 (2:1 applied)", lotB.Shares.String())
	}
	if !lotB.OriginalShares.Equal(types.MustNewQuantity("20")) {
		t.Errorf("lot B original_shares after catch-up = %s, want 20", lotB.OriginalShares.String())
	}
	if !lotB.CostPerShare.Equal(types.MustNewMoney("45")) {
		t.Errorf("lot B cost/share after catch-up = %s, want 45 (90/2)", lotB.CostPerShare.String())
	}

	pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("GetByAccountAndSecurity() error = %v", err)
	}
	if !pos.Shares.Equal(types.MustNewQuantity("40")) {
		t.Errorf("position shares = %s, want 40 (20 + 20)", pos.Shares.String())
	}
}

// A buy dated on or after the split must NOT be caught up (it's already in
// post-split terms).
func TestCorporateActionService_CatchUpSplitsForLot_SkipsPostSplitLot(t *testing.T) {
	env := createCATestEnv(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	splitDate := types.NewDate(2020, time.August, 31)
	postDate := types.NewDate(2021, time.March, 1)

	if _, err := env.invSvc.Deposit(acct.ID, types.NewDate(2020, time.January, 1), types.MustNewMoney("100000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	if _, err := env.caSvc.Split(sec.ID, splitDate, SplitParams{Numerator: 2, Denominator: 1}); err != nil {
		t.Fatalf("Split() error = %v", err)
	}
	total := types.MustNewMoney("500.00") // 5 @ $100, already split-adjusted
	txn, err := env.invSvc.Buy(acct.ID, sec.ID, postDate, types.MustNewQuantity("5"), &total, nil, types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	lot, _ := env.lotRepo.GetBySourceTransaction(txn.ID)

	applied, err := env.caSvc.CatchUpSplitsForLot(lot.ID)
	if err != nil {
		t.Fatalf("CatchUpSplitsForLot() error = %v", err)
	}
	if applied != 0 {
		t.Errorf("splits applied = %d, want 0 (post-split lot)", applied)
	}

	lot, _ = env.lotRepo.GetBySourceTransaction(txn.ID)
	if !lot.Shares.Equal(types.MustNewQuantity("5")) {
		t.Errorf("post-split lot shares = %s, want 5 (must not be caught up)", lot.Shares.String())
	}
}
