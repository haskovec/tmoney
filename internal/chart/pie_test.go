package chart

import (
	"image/color"
	"math"
	"testing"
)

var (
	red   = color.RGBA{R: 0xe0, G: 0x20, B: 0x20, A: 0xff}
	green = color.RGBA{R: 0x20, G: 0xb0, B: 0x20, A: 0xff}
	blue  = color.RGBA{R: 0x20, G: 0x50, B: 0xe0, A: 0xff}
)

// at is the pixel at angle deg (clockwise from 12 o'clock) and distance r
// from the middle of a size×size pie.
func at(size int, deg, r float64) (int, int) {
	rad := deg * math.Pi / 180
	c := float64(size) / 2
	return int(c + r*math.Sin(rad)), int(c - r*math.Cos(rad))
}

// The slices go clockwise from 12 o'clock in order, each in its own color:
// red is the right half, green the bottom-left quarter, blue the top-left.
// A pixel on a gap or outside the circle is transparent.
func TestPie(t *testing.T) {
	const size = 200
	img := Pie([]Slice{{50, red}, {25, green}, {25, blue}}, size, size, 2)
	for _, tc := range []struct {
		name string
		deg  float64
		want color.RGBA
	}{
		{"red, at 3 o'clock", 90, red},
		{"green, at 7:30", 225, green},
		{"blue, at 10:30", 315, blue},
	} {
		x, y := at(size, tc.deg, size/4)
		got := img.NRGBAAt(x, y)
		if got != (color.NRGBA{R: tc.want.R, G: tc.want.G, B: tc.want.B, A: 0xff}) {
			t.Errorf("%s: pixel (%d,%d) = %v, want %v", tc.name, x, y, got, tc.want)
		}
	}
	for _, tc := range []struct {
		name string
		deg  float64
	}{
		{"the gap at 12 o'clock", 0},
		{"the gap at 6 o'clock", 180},
		{"the gap at 9 o'clock", 270},
	} {
		x, y := at(size, tc.deg, size/4)
		if a := img.NRGBAAt(x, y).A; a != 0 {
			t.Errorf("%s: pixel (%d,%d) alpha = %d, want 0", tc.name, x, y, a)
		}
	}
	if a := img.NRGBAAt(2, 2).A; a != 0 {
		t.Errorf("a corner outside the circle: alpha = %d, want 0", a)
	}
}

// The pie is a circle in the middle of a canvas that is not square, as wide
// as the shorter side.
func TestPie_Canvas(t *testing.T) {
	img := Pie([]Slice{{1, red}}, 300, 200, 2)
	if b := img.Bounds(); b.Dx() != 300 || b.Dy() != 200 {
		t.Fatalf("bounds = %v, want 300x200", b)
	}
	if got := img.NRGBAAt(150, 100); got.A != 0xff || got.R != red.R {
		t.Errorf("middle = %v, want red", got)
	}
	// One slice has no gap.
	if got := img.NRGBAAt(150, 20); got.A != 0xff {
		t.Errorf("12 o'clock of one slice = %v, want opaque", got)
	}
	for _, x := range []int{30, 270} {
		if a := img.NRGBAAt(x, 100).A; a != 0 {
			t.Errorf("(%d,100) is outside the circle: alpha = %d", x, a)
		}
	}
}

func TestPie_Empty(t *testing.T) {
	img := Pie(nil, 10, 10, 2)
	for y := range 10 {
		for x := range 10 {
			if a := img.NRGBAAt(x, y).A; a != 0 {
				t.Fatalf("pixel (%d,%d) alpha = %d, want 0", x, y, a)
			}
		}
	}
}

// A slice under the minimum angle is drawn at the minimum, the angles add up
// to the full circle, and the other slices pay in proportion to their value.
func TestSweeps(t *testing.T) {
	deg := func(rad float64) float64 { return rad * 180 / math.Pi }
	got := sweeps([]Slice{{Value: 600}, {Value: 399}, {Value: 1}}, 2)
	if d := deg(got[2]); math.Abs(d-2) > 1e-9 {
		t.Errorf("0.1%% slice = %.4f°, want 2°", d)
	}
	var sum float64
	for _, a := range got {
		sum += deg(a)
	}
	if math.Abs(sum-360) > 1e-9 {
		t.Errorf("sum = %.6f°, want 360°", sum)
	}
	// 358° is shared 600:399.
	if want := 358 * 600.0 / 999; math.Abs(deg(got[0])-want) > 1e-9 {
		t.Errorf("largest = %.4f°, want %.4f°", deg(got[0]), want)
	}
	if want := 358 * 399.0 / 999; math.Abs(deg(got[1])-want) > 1e-9 {
		t.Errorf("second = %.4f°, want %.4f°", deg(got[1]), want)
	}

	// A slice that fixing another one pushes under the minimum is fixed too.
	got = sweeps([]Slice{{Value: 1000}, {Value: 5.6}, {Value: 0.1}}, 2)
	if d := deg(got[1]); math.Abs(d-2) > 1e-9 {
		t.Errorf("second slice = %.4f°, want 2°", d)
	}
	if d := deg(got[0]); math.Abs(d-356) > 1e-9 {
		t.Errorf("largest = %.4f°, want 356°", d)
	}

	// No slice under the minimum: the angles are in proportion.
	got = sweeps([]Slice{{Value: 3}, {Value: 1}}, 2)
	if math.Abs(deg(got[0])-270) > 1e-9 || math.Abs(deg(got[1])-90) > 1e-9 {
		t.Errorf("3:1 = %.4f°, %.4f°, want 270°, 90°", deg(got[0]), deg(got[1]))
	}
}
