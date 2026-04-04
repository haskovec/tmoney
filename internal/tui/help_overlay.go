package tui

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// shortcutEntry represents a single keyboard shortcut for display in the help overlay.
type shortcutEntry struct {
	Key         string
	Description string
}

// shortcutSection represents a group of related keyboard shortcuts.
type shortcutSection struct {
	Title   string
	Entries []shortcutEntry
}

// globalShortcuts returns the global keyboard shortcuts.
func globalShortcuts() shortcutSection {
	return shortcutSection{
		Title: "Global",
		Entries: []shortcutEntry{
			{"?", "Show/hide this help"},
			{"Ctrl+Q", "Quit application"},
			{"Esc", "Close dialog / Go back"},
			{"Tab", "Next pane / Next field"},
			{"Shift+Tab", "Previous pane / Previous field"},
			{undoShortcutLabel(), "Undo"},
			{redoShortcutLabel(), "Redo"},
			{"/", "Search"},
			{"1", "Dashboard view"},
			{"2", "Scheduled view"},
			{"3", "Reports view"},
			{"F10", "Activate menu bar"},
			{"Alt+F", "File menu"},
			{"Alt+E", "Edit menu"},
			{"Alt+A", "Accounts menu"},
			{"Alt+T", "Transactions menu"},
			{"Alt+R", "Reports menu"},
			{"Alt+H", "Help menu"},
		},
	}
}

// navigationShortcuts returns the navigation keyboard shortcuts.
func navigationShortcuts() shortcutSection {
	return shortcutSection{
		Title: "Navigation",
		Entries: []shortcutEntry{
			{"Up / k", "Move up"},
			{"Down / j", "Move down"},
			{"Left / h", "Collapse / Previous"},
			{"Right / l", "Expand / Next"},
			{"Home / g", "Go to first item"},
			{"End / G", "Go to last item"},
			{"PgUp", "Page up"},
			{"PgDn", "Page down"},
		},
	}
}

// mouseShortcuts returns the mouse interaction hints.
func mouseShortcuts() shortcutSection {
	return shortcutSection{
		Title: "Mouse",
		Entries: []shortcutEntry{
			{"Click", "Menu items, accounts, rows"},
			{"Scroll", "Navigate lists and tables"},
		},
	}
}

// dashboardShortcuts returns the dashboard view keyboard shortcuts.
func dashboardShortcuts() shortcutSection {
	return shortcutSection{
		Title: "Dashboard",
		Entries: []shortcutEntry{
			{"Enter", "Open selected account"},
			{"n", "New account"},
			{"Up/Down", "Navigate accounts"},
			{"Left/Right", "Collapse/expand groups"},
		},
	}
}

// registerShortcuts returns the register view keyboard shortcuts.
func registerShortcuts() shortcutSection {
	return shortcutSection{
		Title: "Register",
		Entries: []shortcutEntry{
			{"n", "New transaction"},
			{"Enter", "Edit transaction"},
			{"d", "Delete transaction"},
			{"c", "Toggle cleared/uncleared"},
			{"v", "Void transaction"},
			{"t", "New transfer"},
			{"Tab", "Switch sidebar/table focus"},
		},
	}
}

// scheduledShortcuts returns the scheduled transactions view keyboard shortcuts.
func scheduledShortcuts() shortcutSection {
	return shortcutSection{
		Title: "Scheduled Transactions",
		Entries: []shortcutEntry{
			{"Enter", "Post scheduled transaction"},
			{"s", "Skip occurrence"},
			{"e", "Edit scheduled transaction"},
			{"n", "New scheduled transaction"},
			{"d", "Delete scheduled transaction"},
		},
	}
}

// reportsShortcuts returns the reports view keyboard shortcuts.
func reportsShortcuts() shortcutSection {
	return shortcutSection{
		Title: "Reports",
		Entries: []shortcutEntry{
			{"Left/Right", "Change period"},
			{"n", "Net worth report"},
			{"s", "Spending report"},
			{"y", "Yearly view"},
			{"m", "Monthly view"},
		},
	}
}

// reconciliationShortcuts returns the reconciliation view keyboard shortcuts.
func reconciliationShortcuts() shortcutSection {
	return shortcutSection{
		Title: "Reconciliation",
		Entries: []shortcutEntry{
			{"Space", "Toggle checkbox"},
			{"Up/Down", "Navigate transactions"},
			{"Home/g", "First transaction"},
			{"End/G", "Last transaction"},
			{"Enter", "Finish reconciliation"},
			{"Esc", "Cancel reconciliation"},
			{"a", "Check all"},
			{"u", "Uncheck all"},
		},
	}
}

// securitiesShortcuts returns the securities view keyboard shortcuts.
func securitiesShortcuts() shortcutSection {
	return shortcutSection{
		Title: "Securities",
		Entries: []shortcutEntry{
			{"n", "New security"},
			{"Enter", "Edit security"},
			{"h", "Toggle hidden status"},
			{"d", "Delete security"},
			{"f", "Toggle show hidden"},
			{"/", "Search securities"},
			{"p", "View prices"},
			{"Esc", "Back"},
		},
	}
}

// pricesShortcuts returns the prices view keyboard shortcuts.
func pricesShortcuts() shortcutSection {
	return shortcutSection{
		Title: "Prices",
		Entries: []shortcutEntry{
			{"n", "New price"},
			{"Enter", "Edit price"},
			{"d", "Delete price"},
			{"i", "Import prices from CSV"},
			{"/", "Search securities"},
			{"Left/Right", "Change security"},
			{"Esc", "Back"},
		},
	}
}

// dialogShortcuts returns the dialog keyboard shortcuts.
func dialogShortcuts() shortcutSection {
	return shortcutSection{
		Title: "Dialogs",
		Entries: []shortcutEntry{
			{"Tab", "Next field"},
			{"Shift+Tab", "Previous field"},
			{"Enter", "Submit / Confirm"},
			{"Esc", "Cancel / Close"},
			{"Up/Down", "Navigate options"},
			{"Space", "Toggle checkbox"},
		},
	}
}

// allShortcutSections returns all shortcut sections in display order.
func allShortcutSections() []shortcutSection {
	return []shortcutSection{
		globalShortcuts(),
		navigationShortcuts(),
		dashboardShortcuts(),
		registerShortcuts(),
		scheduledShortcuts(),
		reportsShortcuts(),
		securitiesShortcuts(),
		reconciliationShortcuts(),
		dialogShortcuts(),
	}
}

// viewShortcutSections returns the shortcut sections relevant to the given view,
// including global and navigation sections plus the view-specific section.
func viewShortcutSections(view View) []shortcutSection {
	sections := []shortcutSection{
		globalShortcuts(),
		navigationShortcuts(),
	}

	switch view {
	case ViewDashboard:
		sections = append(sections, dashboardShortcuts())
	case ViewRegister:
		sections = append(sections, registerShortcuts())
	case ViewScheduled:
		sections = append(sections, scheduledShortcuts())
	case ViewReports:
		sections = append(sections, reportsShortcuts())
	case ViewReconciliation:
		sections = append(sections, reconciliationShortcuts())
	case ViewSecurities:
		sections = append(sections, securitiesShortcuts())
	case ViewPrices:
		sections = append(sections, pricesShortcuts())
	case ViewInvestmentRegister:
		sections = append(sections, investmentRegisterShortcuts())
	case ViewPortfolio:
		sections = append(sections, portfolioShortcuts())
	}

	sections = append(sections, dialogShortcuts())
	sections = append(sections, mouseShortcuts())
	return sections
}

// helpOverlayWidth is the default width of the help overlay.
const helpOverlayWidth = 60

// renderHelpOverlay renders the help overlay for the given view and screen dimensions.
func renderHelpOverlay(styles Styles, view View, screenWidth, screenHeight int) string {
	sections := viewShortcutSections(view)

	overlayWidth := helpOverlayWidth
	if screenWidth > 0 && overlayWidth > screenWidth-4 {
		overlayWidth = screenWidth - 4
	}
	if overlayWidth < 30 {
		overlayWidth = 30
	}

	contentWidth := overlayWidth - dialogHorizontalOverhead

	var lines []string

	// Title
	title := styles.DialogTitle.Render("KEYBOARD SHORTCUTS")
	closeBtn := styles.Muted.Render("[x]")
	gap := max(contentWidth-lipgloss.Width(title)-lipgloss.Width(closeBtn), 1)
	lines = append(lines, title+strings.Repeat(" ", gap)+closeBtn)

	// Separator
	lines = append(lines, strings.Repeat("─", contentWidth))

	// Render each section
	maxKeyWidth := 0
	for _, section := range sections {
		for _, entry := range section.Entries {
			if len(entry.Key) > maxKeyWidth {
				maxKeyWidth = len(entry.Key)
			}
		}
	}

	// Dialog border/padding overhead: top border(1) + top padding(1) + bottom padding(1) + bottom border(1)
	// Plus header lines (title + separator = 2) and footer lines (blank + separator + hint = 3)
	dialogOverhead := 4 // border and padding
	headerLines := 2    // title + separator (already in lines)
	footerLines := 3    // blank + separator + close hint
	maxContentLines := max(screenHeight-dialogOverhead-headerLines-footerLines, 5)

	contentLineCount := 0
	for i, section := range sections {
		if i > 0 {
			if contentLineCount >= maxContentLines {
				break
			}
			lines = append(lines, "")
			contentLineCount++
		}
		if contentLineCount >= maxContentLines {
			break
		}
		lines = append(lines, styles.Bold.Render(section.Title))
		contentLineCount++

		for _, entry := range section.Entries {
			if contentLineCount >= maxContentLines {
				break
			}
			keyStr := fmt.Sprintf("  %-*s", maxKeyWidth+2, entry.Key)
			line := styles.Positive.Render(keyStr) + styles.Muted.Render("  "+entry.Description)
			lines = append(lines, line)
			contentLineCount++
		}
	}

	// Footer
	lines = append(lines, "")
	lines = append(lines, strings.Repeat("─", contentWidth))
	lines = append(lines, styles.Muted.Render("Press ? or Esc to close"))

	content := strings.Join(lines, "\n")

	// Use dialog style for consistent appearance
	rendered := styles.Dialog.Width(overlayWidth).Render(content)

	// Final height clamp to ensure we don't exceed screen
	renderedLines := strings.Split(rendered, "\n")
	maxHeight := max(screenHeight-2, 5)
	if len(renderedLines) > maxHeight {
		renderedLines = renderedLines[:maxHeight]
	}

	return strings.Join(renderedLines, "\n")
}

// undoShortcutLabel returns the platform-appropriate label for the undo shortcut.
func undoShortcutLabel() string {
	if runtime.GOOS == "darwin" {
		return "Cmd+Z"
	}
	return "Ctrl+Z"
}

// redoShortcutLabel returns the platform-appropriate label for the redo shortcut.
func redoShortcutLabel() string {
	if runtime.GOOS == "darwin" {
		return "Cmd+Y"
	}
	return "Ctrl+Y"
}
