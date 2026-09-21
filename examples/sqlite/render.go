package main

import (
	"database/sql"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

type clientRender struct {
	directory, path string
	done            bool
	err             error
}

func (r *clientRender) Layout(int, int) (int, int) { return 320, 240 }
func (r *clientRender) Update() error {
	if r.err != nil {
		return r.err
	}
	if r.done {
		return ebiten.Termination
	}
	return nil
}
func (r *clientRender) Draw(*ebiten.Image) {
	if r.done {
		return
	}
	r.err = r.render()
	r.done = true
}
func (r *clientRender) render() error {
	now := time.Unix(100, 0)
	restore := ggui.SetClock(func() time.Time { return now })
	defer restore()
	for _, width := range []int{1280, 1000} {
		m := newModel()
		m.Dark.Set(false)
		p := ggui.ProbeBuilder(func() ggui.Widget { return ggui.Provide(ggui.ReducedMotionKey, true, build(m)) }, ggui.Sz(float64(width), 900))
		p.Setup(func() { ggui.BindTheme(m.Dark, clientTheme(true), clientTheme(false)) })
		save := func(name string) error {
			p.Frame()
			now = now.Add(time.Second)
			p.Frame()
			img := ebiten.NewImage(width, 900)
			defer img.Deallocate()
			img.Fill(ggui.Untrack(ggui.UseTheme).Bg)
			p.Draw(img)
			f, err := os.Create(filepath.Join(r.directory, fmt.Sprintf("%d-%s.png", width, name)))
			if err != nil {
				return err
			}
			err = png.Encode(f, img)
			closeErr := f.Close()
			if err != nil {
				return err
			}
			return closeErr
		}
		for _, name := range []string{"welcome", "data", "filters", "structure", "query", "edit", "insert", "dark", "dark-welcome"} {
			switch name {
			case "data":
				m.open(r.path)
				m.selectTable("products")
			case "filters":
				m.FilterOpen.Set(true)
			case "dark-welcome":
				m.Connected.Set(false)
				m.Path.Set("")
				m.Status.Set("Open a SQLite database to get started.")
			case "structure":
				m.FilterOpen.Set(false)
				m.Tab.Set(1)
			case "query":
				m.SQL.Set("SELECT category, count(*) AS products, round(avg(price),2) AS avg_price FROM products GROUP BY category")
				m.execute()
			case "edit":
				m.Tab.Set(0)
				m.browse()
				m.Selected.Set(1)
				m.edit()
			case "insert":
				m.Editing.Set(false)
				m.add()
			case "dark":
				m.Adding.Set(false)
				m.Dark.Set(true)
			}
			if err := save(name); err != nil {
				p.Close()
				m.close()
				return err
			}
		}
		p.Close()
		m.close()
	}
	return nil
}
func renderClient(directory string) error {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "ggui-sqlite-preview-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	path := filepath.Join(tmp, "preview.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE products(id INTEGER PRIMARY KEY,name TEXT NOT NULL,category TEXT NOT NULL,price REAL NOT NULL DEFAULT 0,stock INTEGER DEFAULT 0,notes TEXT); INSERT INTO products VALUES(1,'Mechanical keyboard','Accessories',129,42,'Hot-swappable switches'),(2,'Studio monitor','Displays',449,12,NULL),(3,'USB-C dock','Accessories',89,65,'Dual display support'),(4,'Desk lamp','Workspace',59.5,31,'Warm white'),(5,'Notebook','Stationery',12,200,NULL),(6,'Wireless mouse','Accessories',79,54,'Rechargeable'); CREATE TABLE orders(id INTEGER PRIMARY KEY,product_id INTEGER REFERENCES products(id),quantity INTEGER NOT NULL); CREATE VIEW low_stock AS SELECT * FROM products WHERE stock<20;`)
	db.Close()
	if err != nil {
		return err
	}
	ebiten.SetWindowSize(320, 240)
	ebiten.SetWindowTitle("SQLite layout previews")
	return ebiten.RunGame(&clientRender{directory: directory, path: path})
}
