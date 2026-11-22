package widget

import (
	"slices"
	"testing"

	"github.com/ironpark/ggui"
)

func TestDragSourceRebuiltMidDragLaysOutPreviewBeforePaint(t *testing.T) {
	t.Parallel()
	previewLaid := 0
	preview := ggui.FromFuncs(
		func(c ggui.Constraints, _ ggui.Env) ggui.Size {
			previewLaid++
			return c.Constrain(ggui.Sz(40, 20))
		},
		func(*ggui.Canvas, ggui.Rect) {},
	)
	prev := DragSource(Box().Size(40, 20), "v")
	prev.DragMoved(ggui.Pt(30, 10), ggui.Pt(5, 5))

	// A rebuilt DragSource lays out before Adopt runs, so its preview is not laid
	// out in Layout; Paint must lay out the preview before painting the overlay.
	rebuilt := DragSource(Box().Size(40, 20), "v").Preview(preview)
	rebuilt.Layout(ggui.Loose(ggui.Sz(200, 100)), ggui.Env{})
	if previewLaid != 0 {
		t.Fatalf("preview laid out %d times before dragging, want 0", previewLaid)
	}
	rebuilt.Adopt(prev)
	p := ggui.NewProbe(ggui.FromFuncs(
		func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(40, 20)) },
		func(dst *ggui.Canvas, r ggui.Rect) { rebuilt.Paint(dst, r) },
	), ggui.Sz(200, 100))
	defer p.Close()
	p.Frame()
	if previewLaid != 1 {
		t.Fatalf("rebuilt DragSource preview laid out %d times before painting overlay, want 1", previewLaid)
	}
}

func TestReorderableRebuiltMidDragAdoptsAndCommitsOnLiveInstance(t *testing.T) {
	t.Parallel()
	items := ggui.State([]int{1, 2, 3})
	rebuild := ggui.State(0)
	reordered := 0
	p := ggui.NewProbe(ggui.View(rebuild, func(int) ggui.Widget {
		return Reorderable(items, func(v int) int { return v }, func(it EachItem[int]) ggui.Widget {
			return Box().Size(100, 20)
		}).Key("list").OnReorder(func(from, to int) { reordered++ })
	}), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()
	p.Press(ggui.Pt(50, 10)) // row 0 (item 1)
	p.Move(ggui.Pt(50, 55))  // drag past row 2
	rebuild.Set(1)           // rebuild Reorderable mid-drag
	p.Frame()
	p.Release(ggui.Pt(50, 55))
	p.Frame()
	got := ggui.Peek(items)
	if !slices.Equal(got, []int{2, 3, 1}) || reordered != 1 {
		t.Fatalf("after mid-drag rebuild and drop: items=%v reordered=%d, want [2 3 1] and 1", got, reordered)
	}
}
