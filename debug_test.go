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
	if !strings.Contains(out.String(), "debug_test.go") {
		t.Errorf("the report does not name the call site:\n%s", out.String())
	}
}

// A debug build reports a signal read while widgets are built, once per
// place, and stays quiet about a read in Untrack, in Reactive, in a
// computation the builder makes, and outside every builder.
func TestBuildersReportASnapshotRead(t *testing.T) {
	var out bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&out)
	defer log.SetOutput(prev)

	count := State(1)
	label := Map(count, func(n int) string { return "n" })
	count.Get()
	p := ProbeBuilder(func() Widget {
		Untrack(count.Get)
		return Column(
			Reactive(func() Widget { count.Get(); return Box() }),
			View(count, func(int) Widget { return Text("x") }),
			Component(func() Widget {
				for range 2 {
					count.Get() // reported once
				}
				return TextOf(label)
			}),
		)
	}, Size{W: 100, H: 100})
	defer p.Close()
	p.Frame()
	if n := strings.Count(out.String(), "reads a signal while building"); n != 1 {
		t.Fatalf("logged %d reports, want 1:\n%s", n, out.String())
	}
	if !strings.Contains(out.String(), "debug_test.go") {
		t.Errorf("the report does not name the read:\n%s", out.String())
	}
}
