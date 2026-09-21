// Command settings is a form over one struct: a single State holds the
// whole Profile, Field lenses bind each control to one member, Combine
// derives a dirty flag from the draft and the saved copy, Styled gives a
// section a muted base style, and Save runs on a worker that hands its
// result back through Post. The inspector opens on F1.
//
// The UI lives in build so main_test.go can drive it headlessly with a
// Probe.
package main

import (
	"errors"
	"log"
	"strings"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// Profile is a plain value. The form edits a copy in one signal and writes
// the whole struct back on every change, so equality is a plain ==.
type Profile struct {
	Name   string
	Email  string
	Notify bool
	Volume float64
}

// model holds the draft, the last saved copy and the status line. persist
// is what Save does on a worker; main stores to a slow fake backend and
// the test substitutes an instant one.
type model struct {
	Draft   *ggui.StateValue[Profile]
	Saved   *ggui.StateValue[Profile]
	Status  *ggui.StateValue[string]
	Saving  *ggui.StateValue[bool]
	persist func(Profile) error
}

func newModel(persist func(Profile) error) *model {
	initial := Profile{Name: "Ada", Email: "ada@example.com", Notify: true, Volume: 0.6}
	return &model{
		Draft: ggui.State(initial), Saved: ggui.State(initial),
		Status: ggui.State("Saved"), Saving: ggui.State(false), persist: persist,
	}
}

func emailError(p Profile) string {
	if p.Email != "" && !strings.Contains(p.Email, "@") {
		return "An address needs an @"
	}
	return ""
}

// save copies the draft, persists it off the UI goroutine, and posts the
// outcome back. Signals are only touched on the UI goroutine: before the
// worker starts and inside the posted function.
func (m *model) save(post func(func())) {
	draft := ggui.Untrack(m.Draft.Get)
	if emailError(draft) != "" || ggui.Untrack(m.Saving.Get) {
		return
	}
	m.Saving.Set(true)
	m.Status.Set("Saving…")
	go func() {
		err := m.persist(draft)
		post(func() {
			m.Saving.Set(false)
			if err != nil {
				m.Status.Set("Could not save: " + err.Error())
				return
			}
			m.Saved.Set(draft)
			m.Status.Set("Saved " + time.Now().Format("15:04:05"))
		})
	}()
}

func (m *model) reset() { m.Draft.Set(ggui.Untrack(m.Saved.Get)) }

// build is the root Builder; post is how Save reaches the UI goroutine
// again (App.Post or Probe.Post, both part of ggui.Host).
func (m *model) build(post func(func())) ggui.Widget {
	// Each lens reads one member of the draft and writes the whole struct
	// back, so the controls stay ordinary Binding consumers.
	name := m.Draft.Field(func(p *Profile) *string { return &p.Name })
	email := m.Draft.Field(func(p *Profile) *string { return &p.Email })
	notify := m.Draft.Field(func(p *Profile) *bool { return &p.Notify })
	volume := m.Draft.Field(func(p *Profile) *float64 { return &p.Volume })
	dirty := ggui.Combine(m.Draft, m.Saved, func(a, b Profile) bool { return a != b })
	invalid := m.Draft.Map(func(p Profile) bool { return emailError(p) != "" })
	cannotSave := ggui.Combine(dirty, invalid, func(d, i bool) bool { return !d || i })
	cannotSave = ggui.Combine(cannotSave, m.Saving, func(c, s bool) bool { return c || s })

	return ggui.Center(ggui.Box(ui.Card(ggui.Column(
		ggui.Title("Profile"),
		ui.Field("Name", ui.TextField(name).Placeholder("How should we address you?")),
		ui.Field("Email", ui.TextField(email).Placeholder("you@example.com")).
			Help("Where receipts go").Error(m.Draft.Map(emailError)),
		ui.Field("Volume", ui.Slider(volume, 0, 1).Step(0.05).Named("Volume")).
			Help("Notification sound level"),
		ui.Switch(notify, "Email me about activity"),
		ui.Divider(),
		ggui.Row(
			ui.Button("Save", func() { m.save(post) }).DisabledWhen(cannotSave),
			ui.Button("Reset", m.reset).Outline().DisabledWhen(dirty.Map(func(d bool) bool { return !d })),
			ggui.Spacer(),
			ggui.TextOf(m.Status).AsCaption().NoWrap(),
		).Space(1),
		// Styled sets the base style for everything below it: these lines
		// are small and muted without a setter on each Text. The color is
		// read once here; an app with a theme switch would put this in a
		// Reactive island or use Caption, which resolves at layout.
		ggui.Styled(ggui.Column(
			ggui.Textf("Draft: %s <%s>, notify=%t, volume=%.2f", name, email, notify, volume),
			ggui.Textf("Saved: %s <%s>", m.Saved.Map(func(p Profile) string { return p.Name }), m.Saved.Map(func(p Profile) string { return p.Email })),
		).Space(0.5)).Size(12).Color(ggui.UseTheme().MutedFg),
	).Space(1.5).Align(ggui.AlignStretch)).Pad(24)).Width(440))
}

// persistSlowly stands in for a backend: it takes a moment and refuses
// empty names, so both outcomes of Save can be seen.
func persistSlowly(p Profile) error {
	time.Sleep(600 * time.Millisecond)
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("name is required")
	}
	return nil
}

func main() {
	m := newModel(persistSlowly)
	var app *ggui.App
	app = ggui.New(ggui.Config{Title: "ggui · settings", Width: 520, Height: 480, Resizable: true, Inspector: "f1"},
		func() ggui.Widget { return m.build(app.Post) })
	app.Shortcut("cmd+s", func() { m.save(app.Post) })
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
