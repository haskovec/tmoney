// Package chart holds what the asset allocation report draws with: the fixed
// color of each asset class, and the pie. It has no terminal and no TUI
// state, so the TUI and the CLI share it. See
// specs/design-asset-allocation-report.md §5.4 and §6.2.
package chart

import (
	"image/color"

	"github.com/haskovec/tmoney/internal/security"
)

// classColors is the light and the dark color of each asset class. Stocks are
// blue and bonds green; Unclassified is gray. Each set was checked with the
// palette validator of the dataviz method, against #fcfcfb and #1a1a19; the
// spec gives the results.
var classColors = map[security.AssetClass][2]color.RGBA{
	security.AssetClassLargeCapStock:      {rgb(0x184f95), rgb(0x3664b2)},
	security.AssetClassMidCapStock:        {rgb(0x307cd7), rgb(0x0587ff)},
	security.AssetClassSmallCapStock:      {rgb(0x71abee), rgb(0x75adf0)},
	security.AssetClassInternationalStock: {rgb(0x653bd8), rgb(0x9677cd)},
	security.AssetClassDomesticBond:       {rgb(0x248702), rgb(0x299244)},
	security.AssetClassForeignBond:        {rgb(0x94c521), rgb(0x8dae00)},
	security.AssetClassCash:               {rgb(0xc88f00), rgb(0x8b5c00)},
	security.AssetClassRealEstate:         {rgb(0x8f3a0e), rgb(0xdf9575)},
	security.AssetClassCommodity:          {rgb(0xe24848), rgb(0xeb515c)},
	security.AssetClassCrypto:             {rgb(0xb257a9), rgb(0xb63d85)},
	security.AssetClassAssetMixture:       {rgb(0x1fba95), rgb(0x37baa5)},
	security.AssetClassUnclassified:       {rgb(0x898781), rgb(0x898781)},
}

// ClassColor is the fixed color of an asset class on a light or a dark
// background. A class it does not know gets the Unclassified gray.
func ClassColor(ac security.AssetClass, dark bool) color.RGBA {
	c, ok := classColors[ac]
	if !ok {
		c = classColors[security.AssetClassUnclassified]
	}
	if dark {
		return c[1]
	}
	return c[0]
}

func rgb(v uint32) color.RGBA {
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}
