package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// txn is a tiny helper to build a regular transaction with an amount and status.
func regTxn(amount string, status transaction.Status) *transaction.Transaction {
	return &transaction.Transaction{
		BaseModel: types.BaseModel{ID: types.NewID()},
		Date:      types.Today(),
		Amount:    types.MustNewMoney(amount),
		Status:    status,
	}
}

func TestRunningBalances_AccumulatesNewestFirst(t *testing.T) {
	// Display order is newest-first. Chronologically (oldest->newest):
	//   +2000, -55, -2000  with opening 0  => balances 2000, 1945, -55
	// In newest-first display order the slice is [-2000, -55, +2000].
	txns := []*transaction.Transaction{
		regTxn("-2000", transaction.StatusUncleared), // newest
		regTxn("-55", transaction.StatusUncleared),
		regTxn("2000", transaction.StatusUncleared), // oldest
	}
	got := runningBalances(txns, types.ZeroMoney)

	want := []string{"-55", "1945", "2000"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].String() != types.MustNewMoney(w).String() {
			t.Errorf("balance[%d] = %s, want %s", i, got[i].String(), w)
		}
	}
}

func TestRunningBalances_IncludesOpeningBalance(t *testing.T) {
	// opening 100, one expense of -42.50 => only row's balance is 57.50.
	txns := []*transaction.Transaction{regTxn("-42.50", transaction.StatusCleared)}
	got := runningBalances(txns, types.MustNewMoney("100"))
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].String() != types.MustNewMoney("57.50").String() {
		t.Errorf("balance = %s, want 57.50", got[0].String())
	}
}

func TestRunningBalances_TopRowEqualsCurrentBalance(t *testing.T) {
	// The newest (index 0) balance must equal opening + sum of all non-void.
	txns := []*transaction.Transaction{
		regTxn("-10", transaction.StatusUncleared),
		regTxn("-20", transaction.StatusCleared),
		regTxn("100", transaction.StatusUncleared),
	}
	opening := types.MustNewMoney("5")
	got := runningBalances(txns, opening)
	// 5 + 100 - 20 - 10 = 75
	if got[0].String() != types.MustNewMoney("75").String() {
		t.Errorf("top balance = %s, want 75", got[0].String())
	}
}

func TestRunningBalances_VoidCarriesForward(t *testing.T) {
	// A void row contributes 0; its balance equals the row below it.
	txns := []*transaction.Transaction{
		regTxn("-30", transaction.StatusUncleared), // newest, real
		regTxn("-999", transaction.StatusVoid),     // void: no effect
		regTxn("50", transaction.StatusUncleared),  // oldest, real
	}
	got := runningBalances(txns, types.ZeroMoney)
	// chronological: +50 => 50; void => 50 (carry); -30 => 20
	want := []string{"20", "50", "50"}
	for i, w := range want {
		if got[i].String() != types.MustNewMoney(w).String() {
			t.Errorf("balance[%d] = %s, want %s", i, got[i].String(), w)
		}
	}
}

func TestRunningBalances_Empty(t *testing.T) {
	got := runningBalances(nil, types.MustNewMoney("10"))
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

// invTxn builds an investment transaction of a type with a total amount.
func invTxn(typ investment.TransactionType, total string) *investment.Transaction {
	return &investment.Transaction{
		BaseModel:   types.BaseModel{ID: types.NewID()},
		Date:        types.Today(),
		Type:        typ,
		TotalAmount: types.MustNewMoney(total),
	}
}

func TestRunningCash_OnlyCashAffectingTypes(t *testing.T) {
	// Newest-first display order. Chronologically (oldest->newest):
	//   Deposit +1000, Buy -300, ReinvestDividend (no cash), Dividend +42
	// cash: 1000, 700, 700 (carry), 742
	txns := []*investment.Transaction{
		invTxn(investment.TransactionTypeDividend, "42"),          // newest
		invTxn(investment.TransactionTypeReinvestDividend, "150"), // no cash
		invTxn(investment.TransactionTypeBuy, "-300"),             //
		invTxn(investment.TransactionTypeDeposit, "1000"),         // oldest
	}
	got := runningCash(txns)
	want := []string{"742", "700", "700", "1000"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].String() != types.MustNewMoney(w).String() {
			t.Errorf("cash[%d] = %s, want %s", i, got[i].String(), w)
		}
	}
}

func TestRunningCash_TransferSharesCarriesForward(t *testing.T) {
	// Transfer Shares moves no cash; it carries the prior cash forward.
	txns := []*investment.Transaction{
		invTxn(investment.TransactionTypeTransferShares, "500"), // newest, no cash
		invTxn(investment.TransactionTypeDeposit, "200"),        // oldest
	}
	got := runningCash(txns)
	want := []string{"200", "200"}
	for i, w := range want {
		if got[i].String() != types.MustNewMoney(w).String() {
			t.Errorf("cash[%d] = %s, want %s", i, got[i].String(), w)
		}
	}
}

func TestColumnsFitWidth_RegularRegister(t *testing.T) {
	// Use the real register column set so the boundary tracks balanceColWidth.
	cols := registerColumns(true)
	// need = 5 seps + (10+1+12+balanceColWidth) fixed + (12+5)+(10+5) flex
	want := 5 + (10 + 1 + 12 + balanceColWidth) + (12 + registerFlexMargin) + (10 + registerFlexMargin)
	if columnsFitWidth(cols, want-1, registerFlexMargin) {
		t.Errorf("expected not-fit at tableWidth %d", want-1)
	}
	if !columnsFitWidth(cols, want, registerFlexMargin) {
		t.Errorf("expected fit at tableWidth %d", want)
	}
}

func TestColumnsFitWidth_InvestmentRegister(t *testing.T) {
	cols := investmentRegisterColumns(true)
	// need = 7 seps + (10+1+19+10+12+12+12+balanceColWidth) fixed (no flex columns)
	want := 7 + (10 + 1 + 19 + 10 + 12 + 12 + 12 + balanceColWidth)
	if columnsFitWidth(cols, want-1, registerFlexMargin) {
		t.Errorf("expected not-fit at tableWidth %d", want-1)
	}
	if !columnsFitWidth(cols, want, registerFlexMargin) {
		t.Errorf("expected fit at tableWidth %d", want)
	}
}

func TestColumnsFitWidth_Empty(t *testing.T) {
	if columnsFitWidth(nil, 1000, registerFlexMargin) {
		t.Error("empty column set should not fit")
	}
}
