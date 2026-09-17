package ggui

import "testing"

func TestConstraintsConstrain(t *testing.T) {
	c := Constraints{MinW: 10, MinH: 10, MaxW: 100, MaxH: 100}
	got := c.Constrain(Size{W: 5, H: 200})
	if got != (Size{W: 10, H: 100}) {
		t.Fatalf("Constrain() = %+v, want {10 100}", got)
	}
}

func TestBoxAddsPaddingAroundChild(t *testing.T) {
	child := &Box{Width: 20, Height: 10}
	b := &Box{Padding: All(5), Child: child}
	got := b.Layout(Loose(Size{W: 200, H: 200}))
	if got != (Size{W: 30, H: 20}) {
		t.Fatalf("Layout() = %+v, want {30 20}", got)
	}
}

func TestColumnSumsHeightsAndGaps(t *testing.T) {
	col := &Column{
		Gap: 4,
		Children: []Widget{
			&Box{Width: 10, Height: 10},
			&Box{Width: 30, Height: 20},
		},
	}
	got := col.Layout(Loose(Size{W: 200, H: 200}))
	if got != (Size{W: 30, H: 34}) {
		t.Fatalf("Layout() = %+v, want {30 34}", got)
	}
}

func TestCenterFillsAvailableSpace(t *testing.T) {
	c := &Center{Child: &Box{Width: 10, Height: 10}}
	got := c.Layout(Loose(Size{W: 100, H: 50}))
	if got != (Size{W: 100, H: 50}) {
		t.Fatalf("Layout() = %+v, want {100 50}", got)
	}
}

func TestSzAndPtAcceptAnyNumericType(t *testing.T) {
	if got := Sz(3, 4); got != (Size{W: 3, H: 4}) {
		t.Fatalf("Sz() = %+v, want {3 4}", got)
	}
	if got := Pt(float32(1.5), float32(2.5)); got != (Point{X: 1.5, Y: 2.5}) {
		t.Fatalf("Pt() = %+v, want {1.5 2.5}", got)
	}
}

func TestChildrenBuildsOnePerItem(t *testing.T) {
	got := Children([]float64{10, 20}, func(h float64) Widget {
		return &Box{Width: 5, Height: h}
	})
	if len(got) != 2 {
		t.Fatalf("len(Children()) = %d, want 2", len(got))
	}
	if s := got[1].Layout(Loose(Sz(100, 100))); s != (Size{W: 5, H: 20}) {
		t.Fatalf("second child = %+v, want {5 20}", s)
	}
}

func TestListLaysOutItemsLikeColumn(t *testing.T) {
	l := &List[float64]{
		Gap:   4,
		Items: []float64{10, 20},
		Item:  func(h float64) Widget { return &Box{Width: 30, Height: h} },
	}
	got := l.Layout(Loose(Sz(200, 200)))
	if got != (Size{W: 30, H: 34}) {
		t.Fatalf("Layout() = %+v, want {30 34}", got)
	}
}

func TestListFollowsItemChanges(t *testing.T) {
	l := &List[float64]{
		Items: []float64{10},
		Item:  func(h float64) Widget { return &Box{Width: 10, Height: h} },
	}
	l.Layout(Loose(Sz(200, 200)))
	l.Items = append(l.Items, 5)
	got := l.Layout(Loose(Sz(200, 200)))
	if got != (Size{W: 10, H: 15}) {
		t.Fatalf("Layout() = %+v, want {10 15} after Items changed", got)
	}
}
