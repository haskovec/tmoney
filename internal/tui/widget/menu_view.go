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

// BuildChartImagesItem returns the "Chart images" entry for the View
// menu, with the setting's value in the label: "Auto", "On" or "Off". It
// has the 2-space prefix of an unchecked toggle, so the labels stay
// column-aligned. Selecting the item dispatches MenuActionCycleChartImages;
// the handler moves the setting to the next value and saves it.
func BuildChartImagesItem(label string) MenuItem {
	return MenuItem{
		Label:  "  Chart images: " + label,
		Action: MenuActionCycleChartImages,
	}
}

// BuildViewMenuItems composes the full View-menu dropdown: the
// "Show closed positions" toggle and the "Chart images" setting on top,
// followed by every theme entry. The settings are listed first because
// they change *what* is rendered, whereas the theme entries change *how*
// it is rendered — the structural settings go ahead of the cosmetic ones.
// chartImages is the label of the Chart images value.
func BuildViewMenuItems(activeTheme string, showClosed bool, chartImages string) []MenuItem {
	items := []MenuItem{BuildShowClosedPositionsItem(showClosed), BuildChartImagesItem(chartImages)}
	items = append(items, BuildThemeMenuItems(activeTheme)...)
	return items
}
