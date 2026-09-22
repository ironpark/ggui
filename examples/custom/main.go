// Command custom shows what the built-in widgets are made of: a Dial
// written from Layout and Paint on top of ggui.Interactive, dragged through
// its pointer handler with a Spring easing the needle; a Disclosure that
// keeps its open flag outside any signal and calls Invalidate when its size
// changes; and a list of 5,000 rows that ItemExtent and Retain keep as
// cheap as the rows in view. The inspector opens on F1.
//
// dial.go and disclosure.go implement the custom widgets. This file composes
// them into a screen; main_test.go drives build headlessly with a Probe.
package main

import (
	"fmt"
	"log"

	"github.com/ironpark/ggui"
	_ "github.com/ironpark/ggui/inspect/panel"
	"github.com/ironpark/ggui/ui"
)

type item struct {
	ID   int
	Name string
}

const rowCount = 5000
const rowHeight = 32.0

type model struct {
	Level  *ggui.StateValue[float64]
	Items  *ggui.StateValue[[]item]
	Scroll *ggui.StateValue[float64]
	Detail *disclosure
}

func newModel() *model {
	items := make([]item, rowCount)
	for i := range items {
		items[i] = item{ID: i + 1, Name: fmt.Sprintf("Row %d", i+1)}
	}
	return &model{
		Level:  ggui.State(0.35),
		Items:  ggui.State(items),
		Scroll: ggui.State(0.0),
	}
}

func (m *model) jump(index int) { m.Scroll.Set(float64(index) * rowHeight) }

// build is the root Builder. The dial and the list are bound to signals,
// the disclosure holds its own flag, and nothing here reads reactively.
func (m *model) build() ggui.Widget {
	m.Detail = newDisclosure("How this works", ggui.Styled(ggui.Column(
		ggui.Text("The dial embeds ggui.Interactive and draws itself; a Spring eases its needle."),
		ggui.Text("This panel calls Invalidate instead of writing a signal when it opens."),
		ggui.Text("The list below builds only the rows in view and keeps 24 spares mounted."),
	).Space(0.5)).Size(12))
	return ggui.Box(ggui.Column(
		ggui.Row(
			newDial(m.Level, "Level"),
			ggui.Column(
				ggui.Title("Custom widgets"),
				ggui.Textf("Level: %.0f%%", m.Level.Map(func(v float64) float64 { return v * 100 })),
				ggui.Caption("Drag the dial, or focus it and use the arrows."),
				ui.Progress(m.Level),
			).Space(1).Align(ggui.AlignStretch),
		).Space(2).Align(ggui.AlignCenter),
		ui.Card(m.Detail),
		ggui.Row(
			ggui.Textf("%d rows", m.Items.Map(func(it []item) int { return len(it) })).AsCaption(),
			ggui.Spacer(),
			ui.Button("Top", func() { m.jump(0) }).Outline(),
			ui.Button("Middle", func() { m.jump(rowCount / 2) }).Outline(),
			ui.Button("End", func() { m.jump(rowCount) }).Outline(),
		).Space(1),
		ggui.Expanded(ggui.Scroll(
			// Each row keeps a checkbox of its own; Retain bounds how many
			// offscreen rows keep that state before being rebuilt.
			ggui.EachKeyed(m.Items, func(it item) int { return it.ID }, func(row ggui.EachItem[item]) ggui.Widget {
				picked := ggui.State(false)
				return ui.Checkbox(picked, row.Value.Get().Name)
			}).ItemExtent(rowHeight).Retain(24),
		).BindOffset(m.Scroll)),
	).Space(1.5).Align(ggui.AlignStretch)).Pad(24)
}

func main() {
	m := newModel()
	app := ggui.New(ggui.Config{
		Title:     "ggui · custom widgets",
		Width:     560,
		Height:    640,
		Resizable: true,
		Inspector: "f1",
	}, m.build)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
