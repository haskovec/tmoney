package widget

import (
	"sort"

	"github.com/haskovec/tmoney/internal/tui/theme"
)

// BuildThemeMenuItems returns the View → Theme dropdown contents:
// every embedded built-in plus every theme discovered in the user
// theme directory, deduped by ID and sorted alphabetically. The entry
// whose ID matches activeID is prefixed with "✓ "; the others use a
// 2-space prefix so the IDs stay column-aligned in the dropdown.
//
// User-directory themes whose ID matches a built-in (e.g. a user
// `default.toml`) appear once: theme.LoadTheme already prefers the
// user file at load time, so showing the ID twice would only be
// noise. A failure to read the user dir falls back to built-ins
// only — first-run users with no themes installed should still see
// the menu work.
func BuildThemeMenuItems(activeID string) []MenuItem {
	seen := map[string]bool{}
	ids := make([]string, 0)
	for _, id := range theme.BuiltinIDs() {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if userIDs, err := theme.DiscoverUserThemes(); err == nil {
		for _, id := range userIDs {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	sort.Strings(ids)

	items := make([]MenuItem, 0, len(ids))
	for _, id := range ids {
		prefix := "  "
		if id == activeID {
			prefix = "✓ "
		}
		items = append(items, MenuItem{
			Label:  prefix + id,
			Action: MenuActionLoadTheme,
			Data:   id,
		})
	}
	return items
}
