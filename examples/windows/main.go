// Command windows opens several windows over one piece of state. The main
// window counts; every inspector window it opens shows the same count and
// can change it, so a click in one repaints them all. Each window moves,
// resizes and closes on its own, and the title bar follows the count.
//
// Pass -smoke to open two inspectors, click through them, print what each
// window saw and quit, which is what a check without a person does.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

type model struct {
	Count  *ggui.StateValue[int]
	Opened *ggui.StateValue[int]
}

// main is the main window's tree: the count, a button that opens another
// window and the main window's own state.
func (m *model) main() ggui.Widget {
	w := ggui.UseWindow()
	ggui.Watch(m.Count, func(n int) { w.SetTitle(fmt.Sprintf("ggui · windows (%d)", n)) })
	return ggui.Column(ui.AppMenubar(w.App().Menu()), ggui.Expanded(ggui.Center(ui.Card(ggui.Column(
		ui.Titlef("count: %d", m.Count),
		ggui.Row(
			ui.Button("+1", func() { ggui.Add(m.Count, 1) }),
			ui.Button("Open inspector", func() { m.open(w.App()) }),
		).Space(1),
		ggui.Textf("windows opened: %d", m.Opened),
		describe(w),
	).Space(1)).Pad(24))))
}

// open adds an inspector window beside the main one.
func (m *model) open(app *ggui.App) *ggui.Window {
	ggui.Add(m.Opened, 1)
	n := ggui.Peek(m.Opened)
	at := app.Position().Add(ggui.Pt(float64(40*n), float64(40*n)))
	w, err := app.OpenWindow(ggui.Config{
		Title:       fmt.Sprintf("Inspector %d", n),
		Width:       320,
		Height:      240,
		Resizable:   true,
		AlwaysOnTop: true,
		Position:    &at,
	}, m.inspector)
	if err != nil {
		log.Print(err)
		return nil
	}
	return w
}

// inspector is an extra window's tree: the shared count and a way to close
// just this window.
func (m *model) inspector() ggui.Widget {
	w := ggui.UseWindow()
	return ggui.Center(ggui.Column(
		ggui.Textf("shared count: %d", m.Count),
		ggui.Row(
			ui.Button("-1", func() { ggui.Add(m.Count, -1) }),
			ui.Button("Close", w.Close),
		).Space(1),
		describe(w),
	).Space(1))
}

// describe follows a window's focus and size.
func describe(w *ggui.Window) ggui.Widget {
	status := ggui.Combine(w.Focused(), w.Viewport(), func(focused bool, size ggui.Size) string {
		return fmt.Sprintf("focused: %t · %.0f×%.0f", focused, size.W, size.H)
	})
	return ui.CaptionOf(status)
}

func main() {
	smoke := flag.Bool("smoke", false, "open two inspectors, report and quit")
	hold := flag.Bool("hold", false, "with -smoke, keep running after the report")
	flag.Parse()
	m := &model{Count: ggui.State(0), Opened: ggui.State(0)}
	app := ggui.New(ggui.Config{Title: "ggui · windows", Width: 480, Height: 320, Resizable: true}, m.main)
	// Every window follows the desktop's light or dark setting, title bars
	// included.
	app.Setup(func() { uitheme.Bind(ggui.SystemDark(), uitheme.Dark(), uitheme.Default()) })
	// On macOS the menus go in the menu bar; elsewhere ui.AppMenubar in
	// the main window draws them. Their chords work in every window.
	onTop := ggui.State(false)
	app.SetMenu(
		ggui.Menu("Counter",
			ggui.MenuAction("Add One", "cmd+up", func() { ggui.Add(m.Count, 1) }),
			ggui.MenuAction("Reset", "cmd+shift+r", func() { m.Count.Set(0) }).
				BindEnabled(ggui.Map(m.Count, func(n int) bool { return n != 0 })),
			ggui.MenuSeparator(),
			ggui.MenuAction("New Inspector", "cmd+shift+n", func() { m.open(app) }),
		),
		ggui.Menu("View",
			ggui.MenuAction("Hide Main Window", "", app.Hide),
			ggui.MenuAction("Show Main Window", "", func() { app.Show(); app.Focus() }),
			ggui.MenuAction("Keep Main Window on Top", "", func() {
				ggui.Toggle(onTop)
				app.SetAlwaysOnTop(ggui.Peek(onTop))
			}).BindChecked(onTop),
		),
	)
	// A tray icon with the count beside it and a menu of its own.
	tray := app.AddTray(ggui.TrayConfig{Icon: dot(), Template: true, Tooltip: "ggui · windows", Menu: []ggui.MenuItem{
		ggui.MenuAction("Add One", "", func() { ggui.Add(m.Count, 1) }),
		ggui.MenuAction("Show Main Window", "", func() { app.Show(); app.Focus() }),
		ggui.MenuSeparator(),
		ggui.MenuAction("Quit", "", app.Quit),
	}})
	app.Setup(func() {
		ggui.Watch(m.Count, func(n int) { tray.SetTitle(fmt.Sprint(n)) })
	})
	if *smoke {
		app.Setup(func() { smokeTest(app, m, *hold) })
	}
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// smokeTest drives the windows from a goroutine, the way a worker would:
// every step is posted to the UI thread.
func smokeTest(app *ggui.App, m *model, hold bool) {
	go func() {
		step := func(fn func()) {
			done := make(chan struct{})
			app.Post(func() { fn(); close(done) })
			<-done
			time.Sleep(300 * time.Millisecond)
		}
		var a, b *ggui.Window
		step(func() { a = m.open(app) })
		step(func() { b = m.open(app) })
		step(func() { ggui.Add(m.Count, 5) })
		step(func() {
			fmt.Printf("windows: %d, count %d\n", len(app.Windows()), m.Count.Get())
			for _, w := range app.Windows() {
				fmt.Printf("  %q at %v size %v state %d\n", w.Title(), w.Position(), w.Size(), ggui.Untrack(w.State().Get))
			}
			a.SetSize(ggui.Sz(400, 300))
		})
		step(func() {})
		step(func() {
			fmt.Printf("resized: %v, viewport %v\n", a.Size(), ggui.Untrack(a.Viewport().Get))
			a.Close()
		})
		step(func() { fmt.Printf("after close: %d windows, b closed %t\n", len(app.Windows()), b.Closed()) })
		step(b.Maximize)
		time.Sleep(time.Second)
		step(func() { fmt.Printf("maximized: state %d\n", ggui.Untrack(b.State().Get)); b.Restore() })
		time.Sleep(time.Second)
		step(func() {
			fmt.Printf("restored: state %d, size %v\n", ggui.Untrack(b.State().Get), ggui.Untrack(b.Viewport().Get))
		})
		time.Sleep(1500 * time.Millisecond)
		step(func() {
			fmt.Printf("system dark: %t, screens: %v\n", ggui.Untrack(ggui.SystemDark().Get), ggui.Screens())
		})
		if hold {
			return
		}
		app.Quit()
	}()
}

// dot is the tray icon: a filled circle, drawn by its alpha alone.
func dot() image.Image {
	const size = 36
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			dx, dy := float64(x)-size/2+0.5, float64(y)-size/2+0.5
			if dx*dx+dy*dy <= 14*14 {
				img.Set(x, y, color.Black)
			}
		}
	}
	return img
}
