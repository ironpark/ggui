package ggui

import (
	"image"
	"testing"
)

// sdfBounds decides the quad a distance-field shape is drawn over. It has to
// cover every pixel the shape can tint, including the edge it feathers over
// and a border drawn outside the outline, and nothing the target cannot show.
func TestSDFBoundsCoversTheShapeAndClipsToTheTarget(t *testing.T) {
	target := image.Rect(0, 0, 100, 60)
	for _, tc := range []struct {
		name                  string
		cx, cy, hw, hh, reach float64
		want                  image.Rectangle
		ok                    bool
	}{
		{"inside", 50, 30, 10, 5, 0, image.Rect(40, 25, 60, 35), true},
		{"reach widens the quad", 50, 30, 10, 5, 2, image.Rect(38, 23, 62, 37), true},
		{"a fractional edge rounds outwards", 50.4, 30, 10, 5, 0, image.Rect(40, 25, 61, 35), true},
		{"clipped at the left", 2, 30, 10, 5, 0, image.Rect(0, 25, 12, 35), true},
		{"clipped on every side", 50, 30, 500, 500, 0, target, true},
		{"entirely off the left", -20, 30, 5, 5, 0, image.Rectangle{}, false},
		{"entirely below", 50, 200, 5, 5, 0, image.Rectangle{}, false},
		{"touching the edge covers nothing", -5, 30, 5, 5, 0, image.Rectangle{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := sdfBounds(target, tc.cx, tc.cy, tc.hw, tc.hh, tc.reach)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("bounds = %v, want %v", got, tc.want)
			}
		})
	}
}

// A clip paints into a sub-image, whose bounds do not start at the origin.
// The quad has to stay inside them, or the shape would spill past the clip.
func TestSDFBoundsStaysInsideAClip(t *testing.T) {
	clip := image.Rect(20, 10, 60, 40)
	got, ok := sdfBounds(clip, 30, 20, 100, 100, 3)
	if !ok || got != clip {
		t.Fatalf("bounds = %v (%v), want the clip %v", got, ok, clip)
	}
	if _, ok := sdfBounds(clip, 5, 20, 5, 5, 0); ok {
		t.Fatal("a shape left of the clip was given a quad")
	}
}
