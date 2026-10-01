package ui_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// buttonTranscript is n 80px rows ten apart, each holding a button named
// "Row i", so a test can focus or reveal a row.
func buttonTranscript(n int) []ui.MessageEntry {
	out := make([]ui.MessageEntry, n)
	for i := range out {
		name := fmt.Sprintf("Row %d", i)
		out[i] = ui.MessageEntry{ID: fmt.Sprint(i), Content: ggui.Box(ui.Button(name, func() {})).Height(80)}
	}
	return out
}

func TestMessageScrollerKeyboardScrollsAndClamps(t *testing.T) {
	t.Parallel()
	// Six 80px rows ten apart are 530 tall: 330 past a 200px viewport.
	s := ui.MessageScroller(ggui.State(transcript(6))).Height(200).Gap(10).Animation(0).Name("Chat")
	p := ggui.NewProbe(s, ggui.Sz(300, 200))
	defer p.Close()
	chat, ok := p.Semantics().Find(ggui.RoleGroup, "Chat")
	if !ok {
		t.Fatalf("no group named Chat:\n%s", p.Semantics())
	}
	p.Perform(chat.ID, ggui.Action{Kind: ggui.ActionFocus})
	for _, step := range []struct {
		key  ggui.KeyboardKey
		want float64
	}{
		{ggui.KeyHome, 0},
		{ggui.KeyArrowUp, 0},
		{ggui.KeyArrowDown, 40},
		{ggui.KeyPageDown, 220},
		{ggui.KeyPageDown, 330},
		{ggui.KeyArrowUp, 290},
		{ggui.KeyPageUp, 110},
		{ggui.KeyEnd, 330},
	} {
		p.Type(ggui.Mods{}, step.key)
		if got := s.Position(); got != step.want {
			t.Fatalf("after %v the transcript is at %v, want %v", step.key, got, step.want)
		}
	}
	if s.ConsumesKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyEnter}) || !s.ConsumesKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyPageUp}) {
		t.Fatal("the scroller consumes Enter or leaves PageUp to its parent")
	}
}

func TestMessageScrollerScrollMarginOffsetsMessageTargets(t *testing.T) {
	t.Parallel()
	s := ui.MessageScroller(ggui.State(transcript(8))).Height(200).Gap(10).Animation(0).ScrollMargin(20).Opening(ui.ScrollStart)
	p := ggui.NewProbe(s, ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
	s.ScrollToMessage("3", ui.ScrollAlignStart)
	p.Frame()
	// Row 3 starts at 270; the margin keeps 20px above it.
	if got := s.Position(); got != 250 {
		t.Fatalf("ScrollToMessage start with a 20px margin went to %v, want 250", got)
	}
	s.ScrollToMessage("5", ui.ScrollAlignEnd)
	p.Frame()
	// Row 5 ends at 530; its bottom sits 20px above the viewport's.
	if got := s.Position(); got != 530-200+20 {
		t.Fatalf("ScrollToMessage end with a 20px margin went to %v, want 350", got)
	}
	s.ScrollMargin(-5)
	s.ScrollToMessage("1", ui.ScrollAlignStart)
	p.Frame()
	if got := s.Position(); got != 90 {
		t.Fatalf("a negative margin was not treated as none: %v, want 90", got)
	}
}

func TestMessageScrollerNewAnchorLeavesAPeekOfThePreviousMessage(t *testing.T) {
	t.Parallel()
	rows := ggui.State(transcript(3))
	s := ui.MessageScroller(rows).Height(200).Gap(10).Animation(0).PreviousPeek(30)
	p := ggui.NewProbe(s, ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
	more := append(transcript(3), ui.MessageEntry{ID: "q", Anchor: true, Content: ggui.Box(ggui.Text("question")).Height(80)})
	rows.Set(more)
	p.Frame()
	// The new anchor starts at 270; 30px of the message before it stay in view.
	if got := s.Position(); got != 240 {
		t.Fatalf("a new anchor scrolled to %v, want 240", got)
	}
	if got := s.Visibility().CurrentAnchorID; got != "q" {
		t.Fatalf("current anchor is %q, want q", got)
	}
}

func TestMessageScrollerEdgeThresholdAndVisibilityCallback(t *testing.T) {
	t.Parallel()
	var seen []ui.TranscriptVisibility
	s := ui.MessageScroller(ggui.State(transcript(6))).Height(200).Gap(10).Animation(0).
		Opening(ui.ScrollStart).EdgeThreshold(50).OnVisibility(func(v ui.TranscriptVisibility) { seen = append(seen, v) })
	p := ggui.NewProbe(s, ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
	if len(seen) != 1 || !slices.Equal(seen[0].VisibleMessageIDs, []string{"0", "1", "2"}) || seen[0].CanScrollStart || !seen[0].CanScrollEnd {
		t.Fatalf("first visibility = %+v, want rows 0-2 and only CanScrollEnd", seen)
	}
	s.ScrollToMessage("0", ui.ScrollAlignStart)
	p.Frame()
	p.Frame()
	if len(seen) != 1 {
		t.Fatalf("OnVisibility ran again with nothing changed: %+v", seen)
	}
	chat, _ := p.Semantics().Find(ggui.RoleGroup, "Conversation")
	p.Perform(chat.ID, ggui.Action{Kind: ggui.ActionFocus})
	p.Type(ggui.Mods{}, ggui.KeyArrowDown)
	p.Frame() // visibility is measured where rows paint
	if v := s.Visibility(); v.CanScrollStart {
		t.Fatalf("40px from the start is within a 50px threshold, but CanScrollStart is set: %+v", v)
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowDown)
	p.Frame()
	if v := s.Visibility(); !v.CanScrollStart {
		t.Fatalf("80px from the start is past a 50px threshold, but CanScrollStart is clear: %+v", v)
	}
	if last := seen[len(seen)-1]; !last.CanScrollStart {
		t.Fatalf("OnVisibility missed the change: %+v", last)
	}
}

func TestMessageScrollerRevealsARowScrolledIntoView(t *testing.T) {
	t.Parallel()
	s := ui.MessageScroller(ggui.State(buttonTranscript(6))).Height(200).Gap(10).Animation(0)
	p := ggui.NewProbe(s, ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
	if got := s.Position(); got != 330 {
		t.Fatalf("opened at %v, want the end (330)", got)
	}
	row, ok := p.Semantics().Find(ggui.RoleButton, "Row 1")
	if !ok {
		t.Fatalf("row 1 is not in the tree:\n%s", p.Semantics())
	}
	p.Perform(row.ID, ggui.Action{Kind: ggui.ActionScrollIntoView})
	p.Frame()
	if got := s.Position(); got > 90 || got+200 < 170 {
		t.Fatalf("revealing row 1 (90..170) left the transcript at %v", got)
	}
}
