package widget

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// PadRight pads a string with spaces to the given width.
func PadRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// Truncate cuts s to at most maxLen cells, ending it with "..." when it is
// cut. A cut never splits a character, so the result is always valid UTF-8.
func Truncate(s string, maxLen int) string {
	if ansi.StringWidth(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return ansi.Truncate(s, maxLen, "")
	}
	return ansi.Truncate(s, maxLen, "...")
}

// DropLastGrapheme returns s without its last character as the user sees it
// (a grapheme cluster), so "e" + U+0301, "👍🏽", a ZWJ family or a flag goes as
// one. Backspace in a search calls it: one press removes what one key added
// and never leaves half a character or invalid UTF-8. An empty s stays empty.
func DropLastGrapheme(s string) string {
	last := 0
	for i := 0; i < len(s); {
		last = i
		g, _ := ansi.FirstGraphemeCluster(s[i:], ansi.GraphemeWidth)
		i += max(len(g), 1) // a cluster is never empty; never loop forever
	}
	return s[:last]
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

// FitParts joins parts with sep in their order, leaving out parts until the
// line fits in width cells. drop names the parts, by index, in the order they
// go; a part not in drop always stays, and an index out of range is skipped. A part is never split, so a figure is
// whole or absent. If the parts that must stay still do not fit, the line is
// cut with "…".
func FitParts(parts []string, sep string, width int, drop []int) string {
	keep := make([]bool, len(parts))
	for i := range keep {
		keep[i] = true
	}
	join := func() string {
		var kept []string
		for i, p := range parts {
			if keep[i] {
				kept = append(kept, p)
			}
		}
		return strings.Join(kept, sep)
	}
	line := join()
	for _, i := range drop {
		if lipgloss.Width(line) <= width {
			return line
		}
		if i < 0 || i >= len(parts) {
			continue
		}
		keep[i] = false
		line = join()
	}
	if lipgloss.Width(line) <= width {
		return line
	}
	return ansi.Truncate(line, width, "…")
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
