package ui

import "testing"

// A link button's underline starts 4px under the letters, shadcn's
// underline-offset-4, which means under the baseline they were drawn on.
// These are link buttons rendered at four positions and three sizes: the
// top of the label's line, the ascent layout reports for it, and the row
// under the last one an H leaves ink on.
func TestPaintedBaselineIsWhereTheGlyphsStand(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ top, ascent, want float64 }{
		{30.242, 14.966, 44}, {30.542, 14.966, 45}, {30.742, 14.966, 45}, {31.042, 14.966, 45}, // default, 14px
		{29.078, 13.683, 42}, {29.378, 13.683, 42}, {29.578, 13.683, 43}, {29.878, 13.683, 43}, // sm, 12.8px
		{27.636, 12.828, 40}, {27.936, 12.828, 40}, {28.136, 12.828, 40}, {28.436, 12.828, 40}, // xs, 12px
	} {
		if got := paintedBaseline(c.top, c.ascent, 1); got != c.want {
			t.Errorf("a line at %v with an ascent of %v: baseline %v, want %v as rendered", c.top, c.ascent, got, c.want)
		}
	}
	// On a display of twice the density the pixels are half the size.
	if got := paintedBaseline(30.242, 14.966, 2); got != 44.5 {
		t.Errorf("at twice the scale the baseline is %v, want 44.5", got)
	}
}
