package ui_test

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestAttachmentIndependentActionsAndStates(t *testing.T) {
	state := ggui.State(ui.AttachmentIdle)
	opened, removed := 0, 0
	file := ui.Attachment("report.pdf", "PDF · 2.4 MB").Media(ggui.Text("PDF")).StateOf(state).
		Trigger("Preview report", func() { opened++ }).Actions(ui.AttachmentAction("Remove report", ggui.Text("×"), func() { removed++ }))
	p := ggui.NewProbe(ggui.Column(file), ggui.Sz(300, 120))
	defer p.Close()
	p.Tap("Remove report")
	if removed != 1 || opened != 0 {
		t.Fatal("action propagated to trigger", opened, removed)
	}
	p.Tap("Preview report")
	p.Type(ggui.Mods{}, ebiten.KeyEnter)
	if opened != 2 {
		t.Fatal("trigger pointer/keyboard", opened)
	}
	p.Type(ggui.Mods{}, ebiten.KeyTab, ebiten.KeySpace)
	if removed != 2 {
		t.Fatal("action not independently focusable")
	}
	for _, s := range []ui.AttachmentState{ui.AttachmentIdle, ui.AttachmentUploading, ui.AttachmentProcessing, ui.AttachmentError, ui.AttachmentDone} {
		state.Set(s)
		n := node(t, p.Semantics(), ggui.RoleGroup, "report.pdf")
		if n.Description != string(s) {
			t.Fatal(n)
		}
		node(t, p.Semantics(), ggui.RoleText, "PDF · 2.4 MB")
	}
	file.State(ui.AttachmentDone)
	state.Set(ui.AttachmentError)
	if node(t, p.Semantics(), ggui.RoleGroup, "report.pdf").Description != "done" {
		t.Fatal("State did not replace reader")
	}
}
func TestAttachmentSizesLongNamesAndGroup(t *testing.T) {
	name := strings.Repeat("긴파일명", 30) + ".pdf"
	heights := []float64{}
	for _, size := range []ui.AttachmentSize{ui.AttachmentDefault, ui.AttachmentSmall, ui.AttachmentExtraSmall} {
		a := ui.Attachment(name, "Long description").Media(ggui.Text("F")).Size(size).Width(180)
		heights = append(heights, a.Layout(ggui.Loose(ggui.Sz(180, 400)), ggui.Env{}).H)
		p := ggui.NewProbe(ggui.Column(a), ggui.Sz(180, 150))
		node(t, p.Semantics(), ggui.RoleText, name)
		p.Close()
	}
	if !(heights[0] > heights[1] && heights[1] > heights[2]) {
		t.Fatal("size scale", heights)
	}
	cards := []*ui.AttachmentWidget{}
	for i := range 4 {
		cards = append(cards, ui.Attachment(fmt.Sprintf("File %d", i), "PDF").Width(180).Trigger(fmt.Sprintf("Open %d", i), func() {}))
	}
	group := ui.AttachmentGroup(cards...)
	p := ggui.NewProbe(ggui.Column(group), ggui.Sz(230, 140))
	defer p.Close()
	p.Type(ggui.Mods{}, ebiten.KeyTab, ebiten.KeyArrowRight)
	if group.Position() != 192 {
		t.Fatal("keyboard must align next card", group.Position())
	}
	p.Type(ggui.Mods{}, ebiten.KeyHome)
	p.Scroll(ggui.Pt(100, 20), ggui.Pt(-8, 0))
	p.Advance(200 * time.Millisecond)
	if group.Position() != 192 {
		t.Fatal("wheel must settle at snap point", group.Position())
	}
	p.Type(ggui.Mods{}, ebiten.KeyHome)
	for range 4 {
		p.Type(ggui.Mods{}, ebiten.KeyTab)
	}
	if group.Position() == 0 {
		t.Fatal("Tab did not reveal offscreen actions")
	}
	p.Resize(ggui.Sz(1000, 140))
	p.Frame()
	if group.Position() != 0 {
		t.Fatal("resize did not clamp offset")
	}
}
func TestBubbleMessageAndMarkerComposition(t *testing.T) {
	pressed, reactions := 0, 0
	b := ui.Bubble(ggui.Text("Choose this reply")).End().Outline().Action("Reply", func() { pressed++ }).Reactions(ui.Button("Like", func() { reactions++ }).Ghost()).ReactionsTop()
	msg := ui.Message(b).Avatar(ui.Avatar("Ada").Size(32)).Header(ggui.Text("Ada")).Footer(ui.Button("Retry", func() {}).Ghost()).End()
	p := ggui.NewProbe(ggui.Padding(ggui.Column(msg, ui.Marker(ggui.Text("Today")).Separator(), ui.Marker(ggui.Text("Saved")).Border()).Gap(30), 24), ggui.Sz(500, 300))
	defer p.Close()
	p.Tap("Reply")
	p.Tap("Like")
	if pressed != 1 || reactions != 1 {
		t.Fatal("reaction and bubble hit separation", pressed, reactions)
	}
	tree := p.Semantics()
	avatar := node(t, tree, ggui.RoleImage, "Ada")
	reply := node(t, tree, ggui.RoleButton, "Reply")
	retry := node(t, tree, ggui.RoleButton, "Retry")
	if avatar.Rect.Origin.X <= reply.Rect.Origin.X || avatar.Rect.Origin.Y+avatar.Rect.Size.H > retry.Rect.Origin.Y {
		t.Fatal("avatar does not rest above end-aligned footer")
	}
	node(t, tree, ggui.RoleText, "Today")
	node(t, tree, ggui.RoleText, "Saved")
}

func transcript(n int) []ui.MessageEntry {
	out := []ui.MessageEntry{}
	for i := range n {
		out = append(out, ui.MessageEntry{ID: fmt.Sprint(i), Content: ggui.Box(ggui.Text(fmt.Sprint(i))).Height(80)})
	}
	return out
}
func TestMessageScrollerOpeningFollowingAndReaderIntent(t *testing.T) {
	rows := ggui.State(transcript(6))
	s := ui.MessageScroller(rows).Height(200).Gap(10).AutoScroll(true).Animation(0)
	p := ggui.NewProbe(s, ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
	if s.Position() != 330 {
		t.Fatal("opening end", s.Position())
	}
	rows.Set(transcript(7))
	p.Frame()
	if s.Position() != 420 {
		t.Fatal("not following appended output", s.Position())
	}
	p.Scroll(ggui.Pt(10, 100), ggui.Pt(0, 4))
	held := s.Position()
	rows.Set(transcript(8))
	p.Frame()
	if s.Position() != held {
		t.Fatal("scrolling away must stop following")
	}
	s.ScrollToEnd()
	p.Frame()
	rows.Set(transcript(9))
	p.Frame()
	if s.Position() != 600 {
		t.Fatal("explicit end did not resume following", s.Position())
	}
	// A click on a descendant must pause following, even when it consumes input.
	data := transcript(9)
	data[8].Content = ui.Button("Select text action", func() {})
	rows.Set(data)
	p.Frame()
	s.ScrollToEnd()
	p.Frame()
	p.Tap("Select text action")
	held = s.Position()
	rows.Set(append(data, transcript(10)[9]))
	p.Frame()
	if s.Position() != held {
		t.Fatal("descendant interaction did not pause")
	}
}
func TestMessageScrollerHistoryAnchorsAndRestore(t *testing.T) {
	rows := ggui.State(transcript(6))
	s := ui.MessageScroller(rows).Height(200).Gap(10).Opening(ui.ScrollStart).Animation(0)
	p := ggui.NewProbe(s, ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
	s.ScrollToMessage("2", ui.ScrollAlignStart)
	p.Frame()
	saved := s.Save()
	data := append([]ui.MessageEntry{{ID: "older", Content: ggui.Box().Height(100)}}, rows.Peek()...)
	rows.Set(data)
	p.Frame()
	if got := s.Save(); got.MessageID != saved.MessageID || got.Offset != saved.Offset {
		t.Fatal("prepend shifted reading position", got, saved)
	}
	s.ScrollToStart()
	p.Frame()
	s.Restore(saved)
	p.Frame()
	if s.Save().MessageID != saved.MessageID {
		t.Fatal("restore lost row")
	}
	data = append(data, ui.MessageEntry{ID: "turn", Content: ggui.Box(ggui.Text("new turn")).Height(40), Anchor: true})
	rows.Set(data)
	p.Frame()
	v := s.Visibility()
	if v.CurrentAnchorID != "turn" {
		t.Fatal("new turn not anchored", v)
	}
	s.ScrollToMessage("missing", ui.ScrollAlignCenter)
	data = append(data, ui.MessageEntry{ID: "missing", Content: ggui.Box().Height(60)})
	rows.Set(data)
	p.Frame()
	if !slicesContains(s.Visibility().VisibleMessageIDs, "missing") {
		t.Fatal("queued jump not resolved")
	}
	p.Resize(ggui.Sz(300, 1000))
	p.Frame()
	if math.IsNaN(s.Position()) || s.Position() < 0 {
		t.Fatal("invalid resized position")
	}
}
func slicesContains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func TestQuestionnaireValidationSkipSubmissionAndReset(t *testing.T) {
	answers := ggui.State(ui.QuestionAnswers{})
	submits := 0
	q := ui.Questionnaire(answers,
		ui.Question{Name: "one", Title: "Choose a task", Required: true, Choices: []ui.QuestionOption{{Value: "a", Label: "Task A"}, {Value: "x", Label: "Unavailable", Disabled: true}}},
		ui.Question{Name: "two", Title: "Choose extras", Multiple: true, Choices: []ui.QuestionOption{{Value: "b", Label: "Extra B"}, {Value: "c", Label: "Extra C"}}},
		ui.Question{Name: "three", Title: "Add context", Required: true, InputLabel: "Context", Placeholder: "Details"},
	).OnSubmit(func(a ui.QuestionAnswers) {
		submits++
		if len(a) != 2 || a["one"].Values[0] != "a" || a["three"].Text != "A useful answer" {
			t.Fatal("serialized answers", a)
		}
	})
	p := ggui.NewProbe(q, ggui.Sz(450, 450))
	defer p.Close()
	p.Tap("Next")
	if q.Error("one") == "" {
		t.Fatal("required validation missing")
	}
	p.Frame()
	if n, ok := p.Semantics().Focused(); !ok || n.Name != "Task A" {
		t.Fatal("validation did not focus answer", n)
	}
	p.Tap("Task A")
	p.Tap("Next")
	p.Tap("Skip")
	if q.Status("two") != ui.QuestionSkipped {
		t.Fatal("skip not recorded")
	}
	p.Tap("Submit")
	if submits != 0 {
		t.Fatal("submitted unanswered last step")
	}
	p.Tap("Context")
	clip := &ggui.MemoryClipboard{}
	clip.Write("A useful answer")
	ggui.SetClipboard(clip)
	p.Type(ggui.Mods{Meta: true}, ebiten.KeyV)
	p.Tap("Submit")
	if submits != 1 {
		t.Fatal("valid submission not delivered")
	}
	q.Reset()
	p.Frame()
	if len(answers.Peek()) != 0 || q.Error("three") != "" {
		t.Fatal("reset did not restore initial state")
	}
	if current, total := q.Progress(); current != 1 || total != 3 {
		t.Fatal(current, total)
	}
}
func TestQuestionnaireKeyboardMultipleControlledAndDisabled(t *testing.T) {
	answers := ggui.State(ui.QuestionAnswers{})
	active := ggui.State("multi")
	items := []ui.Question{{Name: "single", Title: "Single", Required: true, Choices: []ui.QuestionOption{{Value: "a", Label: "A"}}}, {Name: "multi", Title: "Multiple", Required: true, Multiple: true, Choices: []ui.QuestionOption{{Value: "b", Label: "B"}, {Value: "off", Label: "Off", Disabled: true}, {Value: "c", Label: "C"}}}, {Name: "disabled", Title: "Hidden", Disabled: true}}
	q := ui.Questionnaire(answers, items...).Active(active).Shortcuts(ui.QuestionNumbers)
	p := ggui.NewProbe(q, ggui.Sz(400, 400))
	defer p.Close()
	p.Tap("B")
	p.Type(ggui.Mods{}, ebiten.KeyDigit2)
	if len(answers.Peek()["multi"].Values) != 2 {
		t.Fatal("multiple shortcut must skip disabled choices", answers.Peek())
	}
	p.Tap("Submit")
	p.Frame()
	if active.Peek() != "single" {
		t.Fatal("submit did not return to first invalid step")
	}
	p.Tap("A")
	p.Type(ggui.Mods{}, ebiten.KeyEnter)
	if active.Peek() != "multi" {
		t.Fatal("Enter did not continue")
	}
	q.SetError("multi", "Choose fewer options")
	p.Frame()
	node(t, p.Semantics(), ggui.RoleText, "Choose fewer options")
	p.Tap("C")
	if q.Error("multi") != "" {
		t.Fatal("editing did not clear error")
	}
	q.SetItems(items[1])
	p.Frame()
	if _, total := q.Progress(); total != 1 {
		t.Fatal("conditional items not reflected")
	}
}

func TestQuestionnaireFreeformDefaultsAndExternalErrors(t *testing.T) {
	answers := ggui.State(ui.QuestionAnswers{"one": {Values: []string{"a"}}})
	submits := 0
	q := ui.Questionnaire(answers, ui.Question{Name: "one", Title: "Answer", Required: true, Choices: []ui.QuestionOption{{Value: "a", Label: "Fixed"}}, InputLabel: "Custom"}).OnSubmit(func(ui.QuestionAnswers) { submits++ })
	p := ggui.NewProbe(q, ggui.Sz(360, 360))
	defer p.Close()
	p.Frame()
	q.SetError("one", "Rejected by host")
	p.Tap("Submit")
	if submits != 0 {
		t.Fatal("host error bypassed")
	}
	p.Tap("Custom")
	clip := &ggui.MemoryClipboard{}
	clip.Write("Custom answer")
	ggui.SetClipboard(clip)
	p.Type(ggui.Mods{Meta: true}, ebiten.KeyV)
	if a := answers.Peek()["one"]; len(a.Values) != 0 || a.Text != "Custom answer" {
		t.Fatal("single freeform did not replace fixed choice", a)
	}
	p.Type(ggui.Mods{Meta: true}, ebiten.KeyEnter)
	if submits != 1 {
		t.Fatal("command enter not submitted")
	}
	q.Reset()
	p.Frame()
	if a := answers.Peek()["one"]; len(a.Values) != 1 || a.Values[0] != "a" || a.Text != "" {
		t.Fatal("default answer not restored", a)
	}
}

func TestMessageScrollerNoImplicitFollowAndReducedMotion(t *testing.T) {
	rows := ggui.State(transcript(6))
	s := ui.MessageScroller(rows).Gap(10)
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, s), ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
	old := s.Position()
	rows.Set(transcript(7))
	p.Frame()
	if s.Position() != old {
		t.Fatal("default must not auto-follow")
	}
	s.ScrollToStart()
	p.Frame()
	if s.Position() != 0 {
		t.Fatal("reduced motion did not jump")
	}
	s.ScrollToMessage("future", ui.ScrollAlignStart)
	s.ScrollToEnd()
	rows.Set(append(transcript(7), ui.MessageEntry{ID: "future", Content: ggui.Box().Height(40)}))
	p.Frame()
	// The later edge command must cancel a pending missing-message command.
	if s.Position() != 420 {
		t.Fatal("obsolete queued jump applied", s.Position())
	}
}

func TestMessageScrollerRestoreReplacesQueuedJump(t *testing.T) {
	rows := ggui.State(transcript(8))
	s := ui.MessageScroller(rows).Gap(10).Animation(0)
	p := ggui.NewProbe(s, ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
	s.ScrollToStart()
	p.Frame()
	saved := s.Save()
	s.ScrollToMessage("future", ui.ScrollAlignStart)
	s.Restore(saved)
	rows.Set(append(transcript(8), ui.MessageEntry{ID: "future", Content: ggui.Box().Height(40)}))
	p.Frame()
	if s.Position() != 0 {
		t.Fatal("queued jump overrode restored position", s.Position())
	}
}

func TestAttachmentGroupTouchPixelsAndMomentum(t *testing.T) {
	group := ui.AttachmentGroup(
		ui.Attachment("One", "PDF").Width(180),
		ui.Attachment("Two", "PDF").Width(180),
		ui.Attachment("Three", "PDF").Width(180),
	)
	p := ggui.NewProbe(ggui.Column(group), ggui.Sz(230, 140))
	defer p.Close()
	p.Frame()
	// A prior wheel snap must not fire while the finger holds the strip.
	group.HandlePointer(ggui.PointerEvent{Kind: ggui.PointerScroll, Scroll: ggui.Pt(-1, 0)})
	group.HandlePointer(ggui.PointerEvent{Kind: ggui.PointerScroll, Scroll: ggui.Pt(-15, 2), ScrollPixels: true})
	if group.Position() != 35 {
		t.Fatalf("touch distance was scaled: %v", group.Position())
	}
	p.Advance(200 * time.Millisecond)
	if group.Position() != 35 {
		t.Fatalf("touch hold snapped: %v", group.Position())
	}
	if group.HandlePointer(ggui.PointerEvent{Kind: ggui.PointerScroll, Scroll: ggui.Pt(0, -20), ScrollPixels: true}) || group.Position() != 35 {
		t.Fatal("vertical touch was consumed by horizontal strip")
	}
	momentum := ggui.PointerEvent{Kind: ggui.PointerScroll, Scroll: ggui.Pt(-10, 0), ScrollPixels: true, ScrollMomentum: true}
	if !group.HandlePointer(momentum) || group.Position() != 45 {
		t.Fatalf("momentum distance was scaled: %v", group.Position())
	}
	momentum.Scroll.X = -10000
	group.HandlePointer(momentum)
	if group.HandlePointer(momentum) {
		t.Fatal("momentum continued at the edge")
	}
	end := group.Position()
	p.Advance(200 * time.Millisecond)
	if group.Position() != end {
		t.Fatal("momentum scheduled a wheel snap")
	}
}
