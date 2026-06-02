package investment

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

func TestParseLotMethod(t *testing.T) {
	cases := []struct {
		in      string
		want    LotMethod
		wantErr bool
	}{
		{"", LotMethodFIFO, false},
		{"fifo", LotMethodFIFO, false},
		{"FIFO", LotMethodFIFO, false},
		{" lifo ", LotMethodLIFO, false},
		{"hifo", LotMethodHIFO, false},
		{"bogus", LotMethodFIFO, true},
	}
	for _, c := range cases {
		got, err := ParseLotMethod(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseLotMethod(%q) err = %v, wantErr %v", c.in, err, c.wantErr)
		}
		if err == nil && got != c.want {
			t.Errorf("ParseLotMethod(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// findLotByCost returns the first plan lot whose cost-per-share matches want.
func findLotByCost(t *testing.T, plan *BackfillPlan, want string) *Lot {
	t.Helper()
	for _, l := range plan.Lots {
		if l.CostPerShare.String() == want {
			return l
		}
	}
	t.Fatalf("no lot with cost-per-share %q (have %d lots)", want, len(plan.Lots))
	return nil
}

func TestPlanLotBackfill_FIFO(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage") // non-lot
	sec := createSec(t, env.secRepo, "VTI")

	p1 := types.MustNewMoney("100.00")
	p2 := types.MustNewMoney("120.00")
	if _, err := env.svc.Buy(acct.ID, sec.ID, types.NewDate(2024, time.January, 2), types.MustNewQuantity("10"), nil, &p1, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy 1: %v", err)
	}
	if _, err := env.svc.Buy(acct.ID, sec.ID, types.NewDate(2024, time.February, 2), types.MustNewQuantity("5"), nil, &p2, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy 2: %v", err)
	}
	sp := types.MustNewMoney("130.00")
	if _, err := env.svc.Sell(acct.ID, sec.ID, types.NewDate(2024, time.March, 2), types.MustNewQuantity("4"), nil, &sp, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell: %v", err)
	}

	plan, err := env.svc.PlanLotBackfill(acct.ID, LotMethodFIFO)
	if err != nil {
		t.Fatalf("PlanLotBackfill: %v", err)
	}
	if len(plan.Lots) != 2 {
		t.Fatalf("want 2 lots, got %d", len(plan.Lots))
	}
	if len(plan.Shortfalls) != 0 {
		t.Fatalf("want 0 shortfalls, got %d", len(plan.Shortfalls))
	}
	// FIFO consumes the oldest ($100) lot first.
	oldest := findLotByCost(t, plan, "100")
	newest := findLotByCost(t, plan, "120")
	if oldest.Shares.String() != "6" {
		t.Errorf("oldest lot remaining = %s, want 6", oldest.Shares.String())
	}
	if oldest.OriginalShares.String() != "10" {
		t.Errorf("oldest lot original = %s, want 10", oldest.OriginalShares.String())
	}
	if newest.Shares.String() != "5" {
		t.Errorf("newest lot remaining = %s, want 5 (untouched)", newest.Shares.String())
	}
	if oldest.Closed || newest.Closed {
		t.Error("no lot should be closed")
	}
	if len(plan.Junctions) != 1 {
		t.Fatalf("want 1 junction, got %d", len(plan.Junctions))
	}
	j := plan.Junctions[0]
	if j.LotID != oldest.ID {
		t.Error("junction should reference the oldest (FIFO) lot")
	}
	if j.Shares.String() != "4" {
		t.Errorf("junction shares = %s, want 4", j.Shares.String())
	}
}

func TestPlanLotBackfill_Methods(t *testing.T) {
	// Costs deliberately non-monotonic with date so FIFO/LIFO/HIFO each pick a
	// different lot: 10@100 (Jan), 5@150 (Feb), 8@90 (Mar); sell 4 in April.
	cases := []struct {
		method   LotMethod
		wantCost string // cost-per-share of the lot the sell should consume from
	}{
		{LotMethodFIFO, "100"}, // oldest
		{LotMethodLIFO, "90"},  // newest
		{LotMethodHIFO, "150"}, // most expensive
	}
	for _, c := range cases {
		t.Run(c.method.String(), func(t *testing.T) {
			env := createFullTestService(t)
			acct := createInvAccount(t, env.accountRepo, "Brokerage")
			sec := createSec(t, env.secRepo, "VEA")
			pJan := types.MustNewMoney("100.00")
			pFeb := types.MustNewMoney("150.00")
			pMar := types.MustNewMoney("90.00")
			mustBuy(t, env, acct.ID, sec.ID, types.NewDate(2024, time.January, 2), "10", &pJan)
			mustBuy(t, env, acct.ID, sec.ID, types.NewDate(2024, time.February, 2), "5", &pFeb)
			mustBuy(t, env, acct.ID, sec.ID, types.NewDate(2024, time.March, 2), "8", &pMar)
			sp := types.MustNewMoney("200.00")
			if _, err := env.svc.Sell(acct.ID, sec.ID, types.NewDate(2024, time.April, 2), types.MustNewQuantity("4"), nil, &sp, types.ZeroMoney, "", nil); err != nil {
				t.Fatalf("Sell: %v", err)
			}

			plan, err := env.svc.PlanLotBackfill(acct.ID, c.method)
			if err != nil {
				t.Fatalf("PlanLotBackfill: %v", err)
			}
			if len(plan.Junctions) != 1 {
				t.Fatalf("want 1 junction, got %d", len(plan.Junctions))
			}
			consumed := findLotByCost(t, plan, c.wantCost)
			if plan.Junctions[0].LotID != consumed.ID {
				t.Errorf("%s: junction should consume the %s lot", c.method, c.wantCost)
			}
		})
	}
}

func TestPlanLotBackfill_TransferIn(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Wealthfront IRA")
	sec := createSec(t, env.secRepo, "NEM")

	// Inbound transfer_shares carries its basis on total_amount (price NULL),
	// mirroring the real ledger (NEM 200 shares @ $9623 total).
	txn := NewTransactionWithSecurity(acct.ID, types.NewDate(2020, time.January, 2),
		TransactionTypeTransferShares, types.MustNewMoney("9623.00"), sec.ID, types.MustNewQuantity("200"))
	if err := env.invRepo.Create(txn); err != nil {
		t.Fatalf("Create transfer_shares: %v", err)
	}

	plan, err := env.svc.PlanLotBackfill(acct.ID, LotMethodFIFO)
	if err != nil {
		t.Fatalf("PlanLotBackfill: %v", err)
	}
	if len(plan.Lots) != 1 {
		t.Fatalf("want 1 lot, got %d", len(plan.Lots))
	}
	lot := plan.Lots[0]
	if lot.Shares.String() != "200" {
		t.Errorf("lot shares = %s, want 200", lot.Shares.String())
	}
	// Basis carried from total_amount: 9623 / 200 = 48.115 per share.
	if lot.CostBasis().String() != "9623" {
		t.Errorf("lot cost basis = %s, want 9623", lot.CostBasis().String())
	}
	if lot.PurchaseDate.Time().Year() != 2020 {
		t.Errorf("lot purchase date year = %d, want 2020", lot.PurchaseDate.Time().Year())
	}
}

func TestPlanLotBackfill_Shortfall(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "GLD")

	// A sell with no preceding buy (e.g. imported out of order). Created
	// directly so the position-level guard doesn't reject it.
	txn := NewTransactionWithSecurity(acct.ID, types.NewDate(2021, time.June, 1),
		TransactionTypeSell, types.MustNewMoney("500.00"), sec.ID, types.MustNewQuantity("10"))
	txn.SetPricePerShare(types.MustNewMoney("50.00"))
	if err := env.invRepo.Create(txn); err != nil {
		t.Fatalf("Create sell: %v", err)
	}

	plan, err := env.svc.PlanLotBackfill(acct.ID, LotMethodFIFO)
	if err != nil {
		t.Fatalf("PlanLotBackfill (should not error on shortfall): %v", err)
	}
	if len(plan.Lots) != 0 || len(plan.Junctions) != 0 {
		t.Fatalf("want 0 lots/junctions, got %d/%d", len(plan.Lots), len(plan.Junctions))
	}
	if len(plan.Shortfalls) != 1 {
		t.Fatalf("want 1 shortfall, got %d", len(plan.Shortfalls))
	}
	sf := plan.Shortfalls[0]
	if sf.Requested.String() != "10" || sf.Covered.String() != "0" {
		t.Errorf("shortfall requested/covered = %s/%s, want 10/0", sf.Requested.String(), sf.Covered.String())
	}
}

func TestApplyLotBackfill(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "QQQ")

	p1 := types.MustNewMoney("100.00")
	p2 := types.MustNewMoney("120.00")
	mustBuy(t, env, acct.ID, sec.ID, types.NewDate(2024, time.January, 2), "10", &p1)
	mustBuy(t, env, acct.ID, sec.ID, types.NewDate(2024, time.February, 2), "5", &p2)
	sp := types.MustNewMoney("130.00")
	if _, err := env.svc.Sell(acct.ID, sec.ID, types.NewDate(2024, time.March, 2), types.MustNewQuantity("4"), nil, &sp, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell: %v", err)
	}

	plan, err := env.svc.PlanLotBackfill(acct.ID, LotMethodFIFO)
	if err != nil {
		t.Fatalf("PlanLotBackfill: %v", err)
	}
	if err := env.svc.ApplyLotBackfill(plan); err != nil {
		t.Fatalf("ApplyLotBackfill: %v", err)
	}

	lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, true)
	if err != nil {
		t.Fatalf("ListByAccountAndSecurity: %v", err)
	}
	if len(lots) != 2 {
		t.Fatalf("want 2 persisted lots, got %d", len(lots))
	}
	// Net shares across lots = 11 (10 + 5 - 4).
	total := types.ZeroQuantity
	lotIDs := make([]types.ID, 0, len(lots))
	for _, l := range lots {
		total = total.Add(l.Shares)
		lotIDs = append(lotIDs, l.ID)
	}
	if total.String() != "11" {
		t.Errorf("total persisted lot shares = %s, want 11", total.String())
	}
	consumed, err := env.transactionLotRepo.SumSharesByLot(lotIDs)
	if err != nil {
		t.Fatalf("SumSharesByLot: %v", err)
	}
	totalConsumed := types.ZeroQuantity
	for _, q := range consumed {
		totalConsumed = totalConsumed.Add(q)
	}
	if totalConsumed.String() != "4" {
		t.Errorf("total consumed via junctions = %s, want 4", totalConsumed.String())
	}
}

func TestAccountBackfillBlockers(t *testing.T) {
	env := createFullTestService(t)
	brokerage := createInvAccount(t, env.accountRepo, "Brokerage")
	ira := createInvAccount(t, env.accountRepo, "IRA")
	schb := createSec(t, env.secRepo, "SCHB")
	vti := createSec(t, env.secRepo, "VTI")

	// Brokerage holds SCHB; IRA holds VTI.
	pSchb := types.MustNewMoney("50.00")
	mustBuy(t, env, brokerage.ID, schb.ID, types.NewDate(2022, time.January, 3), "47", &pSchb)
	pVti := types.MustNewMoney("200.00")
	mustBuy(t, env, ira.ID, vti.ID, types.NewDate(2022, time.January, 3), "100", &pVti)

	// A 2-for-1 split on SCHB — held only in Brokerage.
	ca := NewCorporateAction(ActionTypeSplit, schb.ID, types.NewDate(2022, time.March, 11), `{"numerator":2,"denominator":1}`)
	if err := env.caRepo.Create(ca); err != nil {
		t.Fatalf("create corporate action: %v", err)
	}

	// Brokerage is blocked by the SCHB split.
	blockers, err := env.svc.AccountBackfillBlockers(brokerage.ID)
	if err != nil {
		t.Fatalf("AccountBackfillBlockers(brokerage): %v", err)
	}
	if len(blockers) != 1 || blockers[0].SecurityID != schb.ID {
		t.Fatalf("brokerage blockers = %+v, want exactly SCHB", blockers)
	}

	// The IRA holds no corporate-action security, so it is NOT blocked even
	// though a split exists elsewhere in the file (the SCHB-in-Brokerage vs
	// clean-IRA scenario).
	iraBlockers, err := env.svc.AccountBackfillBlockers(ira.ID)
	if err != nil {
		t.Fatalf("AccountBackfillBlockers(ira): %v", err)
	}
	if len(iraBlockers) != 0 {
		t.Fatalf("ira blockers = %+v, want none", iraBlockers)
	}
}

func TestEnableLots_PreviewAndApply(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage") // non-lot
	sec := createSec(t, env.secRepo, "VTI")
	p1 := types.MustNewMoney("100.00")
	p2 := types.MustNewMoney("120.00")
	mustBuy(t, env, acct.ID, sec.ID, types.NewDate(2024, time.January, 2), "10", &p1)
	mustBuy(t, env, acct.ID, sec.ID, types.NewDate(2024, time.February, 2), "5", &p2)
	sp := types.MustNewMoney("130.00")
	if _, err := env.svc.Sell(acct.ID, sec.ID, types.NewDate(2024, time.March, 2), types.MustNewQuantity("4"), nil, &sp, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell: %v", err)
	}

	// Preview: returns a plan, writes nothing.
	preview, err := env.svc.EnableLots(acct.ID, LotMethodFIFO, false)
	if err != nil {
		t.Fatalf("EnableLots preview: %v", err)
	}
	if preview.Applied {
		t.Error("preview should not be Applied")
	}
	if preview.Plan == nil || len(preview.Plan.Lots) != 2 {
		t.Fatalf("preview plan should have 2 lots, got %v", preview.Plan)
	}
	gotLots, _ := env.lotRepo.ListAllByAccount(acct.ID)
	if len(gotLots) != 0 {
		t.Errorf("preview must not persist lots, got %d", len(gotLots))
	}
	reloaded, _ := env.accountRepo.GetByID(acct.ID)
	if reloaded.TrackLots {
		t.Error("preview must not flip TrackLots")
	}

	// Apply.
	applied, err := env.svc.EnableLots(acct.ID, LotMethodFIFO, true)
	if err != nil {
		t.Fatalf("EnableLots apply: %v", err)
	}
	if !applied.Applied {
		t.Error("apply should be Applied")
	}
	gotLots, _ = env.lotRepo.ListAllByAccount(acct.ID)
	if len(gotLots) != 2 {
		t.Errorf("apply should persist 2 lots, got %d", len(gotLots))
	}
	reloaded, _ = env.accountRepo.GetByID(acct.ID)
	if !reloaded.TrackLots {
		t.Error("apply must flip TrackLots on")
	}
}

func TestEnableLots_RefusesExistingLots(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Lot Brokerage")
	sec := createSec(t, env.secRepo, "QQQ")
	p := types.MustNewMoney("100.00")
	mustBuy(t, env, acct.ID, sec.ID, types.NewDate(2024, time.January, 2), "10", &p) // creates a lot

	_, err := env.svc.EnableLots(acct.ID, LotMethodFIFO, true)
	if err == nil {
		t.Fatal("expected refusal when account already has lots")
	}
	if !strings.Contains(err.Error(), "already has") {
		t.Errorf("expected 'already has' error, got: %v", err)
	}
}

func TestEnableLots_BlockedByCorporateAction(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "SCHB")
	p := types.MustNewMoney("50.00")
	mustBuy(t, env, acct.ID, sec.ID, types.NewDate(2022, time.January, 3), "47", &p)
	ca := NewCorporateAction(ActionTypeSplit, sec.ID, types.NewDate(2022, time.March, 11), `{"numerator":2,"denominator":1}`)
	if err := env.caRepo.Create(ca); err != nil {
		t.Fatalf("create corporate action: %v", err)
	}

	_, err := env.svc.EnableLots(acct.ID, LotMethodFIFO, true)
	var blocked *BackfillBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("expected *BackfillBlockedError, got %v", err)
	}
	if len(blocked.Blockers) != 1 || blocked.Blockers[0].SecurityID != sec.ID {
		t.Errorf("expected SCHB blocker, got %+v", blocked.Blockers)
	}
	// Nothing should have been applied.
	reloaded, _ := env.accountRepo.GetByID(acct.ID)
	if reloaded.TrackLots {
		t.Error("blocked enable-lots must not flip TrackLots")
	}
}

func TestTotalSharesForSecurity(t *testing.T) {
	env := createFullTestService(t)
	nonLot := createInvAccount(t, env.accountRepo, "Brokerage")
	lotAcct := createLotTrackingAccount(t, env.accountRepo, "IRA")
	sec := createSec(t, env.secRepo, "GBTC")
	p := types.MustNewMoney("50.00")
	mustBuy(t, env, nonLot.ID, sec.ID, types.NewDate(2024, time.January, 2), "100", &p)  // non-lot position
	mustBuy(t, env, lotAcct.ID, sec.ID, types.NewDate(2024, time.January, 2), "127", &p) // lot-tracked

	total, err := env.svc.TotalSharesForSecurity(sec.ID)
	if err != nil {
		t.Fatalf("TotalSharesForSecurity: %v", err)
	}
	if total.String() != "227" {
		t.Errorf("total shares = %s, want 227 (100 non-lot + 127 lot)", total.String())
	}
}

func mustBuy(t *testing.T, env *testServiceEnv, accountID, securityID types.ID, date types.Date, shares string, price *types.Money) {
	t.Helper()
	if _, err := env.svc.Buy(accountID, securityID, date, types.MustNewQuantity(shares), nil, price, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy %s shares: %v", shares, err)
	}
}
