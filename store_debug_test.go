//go:build ggui_debug

package ggui

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// A debug build reports a model changed with nothing published, once, and
// stays quiet about one changed through Update.
func TestStoreReportsAChangeLeftUnpublished(t *testing.T) {
	var out bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&out)
	defer log.SetOutput(prev)

	store := NewStore(&storeModel{title: "a"})
	title := Select(store, func(m *storeModel) string { return m.title })
	p := ProbeBuilder(func() Widget { return TextOf(title) }, Sz(100, 20))
	defer p.Close()
	p.Frame()
	store.Update(func(m *storeModel) { m.title = "b" })
	p.Frame()
	if out.Len() != 0 {
		t.Fatalf("reported a published change:\n%s", out.String())
	}
	store.Model().title = "c"
	p.Frame()
	p.Frame()
	if n := strings.Count(out.String(), "changed without being published"); n != 1 {
		t.Fatalf("logged %d reports, want 1:\n%s", n, out.String())
	}
	if !strings.Contains(out.String(), "store_debug_test.go") {
		t.Errorf("the report does not name the Select:\n%s", out.String())
	}
}
