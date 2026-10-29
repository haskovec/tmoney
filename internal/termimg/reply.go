package termimg

import (
	"bytes"
	"image/color"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// probeQuery is what the CLI sends: the kitty query, the background color
// (OSC 11), the cell size, and DA1. Every terminal answers DA1, and it
// answers in order, so the DA1 reply ends the replies.
const probeQuery = kittyQuery + "\x1b]11;?\x1b\\" + cellSizeQuery + "\x1b[c"

// Reply is what the terminal answered to the CLI's query.
type Reply struct {
	// KittyOK is a kitty "OK" before the DA1 reply.
	KittyOK bool
	// CellW and CellH are the cell size in pixels, or 0 with no reply.
	CellW, CellH int
	// Background is the background color; HasBackground is false with no
	// reply.
	Background    color.RGBA
	HasBackground bool
}

// Dark reports whether the background is dark. With no reply it is dark, as
// in the TUI (C3).
func (r Reply) Dark() bool {
	if !r.HasBackground {
		return true
	}
	c := r.Background
	// The HSL lightness, as Bubble Tea decides it.
	return int(max(c.R, c.G, c.B))+int(min(c.R, c.G, c.B)) < 255
}

// parseReply reads the replies in b, in order. done is true when b has the
// DA1 reply; a kitty reply after it is not counted.
func parseReply(b []byte) (r Reply, done bool) {
	for len(b) > 0 {
		i := bytes.IndexByte(b, ansi.ESC)
		if i < 0 || i+1 >= len(b) {
			return r, false
		}
		b = b[i+1:]
		switch b[0] {
		case '_': // APC: the kitty reply
			body, rest, ok := untilST(b[1:])
			if !ok {
				return r, false
			}
			b = rest
			if keys, msg, found := strings.Cut(body, ";"); found && strings.HasPrefix(keys, "G") &&
				hasKey(keys[1:], "i", strconv.Itoa(QueryID)) && msg == "OK" {
				r.KittyOK = true
			}
		case ']': // OSC: the background color
			body, rest, ok := untilST(b[1:])
			if !ok {
				return r, false
			}
			b = rest
			if spec, found := strings.CutPrefix(body, "11;"); found {
				if c := ansi.XParseColor(spec); c != nil {
					cr, cg, cb, _ := c.RGBA()
					r.Background = color.RGBA{R: uint8(cr >> 8), G: uint8(cg >> 8), B: uint8(cb >> 8), A: 0xff}
					r.HasBackground = true
				}
			}
		case '[': // CSI: the cell size or DA1
			end := bytes.IndexFunc(b[1:], func(c rune) bool { return c >= 0x40 && c <= 0x7e })
			if end < 0 {
				return r, false
			}
			params, final := string(b[1:1+end]), b[1+end]
			b = b[2+end:]
			switch {
			case final == 'c' && strings.HasPrefix(params, "?"):
				return r, true
			case final == 't':
				if f := strings.Split(params, ";"); len(f) == 3 && f[0] == "6" {
					h, errH := strconv.Atoi(f[1])
					w, errW := strconv.Atoi(f[2])
					if errH == nil && errW == nil && h > 0 && w > 0 {
						r.CellW, r.CellH = w, h
					}
				}
			}
		}
	}
	return r, false
}

// untilST splits b at the end of a string sequence: ST (ESC \) or BEL. ok is
// false when the end has not come yet.
func untilST(b []byte) (body string, rest []byte, ok bool) {
	for i, c := range b {
		switch {
		case c == ansi.BEL:
			return string(b[:i]), b[i+1:], true
		case c == ansi.ESC && i+1 < len(b) && b[i+1] == '\\':
			return string(b[:i]), b[i+2:], true
		}
	}
	return "", b, false
}

// hasKey reports whether the comma-separated key=value list has key=value.
func hasKey(list, key, value string) bool {
	for kv := range strings.SplitSeq(list, ",") {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key && v == value {
			return true
		}
	}
	return false
}
