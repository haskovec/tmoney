package widget

import (
	"strings"
	"testing"
)

// OverlayCenter and spliceLine.

func TestOverlayCenter_CentersOverlay(t *testing.T) {
	// Create a 10x5 background of dots
	bgLines := make([]string, 5)
	for i := range bgLines {
		bgLines[i] = strings.Repeat(".", 10)
	}
	background := strings.Join(bgLines, "\n")

	overlay := "XX\nXX"

	result := OverlayCenter(background, overlay, 10, 5)
	lines := strings.Split(result, "\n")

	// Overlay is 2x2, screen is 10x5
	// startCol = (10-2)/2 = 4
	// startRow = (5-2)/2 = 1
	if len(lines) < 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}

	// Row 1 should have XX at position 4-5
	if !strings.Contains(lines[1], "XX") {
		t.Errorf("row 1 should contain 'XX', got %q", lines[1])
	}
	if !strings.Contains(lines[2], "XX") {
		t.Errorf("row 2 should contain 'XX', got %q", lines[2])
	}

	// Row 0 should be unchanged
	if lines[0] != ".........." {
		t.Errorf("row 0 should be unchanged, got %q", lines[0])
	}
}

func TestOverlayCenter_SmallBackground(t *testing.T) {
	background := ".\n."
	overlay := "XXXXX\nXXXXX\nXXXXX"

	// Should not panic even if overlay is larger
	result := OverlayCenter(background, overlay, 2, 2)
	if result == "" {
		t.Error("should produce non-empty output")
	}
}

func TestOverlayCenter_EmptyOverlay(t *testing.T) {
	background := "hello\nworld"
	result := OverlayCenter(background, "", 5, 2)
	if result != background {
		t.Error("empty overlay should not change background")
	}
}

// TestOverlayCenter_PreservesBackgroundANSI guards the Turbo Vision
// "blue desktop" use case: when the background lines carry an SGR
// background color (here \x1b[44m for blue), the prefix and suffix
// bands on either side of a centered overlay must still carry the
// background ANSI. The previous implementation stripped ANSI from
// bgLine before slicing prefix/suffix, which collapsed the bands to
// terminal default and produced a black band around dialogs on
// colored desktops.

func TestOverlayCenter_PreservesBackgroundANSI(t *testing.T) {
	const blueBg = "\x1b[44m"
	const reset = "\x1b[0m"
	bgRow := blueBg + strings.Repeat(" ", 20) + reset
	background := strings.Join([]string{bgRow, bgRow, bgRow, bgRow, bgRow}, "\n")

	overlay := "DIALOG" // visible width 6

	result := OverlayCenter(background, overlay, 20, 5)
	lines := strings.Split(result, "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}

	// Overlay sits on the middle row: startRow=(5-1)/2=2.
	middle := lines[2]
	if !strings.Contains(middle, "DIALOG") {
		t.Fatalf("middle row should contain overlay, got %q", middle)
	}
	if !strings.Contains(middle, blueBg) {
		t.Errorf("middle row should preserve blue-bg ANSI on prefix/suffix, got %q", middle)
	}

	// Sanity: untouched rows still carry the blue ANSI.
	for _, idx := range []int{0, 1, 3, 4} {
		if !strings.Contains(lines[idx], blueBg) {
			t.Errorf("row %d should still carry blue-bg ANSI, got %q", idx, lines[idx])
		}
	}
}

// Unicode text editing tests
