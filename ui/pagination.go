package ui

import (
	"fmt"
	"github.com/ironpark/ggui"
)

// PaginationWidget navigates a one-based page binding. It does not slice data.
// At most five numbered buttons are shown, plus Previous, Next and ellipses for hidden pages.
type PaginationWidget struct {
	page           ggui.Binding[int]
	pages          ggui.Reader[int]
	previous, next *ButtonWidget
	numbers        [5]*ButtonWidget
	targets        [5]int
	row            *ggui.WrapWidget
	disabled       bool
	onChange       func(int)
}

// Pagination creates navigation for pages pages. Zero or negative counts disable it.
// Out-of-range page values are clamped for display, without writing the binding.
func Pagination(page ggui.Binding[int], pages ggui.Reader[int]) *PaginationWidget {
	p := &PaginationWidget{page: page, pages: pages}
	p.previous = Button("‹ Previous", func() { p.move(-1) }).Label("Previous").Ghost().Pad(6, 10)
	p.next = Button("Next ›", func() { p.move(1) }).Label("Next").Ghost().Pad(6, 10)
	for i := range p.numbers {
		p.numbers[i] = Button("", func() { p.selectPage(p.targets[i]) })
	}
	return p
}

// Disabled disables all navigation.
func (p *PaginationWidget) Disabled(v bool) *PaginationWidget {
	p.disabled = v
	n := max(p.pages.Get(), 0)
	current := p.current(n)
	p.previous.Disabled(v || n == 0 || current == 1)
	p.next.Disabled(v || n == 0 || current == n)
	for _, b := range p.numbers {
		b.Disabled(v)
	}
	return p
}

// OnChange runs after a user selects a different page.
func (p *PaginationWidget) OnChange(fn func(int)) *PaginationWidget { p.onChange = fn; return p }

func (p *PaginationWidget) current(n int) int { return min(max(p.page.Peek(), 1), max(n, 1)) }
func (p *PaginationWidget) move(delta int)    { p.selectPage(p.current(max(p.pages.Get(), 0)) + delta) }
func (p *PaginationWidget) selectPage(page int) {
	n := max(p.pages.Get(), 0)
	if !p.disabled && page >= 1 && page <= n {
		setChanged(p.page, page, p.onChange)
	}
}

// Layout implements ggui.Widget.
func (p *PaginationWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	n := max(p.pages.Get(), 0)
	current := p.current(n)
	p.previous.Disabled(p.disabled || n == 0 || current == 1)
	p.next.Disabled(p.disabled || n == 0 || current == n)
	children := []ggui.Widget{p.previous}
	start := max(1, min(current-2, n-4))
	if start > 1 {
		children = append(children, ggui.Padding(ggui.Text("…"), 6, 4))
	}
	for i := range min(n, len(p.numbers)) {
		target := start + i
		p.targets[i] = target
		b := p.numbers[i]
		b.label.Set(fmt.Sprint(target))
		b.SetName(fmt.Sprintf("Page %d", target))
		b.selected = target == current
		if target == current {
			b.Outline()
		} else {
			b.Ghost()
		}
		b.Disabled(p.disabled).Pad(6, 10)
		children = append(children, b)
	}
	if start+len(p.numbers) <= n {
		children = append(children, ggui.Padding(ggui.Text("…"), 6, 4))
	}
	children = append(children, p.next)
	p.row = ggui.Wrap(children...).Gap(env.Theme().Space / 2)
	return p.row.Layout(c, env)
}

// Paint implements ggui.Widget.
func (p *PaginationWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(p.row, r) }
