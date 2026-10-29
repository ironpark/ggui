package ggui

import (
	"math"
	"testing"

	"github.com/ironpark/ggui/internal/text"
	"golang.org/x/image/font/gofont/gomono"
)

// Each grapheme starts its own cell on a whole pixel, however far the
// font's advance is from the cell's width, so a long row does not drift.
func TestDrawTextCellsKeepsEveryGraphemeOnItsCell(t *testing.T) {
	t.Parallel()
	face := MustFont(gomono.TTF).NoFallback().face(13*2, 0)
	adv := lineWidth("M", face)
	cell := math.Round(adv) + 1 // well off the advance, so drift would show
	const x0 = 20.25
	s := "é" + "abcdefghijklmnopqrstuvwxyz0123456789"
	glyphs := text.AppendLazyGlyphs(nil, s, face, nil)
	shifts := cellShifts(s, glyphs, x0, cell)
	k, last := -1, -1
	for i, g := range glyphs {
		at := x0 + g.OriginX + shifts[i]
		if g.StartIndexInBytes < 3 { // é, its mark included
			if g.StartIndexInBytes == 0 {
				k = 0
			}
			if want := math.Round(x0) + g.OriginX - glyphs[0].OriginX; math.Abs(at-want) > 1e-9 {
				t.Fatalf("glyph %d of é at %v, want %v: the mark left its letter", i, at, want)
			}
			continue
		}
		if g.StartIndexInBytes != last {
			k++
			last = g.StartIndexInBytes
		}
		if want := math.Round(x0 + float64(k)*cell); at != want {
			t.Fatalf("grapheme %d at %v, want its cell at %v", k, at, want)
		}
	}
	if k != 36 {
		t.Fatalf("placed %d graphemes after é, want 36", k)
	}
	(&Canvas{}).DrawTextCells("no image, no panic", TextStyle{}, Pt(0, 0), 8)
}
