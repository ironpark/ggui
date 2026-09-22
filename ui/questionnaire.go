package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// QuestionAnswer distinguishes unanswered, explicitly skipped and answered
// questions. Values are choice keys; Text is a freeform answer.
type QuestionAnswer struct {
	Values  []string
	Text    string
	Skipped bool
}
type QuestionAnswers map[string]QuestionAnswer
type QuestionOption struct {
	Value, Label, Description string
	Disabled                  bool
}

// Question defines an ordered step. Optional steps still need an answer or an
// explicit Skip before advancing. Disabled steps and choices never serialize.
type Question struct {
	Name, Title, Description     string
	Choices                      []QuestionOption
	Required, Multiple, Disabled bool
	InputLabel, Placeholder      string
	Validate                     func(QuestionAnswer) string
}
type QuestionStatus string

const (
	QuestionUnanswered QuestionStatus = "unanswered"
	QuestionAnswered   QuestionStatus = "answered"
	QuestionSkipped    QuestionStatus = "skipped"
)

type QuestionnaireShortcuts string

const (
	QuestionLetters QuestionnaireShortcuts = "letters"
	QuestionNumbers QuestionnaireShortcuts = "numbers"
)

// QuestionnaireWidget owns navigation and validation around caller-bound
// answers. Keep the widget alive to preserve local errors and reset defaults.
// It composes inside cards, dialogs or sheets and performs no submission I/O.
type QuestionnaireWidget struct {
	props property.Owner
	ggui.Interactive
	answers        ggui.Binding[QuestionAnswers]
	initial        QuestionAnswers
	items          []Question
	active         ggui.Binding[string]
	localActive    *ggui.StateValue[string]
	initialItem    string
	revision       *ggui.StateValue[int]
	errors         map[string]string
	externalErrors map[string]string
	view           ggui.Widget
	choices        []*questionChoice
	input          *TextFieldWidget
	pendingFocus   bool
	focusChoice    int
	shortcuts      QuestionnaireShortcuts
	submitLabel    string
	onSubmit       func(QuestionAnswers)
	onItem         func(string)
	onStatus       func(string, QuestionStatus)
}

func Questionnaire(answers ggui.Binding[QuestionAnswers], items ...Question) *QuestionnaireWidget {
	q := &QuestionnaireWidget{answers: answers, initial: cloneAnswers(ggui.Untrack(answers.Get)), revision: ggui.State(0), errors: map[string]string{}, externalErrors: map[string]string{}, submitLabel: "Submit", focusChoice: -1}
	q.Role = ggui.RoleGroup
	q.SetName("Questionnaire")
	q.AutoKey()
	q.SetItems(items...)
	first := ""
	for _, item := range items {
		if !item.Disabled {
			first = item.Name
			break
		}
	}
	q.localActive = ggui.State(first)
	q.active = q.localActive
	q.initialItem = first
	q.view = ggui.Reactive(q.build)
	return q
}
func cloneAnswers(src QuestionAnswers) QuestionAnswers {
	dst := QuestionAnswers{}
	for k, v := range src {
		v.Values = slices.Clone(v.Values)
		dst[k] = v
	}
	return dst
}
func (q *QuestionnaireWidget) Name(s string) *QuestionnaireWidget { q.SetName(s); return q }

// BindActive binds navigation to a stable question name for resume and host control.
func (q *QuestionnaireWidget) BindActive(v ggui.Binding[string]) *QuestionnaireWidget {
	property.Require(v, "BindActive")
	if property.Same(q.active, v) {
		return q
	}
	q.active = v
	q.initialItem = ggui.Untrack(v.Get)
	q.changed()
	return q
}
func (q *QuestionnaireWidget) Shortcuts(s QuestionnaireShortcuts) *QuestionnaireWidget {
	if property.Equal(q.shortcuts, s) {
		return q
	}
	defer property.Watch(&q.props, &q.shortcuts)()
	q.shortcuts = s
	q.changed()
	return q
}
func (q *QuestionnaireWidget) SubmitLabel(s string) *QuestionnaireWidget {
	if q.submitLabel == s {
		return q
	}
	defer property.Watch(&q.props, &q.submitLabel)()
	q.submitLabel = s
	q.changed()
	return q
}
func (q *QuestionnaireWidget) OnSubmit(fn func(QuestionAnswers)) *QuestionnaireWidget {
	q.onSubmit = fn
	return q
}
func (q *QuestionnaireWidget) OnItemChange(fn func(string)) *QuestionnaireWidget {
	q.onItem = fn
	return q
}
func (q *QuestionnaireWidget) OnStatusChange(fn func(string, QuestionStatus)) *QuestionnaireWidget {
	q.onStatus = fn
	return q
}

// SetItems replaces conditional steps. Answers remain stored, but disabled or
// removed steps are excluded from validation, progress and submitted answers.
func (q *QuestionnaireWidget) SetItems(items ...Question) {
	seen := map[string]bool{}
	for _, item := range items {
		if item.Name == "" || seen[item.Name] {
			panic("ui.Questionnaire: question names must be nonempty and unique")
		}
		seen[item.Name] = true
		values := map[string]bool{}
		for _, choice := range item.Choices {
			if choice.Value == "" || values[choice.Value] {
				panic("ui.Questionnaire: choice values must be nonempty and unique")
			}
			values[choice.Value] = true
		}
	}
	q.items = slices.Clone(items)
	for i := range q.items {
		q.items[i].Choices = slices.Clone(q.items[i].Choices)
	}
	q.changed()
}
func (q *QuestionnaireWidget) changed() {
	if q.revision != nil {
		ggui.Add(q.revision, 1)
	}
}
func (q *QuestionnaireWidget) enabled() []int {
	out := []int{}
	for i, item := range q.items {
		if !item.Disabled {
			out = append(out, i)
		}
	}
	return out
}
func (q *QuestionnaireWidget) current() int {
	if q.active == nil {
		return -1
	}
	name := ggui.Untrack(q.active.Get)
	first := -1
	for i, item := range q.items {
		if !item.Disabled {
			if first < 0 {
				first = i
			}
			if item.Name == name {
				return i
			}
		}
	}
	return first
}
func (q *QuestionnaireWidget) answer(item Question) QuestionAnswer {
	a := ggui.Untrack(q.answers.Get)[item.Name]

	selected := a.Values
	a.Values = nil
	for _, choice := range item.Choices {
		if !choice.Disabled && slices.Contains(selected, choice.Value) {
			a.Values = append(a.Values, choice.Value)
		}
	}

	if !item.Multiple && len(a.Values) > 1 {
		a.Values = a.Values[:1]
	}
	if item.InputLabel == "" {
		a.Text = ""
	}
	if item.Required {
		a.Skipped = false
	}
	return a
}
func status(a QuestionAnswer) QuestionStatus {
	if len(a.Values) > 0 || strings.TrimSpace(a.Text) != "" {
		return QuestionAnswered
	}
	if a.Skipped {
		return QuestionSkipped
	}
	return QuestionUnanswered
}
func (q *QuestionnaireWidget) Status(name string) QuestionStatus {
	for _, item := range q.items {
		if item.Name == name {
			return status(q.answer(item))
		}
	}
	return QuestionUnanswered
}
func (q *QuestionnaireWidget) store(item Question, a QuestionAnswer) {
	before := q.Status(item.Name)
	all := cloneAnswers(ggui.Untrack(q.answers.Get))
	all[item.Name] = a
	q.answers.Set(all)
	delete(q.errors, item.Name)
	delete(q.externalErrors, item.Name)
	q.changed()
	if after := q.Status(item.Name); before != after && q.onStatus != nil {
		q.onStatus(item.Name, after)
	}
}
func (q *QuestionnaireWidget) selectChoice(index int) {
	i := q.current()
	if i < 0 {
		return
	}
	item := q.items[i]
	if index < 0 || index >= len(item.Choices) || item.Choices[index].Disabled {
		return
	}
	a := q.answer(item)
	value := item.Choices[index].Value
	if item.Multiple {
		if slices.Contains(a.Values, value) {
			a.Values = slices.DeleteFunc(a.Values, func(v string) bool { return v == value })
		} else {
			a.Values = append(a.Values, value)
		}
	} else {
		a.Values = []string{value}
		a.Text = ""
	}
	a.Skipped = false
	q.store(item, a)
}
func (q *QuestionnaireWidget) move(i int) {
	if i < 0 || i >= len(q.items) || q.items[i].Disabled {
		return
	}
	name := q.items[i].Name
	if name != ggui.Untrack(q.active.Get) {
		q.active.Set(name)
		if q.onItem != nil {
			q.onItem(name)
		}
	}
	q.pendingFocus = true
	q.focusChoice = -1
	q.changed()
}
func (q *QuestionnaireWidget) validate(i int) bool {
	item := q.items[i]
	a := q.answer(item)
	err := q.externalErrors[item.Name]
	if status(a) == QuestionUnanswered {
		err = pick(item.Required, "Choose an answer to continue.", "Choose an answer or skip this question.")
	}
	if err == "" && item.Validate != nil {
		err = item.Validate(a)
	}
	if err != "" {
		q.errors[item.Name] = err
		q.move(i)
		return false
	}
	delete(q.errors, item.Name)
	return true
}
func (q *QuestionnaireWidget) Previous() {
	i := q.current()
	for j := i - 1; j >= 0; j-- {
		if !q.items[j].Disabled {
			q.move(j)
			return
		}
	}
}

// Next validates the active step. On the final step it validates all enabled
// steps before invoking OnSubmit, returning to the first invalid one if needed.
func (q *QuestionnaireWidget) Next() bool {
	i := q.current()
	if i < 0 || !q.validate(i) {
		return false
	}
	for j := i + 1; j < len(q.items); j++ {
		if !q.items[j].Disabled {
			q.move(j)
			return true
		}
	}
	return q.Submit()
}
func (q *QuestionnaireWidget) Skip() bool {
	i := q.current()
	if i < 0 || q.items[i].Required {
		return false
	}
	q.store(q.items[i], QuestionAnswer{Skipped: true})
	return q.Next()
}
func (q *QuestionnaireWidget) Submit() bool {
	enabled := q.enabled()
	if len(enabled) == 0 {
		return false
	}
	out := QuestionAnswers{}
	for _, i := range enabled {
		if !q.validate(i) {
			return false
		}
		a := q.answer(q.items[i])
		if !a.Skipped {
			out[q.items[i].Name] = a
		}
	}
	if q.onSubmit != nil {
		q.onSubmit(cloneAnswers(out))
	}
	return true
}
func (q *QuestionnaireWidget) Reset() {
	q.answers.Set(cloneAnswers(q.initial))
	clear(q.errors)
	clear(q.externalErrors)
	q.active.Set(q.initialItem)
	q.pendingFocus = true
	q.changed()
}

// SetError supports a host/schema validation error and returns to its step.
func (q *QuestionnaireWidget) SetError(name, message string) {
	q.errors[name] = message
	q.externalErrors[name] = message
	for i, item := range q.items {
		if item.Name == name {
			q.move(i)
			return
		}
	}
	q.changed()
}
func (q *QuestionnaireWidget) Error(name string) string { return q.errors[name] }
func (q *QuestionnaireWidget) Progress() (current, total int) {
	indices := q.enabled()
	return slices.Index(indices, q.current()) + 1, len(indices)
}

func (q *QuestionnaireWidget) build() ggui.Widget {
	q.revision.Get()
	q.answers.Get()
	q.active.Get()
	q.choices = nil
	q.input = nil
	i := q.current()
	if i < 0 {
		return Empty("No questions", "There are no enabled questions.")
	}
	item := q.items[i]
	current, total := q.Progress()
	heading := []ggui.Widget{ggui.Text(item.Title).StyleKey(uitheme.TitleKey, uitheme.Default().Title).Role(ggui.RoleHeading).Size(16).LineHeight(1.5)}
	if item.Description != "" {
		heading = append(heading, ggui.Text(item.Description).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption).Size(14).LineHeight(1.5))
	}
	body := []ggui.Widget{ggui.Column(
		&questionProgress{Caption(fmt.Sprintf("Question %d of %d", current, total)), current, total},
		ggui.Column(heading...).Gap(2).Align(ggui.AlignStretch),
	).Gap(20).Align(ggui.AlignStretch)}
	rows := []ggui.Widget{}
	shortcut := 0
	for ci, choice := range item.Choices {
		c := &questionChoice{q: q, item: item, option: choice, index: ci}
		c.Role = pick(item.Multiple, ggui.RoleCheckbox, ggui.RoleRadio)
		c.SetName(choice.Label)
		c.SetInert(choice.Disabled)
		c.SetKey(struct {
			Q           *QuestionnaireWidget
			Item, Value string
		}{q, item.Name, choice.Value})
		if !choice.Disabled {
			if q.shortcuts == QuestionLetters && shortcut < 26 {
				c.shortcut = string(rune('A' + shortcut))
			}
			if q.shortcuts == QuestionNumbers && shortcut < 9 {
				c.shortcut = fmt.Sprint(shortcut + 1)
			}
			shortcut++
		}
		q.choices = append(q.choices, c)
		rows = append(rows, c)
	}
	if item.InputLabel != "" {
		q.input = TextField(questionTextBinding{q, item}).Name(item.InputLabel).Placeholder(item.Placeholder)
		q.input.Input().Key(struct {
			Q    *QuestionnaireWidget
			Item string
		}{q, item.Name})
		q.input.OnKey(func(ev ggui.KeyEvent) bool {
			if ev.Key == ggui.KeyEnter && (ev.Mods.Ctrl || ev.Mods.Meta || strings.TrimSpace(q.answer(item).Text) != "") {
				q.Next()
				return true
			}
			if q.answer(item).Text == "" && (ev.Key == ggui.KeyArrowUp || ev.Key == ggui.KeyArrowDown) {
				q.focusAnswer(pick(ev.Key == ggui.KeyArrowUp, -1, 1), len(q.choices))
				return true
			}
			return false
		})
		rows = append(rows, &questionInput{q.input})
	}
	body = append(body, ggui.Column(rows...).Gap(8).Align(ggui.AlignStretch))
	if message := q.errors[item.Name]; message != "" {
		body = append(body, &questionError{ggui.Text(message).Size(14)})
	}
	actions := []ggui.Widget{}
	if current > 1 {
		actions = append(actions, q.button("Previous", q.Previous, true))
	}
	if !item.Required {
		actions = append(actions, q.button("Skip", func() { q.Skip() }, true))
	}
	label := pick(current == total, q.submitLabel, "Next")
	actions = append(actions, q.button(label, func() { q.Next() }, false))
	body = append(body, &questionActions{children: actions, previous: current > 1})
	return &questionStack{ggui.Column(body...).Align(ggui.AlignStretch)}
}
func (q *QuestionnaireWidget) button(label string, fn func(), outline bool) ggui.Widget {
	b := Button(label, fn)
	if outline {
		b.Outline()
	}
	return &questionButton{ButtonWidget: b, q: q}
}
func (q *QuestionnaireWidget) focusAnswer(direction, from int) {
	// The text input, when present, is the slot after the last choice.
	enabled := func(i int) bool {
		if i < len(q.choices) {
			return !q.choices[i].IsInert()
		}
		return q.input != nil
	}
	next := stepIndex(from, direction, len(q.choices)+1, enabled)
	if !enabled(next) {
		return
	}
	q.pendingFocus = true
	q.focusChoice = next
	if next < len(q.choices) && !q.choices[next].item.Multiple {
		q.selectChoice(next)
	}
}
func (q *QuestionnaireWidget) shortcut(ev ggui.KeyEvent) bool {
	if ev.Kind != ggui.KeyPress {
		return false
	}
	if ev.Key == ggui.KeyEnter && (ev.Mods.Ctrl || ev.Mods.Meta) {
		q.Next()
		return true
	}
	if ev.Mods.Ctrl || ev.Mods.Meta || ev.Mods.Alt {
		return false
	}
	for i, c := range q.choices {
		if c.shortcut != "" && strings.EqualFold(strings.TrimPrefix(ev.Key.String(), "Digit"), c.shortcut) {
			q.selectChoice(i)
			q.pendingFocus = true
			q.focusChoice = i
			return true
		}
	}
	return false
}
func (q *QuestionnaireWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer q.props.Layout()()
	return q.view.Layout(c, env)
}
func (q *QuestionnaireWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.DescribeNode(r, q, func(dst *ggui.Canvas) { dst.Paint(q.view, r) })
	if q.pendingFocus {
		target := q.focusChoice
		if target < 0 {
			for i, c := range q.choices {
				if !c.IsInert() {
					target = i
					break
				}
			}
			if target < 0 && q.input != nil {
				target = len(q.choices)
			}
		}
		if target >= 0 && target < len(q.choices) {
			dst.RequestFocus(q.choices[target])
		} else if target == len(q.choices) && q.input != nil {
			dst.RequestFocus(q.input.Input())
		}
		q.pendingFocus = false
	}
}
func (q *QuestionnaireWidget) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleGroup, Name: q.SemanticName()}
}

type questionTextBinding struct {
	q    *QuestionnaireWidget
	item Question
}

func (b questionTextBinding) Get() string { b.q.answers.Get(); return b.q.answer(b.item).Text }
func (b questionTextBinding) Set(s string) {
	a := b.q.answer(b.item)
	a.Text = s
	a.Skipped = false
	if !b.item.Multiple && strings.TrimSpace(s) != "" {
		a.Values = nil
	}
	b.q.store(b.item, a)
}

type questionProgress struct {
	ggui.Widget
	current, total int
}

func (p *questionProgress) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleProgress, Name: "Questionnaire progress", Min: 0, Max: float64(p.total), Now: float64(p.current)}, func(dst *ggui.Canvas) { dst.Paint(p.Widget, r) })
}

type questionButton struct {
	*ButtonWidget
	q *QuestionnaireWidget
}

func (b *questionButton) Paint(dst *ggui.Canvas, r ggui.Rect) {
	b.ButtonWidget.Paint(dst.Inert(), r)
	b.Hit(dst, r, b, ggui.CursorShapePointer)
}
func (b *questionButton) HandleKey(ev ggui.KeyEvent) {
	if b.q.shortcut(ev) {
		return
	}
	if ev.Kind == ggui.KeyPress && ev.Key == ggui.KeyArrowLeft {
		b.q.Previous()
		return
	}
	if ev.Kind == ggui.KeyPress && ev.Key == ggui.KeyArrowRight {
		b.q.Next()
		return
	}
	b.ButtonWidget.HandleKey(ev)
}

type questionChoice struct {
	ggui.Interactive
	q        *QuestionnaireWidget
	item     Question
	option   QuestionOption
	index    int
	shortcut string
	body     ggui.Widget
	theme    uitheme.Theme
	env      ggui.Env
}

func (c *questionChoice) selected() bool {
	return slices.Contains(c.q.answer(c.item).Values, c.option.Value)
}
func (c *questionChoice) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	c.theme = uitheme.From(env)
	c.env = env
	text := []ggui.Widget{ggui.Text(c.option.Label).Font(uitheme.From(env).Title.Font).Size(14).LineHeight(1.5).Color(pick(c.IsInert(), c.theme.MutedFg, c.theme.Fg))}
	if c.option.Description != "" {
		text = append(text, ggui.Text(c.option.Description).Size(14).LineHeight(1.5).Color(c.theme.MutedFg))
	}
	parts := []ggui.Widget{ggui.Box().Width(16).Height(16), ggui.Expanded(ggui.Column(text...).Gap(c.theme.ChatTokens().QuestionTextGap))}
	if c.shortcut != "" {
		parts = append(parts, ggui.Box(ggui.Center(ggui.Text(c.shortcut).Size(10).Color(c.theme.MutedFg))).
			Width(20).Height(20).Radius(8).Fill(c.theme.Bg).Border(1, c.theme.Border))
	}
	tokens := c.theme.ChatTokens()
	c.body = ggui.Box(ggui.Row(parts...).Gap(tokens.QuestionChoiceGap).Align(ggui.AlignStart)).Padding(tokens.QuestionChoicePadding)
	s := c.body.Layout(cs, env)
	s.H = max(44, s.H)
	return cs.Constrain(s)
}
func (c *questionChoice) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := c.theme
	selected := c.selected()
	fill, border := mix(t.Bg, t.Muted, .2), t.Border
	if selected {
		fill = t.Muted
		border = mix(t.Primary, t.Card, .6)
	} else if c.Hovered && !c.IsInert() {
		fill = mix(t.Card, t.Muted, .5)
	}
	if c.q.errors[c.item.Name] != "" {
		border = t.Destructive
	}
	tokens := t.ChatTokens()
	dst.FillRoundRect(r, tokens.QuestionRadius, fill)
	dst.StrokeRoundRect(r, tokens.QuestionRadius, 1, border)
	c.Hit(dst, r, c, ggui.CursorShapePointer)
	dst.Paint(c.body, r)
	glyph := ggui.Rct(r.Origin.Add(ggui.Pt(tokens.QuestionChoicePadding.Left, tokens.QuestionChoicePadding.Top+2)), ggui.Sz(16, 16))
	radius := pick(c.item.Multiple, 4.0, 8.0)
	dst.FillRoundRect(glyph, radius, pick(selected, t.Primary, t.Input))
	dst.StrokeRoundRect(glyph, radius, 1, pick(selected, t.Primary, t.Border))
	if selected && c.item.Multiple {
		paintIcon(dst, c.env, icons.Check, glyph, t.PrimaryFg, 0)
	} else if selected {
		dst.FillCircle(glyph.Origin.Add(ggui.Pt(8, 8)), 4, t.PrimaryFg)
	}
	c.FocusRing(dst, r, tokens.QuestionRadius, t.Ring)
}
func (c *questionChoice) HandlePointer(ev ggui.PointerEvent) bool {
	if c.IsInert() {
		return false
	}
	return c.Pointer(ev, func() { c.q.selectChoice(c.index) })
}
func (c *questionChoice) HandleKey(ev ggui.KeyEvent) {
	if c.IsInert() {
		return
	}
	if c.q.shortcut(ev) {
		return
	}
	if ev.Kind == ggui.KeyPress {
		switch ev.Key {
		case ggui.KeyArrowUp, ggui.KeyArrowDown:
			c.q.focusAnswer(pick(ev.Key == ggui.KeyArrowUp, -1, 1), c.index)
			return
		case ggui.KeyArrowLeft, ggui.KeyArrowRight:
			if !c.item.Multiple {
				c.q.focusAnswer(pick(ev.Key == ggui.KeyArrowLeft, -1, 1), c.index)
			} else if ev.Key == ggui.KeyArrowLeft {
				c.q.Previous()
			} else {
				c.q.Next()
			}
			return
		case ggui.KeyEnter:
			if c.selected() {
				c.q.Next()
			} else {
				c.q.selectChoice(c.index)
			}
			return
		}
	}
	c.Keyboard(ev, func() { c.q.selectChoice(c.index) })
}
func (c *questionChoice) ConsumesKey(ev ggui.KeyEvent) bool {
	switch ev.Key {
	case ggui.KeyArrowUp, ggui.KeyArrowDown, ggui.KeyArrowLeft, ggui.KeyArrowRight:
		return ev.Kind == ggui.KeyPress
	}
	return c.Interactive.ConsumesKey(ev)
}
func (c *questionChoice) Describe() ggui.Node {
	return ggui.Node{Role: c.Role, Name: c.SemanticName(), Description: c.option.Description, Checked: ggui.Tri(c.selected()), Disabled: c.IsInert(), Actions: ggui.ActionPress | ggui.ActionSelect | ggui.ActionFocus}
}
func (c *questionChoice) Act(a ggui.Action) bool {
	if c.IsInert() || (a.Kind != ggui.ActionPress && a.Kind != ggui.ActionSelect) {
		return false
	}
	if a.Kind == ggui.ActionPress || !c.selected() {
		c.q.selectChoice(c.index)
	}
	return true
}

// Keep navigation at the trailing edge, but wrap instead of clipping when a
// questionnaire is embedded in a narrow sheet or a large-text environment.
type questionActions struct {
	children []ggui.Widget
	previous bool
	body     ggui.Widget
}

func (a *questionActions) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	width := float64(max(0, len(a.children)-1)) * 8
	for _, child := range a.children {
		width += child.Layout(c.Loosen(), env).W
	}
	if width > c.MaxW {
		a.body = ggui.Wrap(a.children...).Gap(8)
	} else {
		parts := slices.Clone(a.children)
		at := 0
		if a.previous {
			at = 1
		}
		parts = slices.Insert(parts, at, ggui.Widget(ggui.Spacer()))
		a.body = ggui.Row(parts...).Gap(8)
	}
	return a.body.Layout(c, env)
}
func (a *questionActions) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(a.body, r) }

// The questionnaire input uses the compact 32px chrome from its reference,
// while preserving the editor's normal focus, IME and clipboard handling.
type questionInput struct{ *TextFieldWidget }

func (f *questionInput) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.From(env)
	t.FieldPad = ggui.Insets(5, 10)
	t.Radius = t.ChatTokens().QuestionInputRadius
	t.Input = mix(t.Bg, t.Input, .3)
	c.MinH = min(c.MaxH, max(c.MinH, 32))
	return f.TextFieldWidget.Layout(c, t.Apply(env))
}

// Theme-dependent presentation resolves at layout, including under Themed.
// A palette swap must not reconstruct the editor or reset the answers.
type questionStack struct{ *ggui.ColumnWidget }

func (s *questionStack) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	s.Gap(uitheme.From(env).ChatTokens().QuestionGap)
	return s.ColumnWidget.Layout(c, env)
}

type questionError struct{ *ggui.TextWidget }

func (e *questionError) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	e.Color(uitheme.From(env).Destructive)
	return e.TextWidget.Layout(c, env)
}

// Active detaches navigation from its reader and selects a local question.
func (q *QuestionnaireWidget) Active(v string) *QuestionnaireWidget {
	q.localActive.Set(v)
	q.initialItem = v
	return q.BindActive(q.localActive)
}
