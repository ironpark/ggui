package ggui

import (
	"errors"
	"testing"

	"github.com/ironpark/ggui/runtime"
)

// A Probe answers dialogs with a stub that cancels until its Paths are set,
// and records what was asked, so code written against Host runs headless.
func TestProbeDialogsAreAStub(t *testing.T) {
	p := NewProbe(Text("x"), Sz(10, 10))
	defer p.Close()
	var h Host = p
	if _, err := h.Dialogs().OpenFile(runtime.FileDialog{Title: "one"}); !errors.Is(err, runtime.ErrCanceled) {
		t.Fatalf("default stub err = %v, want ErrCanceled", err)
	}
	stub := p.Dialogs().(*runtime.StubFilePicker)
	if len(stub.Asked) != 1 || stub.Asked[0].Title != "one" {
		t.Fatalf("Asked = %+v", stub.Asked)
	}
	stub.Paths = []string{"/a"}
	if got, err := h.Dialogs().SaveFile(runtime.FileDialog{}); err != nil || got != "/a" {
		t.Fatalf("SaveFile = %q, %v", got, err)
	}
	if len(stub.Asked) != 2 {
		t.Fatalf("Asked = %+v, want both dialogs recorded", stub.Asked)
	}
}
