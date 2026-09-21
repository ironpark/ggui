package main

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/runtime"
)

func newProbe(t *testing.T) (*model, *ggui.Probe) {
	t.Helper()
	m := newModel()
	p := ggui.ProbeBuilder(m.build, ggui.Sz(560, 420))
	t.Cleanup(p.Close)
	return m, p
}

// The dialogs are answered by a Stub, so the test never opens a window;
// the same picker is what an app's own tests would install.
func TestDialogsFillTheList(t *testing.T) {
	stub := &runtime.StubFilePicker{Paths: []string{"/pictures/a.png", "/pictures/b.png"}}
	runtime.SetFilePicker(stub)
	defer runtime.SetFilePicker(nil)
	m, p := newProbe(t)

	p.Tap("Open…")
	if got := ggui.Untrack(m.Files.Get); !slices.Equal(got, []string{"/pictures/a.png"}) {
		t.Fatalf("after Open: %q", got)
	}
	if asked := stub.Asked[0]; asked.Title != "Open a file" || len(asked.Filters) != 2 {
		t.Fatalf("Open asked with %+v", asked)
	}

	p.Tap("Open several…")
	if got := ggui.Untrack(m.Files.Get); len(got) != 3 {
		t.Fatalf("after OpenMultiple: %q", got)
	}

	stub.Paths = nil
	p.Tap("Save…")
	if got := ggui.Untrack(m.Status.Get); got != "Canceled." {
		t.Fatalf("status after cancel = %q", got)
	}
	if got := ggui.Untrack(m.Files.Get); len(got) != 3 {
		t.Fatalf("a cancel changed the list: %q", got)
	}

	p.Tap("Clear")
	if got := ggui.Untrack(m.Files.Get); len(got) != 0 {
		t.Fatalf("after Clear: %q", got)
	}
}

func TestDropOntoTheCard(t *testing.T) {
	m, p := newProbe(t)
	// The card fills the window above the buttons, so its middle is well
	// inside the zone; a plain text has no label a Find could locate.
	p.Drop(ggui.Pt(280, 150), fstest.MapFS{
		"notes.txt": {Data: []byte("hi")},
		"photos":    {Mode: fs.ModeDir},
	})
	got := ggui.Untrack(m.Files.Get)
	if len(got) != 2 || got[0] != "notes.txt" || !strings.HasPrefix(got[1], "photos") || !strings.HasSuffix(got[1], string(filepath.Separator)) {
		t.Fatalf("after drop: %q", got)
	}
	if status := ggui.Untrack(m.Status.Get); status != "Dropped 2 item(s)." {
		t.Fatalf("status = %q", status)
	}

	// A drop outside the card reaches no zone, and main registers no
	// App.OnDrop, so nothing changes.
	p.Drop(ggui.Pt(1, 1), fstest.MapFS{"stray": {}})
	if got := ggui.Untrack(m.Files.Get); len(got) != 2 {
		t.Fatalf("a drop outside the card was taken: %q", got)
	}
}
