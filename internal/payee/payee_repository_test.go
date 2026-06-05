package payee

import (
	"testing"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/types"
)

func createTestDB(t *testing.T) *db.DB {
	t.Helper()
	return dbtest.New(t)
}

// =============================================================================
// Payee CRUD Tests
// =============================================================================

func TestRepository_Create(t *testing.T) {
	t.Run("creates valid payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Coffee Shop")
		err := repo.Create(p)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify it was created
		retrieved, err := repo.GetByID(p.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Name != "Coffee Shop" {
			t.Errorf("Expected name 'Coffee Shop', got %q", retrieved.Name)
		}
	})

	t.Run("creates payee with default category", func(t *testing.T) {
		database := createTestDB(t)
		payeeRepo := NewRepository(database)

		// Create a category first using raw SQL (category is in a different package)
		categoryID := types.NewID()
		_, err := database.Conn().Exec(`
			INSERT INTO categories (id, name, parent_id, type, system_category, created_at, updated_at)
			VALUES (?, 'Food', NULL, 'expense', false, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, categoryID)
		if err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		p := NewPayeeWithCategory("Restaurant", categoryID)
		err = payeeRepo.Create(p)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := payeeRepo.GetByID(p.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !retrieved.DefaultCategoryID.Valid {
			t.Error("Expected default category ID to be set")
		}
		if retrieved.DefaultCategoryID.ID != categoryID {
			t.Errorf("Expected category ID %v, got %v", categoryID, retrieved.DefaultCategoryID.ID)
		}
	})

	t.Run("rejects duplicate name", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p1 := NewPayee("Duplicate Name")
		if err := repo.Create(p1); err != nil {
			t.Fatalf("Create first payee error = %v", err)
		}

		p2 := NewPayee("Duplicate Name")
		err := repo.Create(p2)
		if err == nil {
			t.Error("Create() expected error for duplicate name")
		}
		if _, ok := err.(*dberrors.DuplicateError); !ok {
			t.Errorf("Expected DuplicateError, got %T: %v", err, err)
		}
	})
}

func TestRepository_GetByID(t *testing.T) {
	t.Run("retrieves existing payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Test Payee")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByID(p.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.ID != p.ID {
			t.Errorf("Expected ID %v, got %v", p.ID, retrieved.ID)
		}
		if retrieved.Name != "Test Payee" {
			t.Errorf("Expected name 'Test Payee', got %q", retrieved.Name)
		}
	})

	t.Run("returns NotFoundError for non-existent payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		fakeID := types.NewID()
		_, err := repo.GetByID(fakeID)
		if err == nil {
			t.Error("GetByID() expected error for non-existent payee")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestRepository_GetByName(t *testing.T) {
	t.Run("retrieves existing payee by name", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Unique Payee Name")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByName("Unique Payee Name")
		if err != nil {
			t.Fatalf("GetByName() error = %v", err)
		}
		if retrieved.ID != p.ID {
			t.Errorf("Expected ID %v, got %v", p.ID, retrieved.ID)
		}
	})

	t.Run("returns NotFoundError for non-existent name", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		_, err := repo.GetByName("Does Not Exist")
		if err == nil {
			t.Error("GetByName() expected error for non-existent name")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestRepository_List(t *testing.T) {
	t.Run("returns empty list for empty database", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

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
		repo := NewRepository(database)

		// Create payees in non-alphabetical order
		p3 := NewPayee("Zoe's Bakery")
		p1 := NewPayee("Alice's Coffee")
		p2 := NewPayee("Bob's Market")

		for _, p := range []*Payee{p3, p1, p2} {
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

func TestRepository_Update(t *testing.T) {
	t.Run("updates existing payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Original Name")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		p.Name = "Updated Name"
		p.SetNotes("Some notes")
		if err := repo.Update(p); err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, err := repo.GetByID(p.ID)
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
		repo := NewRepository(database)

		p := NewPayee("Non-existent")
		err := repo.Update(p)
		if err == nil {
			t.Error("Update() expected error for non-existent payee")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("rejects duplicate name on update", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p1 := NewPayee("Payee One")
		p2 := NewPayee("Payee Two")
		if err := repo.Create(p1); err != nil {
			t.Fatalf("Create payee1 error = %v", err)
		}
		if err := repo.Create(p2); err != nil {
			t.Fatalf("Create payee2 error = %v", err)
		}

		// Try to rename payee2 to payee1's name
		p2.Name = "Payee One"
		err := repo.Update(p2)
		if err == nil {
			t.Error("Update() expected error for duplicate name")
		}
		if _, ok := err.(*dberrors.DuplicateError); !ok {
			t.Errorf("Expected DuplicateError, got %T: %v", err, err)
		}
	})
}

func TestRepository_Delete(t *testing.T) {
	t.Run("deletes existing payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("To Delete")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := repo.Delete(p.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, err := repo.GetByID(p.ID)
		if err == nil {
			t.Error("GetByID() expected error after delete")
		}
	})

	t.Run("deletes payee with aliases", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("With Aliases")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		// Create aliases
		alias1 := NewExactAlias(p.ID, "alias1")
		alias2 := NewContainsAlias(p.ID, "alias2")
		if err := repo.CreateAlias(alias1); err != nil {
			t.Fatalf("CreateAlias1 error = %v", err)
		}
		if err := repo.CreateAlias(alias2); err != nil {
			t.Fatalf("CreateAlias2 error = %v", err)
		}

		// Delete should cascade to aliases
		if err := repo.Delete(p.ID); err != nil {
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
		repo := NewRepository(database)

		fakeID := types.NewID()
		err := repo.Delete(fakeID)
		if err == nil {
			t.Error("Delete() expected error for non-existent payee")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// Alias CRUD Tests
// =============================================================================

func TestRepository_CreateAlias(t *testing.T) {
	t.Run("creates valid alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Test Payee")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := NewExactAlias(p.ID, "TEST PATTERN")
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
		if retrieved.MatchType != MatchTypeExact {
			t.Errorf("Expected match type 'exact', got %q", retrieved.MatchType)
		}
	})

	t.Run("creates aliases with different match types", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Test Payee")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		testCases := []struct {
			name      string
			alias     *Alias
			matchType MatchType
		}{
			{"exact", NewExactAlias(p.ID, "exact-pattern"), MatchTypeExact},
			{"contains", NewContainsAlias(p.ID, "contains-pattern"), MatchTypeContains},
			{"starts_with", NewStartsWithAlias(p.ID, "starts-pattern"), MatchTypeStartsWith},
			{"regex", NewRegexAlias(p.ID, "regex.*pattern"), MatchTypeRegex},
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
		repo := NewRepository(database)

		fakePayeeID := types.NewID()
		alias := NewExactAlias(fakePayeeID, "pattern")
		err := repo.CreateAlias(alias)
		if err == nil {
			t.Error("CreateAlias() expected error for non-existent payee")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("rejects duplicate pattern", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Test Payee")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias1 := NewExactAlias(p.ID, "duplicate-pattern")
		if err := repo.CreateAlias(alias1); err != nil {
			t.Fatalf("CreateAlias first error = %v", err)
		}

		alias2 := NewContainsAlias(p.ID, "duplicate-pattern")
		err := repo.CreateAlias(alias2)
		if err == nil {
			t.Error("CreateAlias() expected error for duplicate pattern")
		}
		if _, ok := err.(*dberrors.DuplicateError); !ok {
			t.Errorf("Expected DuplicateError, got %T: %v", err, err)
		}
	})
}

func TestRepository_GetAliasByID(t *testing.T) {
	t.Run("retrieves existing alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Test Payee")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := NewContainsAlias(p.ID, "test pattern")
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
		repo := NewRepository(database)

		fakeID := types.NewID()
		_, err := repo.GetAliasByID(fakeID)
		if err == nil {
			t.Error("GetAliasByID() expected error for non-existent alias")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestRepository_GetAliasesByPayee(t *testing.T) {
	t.Run("returns empty list for payee without aliases", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("No Aliases")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		aliases, err := repo.GetAliasesByPayee(p.ID)
		if err != nil {
			t.Fatalf("GetAliasesByPayee() error = %v", err)
		}
		if len(aliases) != 0 {
			t.Errorf("Expected 0 aliases, got %d", len(aliases))
		}
	})

	t.Run("returns all aliases for payee", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("With Aliases")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias1 := NewExactAlias(p.ID, "alias-a")
		alias2 := NewContainsAlias(p.ID, "alias-b")
		alias3 := NewStartsWithAlias(p.ID, "alias-c")
		for _, a := range []*Alias{alias1, alias2, alias3} {
			if err := repo.CreateAlias(a); err != nil {
				t.Fatalf("CreateAlias() error = %v", err)
			}
		}

		aliases, err := repo.GetAliasesByPayee(p.ID)
		if err != nil {
			t.Fatalf("GetAliasesByPayee() error = %v", err)
		}
		if len(aliases) != 3 {
			t.Errorf("Expected 3 aliases, got %d", len(aliases))
		}
	})

	t.Run("does not return aliases from other payees", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p1 := NewPayee("Payee 1")
		p2 := NewPayee("Payee 2")
		if err := repo.Create(p1); err != nil {
			t.Fatalf("Create payee1 error = %v", err)
		}
		if err := repo.Create(p2); err != nil {
			t.Fatalf("Create payee2 error = %v", err)
		}

		alias1 := NewExactAlias(p1.ID, "payee1-alias")
		alias2 := NewExactAlias(p2.ID, "payee2-alias")
		if err := repo.CreateAlias(alias1); err != nil {
			t.Fatalf("CreateAlias1 error = %v", err)
		}
		if err := repo.CreateAlias(alias2); err != nil {
			t.Fatalf("CreateAlias2 error = %v", err)
		}

		aliases, err := repo.GetAliasesByPayee(p1.ID)
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

func TestRepository_UpdateAlias(t *testing.T) {
	t.Run("updates existing alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Test Payee")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := NewExactAlias(p.ID, "original-pattern")
		if err := repo.CreateAlias(alias); err != nil {
			t.Fatalf("CreateAlias() error = %v", err)
		}

		alias.Pattern = "updated-pattern"
		alias.MatchType = MatchTypeContains
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
		if retrieved.MatchType != MatchTypeContains {
			t.Errorf("Expected match type 'contains', got %q", retrieved.MatchType)
		}
	})

	t.Run("returns NotFoundError for non-existent alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Test Payee")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := NewExactAlias(p.ID, "non-existent")
		err := repo.UpdateAlias(alias)
		if err == nil {
			t.Error("UpdateAlias() expected error for non-existent alias")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("rejects duplicate pattern on update", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Test Payee")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias1 := NewExactAlias(p.ID, "pattern-one")
		alias2 := NewExactAlias(p.ID, "pattern-two")
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
		if _, ok := err.(*dberrors.DuplicateError); !ok {
			t.Errorf("Expected DuplicateError, got %T: %v", err, err)
		}
	})
}

func TestRepository_DeleteAlias(t *testing.T) {
	t.Run("deletes existing alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Test Payee")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := NewExactAlias(p.ID, "to-delete")
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
		repo := NewRepository(database)

		fakeID := types.NewID()
		err := repo.DeleteAlias(fakeID)
		if err == nil {
			t.Error("DeleteAlias() expected error for non-existent alias")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// Pattern Matching Tests
// =============================================================================

func TestRepository_FindPayeeByPattern(t *testing.T) {
	t.Run("finds payee by exact match", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Coffee Shop")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := NewExactAlias(p.ID, "COFFEE SHOP TX")
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
		if found.ID != p.ID {
			t.Errorf("Expected payee ID %v, got %v", p.ID, found.ID)
		}
	})

	t.Run("finds payee by contains match", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Amazon")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := NewContainsAlias(p.ID, "AMAZON")
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
		if found.ID != p.ID {
			t.Errorf("Expected payee ID %v, got %v", p.ID, found.ID)
		}
	})

	t.Run("finds payee by starts_with match", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Starbucks")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := NewStartsWithAlias(p.ID, "STARBUCKS")
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
		if found.ID != p.ID {
			t.Errorf("Expected payee ID %v, got %v", p.ID, found.ID)
		}
	})

	t.Run("finds payee by regex match", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Gas Station")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := NewRegexAlias(p.ID, "^SHELL.*#\\d+$")
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
		if found.ID != p.ID {
			t.Errorf("Expected payee ID %v, got %v", p.ID, found.ID)
		}
	})

	t.Run("returns nil when no match found", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Test Payee")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := NewExactAlias(p.ID, "EXACT MATCH ONLY")
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
		repo := NewRepository(database)

		p1 := NewPayee("Payee A")
		p2 := NewPayee("Payee B")
		if err := repo.Create(p1); err != nil {
			t.Fatalf("Create payee1 error = %v", err)
		}
		if err := repo.Create(p2); err != nil {
			t.Fatalf("Create payee2 error = %v", err)
		}

		alias1 := NewContainsAlias(p1.ID, "aaa")
		alias2 := NewContainsAlias(p2.ID, "bbb")
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
		if found.ID != p1.ID {
			t.Errorf("Expected payee1 ID %v, got %v", p1.ID, found.ID)
		}
	})
}

func TestRepository_FindAliasMatch(t *testing.T) {
	t.Run("returns matching alias", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		p := NewPayee("Test Payee")
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		alias := NewContainsAlias(p.ID, "TEST")
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
		repo := NewRepository(database)

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

func TestRepository_PayeeWithNotesAndCategory(t *testing.T) {
	t.Run("full payee lifecycle with all fields", func(t *testing.T) {
		database := createTestDB(t)
		payeeRepo := NewRepository(database)

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
		p.SetNotes("Favorite Italian place")
		if err := payeeRepo.Create(p); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		// Retrieve and verify
		retrieved, err := payeeRepo.GetByID(p.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if retrieved.Name != "Restaurant" {
			t.Errorf("Expected name 'Restaurant', got %q", retrieved.Name)
		}
		if !retrieved.DefaultCategoryID.Valid || retrieved.DefaultCategoryID.ID != categoryID {
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
		updated, err := payeeRepo.GetByID(p.ID)
		if err != nil {
			t.Fatalf("GetByID after update error = %v", err)
		}
		if updated.DefaultCategoryID.Valid {
			t.Error("Expected default category to be cleared")
		}
	})
}

func TestRepository_ComplexPatternMatching(t *testing.T) {
	t.Run("matches complex patterns correctly", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		// Create payees with different alias types
		amazon := NewPayee("Amazon")
		starbucks := NewPayee("Starbucks")
		shell := NewPayee("Shell Gas")

		for _, p := range []*Payee{amazon, starbucks, shell} {
			if err := repo.Create(p); err != nil {
				t.Fatalf("Create payee error = %v", err)
			}
		}

		// Create aliases
		if err := repo.CreateAlias(NewContainsAlias(amazon.ID, "AMZN")); err != nil {
			t.Fatalf("CreateAlias error = %v", err)
		}
		if err := repo.CreateAlias(NewStartsWithAlias(starbucks.ID, "STARBUCKS")); err != nil {
			t.Fatalf("CreateAlias error = %v", err)
		}
		if err := repo.CreateAlias(NewRegexAlias(shell.ID, "SHELL.*\\d{4}")); err != nil {
			t.Fatalf("CreateAlias error = %v", err)
		}

		testCases := []struct {
			input       string
			expectedID  *types.ID
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
