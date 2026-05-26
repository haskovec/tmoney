package widget

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// PadRight pads a string with spaces to the given width.
func PadRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// Truncate truncates a string to maxLen characters, adding "..." if needed.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// TruncateRunes truncates a string to maxLen runes, adding "..." if needed.
func TruncateRunes(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

// OverlayDropdown places a dropdown string on top of the layout at the given row and column offset.
func OverlayDropdown(layout, dropdown string, colOffset, rowOffset, totalWidth int) string {
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

// StripAnsi removes ANSI escape codes from a string for width calculation.
func StripAnsi(s string) string {
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
