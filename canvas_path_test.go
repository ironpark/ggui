package ggui

import (
	"reflect"
	"testing"

	"github.com/hajimehoshi/ebiten/v2/vector"
)

func TestRoundRectPathReuse(t *testing.T) {
	// Alternate geometry and scale to catch commands left over from an earlier
	// draw. Compare both fill and stroke tessellation with a fresh path.
	p := new(vector.Path)
	for _, scale := range []float64{1, 2, 1.25} {
		c := &Canvas{scale: scale}
		for _, radius := range []float64{8, 0, 100, 0.5} {
			r := Rct(Pt(-3.25, 7.5), Sz(31, 19))
			p.Reset()
			c.roundRect(p, r, radius)
			var fresh vector.Path
			c.roundRect(&fresh, r, radius)
			gotV, gotI := p.AppendVerticesAndIndicesForFilling(nil, nil)
			wantV, wantI := fresh.AppendVerticesAndIndicesForFilling(nil, nil)
			if !reflect.DeepEqual(gotV, wantV) || !reflect.DeepEqual(gotI, wantI) {
				t.Fatal("reused fill path differs from fresh path")
			}
			op := &vector.StrokeOptions{Width: c.Px(3.5)}
			gotV, gotI = p.AppendVerticesAndIndicesForStroke(nil, nil, op)
			wantV, wantI = fresh.AppendVerticesAndIndicesForStroke(nil, nil, op)
			if !reflect.DeepEqual(gotV, wantV) || !reflect.DeepEqual(gotI, wantI) {
				t.Fatal("reused stroke path differs from fresh path")
			}
		}
	}
}

func BenchmarkRoundRectPath(b *testing.B) {
	c := &Canvas{scale: 2}
	r := Rct(Pt(10.25, 20.5), Sz(120, 36))
	b.Run("Fresh", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			p := new(vector.Path)
			c.roundRect(p, r, 8)
			roundRectBenchmarkSink = p
		}
	})
	b.Run("Pooled", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			p := roundRectPaths.Get().(*vector.Path)
			c.roundRect(p, r, 8)
			roundRectBenchmarkSink = p
			releaseRoundRectPath(p)
		}
	})
}

var roundRectBenchmarkSink *vector.Path
