package ggui

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestDropGoesToTheZoneUnderTheCursor(t *testing.T) {
	var zone, host []string
	build := func() Widget {
		target := Pointer(Box(Text("drop here")).Size(200, 100)).OnDrop(func(ev DropEvent) {
			for _, f := range ev.Files {
				zone = append(zone, f.Name)
			}
		})
		return Column(target, Box(Text("elsewhere")).Size(200, 100))
	}
	p := ProbeBuilder(build, Sz(400, 400))
	defer p.Close()
	p.OnDrop(func(ev DropEvent) {
		for _, f := range ev.Files {
			host = append(host, f.Name)
		}
	})
	fsys := fstest.MapFS{
		"a.txt": {Data: []byte("hello")},
		"b.png": {Data: []byte{1, 2, 3}},
	}

	p.Drop(Pt(50, 50), fsys)
	if len(zone) != 2 || zone[0] != "a.txt" || zone[1] != "b.png" {
		t.Fatalf("zone got %q", zone)
	}
	if len(host) != 0 {
		t.Fatalf("host handler ran for a drop the zone took: %q", host)
	}

	p.Drop(Pt(50, 150), fsys)
	if len(host) != 2 {
		t.Fatalf("host got %q, want both files", host)
	}
	if len(zone) != 2 {
		t.Fatalf("zone took a drop outside it: %q", zone)
	}
}

func TestDroppedFileOpensWithoutAPath(t *testing.T) {
	var got string
	p := ProbeBuilder(func() Widget {
		return Pointer(Box(Text("zone")).Size(100, 100)).OnDrop(func(ev DropEvent) {
			f, err := ev.Files[0].Open()
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			b, _ := io.ReadAll(f)
			got = string(b)
			if ev.Files[0].Path != "" {
				t.Fatalf("in-memory drop has a Path %q", ev.Files[0].Path)
			}
		})
	}, Sz(200, 200))
	defer p.Close()
	p.Drop(Pt(10, 10), fstest.MapFS{"note.txt": {Data: []byte("contents")}})
	if got != "contents" {
		t.Fatalf("read %q", got)
	}
}

func TestDropPathsCarriesRealFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(file, []byte("on disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	var ev DropEvent
	p := ProbeBuilder(func() Widget { return Text("nothing accepts") }, Sz(200, 200))
	defer p.Close()
	p.OnDrop(func(e DropEvent) { ev = e })

	p.DropPaths(Pt(10, 10), file, sub)
	if len(ev.Files) != 2 {
		t.Fatalf("files = %+v", ev.Files)
	}
	if ev.Files[0].Name != "real.txt" || ev.Files[0].Path != file || ev.Files[0].Dir {
		t.Fatalf("file = %+v", ev.Files[0])
	}
	if ev.Files[1].Name != "sub" || !ev.Files[1].Dir {
		t.Fatalf("dir = %+v", ev.Files[1])
	}
	if paths := ev.Paths(); len(paths) != 2 || paths[0] != file {
		t.Fatalf("Paths = %q", paths)
	}
	f, err := ev.Files[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if b, _ := io.ReadAll(f); string(b) != "on disk" {
		t.Fatalf("read %q", b)
	}
}

func TestZoneWithoutOnDropLetsTheDropFallThrough(t *testing.T) {
	var outer, host int
	p := ProbeBuilder(func() Widget {
		inner := Pointer(Box(Text("tap only")).Size(50, 50)).OnTap(func() {})
		return Pointer(Box(inner).Size(200, 200)).OnDrop(func(DropEvent) { outer++ })
	}, Sz(200, 200))
	defer p.Close()
	p.OnDrop(func(DropEvent) { host++ })
	p.Drop(Pt(10, 10), fstest.MapFS{"x": {}})
	if outer != 1 || host != 0 {
		t.Fatalf("outer=%d host=%d; want the enclosing zone to take it", outer, host)
	}
}

func TestEmptyDropIsNotDispatched(t *testing.T) {
	var host int
	p := ProbeBuilder(func() Widget { return Text("x") }, Sz(100, 100))
	defer p.Close()
	p.OnDrop(func(DropEvent) { host++ })
	p.Drop(Pt(1, 1), fstest.MapFS{})
	if host != 0 {
		t.Fatal("a drop of nothing reached the host handler")
	}
}

func TestDragHoverFollowsTheZoneUnderTheCursor(t *testing.T) {
	var top, bottom []bool
	build := func() Widget {
		return Column(
			Pointer(Box(Text("top")).Size(200, 100)).OnDrop(func(DropEvent) {}).OnDropHover(func(over bool) { top = append(top, over) }),
			Pointer(Box(Text("bottom")).Size(200, 100)).OnDrop(func(DropEvent) {}).OnDropHover(func(over bool) { bottom = append(bottom, over) }),
			Pointer(Box(Text("plain")).Size(200, 100)).OnDrop(func(DropEvent) {}),
		)
	}
	p := ProbeBuilder(build, Sz(400, 400))
	defer p.Close()

	p.DragOver(Pt(50, 50))
	p.DragOver(Pt(60, 60))
	if len(top) != 1 || !top[0] {
		t.Fatalf("top zone heard %v after two frames over it, want one enter", top)
	}
	p.DragOver(Pt(50, 150))
	if len(top) != 2 || top[1] || len(bottom) != 1 || !bottom[0] {
		t.Fatalf("moving to the bottom zone: top heard %v, bottom heard %v", top, bottom)
	}
	p.DragOver(Pt(50, 250))
	if len(bottom) != 2 || bottom[1] {
		t.Fatalf("moving over a zone without OnDropHover: bottom heard %v, want an exit", bottom)
	}
	p.DragOver(Pt(50, 50))
	p.Drop(Pt(50, 50), fstest.MapFS{"a.txt": {Data: []byte("hi")}})
	if len(top) != 4 || !top[2] || top[3] {
		t.Fatalf("a drop should end the hover: top heard %v", top)
	}
	p.DragOver(Pt(50, 50))
	p.DragEnd()
	if len(top) != 6 || top[5] {
		t.Fatalf("an abandoned drag should end the hover: top heard %v", top)
	}
}
