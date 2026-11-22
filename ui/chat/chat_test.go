package chat_test

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/chat"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

func TestAttachmentIndependentActionsAndStates(t *testing.T) {
	t.Parallel()
	state := ggui.State(chat.AttachmentIdle)
	opened, removed := 0, 0
	file := chat.Attachment("report.pdf", "PDF · 2.4 MB").Media(widget.Text("PDF")).BindState(state).
		Trigger("Preview report", func() { opened++ }).Actions(chat.AttachmentAction("Remove report", widget.Text("×"), func() { removed++ }))
	p := ggui.NewProbe(widget.Column(file), ggui.Sz(300, 120))
	defer p.Close()
	p.Tap("Remove report")
	if removed != 1 || opened != 0 {
		t.Fatal("action propagated to trigger", opened, removed)
	}
	p.Tap("Preview report")
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if opened != 2 {
		t.Fatal("trigger pointer/keyboard", opened)
	}
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeySpace)
	if removed != 2 {
		t.Fatal("action not independently focusable")
	}
	for _, s := range []chat.AttachmentState{chat.AttachmentIdle, chat.AttachmentUploading, chat.AttachmentProcessing, chat.AttachmentError, chat.AttachmentDone} {
		state.Set(s)
		n := node(t, p.Semantics(), ggui.RoleGroup, "report.pdf")
		if n.Description != string(s) {
			t.Fatal(n)
		}
		node(t, p.Semantics(), ggui.RoleText, "PDF · 2.4 MB")
	}
	file.State(chat.AttachmentDone)
	state.Set(chat.AttachmentError)
	if node(t, p.Semantics(), ggui.RoleGroup, "report.pdf").Description != "done" {
		t.Fatal("State did not replace reader")
	}
}
func TestAttachmentSizesLongNamesAndGroup(t *testing.T) {
	t.Parallel()
	name := strings.Repeat("긴파일명", 30) + ".pdf"
	heights := []float64{}
	for _, size := range []chat.AttachmentSize{chat.AttachmentDefault, chat.AttachmentSmall, chat.AttachmentExtraSmall} {
		a := chat.Attachment(name, "Long description").Media(widget.Text("F")).Size(size).Width(180)
		heights = append(heights, a.Layout(ggui.Loose(ggui.Sz(180, 400)), ggui.Env{}).H)
		p := ggui.NewProbe(widget.Column(a), ggui.Sz(180, 150))
		node(t, p.Semantics(), ggui.RoleText, name)
		p.Close()
	}
	if !(heights[0] > heights[1] && heights[1] > heights[2]) {
		t.Fatal("size scale", heights)
	}
	cards := []*chat.AttachmentWidget{}
	for i := range 4 {
		cards = append(cards, chat.Attachment(fmt.Sprintf("File %d", i), "PDF").Width(180).Trigger(fmt.Sprintf("Open %d", i), func() {}))
	}
	group := chat.AttachmentGroup(cards...)
	p := ggui.NewProbe(widget.Column(group), ggui.Sz(230, 140))
	defer p.Close()
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeyArrowRight)
	if group.Position() != 192 {
		t.Fatal("keyboard must align next card", group.Position())
	}
	p.Type(ggui.Mods{}, ggui.KeyHome)
	p.Scroll(ggui.Pt(100, 20), ggui.Pt(-8, 0))
	p.Advance(200 * time.Millisecond)
	if group.Position() != 192 {
		t.Fatal("wheel must settle at snap point", group.Position())
	}
	p.Type(ggui.Mods{}, ggui.KeyHome)
	for range 4 {
		p.Type(ggui.Mods{}, ggui.KeyTab)
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

// A vertical attachment is shadcn's w-24, 96 across, and
// has-data-[slot=attachment-content]:w-30, 120, once it has a title or a
// description: media alone is a square card with nothing kept under it.
func TestVerticalAttachmentIsNarrowerWithNoText(t *testing.T) {
	t.Parallel()
	env := uitheme.Default().Apply(ggui.Env{})
	c := ggui.Loose(ggui.Sz(400, 400))
	icon := func() ggui.Widget { return ui.Icon(icons.File) }
	named := chat.Attachment("report.pdf", "2 MB").Media(icon()).Vertical().Layout(c, env)
	bare := chat.Attachment("", "").Media(icon()).Vertical().Layout(c, env)
	if named.W != 120 || bare.W != 96 {
		t.Fatalf("a vertical attachment is %v wide with text and %v without, want 120 and 96", named.W, bare.W)
	}
	if bare.H != bare.W {
		t.Fatalf("media alone makes a card %v, want it square", bare)
	}
}

func TestBubbleMessageAndMarkerComposition(t *testing.T) {
	t.Parallel()
	pressed, reactions := 0, 0
	b := chat.Bubble(widget.Text("Choose this reply")).End().Outline().Action("Reply", func() { pressed++ }).Reactions(ui.Button("Like", func() { reactions++ }).Ghost()).ReactionsTop()
	msg := chat.Message(b).Avatar(ui.Avatar("Ada").Size(32)).Header(widget.Text("Ada")).Footer(ui.Button("Retry", func() {}).Ghost()).End()
	p := ggui.NewProbe(widget.Padding(widget.Column(msg, chat.Marker(widget.Text("Today")).Separator(), chat.Marker(widget.Text("Saved")).Border()).Gap(30), 24), ggui.Sz(500, 300))
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

func transcript(n int) []chat.MessageEntry {
	out := []chat.MessageEntry{}
	for i := range n {
		out = append(out, chat.MessageEntry{ID: fmt.Sprint(i), Content: widget.Box(widget.Text(fmt.Sprint(i))).Height(80)})
	}
	return out
}
func TestMessageScrollerOpeningFollowingAndReaderIntent(t *testing.T) {
	t.Parallel()
	rows := ggui.State(transcript(6))
	s := chat.MessageScroller(rows).Height(200).Gap(10).AutoScroll(true).Animation(0)
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
	t.Parallel()
	rows := ggui.State(transcript(6))
	s := chat.MessageScroller(rows).Height(200).Gap(10).Opening(chat.ScrollStart).Animation(0)
	p := ggui.NewProbe(s, ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
	s.ScrollToMessage("2", chat.ScrollAlignStart)
	p.Frame()
	saved := s.Save()
	data := append([]chat.MessageEntry{{ID: "older", Content: widget.Box().Height(100)}}, ggui.Untrack(rows.Get)...)
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
	data = append(data, chat.MessageEntry{ID: "turn", Content: widget.Box(widget.Text("new turn")).Height(40), Anchor: true})
	rows.Set(data)
	p.Frame()
	v := s.Visibility()
	if v.CurrentAnchorID != "turn" {
		t.Fatal("new turn not anchored", v)
	}
	s.ScrollToMessage("missing", chat.ScrollAlignCenter)
	data = append(data, chat.MessageEntry{ID: "missing", Content: widget.Box().Height(60)})
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
	return slices.Contains(xs, s)
}

func TestQuestionnaireValidationSkipSubmissionAndReset(t *testing.T) {
	t.Parallel()
	answers := ggui.State(chat.QuestionAnswers{})
	submits := 0
	q := chat.Questionnaire(answers,
		chat.Question{Name: "one", Title: "Choose a task", Required: true, Choices: []chat.QuestionOption{{Value: "a", Label: "Task A"}, {Value: "x", Label: "Unavailable", Disabled: true}}},
		chat.Question{Name: "two", Title: "Choose extras", Multiple: true, Choices: []chat.QuestionOption{{Value: "b", Label: "Extra B"}, {Value: "c", Label: "Extra C"}}},
		chat.Question{Name: "three", Title: "Add context", Required: true, InputLabel: "Context", Placeholder: "Details"},
	).OnSubmit(func(a chat.QuestionAnswers) {
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
	if q.Status("two") != chat.QuestionSkipped {
		t.Fatal("skip not recorded")
	}
	p.Tap("Submit")
	if submits != 0 {
		t.Fatal("submitted unanswered last step")
	}
	p.Tap("Context")
	p.Clipboard().Write("A useful answer")
	p.Key("cmd+v")
	p.Tap("Submit")
	if submits != 1 {
		t.Fatal("valid submission not delivered")
	}
	q.Reset()
	p.Frame()
	if len(ggui.Untrack(answers.Get)) != 0 || q.Error("three") != "" {
		t.Fatal("reset did not restore initial state")
	}
	if current, total := q.Progress(); current != 1 || total != 3 {
		t.Fatal(current, total)
	}
}
func TestQuestionnaireKeyboardMultipleControlledAndDisabled(t *testing.T) {
	t.Parallel()
	answers := ggui.State(chat.QuestionAnswers{})
	active := ggui.State("multi")
	items := []chat.Question{{Name: "single", Title: "Single", Required: true, Choices: []chat.QuestionOption{{Value: "a", Label: "A"}}}, {Name: "multi", Title: "Multiple", Required: true, Multiple: true, Choices: []chat.QuestionOption{{Value: "b", Label: "B"}, {Value: "off", Label: "Off", Disabled: true}, {Value: "c", Label: "C"}}}, {Name: "disabled", Title: "Hidden", Disabled: true}}
	q := chat.Questionnaire(answers, items...).BindActive(active).Shortcuts(chat.QuestionNumbers)
	p := ggui.NewProbe(q, ggui.Sz(400, 400))
	defer p.Close()
	p.Tap("B")
	p.Type(ggui.Mods{}, ggui.KeyDigit2)
	if len(ggui.Untrack(answers.Get)["multi"].Values) != 2 {
		t.Fatal("multiple shortcut must skip disabled choices", ggui.Untrack(answers.Get))
	}
	p.Tap("Submit")
	p.Frame()
	if ggui.Untrack(active.Get) != "single" {
		t.Fatal("submit did not return to first invalid step")
	}
	p.Tap("A")
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if ggui.Untrack(active.Get) != "multi" {
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
	t.Parallel()
	answers := ggui.State(chat.QuestionAnswers{"one": {Values: []string{"a"}}})
	submits := 0
	q := chat.Questionnaire(answers, chat.Question{Name: "one", Title: "Answer", Required: true, Choices: []chat.QuestionOption{{Value: "a", Label: "Fixed"}}, InputLabel: "Custom"}).OnSubmit(func(chat.QuestionAnswers) { submits++ })
	p := ggui.NewProbe(q, ggui.Sz(360, 360))
	defer p.Close()
	p.Frame()
	q.SetError("one", "Rejected by host")
	p.Tap("Submit")
	if submits != 0 {
		t.Fatal("host error bypassed")
	}
	p.Tap("Custom")
	p.Clipboard().Write("Custom answer")
	p.Key("cmd+v")
	if a := ggui.Untrack(answers.Get)["one"]; len(a.Values) != 0 || a.Text != "Custom answer" {
		t.Fatal("single freeform did not replace fixed choice", a)
	}
	p.Key("cmd+enter")
	if submits != 1 {
		t.Fatal("command enter not submitted")
	}
	q.Reset()
	p.Frame()
	if a := ggui.Untrack(answers.Get)["one"]; len(a.Values) != 1 || a.Values[0] != "a" || a.Text != "" {
		t.Fatal("default answer not restored", a)
	}
}

func TestMessageScrollerNoImplicitFollowAndReducedMotion(t *testing.T) {
	t.Parallel()
	rows := ggui.State(transcript(6))
	s := chat.MessageScroller(rows).Gap(10)
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
	s.ScrollToMessage("future", chat.ScrollAlignStart)
	s.ScrollToEnd()
	rows.Set(append(transcript(7), chat.MessageEntry{ID: "future", Content: widget.Box().Height(40)}))
	p.Frame()
	// The later edge command must cancel a pending missing-message command.
	if s.Position() != 420 {
		t.Fatal("obsolete queued jump applied", s.Position())
	}
}

func TestMessageScrollerRestoreReplacesQueuedJump(t *testing.T) {
	t.Parallel()
	rows := ggui.State(transcript(8))
	s := chat.MessageScroller(rows).Gap(10).Animation(0)
	p := ggui.NewProbe(s, ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
	s.ScrollToStart()
	p.Frame()
	saved := s.Save()
	s.ScrollToMessage("future", chat.ScrollAlignStart)
	s.Restore(saved)
	rows.Set(append(transcript(8), chat.MessageEntry{ID: "future", Content: widget.Box().Height(40)}))
	p.Frame()
	if s.Position() != 0 {
		t.Fatal("queued jump overrode restored position", s.Position())
	}
}

func TestAttachmentGroupTouchPixelsAndMomentum(t *testing.T) {
	t.Parallel()
	group := chat.AttachmentGroup(
		chat.Attachment("One", "PDF").Width(180),
		chat.Attachment("Two", "PDF").Width(180),
		chat.Attachment("Three", "PDF").Width(180),
	)
	p := ggui.NewProbe(widget.Column(group), ggui.Sz(230, 140))
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

func TestChatWidgetsFollowStringsLocalization(t *testing.T) {
	t.Parallel()
	ko := ui.Strings{
		AttachmentsName:      "첨부 파일",
		AttachmentStatusDone: "완료",
		ConversationName:     "대화",
		TraceName:            "실행 추적",
		TraceStatusRunning:   "실행 중",
		NoQuestions:          "질문 없음",
		NoQuestionsDetail:    "활성화된 질문이 없습니다.",
	}
	lang := ggui.State(ko)
	root := ui.BindStrings(lang, widget.Column(
		chat.AttachmentGroup(chat.Attachment("a.txt", "")),
		chat.MessageScroller(ggui.State([]chat.MessageEntry{{ID: "1", Content: widget.Text("hi")}})).Height(80),
		chat.Trace(chat.TraceStep{ID: "run", Title: "Run", Status: chat.TraceRunning}),
		chat.Questionnaire(ggui.State(chat.QuestionAnswers{})),
	))
	p := ggui.NewProbe(root, ggui.Sz(600, 600))
	defer p.Close()
	p.Frame()
	tree := p.Semantics()
	node(t, tree, ggui.RoleGroup, "첨부 파일")
	if n := node(t, tree, ggui.RoleGroup, "a.txt"); n.Description != "완료" {
		t.Fatalf("attachment state description = %q, want 완료", n.Description)
	}
	node(t, tree, ggui.RoleGroup, "대화")
	node(t, tree, ggui.RoleList, "실행 추적")
	node(t, tree, ggui.RoleStatus, "Run: 실행 중")
	node(t, tree, ggui.RoleText, "질문 없음")
	node(t, tree, ggui.RoleText, "활성화된 질문이 없습니다.")

	lang.Set(ui.DefaultStrings())
	p.Frame()
	tree = p.Semantics()
	node(t, tree, ggui.RoleText, "No questions")
	node(t, tree, ggui.RoleText, "There are no enabled questions.")
}

func TestChatIdentityAndNilBindingRejection(t *testing.T) {
	t.Parallel()
	name := ggui.State("First")
	b := chat.Bubble(widget.Text("hello")).Action("First", func() {}).Key("bubble-1").BindName(name)
	ag := chat.AttachmentGroup(chat.Attachment("a.txt", "1 KB")).Key("group-1").BindName(name)
	ms := chat.MessageScroller(ggui.State([]chat.MessageEntry{{ID: "1", Content: widget.Text("hi")}})).Height(60).Key("scroll-1").BindName(name)
	q := chat.Questionnaire(ggui.State(chat.QuestionAnswers{}), chat.Question{Name: "q", Title: "Pick", Choices: []chat.QuestionOption{{Value: "a", Label: "A"}}}).Key("q-1").BindName(name)

	p := ggui.NewProbe(widget.Column(b, ag, ms, q), ggui.Sz(400, 500))
	defer p.Close()
	p.Frame()
	name.Set("Second")
	p.Frame()
	node(t, p.Semantics(), ggui.RoleButton, "Second")

	var nilState *ggui.StateValue[chat.AttachmentState]
	func() {
		defer func() {
			if recover() == nil {
				t.Error("BindState accepted typed nil")
			}
		}()
		chat.Attachment("file", "").BindState(nilState)
	}()
}
