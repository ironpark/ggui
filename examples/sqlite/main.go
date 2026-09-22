// Command sqlite is a small SQLite client for existing local database files.
package main

import (
	"flag"
	"github.com/ironpark/ggui"
	_ "github.com/ironpark/ggui/inspect/panel"
	"log"
)

func run(path string, readOnly bool) error {
	m := newModel()
	m.ReadOnly.Set(readOnly)
	app := ggui.New(ggui.Config{Title: "ggui · SQLite", Width: 1280, Height: 900, Resizable: true, Inspector: "f1"}, func() ggui.Widget { return build(m) })
	m.post = app.Post
	m.dialogs = app.Dialogs()
	app.Setup(func() {
		ggui.BindTheme(m.Dark, clientTheme(true), clientTheme(false))
		if path != "" {
			m.open(path)
		}
	})
	app.Shortcut("cmd+o", m.choose)
	app.Shortcut("cmd+enter", m.execute)
	app.Shortcut("cmd+r", m.browse)
	defer app.Close()
	defer m.close()
	return app.Run()
}
func main() {
	path := flag.String("db", "", "existing SQLite database file")
	readOnly := flag.Bool("readonly", false, "open database read-only")
	renderDir := flag.String("render-dir", "", "render UI previews, then exit")
	flag.Parse()
	if *renderDir != "" {
		if err := renderClient(*renderDir); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := run(*path, *readOnly); err != nil {
		log.Fatal(err)
	}
}
