package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/types"
)

// formatDashboardMoney formats a Money value with $ prefix for dashboard display.
func formatDashboardMoney(m types.Money) string {
	value := fmt.Sprintf("%.2f", m.Float64())
	if m.IsNegative() {
		return fmt.Sprintf("-$%s", strings.TrimPrefix(value, "-"))
	}
	return fmt.Sprintf("$%s", value)
}

// padRight pads a string with spaces to the given width.
func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// truncate truncates a string to maxLen characters, adding "..." if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// truncateRunes truncates a string to maxLen runes, adding "..." if needed.
func truncateRunes(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

// overlayDropdown places a dropdown string on top of the layout at the given row and column offset.
func overlayDropdown(layout, dropdown string, colOffset, rowOffset, totalWidth int) string {
	_ = totalWidth // reserved for future right-edge clipping; kept to avoid touching call sites.
	layoutLines := strings.Split(layout, "\n")
	dropdownLines := strings.Split(dropdown, "\n")

	for i, dLine := range dropdownLines {
		targetRow := rowOffset + i
		if targetRow >= len(layoutLines) {
			break
		}
		layoutLines[targetRow] = spliceLine(layoutLines[targetRow], colOffset, dLine)
	}

	return strings.Join(layoutLines, "\n")
}

// stripAnsi removes ANSI escape codes from a string for width calculation.
func stripAnsi(s string) string {
	var result []rune
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
			continue
		}
		result = append(result, r)
	}
	return string(result)
}
