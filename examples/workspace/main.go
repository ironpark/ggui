// Command workspace combines GGUI features in a small project dashboard.
// model.go holds state and actions; view.go composes the screens.
// Everything stays in memory, so the example needs no backend or credentials.
package main

import (
	"flag"
	"log"

	"github.com/ironpark/ggui"
	_ "github.com/ironpark/ggui/inspect/panel"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func shortcuts(host ggui.Host, m *model) {
	host.Shortcut("cmd+n", m.create)
}

func run() error {
	var app *ggui.App
	dispose := ggui.Root(func() {
		m := newModel()
		app = ggui.New(ggui.Config{
			Title:     "ggui · workspace",
			Width:     1040,
			Height:    800,
			Resizable: true,
			Inspector: "f1",
		}, func() ggui.Widget { return build(m) })
		app.Setup(func() { bindTheme(m) })
		shortcuts(app, m)
	})
	defer dispose()
	defer app.Close()
	return app.Run()
}

func main() {
	renderDir := flag.String("render-dir", "", "render layout previews to PNG, then exit")
	flag.Parse()
	if *renderDir != "" {
		if err := renderWorkspace(*renderDir); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func bindTheme(m *model) {
	preset := uitheme.Preset{Base: uitheme.BaseNeutral, Accent: uitheme.AccentBlue}
	uitheme.Bind(m.Dark, preset.Dark(), preset.Light())
}
