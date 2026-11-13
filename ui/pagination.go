package ui

import (
	"math"
	"strconv"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// PaginationWidget navigates a one-based page binding. It does not slice data.
// At most five numbered buttons are shown, plus Previous, Next and ellipses for hidden pages.
type PaginationWidget struct {
	ggui.Interactive
	page                     ggui.Binding[int]
	pages                    ggui.Readable[int]
	previous, next           *ButtonWidget
	previousIcon, nextIcon   *icons.Widget
	previousLabel, nextLabel *ggui.TextWidget
	numbers                  [5]*ButtonWidget
	targets                  [5]int
	gaps                     [2]ggui.Widget // the leading and trailing ellipses
	children                 []ggui.Widget
	row                      *ggui.WrapWidget
	shape                    paginationShape // what row was built for
	onChange                 func(int)
}

// paginationShape is which of its parts a pager shows: enough to tell
// whether the row built for one layout serves the next.
type paginationShape struct {
	lead, trail bool
	numbers     int
}

// Pagination creates navigation for pages pages. Zero or negative counts disable it.
// Out-of-range page values are clamped for display, without writing the binding.
func Pagination(page ggui.Binding[int], pages ggui.Readable[int]) *PaginationWidget {
	p := &PaginationWidget{page: page, pages: pages}
	p.previousIcon, p.nextIcon = Icon(icons.ChevronLeft), Icon(icons.ChevronRight)
	p.previousLabel, p.nextLabel = ggui.Text("Previous").NoWrap(), ggui.Text("Next").NoWrap()
	p.previous = ButtonOf(ggui.Row(p.previousIcon, p.previousLabel).Gap(paginationGap), func() { p.move(-1) }).Name("Previous").Ghost()
	p.next = ButtonOf(ggui.Row(p.nextLabel, p.nextIcon).Gap(paginationGap), func() { p.move(1) }).Name("Next").Ghost()
	for i := range p.numbers {
		p.numbers[i] = Button("", func() { p.selectPage(p.targets[i]) }).Size(ButtonIcon)
	}
	for i := range p.gaps {
		p.gaps[i] = &moreIcon{icon: Icon(icons.MoreHorizontal).Alt("More pages"), side: func(t *uitheme.Theme) float64 { return ButtonIcon.Style(*t).Height }}
	}
	return p
}

// paginationGap is between the chevron and the word of Previous and Next,
// the gap shadcn's button puts between an icon and its label.
const paginationGap = 6.0

// Labels replaces the words on the Previous and Next buttons, which also
// name them, for an application in another language.
func (p *PaginationWidget) Labels(previous, next string) *PaginationWidget {
	p.previousLabel.Content(previous)
	p.nextLabel.Content(next)
	p.previous.Name(previous)
	p.next.Name(next)
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
	t := uitheme.Of(env)
	// Previous and Next are default-size buttons with less padding on the
	// side their chevron is on.
	pad, near := t.ButtonPad, t.ButtonPad.Left*.6
	if env.Direction() == ggui.DirRTL {
		p.previous.Pad(pad.Top, near, pad.Bottom, pad.Left)
		p.next.Pad(pad.Top, pad.Right, pad.Bottom, near)
	} else {
		p.previous.Pad(pad.Top, pad.Right, pad.Bottom, near)
		p.next.Pad(pad.Top, near, pad.Bottom, pad.Left)
	}
	angle := pick(env.Direction() == ggui.DirRTL, math.Pi, 0.0)
	p.previousIcon.Rotate(angle)
	p.nextIcon.Rotate(angle)
	n := max(p.pages.Get(), 0)
	current := p.current(n)
	p.previous.Disabled(p.IsInert() || n == 0 || current == 1)
	p.next.Disabled(p.IsInert() || n == 0 || current == n)
	start := max(1, min(current-2, n-4))
	shape := paginationShape{lead: start > 1, trail: start+len(p.numbers) <= n, numbers: min(n, len(p.numbers))}
	for i := range shape.numbers {
		target := start + i
		b := p.numbers[i]
		if p.targets[i] != target {
			p.targets[i] = target
			b.label.Content(strconv.Itoa(target))
			b.SetName("Page " + strconv.Itoa(target))
		}
		b.selected = target == current
		if target == current {
			b.Outline()
		} else {
			b.Ghost()
		}
		b.Disabled(p.IsInert())
	}
	if p.row == nil || shape != p.shape {
		children := append(p.children[:0], p.previous)
		if shape.lead {
			children = append(children, p.gaps[0])
		}
		for i := range shape.numbers {
			children = append(children, p.numbers[i])
		}
		if shape.trail {
			children = append(children, p.gaps[1])
		}
		p.children = append(children, p.next)
		p.row, p.shape = ggui.Wrap(p.children...).Align(ggui.AlignCenter), shape
	}
	return p.row.Gap(t.Space/4).Layout(c, env)
}

// Paint implements ggui.Widget. The buttons are one group, named by Name
// or else "Pagination", so a screen reader reads them as one navigation.
func (p *PaginationWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	name := p.SemanticName()
	if name == "" {
		name = "Pagination"
	}
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup, Name: name}, func(dst *ggui.Canvas) {
		dst.Paint(p.row, r)
	})
}

// BindDisabled follows r for all navigation buttons without rebuilding.
func (p *PaginationWidget) BindDisabled(r ggui.Readable[bool]) *PaginationWidget {
	p.BindInert(r)
	return p
}
