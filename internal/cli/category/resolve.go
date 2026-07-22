package category

import (
	"fmt"
	"strings"

	categorydom "github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/types"
)

// categoryResolver resolves category references for the CLI. It wraps the
// domain service; the methods here define the management noun's stricter
// name-resolution semantics (exact-match first, explicit ambiguity errors)
// rather than reusing the looser transaction-edit resolver.
type categoryResolver struct {
	svc *categorydom.Service
}

// resolve turns a user-supplied reference into a single category. The
// reference may be, in priority order:
//
//   - a raw UUID (resolved via GetByID);
//   - a "Parent:Child" path (parent resolved as an exact top-level name,
//     then the child under it);
//   - an exact top-level category name;
//   - a bare name matching exactly one category anywhere in the tree.
//
// A bare name matching more than one category is an error that tells the
// user to disambiguate with "Parent:Child" or --id. Zero matches yields a
// "not found" error.
func (r categoryResolver) resolve(ref string) (*categorydom.Category, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("category reference must not be empty")
	}

	// Raw UUID takes precedence wherever an id-or-name is accepted.
	if id, err := types.ParseID(ref); err == nil {
		cat, err := r.svc.GetByID(id)
		if err != nil {
			return nil, fmt.Errorf("category %q not found", ref)
		}
		return cat, nil
	}

	// Parent:Child path.
	if before, after, ok := strings.Cut(ref, ":"); ok {
		parentName := strings.TrimSpace(before)
		childName := strings.TrimSpace(after)
		if parentName == "" || childName == "" {
			return nil, fmt.Errorf("invalid category path %q: expected Parent:Child", ref)
		}
		parent, err := r.svc.GetByName(parentName, nil)
		if err != nil {
			return nil, fmt.Errorf("parent category %q not found", parentName)
		}
		child, err := r.svc.GetByName(childName, &parent.ID)
		if err != nil {
			return nil, fmt.Errorf("category %q not found", ref)
		}
		return child, nil
	}

	// Exact top-level match wins outright.
	if cat, err := r.svc.GetByName(ref, nil); err == nil {
		return cat, nil
	}

	// Otherwise scan every category for a bare-name match, erroring on
	// ambiguity so the user disambiguates deliberately.
	all, err := r.svc.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list categories: %w", err)
	}
	var matches []*categorydom.Category
	for _, cat := range all {
		if cat.Name == ref {
			matches = append(matches, cat)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("category %q not found", ref)
	case 1:
		return matches[0], nil
	default:
		return nil, fmt.Errorf(
			"category %q is ambiguous (%d matches); use Parent:Child or --id (see `category list --show-ids`)",
			ref, len(matches))
	}
}
