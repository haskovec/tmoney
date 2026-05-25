package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// OverlayCenter places the overlay string centered on top of the background string.
func OverlayCenter(background, overlay string, screenWidth, screenHeight int) string {
	bgLines := strings.Split(background, "\n")
	ovLines := strings.Split(overlay, "\n")

	overlayWidth := 0
	for _, line := range ovLines {
		w := lipgloss.Width(line)
		if w > overlayWidth {
			overlayWidth = w
		}
	}
	overlayHeight := len(ovLines)

	startCol := max((screenWidth-overlayWidth)/2, 0)
	startRow := max((screenHeight-overlayHeight)/2, 0)

	for i, ovLine := range ovLines {
		targetRow := startRow + i
		if targetRow >= len(bgLines) {
			break
		}
		bgLines[targetRow] = spliceLine(bgLines[targetRow], startCol, ovLine)
	}

	return strings.Join(bgLines, "\n")
}

// spliceLine overlays ovLine onto bgLine at visible column startCol,
// preserving ANSI escape sequences (notably background colors) in the
// prefix and suffix bands. ansi.Cut handles the SGR-aware slicing so a
// blue desktop fill behind the dialog stays blue on either side of the
// overlay rather than collapsing to terminal default.
//
// If startCol is past bgLine's visible end, the gap is padded with
// plain spaces — those cells weren't painted by the background to begin
// with, so we have nothing to extend.
func spliceLine(bgLine string, startCol int, ovLine string) string {
	bgWidth := lipgloss.Width(bgLine)
	ovWidth := lipgloss.Width(ovLine)
	endCol := startCol + ovWidth

	var prefix string
	switch {
	case startCol <= 0:
		prefix = ""
	case startCol >= bgWidth:
		prefix = bgLine + strings.Repeat(" ", startCol-bgWidth)
	default:
		prefix = ansi.Cut(bgLine, 0, startCol)
	}

	suffix := ""
	if endCol < bgWidth {
		suffix = ansi.Cut(bgLine, endCol, bgWidth)
	}

	return prefix + ovLine + suffix
}
