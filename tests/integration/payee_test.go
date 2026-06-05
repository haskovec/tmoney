package integration

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// createPayeeTestService creates a test database with all repositories and the payee service.
func createPayeeTestService(t *testing.T) (*payee.Service, *db.DB, *payee.Repository, func()) {
	t.Helper()

	database := dbtest.New(t)

	repo := payee.NewRepository(database)
	svc := payee.NewService(repo, database)

	cleanup := func() {}

	return svc, database, repo, cleanup
}

func TestPayeeServiceCreate(t *testing.T) {
	svc, _, _, cleanup := createPayeeTestService(t)
	defer cleanup()

	t.Run("creates valid payee", func(t *testing.T) {
		py := payee.NewPayee("Coffee Shop")
		if err := svc.Create(py); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		retrieved, err := svc.GetByID(py.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve payee: %v", err)
		}
		if retrieved.Name != "Coffee Shop" {
			t.Errorf("Expected name 'Coffee Shop', got %q", retrieved.Name)
		}
	})

	t.Run("rejects empty name", func(t *testing.T) {
		py := payee.NewPayee("")
		err := svc.Create(py)
		if err == nil {
			t.Error("Expected validation error for empty name")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T: %v", err, err)
		}
	})

	t.Run("rejects duplicate name", func(t *testing.T) {
		p1 := payee.NewPayee("Duplicate Payee")
		if err := svc.Create(p1); err != nil {
			t.Fatalf("Failed to create first payee: %v", err)
		}

		p2 := payee.NewPayee("Duplicate Payee")
		err := svc.Create(p2)
		if err == nil {
			t.Error("Expected error for duplicate payee name")
		}
		if _, ok := err.(*dberrors.DuplicateError); !ok {
			t.Errorf("Expected DuplicateError, got %T: %v", err, err)
		}
	})
}

func TestPayeeServiceUpdate(t *testing.T) {
	svc, _, _, cleanup := createPayeeTestService(t)
	defer cleanup()

	t.Run("updates payee name", func(t *testing.T) {
		py := payee.NewPayee("Old Name")
		if err := svc.Create(py); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		py.Name = "New Name"
		py.SetNotes("Some notes")
		if err := svc.Update(py); err != nil {
			t.Fatalf("Failed to update payee: %v", err)
		}

		retrieved, err := svc.GetByID(py.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve payee: %v", err)
		}
		if retrieved.Name != "New Name" {
			t.Errorf("Expected name 'New Name', got %q", retrieved.Name)
		}
		if !retrieved.Notes.Valid || retrieved.Notes.String != "Some notes" {
			t.Errorf("Expected notes 'Some notes', got %v", retrieved.Notes)
		}
	})
}

func TestPayeeServiceGetByName(t *testing.T) {
	svc, _, _, cleanup := createPayeeTestService(t)
	defer cleanup()

	py := payee.NewPayee("Target Payee")
	if err := svc.Create(py); err != nil {
		t.Fatalf("Failed to create payee: %v", err)
	}

	t.Run("finds existing payee by name", func(t *testing.T) {
		found, err := svc.GetByName("Target Payee")
		if err != nil {
			t.Fatalf("Failed to get by name: %v", err)
		}
		if found.ID != py.ID {
			t.Errorf("Expected ID %s, got %s", py.ID.String(), found.ID.String())
		}
	})

	t.Run("returns error for non-existent name", func(t *testing.T) {
		_, err := svc.GetByName("Does Not Exist")
		if err == nil {
			t.Error("Expected error for non-existent payee")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestPayeeServiceList(t *testing.T) {
	svc, _, _, cleanup := createPayeeTestService(t)
	defer cleanup()

	names := []string{"Alpha Payee", "Beta Payee", "Gamma Payee"}
	for _, name := range names {
		if err := svc.Create(payee.NewPayee(name)); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}
	}

	payees, err := svc.List()
	if err != nil {
		t.Fatalf("Failed to list payees: %v", err)
	}
	if len(payees) != 3 {
		t.Errorf("Expected 3 payees, got %d", len(payees))
	}
	// Should be ordered by name
	if payees[0].Name != "Alpha Payee" {
		t.Errorf("Expected first payee 'Alpha Payee', got %q", payees[0].Name)
	}
}

func TestPayeeServiceDelete(t *testing.T) {
	svc, _, _, cleanup := createPayeeTestService(t)
	defer cleanup()

	t.Run("deletes payee without transactions", func(t *testing.T) {
		py := payee.NewPayee("Payee To Delete")
		if err := svc.Create(py); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		if err := svc.Delete(py.ID); err != nil {
			t.Fatalf("Failed to delete payee: %v", err)
		}

		_, err := svc.GetByID(py.ID)
		if err == nil {
			t.Error("Expected error when getting deleted payee")
		}
	})
}

// =============================================================================
// Auto-create Operations
// =============================================================================

func TestPayeeServiceGetOrCreate(t *testing.T) {
	svc, _, _, cleanup := createPayeeTestService(t)
	defer cleanup()

	t.Run("creates new payee when not found", func(t *testing.T) {
		py, created, err := svc.GetOrCreate("New Payee")
		if err != nil {
			t.Fatalf("Failed to get or create: %v", err)
		}
		if !created {
			t.Error("Expected payee to be newly created")
		}
		if py.Name != "New Payee" {
			t.Errorf("Expected name 'New Payee', got %q", py.Name)
		}
	})

	t.Run("returns existing payee when found", func(t *testing.T) {
		// The payee "New Payee" was created in the previous sub-test
		py, created, err := svc.GetOrCreate("New Payee")
		if err != nil {
			t.Fatalf("Failed to get or create: %v", err)
		}
		if created {
			t.Error("Expected payee to already exist")
		}
		if py.Name != "New Payee" {
			t.Errorf("Expected name 'New Payee', got %q", py.Name)
		}
	})
}

func TestPayeeServiceGetOrCreateWithCategory(t *testing.T) {
	svc, database, _, cleanup := createPayeeTestService(t)
	defer cleanup()

	// Create a category
	catRepo := category.NewRepository(database)
	cat := category.NewCategory("Food", category.TypeExpense)
	if err := catRepo.Create(cat); err != nil {
		t.Fatalf("Failed to create category: %v", err)
	}

	t.Run("creates payee with default category", func(t *testing.T) {
		py, created, err := svc.GetOrCreateWithCategory("Restaurant", cat.ID)
		if err != nil {
			t.Fatalf("Failed to get or create: %v", err)
		}
		if !created {
			t.Error("Expected payee to be newly created")
		}
		if !py.HasDefaultCategory() {
			t.Error("Expected payee to have default category")
		}
		if py.DefaultCategoryID.ID != cat.ID {
			t.Errorf("Expected default category %s, got %s",
				cat.ID.String(), py.DefaultCategoryID.ID.String())
		}
	})

	t.Run("returns existing payee without modifying category", func(t *testing.T) {
		// Create a payee without a category
		existing := payee.NewPayee("Grocery Store")
		if err := svc.Create(existing); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		py, created, err := svc.GetOrCreateWithCategory("Grocery Store", cat.ID)
		if err != nil {
			t.Fatalf("Failed to get or create: %v", err)
		}
		if created {
			t.Error("Expected payee to already exist")
		}
		// Existing payee should not have been modified
		if py.HasDefaultCategory() {
			t.Error("Expected existing payee category to remain unchanged")
		}
	})
}

// =============================================================================
// Default Category Operations
// =============================================================================

func TestPayeeServiceDefaultCategory(t *testing.T) {
	svc, database, _, cleanup := createPayeeTestService(t)
	defer cleanup()

	catRepo := category.NewRepository(database)
	cat := category.NewCategory("Utilities", category.TypeExpense)
	if err := catRepo.Create(cat); err != nil {
		t.Fatalf("Failed to create category: %v", err)
	}

	py := payee.NewPayee("Electric Company")
	if err := svc.Create(py); err != nil {
		t.Fatalf("Failed to create payee: %v", err)
	}

	t.Run("set default category", func(t *testing.T) {
		if err := svc.SetDefaultCategory(py.ID, cat.ID); err != nil {
			t.Fatalf("Failed to set default category: %v", err)
		}

		catID, err := svc.GetDefaultCategory(py.ID)
		if err != nil {
			t.Fatalf("Failed to get default category: %v", err)
		}
		if catID == nil {
			t.Fatal("Expected default category, got nil")
		}
		if *catID != cat.ID {
			t.Errorf("Expected category %s, got %s", cat.ID.String(), catID.String())
		}
	})

	t.Run("clear default category", func(t *testing.T) {
		if err := svc.ClearDefaultCategory(py.ID); err != nil {
			t.Fatalf("Failed to clear default category: %v", err)
		}

		catID, err := svc.GetDefaultCategory(py.ID)
		if err != nil {
			t.Fatalf("Failed to get default category: %v", err)
		}
		if catID != nil {
			t.Errorf("Expected nil default category, got %s", catID.String())
		}
	})

	t.Run("returns nil when no default category", func(t *testing.T) {
		noCat := payee.NewPayee("No Category Payee")
		if err := svc.Create(noCat); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		catID, err := svc.GetDefaultCategory(noCat.ID)
		if err != nil {
			t.Fatalf("Failed to get default category: %v", err)
		}
		if catID != nil {
			t.Error("Expected nil default category for new payee")
		}
	})
}

// =============================================================================
// Alias Management Operations
// =============================================================================

func TestPayeeServiceAliasLifecycle(t *testing.T) {
	svc, _, _, cleanup := createPayeeTestService(t)
	defer cleanup()

	py := payee.NewPayee("Amazon")
	if err := svc.Create(py); err != nil {
		t.Fatalf("Failed to create payee: %v", err)
	}

	var aliasID types.ID

	t.Run("create alias", func(t *testing.T) {
		alias := payee.NewContainsAlias(py.ID, "AMZN")
		if err := svc.CreateAlias(alias); err != nil {
			t.Fatalf("Failed to create alias: %v", err)
		}
		aliasID = alias.ID
	})

	t.Run("get alias by ID", func(t *testing.T) {
		alias, err := svc.GetAliasByID(aliasID)
		if err != nil {
			t.Fatalf("Failed to get alias: %v", err)
		}
		if alias.Pattern != "AMZN" {
			t.Errorf("Expected pattern 'AMZN', got %q", alias.Pattern)
		}
		if alias.MatchType != payee.MatchTypeContains {
			t.Errorf("Expected match type 'contains', got %q", alias.MatchType)
		}
	})

	t.Run("get aliases by payee", func(t *testing.T) {
		// Add another alias
		alias2 := payee.NewExactAlias(py.ID, "Amazon.com")
		if err := svc.CreateAlias(alias2); err != nil {
			t.Fatalf("Failed to create second alias: %v", err)
		}

		aliases, err := svc.GetAliasesByPayee(py.ID)
		if err != nil {
			t.Fatalf("Failed to get aliases: %v", err)
		}
		if len(aliases) != 2 {
			t.Errorf("Expected 2 aliases, got %d", len(aliases))
		}
	})

	t.Run("update alias", func(t *testing.T) {
		alias, err := svc.GetAliasByID(aliasID)
		if err != nil {
			t.Fatalf("Failed to get alias: %v", err)
		}

		alias.Pattern = "AMAZON"
		if err := svc.UpdateAlias(alias); err != nil {
			t.Fatalf("Failed to update alias: %v", err)
		}

		updated, err := svc.GetAliasByID(aliasID)
		if err != nil {
			t.Fatalf("Failed to get updated alias: %v", err)
		}
		if updated.Pattern != "AMAZON" {
			t.Errorf("Expected pattern 'AMAZON', got %q", updated.Pattern)
		}
	})

	t.Run("delete alias", func(t *testing.T) {
		if err := svc.DeleteAlias(aliasID); err != nil {
			t.Fatalf("Failed to delete alias: %v", err)
		}

		_, err := svc.GetAliasByID(aliasID)
		if err == nil {
			t.Error("Expected error when getting deleted alias")
		}
	})
}

// =============================================================================
// Pattern Matching Operations
// =============================================================================

func TestPayeeServicePatternMatching(t *testing.T) {
	svc, _, _, cleanup := createPayeeTestService(t)
	defer cleanup()

	// Create payees with aliases
	amazon := payee.NewPayee("Amazon")
	if err := svc.Create(amazon); err != nil {
		t.Fatalf("Failed to create payee: %v", err)
	}
	if err := svc.CreateAlias(payee.NewContainsAlias(amazon.ID, "AMZN")); err != nil {
		t.Fatalf("Failed to create alias: %v", err)
	}
	if err := svc.CreateAlias(payee.NewStartsWithAlias(amazon.ID, "Amazon")); err != nil {
		t.Fatalf("Failed to create alias: %v", err)
	}

	starbucks := payee.NewPayee("Starbucks")
	if err := svc.Create(starbucks); err != nil {
		t.Fatalf("Failed to create payee: %v", err)
	}
	if err := svc.CreateAlias(payee.NewContainsAlias(starbucks.ID, "SBUX")); err != nil {
		t.Fatalf("Failed to create alias: %v", err)
	}

	t.Run("FindPayeeByPattern with contains match", func(t *testing.T) {
		found, err := svc.FindPayeeByPattern("Payment AMZN Marketplace")
		if err != nil {
			t.Fatalf("Failed to find by pattern: %v", err)
		}
		if found == nil {
			t.Fatal("Expected to find Amazon payee")
		}
		if found.ID != amazon.ID {
			t.Errorf("Expected Amazon, got %q", found.Name)
		}
	})

	t.Run("FindPayeeByPattern with starts_with match", func(t *testing.T) {
		found, err := svc.FindPayeeByPattern("Amazon Prime")
		if err != nil {
			t.Fatalf("Failed to find by pattern: %v", err)
		}
		if found == nil {
			t.Fatal("Expected to find Amazon payee")
		}
		if found.ID != amazon.ID {
			t.Errorf("Expected Amazon, got %q", found.Name)
		}
	})

	t.Run("FindPayeeByPattern case-insensitive", func(t *testing.T) {
		found, err := svc.FindPayeeByPattern("payment amzn marketplace")
		if err != nil {
			t.Fatalf("Failed to find by pattern: %v", err)
		}
		if found == nil {
			t.Fatal("Expected to find Amazon payee")
		}
		if found.ID != amazon.ID {
			t.Errorf("Expected Amazon, got %q", found.Name)
		}
	})

	t.Run("FindPayeeByPattern no match returns nil", func(t *testing.T) {
		found, err := svc.FindPayeeByPattern("Walmart Purchase")
		if err != nil {
			t.Fatalf("Failed to find by pattern: %v", err)
		}
		if found != nil {
			t.Errorf("Expected nil, got payee %q", found.Name)
		}
	})

	t.Run("ResolvePayee by exact name", func(t *testing.T) {
		found, err := svc.ResolvePayee("Starbucks")
		if err != nil {
			t.Fatalf("Failed to resolve: %v", err)
		}
		if found == nil {
			t.Fatal("Expected to find Starbucks")
		}
		if found.ID != starbucks.ID {
			t.Errorf("Expected Starbucks, got %q", found.Name)
		}
	})

	t.Run("ResolvePayee by alias", func(t *testing.T) {
		found, err := svc.ResolvePayee("SBUX Store #1234")
		if err != nil {
			t.Fatalf("Failed to resolve: %v", err)
		}
		if found == nil {
			t.Fatal("Expected to find Starbucks via alias")
		}
		if found.ID != starbucks.ID {
			t.Errorf("Expected Starbucks, got %q", found.Name)
		}
	})

	t.Run("ResolvePayee returns nil when not found", func(t *testing.T) {
		found, err := svc.ResolvePayee("Unknown Store")
		if err != nil {
			t.Fatalf("Failed to resolve: %v", err)
		}
		if found != nil {
			t.Errorf("Expected nil, got payee %q", found.Name)
		}
	})

	t.Run("ResolveOrCreate creates when not found", func(t *testing.T) {
		py, created, err := svc.ResolveOrCreate("Brand New Store")
		if err != nil {
			t.Fatalf("Failed to resolve or create: %v", err)
		}
		if !created {
			t.Error("Expected payee to be newly created")
		}
		if py.Name != "Brand New Store" {
			t.Errorf("Expected name 'Brand New Store', got %q", py.Name)
		}
	})

	t.Run("ResolveOrCreate finds existing by alias", func(t *testing.T) {
		py, created, err := svc.ResolveOrCreate("SBUX Downtown")
		if err != nil {
			t.Fatalf("Failed to resolve or create: %v", err)
		}
		if created {
			t.Error("Expected to find existing payee via alias")
		}
		if py.ID != starbucks.ID {
			t.Errorf("Expected Starbucks, got %q", py.Name)
		}
	})
}

// =============================================================================
// Merge Operations
// =============================================================================

func TestPayeeServiceMerge(t *testing.T) {
	svc, database, _, cleanup := createPayeeTestService(t)
	defer cleanup()

	accountRepo := account.NewRepository(database)
	txnRepo := transaction.NewRepository(database)

	// Create an account for transactions
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(acct); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	t.Run("merge payees moves transactions and aliases", func(t *testing.T) {
		source := payee.NewPayee("Source Payee")
		target := payee.NewPayee("Target Payee")
		if err := svc.Create(source); err != nil {
			t.Fatalf("Failed to create source: %v", err)
		}
		if err := svc.Create(target); err != nil {
			t.Fatalf("Failed to create target: %v", err)
		}

		// Create a transaction assigned to the source payee
		txn := transaction.NewTransactionWithPayee(acct.ID, types.Today(), types.MustNewMoney("-25.00"), source.ID)
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}

		// Create an alias on the source payee
		alias := payee.NewContainsAlias(source.ID, "SRC")
		if err := svc.CreateAlias(alias); err != nil {
			t.Fatalf("Failed to create alias: %v", err)
		}

		// Merge source into target
		if err := svc.MergePayees(source.ID, target.ID); err != nil {
			t.Fatalf("Failed to merge payees: %v", err)
		}

		// Source should no longer exist
		_, err := svc.GetByID(source.ID)
		if err == nil {
			t.Error("Expected source payee to be deleted after merge")
		}

		// Transaction should now reference target
		updatedTxn, err := txnRepo.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("Failed to get transaction: %v", err)
		}
		if !updatedTxn.PayeeID.Valid || updatedTxn.PayeeID.ID != target.ID {
			t.Errorf("Expected transaction payee to be target %s, got %v",
				target.ID.String(), updatedTxn.PayeeID)
		}

		// Target should have the reassigned alias
		aliases, err := svc.GetAliasesByPayee(target.ID)
		if err != nil {
			t.Fatalf("Failed to get target aliases: %v", err)
		}
		// Should have the original alias ("SRC") plus a new exact alias for source name
		if len(aliases) < 1 {
			t.Errorf("Expected at least 1 alias on target, got %d", len(aliases))
		}
	})

	t.Run("rejects merging payee into itself", func(t *testing.T) {
		py := payee.NewPayee("Self Payee")
		if err := svc.Create(py); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		err := svc.MergePayees(py.ID, py.ID)
		if err == nil {
			t.Error("Expected error when merging payee into itself")
		}
		if _, ok := err.(*payee.MergeSameError); !ok {
			t.Errorf("Expected PayeeMergeSameError, got %T: %v", err, err)
		}
	})
}
