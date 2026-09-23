package main

import (
	"testing"

	"github.com/ironpark/ggui"
)

// TestRouter taps through the sidebar and history buttons headlessly.
func TestRouter(t *testing.T) {
	r := routes()
	p := ggui.NewProbe(r.View(), ggui.Sz(820, 520))
	defer p.Close()
	p.Frame()

	steps := []struct{ tap, want string }{
		{"Project 1", "/projects/1"},
		{"Tasks", "/projects/1/tasks"},
		{"Open tasks", "/projects/1/tasks?filter=open"},
		{"Old home link", "/"},
		{"Broken link", "/nowhere"},
		{"Go home", "/"},
		{"‹ Back", "/nowhere"},
		{"Forward ›", "/"},
	}
	for _, s := range steps {
		p.Tap(s.tap)
		p.Frame()
		if got := ggui.Untrack(r.Location).String(); got != s.want {
			t.Fatalf("after tapping %q: %s, want %s", s.tap, got, s.want)
		}
	}
	if _, ok := p.Find("No page at /nowhere."); ok {
		t.Fatal("not-found screen still showing on /")
	}
}
