//go:build ggui_debug

package ggui

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// A debug build reports a Bind getter that reads no signal, once per call
// site, and stays quiet about one that reads state.
func TestBindReportsAGetterThatReadsNoSignal(t *testing.T) {
	var out bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&out)
	defer log.SetOutput(prev)

	type item struct{ Link bool }
	it := item{}
	on := State(false)
	stale := func() Binding[bool] { return Bind(func() bool { return it.Link }, func(v bool) { it.Link = v }) }
	live := Bind(on.Get, on.Set)
	for range 2 {
		b := stale()
		b.Get()
		b.Get()
	}
	live.Get()
	Controlled(3, func(int) {}).Get() // reads no signal on purpose
	if n := strings.Count(out.String(), "read no signal"); n != 1 {
		t.Fatalf("logged %d reports, want 1 for the one call site:\n%s", n, out.String())
	}
	if !strings.Contains(out.String(), "bind_debug_test.go") {
		t.Errorf("the report does not name the call site:\n%s", out.String())
	}
}
