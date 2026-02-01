package repository

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
)

// CategoryRepository provides database operations for categories.
type CategoryRepository struct {
	db *db.DB
}

// NewCategoryRepository creates a new CategoryRepository.
func NewCategoryRepository(database *db.DB) *CategoryRepository {
	return &CategoryRepository{db: database}
}

// Create inserts a new category into the database.
func (r *CategoryRepository) Create(category *models.Category) error {
	// Check for duplicate name within the same parent
	var exists bool
	if category.ParentID.Valid {
		err := r.db.Conn().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM categories WHERE name = ? AND CAST(parent_id AS VARCHAR) = ?)`,
			category.Name, category.ParentID.ID.String(),
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check category name uniqueness: %w", err)
		}
	} else {
		err := r.db.Conn().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM categories WHERE name = ? AND parent_id IS NULL)`,
			category.Name,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check category name uniqueness: %w", err)
		}
	}
	if exists {
		return &DuplicateError{Entity: "category", Field: "name", Value: category.Name}
	}

	// If this is a subcategory, verify parent exists and has the same type
	if category.ParentID.Valid {
		parent, err := r.GetByID(category.ParentID.ID)
		if err != nil {
			if _, ok := err.(*NotFoundError); ok {
				return fmt.Errorf("parent category not found: %s", category.ParentID.ID.String())
			}
			return fmt.Errorf("failed to verify parent category: %w", err)
		}
		if parent.Type != category.Type {
			return fmt.Errorf("subcategory type must match parent type")
		}
		if parent.ParentID.Valid {
			return fmt.Errorf("cannot create subcategory under another subcategory")
		}
	}

	query := `
		INSERT INTO categories (
			id, name, parent_id, type, system_category,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.Conn().Exec(query,
		category.ID,
		category.Name,
		nullID(category.ParentID),
		category.Type,
		category.IsSystem,
		category.CreatedAt,
		category.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create category: %w", err)
	}

	return nil
}

// GetByID retrieves a category by its ID.
func (r *CategoryRepository) GetByID(id models.ID) (*models.Category, error) {
	query := `
		SELECT id, name, parent_id, type, system_category,
			created_at, updated_at
		FROM categories
		WHERE CAST(id AS VARCHAR) = ?
	`

	category := &models.Category{}
	err := r.db.Conn().QueryRow(query, id.String()).Scan(
		&category.ID,
		&category.Name,
		&category.ParentID,
		&category.Type,
		&category.IsSystem,
		&category.CreatedAt,
		&category.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Entity: "category", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get category: %w", err)
	}

	return category, nil
}

// GetByName retrieves a category by its name within a parent.
// Pass nil for parentID to search for top-level categories.
func (r *CategoryRepository) GetByName(name string, parentID *models.ID) (*models.Category, error) {
	var query string
	var args []interface{}

	if parentID != nil {
		query = `
			SELECT id, name, parent_id, type, system_category,
				created_at, updated_at
			FROM categories
			WHERE name = ? AND CAST(parent_id AS VARCHAR) = ?
		`
		args = []interface{}{name, parentID.String()}
	} else {
		query = `
			SELECT id, name, parent_id, type, system_category,
				created_at, updated_at
			FROM categories
			WHERE name = ? AND parent_id IS NULL
		`
		args = []interface{}{name}
	}

	category := &models.Category{}
	err := r.db.Conn().QueryRow(query, args...).Scan(
		&category.ID,
		&category.Name,
		&category.ParentID,
		&category.Type,
		&category.IsSystem,
		&category.CreatedAt,
		&category.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &NotFoundError{Entity: "category", ID: name}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get category by name: %w", err)
	}

	return category, nil
}

// GetWithParent retrieves a category and its parent (if any) by ID.
// Returns the category and its parent. Parent will be nil if category is top-level.
func (r *CategoryRepository) GetWithParent(id models.ID) (*models.Category, *models.Category, error) {
	category, err := r.GetByID(id)
	if err != nil {
		return nil, nil, err
	}

	if !category.ParentID.Valid {
		return category, nil, nil
	}

	parent, err := r.GetByID(category.ParentID.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get parent category: %w", err)
	}

	return category, parent, nil
}

// List retrieves all categories ordered by name.
func (r *CategoryRepository) List() ([]*models.Category, error) {
	query := `
		SELECT id, name, parent_id, type, system_category,
			created_at, updated_at
		FROM categories
		ORDER BY name
	`

	return r.queryCategories(query)
}

// ListByType retrieves all categories of a specific type.
func (r *CategoryRepository) ListByType(categoryType models.CategoryType) ([]*models.Category, error) {
	query := `
		SELECT id, name, parent_id, type, system_category,
			created_at, updated_at
		FROM categories
		WHERE type = ?
		ORDER BY name
	`

	return r.queryCategoriesWithArgs(query, categoryType.String())
}

// ListTopLevel retrieves all top-level categories (those without a parent).
func (r *CategoryRepository) ListTopLevel() ([]*models.Category, error) {
	query := `
		SELECT id, name, parent_id, type, system_category,
			created_at, updated_at
		FROM categories
		WHERE parent_id IS NULL
		ORDER BY name
	`

	return r.queryCategories(query)
}

// ListChildren retrieves all child categories of a parent.
func (r *CategoryRepository) ListChildren(parentID models.ID) ([]*models.Category, error) {
	query := `
		SELECT id, name, parent_id, type, system_category,
			created_at, updated_at
		FROM categories
		WHERE CAST(parent_id AS VARCHAR) = ?
		ORDER BY name
	`

	return r.queryCategoriesWithArgs(query, parentID.String())
}

// Update updates an existing category in the database.
// Note: Uses DELETE + INSERT (non-transactional) due to DuckDB limitations.
func (r *CategoryRepository) Update(category *models.Category) error {
	category.Touch()

	// Check for duplicate name within the same parent (excluding current category)
	var exists bool
	if category.ParentID.Valid {
		err := r.db.Conn().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM categories WHERE name = ? AND CAST(parent_id AS VARCHAR) = ? AND CAST(id AS VARCHAR) != ?)`,
			category.Name, category.ParentID.ID.String(), category.ID.String(),
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check category name uniqueness: %w", err)
		}
	} else {
		err := r.db.Conn().QueryRow(
			`SELECT EXISTS(SELECT 1 FROM categories WHERE name = ? AND parent_id IS NULL AND CAST(id AS VARCHAR) != ?)`,
			category.Name, category.ID.String(),
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check category name uniqueness: %w", err)
		}
	}
	if exists {
		return &DuplicateError{Entity: "category", Field: "name", Value: category.Name}
	}

	// Check if category exists
	var count int
	err := r.db.Conn().QueryRow(`SELECT COUNT(*) FROM categories WHERE CAST(id AS VARCHAR) = ?`, category.ID.String()).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check category exists: %w", err)
	}
	if count == 0 {
		return &NotFoundError{Entity: "category", ID: category.ID.String()}
	}

	// If setting a parent, verify it exists and matches type
	if category.ParentID.Valid {
		parent, err := r.GetByID(category.ParentID.ID)
		if err != nil {
			if _, ok := err.(*NotFoundError); ok {
				return fmt.Errorf("parent category not found: %s", category.ParentID.ID.String())
			}
			return fmt.Errorf("failed to verify parent category: %w", err)
		}
		if parent.Type != category.Type {
			return fmt.Errorf("subcategory type must match parent type")
		}
		if parent.ParentID.Valid {
			return fmt.Errorf("cannot create subcategory under another subcategory")
		}
	}

	// Delete the existing record
	_, err = r.db.Conn().Exec(`DELETE FROM categories WHERE CAST(id AS VARCHAR) = ?`, category.ID.String())
	if err != nil {
		return fmt.Errorf("failed to delete for update: %w", err)
	}

	// Insert the updated record
	insertQuery := `
		INSERT INTO categories (
			id, name, parent_id, type, system_category,
			created_at, updated_at
		) VALUES (CAST(? AS UUID), ?, ?, ?, ?, ?, ?)
	`
	_, err = r.db.Conn().Exec(insertQuery,
		category.ID.String(),
		category.Name,
		nullID(category.ParentID),
		category.Type.String(),
		category.IsSystem,
		category.CreatedAt.Time(),
		category.UpdatedAt.Time(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert for update: %w", err)
	}

	return nil
}

// Delete removes a category from the database.
// This will fail if the category has any subcategories or transactions.
func (r *CategoryRepository) Delete(id models.ID) error {
	// Check for subcategories
	var childCount int
	err := r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM categories WHERE CAST(parent_id AS VARCHAR) = ?
	`, id.String()).Scan(&childCount)
	if err != nil {
		return fmt.Errorf("failed to check subcategories: %w", err)
	}
	if childCount > 0 {
		return &HasDependentsError{
			Entity:     "category",
			ID:         id.String(),
			Dependents: "subcategories",
			Count:      childCount,
		}
	}

	// Check for transactions
	var txnCount int
	err = r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM transactions WHERE CAST(category_id AS VARCHAR) = ?
	`, id.String()).Scan(&txnCount)
	if err != nil {
		return fmt.Errorf("failed to check transactions: %w", err)
	}
	if txnCount > 0 {
		return &HasDependentsError{
			Entity:     "category",
			ID:         id.String(),
			Dependents: "transactions",
			Count:      txnCount,
		}
	}

	result, err := r.db.Conn().Exec(`DELETE FROM categories WHERE CAST(id AS VARCHAR) = ?`, id.String())
	if err != nil {
		return fmt.Errorf("failed to delete category: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &NotFoundError{Entity: "category", ID: id.String()}
	}

	return nil
}

// queryCategories executes a query and returns a slice of categories.
func (r *CategoryRepository) queryCategories(query string) ([]*models.Category, error) {
	rows, err := r.db.Conn().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query categories: %w", err)
	}
	defer rows.Close()

	return r.scanCategories(rows)
}

// queryCategoriesWithArgs executes a query with arguments and returns a slice of categories.
func (r *CategoryRepository) queryCategoriesWithArgs(query string, args ...interface{}) ([]*models.Category, error) {
	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query categories: %w", err)
	}
	defer rows.Close()

	return r.scanCategories(rows)
}

// scanCategories scans rows into a slice of categories.
func (r *CategoryRepository) scanCategories(rows *sql.Rows) ([]*models.Category, error) {
	var categories []*models.Category
	for rows.Next() {
		category := &models.Category{}
		err := rows.Scan(
			&category.ID,
			&category.Name,
			&category.ParentID,
			&category.Type,
			&category.IsSystem,
			&category.CreatedAt,
			&category.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan category: %w", err)
		}
		categories = append(categories, category)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating categories: %w", err)
	}

	return categories, nil
}

// nullID converts NullableID to a value for database insertion.
func nullID(nid models.NullableID) interface{} {
	if nid.Valid {
		return nid.ID.String()
	}
	return nil
}
