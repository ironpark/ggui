package chart

import (
	"cmp"
	"image/color"

	"github.com/ironpark/ggui/gem"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func pick[T any](cond bool, a, b T) T { return gem.Pick(cond, a, b) }

func clamp[T cmp.Ordered](v, lo, hi T) T { return gem.Clamp(v, lo, hi) }

func bounded(v, fallback float64) float64 { return gem.Bounded(v, fallback) }

func fade(c color.Color, alpha float64) color.Color { return uitheme.Fade(c, alpha) }
