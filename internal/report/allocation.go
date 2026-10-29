package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/haskovec/tmoney/internal/chart"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// Allocation returns the asset allocation report as of today: the rows of
// the holdings report, grouped by the asset class of each security. See
// specs/design-asset-allocation-report.md §5.2.
//
// It is built from Holdings, so the two reports value the same accounts the
// same way. The one difference is a holding with no price: the holdings
// report values it at cost, and this report leaves it out of every value and
// percentage and lists it in its class's NoPrice.
func (s *Service) Allocation() (*Allocation, error) {
	h, err := s.Holdings()
	if err != nil {
		return nil, err
	}
	rpt := &Allocation{AsOfDate: h.AsOfDate, AccountCount: h.AccountCount, Failed: h.Failed}
	for _, hs := range h.Sections {
		sec := allocationSection(hs)
		// A section that is not Available stays, to show that its total is
		// not available.
		if len(sec.Classes) == 0 && sec.Available {
			continue
		}
		rpt.Sections = append(rpt.Sections, sec)
	}
	return rpt, nil
}

// classShare is the part of a holding's value that is in one asset class.
type classShare struct {
	class security.AssetClass
	value types.Money
}

// classShares divides a holding's value across the asset classes. Today a
// security is in one class with all of its value (D1). A later version that
// splits a fund across classes by weight changes only this function.
func classShares(row HoldingRow) []classShare {
	return []classShare{{class: row.AssetClass, value: row.Value}}
}

// allocationSection groups one currency's holdings by asset class.
func allocationSection(hs HoldingsSection) AllocationSection {
	sec := AllocationSection{Currency: hs.Currency, Value: types.ZeroMoney, Available: hs.Available}
	classes := make(map[security.AssetClass]*ClassRow)
	class := func(ac security.AssetClass) *ClassRow {
		c, ok := classes[ac]
		if !ok {
			c = &ClassRow{Class: ac, Value: types.ZeroMoney}
			classes[ac] = c
		}
		return c
	}
	for _, row := range hs.Rows {
		for _, share := range classShares(row) {
			c := class(share.class)
			if row.Estimated {
				c.NoPrice = append(c.NoPrice, row)
				continue
			}
			h := ClassHolding{HoldingRow: row}
			h.Value = share.value
			c.Holdings = append(c.Holdings, h)
			c.Value = c.Value.Add(share.value)
		}
	}

	for _, c := range classes {
		sec.Value = sec.Value.Add(c.Value)
	}
	for _, c := range classes {
		c.Percent = percentOf(c.Value, sec.Value)
		for i := range c.Holdings {
			h := &c.Holdings[i]
			h.PercentOfClass = percentOf(h.Value, c.Value)
			h.PercentOfTotal = percentOf(h.Value, sec.Value)
		}
		sort.SliceStable(c.Holdings, func(i, j int) bool {
			a, b := c.Holdings[i], c.Holdings[j]
			if cmp := a.Value.Cmp(b.Value); cmp != 0 {
				return cmp > 0
			}
			if a.Label != b.Label {
				return a.Label < b.Label
			}
			return a.SecurityID.String() < b.SecurityID.String()
		})
		sort.SliceStable(c.NoPrice, func(i, j int) bool { return c.NoPrice[i].Label < c.NoPrice[j].Label })
		sec.Classes = append(sec.Classes, *c)
	}
	sort.Slice(sec.Classes, func(i, j int) bool {
		a, b := sec.Classes[i], sec.Classes[j]
		if (a.Class == security.AssetClassUnclassified) != (b.Class == security.AssetClassUnclassified) {
			return b.Class == security.AssetClassUnclassified
		}
		if cmp := a.Value.Cmp(b.Value); cmp != 0 {
			return cmp > 0
		}
		return a.Class.DisplayName() < b.Class.DisplayName()
	})
	return sec
}

// Count is the number of priced securities in the class: the uninvested
// cash row is not a security.
func (c ClassRow) Count() int {
	n := 0
	for _, h := range c.Holdings {
		if !h.Cash {
			n++
		}
	}
	return n
}

// Count is the number of priced securities in the section.
func (s AllocationSection) Count() int {
	n := 0
	for _, c := range s.Classes {
		n += c.Count()
	}
	return n
}

// Class is the row of one asset class, if the section has it.
func (s AllocationSection) Class(ac security.AssetClass) (ClassRow, bool) {
	for _, c := range s.Classes {
		if c.Class == ac {
			return c, true
		}
	}
	return ClassRow{}, false
}

// NoPriceNames are the holdings of the section that have no price, each as
// "Label (Class)", in the order of the classes.
func (s AllocationSection) NoPriceNames() []string {
	var out []string
	for _, c := range s.Classes {
		for _, row := range c.NoPrice {
			out = append(out, fmt.Sprintf("%s (%s)", row.Label, c.Class.DisplayName()))
		}
	}
	return out
}

// NoPriceNote is the note that names the holdings with no price, or "" when
// there are none.
func NoPriceNote(names []string) string {
	if len(names) == 0 {
		return ""
	}
	them := "it"
	if len(names) > 1 {
		them = "them"
	}
	return fmt.Sprintf("Not included (no price): %s. Add a price to include %s.", strings.Join(names, ", "), them)
}

// UnclassifiedNote is the start of the note for n priced holdings with no
// asset class, or "" when n is 0. The TUI and the CLI each add how to set a
// class.
func UnclassifiedNote(n int) string {
	switch n {
	case 0:
		return ""
	case 1:
		return "1 holding has no asset class."
	}
	return fmt.Sprintf("%d holdings have no asset class.", n)
}

// Drawable reports whether a pie can show the section: its Value is more
// than zero and no class Value is negative (D11). Negative cash is the usual
// reason it cannot.
func (s AllocationSection) Drawable() bool {
	if !s.Value.IsPositive() {
		return false
	}
	for _, c := range s.Classes {
		if c.Value.IsNegative() {
			return false
		}
	}
	return true
}

// NoPieNote says why a section that is not Drawable has no pie.
func (s AllocationSection) NoPieNote() string {
	for _, c := range s.Classes {
		if c.Value.IsNegative() {
			return fmt.Sprintf("No pie: %s is negative.", c.Class.DisplayName())
		}
	}
	return "No pie: the total is zero."
}

// PieSlices are the slices of the section's pie: the shown classes in the
// order of the table, in their light or dark colors. A class with no value
// has a row but no slice (D5).
func (s AllocationSection) PieSlices(dark bool) []chart.Slice {
	var out []chart.Slice
	for _, c := range s.Shown() {
		if c.Value.IsPositive() {
			out = append(out, chart.Slice{Value: c.Value.Float64(), Color: chart.ClassColor(c.Class, dark)})
		}
	}
	return out
}

// Shown are the classes that have a row in the table and a slice in the pie:
// the ones with a priced holding. A class whose holdings all have no price
// is left out; the no-price note names its holdings.
func (s AllocationSection) Shown() []ClassRow {
	var out []ClassRow
	for _, c := range s.Classes {
		if len(c.Holdings) > 0 {
			out = append(out, c)
		}
	}
	return out
}
