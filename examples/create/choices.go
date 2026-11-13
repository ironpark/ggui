package main

import (
	"image/color"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/theme"
)

const (
	lockStyle = iota
	lockBase
	lockAccent
	lockChart
	lockHeading
	lockFont
	lockIcons
	lockRadius
	lockMenu
	lockCount
)

// A tile remains a real Select: the custom face changes its presentation only.
// The lock is a sibling action, not an interactive child of the select node.
func choice[T comparable](m *model, slot int, label string, value ggui.Binding[T], options []T, format func(T) string) ggui.Widget {
	face := ggui.Row(
		ggui.Expanded(ggui.Column(
			ggui.Text(label).Size(12).Color(theme.MutedFg).NoWrap(),
			ggui.TextOf(ggui.Map(value, format)).Size(14).Weight(ggui.WeightMedium).NoWrap(),
		).Gap(2).Stretch()),
		ggui.Box().Width(22), tileMark(m, slot),
	).Gap(4).Align(ggui.AlignCenter)
	selectWidget := ui.Select(value).Options(options).Format(format).Name(label).
		AlignItemWithTrigger(false).TriggerContent(face).Key("setting-" + label)
	styled := theme.Override(func(t *theme.Theme) {
		t.FieldPad = ggui.Insets(8, 10)
		t.Radius = 10
	}, selectWidget)
	lock := ui.ButtonOf(lockMark(m, slot), func() {
		v := ggui.Peek(m.locked)
		v[slot] = !v[slot]
		m.locked.Set(v)
	}).BindName(ggui.Map(m.locked, func(v [lockCount]bool) string {
		if v[slot] {
			return "Unlock " + label
		}
		return "Lock " + label
	})).Key("lock-" + label).Ghost().Pad(4).Radius(5)
	tooltip := ui.TooltipText(lock, func() string {
		if ggui.Peek(m.locked)[slot] {
			return "Unlock " + label + " for shuffle"
		}
		return "Keep " + label + " when shuffling"
	})
	var hoverInk color.Color
	return ggui.FromFuncs(func(c ggui.Constraints, env ggui.Env) ggui.Size {
		hoverInk = theme.Fade(theme.Of(env).Fg, .045)
		size := styled.Layout(c, env)
		tooltip.Layout(ggui.Tight(ggui.Sz(24, 24)), env)
		return size
	}, func(dst *ggui.Canvas, r ggui.Rect) {
		dst.Paint(styled, r)
		if selectWidget.Hovered {
			dst.FillRoundRect(r, 10, hoverInk)
		}
		if selectWidget.Hovered || selectWidget.Focused || lock.Hovered || lock.Focused || ggui.Peek(m.locked)[slot] {
			dst.Paint(tooltip, ggui.Rct(ggui.Pt(r.Origin.X+r.Size.W-52, r.Origin.Y+(r.Size.H-24)/2), ggui.Sz(24, 24)))
		}
	})
}

func tileMark(m *model, slot int) ggui.Widget {
	if slot == lockFont || slot == lockHeading {
		return ggui.Text("Aa").Size(16).NoWrap()
	}
	var ink color.Color
	return ggui.FromFuncs(func(c ggui.Constraints, env ggui.Env) ggui.Size {
		ink = theme.Of(env).Fg
		s := m.settings.Get()
		switch slot {
		case lockBase:
			ink = (theme.Preset{Base: s.Theme.Preset.Base}).For(s.dark()).MutedFg
		case lockAccent:
			ink = s.Theme.For(s.dark()).Primary
			if s.Theme.Preset.Accent == theme.AccentBase {
				ink = (theme.Preset{}).For(s.dark()).MutedFg
			}
		case lockChart:
			ink = s.Theme.For(s.dark()).Chart[1]
		}
		return c.Constrain(ggui.Sz(16, 16))
	}, func(dst *ggui.Canvas, r ggui.Rect) {
		x, y := r.Origin.X, r.Origin.Y
		dst = dst.Clip(r)
		switch slot {
		case lockBase, lockAccent, lockChart:
			dst.FillCircle(r.Center(), 7, ink)
		case lockStyle:
			dst.StrokeRoundRect(tileRect(x+2, y+3, 12, 10), 3, 1.4, ink)
		case lockRadius:
			dst.StrokeRoundRect(tileRect(x+2, y+2, 22, 22), 9, 1.4, ink)
		case lockIcons:
			dst.StrokeRoundRect(tileRect(x+1, y+1, 14, 14), 7, 1.2, ink)
			dst.StrokeRoundRect(tileRect(x+4, y+4, 8, 8), 4, 1.2, ink)
		default:
			for i := 0; i < 3; i++ {
				dst.StrokeLine(ggui.Pt(x+2, y+3+float64(i)*5), ggui.Pt(x+14-float64(i)*3, y+3+float64(i)*5), 1.4, ink)
			}
		}
	})
}

func lockMark(m *model, slot int) ggui.Widget {
	var ink color.Color
	return ggui.FromFuncs(func(c ggui.Constraints, env ggui.Env) ggui.Size {
		ink = theme.Of(env).MutedFg
		if m.locked.Get()[slot] {
			ink = theme.Of(env).Fg
		}
		return c.Constrain(ggui.Sz(16, 16))
	}, func(dst *ggui.Canvas, r ggui.Rect) {
		x, y := r.Origin.X, r.Origin.Y
		dst.StrokeRoundRect(tileRect(x+3, y+7, 10, 8), 2, 1.3, ink)
		shackle := tileRect(x+5, y+1, 6, 10)
		if !ggui.Peek(m.locked)[slot] {
			shackle.Origin.X += 3
		}
		dst.Clip(tileRect(x, y, 16, 8)).StrokeRoundRect(shackle, 3, 1.3, ink)
		dst.StrokeLine(ggui.Pt(x+8, y+10), ggui.Pt(x+8, y+12), 1.3, ink)
	})
}

func menuMark() ggui.Widget {
	var ink color.Color
	return ggui.FromFuncs(func(c ggui.Constraints, env ggui.Env) ggui.Size {
		ink = theme.Of(env).Fg
		return c.Constrain(ggui.Sz(16, 16))
	}, func(dst *ggui.Canvas, r ggui.Rect) {
		for _, y := range []float64{5, 11} {
			dst.StrokeLine(r.Origin.Add(ggui.Pt(2, y)), r.Origin.Add(ggui.Pt(14, y)), 1.5, ink)
		}
	})
}

func tileRect(x, y, w, h float64) ggui.Rect { return ggui.Rct(ggui.Pt(x, y), ggui.Sz(w, h)) }
