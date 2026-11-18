package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// These tests pin the split editor that the transaction dialog opens. On an
// asset account the dialog offers Value Adjustment, and so must its split
// lines: the editor used to leave it out, so a Value Adjustment line opened as
// "(None)" and Save failed with "category is required".

// openTxnSplitEditor opens Edit Transaction on a split transaction of acct,
// the way the register does, and saves it into the split editor.
func openTxnSplitEditor(t *testing.T, f signFixture, acct *account.Account, splits []*transaction.Split) *SplitDialog {
	t.Helper()
	a := newModalTestApp()
	selectAccount(t, a, acct)
	existing := transaction.NewTransaction(acct.ID, types.NewDate(2026, 3, 15), types.MustNewMoney("500.00"))
	data := &transactionDialogData{
		categories:     f.cats,
		accounts:       []*account.Account{acct},
		mode:           transactionDialogModeEdit,
		existing:       existing,
		existingSplits: splits,
	}
	a.txn.applyData(data, a.sidebar.SelectedAccount(), types.ZeroDate)
	if !a.txn.dlg.Fields()[6].Checked {
		t.Fatal("the Split box is not checked for a transaction with splits")
	}
	a.submitTransactionDialog()
	if a.split.editor == nil || !a.split.IsVisible() {
		t.Fatal("Save on a split transaction did not open the split editor")
	}
	return a.split.editor
}

func offersValueAdjustment(sd *SplitDialog) bool {
	for _, o := range sd.categoryOptions {
		if o == category.ValueAdjustmentCategoryName {
			return true
		}
	}
	return false
}

func TestTxnSplit_AssetAccountKeepsValueAdjustmentLine(t *testing.T) {
	f := newSignFixture()
	house := account.NewAccount("Lake Cabin", account.TypeAsset, "USD", types.ZeroMoney, types.NewDate(2020, 1, 1))
	splits := []*transaction.Split{
		{BaseModel: types.BaseModel{ID: types.NewID()}, CategoryID: f.valueAdj.ID, Amount: types.MustNewMoney("450.00")},
		{BaseModel: types.BaseModel{ID: types.NewID()}, CategoryID: f.dining.ID, Amount: types.MustNewMoney("50.00")},
	}

	sd := openTxnSplitEditor(t, f, house, splits)

	if !offersValueAdjustment(sd) {
		t.Fatal("the split editor does not offer Value Adjustment on an asset account")
	}
	if got := sd.categoryOptions[sd.rows[0].categoryIndex]; got != category.ValueAdjustmentCategoryName {
		t.Errorf("line 1 opens on %q, want Value Adjustment", got)
	}
	if err := sd.validate(); err != nil {
		t.Errorf("Save would fail: %v", err)
	}
}

// Value Adjustment stays off the lines of every other account type, as it is
// off the dialog's own Category.
func TestTxnSplit_CheckingAccountHasNoValueAdjustment(t *testing.T) {
	f := newSignFixture()
	checking := account.NewAccount("Everyday Checking", account.TypeChecking, "USD", types.ZeroMoney, types.NewDate(2020, 1, 1))
	splits := []*transaction.Split{
		{BaseModel: types.BaseModel{ID: types.NewID()}, CategoryID: f.dining.ID, Amount: types.MustNewMoney("500.00")},
	}

	sd := openTxnSplitEditor(t, f, checking, splits)

	if offersValueAdjustment(sd) {
		t.Error("the split editor offers Value Adjustment on a checking account")
	}
}
