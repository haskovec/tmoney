package tui

import "github.com/haskovec/tmoney/internal/tui/theme"

// buildThemeMenuItems returns the View → Theme dropdown contents:
// one entry per built-in theme, in the order BuiltinIDs() returns
// them. The entry whose ID matches activeID is prefixed with "✓ ";
// the others use a 2-space prefix so the IDs stay column-aligned in
// the dropdown.
//
// User-directory themes are folded in by Phase 7 (TH-027); this build
// covers built-ins only.
func buildThemeMenuItems(activeID string) []menuItem {
	ids := theme.BuiltinIDs()
	items := make([]menuItem, 0, len(ids))
	for _, id := range ids {
		prefix := "  "
		if id == activeID {
			prefix = "✓ "
		}
		items = append(items, menuItem{
			label:  prefix + id,
			action: MenuActionLoadTheme,
			data:   id,
		})
	}
	return items
}
