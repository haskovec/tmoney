package category

import (
	"testing"
)

func TestGetOrCreateLoanInterestCategory_FreshDB(t *testing.T) {
	database := createTestDB(t)
	svc := NewService(NewRepository(database), database)

	child, err := svc.GetOrCreateLoanInterestCategory()
	if err != nil {
		t.Fatalf("GetOrCreateLoanInterestCategory: %v", err)
	}
	if child.Name != LoanInterestChildName {
		t.Errorf("child name = %q, want %q", child.Name, LoanInterestChildName)
	}
	if !child.IsSubcategory() {
		t.Error("interest category should be a subcategory of Loan")
	}
	if child.Type != TypeExpense {
		t.Errorf("interest category type = %v, want expense", child.Type)
	}
	if child.IsSystem {
		t.Error("loan interest category must be a regular (non-system) category")
	}

	// Parent must exist, be named Loan, and be top-level.
	parent, err := svc.repo.GetByName(LoanCategoryName, nil)
	if err != nil {
		t.Fatalf("parent lookup: %v", err)
	}
	if !parent.IsTopLevel() {
		t.Error("Loan parent should be top-level")
	}
	if !child.ParentID.Valid || child.ParentID.ID != parent.ID {
		t.Errorf("child parent = %v, want %v", child.ParentID, parent.ID)
	}
}

func TestGetOrCreateLoanInterestCategory_Idempotent(t *testing.T) {
	database := createTestDB(t)
	svc := NewService(NewRepository(database), database)

	first, err := svc.GetOrCreateLoanInterestCategory()
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := svc.GetOrCreateLoanInterestCategory()
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("non-idempotent: first %v, second %v", first.ID, second.ID)
	}

	// No duplicate Loan parents or Interest children created.
	cats, err := svc.repo.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var loanParents, interestChildren int
	for _, c := range cats {
		if c.IsTopLevel() && c.Name == LoanCategoryName {
			loanParents++
		}
		if c.IsSubcategory() && c.Name == LoanInterestChildName && c.ParentID.ID == first.ParentID.ID {
			interestChildren++
		}
	}
	if loanParents != 1 {
		t.Errorf("got %d Loan parents, want 1", loanParents)
	}
	if interestChildren != 1 {
		t.Errorf("got %d Interest children, want 1", interestChildren)
	}
}

func TestGetOrCreateLoanInterestCategory_ReusesExistingParent(t *testing.T) {
	database := createTestDB(t)
	svc := NewService(NewRepository(database), database)

	// A pre-existing user "Loan" parent (e.g. from SeedDefaultCategories).
	existingParent := NewCategory(LoanCategoryName, TypeExpense)
	if err := svc.repo.Create(existingParent); err != nil {
		t.Fatalf("seed parent: %v", err)
	}

	child, err := svc.GetOrCreateLoanInterestCategory()
	if err != nil {
		t.Fatalf("GetOrCreateLoanInterestCategory: %v", err)
	}
	if child.ParentID.ID != existingParent.ID {
		t.Errorf("child reparented to %v, want existing %v", child.ParentID.ID, existingParent.ID)
	}
}
