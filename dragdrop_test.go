package ggui

import (
	"slices"
	"testing"
)

func TestDragFromASourceDropsOnAZoneThatTakesIt(t *testing.T) {
	t.Parallel()
	taps, ended := 0, []bool{}
	var got []Dropped[int]
	var hovers []bool
	source := DragSource(Tap(Box().Size(100, 40), func() { taps++ }), 7).OnDragEnd(func(d bool) { ended = append(ended, d) })
	strings := DropZone(Box().Size(100, 40), func(Dropped[string]) { t.Fatal("a zone for strings took an int") })
	zone := DropZone(Box().Size(100, 40), func(d Dropped[int]) { got = append(got, d) }).OnHover(func(over bool) { hovers = append(hovers, over) })
	p := NewProbe(Column(source, Stack(zone, strings)), Sz(100, 120))
	defer p.Close()
	p.Frame()

	p.Click(Pt(50, 20))
	if taps != 1 || len(ended) != 0 {
		t.Fatalf("a click on the source: %d taps, %d drags; want the child's tap alone", taps, len(ended))
	}
	p.Press(Pt(50, 20))
	p.Move(Pt(52, 21)) // inside the threshold: still a press
	if source.Dragging() {
		t.Fatal("a drag started before the pointer moved far enough")
	}
	p.Move(Pt(50, 50))
	p.Move(Pt(50, 70))
	if !source.Dragging() || !zone.Over() {
		t.Fatalf("over the zone: dragging %v, zone over %v", source.Dragging(), zone.Over())
	}
	p.Release(Pt(50, 70))
	if taps != 1 {
		t.Fatal("the drag ended in a tap on the child")
	}
	if len(got) != 1 || got[0].Value != 7 || got[0].Pos != Pt(50, 30) || got[0].Size != Sz(100, 40) || !slices.Equal(ended, []bool{true}) {
		t.Fatalf("dropped %+v, ended %v; want 7 at (50,30) in the zone, ended with a drop", got, ended)
	}
	if !slices.Equal(hovers, []bool{true, false}) || source.Dragging() {
		t.Fatalf("hover %v, dragging after the drop %v", hovers, source.Dragging())
	}

	p.Press(Pt(50, 20))
	p.Move(Pt(50, 70))
	p.Type(Mods{}, KeyEscape)
	p.Release(Pt(50, 70))
	if len(got) != 1 || !slices.Equal(ended, []bool{true, false}) || zone.Over() {
		t.Fatalf("Escape: dropped %d times, ended %v, zone over %v; want the drag cancelled", len(got), ended, zone.Over())
	}
}

func TestDropZoneAcceptNarrowsWhatItTakes(t *testing.T) {
	t.Parallel()
	dropped := 0
	zone := DropZone(Box().Size(100, 40), func(Dropped[int]) { dropped++ }).Accept(func(v int) bool { return v > 10 })
	p := NewProbe(Column(DragSource(Box().Size(100, 40), 7), zone), Sz(100, 80))
	defer p.Close()
	p.Frame()
	p.Press(Pt(50, 20))
	p.Move(Pt(50, 60))
	p.Release(Pt(50, 60))
	if dropped != 0 || zone.Over() {
		t.Fatal("a zone took a value its Accept refused")
	}
}

func TestMoveReordersAsADragWould(t *testing.T) {
	t.Parallel()
	in := []string{"a", "b", "c", "d"}
	for _, c := range []struct {
		from, to int
		want     []string
	}{
		{0, 2, []string{"b", "a", "c", "d"}},
		{0, 4, []string{"b", "c", "d", "a"}},
		{3, 0, []string{"d", "a", "b", "c"}},
		{1, 1, in}, {1, 2, in}, {-1, 0, in}, {0, 5, in},
	} {
		if got := Move(in, c.from, c.to); !slices.Equal(got, c.want) {
			t.Errorf("Move(%v, %d, %d) = %v, want %v", in, c.from, c.to, got, c.want)
		}
	}
	if !slices.Equal(in, []string{"a", "b", "c", "d"}) {
		t.Fatal("Move changed its argument")
	}
}

func TestAListReordersByDraggingItsRows(t *testing.T) {
	t.Parallel()
	type item struct {
		ID   int
		Name string
	}
	items := State([]item{{1, "a"}, {2, "b"}, {3, "c"}})
	list := EachKeyed(items, func(it item) int { return it.ID }, func(row EachItem[item]) Widget {
		id := Untrack(row.Value.Get).ID
		return DropZone(DragSource(Box().Size(100, 30), id), func(d Dropped[int]) {
			list := Untrack(items.Get)
			from := slices.IndexFunc(list, func(it item) bool { return it.ID == d.Value })
			to := Untrack(row.Index.Get)
			if !d.Before() {
				to++
			}
			items.Set(Move(list, from, to))
		})
	}).Gap(0)
	p := NewProbe(list, Sz(100, 90))
	defer p.Close()
	p.Frame()
	drag := func(from, to Point) {
		p.Press(from)
		p.Move(Pt(from.X, from.Y+10))
		p.Move(to)
		p.Release(to)
	}
	names := func() string {
		s := ""
		for _, it := range Untrack(items.Get) {
			s += it.Name
		}
		return s
	}
	drag(Pt(50, 15), Pt(50, 80)) // a onto the lower half of c
	if got := names(); got != "bca" {
		t.Fatalf("after dragging a below c: %q, want bca", got)
	}
	drag(Pt(50, 75), Pt(50, 5)) // a onto the upper half of b
	if got := names(); got != "abc" {
		t.Fatalf("after dragging a above b: %q, want abc", got)
	}
}
