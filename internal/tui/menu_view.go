package tui

// buildShowClosedPositionsItem returns the "Show closed positions"
// toggle entry for the View menu. When the toggle is on the label is
// prefixed with "✓ "; when off it uses a 2-space prefix so the labels
// stay column-aligned with the other View-menu entries. Selecting the
// item dispatches MenuActionToggleClosedPositions; the handler flips
// `cfg.ShowClosedPositions` and reloads any open valuation-bearing
// view so the new IncludeClosed setting takes effect immediately.
func buildShowClosedPositionsItem(showClosed bool) menuItem {
	prefix := "  "
	if showClosed {
		prefix = "✓ "
	}
	return menuItem{
		label:  prefix + "Show closed positions",
		action: MenuActionToggleClosedPositions,
	}
}

// buildViewMenuItems composes the full View-menu dropdown: the
// "Show closed positions" toggle on top, followed by every theme
// entry. The toggle is listed first because it changes *what* is
// rendered, whereas the theme entries change *how* it is rendered —
// the structural setting goes ahead of the cosmetic ones.
func buildViewMenuItems(activeTheme string, showClosed bool) []menuItem {
	items := []menuItem{buildShowClosedPositionsItem(showClosed)}
	items = append(items, buildThemeMenuItems(activeTheme)...)
	return items
}
