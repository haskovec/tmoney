package undo

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// CreateCategoryCommand
// =============================================================================

// CreateCategoryCommand creates a category and can undo it by deleting.
type CreateCategoryCommand struct {
	svc *category.Service
	cat *category.Category
}

// NewCreateCategoryCommand creates a command that will create a category.
// The category is created on Execute and deleted on Undo.
func NewCreateCategoryCommand(svc *category.Service, cat *category.Category) *CreateCategoryCommand {
	return &CreateCategoryCommand{
		svc: svc,
		cat: cat,
	}
}

func (c *CreateCategoryCommand) Execute() error {
	return c.svc.Create(c.cat)
}

func (c *CreateCategoryCommand) Undo() error {
	return c.svc.Delete(c.cat.ID)
}

func (c *CreateCategoryCommand) Description() string {
	return "Create category"
}

// =============================================================================
// EditCategoryCommand
// =============================================================================

// EditCategoryCommand edits a category and can undo it by restoring
// the previous state.
type EditCategoryCommand struct {
	svc    *category.Service
	before *category.Category // state before editing (captured on Execute)
	after  *category.Category // desired new state
}

// NewEditCategoryCommand creates a command that will update a category.
// The before state is captured at execute time by reading from the database.
func NewEditCategoryCommand(svc *category.Service, after *category.Category) *EditCategoryCommand {
	return &EditCategoryCommand{
		svc:   svc,
		after: after,
	}
}

func (c *EditCategoryCommand) Execute() error {
	// Capture before state from the database
	before, err := c.svc.GetByID(c.after.ID)
	if err != nil {
		return err
	}
	c.before = before

	return c.svc.Update(c.after)
}

func (c *EditCategoryCommand) Undo() error {
	return c.svc.Update(c.before)
}

func (c *EditCategoryCommand) Description() string {
	return "Edit category"
}

// =============================================================================
// DeleteCategoryCommand
// =============================================================================

// DeleteCategoryCommand deletes a category and can undo it by recreating.
type DeleteCategoryCommand struct {
	svc    *category.Service
	id     types.ID
	before *category.Category // full entity captured on Execute for undo
}

// NewDeleteCategoryCommand creates a command that will delete a category.
// The full entity is captured at execute time so it can be recreated on undo.
func NewDeleteCategoryCommand(svc *category.Service, id types.ID) *DeleteCategoryCommand {
	return &DeleteCategoryCommand{
		svc: svc,
		id:  id,
	}
}

func (c *DeleteCategoryCommand) Execute() error {
	// Capture full entity before deleting
	cat, err := c.svc.GetByID(c.id)
	if err != nil {
		return err
	}
	c.before = cat

	return c.svc.Delete(c.id)
}

func (c *DeleteCategoryCommand) Undo() error {
	return c.svc.Create(c.before)
}

func (c *DeleteCategoryCommand) Description() string {
	return "Delete category"
}

// =============================================================================
// MergeCategoriesCommand
// =============================================================================

// MergeCategoriesCommand merges a source category into a target category.
// This is a compound operation: it updates all references and deletes the source.
// Undo is not supported for merge operations due to their complexity
// (they modify transactions, splits, scheduled transactions, payee defaults,
// and child categories across multiple tables).
type MergeCategoriesCommand struct {
	svc      *category.Service
	sourceID types.ID
	targetID types.ID
}

// NewMergeCategoriesCommand creates a command that will merge sourceID into targetID.
func NewMergeCategoriesCommand(svc *category.Service, sourceID, targetID types.ID) *MergeCategoriesCommand {
	return &MergeCategoriesCommand{
		svc:      svc,
		sourceID: sourceID,
		targetID: targetID,
	}
}

func (c *MergeCategoriesCommand) Execute() error {
	return c.svc.MergeCategories(c.sourceID, c.targetID)
}

func (c *MergeCategoriesCommand) Undo() error {
	// Merge is not reversible - the source category and all original references
	// are lost. A backup should be created before merge operations.
	return fmt.Errorf("merge categories cannot be undone")
}

func (c *MergeCategoriesCommand) Description() string {
	return "Merge categories"
}
