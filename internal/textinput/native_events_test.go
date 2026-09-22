//go:build (darwin && !ios) || windows

package textinput

import (
	"testing"

	"github.com/ironpark/ggfx"
)

func TestTextBeforeFocusSeedsTheSameFrameSession(t *testing.T) {
	w := new(ggfx.Window)
	defer delete(windowInputs, w)
	if HandleEvent(ggfx.TextEvent{Window: w, Text: "t"}) {
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

func testWindowInput(t *testing.T, w *ggfx.Window) *textInput {
	t.Helper()
	input := inputFor(w)
	input.impl.enabled = true
	t.Cleanup(func() { delete(windowInputs, w) })
	return input
}

func TestNativeCommitCarriesNextKoreanComposition(t *testing.T) {
	w := new(ggfx.Window)
	input := testWindowInput(t, w)
	ch, _ := input.events.start()
	HandleEvent(ggfx.CompositionEvent{Window: w, Text: "한", Start: 3, End: 3})
	HandleEvent(ggfx.CompositionEvent{Window: w, Done: true})
	HandleEvent(ggfx.TextEvent{Window: w, Text: "한"})
	HandleEvent(ggfx.CompositionEvent{Window: w, Text: "ㄱ", Start: 3, End: 3})
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
	a, b := new(ggfx.Window), new(ggfx.Window)
	ia, ib := testWindowInput(t, a), testWindowInput(t, b)
	ca, _ := ia.events.start()
	cb, _ := ib.events.start()
	HandleEvent(ggfx.TextEvent{Window: b, Text: "乙"})
	HandleEvent(ggfx.TextEvent{Window: a, Text: "甲"})
	if got := <-ca; got.Text != "甲" {
		t.Fatalf("first window: %+v", got)
	}
	if got := <-cb; got.Text != "乙" {
		t.Fatalf("second window: %+v", got)
	}
}

func TestNativeReplacementIsRelativeToReceivingCaret(t *testing.T) {
	w := new(ggfx.Window)
	input := testWindowInput(t, w)
	ch, _ := input.events.start()
	HandleEvent(ggfx.TextEvent{Window: w, Text: "é", HasReplacement: true, ReplacementStart: -1, ReplacementEnd: 0})
	s := <-ch
	if !s.ReplacementRelativeToCaret || s.ReplacementStartInBytes != -1 || s.ReplacementEndInBytes != 0 {
		t.Fatalf("replacement: %+v", s)
	}
}

func TestNativeTargetWithoutEditorDoesNotConsumeText(t *testing.T) {
	w := new(ggfx.Window)
	input := inputFor(w)
	defer delete(windowInputs, w)
	if HandleEvent(ggfx.TextEvent{Window: w, Text: "x"}) {
		t.Fatal("unfocused target consumed text")
	}
	restore := WithWindow(w)
	restore()
	if len(input.events.queuedStates) != 0 {
		t.Fatal("unfocused target retained text after its frame")
	}
}
