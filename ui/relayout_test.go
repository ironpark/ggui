package ui

import (
	"fmt"
	"testing"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// Controls keep their child widgets across layouts so the text caches
// survive. These check that what a fresh child used to pick up each
// layout still reaches the kept one.

func TestSelectKeptLabelFollowsTheValue(t *testing.T) {
	t.Parallel()
	value := ggui.State("a")
	s := Select(value).Options([]string{"a", "a much longer option"})
	p := ggui.NewProbe(s, ggui.Sz(400, 100))
	defer p.Close()
	p.Frame()
	label, short := s.text, s.textSize.W
	value.Set("a much longer option")
	p.Frame()
	if s.text != label {
		t.Fatal("the label was rebuilt")
	}
	if s.textSize.W <= short {
		t.Fatalf("label kept the old value: %v <= %v", s.textSize.W, short)
	}
	s.Disabled(true)
	p.Frame()
	if s.text != label || s.textSize.W <= short {
		t.Fatal("label lost its value when disabled")
	}
}

func TestChartCachedLabelsRemeasureUnderANewEnv(t *testing.T) {
	t.Parallel()
	c := LineChart(chartTestData(), chartTestConfig)
	cs := ggui.Loose(ggui.Sz(400, 240))
	env := uitheme.Default().Apply(ggui.Env{})
	c.Layout(cs, env)
	first := c.measuredText("Jan", 12, c.theme.Fg)
	small := first.size
	c.Layout(cs, env)
	if c.measuredText("Jan", 12, c.theme.Fg) != first {
		t.Fatal("a relayout dropped the measured label")
	}
	c.Layout(cs, env.With(ggui.TextScaleKey, 2.))
	if got := c.measuredText("Jan", 12, c.theme.Fg).size; got.W < small.W*1.5 || got.H < small.H*1.5 {
		t.Fatalf("label kept its measurement under a larger text scale: %v, was %v", got, small)
	}
	dark := uitheme.Dark().Apply(ggui.Env{})
	c.Layout(cs, dark)
	if got := c.measuredText("Jan", 12, c.theme.Fg).size; got != small {
		t.Fatalf("label after a theme switch measured %v, want %v", got, small)
	}
}

func TestChartTooltipContentRemeasuresUnderANewEnv(t *testing.T) {
	t.Parallel()
	w := ChartTooltipContent(LineChart(chartTestData(), chartTestConfig), 0)
	env := uitheme.Default().Apply(ggui.Env{})
	cs := ggui.Loose(ggui.Sz(1000, 1000))
	small := w.Layout(cs, env)
	if big := w.Layout(cs, env.With(ggui.TextScaleKey, 2.)); big.W <= small.W {
		t.Fatalf("tooltip kept its measurement under a larger text scale: %v, was %v", big, small)
	}
	if again := w.Layout(cs, env); again != small {
		t.Fatalf("tooltip %v, want %v", again, small)
	}
}

// measured records the size its child was last laid out at.
type measured struct {
	ggui.Widget
	size ggui.Size
}

func (m *measured) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	m.size = m.Widget.Layout(c.Loosen(), env)
	return c.Constrain(m.size)
}

func TestFieldKeptColumnFollowsTheNote(t *testing.T) {
	t.Parallel()
	msg := ggui.State("")
	f := &measured{Widget: Field("Email", TextField(ggui.State(""))).BindError(msg)}
	p := ggui.NewProbe(f, ggui.Sz(300, 400))
	defer p.Close()
	p.Frame()
	bare := f.size
	msg.Set("Required")
	p.Frame()
	shown := f.size
	if shown.H <= bare.H {
		t.Fatalf("error not shown: %v <= %v", shown.H, bare.H)
	}
	msg.Set("Required, and it must contain an @ and a domain after it, which this does not")
	if p.Frame(); f.size.H <= shown.H {
		t.Fatalf("error text did not update: %v <= %v", f.size.H, shown.H)
	}
	msg.Set("")
	if p.Frame(); f.size != bare {
		t.Fatalf("error not removed: %v, want %v", f.size, bare)
	}
}

func TestAlertAndEmptyRebuildForALaterAction(t *testing.T) {
	t.Parallel()
	env := uitheme.Default().Apply(ggui.Env{})
	cs := ggui.Loose(ggui.Sz(300, 400))
	// The action hangs at the top right and the text keeps clear of it,
	// so a description that just fit beside nothing wraps beside it.
	a := Alert("Saved", "All of your changes are stored safely")
	bare := a.Layout(cs, env)
	if got := a.Action(Button("Undo", nil)).Layout(cs, env); got.H <= bare.H {
		t.Fatalf("alert ignored its action: %v <= %v", got.H, bare.H)
	}
	e := Empty("Nothing here", "Add an item")
	bare = e.Layout(cs, env)
	if got := e.Media(Icon("search")).Action(Button("Add", nil)).Layout(cs, env); got.H <= bare.H {
		t.Fatalf("empty ignored its media and action: %v <= %v", got.H, bare.H)
	}
}

func TestCommandKeptPanelFollowsItsOptions(t *testing.T) {
	t.Parallel()
	env := uitheme.Default().Apply(ggui.Env{})
	cs := ggui.Loose(ggui.Sz(400, 600))
	c := Command(ggui.State(""), CommandItem("Open", nil), CommandItem("Save", nil))
	bare := c.Layout(cs, env)
	panel := c.panel
	if c.Layout(cs, env); c.panel != panel {
		t.Fatal("a relayout rebuilt the panel")
	}
	hinted := c.Hints().Layout(cs, env)
	if hinted.H <= bare.H {
		t.Fatalf("hints not shown: %v <= %v", hinted.H, bare.H)
	}
	c.HintLabels("Move between the actions of this palette", "Run")
	c.Layout(ggui.Loose(ggui.Sz(120, 600)), env)
	short := c.hint
	if c.Layout(cs, env); c.hint != short {
		t.Fatal("a relayout rebuilt the hint")
	}
	if got, want := c.hint.Layout(ggui.Loose(ggui.Sz(1000, 100)), env).W, Caption("↑↓ Move between the actions of this palette   ↵ Run").Layout(ggui.Loose(ggui.Sz(1000, 100)), env).W; got != want {
		t.Fatalf("hint %v wide, want %v", got, want)
	}
	// The search field is an inset pill whether or not InsetSearch asks.
	if inset := c.InsetSearch().Layout(cs, env); c.searchBox == nil || inset != hinted {
		t.Fatalf("InsetSearch changed the panel: %v, want %v", inset, hinted)
	}
}

func TestQuestionChoiceKeptPartsFollowTheTheme(t *testing.T) {
	t.Parallel()
	var options []QuestionOption
	for i := range 3 {
		options = append(options, QuestionOption{Value: fmt.Sprint(i), Label: fmt.Sprint("Option ", i), Description: "Details"})
	}
	q := Questionnaire(ggui.State(QuestionAnswers{}), Question{Name: "q", Title: "Pick", Choices: options})
	scale := ggui.State(1.)
	m := &measured{Widget: q}
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

// A chart and twenty selects laid out again at a width one pixel apart, as
// a window resize does: what keeping their labels across layouts saves.
func BenchmarkRelayoutChartAndSelects(b *testing.B) {
	widgets := []ggui.Widget{LineChart(chartTestData(), chartTestConfig).Animation(0)}
	for range 20 {
		widgets = append(widgets, Select(ggui.State("alpha")).Options([]string{"alpha", "beta", "gamma"}))
	}
	p := ggui.NewProbe(uitheme.With(uitheme.Default(), ggui.Column(widgets...)), ggui.Sz(800, 2000))
	defer p.Close()
	p.Frame()
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		i++
		p.Resize(ggui.Sz(800+float64(i%2), 2000))
		p.Frame()
	}
}

// A widget keeps the theme by pointer from Layout to Paint; a theme switch
// must reach it at the next layout, as the copy it kept before did.
func TestKeptThemeFollowsAThemeSwitch(t *testing.T) {
	t.Parallel()
	dark := ggui.State(false)
	b := Button("Go", nil)
	p := ggui.NewProbe(ggui.Reactive(func() ggui.Widget {
		return uitheme.With(pick(dark.Get(), uitheme.Dark(), uitheme.Default()), ggui.Widget(b))
	}), ggui.Sz(200, 60))
	defer p.Close()
	p.Frame()
	light := b.theme
	if light.Bg != uitheme.Default().Bg {
		t.Fatal("button not laid out under the light theme")
	}
	dark.Set(true)
	p.Frame()
	if b.theme.Bg != uitheme.Dark().Bg || light.Bg != uitheme.Default().Bg {
		t.Fatalf("button theme %v after the switch, light %v", b.theme.Bg, light.Bg)
	}
}

func TestIconResolvedAtLayoutFollowsItsFallback(t *testing.T) {
	t.Parallel()
	env := uitheme.Default().Apply(ggui.Env{})
	w := Icon("search")
	w.Layout(ggui.Loose(ggui.Sz(16, 16)), env)
	w.Fallback(nil)
	w.Layout(ggui.Loose(ggui.Sz(16, 16)), env)
	w.Paint(nil, ggui.Rct(ggui.Point{}, ggui.Sz(16, 16))) // no fallback, no asset: draws nothing, does not panic
}
