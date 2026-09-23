// Command router shows page and layout routing: a sidebar layout that
// survives navigation, a parameterized project section with its own layout,
// a redirect, scoped not-found screens, and Back/Forward buttons that follow
// history. On the web the location lives in the URL fragment, so the
// browser's own Back button and reloads work too.
//
// The UI lives in routes so main_test.go can drive it with a Probe.
package main

import (
	"fmt"
	"log"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/router"
	"github.com/ironpark/ggui/ui"
)

func routes() *router.Router {
	var r *router.Router
	r = router.New(
		router.Layout("", func(c *router.Context, page ggui.Widget) ggui.Widget {
			return appLayout(r, page)
		},
			router.Page("/", homePage),
			router.Redirect("/home", "/"),
			router.Layout("/projects/:id", projectLayout,
				router.Page("/", overviewPage),
				router.Page("/tasks", tasksPage),
			),
			router.Page("/settings", settingsPage),
			router.NotFound(notFoundPage),
		),
	)
	return r
}

// appLayout is the persistent chrome: history buttons, the sidebar and the
// outlet. It is built once; only the outlet's content switches.
func appLayout(r *router.Router, page ggui.Widget) ggui.Widget {
	back := ui.Button("‹ Back", r.Back).Ghost().BindDisabled(not(r.CanBack()))
	forward := ui.Button("Forward ›", r.Forward).Ghost().BindDisabled(not(r.CanForward()))
	path := ggui.Derived(func() string { return r.Location().String() })
	toolbar := ggui.Row(back, forward, ggui.TextOf(path)).Space(1).Align(ggui.AlignCenter)

	sidebar := ggui.Column(
		ui.Title("Router"),
		// Active("/") is true everywhere, so Home compares the path exactly.
		navLink(r, "Home", "/", ggui.Derived(func() bool { return r.Location().Path == "/" })),
		navLink(r, "Project 1", "/projects/1", r.Active("/projects/1")),
		navLink(r, "Project 2", "/projects/2", r.Active("/projects/2")),
		navLink(r, "Settings", "/settings", r.Active("/settings")),
		navLink(r, "Old home link", "/home", ggui.Const(false)),
		navLink(r, "Broken link", "/nowhere", ggui.Const(false)),
	).Space(0.5)

	return ggui.Box(ggui.Row(
		ggui.Box(sidebar).Width(200),
		ggui.Expanded(ggui.Column(toolbar, ui.Card(page).Pad(24)).Space(1)),
	).Space(2)).Pad(16)
}

// navLink is a sidebar entry that stays highlighted while active.
func navLink(r *router.Router, label, url string, active ggui.Readable[bool]) ggui.Widget {
	go_ := func() {
		if err := r.Navigate(url); err != nil {
			log.Print(err)
		}
	}
	return ggui.If(active, func() ggui.Widget { return ui.Button(label, go_).Secondary() }).
		Else(func() ggui.Widget { return ui.Button(label, go_).Ghost() })
}

func not(b ggui.Readable[bool]) ggui.Readable[bool] {
	return ggui.Map(b, func(v bool) bool { return !v })
}

func homePage(c *router.Context) ggui.Widget {
	return ggui.Column(
		ui.Title("Home"),
		ggui.Text("The sidebar and toolbar belong to the app layout and stay mounted while pages change."),
	).Space(1)
}

// projectLayout is keyed by :id, so switching projects resets its state while
// moving between its tabs keeps it.
func projectLayout(c *router.Context, page ggui.Widget) ggui.Widget {
	id := c.Param("id")
	visits := ggui.State(0)
	ggui.Effect(func() ggui.Cleanup {
		c.Location() // count every location change inside this project
		ggui.Untrack(func() int { visits.Update(func(n int) int { return n + 1 }); return 0 })
		return nil
	})
	tab := func(label, url string) ggui.Widget {
		return ui.Button(label, func() { _ = c.Navigate(url) }).Outline()
	}
	return ggui.Column(
		ui.Title("Project "+id),
		ggui.Textf("locations seen by this project layout: %d", visits),
		ggui.Row(tab("Overview", "/projects/"+id), tab("Tasks", "/projects/"+id+"/tasks"), tab("Open tasks", "/projects/"+id+"/tasks?filter=open")).Space(0.5),
		page,
	).Space(1)
}

func overviewPage(c *router.Context) ggui.Widget {
	return ggui.Text(fmt.Sprintf("Overview of project %s.", c.Param("id")))
}

func tasksPage(c *router.Context) ggui.Widget {
	filter := ggui.Derived(func() string {
		if f := c.Query("filter"); f != "" {
			return "Tasks filtered by " + f + "."
		}
		return "All tasks."
	})
	return ggui.TextOf(filter)
}

func settingsPage(c *router.Context) ggui.Widget {
	return ggui.Column(ui.Title("Settings"), ggui.Text("Nothing to configure.")).Space(1)
}

func notFoundPage(c *router.Context) ggui.Widget {
	return ggui.Column(
		ui.Title("Not found"),
		ggui.Text("No page at "+c.Location().Path+"."),
		ui.Button("Go home", func() { _ = c.Navigate("/") }),
	).Space(1)
}

func main() {
	r := routes()
	app := ggui.New(ggui.Config{Title: "ggui · router", Width: 820, Height: 520, Resizable: true}, r.View)
	app.Shortcut("alt+left", r.Back)
	app.Shortcut("alt+right", r.Forward)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
