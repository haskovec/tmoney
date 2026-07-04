package transfer

import (
	"errors"
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// resolveTransferCategory resolves a category path ("Parent" or
// "Parent:Subcategory") to a NullableID for labeling a transfer.
//
// Unlike the loan wizard's getOrCreateCategoryPath, it looks up an existing
// category only — `transfer add` / `transfer edit` label transfers with
// categories you already report on and never create new ones — and it rejects
// the system categories (Transfer, Value Adjustment) via
// transaction.ValidateTransferCategory. An empty (or all-whitespace) path
// returns a cleared NullableID, meaning "no category" (the `transfer edit
// --category ""` clear).
func resolveTransferCategory(svc *app.Services, path string) (types.NullableID, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return types.NullableID{}, nil
	}

	parentName, childName, hasChild := strings.Cut(path, ":")
	parentName = strings.TrimSpace(parentName)
	if parentName == "" {
		return types.NullableID{}, fmt.Errorf("empty category name in %q", path)
	}

	cat, err := lookupCategory(svc, parentName, nil, path)
	if err != nil {
		return types.NullableID{}, err
	}

	if hasChild {
		childName = strings.TrimSpace(childName)
		if childName == "" {
			return types.NullableID{}, fmt.Errorf("invalid category path %q (missing subcategory name)", path)
		}
		cat, err = lookupCategory(svc, childName, &cat.ID, path)
		if err != nil {
			return types.NullableID{}, err
		}
	}

	if err := transaction.ValidateTransferCategory(cat); err != nil {
		return types.NullableID{}, err
	}
	return types.NullableID{ID: cat.ID, Valid: true}, nil
}

// lookupCategory fetches a single category by name (and optional parent),
// mapping a not-found result to a user-facing "category %q not found" error
// that names the full path the user supplied.
func lookupCategory(svc *app.Services, name string, parentID *types.ID, path string) (*category.Category, error) {
	cat, err := svc.Category.GetByName(name, parentID)
	if err == nil {
		return cat, nil
	}
	var notFound *dberrors.NotFoundError
	if errors.As(err, &notFound) {
		return nil, fmt.Errorf("category %q not found", path)
	}
	return nil, fmt.Errorf("failed to look up category %q: %w", path, err)
}
