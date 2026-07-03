package scheduled

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// scheduledSplitFixtures builds an account, target account, category, and a
// parent scheduled transaction ready to receive child split items. Returned
// IDs are: source account, target account, category, scheduled transaction.
func scheduledSplitFixtures(t *testing.T) (
	srcAcct, dstAcct *account.Account,
	cat *category.Category,
	st *Transaction,
	splitRepo *SplitRepository,
) {
	t.Helper()
	database := createTestDB(t)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	stRepo := NewRepository(database)
	splitRepo = NewSplitRepository(database)

	now := time.Now()
	today := types.NewDate(now.Year(), now.Month(), now.Day())

	srcAcct = account.NewAccount("Checking", account.TypeChecking, "USD",
		types.ZeroMoney, today)
	if err := accountRepo.Create(srcAcct); err != nil {
		t.Fatalf("Create source account: %v", err)
	}
	dstAcct = account.NewAccount("401k", account.TypeSavings, "USD",
		types.ZeroMoney, today)
	if err := accountRepo.Create(dstAcct); err != nil {
		t.Fatalf("Create dest account: %v", err)
	}

	cat = category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(cat); err != nil {
		t.Fatalf("Create category: %v", err)
	}

	st = NewTransaction(srcAcct.ID, FrequencyMonthly, today)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("Create scheduled transaction: %v", err)
	}
	return srcAcct, dstAcct, cat, st, splitRepo
}

func TestScheduledSplitItemRepo_RoundTrip(t *testing.T) {
	t.Run("create and read categorized split", func(t *testing.T) {
		_, _, cat, st, splitRepo := scheduledSplitFixtures(t)

		split := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("5000.00"))
		split.SetMemo("Gross pay")
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create categorized split: %v", err)
		}

		got, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if !got.CategoryID.Valid || got.CategoryID.ID != cat.ID {
			t.Errorf("CategoryID = %+v, want valid %v", got.CategoryID, cat.ID)
		}
		if got.TransferAccountID.Valid {
			t.Errorf("TransferAccountID should be invalid, got %v", got.TransferAccountID.ID)
		}
		if !got.Amount.Equal(types.MustNewMoney("5000.00")) {
			t.Errorf("Amount = %s, want 5000.00", got.Amount.String())
		}
		if !got.Memo.Valid || got.Memo.String != "Gross pay" {
			t.Errorf("Memo = %+v, want 'Gross pay'", got.Memo)
		}
	})

	t.Run("create and read transfer-line split", func(t *testing.T) {
		_, dstAcct, _, st, splitRepo := scheduledSplitFixtures(t)

		split := NewTransferSplit(st.ID, dstAcct.ID, types.MustNewMoney("-500.00"))
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create transfer split: %v", err)
		}

		got, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.CategoryID.Valid {
			t.Errorf("CategoryID should be invalid, got %v", got.CategoryID.ID)
		}
		if !got.TransferAccountID.Valid || got.TransferAccountID.ID != dstAcct.ID {
			t.Errorf("TransferAccountID = %+v, want valid %v", got.TransferAccountID, dstAcct.ID)
		}
		if !got.Amount.Equal(types.MustNewMoney("-500.00")) {
			t.Errorf("Amount = %s, want -500.00", got.Amount.String())
		}
	})

	t.Run("list by scheduled transaction returns both types", func(t *testing.T) {
		_, dstAcct, cat, st, splitRepo := scheduledSplitFixtures(t)

		categorized := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("5000.00"))
		if err := splitRepo.Create(categorized); err != nil {
			t.Fatalf("Create categorized: %v", err)
		}
		transfer := NewTransferSplit(st.ID, dstAcct.ID, types.MustNewMoney("-500.00"))
		if err := splitRepo.Create(transfer); err != nil {
			t.Fatalf("Create transfer: %v", err)
		}

		list, err := splitRepo.ListByScheduledTransaction(st.ID)
		if err != nil {
			t.Fatalf("ListByScheduledTransaction: %v", err)
		}
		if len(list) != 2 {
			t.Fatalf("expected 2 splits, got %d", len(list))
		}

		var foundCategorized, foundTransfer bool
		for _, s := range list {
			switch s.ID {
			case categorized.ID:
				foundCategorized = true
				if !s.CategoryID.Valid || s.CategoryID.ID != cat.ID {
					t.Errorf("categorized CategoryID mismatch: %+v", s.CategoryID)
				}
				if s.TransferAccountID.Valid {
					t.Errorf("categorized TransferAccountID should be null")
				}
			case transfer.ID:
				foundTransfer = true
				if s.CategoryID.Valid {
					t.Errorf("transfer CategoryID should be null")
				}
				if !s.TransferAccountID.Valid || s.TransferAccountID.ID != dstAcct.ID {
					t.Errorf("transfer TransferAccountID mismatch: %+v", s.TransferAccountID)
				}
			}
		}
		if !foundCategorized || !foundTransfer {
			t.Errorf("missing rows: foundCategorized=%v foundTransfer=%v",
				foundCategorized, foundTransfer)
		}
	})

	t.Run("update categorized split amount and memo", func(t *testing.T) {
		_, _, cat, st, splitRepo := scheduledSplitFixtures(t)

		split := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("1000.00"))
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create: %v", err)
		}

		split.Amount = types.MustNewMoney("1200.00")
		split.SetMemo("revised")
		if err := splitRepo.Update(split); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID after update: %v", err)
		}
		if !got.Amount.Equal(types.MustNewMoney("1200.00")) {
			t.Errorf("Amount = %s, want 1200.00", got.Amount.String())
		}
		if !got.Memo.Valid || got.Memo.String != "revised" {
			t.Errorf("Memo = %+v, want 'revised'", got.Memo)
		}
	})

	t.Run("update transfer split target account", func(t *testing.T) {
		_, dstAcct, _, st, splitRepo := scheduledSplitFixtures(t)

		// Build a second target account so we can swap to it.
		now := time.Now()
		today := types.NewDate(now.Year(), now.Month(), now.Day())
		dst2 := account.NewAccount("HSA", account.TypeSavings, "USD",
			types.ZeroMoney, today)
		if err := account.NewRepository(splitRepo.db).Create(dst2); err != nil {
			t.Fatalf("Create dst2: %v", err)
		}

		split := NewTransferSplit(st.ID, dstAcct.ID, types.MustNewMoney("-200.00"))
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create: %v", err)
		}

		split.TransferAccountID = types.NullableID{ID: dst2.ID, Valid: true}
		split.Amount = types.MustNewMoney("-150.00")
		if err := splitRepo.Update(split); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID after update: %v", err)
		}
		if !got.TransferAccountID.Valid || got.TransferAccountID.ID != dst2.ID {
			t.Errorf("TransferAccountID = %+v, want %v", got.TransferAccountID, dst2.ID)
		}
		if !got.Amount.Equal(types.MustNewMoney("-150.00")) {
			t.Errorf("Amount = %s, want -150.00", got.Amount.String())
		}
		if got.CategoryID.Valid {
			t.Errorf("CategoryID should remain null after transfer-target swap, got %v",
				got.CategoryID.ID)
		}
	})

	t.Run("delete removes the row", func(t *testing.T) {
		_, _, cat, st, splitRepo := scheduledSplitFixtures(t)

		split := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("100.00"))
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create: %v", err)
		}

		if err := splitRepo.Delete(split.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		_, err := splitRepo.GetByID(split.ID)
		if err == nil {
			t.Fatal("GetByID after delete: expected error, got nil")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("delete non-existent returns NotFoundError", func(t *testing.T) {
		_, _, _, _, splitRepo := scheduledSplitFixtures(t)

		err := splitRepo.Delete(types.NewID())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("create rejects non-existent scheduled transaction", func(t *testing.T) {
		_, _, cat, _, splitRepo := scheduledSplitFixtures(t)

		split := NewCategorizedSplit(types.NewID(), cat.ID, types.MustNewMoney("100.00"))
		err := splitRepo.Create(split)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("create rejects non-existent category", func(t *testing.T) {
		_, _, _, st, splitRepo := scheduledSplitFixtures(t)

		split := NewCategorizedSplit(st.ID, types.NewID(), types.MustNewMoney("100.00"))
		err := splitRepo.Create(split)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("create rejects non-existent transfer account", func(t *testing.T) {
		_, _, _, st, splitRepo := scheduledSplitFixtures(t)

		split := NewTransferSplit(st.ID, types.NewID(), types.MustNewMoney("-100.00"))
		err := splitRepo.Create(split)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("create accepts a categorized transfer (both category and transfer)", func(t *testing.T) {
		_, dstAcct, cat, st, splitRepo := scheduledSplitFixtures(t)

		split := NewTransferSplit(st.ID, dstAcct.ID, types.MustNewMoney("-100.00"))
		split.CategoryID = types.NullableID{ID: cat.ID, Valid: true}
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create categorized transfer scheduled split: %v", err)
		}

		got, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if !got.CategoryID.Valid || got.CategoryID.ID != cat.ID {
			t.Errorf("CategoryID = %+v, want valid %v", got.CategoryID, cat.ID)
		}
		if !got.TransferAccountID.Valid || got.TransferAccountID.ID != dstAcct.ID {
			t.Errorf("TransferAccountID = %+v, want valid %v", got.TransferAccountID, dstAcct.ID)
		}
	})

	t.Run("create rejects a categorized transfer whose category does not exist", func(t *testing.T) {
		// Pins the migration-029 verifyReferences fall-through: with the
		// transfer account set, the category check must still run (the old
		// code short-circuited past it).
		_, dstAcct, _, st, splitRepo := scheduledSplitFixtures(t)

		split := NewTransferSplit(st.ID, dstAcct.ID, types.MustNewMoney("-100.00"))
		split.CategoryID = types.NullableID{ID: types.NewID(), Valid: true}
		err := splitRepo.Create(split)
		if err == nil {
			t.Fatal("expected error for categorized transfer with non-existent category, got nil")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("expected NotFoundError (category), got %T: %v", err, err)
		}
	})

	t.Run("count by scheduled transaction", func(t *testing.T) {
		_, dstAcct, cat, st, splitRepo := scheduledSplitFixtures(t)

		got, err := splitRepo.CountByScheduledTransaction(st.ID)
		if err != nil {
			t.Fatalf("CountByScheduledTransaction: %v", err)
		}
		if got != 0 {
			t.Errorf("CountByScheduledTransaction empty = %d, want 0", got)
		}

		if err := splitRepo.Create(NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("100.00"))); err != nil {
			t.Fatalf("Create 1: %v", err)
		}
		if err := splitRepo.Create(NewTransferSplit(st.ID, dstAcct.ID, types.MustNewMoney("-100.00"))); err != nil {
			t.Fatalf("Create 2: %v", err)
		}

		got, err = splitRepo.CountByScheduledTransaction(st.ID)
		if err != nil {
			t.Fatalf("CountByScheduledTransaction: %v", err)
		}
		if got != 2 {
			t.Errorf("CountByScheduledTransaction = %d, want 2", got)
		}
	})

	t.Run("delete by scheduled transaction removes all children", func(t *testing.T) {
		_, dstAcct, cat, st, splitRepo := scheduledSplitFixtures(t)

		if err := splitRepo.Create(NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("100.00"))); err != nil {
			t.Fatalf("Create 1: %v", err)
		}
		if err := splitRepo.Create(NewTransferSplit(st.ID, dstAcct.ID, types.MustNewMoney("-100.00"))); err != nil {
			t.Fatalf("Create 2: %v", err)
		}

		n, err := splitRepo.DeleteByScheduledTransaction(st.ID)
		if err != nil {
			t.Fatalf("DeleteByScheduledTransaction: %v", err)
		}
		if n != 2 {
			t.Errorf("DeleteByScheduledTransaction = %d, want 2", n)
		}

		count, err := splitRepo.CountByScheduledTransaction(st.ID)
		if err != nil {
			t.Fatalf("CountByScheduledTransaction: %v", err)
		}
		if count != 0 {
			t.Errorf("CountByScheduledTransaction after delete = %d, want 0", count)
		}
	})
}

func TestScheduledRepo_LoadsChildren(t *testing.T) {
	t.Run("GetByID populates Splits with both shapes", func(t *testing.T) {
		_, dstAcct, cat, st, splitRepo := scheduledSplitFixtures(t)
		stRepo := NewRepository(splitRepo.db)

		categorized := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("5000.00"))
		if err := splitRepo.Create(categorized); err != nil {
			t.Fatalf("Create categorized: %v", err)
		}
		transfer := NewTransferSplit(st.ID, dstAcct.ID, types.MustNewMoney("-500.00"))
		if err := splitRepo.Create(transfer); err != nil {
			t.Fatalf("Create transfer: %v", err)
		}

		got, err := stRepo.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if len(got.Splits) != 2 {
			t.Fatalf("Splits length = %d, want 2", len(got.Splits))
		}

		var sawCategorized, sawTransfer bool
		for _, s := range got.Splits {
			switch s.ID {
			case categorized.ID:
				sawCategorized = true
				if !s.CategoryID.Valid || s.CategoryID.ID != cat.ID {
					t.Errorf("categorized CategoryID mismatch in GetByID load")
				}
				if !s.Amount.Equal(types.MustNewMoney("5000.00")) {
					t.Errorf("categorized Amount = %s, want 5000.00", s.Amount.String())
				}
			case transfer.ID:
				sawTransfer = true
				if !s.TransferAccountID.Valid || s.TransferAccountID.ID != dstAcct.ID {
					t.Errorf("transfer TransferAccountID mismatch in GetByID load")
				}
				if !s.Amount.Equal(types.MustNewMoney("-500.00")) {
					t.Errorf("transfer Amount = %s, want -500.00", s.Amount.String())
				}
			}
		}
		if !sawCategorized || !sawTransfer {
			t.Errorf("missing children in GetByID load: cat=%v xfer=%v",
				sawCategorized, sawTransfer)
		}
	})

	t.Run("GetByID returns empty Splits for single-line scheduled transaction", func(t *testing.T) {
		_, _, _, st, splitRepo := scheduledSplitFixtures(t)
		stRepo := NewRepository(splitRepo.db)

		got, err := stRepo.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if len(got.Splits) != 0 {
			t.Errorf("Splits should be empty, got %d", len(got.Splits))
		}
	})

	t.Run("List populates Splits", func(t *testing.T) {
		_, _, cat, st, splitRepo := scheduledSplitFixtures(t)
		stRepo := NewRepository(splitRepo.db)

		categorized := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("250.00"))
		if err := splitRepo.Create(categorized); err != nil {
			t.Fatalf("Create: %v", err)
		}

		list, err := stRepo.List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("expected 1 scheduled transaction, got %d", len(list))
		}
		if len(list[0].Splits) != 1 {
			t.Fatalf("Splits length = %d, want 1", len(list[0].Splits))
		}
		if list[0].Splits[0].ID != categorized.ID {
			t.Errorf("loaded split ID mismatch")
		}
	})

	t.Run("Delete cascades to child splits", func(t *testing.T) {
		_, _, cat, st, splitRepo := scheduledSplitFixtures(t)
		stRepo := NewRepository(splitRepo.db)

		split := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("100.00"))
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create: %v", err)
		}

		if err := stRepo.Delete(st.ID); err != nil {
			t.Fatalf("Delete scheduled transaction: %v", err)
		}

		count, err := splitRepo.CountByScheduledTransaction(st.ID)
		if err != nil {
			t.Fatalf("CountByScheduledTransaction: %v", err)
		}
		if count != 0 {
			t.Errorf("expected child splits to be cascade-deleted, found %d", count)
		}
	})
}

// =============================================================================
// PaycheckSection Round-Trip (PW2-002)
// =============================================================================

func TestSplitItemRepo_PaycheckSection_RoundTrip(t *testing.T) {
	t.Run("categorized tag round-trips through Create + GetByID", func(t *testing.T) {
		_, _, cat, st, splitRepo := scheduledSplitFixtures(t)

		split := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("5000.00"))
		split.PaycheckSection = types.NullableString{String: "earnings", Valid: true}
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if !got.PaycheckSection.Valid || got.PaycheckSection.String != "earnings" {
			t.Errorf("PaycheckSection = %+v, want valid 'earnings'", got.PaycheckSection)
		}
	})

	t.Run("transfer-line tag round-trips through List", func(t *testing.T) {
		_, dstAcct, _, st, splitRepo := scheduledSplitFixtures(t)

		split := NewTransferSplit(st.ID, dstAcct.ID, types.MustNewMoney("-200.00"))
		split.PaycheckSection = types.NullableString{String: "pre_tax", Valid: true}
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create: %v", err)
		}

		list, err := splitRepo.ListByScheduledTransaction(st.ID)
		if err != nil {
			t.Fatalf("ListByScheduledTransaction: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("list length = %d, want 1", len(list))
		}
		if !list[0].PaycheckSection.Valid || list[0].PaycheckSection.String != "pre_tax" {
			t.Errorf("ListByScheduledTransaction PaycheckSection = %+v, want valid 'pre_tax'",
				list[0].PaycheckSection)
		}
	})

	t.Run("tag survives Update", func(t *testing.T) {
		_, _, cat, st, splitRepo := scheduledSplitFixtures(t)

		split := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("5000.00"))
		split.PaycheckSection = types.NullableString{String: "post_tax", Valid: true}
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create: %v", err)
		}

		split.Amount = types.MustNewMoney("4500.00")
		if err := splitRepo.Update(split); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID after update: %v", err)
		}
		if !got.PaycheckSection.Valid || got.PaycheckSection.String != "post_tax" {
			t.Errorf("PaycheckSection after Update = %+v, want valid 'post_tax'",
				got.PaycheckSection)
		}
	})

	t.Run("parent loader (GetByID) returns tagged splits", func(t *testing.T) {
		_, _, cat, st, splitRepo := scheduledSplitFixtures(t)
		stRepo := NewRepository(splitRepo.db)

		split := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("5000.00"))
		split.PaycheckSection = types.NullableString{String: "earnings", Valid: true}
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := stRepo.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID parent: %v", err)
		}
		if len(got.Splits) != 1 {
			t.Fatalf("Splits length = %d, want 1", len(got.Splits))
		}
		if !got.Splits[0].PaycheckSection.Valid ||
			got.Splits[0].PaycheckSection.String != "earnings" {
			t.Errorf("parent-loaded PaycheckSection = %+v, want valid 'earnings'",
				got.Splits[0].PaycheckSection)
		}
	})
}

func TestSplitItemRepo_NullPaycheckSection_RoundTrip(t *testing.T) {
	t.Run("unset tag stays NULL through Create + GetByID", func(t *testing.T) {
		_, _, cat, st, splitRepo := scheduledSplitFixtures(t)

		// Generic multi-line dialog path: PaycheckSection left at zero value.
		split := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("5000.00"))
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.PaycheckSection.Valid {
			t.Errorf("PaycheckSection should be NULL, got %+v", got.PaycheckSection)
		}
	})

	t.Run("unset tag stays NULL through Update", func(t *testing.T) {
		_, _, cat, st, splitRepo := scheduledSplitFixtures(t)

		split := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("5000.00"))
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create: %v", err)
		}

		split.Amount = types.MustNewMoney("4500.00")
		if err := splitRepo.Update(split); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID after update: %v", err)
		}
		if got.PaycheckSection.Valid {
			t.Errorf("PaycheckSection should remain NULL after Update, got %+v",
				got.PaycheckSection)
		}
	})
}

// =============================================================================
// LoanSection Round-Trip (loan-wizard phase 4)
// =============================================================================

func TestSplitItemRepo_LoanSection_RoundTrip(t *testing.T) {
	t.Run("interest tag round-trips through Create + GetByID", func(t *testing.T) {
		_, _, cat, st, splitRepo := scheduledSplitFixtures(t)

		split := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("-1200.00"))
		split.LoanSection = types.NullableString{String: "interest", Valid: true}
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if !got.LoanSection.Valid || got.LoanSection.String != "interest" {
			t.Errorf("LoanSection = %+v, want valid 'interest'", got.LoanSection)
		}
	})

	t.Run("principal transfer-line tag round-trips through List", func(t *testing.T) {
		_, dstAcct, _, st, splitRepo := scheduledSplitFixtures(t)

		split := NewTransferSplit(st.ID, dstAcct.ID, types.MustNewMoney("-800.00"))
		split.LoanSection = types.NullableString{String: "principal", Valid: true}
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create: %v", err)
		}

		list, err := splitRepo.ListByScheduledTransaction(st.ID)
		if err != nil {
			t.Fatalf("ListByScheduledTransaction: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("list length = %d, want 1", len(list))
		}
		if !list[0].LoanSection.Valid || list[0].LoanSection.String != "principal" {
			t.Errorf("ListByScheduledTransaction LoanSection = %+v, want valid 'principal'",
				list[0].LoanSection)
		}
	})

	t.Run("re-tagging round-trips through Update", func(t *testing.T) {
		_, _, cat, st, splitRepo := scheduledSplitFixtures(t)

		// Create tagged 'interest', then re-tag to 'escrow' via Update. The
		// mutation matters: if Update failed to write loan_section, the column
		// would remain 'interest' and this assertion would catch it (a test
		// that kept the tag constant across Update would pass even with the
		// write dropped).
		split := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("-650.00"))
		split.LoanSection = types.NullableString{String: "interest", Valid: true}
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create: %v", err)
		}

		split.LoanSection = types.NullableString{String: "escrow", Valid: true}
		split.Amount = types.MustNewMoney("-700.00")
		if err := splitRepo.Update(split); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID after update: %v", err)
		}
		if !got.LoanSection.Valid || got.LoanSection.String != "escrow" {
			t.Errorf("LoanSection after Update = %+v, want valid 'escrow'",
				got.LoanSection)
		}
	})

	t.Run("parent loader (GetByID) returns tagged splits", func(t *testing.T) {
		_, _, cat, st, splitRepo := scheduledSplitFixtures(t)
		stRepo := NewRepository(splitRepo.db)

		split := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("-1200.00"))
		split.LoanSection = types.NullableString{String: "interest", Valid: true}
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := stRepo.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID parent: %v", err)
		}
		if len(got.Splits) != 1 {
			t.Fatalf("Splits length = %d, want 1", len(got.Splits))
		}
		if !got.Splits[0].LoanSection.Valid ||
			got.Splits[0].LoanSection.String != "interest" {
			t.Errorf("parent-loaded LoanSection = %+v, want valid 'interest'",
				got.Splits[0].LoanSection)
		}
	})
}

func TestSplitItemRepo_NullLoanSection_RoundTrip(t *testing.T) {
	t.Run("unset tag stays NULL through Create + GetByID", func(t *testing.T) {
		_, _, cat, st, splitRepo := scheduledSplitFixtures(t)

		// Generic multi-line dialog path: LoanSection left at zero value.
		split := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("5000.00"))
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.LoanSection.Valid {
			t.Errorf("LoanSection should be NULL, got %+v", got.LoanSection)
		}
	})

	t.Run("clearing the tag to NULL round-trips through Update", func(t *testing.T) {
		_, _, cat, st, splitRepo := scheduledSplitFixtures(t)

		// Create tagged 'escrow', then clear the tag via Update. The mutation
		// matters: if Update failed to write loan_section, the column would
		// remain 'escrow' and this assertion would catch it.
		split := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("5000.00"))
		split.LoanSection = types.NullableString{String: "escrow", Valid: true}
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create: %v", err)
		}

		split.LoanSection = types.NullableString{Valid: false}
		split.Amount = types.MustNewMoney("4500.00")
		if err := splitRepo.Update(split); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID after update: %v", err)
		}
		if got.LoanSection.Valid {
			t.Errorf("LoanSection should be NULL after clearing via Update, got %+v",
				got.LoanSection)
		}
	})

	t.Run("a paycheck-tagged split leaves LoanSection NULL and round-trips", func(t *testing.T) {
		_, _, cat, st, splitRepo := scheduledSplitFixtures(t)

		// A wizard family is mutually exclusive: setting PaycheckSection must
		// not disturb LoanSection (and vice versa), and both must round-trip.
		split := NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney("5000.00"))
		split.PaycheckSection = types.NullableString{String: "earnings", Valid: true}
		if err := splitRepo.Create(split); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := splitRepo.GetByID(split.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if !got.PaycheckSection.Valid || got.PaycheckSection.String != "earnings" {
			t.Errorf("PaycheckSection = %+v, want valid 'earnings'", got.PaycheckSection)
		}
		if got.LoanSection.Valid {
			t.Errorf("LoanSection should be NULL for a paycheck split, got %+v", got.LoanSection)
		}
	})
}
