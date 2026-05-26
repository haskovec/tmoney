package widget

// BuildShowClosedPositionsItem returns the "Show closed positions"
// toggle entry for the View menu. When the toggle is on the label is
// prefixed with "✓ "; when off it uses a 2-space prefix so the labels
// stay column-aligned with the other View-menu entries. Selecting the
// item dispatches MenuActionToggleClosedPositions; the handler flips
// `cfg.ShowClosedPositions` and reloads any open valuation-bearing
// view so the new IncludeClosed setting takes effect immediately.
func BuildShowClosedPositionsItem(showClosed bool) MenuItem {
	prefix := "  "
	if showClosed {
		prefix = "✓ "
	}
	return MenuItem{
		Label:  prefix + "Show closed positions",
		Action: MenuActionToggleClosedPositions,
	}
}

// BuildViewMenuItems composes the full View-menu dropdown: the
// "Show closed positions" toggle on top, followed by every theme
// entry. The toggle is listed first because it changes *what* is
// rendered, whereas the theme entries change *how* it is rendered —
// the structural setting goes ahead of the cosmetic ones.
func BuildViewMenuItems(activeTheme string, showClosed bool) []MenuItem {
	items := []MenuItem{BuildShowClosedPositionsItem(showClosed)}
	items = append(items, BuildThemeMenuItems(activeTheme)...)
	return items
}
