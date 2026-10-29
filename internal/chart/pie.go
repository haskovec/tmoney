package chart

import (
	"image"
	"image/color"
	"math"
)

// Slice is one slice of a pie.
type Slice struct {
	Value float64 // more than zero
	Color color.RGBA
}

// pieSamples is the samples per pixel on each axis: 4×4 samples give smooth
// edges.
const pieSamples = 4

// Pie draws the slices clockwise from 12 o'clock, in order, on a transparent
// canvas of w×h pixels. The pie is a circle in the middle of the canvas, as
// wide as the shorter side. A slice smaller than minDeg is drawn at minDeg;
// the other slices get smaller in proportion to pay for it. A transparent
// gap of max(2, size/200) px separates the slices (C13).
//
// The canvas is the cells the pie takes on the terminal, so the image is
// sharp at the terminal's own cell size.
func Pie(slices []Slice, w, h int, minDeg float64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	if len(slices) == 0 || w <= 0 || h <= 0 {
		return img
	}
	size := float64(min(w, h))
	gap := math.Max(2, size/200)
	ends := make([]float64, len(slices))
	var acc float64
	for i, a := range sweeps(slices, minDeg) {
		acc += a
		ends[i] = acc
	}
	cx, cy := float64(w)/2, float64(h)/2
	radius := size/2 - 1
	for y := range h {
		for x := range w {
			// A pixel with its middle more than one pixel outside the circle
			// has no sample in it.
			if math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) > radius+1 {
				continue
			}
			var rs, gs, bs, n float64
			for sy := range pieSamples {
				for sx := range pieSamples {
					dx := float64(x) + (float64(sx)+0.5)/pieSamples - cx
					dy := float64(y) + (float64(sy)+0.5)/pieSamples - cy
					r := math.Hypot(dx, dy)
					if r > radius {
						continue
					}
					i, ok := sliceAt(ends, r, math.Atan2(dx, -dy), gap)
					if !ok {
						continue
					}
					c := slices[i].Color
					rs += float64(c.R)
					gs += float64(c.G)
					bs += float64(c.B)
					n++
				}
			}
			if n == 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(rs / n), G: uint8(gs / n), B: uint8(bs / n),
				A: uint8(math.Round(n / (pieSamples * pieSamples) * 255)),
			})
		}
	}
	return img
}

// sliceAt is the slice under a point at distance r from the middle and at
// angle theta (radians, clockwise from 12 o'clock, from -π to π). It is
// false on a gap: nearer than gap/2 to the edge between two slices.
func sliceAt(ends []float64, r, theta, gap float64) (int, bool) {
	if theta < 0 {
		theta += 2 * math.Pi
	}
	i := 0
	for i < len(ends)-1 && theta > ends[i] {
		i++
	}
	if len(ends) == 1 {
		return 0, true
	}
	start := 0.0
	if i > 0 {
		start = ends[i-1]
	}
	d := math.Min(theta-start, ends[i]-theta)
	if d < math.Pi/2 && r*math.Sin(d) < gap/2 {
		return 0, false
	}
	return i, true
}

// sweeps returns the angle of each slice in radians. A slice smaller than
// minDeg is fixed at minDeg, the rest of the circle is shared among the other
// slices by value, and this repeats until no free slice is under the minimum.
func sweeps(slices []Slice, minDeg float64) []float64 {
	minRad := minDeg * math.Pi / 180
	out := make([]float64, len(slices))
	fixed := make([]bool, len(slices))
	for {
		var fixedSum, free float64
		for i, s := range slices {
			if fixed[i] {
				fixedSum += minRad
			} else {
				free += s.Value
			}
		}
		changed := false
		for i, s := range slices {
			if fixed[i] {
				out[i] = minRad
				continue
			}
			out[i] = s.Value / free * (2*math.Pi - fixedSum)
			if out[i] < minRad {
				fixed[i] = true
				changed = true
			}
		}
		if !changed {
			return out
		}
	}
}
