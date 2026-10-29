package ggui

import (
	"math"
	"testing"

	"github.com/ironpark/ggui/internal/text"
	"github.com/ironpark/ggui/internal/textedit"
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
	shifts := cellShifts(nil, textedit.Bounds(s), glyphs, x0, cell)
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

// Shifts found from a run's bounds are those stepping through it grapheme
// by grapheme finds, whatever its clusters are.
func TestCellShiftsFollowTheRunsGraphemes(t *testing.T) {
	face := MustFont(gomono.TTF).NoFallback().face(13*2, 0)
	const x0, cell = 20.25, 17.0
	for _, s := range []string{"é" + "abc", "한글 ok", "á̂b", "👩‍💻x", "ffi fl", "\xffz", "❯ go test"} {
		glyphs := text.AppendLazyGlyphs(nil, s, face, nil)
		got := cellShifts(nil, textedit.Bounds(s), glyphs, x0, cell)
		k, next, shift := -1, 0, 0.0
		for i, g := range glyphs {
			if g.StartIndexInBytes >= next {
				for g.StartIndexInBytes >= next && next < len(s) {
					next = textedit.NextGrapheme(s, next)
					k++
				}
				shift = math.Round(x0+float64(k)*cell) - x0 - g.OriginX
			}
			if got[i] != shift {
				t.Errorf("%q glyph %d: shift %v, want %v", s, i, got[i], shift)
			}
		}
	}
}
