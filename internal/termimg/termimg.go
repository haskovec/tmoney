// Package termimg is the kitty graphics code that the TUI and the CLI share:
// the decision rule, the image encoding, the Unicode placeholder lines, and
// the CLI's terminal query. See specs/design-asset-allocation-report.md §6.
package termimg

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"math/rand/v2"
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/charmbracelet/x/term"
)

// Mode is the chart_images setting.
type Mode int

const (
	// ModeAuto shows a chart image when the terminal says it can.
	ModeAuto Mode = iota
	// ModeOn shows a chart image when it fits, with no terminal check (C8).
	ModeOn
	// ModeOff never shows a chart image.
	ModeOff
)

// ParseMode reads the chart_images setting. Empty or unknown is ModeAuto, so
// old config files keep working.
func ParseMode(s string) Mode {
	switch s {
	case "on":
		return ModeOn
	case "off":
		return ModeOff
	}
	return ModeAuto
}

// String is the setting's value in config.json.
func (m Mode) String() string {
	switch m {
	case ModeOn:
		return "on"
	case ModeOff:
		return "off"
	}
	return "auto"
}

// Label is the setting's name in the View menu.
func (m Mode) Label() string {
	switch m {
	case ModeOn:
		return "On"
	case ModeOff:
		return "Off"
	}
	return "Auto"
}

// Next is the mode after m in the View menu: Auto, On, Off, Auto.
func (m Mode) Next() Mode {
	switch m {
	case ModeAuto:
		return ModeOn
	case ModeOn:
		return ModeOff
	}
	return ModeAuto
}

// Show reports whether to draw a chart image. kittyOK is a kitty "OK" reply
// to the query, inTmux is a TMUX environment variable, and fits is room on
// the screen. The caller checks the other conditions first: a color profile
// of 256 colors or more in the TUI, a terminal on stdout in the CLI.
func Show(mode Mode, kittyOK, inTmux, fits bool) bool {
	switch mode {
	case ModeOff:
		return false
	case ModeOn:
		return fits
	}
	return kittyOK && !inTmux && fits
}

// InTmux reports whether the program runs in tmux, which does not pass kitty
// images on.
func InTmux() bool { return os.Getenv("TMUX") != "" }

// TerminalWidth is the width of f in cells, when f is a terminal. A pipe, a
// file and an AI agent's output are not, and get text only.
func TerminalWidth(f *os.File) (int, bool) {
	if !term.IsTerminal(f.Fd()) {
		return 0, false
	}
	w, _, err := term.GetSize(f.Fd())
	if err != nil {
		return 0, false
	}
	return w, true
}

// QueryID is the image ID of the kitty query; a reply with this ID answers it.
const QueryID = 31

// kittyQuery asks whether the terminal takes kitty images: a 1×1 RGB image
// that the terminal checks and does not keep.
const kittyQuery = "\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\"

// cellSizeQuery asks for the cell size in pixels (CSI 16 t).
const cellSizeQuery = "\x1b[16t"

// Query is what the TUI sends at start. The cell size query is first, so
// that its reply comes before the kitty "OK": then the first pie already has
// the terminal's cell size, and is not sent two times.
const Query = cellSizeQuery + kittyQuery

// ID is a kitty image ID. Lo goes in the 256-color foreground of the
// placeholder cells and Hi in their third diacritic (C5). Both are from 1 to
// 255.
type ID struct{ Lo, Hi uint8 }

// NewID returns a random ID, so that each run has its own: a pie of an
// earlier CLI run in the scrollback does not change.
func NewID() ID {
	return ID{Lo: uint8(rand.IntN(255) + 1), Hi: uint8(rand.IntN(255) + 1)}
}

// Value is the ID in the kitty protocol: Hi<<24 | Lo.
func (id ID) Value() int { return int(id.Hi)<<24 | int(id.Lo) }

// Transmit is the escape sequence that sends img to the terminal with the ID,
// and makes a virtual placement of cols×rows cells for the placeholders. A
// transmit with an ID the terminal has replaces that image.
func Transmit(img image.Image, id ID, cols, rows int) (string, error) {
	var b bytes.Buffer
	if err := kitty.EncodeGraphics(&b, img, &kitty.Options{
		Action: kitty.Transmit,
		// When Transmission is zero, x/ansi v0.11.9 writes an empty
		// payload (charmbracelet/x#1016).
		Transmission: kitty.Direct,
		ID:           id.Value(),
		Format:       kitty.PNG,
		Quiet:        2,
		Chunk:        true,
	}); err != nil {
		return "", fmt.Errorf("encode the image: %w", err)
	}
	fmt.Fprintf(&b, "\x1b_Ga=p,U=1,i=%d,c=%d,r=%d,q=2\x1b\\", id.Value(), cols, rows)
	return b.String(), nil
}

// defaultPiePixels is the image size, in pixels, when the terminal did not
// give its cell size.
const defaultPiePixels = 600

// Placement is the columns and the image pixels of a placement of rows rows,
// for a terminal cell of cellW×cellH pixels: the columns make the image
// round, and the image fills the cells, so that it is sharp. With no cell
// size (0), a cell is 1:2 and the image is 600 px.
func Placement(rows, cellW, cellH int) (cols, w, h int) {
	if cellW <= 0 || cellH <= 0 {
		return 2 * rows, defaultPiePixels, defaultPiePixels
	}
	cols = int(math.Round(float64(rows*cellH) / float64(cellW)))
	return cols, cols * cellW, rows * cellH
}

// Delete is the escape sequence that deletes the image with the ID.
func Delete(id ID) string {
	return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id.Value())
}

// PlaceholderLine is row y of the image's placement, cols cells wide: each
// cell is U+10EEEE with the diacritics of its row, its column and the ID's
// high byte, in the 256-color foreground of the ID's low byte. A terminal
// counts each cell as one column.
func PlaceholderLine(id ID, y, cols int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "\x1b[38;5;%dm", id.Lo)
	row, hi := kitty.Diacritic(y), kitty.Diacritic(int(id.Hi))
	for x := range cols {
		sb.WriteRune(kitty.Placeholder)
		sb.WriteRune(row)
		sb.WriteRune(kitty.Diacritic(x))
		sb.WriteRune(hi)
	}
	sb.WriteString("\x1b[39m")
	return sb.String()
}
