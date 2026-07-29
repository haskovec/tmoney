package category

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// DefaultCategory represents a default category to be seeded.
type DefaultCategory struct {
	Name     string
	Type     Type
	IsSystem bool
	Children []string // Names of subcategories
}

// DefaultCategories defines the default categories to seed for new files.
// These are based on common personal finance categories.
var DefaultCategories = []DefaultCategory{
	// System category for transfers between accounts
	{
		Name:     TransferCategoryName,
		Type:     TypeExpense,
		IsSystem: true,
	},

	// System category for asset revaluations (home value updates,
	// straight-line depreciation). Excluded from spending reports like
	// Transfer; the TUI picker surfaces it for asset accounts only.
	{
		Name:     ValueAdjustmentCategoryName,
		Type:     TypeExpense,
		IsSystem: true,
	},

	// Income categories
	{
		Name: "Income",
		Type: TypeIncome,
		Children: []string{
			"Salary",
			"Bonus",
			"Interest",
			"Dividends",
			"Gifts Received",
			"Other Income",
		},
	},

	// Expense categories
	{
		Name: "Housing",
		Type: TypeExpense,
		Children: []string{
			"Rent",
			"Mortgage",
			"Property Tax",
			"Home Insurance",
			"Repairs & Maintenance",
		},
	},
	{
		Name: "Utilities",
		Type: TypeExpense,
		Children: []string{
			"Electric",
			"Gas",
			"Water",
			"Internet",
			"Phone",
		},
	},
	{
		Name: "Food",
		Type: TypeExpense,
		Children: []string{
			"Groceries",
			"Dining Out",
			"Coffee & Snacks",
		},
	},
	{
		Name: "Transportation",
		Type: TypeExpense,
		Children: []string{
			"Gas & Fuel",
			"Auto Insurance",
			"Auto Repairs",
			"Parking",
			"Public Transit",
		},
	},
	{
		Name: "Healthcare",
		Type: TypeExpense,
		Children: []string{
			"Doctor",
			"Dentist",
			"Pharmacy",
			"Health Insurance",
		},
	},
	{
		Name: "Personal",
		Type: TypeExpense,
		Children: []string{
			"Clothing",
			"Haircut",
			"Gym",
		},
	},
	{
		Name: "Entertainment",
		Type: TypeExpense,
		Children: []string{
			"Movies",
			"Music & Streaming",
			"Books & Magazines",
			"Hobbies",
		},
	},
	{
		Name: "Shopping",
		Type: TypeExpense,
		Children: []string{
			"Electronics",
			"Home Goods",
			"Gifts Given",
		},
	},
	{
		Name: "Financial",
		Type: TypeExpense,
		Children: []string{
			"Bank Fees",
			"Late Fees",
			"Interest Paid",
		},
	},
	// Loan payment interest expense. The loan wizard books the interest
	// portion of a payment here (default; overridable). Non-system so
	// users can rename/merge it; the child is get-or-created at
	// loan-creation time for existing files.
	{
		Name: "Loan",
		Type: TypeExpense,
		Children: []string{
			"Interest",
		},
	},
	{
		Name: "Taxes",
		Type: TypeExpense,
		Children: []string{
			"Federal Tax",
			"State Tax",
			"Local Tax",
		},
	},
	{
		Name: "Miscellaneous",
		Type: TypeExpense,
	},
}

// PaycheckCategory is a (parent → child) category pair the paycheck
// wizard expects to find in its dropdowns.
type PaycheckCategory struct {
	Parent string
	Child  string
	Type   Type
}

// PaycheckCategories lists the categories the paycheck wizard
// pre-populates. EnsurePaycheckCategories seeds any missing pairs on
// file initialization and on every database open, so existing
// databases gain them without a migration.
var PaycheckCategories = []PaycheckCategory{
	{Parent: "Income", Child: "Salary", Type: TypeIncome},
	{Parent: "Income", Child: "Bonus", Type: TypeIncome},
	{Parent: "Income", Child: "Retro Pay", Type: TypeIncome},
	{Parent: "Tax", Child: "Federal", Type: TypeExpense},
	{Parent: "Tax", Child: "State", Type: TypeExpense},
	{Parent: "Tax", Child: "Social Security", Type: TypeExpense},
	{Parent: "Tax", Child: "Medicare", Type: TypeExpense},
	{Parent: "Insurance", Child: "Health", Type: TypeExpense},
}

// Service provides business logic for category operations.
type Service struct {
	repo *Repository
	db   *db.DB
	tx   db.Queryer // nil outside a transaction
}

// NewService creates a new Service.
func NewService(repo *Repository, database *db.DB) *Service {
	return &Service{
		repo: repo,
		db:   database,
	}
}

// InTx returns a copy of the service bound to tx, with its repository rebound to
// the same transaction so all writes join one atomic unit. The original service
// is unchanged and remains safe for non-transactional use.
func (s *Service) InTx(tx db.Queryer) *Service {
	c := *s
	c.tx = tx
	c.repo = s.repo.WithTx(tx)
	return &c
}

// q returns the active Queryer for ad-hoc service-level SQL: the bound
// transaction if any, else the live connection. A bound service must not query
// the pool directly — a pool read would miss the transaction's own
// uncommitted writes, and a pool write would escape its atomicity.
func (s *Service) q() db.Queryer {
	if s.tx != nil {
		return s.tx
	}
	return s.db.Conn()
}

// runInTx begins a new transaction if the service is unbound, or joins the
// already-bound transaction. This is what makes service methods composable
// without savepoints: an outer flow binds once, inner calls join. A bound
// service must never reach db.WithTx (nesting would deadlock the mutex).
func (s *Service) runInTx(fn func(b *Service) error) error {
	if s.tx != nil {
		return fn(s) // already bound — join the caller's tx
	}
	return s.db.WithTx(func(tx db.Queryer) error {
		return fn(s.InTx(tx))
	})
}

// SeedDefaultCategories creates all default categories in the database.
// This should be called when a new TMoney file is created.
// It skips categories that already exist (based on name and parent).
func (s *Service) SeedDefaultCategories() error {
	for _, dc := range DefaultCategories {
		if err := s.seedCategory(dc); err != nil {
			return fmt.Errorf("failed to seed category %q: %w", dc.Name, err)
		}
	}
	return nil
}

// seedCategory creates a single parent category and its children.
func (s *Service) seedCategory(dc DefaultCategory) error {
	// Check if parent category already exists
	existing, err := s.repo.GetByName(dc.Name, nil)
	if err == nil {
		// Category exists, skip seeding children since they should already exist
		_ = existing
		return nil
	}

	// Check if error is "not found" - that's expected for seeding
	if _, ok := err.(*dberrors.NotFoundError); !ok {
		return fmt.Errorf("failed to check existing category: %w", err)
	}

	// Create the parent category
	var parent *Category
	if dc.IsSystem {
		parent = NewSystemCategory(dc.Name, dc.Type)
	} else {
		parent = NewCategory(dc.Name, dc.Type)
	}

	if err := s.repo.Create(parent); err != nil {
		return fmt.Errorf("failed to create parent category: %w", err)
	}

	// Create child categories
	for _, childName := range dc.Children {
		child := NewSubcategory(childName, parent.ID, dc.Type)
		if err := s.repo.Create(child); err != nil {
			return fmt.Errorf("failed to create subcategory %q: %w", childName, err)
		}
	}

	return nil
}

// GetTransferCategory returns the system Transfer category.
// This category is used for transfers between accounts.
func (s *Service) GetTransferCategory() (*Category, error) {
	categories, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	for _, cat := range categories {
		if cat.IsSystem && cat.Name == TransferCategoryName {
			return cat, nil
		}
	}

	return nil, &dberrors.NotFoundError{Entity: "category", ID: TransferCategoryName}
}

// EnsurePaycheckCategories creates any missing (parent, child) pairs
// from PaycheckCategories. Idempotent: existing parents and children
// are left untouched. Safe to call on every database open so existing
// files gain the paycheck-wizard categories without a migration.
func (s *Service) EnsurePaycheckCategories() error {
	for _, pc := range PaycheckCategories {
		parent, err := s.repo.GetByName(pc.Parent, nil)
		if err != nil {
			if _, ok := err.(*dberrors.NotFoundError); !ok {
				return fmt.Errorf("lookup parent %q: %w", pc.Parent, err)
			}
			parent = NewCategory(pc.Parent, pc.Type)
			if err := s.repo.Create(parent); err != nil {
				return fmt.Errorf("create parent %q: %w", pc.Parent, err)
			}
		}
		if _, err := s.repo.GetByName(pc.Child, &parent.ID); err == nil {
			continue
		} else if _, ok := err.(*dberrors.NotFoundError); !ok {
			return fmt.Errorf("lookup child %q under %q: %w", pc.Child, pc.Parent, err)
		}
		child := NewSubcategory(pc.Child, parent.ID, pc.Type)
		if err := s.repo.Create(child); err != nil {
			return fmt.Errorf("create child %q under %q: %w", pc.Child, pc.Parent, err)
		}
	}
	return nil
}

// GetOrCreateLoanInterestCategory returns the default loan-interest expense
// category (Loan:Interest), creating the parent Loan and/or the Interest child
// when either is missing. It is the loan wizard's and `loan add`'s save-time
// resolution of the default interest category, so the default is always
// available even on files where it was never seeded or was deleted. Idempotent:
// an existing parent/child is reused untouched. The categories are ordinary
// (non-system) so the user can later rename or merge them.
func (s *Service) GetOrCreateLoanInterestCategory() (*Category, error) {
	parent, err := s.repo.GetByName(LoanCategoryName, nil)
	if err != nil {
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			return nil, fmt.Errorf("lookup parent %q: %w", LoanCategoryName, err)
		}
		parent = NewCategory(LoanCategoryName, TypeExpense)
		if err := s.repo.Create(parent); err != nil {
			return nil, fmt.Errorf("create parent %q: %w", LoanCategoryName, err)
		}
	}
	child, err := s.repo.GetByName(LoanInterestChildName, &parent.ID)
	if err == nil {
		return child, nil
	}
	if _, ok := err.(*dberrors.NotFoundError); !ok {
		return nil, fmt.Errorf("lookup child %q under %q: %w", LoanInterestChildName, LoanCategoryName, err)
	}
	child = NewSubcategory(LoanInterestChildName, parent.ID, TypeExpense)
	if err := s.repo.Create(child); err != nil {
		return nil, fmt.Errorf("create child %q under %q: %w", LoanInterestChildName, LoanCategoryName, err)
	}
	return child, nil
}

// GetOrCreateLoanPrincipalCategory returns the default loan-principal expense
// category (Loan:Principal), creating the parent Loan and/or the Principal
// child when either is missing. It is the loan wizard's and `loan add`'s
// save-time resolution of the default principal category, so the default is
// always available even on files where it was never seeded. It is the twin of
// GetOrCreateLoanInterestCategory: idempotent, reuses an existing parent/child,
// and the categories are ordinary (non-system) so the user can later rename or
// merge them. Unlike interest, the principal label is optional — a loan built
// without it stores a bare (uncategorized) transfer line.
func (s *Service) GetOrCreateLoanPrincipalCategory() (*Category, error) {
	parent, err := s.repo.GetByName(LoanCategoryName, nil)
	if err != nil {
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			return nil, fmt.Errorf("lookup parent %q: %w", LoanCategoryName, err)
		}
		parent = NewCategory(LoanCategoryName, TypeExpense)
		if err := s.repo.Create(parent); err != nil {
			return nil, fmt.Errorf("create parent %q: %w", LoanCategoryName, err)
		}
	}
	child, err := s.repo.GetByName(LoanPrincipalChildName, &parent.ID)
	if err == nil {
		return child, nil
	}
	if _, ok := err.(*dberrors.NotFoundError); !ok {
		return nil, fmt.Errorf("lookup child %q under %q: %w", LoanPrincipalChildName, LoanCategoryName, err)
	}
	child = NewSubcategory(LoanPrincipalChildName, parent.ID, TypeExpense)
	if err := s.repo.Create(child); err != nil {
		return nil, fmt.Errorf("create child %q under %q: %w", LoanPrincipalChildName, LoanCategoryName, err)
	}
	return child, nil
}

// GetValueAdjustmentCategory returns the system Value Adjustment
// category used for asset revaluations. Returns a NotFoundError when it
// has not been seeded (call EnsureValueAdjustmentCategory first).
func (s *Service) GetValueAdjustmentCategory() (*Category, error) {
	categories, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	for _, cat := range categories {
		if cat.IsSystem && cat.Name == ValueAdjustmentCategoryName {
			return cat, nil
		}
	}

	return nil, &dberrors.NotFoundError{Entity: "category", ID: ValueAdjustmentCategoryName}
}

// EnsureValueAdjustmentCategory creates the system Value Adjustment
// category when no top-level category of that name exists yet. It is
// idempotent and safe to call on every database open (the
// EnsurePaycheckCategories precedent) so existing files gain the
// category without a migration.
//
// Unlike EnsurePaycheckCategories, which only creates non-system
// categories, this seeds a *system* category so the spending report and
// TUI mutation guards treat it like Transfer.
//
// The boolean return reports a name collision with a pre-existing
// *user* (non-system) category: in that case the system category is NOT
// created (a top-level name is unique), the user's category is left
// untouched, and callers should surface a one-time notice that
// spending-report exclusion will not apply to it. A false return means
// the category was created, already existed as the system category, or
// the lookup failed (see err).
func (s *Service) EnsureValueAdjustmentCategory() (userCollision bool, err error) {
	existing, err := s.repo.GetByName(ValueAdjustmentCategoryName, nil)
	if err == nil {
		// A category with this name already exists at the top level.
		// If it is the system one we seeded, there is nothing to do;
		// otherwise it is a user category we must not touch.
		return !existing.IsSystem, nil
	}
	if _, ok := err.(*dberrors.NotFoundError); !ok {
		return false, fmt.Errorf("lookup %q: %w", ValueAdjustmentCategoryName, err)
	}

	cat := NewSystemCategory(ValueAdjustmentCategoryName, TypeExpense)
	if err := s.repo.Create(cat); err != nil {
		return false, fmt.Errorf("create %q: %w", ValueAdjustmentCategoryName, err)
	}
	return false, nil
}

// HasDefaultCategories checks if default categories have been seeded.
func (s *Service) HasDefaultCategories() (bool, error) {
	categories, err := s.repo.ListTopLevel()
	if err != nil {
		return false, err
	}
	return len(categories) > 0, nil
}

// Create validates and creates a new category.
// If setting a parent, validates the parent exists and type matches.
func (s *Service) Create(category *Category) error {
	if err := s.validateCategory(category); err != nil {
		return err
	}

	// Additional parent validation is done in the repository
	return s.repo.Create(category)
}

// GetByID retrieves a category by its ID.
func (s *Service) GetByID(id types.ID) (*Category, error) {
	return s.repo.GetByID(id)
}

// GetByName retrieves a category by its name within a parent.
// Pass nil for parentID to search for top-level categories.
func (s *Service) GetByName(name string, parentID *types.ID) (*Category, error) {
	return s.repo.GetByName(name, parentID)
}

// GetWithParent retrieves a category and its parent (if any) by ID.
func (s *Service) GetWithParent(id types.ID) (*Category, *Category, error) {
	return s.repo.GetWithParent(id)
}

// Update validates and updates an existing category.
// System categories cannot be updated (except by seeding).
func (s *Service) Update(category *Category) error {
	// Check if this is a system category
	existing, err := s.repo.GetByID(category.ID)
	if err != nil {
		return err
	}
	if existing.IsSystem {
		return &IsSystemError{ID: category.ID.String(), Name: existing.Name}
	}

	if err := s.validateCategory(category); err != nil {
		return err
	}

	return s.repo.Update(category)
}

// Delete removes a category.
// System categories cannot be deleted.
// Categories with transactions or subcategories cannot be deleted.
func (s *Service) Delete(id types.ID) error {
	// Check if this is a system category
	category, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if category.IsSystem {
		return &IsSystemError{ID: id.String(), Name: category.Name}
	}

	// Repository handles checks for subcategories and transactions
	return s.repo.Delete(id)
}

// List returns all categories ordered by name.
func (s *Service) List() ([]*Category, error) {
	return s.repo.List()
}

// ListByType returns all categories of a specific type.
func (s *Service) ListByType(categoryType Type) ([]*Category, error) {
	return s.repo.ListByType(categoryType)
}

// ListTopLevel returns all top-level categories (those without a parent).
func (s *Service) ListTopLevel() ([]*Category, error) {
	return s.repo.ListTopLevel()
}

// ListChildren returns all child categories of a parent.
func (s *Service) ListChildren(parentID types.ID) ([]*Category, error) {
	return s.repo.ListChildren(parentID)
}

// MergeCategories merges the source category into the target category.
// All transactions, splits, scheduled transactions, and payee defaults
// using the source category will be updated to use the target category.
// The source category is then deleted.
//
// Rules:
// - Source and target must have the same type (income/expense)
// - System categories cannot be merged
// - Cannot merge a category into itself
func (s *Service) MergeCategories(sourceID, targetID types.ID) error {
	// Cannot merge into itself
	if sourceID == targetID {
		return &MergeSameError{ID: sourceID.String()}
	}

	// Get both categories
	source, err := s.repo.GetByID(sourceID)
	if err != nil {
		return fmt.Errorf("failed to get source category: %w", err)
	}

	target, err := s.repo.GetByID(targetID)
	if err != nil {
		return fmt.Errorf("failed to get target category: %w", err)
	}

	// System categories cannot be merged
	if source.IsSystem {
		return &IsSystemError{ID: sourceID.String(), Name: source.Name}
	}
	if target.IsSystem {
		return &IsSystemError{ID: targetID.String(), Name: target.Name}
	}

	// Types must match
	if source.Type != target.Type {
		return &MergeTypeMismatchError{
			SourceID:   sourceID.String(),
			SourceType: source.Type.String(),
			TargetID:   targetID.String(),
			TargetType: target.Type.String(),
		}
	}

	// Reassign every reference from source to target in ONE transaction: a
	// mid-reassignment failure rolls the whole set back, so references can never
	// be left split between source and target (the "best-effort" hazard this
	// replaces).
	//
	// The source delete is deliberately NOT inside this transaction. DuckDB
	// forbids deleting a key in the same transaction that earlier dereferenced
	// it: moving every reference off source and then deleting source in one tx
	// fails with "Violates foreign key constraint because key ... is still
	// referenced" — verified against the pinned duckdb-go version for every
	// update form (bulk UPDATE, the repo's DELETE+INSERT, and temp-table
	// DELETE+reinsert). So the delete runs as a separate trailing statement. The
	// residual non-atomicity is benign and self-consistent: if the delete fails
	// after the reassignment commits, source is left as an orphan category with
	// zero references, which a re-run of the merge cleans up idempotently.
	if err := s.runInTx(func(b *Service) error {
		// Update transactions
		if _, err := b.q().Exec(`
			UPDATE transactions
			SET category_id = CAST(? AS UUID), updated_at = CURRENT_TIMESTAMP
			WHERE CAST(category_id AS VARCHAR) = ?
		`, targetID.String(), sourceID.String()); err != nil {
			return fmt.Errorf("failed to update transactions: %w", err)
		}

		// Update transaction splits
		if _, err := b.q().Exec(`
			UPDATE transaction_splits
			SET category_id = CAST(? AS UUID)
			WHERE CAST(category_id AS VARCHAR) = ?
		`, targetID.String(), sourceID.String()); err != nil {
			return fmt.Errorf("failed to update transaction splits: %w", err)
		}

		// Update payee defaults
		if _, err := b.q().Exec(`
			UPDATE payees
			SET default_category_id = CAST(? AS UUID), updated_at = CURRENT_TIMESTAMP
			WHERE CAST(default_category_id AS VARCHAR) = ?
		`, targetID.String(), sourceID.String()); err != nil {
			return fmt.Errorf("failed to update payee defaults: %w", err)
		}

		// Update scheduled transactions
		if _, err := b.q().Exec(`
			UPDATE scheduled_transactions
			SET category_id = CAST(? AS UUID), updated_at = CURRENT_TIMESTAMP
			WHERE CAST(category_id AS VARCHAR) = ?
		`, targetID.String(), sourceID.String()); err != nil {
			return fmt.Errorf("failed to update scheduled transactions: %w", err)
		}

		// If source has children, reassign them to target
		children, err := b.repo.ListChildren(sourceID)
		if err != nil {
			return fmt.Errorf("failed to list source children: %w", err)
		}
		for _, child := range children {
			child.SetParent(targetID)
			if err := b.repo.Update(child); err != nil {
				return fmt.Errorf("failed to reassign child category %s: %w", child.Name, err)
			}
		}

		return nil
	}); err != nil {
		return err
	}

	// Delete the source category — now dereferenced — in its own statement (see
	// the transaction comment above for why it cannot join the reassignment tx).
	if err := s.repo.Delete(sourceID); err != nil {
		return fmt.Errorf("failed to delete source category: %w", err)
	}

	return nil
}

// validateCategory validates a category and returns any validation errors.
func (s *Service) validateCategory(category *Category) error {
	errors := category.Validate()
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}
	return nil
}
