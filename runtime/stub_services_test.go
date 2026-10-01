package runtime

import (
	"errors"
	"slices"
	"sync"
	"testing"
)

func TestMemoryClipboardKeepsTheLastWrite(t *testing.T) {
	t.Parallel()
	var c Clipboard = &MemoryClipboard{}
	if got := c.Read(); got != "" {
		t.Fatalf("a new clipboard holds %q", got)
	}
	c.Write("한글\r\nline")
	if got := c.Read(); got != "한글\r\nline" {
		t.Fatalf("Read = %q, want the text written, byte for byte", got)
	}
	// Probes on goroutines of their own may share one.
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for range 100 {
				c.Write(string(rune('a' + i)))
				_ = c.Read()
			}
		})
	}
	wg.Wait()
	if got := c.Read(); len(got) != 1 || got[0] < 'a' || got[0] > 'h' {
		t.Fatalf("after concurrent writes the clipboard holds %q, want one of them whole", got)
	}
}

func TestStubMessageErrWinsAndIsStillRecorded(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	s := &StubFilePicker{Button: 2, Err: boom}
	if i, err := s.Message(Message{Title: "Save?"}); i != 0 || !errors.Is(err, boom) {
		t.Fatalf("Message = %d, %v; want 0, boom", i, err)
	}
	if len(s.Messages) != 1 {
		t.Fatalf("a failed message was not recorded: %v", s.Messages)
	}
}

func TestNativeDialogsAreThePlatformsForTheWindow(t *testing.T) {
	t.Parallel()
	if _, ok := NativeDialogs().(nativePicker); !ok {
		t.Fatalf("NativeDialogs() = %T, want nativePicker", NativeDialogs())
	}
	calls := 0
	d := NativeDialogsForWindow(func() uintptr { calls++; return 7 })
	p, ok := d.(nativePicker)
	if !ok || p.window == nil || p.window() != 7 {
		t.Fatalf("NativeDialogsForWindow did not keep the window getter: %#v", d)
	}
	if calls != 1 {
		t.Fatalf("the window getter ran %d times before any dialog, want only when asked", calls-1)
	}
}

func TestKdialogMessageArgsByButtonCountAndKind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		m    Message
		want []string
	}{
		{Message{Kind: MessageInfo, Title: "Done"}, []string{"--msgbox", "Done", "--title", "Done", "--ok-label", "OK"}},
		{Message{Kind: MessageWarning, Title: "Low", Detail: "Disk is low."}, []string{"--sorry", "Low\n\nDisk is low.", "--title", "Low", "--ok-label", "OK"}},
		{Message{Kind: MessageError, Title: "Failed", Buttons: []string{"Close"}}, []string{"--error", "Failed", "--title", "Failed", "--ok-label", "Close"}},
		{Message{Kind: MessageError, Title: "Retry?", Buttons: []string{"Retry", "Give up"}},
			[]string{"--warningyesno", "Retry?", "--title", "Retry?", "--yes-label", "Retry", "--no-label", "Give up"}},
		{Message{Kind: MessageQuestion, Title: "Save?", Buttons: []string{"Save", "Discard", "Cancel", "Ignored"}},
			[]string{"--yesnocancel", "Save?", "--title", "Save?", "--yes-label", "Save", "--no-label", "Discard", "--cancel-label", "Cancel"}},
		{Message{Kind: MessageWarning, Title: "Quit?", Buttons: []string{"Save", "Discard", "Cancel"}},
			[]string{"--warningyesnocancel", "Quit?", "--title", "Quit?", "--yes-label", "Save", "--no-label", "Discard", "--cancel-label", "Cancel"}},
	}
	for _, c := range cases {
		if got := kdialogMessageArgs(c.m); !slices.Equal(got, c.want) {
			t.Errorf("kdialogMessageArgs(%+v) = %q, want %q", c.m, got, c.want)
		}
	}
}

func TestGlobsJoinExtensionsOrAdmitAll(t *testing.T) {
	t.Parallel()
	if got := globs(FileFilter{Name: "All"}, " "); got != "*" {
		t.Fatalf("a filter with no extensions = %q, want *", got)
	}
	if got := globs(FileFilter{Name: "Images", Extensions: []string{"png", "jpg"}}, ";"); got != "*.png;*.jpg" {
		t.Fatalf("globs = %q, want *.png;*.jpg", got)
	}
}
