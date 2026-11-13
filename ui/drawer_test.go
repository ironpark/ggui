package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func drawerGrip(s *ui.SheetWidget) ggui.Point {
	r := s.Rect()
	return ggui.Pt(r.Origin.X+r.Size.W/2, r.Origin.Y+8)
}

func TestDrawerDragDismissesAndReturnsFocus(t *testing.T) {
	t.Parallel()
	open := ggui.State(false)
	closed := 0
	s := ui.Drawer(open, ggui.Text("Content")).Name("Details").Size(200).OnClose(func() { closed++ })
	p := ggui.NewProbe(ggui.Column(ui.Button("Show", func() { open.Set(true) }), s), ggui.Sz(400, 300))
	defer p.Close()
	p.Key("tab", "enter")
	p.Frame()
	p.Advance(time.Second)
	start := drawerGrip(s)
	p.Press(start)
	p.Move(start.Add(ggui.Pt(0, 70)))
	p.Frame()
	if !ggui.Peek(open) || s.Rect().Origin.Y != 170 {
		t.Fatalf("drag open=%v rect=%v", ggui.Peek(open), s.Rect())
	}
	p.Release(start.Add(ggui.Pt(0, 140)))
	if ggui.Peek(open) || closed != 1 {
		t.Fatalf("dismiss open=%v closed=%d", ggui.Peek(open), closed)
	}
	p.Frame()
	p.Advance(time.Second)
	p.Move(ggui.Pt(0, 0)) // focus traps restore on input dispatch
	if n, ok := p.Semantics().Focused(); !ok || n.Name != "Show" {
		t.Fatalf("focus not restored: %+v", n)
	}
	p.Frame()
	if closed != 1 {
		t.Fatal("duplicate close")
	}
}

func TestDrawerSnapsOnReleaseAndFollowsBinding(t *testing.T) {
	t.Parallel()
	selected := ggui.State(0)
	changes := 0
	s := ui.Drawer(ggui.State(true), ggui.Text("Content")).Name("Details").SnapPoints(80, 160, 260).BindSnapPoint(selected).OnSnapChange(func(int) { changes++ })
	p := openSheet(t, s)
	if s.Rect().Origin.Y != 220 {
		t.Fatalf("initial snap: %v", s.Rect())
	}
	start := drawerGrip(s)
	p.Press(start)
	p.Move(start.Add(ggui.Pt(0, -100)))
	if ggui.Peek(selected) != 0 || changes != 0 {
		t.Fatal("drag changed selected index before release")
	}
	p.Release(start.Add(ggui.Pt(0, -100)))
	p.Frame()
	p.Advance(time.Second)
	if ggui.Peek(selected) != 1 || changes != 1 || s.Rect().Origin.Y != 140 {
		t.Fatalf("snap=%d changes=%d rect=%v", ggui.Peek(selected), changes, s.Rect())
	}
	selected.Set(2)
	p.Frame()
	p.Frame()
	p.Advance(time.Second)
	if s.Rect().Origin.Y != 40 || changes != 1 {
		t.Fatalf("external snap rect=%v changes=%d", s.Rect(), changes)
	}
	act(t, p, ggui.RoleSlider, "Details size", ggui.Action{Kind: ggui.ActionDecrement})
	p.Frame()
	p.Advance(time.Second)
	if ggui.Peek(selected) != 1 || changes != 2 {
		t.Fatal("accessible decrement did not snap")
	}
	p.Click(drawerGrip(s))
	p.Key("end")
	p.Frame()
	p.Advance(time.Second)
	if ggui.Peek(selected) != 2 {
		t.Fatal("keyboard End did not snap")
	}
	p.Key("home")
	p.Frame()
	p.Advance(time.Second)
	if ggui.Peek(selected) != 0 {
		t.Fatal("keyboard Home did not snap")
	}
}

func TestDrawerControlledSnapCanBeRejected(t *testing.T) {
	t.Parallel()
	requested := -1
	s := ui.Drawer(ggui.State(true), ggui.Text("Content")).SnapPoints(80, 160, 260).BindSnapPoint(ggui.Controlled(0, func(i int) { requested = i }))
	p := openSheet(t, s)
	start := drawerGrip(s)
	p.Press(start)
	p.Release(start.Add(ggui.Pt(0, -170)))
	p.Frame()
	p.Advance(time.Second)
	if requested != 2 || s.Rect().Origin.Y != 220 {
		t.Fatalf("rejected snap requested=%d rect=%v", requested, s.Rect())
	}
}

func TestDrawerNonDismissibleDragSettlesAndEscapeStillCloses(t *testing.T) {
	t.Parallel()
	open := ggui.State(true)
	s := ui.Drawer(open, ggui.Text("Content")).SnapPoints(80, 160).SnapPoint(1).Dismissible(false)
	p := openSheet(t, s)
	start := drawerGrip(s)
	p.Press(start)
	p.Move(start.Add(ggui.Pt(0, 140)))
	p.Release(start.Add(ggui.Pt(0, 140)))
	p.Frame()
	p.Advance(time.Second)
	if !ggui.Peek(open) || s.Rect().Origin.Y != 220 {
		t.Fatalf("non-dismissible open=%v rect=%v", ggui.Peek(open), s.Rect())
	}
	p.Key("escape")
	if ggui.Peek(open) {
		t.Fatal("Escape compatibility broken")
	}
}

func TestDrawerDragDoesNotStealContentAndCanBeDisabled(t *testing.T) {
	t.Parallel()
	open := ggui.State(true)
	clicks := 0
	s := ui.Drawer(open, ui.Button("Save", func() { clicks++ })).Size(200)
	p := openSheet(t, s)
	p.Tap("Save")
	if clicks != 1 {
		t.Fatal("content button blocked")
	}
	s.Draggable(false)
	p.Frame()
	start := drawerGrip(s)
	p.Press(start)
	p.Release(start.Add(ggui.Pt(0, 180)))
	p.Frame()
	p.Advance(time.Second)
	if !ggui.Peek(open) || s.Rect().Origin.Y != 100 {
		t.Fatalf("disabled drag changed drawer: %v", s.Rect())
	}
	if len(rangeNodes(p)) != 0 {
		t.Fatal("disabled drag handle remains focusable")
	}
}

func TestDrawerSnapPointsClampToViewportAndRespectReducedMotion(t *testing.T) {
	t.Parallel()
	selected := ggui.State(1)
	s := ui.Drawer(ggui.State(true), ggui.Text("Content")).SnapPoints(80, 600).BindSnapPoint(selected)
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, s), ggui.Sz(400, 300))
	defer p.Close()
	p.Frame()
	if s.Rect().Origin.Y != 0 || s.Rect().Size.H != 300 {
		t.Fatalf("viewport: %v", s.Rect())
	}
	selected.Set(0)
	p.Frame()
	if s.Rect().Origin.Y != 220 {
		t.Fatalf("reduced motion: %v", s.Rect())
	}
}
