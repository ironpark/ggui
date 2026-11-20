//go:build !js

package textinput

import (
	"image"
	"testing"

	"github.com/ironpark/ggwd"
)

func TestTextBeforeFocusSeedsTheSameFrameSession(t *testing.T) {
	w := new(ggwd.Window)
	defer delete(windowInputs, w)
	if HandleEvent(ggwd.TextEvent{Window: w, Text: "t"}) {
		t.Fatal("text was consumed before an editable target existed")
	}
	input := inputFor(w)
	input.impl.enabled = true
	ch, _ := input.events.start()
	select {
	case s := <-ch:
		if s.Text != "t" || s.CommitKind != commitRegular {
			t.Fatalf("first input: %+v", s)
		}
	default:
		t.Fatal("first character disappeared while focus was being dispatched")
	}
}

func testWindowInput(t *testing.T, w *ggwd.Window) *textInput {
	t.Helper()
	input := inputFor(w)
	input.impl.enabled = true
	t.Cleanup(func() { delete(windowInputs, w) })
	return input
}

func TestNativeCommitCarriesNextKoreanComposition(t *testing.T) {
	w := new(ggwd.Window)
	input := testWindowInput(t, w)
	ch, _ := input.events.start()
	HandleEvent(ggwd.CompositionEvent{Window: w, Text: "한", Start: 3, End: 3})
	HandleEvent(ggwd.CompositionEvent{Window: w, Done: true})
	HandleEvent(ggwd.TextEvent{Window: w, Text: "한"})
	HandleEvent(ggwd.CompositionEvent{Window: w, Text: "ㄱ", Start: 3, End: 3})
	var states []textInputState
	for s := range ch {
		states = append(states, s)
	}
	if len(states) != 3 || states[0].Text != "한" || states[1].Text != "" || states[2].CommitKind != commitRegular {
		t.Fatalf("first session: %+v", states)
	}
	next, _ := input.events.start()
	select {
	case s := <-next:
		if s.Text != "ㄱ" || s.CommitKind != commitNone {
			t.Fatalf("next composition lost: %+v", s)
		}
	default:
		t.Fatal("next session has no composition")
	}
}

func TestNativeEventsStayWithOwningWindow(t *testing.T) {
	a, b := new(ggwd.Window), new(ggwd.Window)
	ia, ib := testWindowInput(t, a), testWindowInput(t, b)
	ca, _ := ia.events.start()
	cb, _ := ib.events.start()
	HandleEvent(ggwd.TextEvent{Window: b, Text: "乙"})
	HandleEvent(ggwd.TextEvent{Window: a, Text: "甲"})
	if got := <-ca; got.Text != "甲" {
		t.Fatalf("first window: %+v", got)
	}
	if got := <-cb; got.Text != "乙" {
		t.Fatalf("second window: %+v", got)
	}
}

func TestNativeReplacementIsRelativeToReceivingCaret(t *testing.T) {
	w := new(ggwd.Window)
	input := testWindowInput(t, w)
	ch, _ := input.events.start()
	HandleEvent(ggwd.TextEvent{Window: w, Text: "é", Replacement: &ggwd.TextRange{Start: -1, End: 0}})
	s := <-ch
	if !s.ReplacementRelativeToCaret || s.ReplacementStartInBytes != -1 || s.ReplacementEndInBytes != 0 {
		t.Fatalf("replacement: %+v", s)
	}
}

func TestNativeTargetWithoutEditorDoesNotConsumeText(t *testing.T) {
	w := new(ggwd.Window)
	input := inputFor(w)
	defer delete(windowInputs, w)
	if HandleEvent(ggwd.TextEvent{Window: w, Text: "x"}) {
		t.Fatal("unfocused target consumed text")
	}
	restore := WithWindow(w)
	restore()
	if len(input.events.queuedStates) != 0 {
		t.Fatal("unfocused target retained text after its frame")
	}
}

type fakeTextInputWindow struct {
	scale   float64
	rects   [][4]float64
	enabled bool
}

func (f *fakeTextInputWindow) DeviceScaleFactor() float64         { return f.scale }
func (f *fakeTextInputWindow) SetTextInputContext(string, string) {}
func (f *fakeTextInputWindow) SetTextInputEnabled(on bool)        { f.enabled = on }
func (f *fakeTextInputWindow) SetTextInputRect(x, y, w, h float64) {
	f.rects = append(f.rects, [4]float64{x, y, w, h})
}

func TestSetRectResendsWhenWindowScaleChanges(t *testing.T) {
	win := &fakeTextInputWindow{scale: 1}
	input := &textInput{}
	input.impl.events = &input.events
	input.impl.window = win
	useTextInput(t, input)

	bounds := image.Rect(10, 20, 12, 36)
	input.impl.Start(bounds, "", "")
	if len(win.rects) != 1 || win.rects[0] != [4]float64{10, 20, 2, 16} {
		t.Fatalf("initial Start rects = %v, want [[10 20 2 16]]", win.rects)
	}

	// Same logical bounds and same scale: must skip the main-thread hop.
	UpdateCaret(bounds)
	if len(win.rects) != 1 {
		t.Fatalf("unchanged caret+scale sent %d rects, want 1", len(win.rects))
	}

	// Scale changes while logical bounds stay identical: must re-send.
	win.scale = 1.5
	UpdateCaret(bounds)
	if len(win.rects) != 2 || win.rects[1] != [4]float64{10, 20, 2, 16} {
		t.Fatalf("after scale change 1->1.5 rects = %v, want 2 calls", win.rects)
	}

	// Repeated tick at the new scale with same bounds: must skip again.
	UpdateCaret(bounds)
	if len(win.rects) != 2 {
		t.Fatalf("steady 1.5x scale sent %d rects, want 2", len(win.rects))
	}

	// Start after another scale change also re-sends.
	win.scale = 2
	input.impl.Start(bounds, "", "")
	if len(win.rects) != 3 {
		t.Fatalf("Start after scale change 1.5->2 sent %d rects, want 3", len(win.rects))
	}

	// Moving the caret at the same scale still sends.
	moved := image.Rect(30, 40, 32, 56)
	UpdateCaret(moved)
	if len(win.rects) != 4 || win.rects[3] != [4]float64{30, 40, 2, 16} {
		t.Fatalf("moved caret rects = %v, want 4 calls ending in [30 40 2 16]", win.rects)
	}
}
