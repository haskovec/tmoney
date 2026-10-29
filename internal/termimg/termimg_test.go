package termimg

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi/kitty"
)

func TestShow(t *testing.T) {
	for _, mode := range []Mode{ModeAuto, ModeOn, ModeOff} {
		for _, kittyOK := range []bool{false, true} {
			for _, inTmux := range []bool{false, true} {
				for _, fits := range []bool{false, true} {
					var want bool
					switch mode {
					case ModeAuto:
						want = kittyOK && !inTmux && fits
					case ModeOn:
						want = fits
					}
					if got := Show(mode, kittyOK, inTmux, fits); got != want {
						t.Errorf("Show(%s, kittyOK=%v, inTmux=%v, fits=%v) = %v, want %v",
							mode, kittyOK, inTmux, fits, got, want)
					}
				}
			}
		}
	}
}

func TestMode(t *testing.T) {
	for _, tc := range []struct {
		in    string
		want  Mode
		label string
		next  Mode
	}{
		{"", ModeAuto, "Auto", ModeOn},
		{"auto", ModeAuto, "Auto", ModeOn},
		{"on", ModeOn, "On", ModeOff},
		{"off", ModeOff, "Off", ModeAuto},
		{"sometimes", ModeAuto, "Auto", ModeOn},
	} {
		m := ParseMode(tc.in)
		if m != tc.want || m.Label() != tc.label || m.Next() != tc.next {
			t.Errorf("ParseMode(%q) = %s (%s, next %s), want %s (%s, next %s)",
				tc.in, m, m.Label(), m.Next(), tc.want, tc.label, tc.next)
		}
		if tc.in == m.String() && ParseMode(m.String()) != m {
			t.Errorf("%s does not read back", m)
		}
	}
}

// The cell size reply must come before the kitty "OK", so the TUI sends its
// query first.
func TestQuery(t *testing.T) {
	if !strings.HasPrefix(Query, "\x1b[16t") || !strings.HasSuffix(Query, kittyQuery) {
		t.Errorf("Query = %q, want the cell size query, then the kitty query", Query)
	}
}

// graphics are the kitty graphics commands in s: the control keys and the
// payload of each.
func graphics(t *testing.T, s string) (keys, payloads []string) {
	t.Helper()
	for {
		i := strings.Index(s, "\x1b_G")
		if i < 0 {
			return keys, payloads
		}
		end := strings.Index(s[i:], "\x1b\\")
		if end < 0 {
			t.Fatalf("a graphics command has no end: %q", s[i:])
		}
		k, p, _ := strings.Cut(s[i+3:i+end], ";")
		keys, payloads = append(keys, k), append(payloads, p)
		s = s[i+end+2:]
	}
}

// The transmit has the image as a PNG payload: x/ansi v0.11.9 writes an
// empty payload when Transmission is zero (charmbracelet/x#1016). It ends
// with a virtual placement of cols×rows cells.
func TestTransmit(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 120, 80))
	img.SetNRGBA(5, 5, color.NRGBA{R: 0xff, A: 0xff})
	id := ID{Lo: 200, Hi: 17}
	seq, err := Transmit(img, id, 12, 4)
	if err != nil {
		t.Fatal(err)
	}
	keys, payloads := graphics(t, seq)
	if len(keys) < 2 {
		t.Fatalf("got %d commands, want a transmit and a placement", len(keys))
	}
	for _, k := range []string{"f=100", fmt.Sprintf("i=%d", id.Value()), "q=2"} {
		if !hasKey(keys[0], strings.Split(k, "=")[0], strings.Split(k, "=")[1]) {
			t.Errorf("first command %q has no %s", keys[0], k)
		}
	}
	// a=t (transmit) and t=d (direct) are the defaults, so the encoder can
	// leave them out; any other value is wrong.
	for kv := range strings.SplitSeq(keys[0], ",") {
		if kv == "a" || strings.HasPrefix(kv, "a=") && kv != "a=t" || strings.HasPrefix(kv, "t=") && kv != "t=d" {
			t.Errorf("first command %q has %s", keys[0], kv)
		}
	}
	var data strings.Builder
	for _, p := range payloads[:len(payloads)-1] {
		data.WriteString(p)
	}
	if data.Len() == 0 {
		t.Fatal("the transmit has an empty payload")
	}
	raw, err := base64.StdEncoding.DecodeString(data.String())
	if err != nil {
		t.Fatalf("payload is not base64: %v", err)
	}
	got, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("payload is not a PNG: %v", err)
	}
	if got.Bounds() != img.Bounds() {
		t.Errorf("PNG bounds = %v, want %v", got.Bounds(), img.Bounds())
	}
	want := fmt.Sprintf("a=p,U=1,i=%d,c=12,r=4,q=2", id.Value())
	if last := keys[len(keys)-1]; last != want {
		t.Errorf("placement = %q, want %q", last, want)
	}
}

func TestID(t *testing.T) {
	if got := (ID{Lo: 0x01, Hi: 0xff}).Value(); got != 0xff000001 {
		t.Errorf("Value = %#x, want 0xff000001", got)
	}
	for range 200 {
		id := NewID()
		if id.Lo == 0 || id.Hi == 0 {
			t.Fatalf("NewID = %+v, want both bytes from 1 to 255", id)
		}
	}
	if got, want := Delete(ID{Lo: 2, Hi: 1}), fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", 1<<24|2); got != want {
		t.Errorf("Delete = %q, want %q", got, want)
	}
}

// A placeholder line has one cell for each column: U+10EEEE and the
// diacritics of its row, its column and the ID's high byte, in the 256-color
// foreground of the low byte.
func TestPlaceholderLine(t *testing.T) {
	id := ID{Lo: 201, Hi: 18}
	line := PlaceholderLine(id, 3, 5)
	body, ok := strings.CutPrefix(line, "\x1b[38;5;201m")
	if !ok {
		t.Fatalf("line = %q, want the 38;5;201 foreground first", line)
	}
	body, ok = strings.CutSuffix(body, "\x1b[39m")
	if !ok {
		t.Fatalf("line = %q, want the default foreground last", line)
	}
	cells := []rune(body)
	if len(cells) != 5*4 {
		t.Fatalf("got %d runes, want 5 cells of 4", len(cells))
	}
	for x := range 5 {
		got := cells[x*4 : x*4+4]
		want := []rune{kitty.Placeholder, kitty.Diacritic(3), kitty.Diacritic(x), kitty.Diacritic(18)}
		if string(got) != string(want) {
			t.Errorf("cell %d = %U, want %U", x, got, want)
		}
	}
}

func TestParseReply(t *testing.T) {
	const (
		ok      = "\x1b_Gi=31;OK\x1b\\"
		failed  = "\x1b_Gi=31;ENOTSUPPORTED:no\x1b\\"
		da1     = "\x1b[?62;22c"
		cell    = "\x1b[6;50;23t"
		bg4     = "\x1b]11;rgb:1a1a/1a1a/1919\x1b\\"
		bg2Bell = "\x1b]11;rgb:fc/fc/fb\a"
	)
	dark := color.RGBA{R: 0x1a, G: 0x1a, B: 0x19, A: 0xff}
	light := color.RGBA{R: 0xfc, G: 0xfc, B: 0xfb, A: 0xff}
	for _, tc := range []struct {
		name string
		in   string
		want Reply
		done bool
	}{
		{"OK before DA1", ok + bg4 + cell + da1,
			Reply{KittyOK: true, CellW: 23, CellH: 50, Background: dark, HasBackground: true}, true},
		{"an error before DA1", failed + da1, Reply{}, true},
		{"DA1 only", da1, Reply{}, true},
		{"no reply in time", "", Reply{}, false},
		{"a reply cut short", ok + "\x1b[?62;", Reply{KittyOK: true}, false},
		{"OSC 11 with 2 digits and BEL", bg2Bell + da1, Reply{Background: light, HasBackground: true}, true},
		{"OK after DA1", da1 + ok, Reply{}, true},
		{"another image ID", "\x1b_Gi=7;OK\x1b\\" + da1, Reply{}, true},
	} {
		got, done := parseReply([]byte(tc.in))
		if got != tc.want || done != tc.done {
			t.Errorf("%s: parseReply = %+v, %v; want %+v, %v", tc.name, got, done, tc.want, tc.done)
		}
	}
}

func TestReplyDark(t *testing.T) {
	for _, tc := range []struct {
		r    Reply
		want bool
	}{
		{Reply{}, true},
		{Reply{Background: color.RGBA{R: 0x1a, G: 0x1a, B: 0x19}, HasBackground: true}, true},
		{Reply{Background: color.RGBA{R: 0xfc, G: 0xfc, B: 0xfb}, HasBackground: true}, false},
	} {
		if got := tc.r.Dark(); got != tc.want {
			t.Errorf("%+v Dark = %v, want %v", tc.r, got, tc.want)
		}
	}
}

func TestPlacement(t *testing.T) {
	for _, tc := range []struct {
		rows, cellW, cellH int
		cols, w, h         int
	}{
		{20, 23, 50, 43, 989, 1000},
		{16, 10, 20, 32, 320, 320},
		{8, 9, 20, 18, 162, 160},
		{16, 0, 0, 32, 600, 600},
	} {
		cols, w, h := Placement(tc.rows, tc.cellW, tc.cellH)
		if cols != tc.cols || w != tc.w || h != tc.h {
			t.Errorf("Placement(%d, %d, %d) = %d, %d, %d; want %d, %d, %d",
				tc.rows, tc.cellW, tc.cellH, cols, w, h, tc.cols, tc.w, tc.h)
		}
	}
}
