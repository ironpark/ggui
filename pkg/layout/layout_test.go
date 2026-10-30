package layout_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/ironpark/ggui/pkg/layout"
)

func fixed(sizes ...layout.Size) layout.MeasureFunc {
	return func(i int, c layout.Constraints) layout.Measurement {
		return layout.Measurement{Size: c.Constrain(sizes[i])}
	}
}

func TestFlowAllocationAndMeasurementOrder(t *testing.T) {
	var scratch layout.Scratch
	var order []int
	items := []layout.Item{{Weight: 1}, {}, {Weight: 3, Loose: true}}
	got := scratch.Flow(layout.Loose(layout.Sz(120, 40)), layout.FlowStyle{Axis: layout.Horizontal, Gap: 5}, items, func(i int, c layout.Constraints) layout.Measurement {
		order = append(order, i)
		switch i {
		case 0:
			if c.MinW != 20 || c.MaxW != 20 {
				t.Fatalf("tight share: %+v", c)
			}
		case 1:
			if c.MaxW != 110 {
				t.Fatalf("rigid budget: %+v", c)
			}
		case 2:
			if c.MinW != 0 || c.MaxW != 60 {
				t.Fatalf("loose share: %+v", c)
			}
		}
		return layout.Measurement{Size: c.Constrain(layout.Sz(30, 10))}
	})
	if !reflect.DeepEqual(order, []int{1, 0, 2}) {
		t.Fatalf("order %v", order)
	}
	want := []layout.Rect{{Size: layout.Sz(20, 10)}, {Origin: layout.Pt(25, 0), Size: layout.Sz(30, 10)}, {Origin: layout.Pt(60, 0), Size: layout.Sz(30, 10)}}
	if got.Size != layout.Sz(120, 10) || !reflect.DeepEqual(got.Children, want) {
		t.Fatalf("layout %+v", got)
	}
}

func TestFlowBaselineAndDirection(t *testing.T) {
	for _, dir := range []layout.Direction{layout.LTR, layout.RTL} {
		var scratch layout.Scratch
		got := scratch.Flow(layout.Loose(layout.Sz(100, 100)), layout.FlowStyle{Axis: layout.Horizontal, Align: layout.AlignBaseline, Direction: dir}, make([]layout.Item, 3), func(i int, c layout.Constraints) layout.Measurement {
			return []layout.Measurement{
				{Size: layout.Sz(10, 20), Baseline: 15, HasBaseline: true},
				{Size: layout.Sz(20, 10), Baseline: 6, HasBaseline: true},
				{Size: layout.Sz(5, 18)},
			}[i]
		})
		if got.Size != layout.Sz(35, 23) || !got.HasBaseline || got.Baseline != 18 {
			t.Fatalf("baseline %+v", got)
		}
		for i, wantY := range []float64{3, 12, 0} {
			if got.Children[i].Origin.Y != wantY {
				t.Fatalf("child %d: %+v", i, got.Children[i])
			}
		}
		wantX := []float64{0, 10, 30}
		if dir == layout.RTL {
			wantX = []float64{25, 5, 0}
		}
		for i, x := range wantX {
			if got.Children[i].Origin.X != x {
				t.Fatalf("direction %s: %v", dir, got.Children)
			}
		}
	}
}

func TestAbsentAndZeroSizedChildrenHaveDifferentGaps(t *testing.T) {
	for _, absent := range []bool{true, false} {
		var scratch layout.Scratch
		got := scratch.Flow(layout.Loose(layout.Sz(100, 100)), layout.FlowStyle{Axis: layout.Horizontal, Gap: 5}, make([]layout.Item, 3), func(i int, c layout.Constraints) layout.Measurement {
			if i == 1 {
				return layout.Measurement{Absent: absent}
			}
			return layout.Measurement{Size: layout.Sz(10, 10)}
		})
		width, lastX := 30.0, 20.0
		if absent {
			width, lastX = 25, 15
		}
		if got.Size.W != width || got.Children[2].Origin.X != lastX {
			t.Fatalf("absent %v: %+v", absent, got)
		}
	}
}

func TestUnboundedFlowMeasuresFlexNaturally(t *testing.T) {
	var scratch layout.Scratch
	got := scratch.Flow(layout.Loose(layout.Sz(math.Inf(1), 100)), layout.FlowStyle{Axis: layout.Horizontal, Justify: layout.SpaceBetween, Align: layout.AlignStretch}, []layout.Item{{Weight: 1}, {}}, fixed(layout.Sz(10, 5), layout.Sz(20, 8)))
	if got.Size != layout.Sz(30, 100) || got.Children[1].Origin.X != 10 {
		t.Fatalf("unbounded %+v", got)
	}
}

func TestColumnStretchAndRTL(t *testing.T) {
	var scratch layout.Scratch
	got := scratch.Flow(layout.Loose(layout.Sz(100, 100)), layout.FlowStyle{Gap: 5, Align: layout.AlignStretch, Direction: layout.RTL}, make([]layout.Item, 2), fixed(layout.Sz(10, 10), layout.Sz(30, 20)))
	if got.Size != layout.Sz(100, 35) || got.Children[1] != (layout.Rect{Origin: layout.Pt(0, 15), Size: layout.Sz(100, 20)}) {
		t.Fatalf("column %+v", got)
	}
}

func TestJustification(t *testing.T) {
	for _, tc := range []struct {
		j           layout.Justify
		first, last float64
	}{
		{layout.JustifyStart, 0, 10}, {layout.JustifyCenter, 40, 50}, {layout.JustifyEnd, 80, 90},
		{layout.SpaceBetween, 0, 90}, {layout.SpaceAround, 20, 70}, {layout.SpaceEvenly, 80.0 / 3, 10 + 160.0/3},
	} {
		var scratch layout.Scratch
		got := scratch.Flow(layout.Tight(layout.Sz(100, 10)), layout.FlowStyle{Axis: layout.Horizontal, Justify: tc.j}, make([]layout.Item, 2), fixed(layout.Sz(10, 10), layout.Sz(10, 10)))
		if math.Abs(got.Children[0].Origin.X-tc.first) > 1e-9 || math.Abs(got.Children[1].Origin.X-tc.last) > 1e-9 {
			t.Fatalf("justify %d: %v", tc.j, got.Children)
		}
	}
}

func TestWrapAndGrid(t *testing.T) {
	var scratch layout.Scratch
	got := scratch.Wrap(layout.Loose(layout.Sz(65, 100)), layout.WrapStyle{Gap: 5, RunGap: 7, Align: layout.AlignEnd}, 3, fixed(layout.Sz(30, 10), layout.Sz(30, 20), layout.Sz(30, 8)))
	want := []layout.Rect{{Origin: layout.Pt(0, 10), Size: layout.Sz(30, 10)}, {Origin: layout.Pt(35, 0), Size: layout.Sz(30, 20)}, {Origin: layout.Pt(0, 27), Size: layout.Sz(30, 8)}}
	if got.Size != layout.Sz(65, 35) || !reflect.DeepEqual(got.Children, want) {
		t.Fatalf("wrap %+v", got)
	}
	grid := scratch.Grid(layout.Loose(layout.Sz(65, 100)), layout.GridStyle{Columns: 2, Gap: 5, RowGap: 7}, 3, fixed(layout.Sz(5, 10), layout.Sz(5, 20), layout.Sz(5, 8)))
	if grid.Size != layout.Sz(65, 35) || grid.Children[2].Origin != layout.Pt(0, 27) || grid.Children[0].Size.W != 30 || grid.CellWidth != 30 {
		t.Fatalf("grid %+v", grid)
	}
}

func TestUnboundedGridMeasuresTwice(t *testing.T) {
	var scratch layout.Scratch
	calls := 0
	measure := fixed(layout.Sz(10, 10), layout.Sz(30, 15))
	got := scratch.Grid(layout.Loose(layout.Sz(math.Inf(1), 100)), layout.GridStyle{Columns: 2, Gap: 5}, 2, func(i int, c layout.Constraints) layout.Measurement {
		calls++
		return measure(i, c)
	})
	if calls != 4 || got.Size != layout.Sz(65, 15) || got.Children[0].Size.W != 30 || got.CellWidth != 30 {
		t.Fatalf("grid calls %d: %+v", calls, got)
	}
}

func TestBoxConstraintsAndParentMinimum(t *testing.T) {
	for _, tc := range []struct {
		c                    layout.Constraints
		width, wantW, innerW float64
	}{
		{layout.Loose(layout.Sz(500, 100)), 300, 100, 80},
		{layout.Tight(layout.Sz(500, 100)), 300, 500, 480},
		{layout.Loose(layout.Sz(500, 100)), 0, 70, 80},
	} {
		got := layout.Box(tc.c, layout.BoxStyle{Width: tc.width, MaxWidth: 100, Padding: layout.Insets{Top: 10, Right: 10, Bottom: 10, Left: 10}}, func(_ int, c layout.Constraints) layout.Measurement {
			if c.MaxW != tc.innerW {
				t.Fatalf("inner %+v", c)
			}
			return layout.Measurement{Size: c.Constrain(layout.Sz(50, 20)), Baseline: 15, HasBaseline: true}
		})
		if got.Size.W != tc.wantW || got.Child.Origin != layout.Pt(10, 10) || got.Baseline != 25 {
			t.Fatalf("box %+v", got)
		}
	}
}

func TestAlignStackAndEmptyLayouts(t *testing.T) {
	var scratch layout.Scratch
	aligned := layout.Align(layout.Loose(layout.Sz(100, 50)), .5, 1, func(_ int, c layout.Constraints) layout.Measurement {
		return layout.Measurement{Size: layout.Sz(20, 10), Baseline: 7, HasBaseline: true}
	})
	if aligned.Child.Origin != layout.Pt(40, 40) || aligned.Baseline != 47 {
		t.Fatalf("align %+v", aligned)
	}
	got := scratch.Stack(layout.Loose(layout.Sz(100, math.Inf(1))), true, 2, fixed(layout.Sz(20, 10), layout.Sz(10, 30)))
	if got.Size != layout.Sz(100, 30) {
		t.Fatalf("stack %+v", got)
	}
	for _, run := range []func() layout.Result{
		func() layout.Result { return scratch.Flow(layout.Constraints{}, layout.FlowStyle{}, nil, nil) },
		func() layout.Result { return scratch.Wrap(layout.Constraints{}, layout.WrapStyle{}, 0, nil) },
		func() layout.Result { return scratch.Grid(layout.Constraints{}, layout.GridStyle{}, 0, nil).Result },
		func() layout.Result { return scratch.Stack(layout.Constraints{}, false, 0, nil) },
	} {
		got = run()
		if got.Size != (layout.Size{}) || len(got.Children) != 0 || got.HasBaseline {
			t.Fatalf("empty %+v", got)
		}
	}
}

func TestBoxWithoutChild(t *testing.T) {
	got := layout.Box(layout.Loose(layout.Sz(100, 100)), layout.BoxStyle{Width: 50, Height: 30}, nil)
	if got.Size != layout.Sz(50, 30) || got.HasChild || got.HasBaseline {
		t.Fatalf("empty box %+v", got)
	}
}

func BenchmarkFlow(b *testing.B) {
	var scratch layout.Scratch
	items := make([]layout.Item, 20)
	items[19].Weight = 1
	measure := func(_ int, c layout.Constraints) layout.Measurement {
		return layout.Measurement{Size: c.Constrain(layout.Sz(20, 10))}
	}
	c := layout.Loose(layout.Sz(1000, 100))
	style := layout.FlowStyle{Axis: layout.Horizontal, Gap: 5}
	scratch.Flow(c, style, items, measure)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		scratch.Flow(c, style, items, measure)
	}
}
