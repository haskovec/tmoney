package repository

import (
	"path/filepath"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
)

func createTestDB(t *testing.T) *db.DB {
	t.Helper()
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(path)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	t.Cleanup(func() {
		database.Close()
	})

	return database
}

// =============================================================================
// Payee CRUD Tests
// =============================================================================

func TestPayeeRepository_Create(t *testing.T) {
	t.Run("creates valid payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Coffee Shop")
		err := repo.Create(payee)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify it was created
		retrieved, err := repo.GetByID(payee.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Name != "Coffee Shop" {
			t.Errorf("Expected name 'Coffee Shop', got %q", retrieved.Name)
		}
	})

	t.Run("creates payee with default category", func(t *testing.T) {
		database := createTestDB(t)
		payeeRepo := NewPayeeRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create a category first
		category := models.NewCategory("Food", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		payee := models.NewPayeeWithCategory("Restaurant", category.ID)
		err := payeeRepo.Create(payee)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := payeeRepo.GetByID(payee.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !retrieved.DefaultCategoryID.Valid {
			t.Error("Expected default category ID to be set")
		}
		if retrieved.DefaultCategoryID.ID != category.ID {
			t.Errorf("Expected category ID %v, got %v", category.ID, retrieved.DefaultCategoryID.ID)
		}
	})

	t.Run("rejects duplicate name", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee1 := models.NewPayee("Duplicate Name")
		if err := repo.Create(payee1); err != nil {
			t.Fatalf("Create first payee error = %v", err)
		}

		payee2 := models.NewPayee("Duplicate Name")
		err := repo.Create(payee2)
		if err == nil {
			t.Error("Create() expected error for duplicate name")
		}
		if _, ok := err.(*DuplicateError); !ok {
			t.Errorf("Expected DuplicateError, got %T: %v", err, err)
		}
	})
}

func TestPayeeRepository_GetByID(t *testing.T) {
	t.Run("retrieves existing payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Test Payee")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByID(payee.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.ID != payee.ID {
			t.Errorf("Expected ID %v, got %v", payee.ID, retrieved.ID)
		}
		if retrieved.Name != "Test Payee" {
			t.Errorf("Expected name 'Test Payee', got %q", retrieved.Name)
		}
	})

	t.Run("returns NotFoundError for non-existent payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		fakeID := models.NewID()
		_, err := repo.GetByID(fakeID)
		if err == nil {
			t.Error("GetByID() expected error for non-existent payee")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestPayeeRepository_GetByName(t *testing.T) {
	t.Run("retrieves existing payee by name", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Unique Payee Name")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByName("Unique Payee Name")
		if err != nil {
			t.Fatalf("GetByName() error = %v", err)
		}
		if retrieved.ID != payee.ID {
			t.Errorf("Expected ID %v, got %v", payee.ID, retrieved.ID)
		}
	})

	t.Run("returns NotFoundError for non-existent name", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		_, err := repo.GetByName("Does Not Exist")
		if err == nil {
			t.Error("GetByName() expected error for non-existent name")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestPayeeRepository_List(t *testing.T) {
	t.Run("returns empty list for empty database", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payees, err := repo.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(payees) != 0 {
			t.Errorf("Expected 0 payees, got %d", len(payees))
		}
	})

	t.Run("returns all payees ordered by name", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		// Create payees in non-alphabetical order
		payee3 := models.NewPayee("Zoe's Bakery")
		payee1 := models.NewPayee("Alice's Coffee")
		payee2 := models.NewPayee("Bob's Market")

		for _, p := range []*models.Payee{payee3, payee1, payee2} {
			if err := repo.Create(p); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		payees, err := repo.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(payees) != 3 {
			t.Fatalf("Expected 3 payees, got %d", len(payees))
		}

		// Should be in alphabetical order
		if payees[0].Name != "Alice's Coffee" {
			t.Errorf("Expected first payee 'Alice's Coffee', got %q", payees[0].Name)
		}
		if payees[1].Name != "Bob's Market" {
			t.Errorf("Expected second payee 'Bob's Market', got %q", payees[1].Name)
		}
		if payees[2].Name != "Zoe's Bakery" {
			t.Errorf("Expected third payee 'Zoe's Bakery', got %q", payees[2].Name)
		}
	})
}

func TestPayeeRepository_Update(t *testing.T) {
	t.Run("updates existing payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Original Name")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		payee.Name = "Updated Name"
		payee.SetNotes("Some notes")
		if err := repo.Update(payee); err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, err := repo.GetByID(payee.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Name != "Updated Name" {
			t.Errorf("Expected name 'Updated Name', got %q", retrieved.Name)
		}
		if !retrieved.Notes.Valid || retrieved.Notes.String != "Some notes" {
			t.Error("Expected notes to be set")
		}
	})

	t.Run("returns NotFoundError for non-existent payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Non-existent")
		err := repo.Update(payee)
		if err == nil {
			t.Error("Update() expected error for non-existent payee")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("rejects duplicate name on update", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee1 := models.NewPayee("Payee One")
		payee2 := models.NewPayee("Payee Two")
		if err := repo.Create(payee1); err != nil {
			t.Fatalf("Create payee1 error = %v", err)
		}
		if err := repo.Create(payee2); err != nil {
			t.Fatalf("Create payee2 error = %v", err)
		}

		// Try to rename payee2 to payee1's name
		payee2.Name = "Payee One"
		err := repo.Update(payee2)
		if err == nil {
			t.Error("Update() expected error for duplicate name")
		}
		if _, ok := err.(*DuplicateError); !ok {
			t.Errorf("Expected DuplicateError, got %T: %v", err, err)
		}
	})
}

func TestPayeeRepository_Delete(t *testing.T) {
	t.Run("deletes existing payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("To Delete")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := repo.Delete(payee.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, err := repo.GetByID(payee.ID)
		if err == nil {
			t.Error("GetByID() expected error after delete")
		}
	})

	t.Run("deletes payee with aliases", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("With Aliases")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		// Create aliases
		alias1 := models.NewExactAlias(payee.ID, "alias1")
		alias2 := models.NewContainsAlias(payee.ID, "alias2")
		if err := repo.CreateAlias(alias1); err != nil {
			t.Fatalf("CreateAlias1 error = %v", err)
		}
		if err := repo.CreateAlias(alias2); err != nil {
			t.Fatalf("CreateAlias2 error = %v", err)
		}

		// Delete should cascade to aliases
		if err := repo.Delete(payee.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Aliases should also be deleted
		_, err := repo.GetAliasByID(alias1.ID)
		if err == nil {
			t.Error("Alias should be deleted with payee")
		}
	})

	t.Run("returns NotFoundError for non-existent payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		fakeID := models.NewID()
		err := repo.Delete(fakeID)
		if err == nil {
			t.Error("Delete() expected error for non-existent payee")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// Alias CRUD Tests
// =============================================================================

func TestPayeeRepository_CreateAlias(t *testing.T) {
	t.Run("creates valid alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Test Payee")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := models.NewExactAlias(payee.ID, "TEST PATTERN")
		err := repo.CreateAlias(alias)
		if err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		retrieved, err := repo.GetAliasByID(alias.ID)
		if err != nil {
			t.Fatalf("GetAliasByID() error = %v", err)
		}
		if retrieved.Pattern != "TEST PATTERN" {
			t.Errorf("Expected pattern 'TEST PATTERN', got %q", retrieved.Pattern)
		}
		if retrieved.MatchType != models.MatchTypeExact {
			t.Errorf("Expected match type 'exact', got %q", retrieved.MatchType)
		}
	})

	t.Run("creates aliases with different match types", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Test Payee")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		testCases := []struct {
			name      string
			alias     *models.Alias
			matchType models.MatchType
		}{
			{"exact", models.NewExactAlias(payee.ID, "exact-pattern"), models.MatchTypeExact},
			{"contains", models.NewContainsAlias(payee.ID, "contains-pattern"), models.MatchTypeContains},
			{"starts_with", models.NewStartsWithAlias(payee.ID, "starts-pattern"), models.MatchTypeStartsWith},
			{"regex", models.NewRegexAlias(payee.ID, "regex.*pattern"), models.MatchTypeRegex},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if err := repo.CreateAlias(tc.alias); err != nil {
					t.Fatalf("CreateAlias() error = %v", err)
				}

				retrieved, err := repo.GetAliasByID(tc.alias.ID)
				if err != nil {
					t.Fatalf("GetAliasByID() error = %v", err)
				}
				if retrieved.MatchType != tc.matchType {
					t.Errorf("Expected match type %q, got %q", tc.matchType, retrieved.MatchType)
				}
			})
		}
	})

	t.Run("rejects alias for non-existent payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		fakePayeeID := models.NewID()
		alias := models.NewExactAlias(fakePayeeID, "pattern")
		err := repo.CreateAlias(alias)
		if err == nil {
			t.Error("CreateAlias() expected error for non-existent payee")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("rejects duplicate pattern", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Test Payee")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias1 := models.NewExactAlias(payee.ID, "duplicate-pattern")
		if err := repo.CreateAlias(alias1); err != nil {
			t.Fatalf("CreateAlias first error = %v", err)
		}

		alias2 := models.NewContainsAlias(payee.ID, "duplicate-pattern")
		err := repo.CreateAlias(alias2)
		if err == nil {
			t.Error("CreateAlias() expected error for duplicate pattern")
		}
		if _, ok := err.(*DuplicateError); !ok {
			t.Errorf("Expected DuplicateError, got %T: %v", err, err)
		}
	})
}

func TestPayeeRepository_GetAliasByID(t *testing.T) {
	t.Run("retrieves existing alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Test Payee")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := models.NewContainsAlias(payee.ID, "test pattern")
		if err := repo.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		retrieved, err := repo.GetAliasByID(alias.ID)
		if err != nil {
			t.Fatalf("GetAliasByID() error = %v", err)
		}
		if retrieved.ID != alias.ID {
			t.Errorf("Expected ID %v, got %v", alias.ID, retrieved.ID)
		}
	})

	t.Run("returns NotFoundError for non-existent alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		fakeID := models.NewID()
		_, err := repo.GetAliasByID(fakeID)
		if err == nil {
			t.Error("GetAliasByID() expected error for non-existent alias")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestPayeeRepository_GetAliasesByPayee(t *testing.T) {
	t.Run("returns empty list for payee without aliases", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("No Aliases")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		aliases, err := repo.GetAliasesByPayee(payee.ID)
		if err != nil {
			t.Fatalf("GetAliasesByPayee() error = %v", err)
		}
		if len(aliases) != 0 {
			t.Errorf("Expected 0 aliases, got %d", len(aliases))
		}
	})

	t.Run("returns all aliases for payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("With Aliases")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias1 := models.NewExactAlias(payee.ID, "alias-a")
		alias2 := models.NewContainsAlias(payee.ID, "alias-b")
		alias3 := models.NewStartsWithAlias(payee.ID, "alias-c")
		for _, a := range []*models.Alias{alias1, alias2, alias3} {
			if err := repo.CreateAlias(a); err != nil {
				t.Fatalf("CreateAlias() error = %v", err)
			}
		}

		aliases, err := repo.GetAliasesByPayee(payee.ID)
		if err != nil {
			t.Fatalf("GetAliasesByPayee() error = %v", err)
		}
		if len(aliases) != 3 {
			t.Errorf("Expected 3 aliases, got %d", len(aliases))
		}
	})

	t.Run("does not return aliases from other payees", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee1 := models.NewPayee("Payee 1")
		payee2 := models.NewPayee("Payee 2")
		if err := repo.Create(payee1); err != nil {
			t.Fatalf("Create payee1 error = %v", err)
		}
		if err := repo.Create(payee2); err != nil {
			t.Fatalf("Create payee2 error = %v", err)
		}

		alias1 := models.NewExactAlias(payee1.ID, "payee1-alias")
		alias2 := models.NewExactAlias(payee2.ID, "payee2-alias")
		if err := repo.CreateAlias(alias1); err != nil {
			t.Fatalf("CreateAlias1 error = %v", err)
		}
		if err := repo.CreateAlias(alias2); err != nil {
			t.Fatalf("CreateAlias2 error = %v", err)
		}

		aliases, err := repo.GetAliasesByPayee(payee1.ID)
		if err != nil {
			t.Fatalf("GetAliasesByPayee() error = %v", err)
		}
		if len(aliases) != 1 {
			t.Errorf("Expected 1 alias, got %d", len(aliases))
		}
		if aliases[0].Pattern != "payee1-alias" {
			t.Errorf("Expected pattern 'payee1-alias', got %q", aliases[0].Pattern)
		}
	})
}

func TestPayeeRepository_UpdateAlias(t *testing.T) {
	t.Run("updates existing alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Test Payee")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := models.NewExactAlias(payee.ID, "original-pattern")
		if err := repo.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		alias.Pattern = "updated-pattern"
		alias.MatchType = models.MatchTypeContains
		if err := repo.UpdateAlias(alias); err != nil {
			t.Fatalf("UpdateAlias() error = %v", err)
		}

		retrieved, err := repo.GetAliasByID(alias.ID)
		if err != nil {
			t.Fatalf("GetAliasByID() error = %v", err)
		}
		if retrieved.Pattern != "updated-pattern" {
			t.Errorf("Expected pattern 'updated-pattern', got %q", retrieved.Pattern)
		}
		if retrieved.MatchType != models.MatchTypeContains {
			t.Errorf("Expected match type 'contains', got %q", retrieved.MatchType)
		}
	})

	t.Run("returns NotFoundError for non-existent alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Test Payee")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := models.NewExactAlias(payee.ID, "non-existent")
		err := repo.UpdateAlias(alias)
		if err == nil {
			t.Error("UpdateAlias() expected error for non-existent alias")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("rejects duplicate pattern on update", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Test Payee")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias1 := models.NewExactAlias(payee.ID, "pattern-one")
		alias2 := models.NewExactAlias(payee.ID, "pattern-two")
		if err := repo.CreateAlias(alias1); err != nil {
			t.Fatalf("CreateAlias1 error = %v", err)
		}
		if err := repo.CreateAlias(alias2); err != nil {
			t.Fatalf("CreateAlias2 error = %v", err)
		}

		// Try to update alias2 to have alias1's pattern
		alias2.Pattern = "pattern-one"
		err := repo.UpdateAlias(alias2)
		if err == nil {
			t.Error("UpdateAlias() expected error for duplicate pattern")
		}
		if _, ok := err.(*DuplicateError); !ok {
			t.Errorf("Expected DuplicateError, got %T: %v", err, err)
		}
	})
}

func TestPayeeRepository_DeleteAlias(t *testing.T) {
	t.Run("deletes existing alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Test Payee")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := models.NewExactAlias(payee.ID, "to-delete")
		if err := repo.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		if err := repo.DeleteAlias(alias.ID); err != nil {
			t.Fatalf("DeleteAlias() error = %v", err)
		}

		_, err := repo.GetAliasByID(alias.ID)
		if err == nil {
			t.Error("GetAliasByID() expected error after delete")
		}
	})

	t.Run("returns NotFoundError for non-existent alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		fakeID := models.NewID()
		err := repo.DeleteAlias(fakeID)
		if err == nil {
			t.Error("DeleteAlias() expected error for non-existent alias")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// Pattern Matching Tests
// =============================================================================

func TestPayeeRepository_FindPayeeByPattern(t *testing.T) {
	t.Run("finds payee by exact match", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Coffee Shop")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := models.NewExactAlias(payee.ID, "COFFEE SHOP TX")
		if err := repo.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		// Case insensitive exact match
		found, err := repo.FindPayeeByPattern("coffee shop tx")
		if err != nil {
			t.Fatalf("FindPayeeByPattern() error = %v", err)
		}
		if found == nil {
			t.Fatal("Expected to find payee")
		}
		if found.ID != payee.ID {
			t.Errorf("Expected payee ID %v, got %v", payee.ID, found.ID)
		}
	})

	t.Run("finds payee by contains match", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Amazon")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := models.NewContainsAlias(payee.ID, "AMAZON")
		if err := repo.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		found, err := repo.FindPayeeByPattern("amazon.com purchase 12345")
		if err != nil {
			t.Fatalf("FindPayeeByPattern() error = %v", err)
		}
		if found == nil {
			t.Fatal("Expected to find payee")
		}
		if found.ID != payee.ID {
			t.Errorf("Expected payee ID %v, got %v", payee.ID, found.ID)
		}
	})

	t.Run("finds payee by starts_with match", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Starbucks")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := models.NewStartsWithAlias(payee.ID, "STARBUCKS")
		if err := repo.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		found, err := repo.FindPayeeByPattern("starbucks store #1234")
		if err != nil {
			t.Fatalf("FindPayeeByPattern() error = %v", err)
		}
		if found == nil {
			t.Fatal("Expected to find payee")
		}
		if found.ID != payee.ID {
			t.Errorf("Expected payee ID %v, got %v", payee.ID, found.ID)
		}
	})

	t.Run("finds payee by regex match", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Gas Station")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := models.NewRegexAlias(payee.ID, "^SHELL.*#\\d+$")
		if err := repo.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		found, err := repo.FindPayeeByPattern("SHELL OIL #1234")
		if err != nil {
			t.Fatalf("FindPayeeByPattern() error = %v", err)
		}
		if found == nil {
			t.Fatal("Expected to find payee")
		}
		if found.ID != payee.ID {
			t.Errorf("Expected payee ID %v, got %v", payee.ID, found.ID)
		}
	})

	t.Run("returns nil when no match found", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Test Payee")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := models.NewExactAlias(payee.ID, "EXACT MATCH ONLY")
		if err := repo.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		found, err := repo.FindPayeeByPattern("something else entirely")
		if err != nil {
			t.Fatalf("FindPayeeByPattern() error = %v", err)
		}
		if found != nil {
			t.Errorf("Expected nil, got payee %v", found)
		}
	})

	t.Run("returns first matching payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee1 := models.NewPayee("Payee A")
		payee2 := models.NewPayee("Payee B")
		if err := repo.Create(payee1); err != nil {
			t.Fatalf("Create payee1 error = %v", err)
		}
		if err := repo.Create(payee2); err != nil {
			t.Fatalf("Create payee2 error = %v", err)
		}

		alias1 := models.NewContainsAlias(payee1.ID, "aaa")
		alias2 := models.NewContainsAlias(payee2.ID, "bbb")
		if err := repo.CreateAlias(alias1); err != nil {
			t.Fatalf("CreateAlias1 error = %v", err)
		}
		if err := repo.CreateAlias(alias2); err != nil {
			t.Fatalf("CreateAlias2 error = %v", err)
		}

		// Should match first alias (aaa comes before bbb alphabetically)
		found, err := repo.FindPayeeByPattern("test aaa test")
		if err != nil {
			t.Fatalf("FindPayeeByPattern() error = %v", err)
		}
		if found == nil {
			t.Fatal("Expected to find payee")
		}
		if found.ID != payee1.ID {
			t.Errorf("Expected payee1 ID %v, got %v", payee1.ID, found.ID)
		}
	})
}

func TestPayeeRepository_FindAliasMatch(t *testing.T) {
	t.Run("returns matching alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		payee := models.NewPayee("Test Payee")
		if err := repo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := models.NewContainsAlias(payee.ID, "TEST")
		if err := repo.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		found, err := repo.FindAliasMatch("something test something")
		if err != nil {
			t.Fatalf("FindAliasMatch() error = %v", err)
		}
		if found == nil {
			t.Fatal("Expected to find alias")
		}
		if found.ID != alias.ID {
			t.Errorf("Expected alias ID %v, got %v", alias.ID, found.ID)
		}
	})

	t.Run("returns nil when no match", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		found, err := repo.FindAliasMatch("no aliases in database")
		if err != nil {
			t.Fatalf("FindAliasMatch() error = %v", err)
		}
		if found != nil {
			t.Errorf("Expected nil, got alias %v", found)
		}
	})
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestPayeeRepository_PayeeWithNotesAndCategory(t *testing.T) {
	t.Run("full payee lifecycle with all fields", func(t *testing.T) {
		database := createTestDB(t)
		payeeRepo := NewPayeeRepository(database)
		categoryRepo := NewCategoryRepository(database)

		// Create category
		category := models.NewCategory("Food", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		// Create payee with category
		payee := models.NewPayeeWithCategory("Restaurant", category.ID)
		payee.SetNotes("Favorite Italian place")
		if err := payeeRepo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		// Retrieve and verify
		retrieved, err := payeeRepo.GetByID(payee.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if retrieved.Name != "Restaurant" {
			t.Errorf("Expected name 'Restaurant', got %q", retrieved.Name)
		}
		if !retrieved.DefaultCategoryID.Valid || retrieved.DefaultCategoryID.ID != category.ID {
			t.Error("Expected default category to be set")
		}
		if !retrieved.Notes.Valid || retrieved.Notes.String != "Favorite Italian place" {
			t.Error("Expected notes to be set")
		}

		// Update to clear category
		retrieved.ClearDefaultCategory()
		if err := payeeRepo.Update(retrieved); err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		// Verify update
		updated, err := payeeRepo.GetByID(payee.ID)
		if err != nil {
			t.Fatalf("GetByID after update error = %v", err)
		}
		if updated.DefaultCategoryID.Valid {
			t.Error("Expected default category to be cleared")
		}
	})
}

func TestPayeeRepository_ComplexPatternMatching(t *testing.T) {
	t.Run("matches complex patterns correctly", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewPayeeRepository(database)

		// Create payees with different alias types
		amazon := models.NewPayee("Amazon")
		starbucks := models.NewPayee("Starbucks")
		shell := models.NewPayee("Shell Gas")

		for _, p := range []*models.Payee{amazon, starbucks, shell} {
			if err := repo.Create(p); err != nil {
				t.Fatalf("Create payee error = %v", err)
			}
		}

		// Create aliases
		if err := repo.CreateAlias(models.NewContainsAlias(amazon.ID, "AMZN")); err != nil {
			t.Fatalf("CreateAlias error = %v", err)
		}
		if err := repo.CreateAlias(models.NewStartsWithAlias(starbucks.ID, "STARBUCKS")); err != nil {
			t.Fatalf("CreateAlias error = %v", err)
		}
		if err := repo.CreateAlias(models.NewRegexAlias(shell.ID, "SHELL.*\\d{4}")); err != nil {
			t.Fatalf("CreateAlias error = %v", err)
		}

		testCases := []struct {
			input       string
			expectedID  *models.ID
			description string
		}{
			{"AMZN MKTP US*1234567", &amazon.ID, "Amazon contains match"},
			{"STARBUCKS STORE #1234", &starbucks.ID, "Starbucks starts_with match"},
			{"SHELL OIL CO 1234", &shell.ID, "Shell regex match"},
			{"WALMART SUPERCENTER", nil, "No match"},
		}

		for _, tc := range testCases {
			t.Run(tc.description, func(t *testing.T) {
				found, err := repo.FindPayeeByPattern(tc.input)
				if err != nil {
					t.Fatalf("FindPayeeByPattern() error = %v", err)
				}

				if tc.expectedID == nil {
					if found != nil {
						t.Errorf("Expected no match, got payee %q", found.Name)
					}
				} else {
					if found == nil {
						t.Fatal("Expected to find payee")
					}
					if found.ID != *tc.expectedID {
						t.Errorf("Expected payee ID %v, got %v", *tc.expectedID, found.ID)
					}
				}
			})
		}
	})
}
