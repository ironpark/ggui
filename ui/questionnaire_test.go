package ui_test

import (
	"image"
	"slices"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// surveyQuestions is a three-step questionnaire: a required single choice
// with a disabled option and a freeform slot, a multiple choice, and an
// optional single choice.
func surveyQuestions() []ui.Question {
	return []ui.Question{
		{Name: "one", Title: "First", Required: true, InputLabel: "Other", Choices: []ui.QuestionOption{{Value: "a", Label: "Alpha"}, {Value: "x", Label: "Off", Disabled: true}, {Value: "b", Label: "Beta"}}},
		{Name: "two", Title: "Second", Multiple: true, Choices: []ui.QuestionOption{{Value: "c", Label: "Gamma"}, {Value: "d", Label: "Delta"}}},
		{Name: "three", Title: "Third", Choices: []ui.QuestionOption{{Value: "e", Label: "Epsilon"}}},
	}
}

func TestQuestionnaireCallbacksFollowNavigationAndStatus(t *testing.T) {
	t.Parallel()
	answers := ggui.State(ui.QuestionAnswers{})
	var items []string
	var statuses []string
	q := ui.Questionnaire(answers, surveyQuestions()...).Name("Survey").SubmitLabel("Send").
		OnItemChange(func(name string) { items = append(items, name) }).
		OnStatusChange(func(name string, s ui.QuestionStatus) { statuses = append(statuses, name+"="+string(s)) })
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
	answers := ggui.State(ui.QuestionAnswers{})
	q := ui.Questionnaire(answers, surveyQuestions()...)
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
	answers := ggui.State(ui.QuestionAnswers{})
	q := ui.Questionnaire(answers, surveyQuestions()...).Active("two")
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
	answers := ggui.State(ui.QuestionAnswers{"one": {Values: []string{"a"}}})
	active := ggui.State("two")
	q := ui.Questionnaire(answers, surveyQuestions()...).BindActive(active)
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
	answers := ggui.State(ui.QuestionAnswers{})
	q := ui.Questionnaire(answers, surveyQuestions()...)
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
