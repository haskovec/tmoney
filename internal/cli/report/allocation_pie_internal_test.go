package report

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/termimg"
	"github.com/haskovec/tmoney/internal/types"
)

// With a pie, a section prints the transmit, then 16 lines of placeholder
// cells with the legend on the right, then the notes. The swatch is the only
// text in a class color, and there are no bars.
func TestPrintAllocationReport_Pie(t *testing.T) {
	var out bytes.Buffer
	printAllocationReport(&out, specAllocation(), &cliPie{cols: 32, w: 64, h: 64, dark: true})
	s := out.String()

	if n := strings.Count(s, "\x1b_Ga=p,U=1,"); n != 1 {
		t.Errorf("got %d placements, want 1", n)
	}
	if strings.Index(s, "\x1b_G") > strings.IndexRune(s, kitty.Placeholder) {
		t.Error("a placeholder comes before the transmit")
	}
	var pie []string
	for line := range strings.Lines(s) {
		if strings.ContainsRune(line, kitty.Placeholder) {
			pie = append(pie, ansi.Strip(line))
		}
	}
	if len(pie) != allocationPieRows {
		t.Fatalf("got %d placeholder lines, want %d:\n%s", len(pie), allocationPieRows, s)
	}
	legend := []string{
		"  Asset Class         Holdings         Value % Total",
		strings.Repeat("─", allocationLegendWidth),
		"■ Large Cap Stock            2    $110175.75   33.6%",
		"■ Asset Mixture              1     $58226.45   17.7%",
	}
	for y, want := range legend {
		if ansi.StringWidth(want) != allocationLegendWidth {
			t.Errorf("legend line %d is %d cells, want %d", y, ansi.StringWidth(want), allocationLegendWidth)
		}
		if !strings.HasSuffix(strings.TrimRight(pie[y], "\n"), "   "+want) {
			t.Errorf("line %d = %q, want it to end with %q", y, pie[y], want)
		}
	}
	if want := "  TOTAL (USD)               12    $328270.17  100.0%"; !strings.HasSuffix(strings.TrimRight(pie[13], "\n"), want) {
		t.Errorf("line 13 = %q, want the TOTAL %q", pie[13], want)
	}
	if !strings.Contains(s, "\x1b[38;2;54;100;178m■\x1b[39m Large Cap Stock") {
		t.Errorf("the Large Cap swatch is not in its dark color:\n%q", s)
	}
	for _, want := range []string{
		"Not included (no price): STRK (Large Cap Stock). Add a price to include it.",
		"1 holding has no asset class.",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks the note %q", want)
		}
	}
	if strings.Contains(s, "█") {
		t.Error("the pie output has a bar")
	}
}

// A section a pie cannot show prints its table with neutral bars and says
// why there is no pie.
func TestPrintAllocationReport_NoPie(t *testing.T) {
	rpt := specAllocation()
	sec := &rpt.Sections[0]
	for i := range sec.Classes {
		if sec.Classes[i].Class == security.AssetClassCash {
			sec.Classes[i].Value = types.MustNewMoney("-500.00")
		}
	}
	var out bytes.Buffer
	printAllocationReport(&out, rpt, &cliPie{cols: 32, w: 64, h: 64, dark: true})
	s := out.String()
	if strings.Contains(s, "\x1b") {
		t.Errorf("output has an escape code: %q", s)
	}
	if !strings.Contains(s, "█") || !strings.Contains(s, "\nNo pie: Cash is negative.\n") {
		t.Errorf("want the table with bars and the no-pie note:\n%s", s)
	}
}

// Output that is not a terminal gets no query and no pie, also with the
// setting on.
func TestProbeCLIPie_NotATerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	for _, mode := range []termimg.Mode{termimg.ModeAuto, termimg.ModeOn, termimg.ModeOff} {
		if pie := probeCLIPie(&bytes.Buffer{}, mode); pie != nil {
			t.Errorf("%s, a buffer: pie = %+v, want nil", mode, pie)
		}
		if pie := probeCLIPie(w, mode); pie != nil {
			t.Errorf("%s, a pipe: pie = %+v, want nil", mode, pie)
		}
	}
}
