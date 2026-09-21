package ggui_test

import (
	"testing"

	"github.com/ironpark/ggui"
)

// This reader tracks through Get, but deliberately has no GetAny method.
type formattedReader struct{ source ggui.Readable[int] }

func (r formattedReader) Get() int     { return r.source.Get() }
func (formattedReader) String() string { return "custom reader" }

func TestSprintfCustomReadablePassesThroughWithoutGetAny(t *testing.T) {
	state := ggui.State(1)
	r := formattedReader{state}
	text := ggui.Sprintf("%v", r)
	defer text.Dispose()
	if got := text.Get(); got != "custom reader" {
		t.Fatalf("custom argument formatted as %q", got)
	}
	state.Set(2)
	if got := text.Get(); got != "custom reader" {
		t.Fatalf("bare reader was implicitly unwrapped: %q", got)
	}
}

func TestTextfAdaptsCustomReadableAndTracksLens(t *testing.T) {
	state := ggui.State(1)
	form := ggui.State(struct{ Name string }{"Ada"})
	reader := formattedReader{state}
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Textf("%d %s", ggui.Derived(reader.Get), form.Field(func(f *struct{ Name string }) *string { return &f.Name }))
	}, ggui.Sz(300, 50))
	defer p.Close()
	if _, ok := p.Semantics().Find(ggui.RoleText, "1 Ada"); !ok {
		t.Fatal("adapted reader or lens was not formatted")
	}
	state.Set(2)
	form.Set(struct{ Name string }{"Grace"})
	if _, ok := p.Semantics().Find(ggui.RoleText, "2 Grace"); !ok {
		t.Fatal("formatted value did not follow both sources")
	}
}
