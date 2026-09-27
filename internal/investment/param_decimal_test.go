package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

func TestParseParamDecimal(t *testing.T) {
	for _, in := range []string{"0.10", " 1.1 ", "2", "0.9851"} {
		if _, err := ParseParamDecimal(in); err != nil {
			t.Errorf("ParseParamDecimal(%q) error = %v", in, err)
		}
	}
	for _, in := range []string{"", "abc", "1.2.3"} {
		if _, err := ParseParamDecimal(in); err == nil {
			t.Errorf("ParseParamDecimal(%q) succeeded, want an error", in)
		}
	}
}

// TestMergerParams_StoredShape pins what a new merger row writes: bare JSON
// numbers with the exact digits typed, the shape the old float64 fields wrote,
// with a zero cash field left out.
func TestMergerParams_StoredShape(t *testing.T) {
	cases := []struct {
		params MergerParams
		want   string
	}{
		{MergerParams{ExchangeRatio: paramDec("1.1"), CashPerShare: paramDec("0.10")}, `{"exchange_ratio":1.1,"cash_per_share":0.1}`},
		{MergerParams{ExchangeRatio: paramDec("0.5")}, `{"exchange_ratio":0.5}`},
	}
	for _, tc := range cases {
		got, err := tc.params.ToJSON()
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("ToJSON() = %s, want %s", got, tc.want)
		}
	}
}

// TestParseMergerParams_ReadsOldAndNewRows: rows written by the float64
// fields hold bare numbers; a quoted decimal string also loads. Both read
// exactly, without a pass through float64.
func TestParseMergerParams_ReadsOldAndNewRows(t *testing.T) {
	cases := []struct {
		name, raw, ratio, cash string
	}{
		{"old number row", `{"exchange_ratio":2.5,"cash_per_share":1.5}`, "2.5", "1.5"},
		{"old row, no cash", `{"exchange_ratio":0.5}`, "0.5", "0"},
		{"new row", `{"exchange_ratio":1.1,"cash_per_share":0.1}`, "1.1", "0.1"},
		{"quoted strings", `{"exchange_ratio":"1.1","cash_per_share":"0.10"}`, "1.1", "0.1"},
		{"inverted by migration 035", `{"exchange_ratio":0.3333333333333333}`, "0.3333333333333333", "0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mp, err := ParseMergerParams(tc.raw)
			if err != nil {
				t.Fatalf("ParseMergerParams(%s) error = %v", tc.raw, err)
			}
			if mp.ExchangeRatio.String() != tc.ratio {
				t.Errorf("exchange ratio = %s, want %s", mp.ExchangeRatio, tc.ratio)
			}
			if mp.CashPerShare.String() != tc.cash {
				t.Errorf("cash per share = %s, want %s", mp.CashPerShare, tc.cash)
			}
		})
	}
}

// TestCorporateActionService_Merger_ExactDecimals locks the amounts that a
// binary float cannot hold: 0.10 cash per share and a 1.1 ratio. 100 source
// shares become exactly 110 target shares and exactly $10.00 of cash, on the
// lot path and the position path.
func TestCorporateActionService_Merger_ExactDecimals(t *testing.T) {
	cases := []struct {
		name    string
		account func(*testing.T, *testCAServiceEnv) types.ID
	}{
		{"lot path", func(t *testing.T, env *testCAServiceEnv) types.ID {
			return createLotTrackingAccount(t, env.accountRepo, "Brokerage").ID
		}},
		{"position path", func(t *testing.T, env *testCAServiceEnv) types.ID {
			return createInvAccount(t, env.accountRepo, "Brokerage").ID
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := createCATestEnv(t)
			acctID := tc.account(t, env)
			sourceSec := createSec(t, env.secRepo, "OLD")
			targetSec := createSec(t, env.secRepo, "NEW")
			date := types.NewDate(2024, time.January, 15)

			if _, err := env.invSvc.Deposit(acctID, date, types.MustNewMoney("2000.00"), ""); err != nil {
				t.Fatal(err)
			}
			total := types.MustNewMoney("1000.00")
			if _, err := env.invSvc.Buy(acctID, sourceSec.ID, date, types.MustNewQuantity("100"), &total, nil, types.ZeroMoney, ""); err != nil {
				t.Fatal(err)
			}
			cashBefore, err := env.invSvc.GetCashBalance(acctID)
			if err != nil {
				t.Fatal(err)
			}

			params := MergerParams{ExchangeRatio: paramDec("1.1"), CashPerShare: paramDec("0.10")}
			if _, err := env.caSvc.Merger(sourceSec.ID, targetSec.ID, types.NewDate(2024, time.June, 1), params); err != nil {
				t.Fatalf("Merger() error = %v", err)
			}

			var shares types.Quantity
			if lots, _ := env.lotRepo.ListByAccountAndSecurity(acctID, targetSec.ID, false); len(lots) > 0 {
				shares = lots[0].Shares
			} else {
				pos, err := env.positionRepo.GetByAccountAndSecurity(acctID, targetSec.ID)
				if err != nil {
					t.Fatal(err)
				}
				shares = pos.Shares
			}
			if !shares.Equal(types.MustNewQuantity("110")) {
				t.Errorf("target shares = %s, want exactly 110", shares)
			}

			cashAfter, err := env.invSvc.GetCashBalance(acctID)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := cashAfter.Sub(cashBefore), types.MustNewMoney("10.00"); !got.Equal(want) {
				t.Errorf("cash consideration = %s, want exactly %s", got, want)
			}
		})
	}
}
