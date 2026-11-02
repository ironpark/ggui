package ggui

import (
	"image/color"
	"testing"

	"github.com/ironpark/ggui/internal/property"
)

func TestTextStylesCompareWithoutAllocating(t *testing.T) {
	a := TextStyle{Size: 13, Color: color.RGBA{1, 2, 3, 255}, LineHeight: 1.4, Weight: 500}
	b := a
	b.Color = color.NRGBA{1, 2, 3, 255}
	if !property.Equal(a, b) {
		t.Fatal("styles of colors drawing alike are not equal")
	}
	c := a
	c.Size = 14
	if property.Equal(a, c) {
		t.Fatal("styles of two sizes are equal")
	}
	if property.Equal(a, TextStyle{Size: 13, LineHeight: 1.4, Weight: 500}) {
		t.Fatal("a style with a color equals one without")
	}
	if n := testing.AllocsPerRun(100, func() { property.Equal(a, b) }); n != 0 {
		t.Fatalf("comparing two styles took %v allocations", n)
	}
}
