package scheduled

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// TestService_PostSingleLineTransfer_CarriesCategory verifies that a categorized
// single-line transfer schedule mirrors its category onto both posted legs at
// manual-post time, and that the template keeps the label.
func TestService_PostSingleLineTransfer_CarriesCategory(t *testing.T) {
	env := newTransferTestEnv(t)
	checking := env.account(t, "Checking", account.TypeChecking)
	visa := env.account(t, "Visa", account.TypeCreditCard)
	bills := env.category(t, "Bills")

	st := newTransferSchedule(checking.ID, visa.ID, "200.00")
	st.SetCategory(bills.ID)
	if err := env.svc.Create(st); err != nil {
		t.Fatalf("Create: %v", err)
	}

	from, err := env.svc.Post(st.ID, nil)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}

	// From leg carries the category.
	if !from.HasCategory() || from.CategoryID.ID != bills.ID {
		t.Errorf("from leg category = %+v, want %s", from.CategoryID, bills.ID)
	}

	// To leg (Visa) carries the same category (mirrored pair).
	to, err := env.txnRepo.ListByAccount(visa.ID)
	if err != nil {
		t.Fatalf("ListByAccount(Visa): %v", err)
	}
	if len(to) != 1 {
		t.Fatalf("expected 1 row in Visa, got %d", len(to))
	}
	if !to[0].HasCategory() || to[0].CategoryID.ID != bills.ID {
		t.Errorf("to leg category = %+v, want %s", to[0].CategoryID, bills.ID)
	}

	// Template keeps the label.
	updated, _ := env.svc.GetByID(st.ID)
	if !updated.HasCategory() || updated.CategoryID.ID != bills.ID {
		t.Errorf("template category = %+v, want %s (unchanged)", updated.CategoryID, bills.ID)
	}
}

// TestService_AutoPostTransfer_CarriesCategory verifies auto-post mirrors the
// schedule's category onto both legs of the created pair.
func TestService_AutoPostTransfer_CarriesCategory(t *testing.T) {
	env := newTransferTestEnv(t)
	checking := env.account(t, "Checking", account.TypeChecking)
	savings := env.account(t, "Savings", account.TypeSavings)
	sweep := env.category(t, "Savings Sweep")

	st := newTransferSchedule(checking.ID, savings.ID, "500.00")
	st.SetCategory(sweep.ID)
	st.SetAutoPost(true)
	st.SetPostLeadDays(0)
	if err := env.svc.Create(st); err != nil {
		t.Fatalf("Create: %v", err)
	}

	summary, err := env.svc.AutoPost()
	if err != nil {
		t.Fatalf("AutoPost: %v", err)
	}
	if summary.PostedCount != 1 {
		t.Fatalf("PostedCount = %d, want 1", summary.PostedCount)
	}

	for _, acctID := range []types.ID{checking.ID, savings.ID} {
		rows, err := env.txnRepo.ListByAccount(acctID)
		if err != nil {
			t.Fatalf("ListByAccount: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 row in %s, got %d", acctID, len(rows))
		}
		if !rows[0].HasCategory() || rows[0].CategoryID.ID != sweep.ID {
			t.Errorf("auto-posted leg category = %+v, want %s", rows[0].CategoryID, sweep.ID)
		}
	}
}

// TestService_UncategorizedTransfer_PostsWithoutCategory pins the zero-behavior-
// change goal: an uncategorized transfer schedule still posts a category-free
// pair.
func TestService_UncategorizedTransfer_PostsWithoutCategory(t *testing.T) {
	env := newTransferTestEnv(t)
	checking := env.account(t, "Checking", account.TypeChecking)
	savings := env.account(t, "Savings", account.TypeSavings)

	st := newTransferSchedule(checking.ID, savings.ID, "300.00")
	if err := env.svc.Create(st); err != nil {
		t.Fatalf("Create: %v", err)
	}
	from, err := env.svc.Post(st.ID, nil)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if from.HasCategory() {
		t.Errorf("uncategorized transfer should have no category, got %+v", from.CategoryID)
	}
}

// TestService_BuildMultiLineTransaction_CarriesTransferLineCategory verifies a
// categorized transfer line in a multi-line template carries its category onto
// the posted split, and that counterpart mirroring (Phase 7) copies it onto the
// bank-side paired row in the target account.
func TestService_BuildMultiLineTransaction_CarriesTransferLineCategory(t *testing.T) {
	env := newTransferTestEnv(t)
	checking := env.account(t, "Checking", account.TypeChecking)
	savings := env.account(t, "Savings", account.TypeSavings)
	principal := env.category(t, "Principal")

	net := types.MustNewMoney("800.00")
	st := NewTransactionWithAmount(checking.ID, FrequencyMonthly, types.Today(), net)
	// One categorized expense line + one categorized transfer line, summing to
	// the parent net.
	spend := env.category(t, "Spending")
	spendAmt := types.MustNewMoney("1000.00")
	transferAmt := types.MustNewMoney("-200.00")
	transferLine := NewTransferSplit(st.ID, savings.ID, transferAmt)
	transferLine.CategoryID = types.NullableID{ID: principal.ID, Valid: true}
	st.Splits = SplitCollection{
		NewCategorizedSplit(st.ID, spend.ID, spendAmt),
		transferLine,
	}
	if err := env.svc.Create(st); err != nil {
		t.Fatalf("Create multi-line: %v", err)
	}

	// Unit-level: the built payload's transfer-line split carries the category.
	built, err := env.svc.buildMultiLineTransaction(st, types.Today())
	if err != nil {
		t.Fatalf("buildMultiLineTransaction: %v", err)
	}
	var builtTransfer *transaction.Split
	for _, sp := range built.splits {
		if sp.TransferAccountID.Valid {
			builtTransfer = sp
		}
	}
	if builtTransfer == nil {
		t.Fatal("built payload has no transfer-line split")
	}
	if builtTransfer.CategoryID != principal.ID {
		t.Errorf("built transfer split category = %s, want %s", builtTransfer.CategoryID, principal.ID)
	}

	// End-to-end: post through PostWithDate and confirm both the posted split
	// and the minted counterpart carry the category.
	txn, err := env.svc.PostWithDate(st.ID, types.Today(), nil)
	if err != nil {
		t.Fatalf("PostWithDate: %v", err)
	}

	gotSplits, err := env.splitRepo.ListByTransaction(txn.ID)
	if err != nil {
		t.Fatalf("ListByTransaction: %v", err)
	}
	var postedTransfer *transaction.Split
	for _, sp := range gotSplits {
		if sp.TransferAccountID.Valid {
			postedTransfer = sp
		}
	}
	if postedTransfer == nil {
		t.Fatal("posted split set has no transfer-line split")
	}
	if postedTransfer.CategoryID != principal.ID {
		t.Errorf("posted transfer split category = %s, want %s", postedTransfer.CategoryID, principal.ID)
	}
	if !postedTransfer.TransferID.Valid {
		t.Fatal("posted transfer split has no TransferID")
	}

	// Counterpart in the target (regular) account carries the mirrored category.
	paired, err := env.txnRepo.ListByTransferID(postedTransfer.TransferID.ID)
	if err != nil {
		t.Fatalf("ListByTransferID: %v", err)
	}
	if len(paired) != 1 {
		t.Fatalf("expected 1 counterpart, got %d", len(paired))
	}
	if paired[0].AccountID != savings.ID {
		t.Errorf("counterpart account = %s, want Savings", paired[0].AccountID)
	}
	if !paired[0].HasCategory() || paired[0].CategoryID.ID != principal.ID {
		t.Errorf("counterpart category = %+v, want %s", paired[0].CategoryID, principal.ID)
	}
}
