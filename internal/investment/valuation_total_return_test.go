package investment

import (
	"math"
	"sort"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

// sortTxnsForReplay sorts transactions by date ascending, then created_at
// ascending — the same canonical chronological order used by replayPosition
// and the TR-008 service wrapper.
func sortTxnsForReplay(txns []*Transaction) {
	sort.SliceStable(txns, func(i, j int) bool {
		if txns[i].Date.Time().Equal(txns[j].Date.Time()) {
			return txns[i].CreatedAt.Time().Before(txns[j].CreatedAt.Time())
		}
		return txns[i].Date.Time().Before(txns[j].Date.Time())
	})
}

// loadAndSortTxnsForSecurity returns this (account, security)'s investment
// transactions in canonical chronological order for replayRealizedGain.
func loadAndSortTxnsForSecurity(t *testing.T, env *testServiceEnv, accountID, securityID types.ID) []*Transaction {
	t.Helper()
	secFilter := securityID
	txns, err := env.invRepo.ListByAccount(accountID, TransactionFilter{SecurityID: &secFilter})
	if err != nil {
		t.Fatalf("ListByAccount(secFilter) error = %v", err)
	}
	sortTxnsForReplay(txns)
	return txns
}

// TR-001 (originally): New total-return fields default to zero values when
// callers pass ValuationOptions{}. After TR-012 wired
// enrichHoldingTotalReturn into the per-holding paths and TR-013 wired
// account-level totals into GetAccountValuation, fields whose components
// have *some* contributing transaction (buys → TotalCostDeployed,
// unrealized → TotalReturn) are now populated. Components with no
// contributing transaction (no sells/dividends/interest/fees) still report
// zero. The legacy unrealized-only TotalGainLoss / TotalGainPct must stay
// unchanged. This is the existing back-compat contract for callers that
// haven't opted into total-return numbers.
func TestValuation_NewFieldsZeroValue_BackCompat(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("5000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	total := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	p := price.NewPrice(sec.ID, types.NewDate(2024, time.March, 20), types.MustNewMoney("120"), price.SourceManual)
	if err := env.priceRepo.Create(p); err != nil {
		t.Fatalf("Create price error = %v", err)
	}

	asOf := types.NewDate(2024, time.March, 20)

	t.Run("GetAccountValuation new fields populated from buy-only fixture", func(t *testing.T) {
		// TR-013 wired account-level totals into GetAccountValuation, so a
		// fixture with a single buy now carries non-zero TotalCostDeployed
		// / TotalReturn / TotalReturnPct at the account level. Components
		// without a contributing transaction (no sells, no dividends, no
		// interest, no commissions/fees) stay zero. The legacy
		// TotalGainLoss / TotalGainPct still mean unrealized only.
		val, err := env.svc.GetAccountValuation(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetAccountValuation() error = %v", err)
		}

		if !val.RealizedGain.IsZero() {
			t.Errorf("Expected RealizedGain to be zero (no sells), got %s", val.RealizedGain.String())
		}
		if !val.DividendsReceived.IsZero() {
			t.Errorf("Expected DividendsReceived to be zero (no dividends), got %s", val.DividendsReceived.String())
		}
		if !val.InterestReceived.IsZero() {
			t.Errorf("Expected InterestReceived to be zero (no interest), got %s", val.InterestReceived.String())
		}
		if !val.FeesPaid.IsZero() {
			t.Errorf("Expected FeesPaid to be zero (no commissions/fees), got %s", val.FeesPaid.String())
		}
		if val.TotalCostDeployed.String() != "1000" {
			t.Errorf("Expected TotalCostDeployed '1000' (one buy), got %s", val.TotalCostDeployed.String())
		}
		if val.TotalReturn.String() != "200" {
			t.Errorf("Expected TotalReturn '200' (unrealized only), got %s", val.TotalReturn.String())
		}
		if val.TotalReturnPct == nil {
			t.Fatalf("Expected TotalReturnPct non-nil when capital has been deployed")
		}
		if *val.TotalReturnPct < 19.99 || *val.TotalReturnPct > 20.01 {
			t.Errorf("Expected TotalReturnPct ~20.0 (200/1000×100), got %f", *val.TotalReturnPct)
		}
		if val.HasClosedPositions {
			t.Errorf("Expected HasClosedPositions to be false, got true")
		}

		// Legacy unrealized-only fields must be unchanged.
		if val.CashBalance.String() != "4000" {
			t.Errorf("Expected CashBalance '4000', got %q", val.CashBalance.String())
		}
		if val.MarketValue.String() != "1200" {
			t.Errorf("Expected MarketValue '1200', got %q", val.MarketValue.String())
		}
		if val.TotalCostBasis.String() != "1000" {
			t.Errorf("Expected TotalCostBasis '1000', got %q", val.TotalCostBasis.String())
		}
		if val.TotalGainLoss.String() != "200" {
			t.Errorf("Expected TotalGainLoss '200' (unrealized only), got %q", val.TotalGainLoss.String())
		}
	})

	t.Run("GetHoldings new fields populated from buy-only fixture", func(t *testing.T) {
		// TR-012 wired enrichHoldingTotalReturn into the holding paths, so
		// holdings that have a buy now carry non-zero TotalCostDeployed /
		// TotalReturn / TotalReturnPct. Components without a contributing
		// transaction (no sells, no dividends, no commissions) stay zero.
		holdings, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetHoldings() error = %v", err)
		}
		if len(holdings) != 1 {
			t.Fatalf("Expected 1 holding, got %d", len(holdings))
		}
		h := holdings[0]

		if !h.RealizedGain.IsZero() {
			t.Errorf("Expected RealizedGain to be zero (no sells), got %s", h.RealizedGain.String())
		}
		if !h.DividendsReceived.IsZero() {
			t.Errorf("Expected DividendsReceived to be zero (no dividends), got %s", h.DividendsReceived.String())
		}
		if !h.FeesPaid.IsZero() {
			t.Errorf("Expected FeesPaid to be zero (no commission), got %s", h.FeesPaid.String())
		}
		if h.TotalCostDeployed.String() != "1000" {
			t.Errorf("Expected TotalCostDeployed '1000' (one buy), got %s", h.TotalCostDeployed.String())
		}
		if h.TotalReturn.String() != "200" {
			t.Errorf("Expected TotalReturn '200' (unrealized only), got %s", h.TotalReturn.String())
		}
		if h.TotalReturnPct == nil {
			t.Fatalf("Expected TotalReturnPct non-nil for holding with a buy")
		}
		if *h.TotalReturnPct < 19.99 || *h.TotalReturnPct > 20.01 {
			t.Errorf("Expected TotalReturnPct ~20.0 (200/1000×100), got %f", *h.TotalReturnPct)
		}
		if h.IsClosed {
			t.Errorf("Expected IsClosed to be false, got true")
		}
		if h.RealizedGainUnavailable {
			t.Errorf("Expected RealizedGainUnavailable to be false, got true")
		}

		// Existing per-holding values unchanged.
		if h.Shares.String() != "10" {
			t.Errorf("Expected Shares '10', got %q", h.Shares.String())
		}
		if h.CostBasis.String() != "1000" {
			t.Errorf("Expected CostBasis '1000', got %q", h.CostBasis.String())
		}
		if h.MarketValue.String() != "1200" {
			t.Errorf("Expected MarketValue '1200', got %q", h.MarketValue.String())
		}
		if h.GainLoss.String() != "200" {
			t.Errorf("Expected GainLoss '200', got %q", h.GainLoss.String())
		}
	})
}

// TR-002: sumDividendsForSecurity sums `dividend` transactions for a single
// (account, security) pair. Reinvested dividends and other securities must
// not contribute.

func TestSumDividendsForSecurity_HappyPath(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("100"), ""); err != nil {
		t.Fatalf("Dividend() error = %v", err)
	}
	if _, err := env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("75"), ""); err != nil {
		t.Fatalf("Dividend() error = %v", err)
	}

	got, err := env.svc.sumDividendsForSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("sumDividendsForSecurity() error = %v", err)
	}
	if got.String() != "175" {
		t.Errorf("Expected total dividends '175', got %q", got.String())
	}

	other := createSec(t, env.secRepo, "MSFT")
	got, err = env.svc.sumDividendsForSecurity(acct.ID, other.ID)
	if err != nil {
		t.Fatalf("sumDividendsForSecurity() error = %v", err)
	}
	if !got.IsZero() {
		t.Errorf("Expected zero for security with no dividends, got %q", got.String())
	}
}

func TestSumDividendsForSecurity_IgnoresReinvest(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("5000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buyTotal := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	if _, err := env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("50"), ""); err != nil {
		t.Fatalf("Dividend() error = %v", err)
	}
	reinvestTotal := types.MustNewMoney("110")
	if _, err := env.svc.ReinvestDividend(acct.ID, sec.ID, date, types.MustNewQuantity("1"), &reinvestTotal, nil, ""); err != nil {
		t.Fatalf("ReinvestDividend() error = %v", err)
	}

	got, err := env.svc.sumDividendsForSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("sumDividendsForSecurity() error = %v", err)
	}
	if got.String() != "50" {
		t.Errorf("Expected only cash dividend '50' (reinvest excluded), got %q", got.String())
	}
}

func TestSumDividendsForSecurity_IgnoresOtherSecurities(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	aapl := createSec(t, env.secRepo, "AAPL")
	msft := createSec(t, env.secRepo, "MSFT")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Dividend(acct.ID, aapl.ID, date, types.MustNewMoney("80"), ""); err != nil {
		t.Fatalf("Dividend() error = %v", err)
	}
	if _, err := env.svc.Dividend(acct.ID, msft.ID, date, types.MustNewMoney("40"), ""); err != nil {
		t.Fatalf("Dividend() error = %v", err)
	}

	got, err := env.svc.sumDividendsForSecurity(acct.ID, aapl.ID)
	if err != nil {
		t.Fatalf("sumDividendsForSecurity() error = %v", err)
	}
	if got.String() != "80" {
		t.Errorf("Expected AAPL-only dividends '80', got %q", got.String())
	}
}

// TR-003: sumInterestForAccount sums `interest` transactions for an investment
// account. No security filter — interest is paid on the cash sweep, not a
// holding. Interest in sibling accounts must not leak in.

func TestSumInterestForAccount_HappyPath(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Interest(acct.ID, date, types.MustNewMoney("12.50"), ""); err != nil {
		t.Fatalf("Interest() error = %v", err)
	}
	if _, err := env.svc.Interest(acct.ID, date, types.MustNewMoney("7.25"), ""); err != nil {
		t.Fatalf("Interest() error = %v", err)
	}
	if _, err := env.svc.Interest(acct.ID, date, types.MustNewMoney("30.25"), ""); err != nil {
		t.Fatalf("Interest() error = %v", err)
	}

	got, err := env.svc.sumInterestForAccount(acct.ID)
	if err != nil {
		t.Fatalf("sumInterestForAccount() error = %v", err)
	}
	if got.String() != "50" {
		t.Errorf("Expected total interest '50', got %q", got.String())
	}

	empty := createInvAccount(t, env.accountRepo, "Empty")
	got, err = env.svc.sumInterestForAccount(empty.ID)
	if err != nil {
		t.Fatalf("sumInterestForAccount() error = %v", err)
	}
	if !got.IsZero() {
		t.Errorf("Expected zero for account with no interest, got %q", got.String())
	}
}

func TestSumInterestForAccount_OtherAccountIgnored(t *testing.T) {
	env := createFullTestService(t)
	a := createInvAccount(t, env.accountRepo, "Brokerage A")
	b := createInvAccount(t, env.accountRepo, "Brokerage B")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Interest(a.ID, date, types.MustNewMoney("20"), ""); err != nil {
		t.Fatalf("Interest() error = %v", err)
	}
	if _, err := env.svc.Interest(b.ID, date, types.MustNewMoney("99"), ""); err != nil {
		t.Fatalf("Interest() error = %v", err)
	}

	got, err := env.svc.sumInterestForAccount(a.ID)
	if err != nil {
		t.Fatalf("sumInterestForAccount() error = %v", err)
	}
	if got.String() != "20" {
		t.Errorf("Expected account A interest '20', got %q", got.String())
	}
}

// TR-004: sumFeesForSecurity sums commissions on buy/sell/reinvest_dividend
// transactions plus the full total_amount of any fee_liquidation transactions
// for a single (account, security) pair. Account-level `fee` transactions
// (no security_id) are deliberately excluded — they are summed separately at
// the account level. The result is a positive magnitude.

func TestSumFeesForSecurity_HappyPath(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}

	buyTotal := types.MustNewMoney("1005")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"),
		&buyTotal, nil, types.MustNewMoney("5"), ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	sellTotal := types.MustNewMoney("800")
	if _, err := env.svc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("5"),
		&sellTotal, nil, types.MustNewMoney("10"), "", nil); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	// Service-level ReinvestDividend hardcodes zero commission; persist a $1
	// commission on the resulting row so the helper exercises the
	// reinvest_dividend.commission branch per the spec.
	reinvestTotal := types.MustNewMoney("100")
	rtxn, err := env.svc.ReinvestDividend(acct.ID, sec.ID, date,
		types.MustNewQuantity("1"), &reinvestTotal, nil, "")
	if err != nil {
		t.Fatalf("ReinvestDividend() error = %v", err)
	}
	rtxn.SetCommission(types.MustNewMoney("1"))
	if err := env.invRepo.Update(rtxn); err != nil {
		t.Fatalf("Update(reinvest commission) error = %v", err)
	}

	feeTotal := types.MustNewMoney("25")
	if _, err := env.svc.FeeLiquidation(acct.ID, sec.ID, date,
		types.MustNewQuantity("0.1"), &feeTotal, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("FeeLiquidation() error = %v", err)
	}

	got, err := env.svc.sumFeesForSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("sumFeesForSecurity() error = %v", err)
	}
	if got.String() != "41" {
		t.Errorf("Expected total fees '41' (5 + 10 + 1 + 25), got %q", got.String())
	}
}

func TestSumFeesForSecurity_NoCommissionField(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("5000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}

	buyTotal := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"),
		&buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	sellTotal := types.MustNewMoney("600")
	if _, err := env.svc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("5"),
		&sellTotal, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	got, err := env.svc.sumFeesForSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("sumFeesForSecurity() error = %v", err)
	}
	if !got.IsZero() {
		t.Errorf("Expected zero (no commissions set), got %q", got.String())
	}
}

func TestSumFeesForSecurity_AccountLevelFeeIgnored(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("5000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	if _, err := env.svc.Fee(acct.ID, date, types.MustNewMoney("50"), ""); err != nil {
		t.Fatalf("Fee() error = %v", err)
	}
	buyTotal := types.MustNewMoney("105")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("1"),
		&buyTotal, nil, types.MustNewMoney("5"), ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	got, err := env.svc.sumFeesForSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("sumFeesForSecurity() error = %v", err)
	}
	if got.String() != "5" {
		t.Errorf("Expected only the per-security commission '5' (account fee excluded), got %q", got.String())
	}
}

// TR-005: sumFeesForAccount aggregates fees across every security in the
// account plus any account-level `fee` transactions (which have no
// security_id). The result is a positive magnitude.

func TestSumFeesForAccount_AccumulatesPerSecurityAndAccount(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	aapl := createSec(t, env.secRepo, "AAPL")
	msft := createSec(t, env.secRepo, "MSFT")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("20000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}

	// AAPL: buy commission $5, sell commission $10, fee_liquidation $25 = $40.
	buyA := types.MustNewMoney("1005")
	if _, err := env.svc.Buy(acct.ID, aapl.ID, date, types.MustNewQuantity("10"),
		&buyA, nil, types.MustNewMoney("5"), ""); err != nil {
		t.Fatalf("Buy(AAPL) error = %v", err)
	}
	sellA := types.MustNewMoney("800")
	if _, err := env.svc.Sell(acct.ID, aapl.ID, date, types.MustNewQuantity("5"),
		&sellA, nil, types.MustNewMoney("10"), "", nil); err != nil {
		t.Fatalf("Sell(AAPL) error = %v", err)
	}
	feeA := types.MustNewMoney("25")
	if _, err := env.svc.FeeLiquidation(acct.ID, aapl.ID, date,
		types.MustNewQuantity("0.1"), &feeA, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("FeeLiquidation(AAPL) error = %v", err)
	}

	// MSFT: buy commission $7, sell commission $3 = $10.
	buyM := types.MustNewMoney("507")
	if _, err := env.svc.Buy(acct.ID, msft.ID, date, types.MustNewQuantity("5"),
		&buyM, nil, types.MustNewMoney("7"), ""); err != nil {
		t.Fatalf("Buy(MSFT) error = %v", err)
	}
	sellM := types.MustNewMoney("300")
	if _, err := env.svc.Sell(acct.ID, msft.ID, date, types.MustNewQuantity("2"),
		&sellM, nil, types.MustNewMoney("3"), "", nil); err != nil {
		t.Fatalf("Sell(MSFT) error = %v", err)
	}

	// Account-level fees (no security_id): $15 + $20 = $35.
	if _, err := env.svc.Fee(acct.ID, date, types.MustNewMoney("15"), ""); err != nil {
		t.Fatalf("Fee() error = %v", err)
	}
	if _, err := env.svc.Fee(acct.ID, date, types.MustNewMoney("20"), ""); err != nil {
		t.Fatalf("Fee() error = %v", err)
	}

	got, err := env.svc.sumFeesForAccount(acct.ID)
	if err != nil {
		t.Fatalf("sumFeesForAccount() error = %v", err)
	}
	// AAPL $40 + MSFT $10 + account-level $35 = $85.
	if got.String() != "85" {
		t.Errorf("Expected total fees '85' (AAPL 40 + MSFT 10 + acct 35), got %q", got.String())
	}
}

func TestSumFeesForAccount_OnlyAccountLevelFees(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("5000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	if _, err := env.svc.Fee(acct.ID, date, types.MustNewMoney("12"), ""); err != nil {
		t.Fatalf("Fee() error = %v", err)
	}
	if _, err := env.svc.Fee(acct.ID, date, types.MustNewMoney("8"), ""); err != nil {
		t.Fatalf("Fee() error = %v", err)
	}

	got, err := env.svc.sumFeesForAccount(acct.ID)
	if err != nil {
		t.Fatalf("sumFeesForAccount() error = %v", err)
	}
	if got.String() != "20" {
		t.Errorf("Expected total fees '20' (account-level only), got %q", got.String())
	}

	empty := createInvAccount(t, env.accountRepo, "Empty")
	got, err = env.svc.sumFeesForAccount(empty.ID)
	if err != nil {
		t.Fatalf("sumFeesForAccount(empty) error = %v", err)
	}
	if !got.IsZero() {
		t.Errorf("Expected zero for account with no fees, got %q", got.String())
	}
}

// TR-006: realizedGainLotTracked walks the transaction_lots junction for sell
// and fee_liquidation transactions and sums
// (txn.price_per_share − lot.cost_per_share) × junction.shares. Commission is
// already netted into txn.price_per_share by ComputePricePerShare; it is
// counted separately as a fee, not subtracted twice.

func TestRealizedGainLotTracked_SingleSell_AtGain(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buyTotal := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"),
		&buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
	if err != nil {
		t.Fatalf("ListByAccountAndSecurity() error = %v", err)
	}
	if len(lots) != 1 {
		t.Fatalf("Expected 1 lot, got %d", len(lots))
	}

	sellTotal := types.MustNewMoney("750")
	allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("5")}}
	if _, err := env.svc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("5"),
		&sellTotal, nil, types.ZeroMoney, "", allocs); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	got, err := env.svc.realizedGainLotTracked(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("realizedGainLotTracked() error = %v", err)
	}
	// (150 − 100) × 5 = 250
	if got.String() != "250" {
		t.Errorf("Expected realized gain '250', got %q", got.String())
	}
}

func TestRealizedGainLotTracked_SingleSell_AtLoss(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buyTotal := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"),
		&buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
	if err != nil {
		t.Fatalf("ListByAccountAndSecurity() error = %v", err)
	}

	sellTotal := types.MustNewMoney("400")
	allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("5")}}
	if _, err := env.svc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("5"),
		&sellTotal, nil, types.ZeroMoney, "", allocs); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	got, err := env.svc.realizedGainLotTracked(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("realizedGainLotTracked() error = %v", err)
	}
	// (80 − 100) × 5 = −100
	if got.String() != "-100" {
		t.Errorf("Expected realized gain '-100', got %q", got.String())
	}
}

func TestRealizedGainLotTracked_MultipleSells_AcrossLots(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date1 := types.NewDate(2024, time.March, 1)
	date2 := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date1, types.MustNewMoney("20000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	// Lot 1: 10 @ $100
	buy1 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date1, types.MustNewQuantity("10"),
		&buy1, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(lot1) error = %v", err)
	}
	// Lot 2: 5 @ $200
	buy2 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date2, types.MustNewQuantity("5"),
		&buy2, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(lot2) error = %v", err)
	}

	lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
	if err != nil {
		t.Fatalf("ListByAccountAndSecurity() error = %v", err)
	}
	if len(lots) != 2 {
		t.Fatalf("Expected 2 lots, got %d", len(lots))
	}

	// Sell 8 @ $250 — 5 from lot 1, 3 from lot 2.
	sellTotal := types.MustNewMoney("2000")
	allocs := []SellLotAllocation{
		{LotID: lots[0].ID, Shares: types.MustNewQuantity("5")},
		{LotID: lots[1].ID, Shares: types.MustNewQuantity("3")},
	}
	if _, err := env.svc.Sell(acct.ID, sec.ID, date2, types.MustNewQuantity("8"),
		&sellTotal, nil, types.ZeroMoney, "", allocs); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	got, err := env.svc.realizedGainLotTracked(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("realizedGainLotTracked() error = %v", err)
	}
	// (250 − 100) × 5 + (250 − 200) × 3 = 750 + 150 = 900
	if got.String() != "900" {
		t.Errorf("Expected realized gain '900', got %q", got.String())
	}
}

func TestRealizedGainLotTracked_FeeLiquidation(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buyTotal := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"),
		&buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
	if err != nil {
		t.Fatalf("ListByAccountAndSecurity() error = %v", err)
	}

	// fee_liquidation: 0.1 shares at $200 from a lot bought at $100.
	feeTotal := types.MustNewMoney("20")
	allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("0.1")}}
	if _, err := env.svc.FeeLiquidation(acct.ID, sec.ID, date, types.MustNewQuantity("0.1"),
		&feeTotal, nil, types.ZeroMoney, "", allocs); err != nil {
		t.Fatalf("FeeLiquidation() error = %v", err)
	}

	got, err := env.svc.realizedGainLotTracked(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("realizedGainLotTracked() error = %v", err)
	}
	// (200 − 100) × 0.1 = 10
	if got.String() != "10" {
		t.Errorf("Expected realized gain '10', got %q", got.String())
	}
}

func TestRealizedGainLotTracked_NoSells_ReturnsZero(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buyTotal := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"),
		&buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	got, err := env.svc.realizedGainLotTracked(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("realizedGainLotTracked() error = %v", err)
	}
	if !got.IsZero() {
		t.Errorf("Expected zero (no sells), got %q", got.String())
	}
}

// TR-007: replayRealizedGain walks a non-lot (account, security) ledger in
// chronological order, accumulating
// (sell.price_per_share − running_avg_cost_per_share) × sell.shares on each
// sell. Buys and reinvest_dividend transactions update the running average
// cost via the same weighted-average that Position.AddShares uses, so the
// avg cost moves with new acquisitions.

func TestReplayRealizedGain_HappyPath(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date1 := types.NewDate(2024, time.March, 1)
	date2 := types.NewDate(2024, time.March, 15)
	date3 := types.NewDate(2024, time.April, 1)
	date4 := types.NewDate(2024, time.April, 15)

	if _, err := env.svc.Deposit(acct.ID, date1, types.MustNewMoney("50000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	// Buy 10 @ $100 → avg cost $100
	buy1 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date1, types.MustNewQuantity("10"),
		&buy1, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(1) error = %v", err)
	}
	// Buy 10 @ $120 → avg cost (1000+1200)/20 = $110
	buy2 := types.MustNewMoney("1200")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date2, types.MustNewQuantity("10"),
		&buy2, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(2) error = %v", err)
	}
	// Sell 5 @ $150 → realized = (150−110)×5 = 200
	sell1 := types.MustNewMoney("750")
	if _, err := env.svc.Sell(acct.ID, sec.ID, date3, types.MustNewQuantity("5"),
		&sell1, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(1) error = %v", err)
	}
	// Sell 5 @ $130 → realized = (130−110)×5 = 100 (RemoveShares leaves avg unchanged)
	sell2 := types.MustNewMoney("650")
	if _, err := env.svc.Sell(acct.ID, sec.ID, date4, types.MustNewQuantity("5"),
		&sell2, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(2) error = %v", err)
	}

	txns := loadAndSortTxnsForSecurity(t, env, acct.ID, sec.ID)
	got, err := env.svc.replayRealizedGain(acct.ID, sec.ID, txns)
	if err != nil {
		t.Fatalf("replayRealizedGain() error = %v", err)
	}
	if got.String() != "300" {
		t.Errorf("Expected realized gain '300' (200 + 100), got %q", got.String())
	}
}

func TestReplayRealizedGain_LossThenGain(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date1 := types.NewDate(2024, time.January, 1)
	date2 := types.NewDate(2024, time.February, 1)
	date3 := types.NewDate(2024, time.March, 1)
	date4 := types.NewDate(2024, time.April, 1)

	if _, err := env.svc.Deposit(acct.ID, date1, types.MustNewMoney("50000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	// Buy 10 @ $100 → avg cost $100
	buy1 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date1, types.MustNewQuantity("10"),
		&buy1, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(1) error = %v", err)
	}
	// Sell 5 @ $80 (loss) → realized = (80−100)×5 = −100. 5 shares remain @ $100.
	sell1 := types.MustNewMoney("400")
	if _, err := env.svc.Sell(acct.ID, sec.ID, date2, types.MustNewQuantity("5"),
		&sell1, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(1) error = %v", err)
	}
	// Buy 5 @ $200 → avg cost = (5×100 + 5×200)/10 = $150. Confirms a subsequent
	// buy does not "reset" the running avg cost — it accumulates through the
	// existing Position.AddShares weighted average.
	buy2 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date3, types.MustNewQuantity("5"),
		&buy2, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(2) error = %v", err)
	}
	// Sell 10 @ $200 → realized = (200−150)×10 = +500.
	sell2 := types.MustNewMoney("2000")
	if _, err := env.svc.Sell(acct.ID, sec.ID, date4, types.MustNewQuantity("10"),
		&sell2, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(2) error = %v", err)
	}

	txns := loadAndSortTxnsForSecurity(t, env, acct.ID, sec.ID)
	got, err := env.svc.replayRealizedGain(acct.ID, sec.ID, txns)
	if err != nil {
		t.Fatalf("replayRealizedGain() error = %v", err)
	}
	// −100 + 500 = 400
	if got.String() != "400" {
		t.Errorf("Expected realized gain '400' (−100 + 500), got %q", got.String())
	}
}

func TestReplayRealizedGain_SameDateOrdering(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	earlier := types.NewDate(2024, time.March, 1)
	same := types.NewDate(2024, time.March, 15)

	if _, err := env.svc.Deposit(acct.ID, earlier, types.MustNewMoney("50000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	// All four share the SAME date, but distinct CreatedAt timestamps from
	// monotonic insertion order. Sorting by (Date asc, CreatedAt asc) must
	// preserve insertion order so the realized gain is deterministic.
	buy1 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, same, types.MustNewQuantity("10"),
		&buy1, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(1) error = %v", err)
	}
	sell1 := types.MustNewMoney("750")
	if _, err := env.svc.Sell(acct.ID, sec.ID, same, types.MustNewQuantity("5"),
		&sell1, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(1) error = %v", err)
	}
	buy2 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, same, types.MustNewQuantity("5"),
		&buy2, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(2) error = %v", err)
	}
	sell2 := types.MustNewMoney("1000")
	if _, err := env.svc.Sell(acct.ID, sec.ID, same, types.MustNewQuantity("5"),
		&sell2, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(2) error = %v", err)
	}

	txns := loadAndSortTxnsForSecurity(t, env, acct.ID, sec.ID)
	got, err := env.svc.replayRealizedGain(acct.ID, sec.ID, txns)
	if err != nil {
		t.Fatalf("replayRealizedGain() error = %v", err)
	}
	// Buy 10 @ 100 → avg 100, 10 sh.
	// Sell 5 @ 150 → realized (150−100)×5 = 250; 5 sh @ 100.
	// Buy 5 @ 200 → avg (5×100 + 5×200)/10 = 150; 10 sh.
	// Sell 5 @ 200 → realized (200−150)×5 = 250.
	// Total = 500.
	//
	// Reordered (e.g. both buys before both sells) the result is 250 + 166.67
	// = 416.67 — a different number. Asserting 500 confirms CreatedAt-order
	// determinism.
	if got.String() != "500" {
		t.Errorf("Expected realized gain '500' (same-date insertion order), got %q", got.String())
	}
}

func TestReplayRealizedGain_ReinvestRaisesAvgCost(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date1 := types.NewDate(2024, time.March, 1)
	date2 := types.NewDate(2024, time.March, 15)
	date3 := types.NewDate(2024, time.April, 1)

	if _, err := env.svc.Deposit(acct.ID, date1, types.MustNewMoney("50000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	// Buy 10 @ $100 → avg cost $100.
	buy1 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date1, types.MustNewQuantity("10"),
		&buy1, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	// Reinvest 10 @ $200 (total $2000) → avg cost (1000 + 2000)/20 = $150.
	reinvest := types.MustNewMoney("2000")
	if _, err := env.svc.ReinvestDividend(acct.ID, sec.ID, date2,
		types.MustNewQuantity("10"), &reinvest, nil, ""); err != nil {
		t.Fatalf("ReinvestDividend() error = %v", err)
	}
	// Sell 5 @ $200 → realized (200−150)×5 = 250.
	// Without the reinvest the avg cost would still be $100 and realized
	// would be (200−100)×5 = 500. Asserting 250 confirms the reinvest
	// raised the avg cost.
	sell := types.MustNewMoney("1000")
	if _, err := env.svc.Sell(acct.ID, sec.ID, date3, types.MustNewQuantity("5"),
		&sell, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	txns := loadAndSortTxnsForSecurity(t, env, acct.ID, sec.ID)
	got, err := env.svc.replayRealizedGain(acct.ID, sec.ID, txns)
	if err != nil {
		t.Fatalf("replayRealizedGain() error = %v", err)
	}
	if got.String() != "250" {
		t.Errorf("Expected realized gain '250' (reinvest raised avg cost to $150), got %q", got.String())
	}
}

// TR-008: realizedGainNonLot is the service wrapper that loads transactions
// for the (account, security) pair, sorts them in canonical order, and
// delegates to replayRealizedGain. The fixture mirrors TR-007's happy path
// and must produce the same $300.
func TestRealizedGainNonLot_DelegatesToReplay(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date1 := types.NewDate(2024, time.March, 1)
	date2 := types.NewDate(2024, time.March, 15)
	date3 := types.NewDate(2024, time.April, 1)
	date4 := types.NewDate(2024, time.April, 15)

	if _, err := env.svc.Deposit(acct.ID, date1, types.MustNewMoney("50000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	// Buy 10 @ $100 → avg cost $100
	buy1 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date1, types.MustNewQuantity("10"),
		&buy1, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(1) error = %v", err)
	}
	// Buy 10 @ $120 → avg cost (1000+1200)/20 = $110
	buy2 := types.MustNewMoney("1200")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date2, types.MustNewQuantity("10"),
		&buy2, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(2) error = %v", err)
	}
	// Sell 5 @ $150 → realized = (150−110)×5 = 200
	sell1 := types.MustNewMoney("750")
	if _, err := env.svc.Sell(acct.ID, sec.ID, date3, types.MustNewQuantity("5"),
		&sell1, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(1) error = %v", err)
	}
	// Sell 5 @ $130 → realized = (130−110)×5 = 100
	sell2 := types.MustNewMoney("650")
	if _, err := env.svc.Sell(acct.ID, sec.ID, date4, types.MustNewQuantity("5"),
		&sell2, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(2) error = %v", err)
	}

	got, err := env.svc.realizedGainNonLot(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("realizedGainNonLot() error = %v", err)
	}
	if got.String() != "300" {
		t.Errorf("Expected realized gain '300' (200 + 100), got %q", got.String())
	}
}

// TR-009: When the database contains any corporate-action record, the
// non-lot realized-gain replay cannot produce a correct number (the
// ledger reflects post-action share counts but `replayPosition`'s walk
// is unaware of the split/merger). The realized-gain entry point detects
// this case and returns (ZeroMoney, unavailable=true) instead. Dividends
// and fees are unaffected.
func TestRealizedGain_NonLot_WithCorporateActions_Unavailable(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	d1 := types.NewDate(2024, time.March, 1)
	dSplit := types.NewDate(2024, time.June, 1)
	d2 := types.NewDate(2024, time.July, 1)

	if _, err := env.svc.Deposit(acct.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	// Buy 10 @ $100 (pre-split).
	buyTotal := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, d1, types.MustNewQuantity("10"),
		&buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	// Record a dividend that should still be visible after the gate trips.
	if _, err := env.svc.Dividend(acct.ID, sec.ID, d1, types.MustNewMoney("50"), ""); err != nil {
		t.Fatalf("Dividend() error = %v", err)
	}

	// Apply a 4:1 split — position is mutated outside the ledger.
	caSvc := NewCorporateActionService(env.caRepo, env.lotRepo, env.positionRepo, env.priceRepo, env.invRepo, env.secRepo, env.db)
	if _, err := caSvc.Split(sec.ID, dSplit, SplitParams{Numerator: 4, Denominator: 1}); err != nil {
		t.Fatalf("Split() error = %v", err)
	}

	// Sell 5 (post-split) shares @ $30. The ledger has post-split share counts;
	// replayRealizedGain would produce a wrong number.
	sellTotal := types.MustNewMoney("150")
	if _, err := env.svc.Sell(acct.ID, sec.ID, d2, types.MustNewQuantity("5"),
		&sellTotal, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	got, unavailable, err := env.svc.realizedGain(acct.ID, sec.ID, false)
	if err != nil {
		t.Fatalf("realizedGain() error = %v", err)
	}
	if !unavailable {
		t.Errorf("Expected unavailable=true for non-lot account with corporate actions")
	}
	if !got.IsZero() {
		t.Errorf("Expected realized gain to be zero when unavailable, got %q", got.String())
	}

	// Dividends and fees must still be computable.
	div, err := env.svc.sumDividendsForSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("sumDividendsForSecurity() error = %v", err)
	}
	if div.String() != "50" {
		t.Errorf("Expected dividends '50', got %q", div.String())
	}
	fees, err := env.svc.sumFeesForSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("sumFeesForSecurity() error = %v", err)
	}
	if !fees.IsZero() {
		t.Errorf("Expected fees zero, got %q", fees.String())
	}
}

// TR-009: A lot-tracked account is robust to corporate actions because
// the corporate-action service mutates lots in place and transaction_lots
// rows reference post-action lots. The realized-gain entry point produces
// a real number for the lot-tracked path even when corporate actions
// exist in the database. No unavailable flag.
func TestRealizedGain_LotTracked_WithCorporateActions_StillComputed(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	d1 := types.NewDate(2024, time.March, 1)
	dSplit := types.NewDate(2024, time.June, 1)
	d2 := types.NewDate(2024, time.July, 1)

	if _, err := env.svc.Deposit(acct.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	// Buy 10 @ $100 pre-split → cost_per_share $100.
	buyTotal := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, d1, types.MustNewQuantity("10"),
		&buyTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	// Apply 4:1 split — the lone lot becomes 40 shares @ $25 cost_per_share.
	caSvc := NewCorporateActionService(env.caRepo, env.lotRepo, env.positionRepo, env.priceRepo, env.invRepo, env.secRepo, env.db)
	if _, err := caSvc.Split(sec.ID, dSplit, SplitParams{Numerator: 4, Denominator: 1}); err != nil {
		t.Fatalf("Split() error = %v", err)
	}

	lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
	if err != nil {
		t.Fatalf("ListByAccountAndSecurity() error = %v", err)
	}
	if len(lots) != 1 {
		t.Fatalf("Expected 1 lot, got %d", len(lots))
	}

	// Sell 5 post-split shares @ $30 → realized (30−25)×5 = 25.
	sellTotal := types.MustNewMoney("150")
	allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("5")}}
	if _, err := env.svc.Sell(acct.ID, sec.ID, d2, types.MustNewQuantity("5"),
		&sellTotal, nil, types.ZeroMoney, "", allocs); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	got, unavailable, err := env.svc.realizedGain(acct.ID, sec.ID, true)
	if err != nil {
		t.Fatalf("realizedGain() error = %v", err)
	}
	if unavailable {
		t.Errorf("Expected unavailable=false for lot-tracked account, got true")
	}
	if got.String() != "25" {
		t.Errorf("Expected realized gain '25', got %q", got.String())
	}
}

// TR-010: totalCostDeployedForSecurity sums total_amount for `buy` and
// `reinvest_dividend` transactions on a (account, security) pair. Shares
// received via `transfer_shares` carry cost basis with them but are not new
// capital deployed in this account, so they are excluded from the
// denominator. Buy transactions store total_amount as a negative cash debit;
// the helper returns a positive magnitude.

func TestTotalCostDeployed_BuyOnly(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	d1 := types.NewDate(2024, time.March, 1)
	d2 := types.NewDate(2024, time.April, 1)

	if _, err := env.svc.Deposit(acct.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buy1 := types.MustNewMoney("500")
	if _, err := env.svc.Buy(acct.ID, sec.ID, d1, types.MustNewQuantity("5"),
		&buy1, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(1) error = %v", err)
	}
	buy2 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, d2, types.MustNewQuantity("10"),
		&buy2, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(2) error = %v", err)
	}

	got, err := env.svc.totalCostDeployedForSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("totalCostDeployedForSecurity() error = %v", err)
	}
	if got.String() != "1500" {
		t.Errorf("Expected total cost deployed '1500', got %q", got.String())
	}
}

func TestTotalCostDeployed_BuyPlusReinvest(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	d1 := types.NewDate(2024, time.March, 1)
	d2 := types.NewDate(2024, time.April, 1)

	if _, err := env.svc.Deposit(acct.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buy := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, d1, types.MustNewQuantity("10"),
		&buy, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	reinvest := types.MustNewMoney("50")
	if _, err := env.svc.ReinvestDividend(acct.ID, sec.ID, d2,
		types.MustNewQuantity("0.5"), &reinvest, nil, ""); err != nil {
		t.Fatalf("ReinvestDividend() error = %v", err)
	}

	got, err := env.svc.totalCostDeployedForSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("totalCostDeployedForSecurity() error = %v", err)
	}
	if got.String() != "1050" {
		t.Errorf("Expected total cost deployed '1050' (1000 buy + 50 reinvest), got %q", got.String())
	}
}

func TestTotalCostDeployed_TransferOnly_Zero(t *testing.T) {
	env := createFullTestService(t)
	src := createInvAccount(t, env.accountRepo, "Source")
	dst := createInvAccount(t, env.accountRepo, "Dest")
	sec := createSec(t, env.secRepo, "AAPL")
	d1 := types.NewDate(2024, time.March, 1)
	d2 := types.NewDate(2024, time.April, 1)

	// Seed source with shares so they can be transferred out.
	if _, err := env.svc.Deposit(src.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buy := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(src.ID, sec.ID, d1, types.MustNewQuantity("10"),
		&buy, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	// Destination receives the shares via transfer only — no buys, no reinvests.
	if _, err := env.svc.TransferShares(src.ID, dst.ID, sec.ID, d2,
		types.MustNewQuantity("10"), "", nil); err != nil {
		t.Fatalf("TransferShares() error = %v", err)
	}

	got, err := env.svc.totalCostDeployedForSecurity(dst.ID, sec.ID)
	if err != nil {
		t.Fatalf("totalCostDeployedForSecurity() error = %v", err)
	}
	if !got.IsZero() {
		t.Errorf("Expected zero for security received only via transfer_shares, got %q", got.String())
	}
}

// TR-011: totalCostDeployedForAccount sums totalCostDeployedForSecurity
// across every security held in the account. Only `buy` and
// `reinvest_dividend` transactions contribute; transfers in carry cost
// basis with them and are excluded.

func TestTotalCostDeployedForAccount_SumsAcrossSecurities(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	aapl := createSec(t, env.secRepo, "AAPL")
	msft := createSec(t, env.secRepo, "MSFT")
	d1 := types.NewDate(2024, time.March, 1)
	d2 := types.NewDate(2024, time.April, 1)
	d3 := types.NewDate(2024, time.May, 1)

	if _, err := env.svc.Deposit(acct.ID, d1, types.MustNewMoney("20000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}

	// AAPL: 500 + 1000 buys, plus a 50 reinvest = 1550
	buyA1 := types.MustNewMoney("500")
	if _, err := env.svc.Buy(acct.ID, aapl.ID, d1, types.MustNewQuantity("5"),
		&buyA1, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(AAPL 1) error = %v", err)
	}
	buyA2 := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, aapl.ID, d2, types.MustNewQuantity("10"),
		&buyA2, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(AAPL 2) error = %v", err)
	}
	reinvest := types.MustNewMoney("50")
	if _, err := env.svc.ReinvestDividend(acct.ID, aapl.ID, d3,
		types.MustNewQuantity("0.5"), &reinvest, nil, ""); err != nil {
		t.Fatalf("ReinvestDividend() error = %v", err)
	}

	// MSFT: single 2000 buy = 2000
	buyM := types.MustNewMoney("2000")
	if _, err := env.svc.Buy(acct.ID, msft.ID, d1, types.MustNewQuantity("8"),
		&buyM, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(MSFT) error = %v", err)
	}

	got, err := env.svc.totalCostDeployedForAccount(acct.ID)
	if err != nil {
		t.Fatalf("totalCostDeployedForAccount() error = %v", err)
	}
	if got.String() != "3550" {
		t.Errorf("Expected total cost deployed '3550' (1500 AAPL buys + 50 reinvest + 2000 MSFT), got %q", got.String())
	}
}

// TR-012: GetHoldings enriches each open holding with the total-return
// breakdown — RealizedGain, DividendsReceived, FeesPaid, TotalCostDeployed,
// TotalReturn, and TotalReturnPct — computed against the (account, security)
// ledger. The existing unrealized-only GainLoss / GainPct fields are
// unchanged.
//
// Fixture: non-lot Brokerage / AAPL.
//   - Buy 10 @ price 100 with $10 commission → total_amount $1010, position
//     avg cost $100 (commission excluded per ComputePricePerShare).
//   - Buy 10 @ price 120, no commission → total_amount $1200, avg cost now
//     (10·100 + 10·120) / 20 = $110.
//   - Sell 5 @ price 150, no commission. replayRealizedGain captures avg
//     cost $110 immediately before the sell → realized gain
//     (150 − 110) × 5 = $200. Position drops to 15 shares @ avg $110.
//   - Cash dividend $50.
//   - Price @ asOf is $130 → unrealized = 15 × (130 − 110) = $300.
//
// Expected total-return components for the open AAPL holding:
//
//	RealizedGain      = 200
//	DividendsReceived = 50
//	FeesPaid          = 10        (one buy commission; positive magnitude)
//	TotalCostDeployed = 2210      (Σ buy.total_amount magnitudes)
//	TotalReturn       = 540       (GainLoss 300 + 200 + 50 − 10)
//	TotalReturnPct    ≈ 24.4344   (540 / 2210 × 100)
func TestGetHoldings_PopulatesTotalReturn(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	d1 := types.NewDate(2024, time.March, 1)
	d2 := types.NewDate(2024, time.April, 1)
	d3 := types.NewDate(2024, time.May, 1)
	asOf := types.NewDate(2024, time.June, 1)

	if _, err := env.svc.Deposit(acct.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}

	buy1 := types.MustNewMoney("1010")
	if _, err := env.svc.Buy(acct.ID, sec.ID, d1, types.MustNewQuantity("10"),
		&buy1, nil, types.MustNewMoney("10"), ""); err != nil {
		t.Fatalf("Buy(1) error = %v", err)
	}
	buy2 := types.MustNewMoney("1200")
	if _, err := env.svc.Buy(acct.ID, sec.ID, d2, types.MustNewQuantity("10"),
		&buy2, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(2) error = %v", err)
	}
	sell := types.MustNewMoney("750")
	if _, err := env.svc.Sell(acct.ID, sec.ID, d3, types.MustNewQuantity("5"),
		&sell, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell() error = %v", err)
	}
	if _, err := env.svc.Dividend(acct.ID, sec.ID, d3, types.MustNewMoney("50"), ""); err != nil {
		t.Fatalf("Dividend() error = %v", err)
	}
	p := price.NewPrice(sec.ID, asOf, types.MustNewMoney("130"), price.SourceManual)
	if err := env.priceRepo.Create(p); err != nil {
		t.Fatalf("Create price error = %v", err)
	}

	holdings, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{})
	if err != nil {
		t.Fatalf("GetHoldings() error = %v", err)
	}
	if len(holdings) != 1 {
		t.Fatalf("Expected 1 holding, got %d", len(holdings))
	}
	h := holdings[0]

	// Existing unrealized-only fields are unchanged.
	if h.Shares.String() != "15" {
		t.Errorf("Expected Shares '15', got %q", h.Shares.String())
	}
	if h.CostBasis.String() != "1650" {
		t.Errorf("Expected CostBasis '1650' (15 × avg 110), got %q", h.CostBasis.String())
	}
	if h.MarketValue.String() != "1950" {
		t.Errorf("Expected MarketValue '1950' (15 × 130), got %q", h.MarketValue.String())
	}
	if h.GainLoss.String() != "300" {
		t.Errorf("Expected GainLoss '300' (unrealized), got %q", h.GainLoss.String())
	}

	// New total-return fields.
	if h.RealizedGain.String() != "200" {
		t.Errorf("Expected RealizedGain '200' ((150−110)×5), got %q", h.RealizedGain.String())
	}
	if h.RealizedGainUnavailable {
		t.Errorf("Expected RealizedGainUnavailable=false, got true")
	}
	if h.DividendsReceived.String() != "50" {
		t.Errorf("Expected DividendsReceived '50', got %q", h.DividendsReceived.String())
	}
	if h.FeesPaid.String() != "10" {
		t.Errorf("Expected FeesPaid '10' (buy commission), got %q", h.FeesPaid.String())
	}
	if h.TotalCostDeployed.String() != "2210" {
		t.Errorf("Expected TotalCostDeployed '2210' (1010 + 1200), got %q", h.TotalCostDeployed.String())
	}
	if h.TotalReturn.String() != "540" {
		t.Errorf("Expected TotalReturn '540' (300 + 200 + 50 − 10), got %q", h.TotalReturn.String())
	}
	if h.TotalReturnPct == nil {
		t.Fatalf("Expected TotalReturnPct non-nil")
	}
	expectedPct := 540.0 / 2210.0 * 100.0
	if math.Abs(*h.TotalReturnPct-expectedPct) > 0.001 {
		t.Errorf("Expected TotalReturnPct ≈ %f, got %f", expectedPct, *h.TotalReturnPct)
	}
}

// TR-012: A holding received only via transfer_shares has zero total cost
// deployed (transfers carry basis with the shares but don't represent new
// capital deployed in this account). TotalReturnPct must be nil so the UI
// can render `—` instead of a misleading percent.
func TestGetHoldings_TotalReturnPctNilWhenNoBuys(t *testing.T) {
	env := createFullTestService(t)
	src := createInvAccount(t, env.accountRepo, "Source")
	dst := createInvAccount(t, env.accountRepo, "Dest")
	sec := createSec(t, env.secRepo, "AAPL")
	d1 := types.NewDate(2024, time.March, 1)
	d2 := types.NewDate(2024, time.April, 1)
	asOf := types.NewDate(2024, time.May, 1)

	if _, err := env.svc.Deposit(src.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	buy := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(src.ID, sec.ID, d1, types.MustNewQuantity("10"),
		&buy, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	if _, err := env.svc.TransferShares(src.ID, dst.ID, sec.ID, d2,
		types.MustNewQuantity("10"), "", nil); err != nil {
		t.Fatalf("TransferShares() error = %v", err)
	}
	p := price.NewPrice(sec.ID, asOf, types.MustNewMoney("110"), price.SourceManual)
	if err := env.priceRepo.Create(p); err != nil {
		t.Fatalf("Create price error = %v", err)
	}

	holdings, err := env.svc.GetHoldings(dst.ID, asOf, ValuationOptions{})
	if err != nil {
		t.Fatalf("GetHoldings() error = %v", err)
	}
	if len(holdings) != 1 {
		t.Fatalf("Expected 1 holding in dest, got %d", len(holdings))
	}
	h := holdings[0]

	if !h.TotalCostDeployed.IsZero() {
		t.Errorf("Expected TotalCostDeployed zero (transfer-only), got %q", h.TotalCostDeployed.String())
	}
	if h.TotalReturnPct != nil {
		t.Errorf("Expected TotalReturnPct nil when TotalCostDeployed is zero, got %v", *h.TotalReturnPct)
	}
	// TotalReturn is still summable: unrealized (shares × (price − avg cost
	// transferred in)) + zero realized + zero dividends − zero fees.
	if h.GainLoss.IsZero() && !h.TotalReturn.IsZero() {
		t.Errorf("Expected TotalReturn zero when only unrealized component (and that is zero), got %q", h.TotalReturn.String())
	}
}

// TR-013: GetAccountValuation populates account-level total-return totals.
// The fixture below covers every component (unrealized, realized, dividends,
// interest, per-security commission, account-level fee, total cost deployed)
// so the assertions exercise the full aggregation path.
//
// Fixture: non-lot Brokerage account with two securities.
//
//	AAPL
//	  Buy 10 @ price 100, $5 commission → total_amount 1005, avg cost 100.
//	  Buy 10 @ price 120, no commission → total_amount 1200, avg cost 110.
//	  Sell 5 @ price 150 → realized (150 − 110) × 5 = 200. Leaves 15 @ 110.
//	  Dividend $50.
//	  Price @ asOf $130 → unrealized 15 × (130 − 110) = 300.
//	    RealizedGain=200, DividendsReceived=50, FeesPaid=5,
//	    TotalCostDeployed=2205, GainLoss=300, TotalReturn=545.
//
//	MSFT
//	  Buy 10 @ price 50, $10 commission → total_amount 510, avg cost 50.
//	  Price @ asOf $60 → unrealized 10 × (60 − 50) = 100.
//	    RealizedGain=0, DividendsReceived=0, FeesPaid=10,
//	    TotalCostDeployed=510, GainLoss=100, TotalReturn=90.
//
//	Account-level
//	  Interest $25 (TransactionTypeInterest).
//	  Fee $30 (account-level TransactionTypeFee, no security_id).
//
// Expected aggregates at the account level:
//
//	TotalGainLoss      = 400        (unrealized only, legacy field)
//	RealizedGain       = 200        (sum of per-holding)
//	DividendsReceived  = 50         (sum of per-holding)
//	InterestReceived   = 25         (account-level helper)
//	FeesPaid           = 45         (5 + 10 + 30, account-level helper)
//	TotalCostDeployed  = 2715       (2205 + 510, account-level helper)
//	TotalReturn        = 630        (400 + 200 + 50 + 25 − 45)
//	TotalReturnPct     ≈ 23.2044    (630 / 2715 × 100)
func TestGetAccountValuation_PopulatesAccountTotals(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	aapl := createSec(t, env.secRepo, "AAPL")
	msft := createSec(t, env.secRepo, "MSFT")
	d1 := types.NewDate(2024, time.March, 1)
	d2 := types.NewDate(2024, time.April, 1)
	d3 := types.NewDate(2024, time.May, 1)
	asOf := types.NewDate(2024, time.June, 1)

	if _, err := env.svc.Deposit(acct.ID, d1, types.MustNewMoney("20000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}

	aaplBuy1 := types.MustNewMoney("1005")
	if _, err := env.svc.Buy(acct.ID, aapl.ID, d1, types.MustNewQuantity("10"),
		&aaplBuy1, nil, types.MustNewMoney("5"), ""); err != nil {
		t.Fatalf("Buy(AAPL 1) error = %v", err)
	}
	aaplBuy2 := types.MustNewMoney("1200")
	if _, err := env.svc.Buy(acct.ID, aapl.ID, d2, types.MustNewQuantity("10"),
		&aaplBuy2, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(AAPL 2) error = %v", err)
	}
	aaplSell := types.MustNewMoney("750")
	if _, err := env.svc.Sell(acct.ID, aapl.ID, d3, types.MustNewQuantity("5"),
		&aaplSell, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(AAPL) error = %v", err)
	}
	if _, err := env.svc.Dividend(acct.ID, aapl.ID, d3, types.MustNewMoney("50"), ""); err != nil {
		t.Fatalf("Dividend(AAPL) error = %v", err)
	}

	msftBuy := types.MustNewMoney("510")
	if _, err := env.svc.Buy(acct.ID, msft.ID, d1, types.MustNewQuantity("10"),
		&msftBuy, nil, types.MustNewMoney("10"), ""); err != nil {
		t.Fatalf("Buy(MSFT) error = %v", err)
	}

	if _, err := env.svc.Interest(acct.ID, d2, types.MustNewMoney("25"), ""); err != nil {
		t.Fatalf("Interest() error = %v", err)
	}
	if _, err := env.svc.Fee(acct.ID, d2, types.MustNewMoney("30"), ""); err != nil {
		t.Fatalf("Fee() error = %v", err)
	}

	if err := env.priceRepo.Create(price.NewPrice(aapl.ID, asOf, types.MustNewMoney("130"), price.SourceManual)); err != nil {
		t.Fatalf("Create AAPL price error = %v", err)
	}
	if err := env.priceRepo.Create(price.NewPrice(msft.ID, asOf, types.MustNewMoney("60"), price.SourceManual)); err != nil {
		t.Fatalf("Create MSFT price error = %v", err)
	}

	val, err := env.svc.GetAccountValuation(acct.ID, asOf, ValuationOptions{})
	if err != nil {
		t.Fatalf("GetAccountValuation() error = %v", err)
	}

	if val.RealizedGain.String() != "200" {
		t.Errorf("Expected RealizedGain '200', got %q", val.RealizedGain.String())
	}
	if val.DividendsReceived.String() != "50" {
		t.Errorf("Expected DividendsReceived '50', got %q", val.DividendsReceived.String())
	}
	if val.InterestReceived.String() != "25" {
		t.Errorf("Expected InterestReceived '25', got %q", val.InterestReceived.String())
	}
	if val.FeesPaid.String() != "45" {
		t.Errorf("Expected FeesPaid '45' (5 + 10 + 30 account-level), got %q", val.FeesPaid.String())
	}
	if val.TotalCostDeployed.String() != "2715" {
		t.Errorf("Expected TotalCostDeployed '2715' (2205 + 510), got %q", val.TotalCostDeployed.String())
	}
	if val.TotalReturn.String() != "630" {
		t.Errorf("Expected TotalReturn '630' (400 + 200 + 50 + 25 − 45), got %q", val.TotalReturn.String())
	}
	if val.TotalReturnPct == nil {
		t.Fatalf("Expected TotalReturnPct non-nil")
	}
	expectedPct := 630.0 / 2715.0 * 100.0
	if math.Abs(*val.TotalReturnPct-expectedPct) > 0.001 {
		t.Errorf("Expected TotalReturnPct ≈ %f, got %f", expectedPct, *val.TotalReturnPct)
	}

	// Per-holding values must match the components that aggregated into the
	// account totals. Order is not guaranteed across holdings paths so look
	// each up by security id.
	if len(val.Holdings) != 2 {
		t.Fatalf("Expected 2 holdings, got %d", len(val.Holdings))
	}
	holdingsBySec := make(map[types.ID]Holding, len(val.Holdings))
	for _, h := range val.Holdings {
		holdingsBySec[h.SecurityID] = h
	}
	hAAPL, ok := holdingsBySec[aapl.ID]
	if !ok {
		t.Fatalf("Expected AAPL holding present")
	}
	hMSFT, ok := holdingsBySec[msft.ID]
	if !ok {
		t.Fatalf("Expected MSFT holding present")
	}
	if hAAPL.RealizedGain.Add(hMSFT.RealizedGain).String() != val.RealizedGain.String() {
		t.Errorf("Account RealizedGain must equal Σ per-holding RealizedGain (AAPL %s + MSFT %s vs account %s)",
			hAAPL.RealizedGain.String(), hMSFT.RealizedGain.String(), val.RealizedGain.String())
	}
	if hAAPL.DividendsReceived.Add(hMSFT.DividendsReceived).String() != val.DividendsReceived.String() {
		t.Errorf("Account DividendsReceived must equal Σ per-holding DividendsReceived")
	}
}

// TR-013: After the total-return aggregation lands, the legacy
// TotalGainLoss / TotalGainPct fields still mean unrealized only and match
// what they did before this feature. They are NOT replaced by TotalReturn.
//
// Uses the same fixture as TestGetAccountValuation_PopulatesAccountTotals.
// Expected legacy values:
//
//	TotalCostBasis = 2150   (AAPL 15 × 110) + (MSFT 10 × 50)
//	MarketValue    = 2550   (AAPL 15 × 130) + (MSFT 10 × 60)
//	TotalGainLoss  = 400    (unrealized only — does NOT include realized 200,
//	                         dividends 50, interest 25, fees 45)
//	TotalGainPct   ≈ 18.605 (400 / 2150 × 100)
func TestGetAccountValuation_LegacyFieldsUnchanged(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	aapl := createSec(t, env.secRepo, "AAPL")
	msft := createSec(t, env.secRepo, "MSFT")
	d1 := types.NewDate(2024, time.March, 1)
	d2 := types.NewDate(2024, time.April, 1)
	d3 := types.NewDate(2024, time.May, 1)
	asOf := types.NewDate(2024, time.June, 1)

	if _, err := env.svc.Deposit(acct.ID, d1, types.MustNewMoney("20000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	aaplBuy1 := types.MustNewMoney("1005")
	if _, err := env.svc.Buy(acct.ID, aapl.ID, d1, types.MustNewQuantity("10"),
		&aaplBuy1, nil, types.MustNewMoney("5"), ""); err != nil {
		t.Fatalf("Buy(AAPL 1) error = %v", err)
	}
	aaplBuy2 := types.MustNewMoney("1200")
	if _, err := env.svc.Buy(acct.ID, aapl.ID, d2, types.MustNewQuantity("10"),
		&aaplBuy2, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(AAPL 2) error = %v", err)
	}
	aaplSell := types.MustNewMoney("750")
	if _, err := env.svc.Sell(acct.ID, aapl.ID, d3, types.MustNewQuantity("5"),
		&aaplSell, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(AAPL) error = %v", err)
	}
	if _, err := env.svc.Dividend(acct.ID, aapl.ID, d3, types.MustNewMoney("50"), ""); err != nil {
		t.Fatalf("Dividend(AAPL) error = %v", err)
	}
	msftBuy := types.MustNewMoney("510")
	if _, err := env.svc.Buy(acct.ID, msft.ID, d1, types.MustNewQuantity("10"),
		&msftBuy, nil, types.MustNewMoney("10"), ""); err != nil {
		t.Fatalf("Buy(MSFT) error = %v", err)
	}
	if _, err := env.svc.Interest(acct.ID, d2, types.MustNewMoney("25"), ""); err != nil {
		t.Fatalf("Interest() error = %v", err)
	}
	if _, err := env.svc.Fee(acct.ID, d2, types.MustNewMoney("30"), ""); err != nil {
		t.Fatalf("Fee() error = %v", err)
	}
	if err := env.priceRepo.Create(price.NewPrice(aapl.ID, asOf, types.MustNewMoney("130"), price.SourceManual)); err != nil {
		t.Fatalf("Create AAPL price error = %v", err)
	}
	if err := env.priceRepo.Create(price.NewPrice(msft.ID, asOf, types.MustNewMoney("60"), price.SourceManual)); err != nil {
		t.Fatalf("Create MSFT price error = %v", err)
	}

	val, err := env.svc.GetAccountValuation(acct.ID, asOf, ValuationOptions{})
	if err != nil {
		t.Fatalf("GetAccountValuation() error = %v", err)
	}

	if val.TotalCostBasis.String() != "2150" {
		t.Errorf("Expected TotalCostBasis '2150' (open cost basis), got %q", val.TotalCostBasis.String())
	}
	if val.MarketValue.String() != "2550" {
		t.Errorf("Expected MarketValue '2550', got %q", val.MarketValue.String())
	}
	if val.TotalGainLoss.String() != "400" {
		t.Errorf("Expected TotalGainLoss '400' (unrealized only, no realized/divs/interest/fees), got %q", val.TotalGainLoss.String())
	}
	expectedPct := 400.0 / 2150.0 * 100.0
	if math.Abs(val.TotalGainPct-expectedPct) > 0.001 {
		t.Errorf("Expected TotalGainPct ≈ %f (unrealized only), got %f", expectedPct, val.TotalGainPct)
	}

	// The new TotalReturn covers everything; assert it is distinct from
	// TotalGainLoss so a future change that conflates the two regresses.
	if val.TotalReturn.String() == val.TotalGainLoss.String() {
		t.Errorf("TotalReturn (%s) must differ from TotalGainLoss (%s) under this fixture",
			val.TotalReturn.String(), val.TotalGainLoss.String())
	}
}

// TR-014: listEverHeldSecurities returns the set of distinct security IDs
// that the account has ever held shares of (open or closed). It draws from
// share-bearing transaction types — buy, sell, reinvest_dividend,
// fee_liquidation, and transfer_shares — so a security that was fully sold
// is still present and an open position is not double-counted. Non-share-
// bearing types (dividend, interest, fee at account level, deposit,
// withdrawal, transfer_cash) do not contribute.

func TestListEverHeldSecurities(t *testing.T) {
	t.Run("open + closed positions both returned", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		aapl := createSec(t, env.secRepo, "AAPL") // still open
		msft := createSec(t, env.secRepo, "MSFT") // fully sold (closed)
		d1 := types.NewDate(2024, time.March, 1)
		d2 := types.NewDate(2024, time.April, 1)

		if _, err := env.svc.Deposit(acct.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		// AAPL: buy and hold.
		buyAAPL := types.MustNewMoney("1000")
		if _, err := env.svc.Buy(acct.ID, aapl.ID, d1, types.MustNewQuantity("10"),
			&buyAAPL, nil, types.ZeroMoney, ""); err != nil {
			t.Fatalf("Buy(AAPL) error = %v", err)
		}

		// MSFT: buy and fully sell — position should be closed.
		buyMSFT := types.MustNewMoney("500")
		if _, err := env.svc.Buy(acct.ID, msft.ID, d1, types.MustNewQuantity("5"),
			&buyMSFT, nil, types.ZeroMoney, ""); err != nil {
			t.Fatalf("Buy(MSFT) error = %v", err)
		}
		sellMSFT := types.MustNewMoney("600")
		if _, err := env.svc.Sell(acct.ID, msft.ID, d2, types.MustNewQuantity("5"),
			&sellMSFT, nil, types.ZeroMoney, "", nil); err != nil {
			t.Fatalf("Sell(MSFT) error = %v", err)
		}

		got, err := env.svc.listEverHeldSecurities(acct.ID)
		if err != nil {
			t.Fatalf("listEverHeldSecurities() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("Expected 2 ever-held securities, got %d: %v", len(got), got)
		}
		seen := make(map[types.ID]bool, len(got))
		for _, id := range got {
			seen[id] = true
		}
		if !seen[aapl.ID] {
			t.Errorf("Expected AAPL (open) to be in ever-held set, got %v", got)
		}
		if !seen[msft.ID] {
			t.Errorf("Expected MSFT (fully sold) to be in ever-held set, got %v", got)
		}
	})

	t.Run("account with no transactions returns empty", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Empty")

		got, err := env.svc.listEverHeldSecurities(acct.ID)
		if err != nil {
			t.Fatalf("listEverHeldSecurities() error = %v", err)
		}
		if len(got) != 0 {
			t.Errorf("Expected empty slice for account with no transactions, got %v", got)
		}
	})

	t.Run("non-share-bearing transactions do not contribute", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		aapl := createSec(t, env.secRepo, "AAPL")
		d1 := types.NewDate(2024, time.March, 1)

		if _, err := env.svc.Deposit(acct.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}
		// A dividend references a security but adds no shares — must NOT
		// place AAPL in the ever-held set on its own.
		if _, err := env.svc.Dividend(acct.ID, aapl.ID, d1, types.MustNewMoney("25"), ""); err != nil {
			t.Fatalf("Dividend() error = %v", err)
		}
		if _, err := env.svc.Interest(acct.ID, d1, types.MustNewMoney("5"), ""); err != nil {
			t.Fatalf("Interest() error = %v", err)
		}
		if _, err := env.svc.Fee(acct.ID, d1, types.MustNewMoney("3"), ""); err != nil {
			t.Fatalf("Fee() error = %v", err)
		}

		got, err := env.svc.listEverHeldSecurities(acct.ID)
		if err != nil {
			t.Fatalf("listEverHeldSecurities() error = %v", err)
		}
		if len(got) != 0 {
			t.Errorf("Expected empty (no share-bearing txns), got %v", got)
		}
	})

	t.Run("transfer-in destination is included", func(t *testing.T) {
		env := createFullTestService(t)
		src := createInvAccount(t, env.accountRepo, "Source")
		dst := createInvAccount(t, env.accountRepo, "Dest")
		sec := createSec(t, env.secRepo, "AAPL")
		d1 := types.NewDate(2024, time.March, 1)
		d2 := types.NewDate(2024, time.April, 1)

		if _, err := env.svc.Deposit(src.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}
		buy := types.MustNewMoney("1000")
		if _, err := env.svc.Buy(src.ID, sec.ID, d1, types.MustNewQuantity("10"),
			&buy, nil, types.ZeroMoney, ""); err != nil {
			t.Fatalf("Buy() error = %v", err)
		}
		if _, err := env.svc.TransferShares(src.ID, dst.ID, sec.ID, d2,
			types.MustNewQuantity("10"), "", nil); err != nil {
			t.Fatalf("TransferShares() error = %v", err)
		}

		got, err := env.svc.listEverHeldSecurities(dst.ID)
		if err != nil {
			t.Fatalf("listEverHeldSecurities() error = %v", err)
		}
		if len(got) != 1 || got[0] != sec.ID {
			t.Errorf("Expected dst's ever-held set to contain only %s, got %v", sec.ID, got)
		}
	})

	t.Run("sibling account ignored", func(t *testing.T) {
		env := createFullTestService(t)
		a := createInvAccount(t, env.accountRepo, "A")
		b := createInvAccount(t, env.accountRepo, "B")
		sec := createSec(t, env.secRepo, "AAPL")
		d1 := types.NewDate(2024, time.March, 1)

		if _, err := env.svc.Deposit(b.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}
		buy := types.MustNewMoney("1000")
		if _, err := env.svc.Buy(b.ID, sec.ID, d1, types.MustNewQuantity("10"),
			&buy, nil, types.ZeroMoney, ""); err != nil {
			t.Fatalf("Buy() error = %v", err)
		}

		got, err := env.svc.listEverHeldSecurities(a.ID)
		if err != nil {
			t.Fatalf("listEverHeldSecurities() error = %v", err)
		}
		if len(got) != 0 {
			t.Errorf("Expected account A (no txns) to have empty set, got %v", got)
		}
	})
}

// TR-015: When opts.IncludeClosed is true, GetHoldings synthesizes a Holding
// row for every security the account has ever held but no longer does. The
// closed row has zero shares / cost basis / market value, IsClosed = true,
// and total-return components (RealizedGain, DividendsReceived, FeesPaid,
// TotalCostDeployed) populated from the ledger. The default
// ValuationOptions{} keeps the legacy behavior of skipping zero-share rows.
//
// Fixture: non-lot Brokerage with two securities.
//   - AAPL: buy 10 @ $100 ($1000), still held — open holding.
//   - MSFT: buy 5 @ $100 ($500 with no commission), one $20 cash dividend,
//     then sell 5 @ $130 ($650 with no commission) — closes the position.
//     Expected closed-row components:
//     RealizedGain      = (130 − 100) × 5 = 150
//     DividendsReceived = 20
//     FeesPaid          = 0
//     TotalCostDeployed = 500
func TestGetHoldings_IncludeClosed_AddsClosedRows(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	aapl := createSec(t, env.secRepo, "AAPL")
	msft := createSec(t, env.secRepo, "MSFT")
	d1 := types.NewDate(2024, time.March, 1)
	d2 := types.NewDate(2024, time.April, 1)
	d3 := types.NewDate(2024, time.May, 1)
	asOf := types.NewDate(2024, time.June, 1)

	if _, err := env.svc.Deposit(acct.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}

	// AAPL: open position.
	buyAAPL := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, aapl.ID, d1, types.MustNewQuantity("10"),
		&buyAAPL, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(AAPL) error = %v", err)
	}
	priceAAPL := price.NewPrice(aapl.ID, asOf, types.MustNewMoney("120"), price.SourceManual)
	if err := env.priceRepo.Create(priceAAPL); err != nil {
		t.Fatalf("Create price AAPL error = %v", err)
	}

	// MSFT: buy, dividend, then fully sell — closed position.
	buyMSFT := types.MustNewMoney("500")
	if _, err := env.svc.Buy(acct.ID, msft.ID, d1, types.MustNewQuantity("5"),
		&buyMSFT, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(MSFT) error = %v", err)
	}
	if _, err := env.svc.Dividend(acct.ID, msft.ID, d2, types.MustNewMoney("20"), ""); err != nil {
		t.Fatalf("Dividend(MSFT) error = %v", err)
	}
	sellMSFT := types.MustNewMoney("650")
	if _, err := env.svc.Sell(acct.ID, msft.ID, d3, types.MustNewQuantity("5"),
		&sellMSFT, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(MSFT) error = %v", err)
	}

	t.Run("default ValuationOptions only returns open holdings", func(t *testing.T) {
		holdings, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetHoldings(default) error = %v", err)
		}
		if len(holdings) != 1 {
			t.Fatalf("Expected 1 open holding, got %d: %+v", len(holdings), holdings)
		}
		if holdings[0].SecurityID != aapl.ID {
			t.Errorf("Expected open holding for AAPL, got %s", holdings[0].SecurityID)
		}
		if holdings[0].IsClosed {
			t.Errorf("Open holding must have IsClosed=false, got true")
		}
	})

	t.Run("IncludeClosed=true appends a synthesized closed row", func(t *testing.T) {
		holdings, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{IncludeClosed: true})
		if err != nil {
			t.Fatalf("GetHoldings(IncludeClosed) error = %v", err)
		}
		if len(holdings) != 2 {
			t.Fatalf("Expected 2 holdings (1 open + 1 closed), got %d", len(holdings))
		}

		var openH, closedH *Holding
		for i := range holdings {
			h := &holdings[i]
			if h.IsClosed {
				closedH = h
			} else {
				openH = h
			}
		}
		if openH == nil {
			t.Fatalf("Expected one open holding, got none")
		}
		if closedH == nil {
			t.Fatalf("Expected one closed holding, got none")
		}

		if openH.SecurityID != aapl.ID {
			t.Errorf("Expected open holding to be AAPL, got %s", openH.SecurityID)
		}
		if closedH.SecurityID != msft.ID {
			t.Errorf("Expected closed holding to be MSFT, got %s", closedH.SecurityID)
		}

		// Closed row is "empty" on open-position numbers.
		if !closedH.Shares.IsZero() {
			t.Errorf("Closed Shares must be zero, got %q", closedH.Shares.String())
		}
		if !closedH.MarketValue.IsZero() {
			t.Errorf("Closed MarketValue must be zero, got %q", closedH.MarketValue.String())
		}
		if !closedH.CostBasis.IsZero() {
			t.Errorf("Closed CostBasis must be zero, got %q", closedH.CostBasis.String())
		}
		if !closedH.GainLoss.IsZero() {
			t.Errorf("Closed GainLoss must be zero, got %q", closedH.GainLoss.String())
		}

		// Total-return components must be populated from the ledger.
		if closedH.RealizedGain.String() != "150" {
			t.Errorf("Closed RealizedGain expected '150' ((130−100)×5), got %q",
				closedH.RealizedGain.String())
		}
		if closedH.DividendsReceived.String() != "20" {
			t.Errorf("Closed DividendsReceived expected '20', got %q",
				closedH.DividendsReceived.String())
		}
		if !closedH.FeesPaid.IsZero() {
			t.Errorf("Closed FeesPaid expected zero (no commissions), got %q",
				closedH.FeesPaid.String())
		}
		if closedH.TotalCostDeployed.String() != "500" {
			t.Errorf("Closed TotalCostDeployed expected '500', got %q",
				closedH.TotalCostDeployed.String())
		}
		// TotalReturn = 0 (unrealized) + 150 (realized) + 20 (div) − 0 (fees) = 170.
		if closedH.TotalReturn.String() != "170" {
			t.Errorf("Closed TotalReturn expected '170' (0+150+20−0), got %q",
				closedH.TotalReturn.String())
		}
		if closedH.TotalReturnPct == nil {
			t.Fatalf("Closed TotalReturnPct must be non-nil when TotalCostDeployed > 0")
		}
		expectedPct := 170.0 / 500.0 * 100.0
		if math.Abs(*closedH.TotalReturnPct-expectedPct) > 0.001 {
			t.Errorf("Closed TotalReturnPct expected ≈ %f, got %f",
				expectedPct, *closedH.TotalReturnPct)
		}
	})
}

// TR-015: When IncludeClosed is true, securities still held must not be
// duplicated — each open security appears exactly once. The synthesized
// closed-position rows include only securities the account no longer holds.
func TestGetHoldings_IncludeClosed_NotDoubleCountingOpen(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	aapl := createSec(t, env.secRepo, "AAPL")
	msft := createSec(t, env.secRepo, "MSFT")
	goog := createSec(t, env.secRepo, "GOOG")
	d1 := types.NewDate(2024, time.March, 1)
	d2 := types.NewDate(2024, time.April, 1)
	asOf := types.NewDate(2024, time.June, 1)

	if _, err := env.svc.Deposit(acct.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}

	// Two open positions.
	buyAAPL := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, aapl.ID, d1, types.MustNewQuantity("10"),
		&buyAAPL, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(AAPL) error = %v", err)
	}
	buyMSFT := types.MustNewMoney("500")
	if _, err := env.svc.Buy(acct.ID, msft.ID, d1, types.MustNewQuantity("5"),
		&buyMSFT, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(MSFT) error = %v", err)
	}

	// One fully-sold (closed) position.
	buyGOOG := types.MustNewMoney("400")
	if _, err := env.svc.Buy(acct.ID, goog.ID, d1, types.MustNewQuantity("4"),
		&buyGOOG, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(GOOG) error = %v", err)
	}
	sellGOOG := types.MustNewMoney("440")
	if _, err := env.svc.Sell(acct.ID, goog.ID, d2, types.MustNewQuantity("4"),
		&sellGOOG, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(GOOG) error = %v", err)
	}

	pAAPL := price.NewPrice(aapl.ID, asOf, types.MustNewMoney("120"), price.SourceManual)
	if err := env.priceRepo.Create(pAAPL); err != nil {
		t.Fatalf("Create price AAPL error = %v", err)
	}
	pMSFT := price.NewPrice(msft.ID, asOf, types.MustNewMoney("110"), price.SourceManual)
	if err := env.priceRepo.Create(pMSFT); err != nil {
		t.Fatalf("Create price MSFT error = %v", err)
	}

	holdings, err := env.svc.GetHoldings(acct.ID, asOf, ValuationOptions{IncludeClosed: true})
	if err != nil {
		t.Fatalf("GetHoldings(IncludeClosed) error = %v", err)
	}
	if len(holdings) != 3 {
		t.Fatalf("Expected 3 holdings (2 open + 1 closed), got %d", len(holdings))
	}

	// Each security must appear exactly once.
	counts := make(map[types.ID]int, 3)
	for _, h := range holdings {
		counts[h.SecurityID]++
	}
	for id, n := range counts {
		if n != 1 {
			t.Errorf("Security %s appears %d times; expected exactly 1", id, n)
		}
	}

	// And the closed flag must match the share state.
	for _, h := range holdings {
		switch h.SecurityID {
		case aapl.ID, msft.ID:
			if h.IsClosed {
				t.Errorf("Open security %s must have IsClosed=false", h.SecurityID)
			}
			if h.Shares.IsZero() {
				t.Errorf("Open security %s must have non-zero shares", h.SecurityID)
			}
		case goog.ID:
			if !h.IsClosed {
				t.Errorf("Closed security %s must have IsClosed=true", h.SecurityID)
			}
			if !h.Shares.IsZero() {
				t.Errorf("Closed security %s must have zero shares, got %q",
					h.SecurityID, h.Shares.String())
			}
		default:
			t.Errorf("Unexpected security %s in holdings", h.SecurityID)
		}
	}
}

// TR-016: AccountValuation.HasClosedPositions advises callers that the
// account has at least one fully-sold security, so the UI can offer the
// "include closed positions" affordance. The flag must be set regardless
// of opts.IncludeClosed — it describes the account's history, not the
// shape of the returned holdings list.
func TestAccountValuation_HasClosedPositionsFlag(t *testing.T) {
	t.Run("flag is true when account has a fully-sold security", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		aapl := createSec(t, env.secRepo, "AAPL")
		msft := createSec(t, env.secRepo, "MSFT")
		d1 := types.NewDate(2024, time.March, 1)
		d2 := types.NewDate(2024, time.April, 1)
		asOf := types.NewDate(2024, time.June, 1)

		if _, err := env.svc.Deposit(acct.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}

		buyAAPL := types.MustNewMoney("1000")
		if _, err := env.svc.Buy(acct.ID, aapl.ID, d1, types.MustNewQuantity("10"),
			&buyAAPL, nil, types.ZeroMoney, ""); err != nil {
			t.Fatalf("Buy(AAPL) error = %v", err)
		}
		buyMSFT := types.MustNewMoney("500")
		if _, err := env.svc.Buy(acct.ID, msft.ID, d1, types.MustNewQuantity("5"),
			&buyMSFT, nil, types.ZeroMoney, ""); err != nil {
			t.Fatalf("Buy(MSFT) error = %v", err)
		}
		sellMSFT := types.MustNewMoney("550")
		if _, err := env.svc.Sell(acct.ID, msft.ID, d2, types.MustNewQuantity("5"),
			&sellMSFT, nil, types.ZeroMoney, "", nil); err != nil {
			t.Fatalf("Sell(MSFT) error = %v", err)
		}

		t.Run("IncludeClosed=false", func(t *testing.T) {
			val, err := env.svc.GetAccountValuation(acct.ID, asOf, ValuationOptions{})
			if err != nil {
				t.Fatalf("GetAccountValuation() error = %v", err)
			}
			if !val.HasClosedPositions {
				t.Errorf("Expected HasClosedPositions=true with a closed position even when IncludeClosed=false, got false")
			}
		})

		t.Run("IncludeClosed=true", func(t *testing.T) {
			val, err := env.svc.GetAccountValuation(acct.ID, asOf, ValuationOptions{IncludeClosed: true})
			if err != nil {
				t.Fatalf("GetAccountValuation() error = %v", err)
			}
			if !val.HasClosedPositions {
				t.Errorf("Expected HasClosedPositions=true with a closed position when IncludeClosed=true, got false")
			}
		})
	})

	t.Run("flag is false when no security has been fully sold", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		aapl := createSec(t, env.secRepo, "AAPL")
		d1 := types.NewDate(2024, time.March, 1)
		asOf := types.NewDate(2024, time.June, 1)

		if _, err := env.svc.Deposit(acct.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
			t.Fatalf("Deposit() error = %v", err)
		}
		buyAAPL := types.MustNewMoney("1000")
		if _, err := env.svc.Buy(acct.ID, aapl.ID, d1, types.MustNewQuantity("10"),
			&buyAAPL, nil, types.ZeroMoney, ""); err != nil {
			t.Fatalf("Buy(AAPL) error = %v", err)
		}

		val, err := env.svc.GetAccountValuation(acct.ID, asOf, ValuationOptions{})
		if err != nil {
			t.Fatalf("GetAccountValuation() error = %v", err)
		}
		if val.HasClosedPositions {
			t.Errorf("Expected HasClosedPositions=false with no closed position, got true")
		}
	})
}

// Account-level RealizedGain and DividendsReceived must include the
// contribution from securities the account no longer holds, regardless of
// opts.IncludeClosed. Without this, an account whose positions have all
// been sold (and which paid dividends along the way) would render the
// portfolio summary's Realized / Div / Total return as $0 even though the
// ledger records the gains.
func TestGetAccountValuation_ClosedPositionsContributeToTotals(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	aapl := createSec(t, env.secRepo, "AAPL")
	d1 := types.NewDate(2024, time.March, 1)
	d2 := types.NewDate(2024, time.April, 1)
	d3 := types.NewDate(2024, time.May, 1)
	asOf := types.NewDate(2024, time.June, 1)

	if _, err := env.svc.Deposit(acct.ID, d1, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	// Buy 10 @ $100, get a $75 dividend, sell all 10 @ $120 → fully closed.
	buyAAPL := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, aapl.ID, d1, types.MustNewQuantity("10"),
		&buyAAPL, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(AAPL) error = %v", err)
	}
	if _, err := env.svc.Dividend(acct.ID, aapl.ID, d2, types.MustNewMoney("75"), ""); err != nil {
		t.Fatalf("Dividend(AAPL) error = %v", err)
	}
	sellAAPL := types.MustNewMoney("1200")
	if _, err := env.svc.Sell(acct.ID, aapl.ID, d3, types.MustNewQuantity("10"),
		&sellAAPL, nil, types.ZeroMoney, "", nil); err != nil {
		t.Fatalf("Sell(AAPL) error = %v", err)
	}

	check := func(t *testing.T, opts ValuationOptions, label string) {
		t.Helper()
		val, err := env.svc.GetAccountValuation(acct.ID, asOf, opts)
		if err != nil {
			t.Fatalf("[%s] GetAccountValuation() error = %v", label, err)
		}
		if val.RealizedGain.String() != "200" {
			t.Errorf("[%s] Expected RealizedGain '200' (1200 − 1000), got %q", label, val.RealizedGain.String())
		}
		if val.DividendsReceived.String() != "75" {
			t.Errorf("[%s] Expected DividendsReceived '75', got %q", label, val.DividendsReceived.String())
		}
		if val.TotalReturn.String() != "275" {
			t.Errorf("[%s] Expected TotalReturn '275' (0 unreal + 200 real + 75 div), got %q", label, val.TotalReturn.String())
		}
	}

	t.Run("IncludeClosed=false", func(t *testing.T) {
		check(t, ValuationOptions{}, "IncludeClosed=false")
	})
	t.Run("IncludeClosed=true", func(t *testing.T) {
		check(t, ValuationOptions{IncludeClosed: true}, "IncludeClosed=true")
	})
}
