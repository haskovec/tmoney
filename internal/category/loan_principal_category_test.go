package category

import (
	"testing"
)

func TestGetOrCreateLoanPrincipalCategory_FreshDB(t *testing.T) {
	database := createTestDB(t)
	svc := NewService(NewRepository(database), database)

	child, err := svc.GetOrCreateLoanPrincipalCategory()
	if err != nil {
		t.Fatalf("GetOrCreateLoanPrincipalCategory: %v", err)
	}
	if child.Name != LoanPrincipalChildName {
		t.Errorf("child name = %q, want %q", child.Name, LoanPrincipalChildName)
	}
	if !child.IsSubcategory() {
		t.Error("principal category should be a subcategory of Loan")
	}
	if child.Type != TypeExpense {
		t.Errorf("principal category type = %v, want expense", child.Type)
	}
	if child.IsSystem {
		t.Error("loan principal category must be a regular (non-system) category")
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

func TestGetOrCreateLoanPrincipalCategory_Idempotent(t *testing.T) {
	database := createTestDB(t)
	svc := NewService(NewRepository(database), database)

	first, err := svc.GetOrCreateLoanPrincipalCategory()
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := svc.GetOrCreateLoanPrincipalCategory()
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("non-idempotent: first %v, second %v", first.ID, second.ID)
	}

	// No duplicate Loan parents or Principal children created.
	cats, err := svc.repo.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var loanParents, principalChildren int
	for _, c := range cats {
		if c.IsTopLevel() && c.Name == LoanCategoryName {
			loanParents++
		}
		if c.IsSubcategory() && c.Name == LoanPrincipalChildName && c.ParentID.ID == first.ParentID.ID {
			principalChildren++
		}
	}
	if loanParents != 1 {
		t.Errorf("got %d Loan parents, want 1", loanParents)
	}
	if principalChildren != 1 {
		t.Errorf("got %d Principal children, want 1", principalChildren)
	}
}

// TestGetOrCreateLoanPrincipalCategory_SharesParentWithInterest confirms the
// principal and interest defaults live under the same single "Loan" parent
// rather than each creating their own.
func TestGetOrCreateLoanPrincipalCategory_SharesParentWithInterest(t *testing.T) {
	database := createTestDB(t)
	svc := NewService(NewRepository(database), database)

	interest, err := svc.GetOrCreateLoanInterestCategory()
	if err != nil {
		t.Fatalf("interest: %v", err)
	}
	principal, err := svc.GetOrCreateLoanPrincipalCategory()
	if err != nil {
		t.Fatalf("principal: %v", err)
	}
	if interest.ParentID.ID != principal.ParentID.ID {
		t.Errorf("interest parent %v != principal parent %v", interest.ParentID.ID, principal.ParentID.ID)
	}

	cats, _ := svc.repo.List()
	var loanParents int
	for _, c := range cats {
		if c.IsTopLevel() && c.Name == LoanCategoryName {
			loanParents++
		}
	}
	if loanParents != 1 {
		t.Errorf("got %d Loan parents, want 1 shared parent", loanParents)
	}
}

func TestGetOrCreateLoanPrincipalCategory_ReusesExistingParent(t *testing.T) {
	database := createTestDB(t)
	svc := NewService(NewRepository(database), database)

	// A pre-existing user "Loan" parent (e.g. from SeedDefaultCategories).
	existingParent := NewCategory(LoanCategoryName, TypeExpense)
	if err := svc.repo.Create(existingParent); err != nil {
		t.Fatalf("seed parent: %v", err)
	}

	child, err := svc.GetOrCreateLoanPrincipalCategory()
	if err != nil {
		t.Fatalf("GetOrCreateLoanPrincipalCategory: %v", err)
	}
	if child.ParentID.ID != existingParent.ID {
		t.Errorf("child reparented to %v, want existing %v", child.ParentID.ID, existingParent.ID)
	}
}
