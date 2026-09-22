package ui

import (
	"fmt"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// PaginationWidget navigates a one-based page binding. It does not slice data.
// At most five numbered buttons are shown, plus Previous, Next and ellipses for hidden pages.
type PaginationWidget struct {
	ggui.Interactive
	page           ggui.Binding[int]
	pages          ggui.Readable[int]
	previous, next *ButtonWidget
	numbers        [5]*ButtonWidget
	targets        [5]int
	gaps           [2]ggui.Widget // the leading and trailing ellipses
	children       []ggui.Widget
	row            *ggui.WrapWidget
	onChange       func(int)
}

// Pagination creates navigation for pages pages. Zero or negative counts disable it.
// Out-of-range page values are clamped for display, without writing the binding.
func Pagination(page ggui.Binding[int], pages ggui.Readable[int]) *PaginationWidget {
	p := &PaginationWidget{page: page, pages: pages}
	p.previous = ButtonOf(ggui.Row(Icon(icons.ChevronLeft), ggui.Text("Previous")).Gap(4), func() { p.move(-1) }).Name("Previous").Ghost().Pad(6, 10)
	p.next = ButtonOf(ggui.Row(ggui.Text("Next"), Icon(icons.ChevronRight)).Gap(4), func() { p.move(1) }).Name("Next").Ghost().Pad(6, 10)
	for i := range p.numbers {
		p.numbers[i] = Button("", func() { p.selectPage(p.targets[i]) })
	}
	for i := range p.gaps {
		p.gaps[i] = ggui.Padding(ggui.Text("…"), 6, 4)
	}
	return p
}

// Disabled disables all navigation.
func (p *PaginationWidget) Disabled(v bool) *PaginationWidget {
	p.SetInert(v)
	return p
}

// OnChange runs after a user selects a different page.
func (p *PaginationWidget) OnChange(fn func(int)) *PaginationWidget { p.onChange = fn; return p }

func (p *PaginationWidget) current(n int) int {
	return min(max(ggui.Untrack(p.page.Get), 1), max(n, 1))
}
func (p *PaginationWidget) move(delta int) { p.selectPage(p.current(max(p.pages.Get(), 0)) + delta) }
func (p *PaginationWidget) selectPage(page int) {
	n := max(p.pages.Get(), 0)
	if !p.IsInert() && page >= 1 && page <= n {
		setChanged(p.page, page, p.onChange)
	}
}

// Layout implements ggui.Widget.
func (p *PaginationWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	p.Sync()
	n := max(p.pages.Get(), 0)
	current := p.current(n)
	p.previous.Disabled(p.IsInert() || n == 0 || current == 1)
	p.next.Disabled(p.IsInert() || n == 0 || current == n)
	children := append(p.children[:0], p.previous)
	start := max(1, min(current-2, n-4))
	if start > 1 {
		children = append(children, p.gaps[0])
	}
	for i := range min(n, len(p.numbers)) {
		target := start + i
		p.targets[i] = target
		b := p.numbers[i]
		b.label.Content(fmt.Sprint(target))
		b.SetName(fmt.Sprintf("Page %d", target))
		b.selected = target == current
		if target == current {
			b.Outline()
		} else {
			b.Ghost()
		}
		b.Disabled(p.IsInert()).Pad(6, 10)
		children = append(children, b)
	}
	if start+len(p.numbers) <= n {
		children = append(children, p.gaps[1])
	}
	p.children = append(children, p.next)
	p.row = ggui.Wrap(p.children...).Gap(uitheme.From(env).Space / 2)
	return p.row.Layout(c, env)
}

// Paint implements ggui.Widget.
func (p *PaginationWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(p.row, r) }

// BindDisabled follows r for all navigation buttons without rebuilding.
func (p *PaginationWidget) BindDisabled(r ggui.Readable[bool]) *PaginationWidget {
	p.BindInert(r)
	return p
}
