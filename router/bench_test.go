package router

import (
	"fmt"
	"testing"

	"github.com/ironpark/ggui"
)

// An app's route tree of n sections, each with a list, an item, an item's
// editor, a catch-all and a redirect, under a layout, with a NotFound.
func benchRouter(n int) *Router {
	page := func(*Context) ggui.Widget { return nil }
	var sections []Route
	for i := range n {
		sections = append(sections, Group(fmt.Sprintf("/section%d", i),
			Page("/", page),
			Page("/items/:id", page),
			Page("/items/:id/edit", page),
			Page("/files/*path", page),
			Redirect("/old", fmt.Sprintf("/section%d", i)),
			NotFound(page),
		))
	}
	return New(Layout("", func(_ *Context, p ggui.Widget) ggui.Widget { return p }, sections...), NotFound(page))
}

// Matching a parsed URL against the tree, as each navigation does, for a
// route deep in the last section and one that matches nothing.
func BenchmarkMatch(b *testing.B) {
	for _, n := range []int{10, 100} {
		r := benchRouter(n)
		prefix := fmt.Sprintf("/section%d", n-1)
		for _, sfx := range []string{"/items/42/edit", "/files/a/b/c.txt", "/old", "/nowhere"} {
			u, err := parseURL(prefix + sfx)
			if err != nil {
				b.Fatal(err)
			}
			b.Run(fmt.Sprint(n)+sfx, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, leaf, _, _ := r.match(u); leaf == nil {
						b.Fatal("no route")
					}
				}
			})
		}
	}
}
