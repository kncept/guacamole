// Command icongen draws the Guacamole app icon: an avocado-green letter G.
//
// The icon is written as a set of PNGs, one per size macOS expects inside an
// .iconset directory, ready to be converted into an .icns file with:
//
//	iconutil -c icns <iconset-dir> -o Guacamole.icns
//
// Usage: go run ./cmd/icongen <iconset-dir>
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// letterGreen is the avocado-green fill of the letter G.
var letterGreen = color.NRGBA{R: 0x7e, G: 0xa1, B: 0x22, A: 0xff}

// samples is the supersampling factor per axis, used to antialias the glyph.
const samples = 4

// Glyph geometry, expressed within a unit square. The letter G is a C-like shape
// (an open annulus) with a horizontal bar on the right side.
const (
	// Bowl (outer ring)
	bowlCX   = 0.5
	bowlCY   = 0.5
	bowlROut = 0.35
	bowlRIn  = 0.20

	// Gap in the C (where the G opens)
	// Angle from -pi/2 to pi/2 (right side open)
	gapStartAngle = math.Pi * 0.4  // ~72 degrees
	gapEndAngle   = -math.Pi * 0.4 // ~-72 degrees

	// Horizontal bar of the G
	barX0     = 0.5  // starts at center
	barX1     = 0.85 // extends to right
	barY      = 0.5  // horizontal through center
	barThick  = 0.08 // thickness of bar
)

// glyphPad is the empty margin kept around the glyph, as a fraction of the
// glyph's own bounding box.
const glyphPad = 0.10

type iconFile struct {
	name string
	size int
}

// iconset is every file macOS expects in an .iconset directory. Sizes are
// rendered once per distinct pixel size, so a few PNGs are duplicated.
var iconset = []iconFile{
	{"icon_16x16.png", 16},
	{"icon_16x16@2x.png", 32},
	{"icon_32x32.png", 32},
	{"icon_32x32@2x.png", 64},
	{"icon_128x128.png", 128},
	{"icon_128x128@2x.png", 256},
	{"icon_256x256.png", 256},
	{"icon_256x256@2x.png", 512},
	{"icon_512x512.png", 512},
	{"icon_512x512@2x.png", 1024},
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: icongen <iconset-dir>")
		os.Exit(2)
	}
	dir := os.Args[1]
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(err)
	}

	x0, y0, x1, y1 := glyphBox()
	for _, f := range iconset {
		img := render(f.size, x0, y0, x1, y1)
		path := filepath.Join(dir, f.name)
		file, err := os.Create(path)
		if err != nil {
			fail(err)
		}
		if err := png.Encode(file, img); err != nil {
			file.Close()
			fail(err)
		}
		if err := file.Close(); err != nil {
			fail(err)
		}
	}
}

// glyphBox returns the square, padded bounding box of the glyph in unit
// square coordinates, so every icon size frames the letter identically.
func glyphBox() (x0, y0, x1, y1 float64) {
	// The glyph spans from the outer radius to the inner radius
	x0 = bowlCX - bowlROut
	y0 = bowlCY - bowlROut
	x1 = bowlCX + bowlROut
	y1 = bowlCY + bowlROut

	// Also consider the horizontal bar
	x1 = math.Max(x1, barX1)
	y0 = math.Min(y0, barY-barThick/2)
	y1 = math.Max(y1, barY+barThick/2)

	// Square the box around its centre, keeping the glyph centred, then pad it.
	side := math.Max(x1-x0, y1-y0) * (1 + 2*glyphPad)
	cx := (x0 + x1) / 2
	cy := (y0 + y1) / 2
	return cx - side/2, cy - side/2, cx + side/2, cy + side/2
}

// render draws the glyph covering the given box of the unit square into a
// size x size image with a transparent background.
func render(size int, x0, y0, x1, y1 float64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	w := x1 - x0
	h := y1 - y0
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			covered := 0
			for sy := 0; sy < samples; sy++ {
				fy := (float64(py) + (float64(sy)+0.5)/samples) / float64(size)
				for sx := 0; sx < samples; sx++ {
					fx := (float64(px) + (float64(sx)+0.5)/samples) / float64(size)
					if inGlyph(x0+fx*w, y0+fy*h) {
						covered++
					}
				}
			}
			img.SetNRGBA(px, py, color.NRGBA{
				R: letterGreen.R,
				G: letterGreen.G,
				B: letterGreen.B,
				A: uint8(covered * 255 / (samples * samples)),
			})
		}
	}
	return img
}

// inGlyph reports whether the point is covered by the bowl or the bar.
func inGlyph(x, y float64) bool {
	// Check if in the C-shaped bowl (annulus with a gap on the right)
	dx := x - bowlCX
	dy := y - bowlCY
	d := math.Hypot(dx, dy)

	if d <= bowlROut && d >= bowlRIn {
		// Check if in the gap (right side opening)
		angle := math.Atan2(dy, dx)
		// Normalize angle to [-pi, pi]
		// Gap is from gapEndAngle to gapStartAngle (crossing -pi/pi boundary)
		if gapEndAngle < gapStartAngle {
			// Gap doesn't cross the -pi/pi boundary
			if angle >= gapEndAngle && angle <= gapStartAngle {
				return false // In the gap
			}
		} else {
			// Gap crosses the boundary
			if angle >= gapEndAngle || angle <= gapStartAngle {
				return false // In the gap
			}
		}
		return true
	}

	// Check if in the horizontal bar of the G
	if x >= barX0 && x <= barX1 && y >= barY-barThick/2 && y <= barY+barThick/2 {
		return true
	}

	return false
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "icongen:", err)
	os.Exit(1)
}