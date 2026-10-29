package chart

import (
	"fmt"
	"testing"

	"github.com/haskovec/tmoney/internal/security"
)

// Every class has the light and the dark color of the spec's table (§5.4).
func TestClassColor(t *testing.T) {
	want := map[security.AssetClass][2]string{
		security.AssetClassLargeCapStock:      {"#184f95", "#3664b2"},
		security.AssetClassMidCapStock:        {"#307cd7", "#0587ff"},
		security.AssetClassSmallCapStock:      {"#71abee", "#75adf0"},
		security.AssetClassInternationalStock: {"#653bd8", "#9677cd"},
		security.AssetClassDomesticBond:       {"#248702", "#299244"},
		security.AssetClassForeignBond:        {"#94c521", "#8dae00"},
		security.AssetClassCash:               {"#c88f00", "#8b5c00"},
		security.AssetClassRealEstate:         {"#8f3a0e", "#df9575"},
		security.AssetClassCommodity:          {"#e24848", "#eb515c"},
		security.AssetClassCrypto:             {"#b257a9", "#b63d85"},
		security.AssetClassAssetMixture:       {"#1fba95", "#37baa5"},
		security.AssetClassUnclassified:       {"#898781", "#898781"},
	}
	hex := func(ac security.AssetClass, dark bool) string {
		c := ClassColor(ac, dark)
		if c.A != 0xff {
			t.Errorf("%s dark=%v: alpha %d, want opaque", ac, dark, c.A)
		}
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	for _, ac := range security.AllAssetClasses() {
		w, ok := want[ac]
		if !ok {
			t.Errorf("%s: no color in the spec table", ac)
			continue
		}
		if got := hex(ac, false); got != w[0] {
			t.Errorf("%s light = %s, want %s", ac, got, w[0])
		}
		if got := hex(ac, true); got != w[1] {
			t.Errorf("%s dark = %s, want %s", ac, got, w[1])
		}
	}
	if len(want) != len(security.AllAssetClasses()) {
		t.Errorf("the table has %d classes, AllAssetClasses has %d", len(want), len(security.AllAssetClasses()))
	}
}
