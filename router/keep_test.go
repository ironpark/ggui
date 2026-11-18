package router

import (
	"encoding/json"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/devstate"
)

// Under ggui dev, a router that keeps its location mounts on the one handed
// over, replacing the history's first entry, and hands on where it is now.
// Not parallel: the session is the process's.
func TestKeepRestoresTheLocation(t *testing.T) {
	defer devstate.Begin(devstate.State{"nav": json.RawMessage(`"/a?tab=2"`)})()
	h := Memory("/")
	r := New(Page("/", text("home")), Page("/a", text("a")), Page("/b", text("b"))).History(h).Keep("nav")
	p := ggui.NewProbe(r.View(), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()

	if got := r.Location(); got.String() != "/a?tab=2" || got.Entry != h.Current().ID {
		t.Errorf("Location = %+v, want the kept /a?tab=2 on the current entry", got)
	}
	if _, n := h.Position(); n != 1 {
		t.Errorf("history has %d entries, want the first replaced", n)
	}
	if _, ok := p.Semantics().Find(ggui.RoleText, "a"); !ok {
		t.Error("the kept page is not showing in the first frame")
	}
	if err := r.Navigate("/b"); err != nil {
		t.Fatal(err)
	}
	if got := string(devstate.Snapshot()["nav"]); got != `"/b"` {
		t.Errorf("the router keeps %s, want the location it is on, \"/b\"", got)
	}
}

// Without ggui dev, Keep changes nothing.
func TestKeepWithoutDevStartsOnTheHistory(t *testing.T) {
	r := New(Page("/", text("home")), Page("/a", text("a"))).History(Memory("/a")).Keep("nav")
	p := ggui.NewProbe(r.View(), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()
	if got := r.Location().Path; got != "/a" {
		t.Errorf("Location.Path = %q, want the history's /a", got)
	}
	if r.kept != nil {
		t.Error("the router keeps a location without ggui dev")
	}
}
