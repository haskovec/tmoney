package payee

import (
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

func TestNewService(t *testing.T) {
	t.Run("creates service with repository", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		if svc == nil {
			t.Fatal("NewService should not return nil")
		}
		if svc.repo != repo {
			t.Error("NewService should store repository")
		}
	})
}

func TestService_Create(t *testing.T) {
	t.Run("creates valid payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("Test Payee")
		err := svc.Create(p)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify it was created
		retrieved, err := svc.GetByID(p.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Name != "Test Payee" {
			t.Errorf("Expected name 'Test Payee', got %q", retrieved.Name)
		}
	})

	t.Run("validates payee before creating", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("") // Invalid: empty name
		err := svc.Create(p)
		if err == nil {
			t.Error("Create() expected error for invalid payee")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("rejects duplicate name", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p1 := NewPayee("Duplicate")
		if err := svc.Create(p1); err != nil {
			t.Fatalf("Create first payee error = %v", err)
		}

		p2 := NewPayee("Duplicate")
		err := svc.Create(p2)
		if err == nil {
			t.Error("Create() expected error for duplicate name")
		}
	})
}

func TestService_GetByName(t *testing.T) {
	t.Run("returns payee by name", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("Grocery Store")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := svc.GetByName("Grocery Store")
		if err != nil {
			t.Fatalf("GetByName() error = %v", err)
		}
		if retrieved.ID != p.ID {
			t.Error("GetByName returned wrong payee")
		}
	})

	t.Run("returns error for non-existent name", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		_, err := svc.GetByName("Non Existent")
		if err == nil {
			t.Error("GetByName() expected error for non-existent payee")
		}
	})
}

func TestService_Update(t *testing.T) {
	t.Run("updates payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("Original")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		p.Name = "Updated"
		if err := svc.Update(p); err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, err := svc.GetByID(p.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Name != "Updated" {
			t.Errorf("Expected name 'Updated', got %q", retrieved.Name)
		}
	})

	t.Run("validates payee before updating", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("Valid")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		p.Name = "" // Invalid
		err := svc.Update(p)
		if err == nil {
			t.Error("Update() expected error for invalid payee")
		}
	})
}

func TestService_Delete(t *testing.T) {
	t.Run("deletes payee without transactions", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("ToDelete")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.Delete(p.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, err := svc.GetByID(p.ID)
		if err == nil {
			t.Error("GetByID() expected error after delete")
		}
	})
}

func TestService_List(t *testing.T) {
	t.Run("returns all payees", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		if err := svc.Create(NewPayee("Payee1")); err != nil {
			t.Fatalf("Create error = %v", err)
		}
		if err := svc.Create(NewPayee("Payee2")); err != nil {
			t.Fatalf("Create error = %v", err)
		}

		payees, err := svc.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(payees) != 2 {
			t.Errorf("Expected 2 payees, got %d", len(payees))
		}
	})
}

func TestService_GetOrCreate(t *testing.T) {
	t.Run("returns existing payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		existing := NewPayee("Existing")
		if err := svc.Create(existing); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		p, created, err := svc.GetOrCreate("Existing")
		if err != nil {
			t.Fatalf("GetOrCreate() error = %v", err)
		}
		if created {
			t.Error("GetOrCreate() should not create existing payee")
		}
		if p.ID != existing.ID {
			t.Error("GetOrCreate() returned wrong payee")
		}
	})

	t.Run("creates new payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p, created, err := svc.GetOrCreate("New Payee")
		if err != nil {
			t.Fatalf("GetOrCreate() error = %v", err)
		}
		if !created {
			t.Error("GetOrCreate() should create new payee")
		}
		if p.Name != "New Payee" {
			t.Errorf("Expected name 'New Payee', got %q", p.Name)
		}

		// Verify it was persisted
		retrieved, err := svc.GetByName("New Payee")
		if err != nil {
			t.Fatalf("GetByName() error = %v", err)
		}
		if retrieved.ID != p.ID {
			t.Error("Created payee not found in database")
		}
	})

	t.Run("second call returns same payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p1, created1, err := svc.GetOrCreate("Test")
		if err != nil {
			t.Fatalf("First GetOrCreate() error = %v", err)
		}
		if !created1 {
			t.Error("First call should create")
		}

		p2, created2, err := svc.GetOrCreate("Test")
		if err != nil {
			t.Fatalf("Second GetOrCreate() error = %v", err)
		}
		if created2 {
			t.Error("Second call should not create")
		}
		if p1.ID != p2.ID {
			t.Error("Both calls should return same payee")
		}
	})
}

func TestService_GetOrCreateWithCategory(t *testing.T) {
	t.Run("creates payee with default category", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		// Create a category using raw SQL
		categoryID := types.NewID()
		_, err := database.Conn().Exec(`
			INSERT INTO categories (id, name, parent_id, type, system_category, created_at, updated_at)
			VALUES (?, 'Groceries', NULL, 'expense', false, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, categoryID)
		if err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		p, created, err := svc.GetOrCreateWithCategory("Store", categoryID)
		if err != nil {
			t.Fatalf("GetOrCreateWithCategory() error = %v", err)
		}
		if !created {
			t.Error("Should create new payee")
		}
		if !p.HasDefaultCategory() {
			t.Error("Payee should have default category")
		}
		if p.DefaultCategoryID.ID != categoryID {
			t.Error("Default category ID mismatch")
		}
	})

	t.Run("does not override existing payee category", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		// Create two categories using raw SQL
		cat1ID := types.NewID()
		cat2ID := types.NewID()
		_, err := database.Conn().Exec(`
			INSERT INTO categories (id, name, parent_id, type, system_category, created_at, updated_at)
			VALUES (?, 'Food', NULL, 'expense', false, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, cat1ID)
		if err != nil {
			t.Fatalf("Create cat1 error = %v", err)
		}
		_, err = database.Conn().Exec(`
			INSERT INTO categories (id, name, parent_id, type, system_category, created_at, updated_at)
			VALUES (?, 'Shopping', NULL, 'expense', false, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, cat2ID)
		if err != nil {
			t.Fatalf("Create cat2 error = %v", err)
		}

		// Create payee with first category
		existing := NewPayeeWithCategory("Store", cat1ID)
		if err := svc.Create(existing); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		// Try to get or create with second category
		p, created, err := svc.GetOrCreateWithCategory("Store", cat2ID)
		if err != nil {
			t.Fatalf("GetOrCreateWithCategory() error = %v", err)
		}
		if created {
			t.Error("Should not create - payee exists")
		}
		// Should still have original category
		if p.DefaultCategoryID.ID != cat1ID {
			t.Error("Should not override existing category")
		}
	})
}

func TestService_ResolvePayee(t *testing.T) {
	t.Run("finds by exact name", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("Coffee Shop")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		resolved, err := svc.ResolvePayee("Coffee Shop")
		if err != nil {
			t.Fatalf("ResolvePayee() error = %v", err)
		}
		if resolved == nil {
			t.Fatal("ResolvePayee() should find payee")
		}
		if resolved.ID != p.ID {
			t.Error("ResolvePayee() returned wrong payee")
		}
	})

	t.Run("finds by alias pattern", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("Amazon")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		alias := NewContainsAlias(p.ID, "AMZN")
		if err := svc.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		resolved, err := svc.ResolvePayee("AMZN MARKETPLACE")
		if err != nil {
			t.Fatalf("ResolvePayee() error = %v", err)
		}
		if resolved == nil {
			t.Fatal("ResolvePayee() should find payee by alias")
		}
		if resolved.ID != p.ID {
			t.Error("ResolvePayee() returned wrong payee")
		}
	})

	t.Run("returns nil for no match", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		resolved, err := svc.ResolvePayee("Unknown")
		if err != nil {
			t.Fatalf("ResolvePayee() error = %v", err)
		}
		if resolved != nil {
			t.Error("ResolvePayee() should return nil for no match")
		}
	})
}

func TestService_ResolveOrCreate(t *testing.T) {
	t.Run("returns existing payee by name", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		existing := NewPayee("Existing")
		if err := svc.Create(existing); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		p, created, err := svc.ResolveOrCreate("Existing")
		if err != nil {
			t.Fatalf("ResolveOrCreate() error = %v", err)
		}
		if created {
			t.Error("Should not create - payee exists")
		}
		if p.ID != existing.ID {
			t.Error("Wrong payee returned")
		}
	})

	t.Run("returns payee matched by alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("Netflix")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		alias := NewStartsWithAlias(p.ID, "NETFLIX.COM")
		if err := svc.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		resolved, created, err := svc.ResolveOrCreate("NETFLIX.COM PAYMENT")
		if err != nil {
			t.Fatalf("ResolveOrCreate() error = %v", err)
		}
		if created {
			t.Error("Should not create - matched by alias")
		}
		if resolved.ID != p.ID {
			t.Error("Wrong payee returned")
		}
	})

	t.Run("creates new payee when not found", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p, created, err := svc.ResolveOrCreate("New Store")
		if err != nil {
			t.Fatalf("ResolveOrCreate() error = %v", err)
		}
		if !created {
			t.Error("Should create new payee")
		}
		if p.Name != "New Store" {
			t.Errorf("Expected name 'New Store', got %q", p.Name)
		}
	})
}

func TestService_DefaultCategory(t *testing.T) {
	t.Run("sets default category", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		// Create category using raw SQL
		categoryID := types.NewID()
		_, err := database.Conn().Exec(`
			INSERT INTO categories (id, name, parent_id, type, system_category, created_at, updated_at)
			VALUES (?, 'Food', NULL, 'expense', false, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, categoryID)
		if err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		// Create payee without category
		p := NewPayee("Restaurant")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		// Set default category
		if err := svc.SetDefaultCategory(p.ID, categoryID); err != nil {
			t.Fatalf("SetDefaultCategory() error = %v", err)
		}

		// Verify
		catID, err := svc.GetDefaultCategory(p.ID)
		if err != nil {
			t.Fatalf("GetDefaultCategory() error = %v", err)
		}
		if catID == nil {
			t.Fatal("GetDefaultCategory() should return category")
		}
		if *catID != categoryID {
			t.Error("Wrong category ID")
		}
	})

	t.Run("clears default category", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		// Create category using raw SQL
		categoryID := types.NewID()
		_, err := database.Conn().Exec(`
			INSERT INTO categories (id, name, parent_id, type, system_category, created_at, updated_at)
			VALUES (?, 'Food', NULL, 'expense', false, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, categoryID)
		if err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		// Create payee with category
		p := NewPayeeWithCategory("Restaurant", categoryID)
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		// Clear category
		if err := svc.ClearDefaultCategory(p.ID); err != nil {
			t.Fatalf("ClearDefaultCategory() error = %v", err)
		}

		// Verify
		catID, err := svc.GetDefaultCategory(p.ID)
		if err != nil {
			t.Fatalf("GetDefaultCategory() error = %v", err)
		}
		if catID != nil {
			t.Error("GetDefaultCategory() should return nil after clear")
		}
	})

	t.Run("returns nil for payee without category", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("No Category")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		catID, err := svc.GetDefaultCategory(p.ID)
		if err != nil {
			t.Fatalf("GetDefaultCategory() error = %v", err)
		}
		if catID != nil {
			t.Error("GetDefaultCategory() should return nil")
		}
	})
}

func TestService_AliasOperations(t *testing.T) {
	t.Run("creates alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("Amazon")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		alias := NewContainsAlias(p.ID, "AMZN")
		if err := svc.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		retrieved, err := svc.GetAliasByID(alias.ID)
		if err != nil {
			t.Fatalf("GetAliasByID() error = %v", err)
		}
		if retrieved.Pattern != "AMZN" {
			t.Errorf("Expected pattern 'AMZN', got %q", retrieved.Pattern)
		}
	})

	t.Run("validates alias before creating", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("Test")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		alias := NewContainsAlias(p.ID, "") // Invalid: empty pattern
		err := svc.CreateAlias(alias)
		if err == nil {
			t.Error("CreateAlias() expected error for invalid alias")
		}
	})

	t.Run("gets aliases by payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("Amazon")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		alias1 := NewContainsAlias(p.ID, "AMZN")
		alias2 := NewStartsWithAlias(p.ID, "Amazon")
		if err := svc.CreateAlias(alias1); err != nil {
			t.Fatalf("CreateAlias 1 error = %v", err)
		}
		if err := svc.CreateAlias(alias2); err != nil {
			t.Fatalf("CreateAlias 2 error = %v", err)
		}

		aliases, err := svc.GetAliasesByPayee(p.ID)
		if err != nil {
			t.Fatalf("GetAliasesByPayee() error = %v", err)
		}
		if len(aliases) != 2 {
			t.Errorf("Expected 2 aliases, got %d", len(aliases))
		}
	})

	t.Run("updates alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("Test")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		alias := NewContainsAlias(p.ID, "OLD")
		if err := svc.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		alias.Pattern = "NEW"
		if err := svc.UpdateAlias(alias); err != nil {
			t.Fatalf("UpdateAlias() error = %v", err)
		}

		retrieved, err := svc.GetAliasByID(alias.ID)
		if err != nil {
			t.Fatalf("GetAliasByID() error = %v", err)
		}
		if retrieved.Pattern != "NEW" {
			t.Errorf("Expected pattern 'NEW', got %q", retrieved.Pattern)
		}
	})

	t.Run("deletes alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("Test")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		alias := NewContainsAlias(p.ID, "DELETE")
		if err := svc.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		if err := svc.DeleteAlias(alias.ID); err != nil {
			t.Fatalf("DeleteAlias() error = %v", err)
		}

		_, err := svc.GetAliasByID(alias.ID)
		if err == nil {
			t.Error("GetAliasByID() expected error after delete")
		}
	})
}

func TestService_FindPayeeByPattern(t *testing.T) {
	t.Run("finds payee by exact alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("Starbucks")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		alias := NewExactAlias(p.ID, "STARBUCKS COFFEE")
		if err := svc.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		found, err := svc.FindPayeeByPattern("starbucks coffee") // case insensitive
		if err != nil {
			t.Fatalf("FindPayeeByPattern() error = %v", err)
		}
		if found == nil {
			t.Fatal("FindPayeeByPattern() should find payee")
		}
		if found.ID != p.ID {
			t.Error("FindPayeeByPattern() returned wrong payee")
		}
	})

	t.Run("finds payee by contains alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("Shell Gas")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		alias := NewContainsAlias(p.ID, "SHELL")
		if err := svc.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		found, err := svc.FindPayeeByPattern("SHELL GAS STATION #1234")
		if err != nil {
			t.Fatalf("FindPayeeByPattern() error = %v", err)
		}
		if found == nil {
			t.Fatal("FindPayeeByPattern() should find payee")
		}
		if found.ID != p.ID {
			t.Error("FindPayeeByPattern() returned wrong payee")
		}
	})

	t.Run("finds payee by starts_with alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("Walmart")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		alias := NewStartsWithAlias(p.ID, "WAL-MART")
		if err := svc.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		found, err := svc.FindPayeeByPattern("WAL-MART STORE #5678")
		if err != nil {
			t.Fatalf("FindPayeeByPattern() error = %v", err)
		}
		if found == nil {
			t.Fatal("FindPayeeByPattern() should find payee")
		}
	})

	t.Run("finds payee by regex alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("Target")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		alias := NewRegexAlias(p.ID, "TARGET.*#[0-9]+")
		if err := svc.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		found, err := svc.FindPayeeByPattern("TARGET STORE #9999")
		if err != nil {
			t.Fatalf("FindPayeeByPattern() error = %v", err)
		}
		if found == nil {
			t.Fatal("FindPayeeByPattern() should find payee by regex")
		}
	})

	t.Run("returns nil when no alias matches", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		found, err := svc.FindPayeeByPattern("UNKNOWN STORE")
		if err != nil {
			t.Fatalf("FindPayeeByPattern() error = %v", err)
		}
		if found != nil {
			t.Error("FindPayeeByPattern() should return nil for no match")
		}
	})
}

func TestService_MergePayees(t *testing.T) {
	t.Run("merges source into target", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		source := NewPayee("Old Store")
		target := NewPayee("New Store")
		if err := svc.Create(source); err != nil {
			t.Fatalf("Create source error = %v", err)
		}
		if err := svc.Create(target); err != nil {
			t.Fatalf("Create target error = %v", err)
		}

		if err := svc.MergePayees(source.ID, target.ID); err != nil {
			t.Fatalf("MergePayees() error = %v", err)
		}

		// Source should be deleted
		_, err := svc.GetByID(source.ID)
		if err == nil {
			t.Error("Source payee should be deleted after merge")
		}

		// Target should still exist
		_, err = svc.GetByID(target.ID)
		if err != nil {
			t.Errorf("Target payee should still exist: %v", err)
		}
	})

	t.Run("rejects merge into self", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		p := NewPayee("Self")
		if err := svc.Create(p); err != nil {
			t.Fatalf("Create error = %v", err)
		}

		err := svc.MergePayees(p.ID, p.ID)
		if err == nil {
			t.Error("MergePayees() expected error when merging into self")
		}
		if _, ok := err.(*MergeSameError); !ok {
			t.Errorf("Expected MergeSameError, got %T: %v", err, err)
		}
	})

	t.Run("reassigns aliases during merge", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		source := NewPayee("Old")
		target := NewPayee("New")
		if err := svc.Create(source); err != nil {
			t.Fatalf("Create source error = %v", err)
		}
		if err := svc.Create(target); err != nil {
			t.Fatalf("Create target error = %v", err)
		}

		// Create alias on source
		alias := NewContainsAlias(source.ID, "OLD PATTERN")
		if err := svc.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		// Merge
		if err := svc.MergePayees(source.ID, target.ID); err != nil {
			t.Fatalf("MergePayees() error = %v", err)
		}

		// Alias should now belong to target
		aliases, err := svc.GetAliasesByPayee(target.ID)
		if err != nil {
			t.Fatalf("GetAliasesByPayee() error = %v", err)
		}

		// Should have original alias + the one created for source name
		found := false
		for _, a := range aliases {
			if a.Pattern == "OLD PATTERN" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Alias should be reassigned to target")
		}
	})

	t.Run("creates alias for source name during merge", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		source := NewPayee("Old Store Name")
		target := NewPayee("New Store Name")
		if err := svc.Create(source); err != nil {
			t.Fatalf("Create source error = %v", err)
		}
		if err := svc.Create(target); err != nil {
			t.Fatalf("Create target error = %v", err)
		}

		// Merge
		if err := svc.MergePayees(source.ID, target.ID); err != nil {
			t.Fatalf("MergePayees() error = %v", err)
		}

		// Should be able to find target by old name
		found, err := svc.FindPayeeByPattern("Old Store Name")
		if err != nil {
			t.Fatalf("FindPayeeByPattern() error = %v", err)
		}
		if found == nil {
			t.Fatal("Should find target by old source name")
		}
		if found.ID != target.ID {
			t.Error("Found wrong payee")
		}
	})
}
