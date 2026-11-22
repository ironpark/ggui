package chat_test

import (
	"fmt"
	"image"
	"math"
	"slices"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/chat"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// surveyQuestions is a three-step questionnaire: a required single choice
// with a disabled option and a freeform slot, a multiple choice, and an
// optional single choice.
func surveyQuestions() []chat.Question {
	return []chat.Question{
		{Name: "one", Title: "First", Required: true, InputLabel: "Other", Choices: []chat.QuestionOption{{Value: "a", Label: "Alpha"}, {Value: "x", Label: "Off", Disabled: true}, {Value: "b", Label: "Beta"}}},
		{Name: "two", Title: "Second", Multiple: true, Choices: []chat.QuestionOption{{Value: "c", Label: "Gamma"}, {Value: "d", Label: "Delta"}}},
		{Name: "three", Title: "Third", Choices: []chat.QuestionOption{{Value: "e", Label: "Epsilon"}}},
	}
}

func TestQuestionnaireCallbacksFollowNavigationAndStatus(t *testing.T) {
	t.Parallel()
	answers := ggui.State(chat.QuestionAnswers{})
	var items []string
	var statuses []string
	q := chat.Questionnaire(answers, surveyQuestions()...).Name("Survey").SubmitLabel("Send").
		OnItemChange(func(name string) { items = append(items, name) }).
		OnStatusChange(func(name string, s chat.QuestionStatus) { statuses = append(statuses, name+"="+string(s)) })
	p := ggui.NewProbe(q, ggui.Sz(450, 500))
	defer p.Close()
	if _, ok := p.Semantics().Find(ggui.RoleGroup, "Survey"); !ok {
		t.Fatalf("no group named Survey:\n%s", p.Semantics())
	}
	q.Previous() // nothing before the first step
	p.Tap("Alpha")
	p.Tap("Alpha") // already chosen: no status change
	p.Tap("Next")
	p.Tap("Previous")
	p.Tap("Next")
	p.Tap("Skip")
	if want := []string{"two", "one", "two", "three"}; !slices.Equal(items, want) {
		t.Fatalf("OnItemChange saw %q, want %q", items, want)
	}
	if want := []string{"one=answered", "two=skipped"}; !slices.Equal(statuses, want) {
		t.Fatalf("OnStatusChange saw %q, want %q", statuses, want)
	}
	if _, ok := p.FindRole(ggui.RoleButton, "Send"); !ok {
		t.Fatalf("the last step has no Send button:\n%s", p.Semantics())
	}
	if _, ok := p.FindRole(ggui.RoleButton, "Next"); ok {
		t.Fatal("the last step still offers Next")
	}
}

func TestQuestionnaireArrowsWalkChoicesIntoTheFreeformSlot(t *testing.T) {
	t.Parallel()
	answers := ggui.State(chat.QuestionAnswers{})
	q := chat.Questionnaire(answers, surveyQuestions()...)
	p := ggui.NewProbe(q, ggui.Sz(450, 500))
	defer p.Close()
	chosen := func() []string { return ggui.Untrack(answers.Get)["one"].Values }
	focused := func() string {
		n, _ := p.Semantics().Focused()
		return n.Name
	}
	p.Tap("Alpha")
	p.Type(ggui.Mods{}, ggui.KeyArrowDown)
	if got := chosen(); focused() != "Beta" || !slices.Equal(got, []string{"b"}) {
		t.Fatalf("Down from Alpha focused %q and chose %q; want Beta, skipping the disabled option", focused(), got)
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowDown)
	if focused() != "Other" {
		t.Fatalf("Down from the last choice focused %q, want the freeform field", focused())
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowUp)
	if focused() != "Beta" {
		t.Fatalf("Up from an empty freeform field focused %q, want the last choice", focused())
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowLeft)
	if got := chosen(); focused() != "Alpha" || !slices.Equal(got, []string{"a"}) {
		t.Fatalf("Left from Beta focused %q and chose %q; want Alpha", focused(), got)
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowUp)
	if focused() != "Other" {
		t.Fatalf("Up from the first choice focused %q, want it to wrap to the freeform field", focused())
	}
}

func TestQuestionnaireChoiceActionsSelectAndToggle(t *testing.T) {
	t.Parallel()
	answers := ggui.State(chat.QuestionAnswers{})
	q := chat.Questionnaire(answers, surveyQuestions()...).Active("two")
	p := ggui.NewProbe(q, ggui.Sz(450, 500))
	defer p.Close()
	act := func(name string, kind ggui.ActionSet) {
		t.Helper()
		n, ok := p.Semantics().Find(ggui.RoleCheckbox, name)
		if !ok {
			t.Fatalf("no checkbox %q:\n%s", name, p.Semantics())
		}
		p.Perform(n.ID, ggui.Action{Kind: kind})
	}
	values := func() []string { return ggui.Untrack(answers.Get)["two"].Values }
	act("Gamma", ggui.ActionSelect)
	act("Gamma", ggui.ActionSelect) // selecting a chosen option keeps it
	if got := values(); !slices.Equal(got, []string{"c"}) {
		t.Fatalf("select twice left %q, want [c]", got)
	}
	act("Delta", ggui.ActionPress)
	act("Gamma", ggui.ActionPress) // pressing toggles
	if got := values(); !slices.Equal(got, []string{"d"}) {
		t.Fatalf("press Delta, press Gamma left %q, want [d]", got)
	}
	if n, _ := p.Semantics().Find(ggui.RoleCheckbox, "Delta"); n.Checked != ggui.Tri(true) {
		t.Fatal("the chosen option is not described as checked")
	}
	q.Active("one")
	p.Frame()
	off, _ := p.Semantics().Find(ggui.RoleRadio, "Off")
	p.Perform(off.ID, ggui.Action{Kind: ggui.ActionPress})
	if got := ggui.Untrack(answers.Get)["one"].Values; len(got) != 0 {
		t.Fatalf("pressing a disabled option chose %q", got)
	}
}

func TestQuestionnaireButtonArrowsGoBackAndForth(t *testing.T) {
	t.Parallel()
	answers := ggui.State(chat.QuestionAnswers{"one": {Values: []string{"a"}}})
	active := ggui.State("two")
	q := chat.Questionnaire(answers, surveyQuestions()...).BindActive(active)
	p := ggui.NewProbe(q, ggui.Sz(450, 500))
	defer p.Close()
	p.Tap("Skip")
	if got := ggui.Untrack(active.Get); got != "three" {
		t.Fatalf("Skip went to %q, want three", got)
	}
	p.Tap("Previous")
	p.Frame()
	p.Tap("Previous")
	if got := ggui.Untrack(active.Get); got != "one" {
		t.Fatalf("Previous twice went to %q, want one", got)
	}
	p.Tap("Next")
	p.Frame()
	p.Tap("Next")
	// A focused navigation button takes Left and Right as Previous and Next.
	focusButton := func(name string) {
		t.Helper()
		n, ok := p.Semantics().Find(ggui.RoleButton, name)
		if !ok {
			t.Fatalf("no %s button:\n%s", name, p.Semantics())
		}
		p.Perform(n.ID, ggui.Action{Kind: ggui.ActionFocus})
	}
	focusButton("Previous")
	p.Type(ggui.Mods{}, ggui.KeyArrowLeft)
	if got := ggui.Untrack(active.Get); got != "two" {
		t.Fatalf("ArrowLeft on Previous went to %q, want two", got)
	}
	focusButton("Next")
	p.Type(ggui.Mods{}, ggui.KeyArrowRight)
	if got := ggui.Untrack(active.Get); got != "three" {
		t.Fatalf("ArrowRight on Next went to %q, want three", got)
	}
}

// koreanIME composes as a Korean input method does: each syllable is
// committed as the next one starts, which the test sets up by hand.
type koreanIME struct {
	h               ggui.IMEHandler
	compose, commit string
	shown           string
	confirmedEarly  *int
}

func (k *koreanIME) Update(image.Rectangle) (bool, error) {
	if k.commit != "" {
		k.h.Session()
		k.h.Commit(k.commit)
		k.commit = ""
	}
	if k.compose != k.shown {
		k.shown = k.compose
		k.h.Composition(k.compose, len(k.compose))
	}
	return k.shown != "", nil
}

func (k *koreanIME) Confirm() {
	if k.shown != "" {
		*k.confirmedEarly++
		k.h.Session()
		k.h.Commit(k.shown)
	}
	k.compose, k.shown = "", ""
	k.h.Composition("", 0)
}

func (k *koreanIME) Cancel()               { k.compose, k.shown = "", "" }
func (k *koreanIME) Rebase(string, string) {}
func (k *koreanIME) Typed(string)          {}

// The freeform answer's own write rebuilds the questionnaire around it: a
// Korean word typed there keeps each syllable whole, rather than having
// the rebuild commit the letter the next syllable began with.
func TestQuestionnaireFreeformTakesKorean(t *testing.T) {
	var early int
	var imes []*koreanIME
	t.Cleanup(ggui.SetIME(func(h ggui.IMEHandler) ggui.IME {
		k := &koreanIME{h: h, confirmedEarly: &early}
		imes = append(imes, k)
		return k
	}))
	answers := ggui.State(chat.QuestionAnswers{})
	q := chat.Questionnaire(answers, surveyQuestions()...)
	p := ggui.NewProbe(q, ggui.Sz(450, 500))
	defer p.Close()
	field, ok := p.Find("Other")
	if !ok {
		t.Fatalf("no freeform field:\n%s", p.Semantics())
	}
	p.Click(field.Center())
	ime := imes[len(imes)-1]
	step := func(commit, compose string) {
		ime.commit, ime.compose = commit, compose
		p.Move(field.Center()) // the IME runs on the input
		p.Frame()              // and the form is built again on the paint
	}
	step("", "ㅎ")
	step("", "하")
	step("", "한")
	step("한", "ㄱ")
	step("", "그")
	step("", "글")
	step("글", "")
	if got := ggui.Untrack(answers.Get)["one"].Text; got != "한글" || early != 0 {
		t.Fatalf("typed %q, with %d compositions committed early; want 한글 and none", got, early)
	}
}

// shadcn's Questionnaire and its item are both flex-col gap-4, and the
// item is a fieldset whose title is its legend, which no flex gap reaches:
// 16 from the progress to the title, the description straight under the
// title, 16 on to the choices, and the title's mb-4 in the description's
// place where there is none.
func TestQuestionnairePartsAreAGapApartAndTheDescriptionUnderTheTitle(t *testing.T) {
	t.Parallel()
	rects := func(q chat.Question) (progress, title, description, choice ggui.Rect) {
		w := chat.Questionnaire(ggui.State(chat.QuestionAnswers{}), q)
		p := ggui.NewProbe(w, ggui.Sz(450, 450))
		defer p.Close()
		for _, n := range p.Semantics().All() {
			switch {
			case n.Role == ggui.RoleProgress:
				progress = n.Rect
			case n.Name == q.Title:
				title = n.Rect
			case q.Description != "" && n.Name == q.Description:
				description = n.Rect
			case n.Name == "Task A":
				choice = n.Rect
			}
		}
		return
	}
	bottom := func(r ggui.Rect) float64 { return r.Origin.Y + r.Size.H }
	choices := []chat.QuestionOption{{Value: "a", Label: "Task A"}}
	progress, title, description, choice := rects(chat.Question{Name: "one", Title: "Choose a task", Description: "Pick the one to start with.", Choices: choices})
	if title.Size.H == 0 || description.Size.H == 0 || choice.Size.H == 0 {
		t.Fatalf("the parts were not found: %v %v %v", title, description, choice)
	}
	if got := title.Origin.Y - bottom(progress); math.Abs(got-16) > .01 {
		t.Errorf("the title is %v under the progress, want 16", got)
	}
	if got := description.Origin.Y - bottom(title); math.Abs(got) > .01 {
		t.Errorf("the description is %v under the title, want 0", got)
	}
	if got := choice.Origin.Y - bottom(description); math.Abs(got-16) > .01 {
		t.Errorf("the choices are %v under the description, want 16", got)
	}
	_, title, _, choice = rects(chat.Question{Name: "one", Title: "Choose a task", Choices: choices})
	if got := choice.Origin.Y - bottom(title); math.Abs(got-16) > .01 {
		t.Errorf("with no description the choices are %v under the title, want 16", got)
	}
}

type measuredWidget struct {
	ggui.Widget
	size ggui.Size
}

func (m *measuredWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	m.size = m.Widget.Layout(c.Loosen(), env)
	return m.size
}

func TestQuestionChoiceKeptPartsFollowTheTheme(t *testing.T) {
	t.Parallel()
	var options []chat.QuestionOption
	for i := range 3 {
		options = append(options, chat.QuestionOption{Value: fmt.Sprint(i), Label: fmt.Sprint("Option ", i), Description: "Details"})
	}
	q := chat.Questionnaire(ggui.State(chat.QuestionAnswers{}), chat.Question{Name: "q", Title: "Pick", Choices: options})
	scale := ggui.State(1.)
	m := &measuredWidget{Widget: q}
	p := ggui.NewProbe(uitheme.With(uitheme.Default(), ggui.Reactive(func() ggui.Widget {
		return ggui.Provide(ggui.TextScaleKey, scale.Get(), ggui.Widget(m))
	})), ggui.Sz(500, 4000))
	defer p.Close()
	p.Frame()
	before := m.size
	if _, ok := p.FindRole(ggui.RoleRadio, "Option 1"); !ok {
		t.Fatal("choice missing")
	}
	scale.Set(2)
	if p.Frame(); m.size.H <= before.H {
		t.Fatalf("choices kept their measurement under a larger text scale: %v <= %v", m.size.H, before.H)
	}
}
