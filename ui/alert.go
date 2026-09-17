package ui

import "github.com/ironpark/ggui"

// AlertWidget is an inline notice with a title, description and optional action.
type AlertWidget struct {
	title, description *ggui.TextWidget
	action             ggui.Widget
	destructive        bool
	box                *ggui.BoxWidget
}

// Alert creates an inline notice. It does not interrupt focus or open a modal.
func Alert(title, description string) *AlertWidget {
	return &AlertWidget{title: ggui.Text(title), description: ggui.Text(description)}
}

// Destructive uses the theme's DangerColor for the heading and border.
func (a *AlertWidget) Destructive() *AlertWidget { a.destructive = true; return a }

// Action places a widget below the notice, such as a retry button.
func (a *AlertWidget) Action(w ggui.Widget) *AlertWidget { a.action = w; return a }

// Layout implements ggui.Widget.
func (a *AlertWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	fg, border := t.Fg, t.Border
	if a.destructive {
		fg, border = dangerColor(t), mix(t.Border, dangerColor(t), .3)
	}
	a.title.Style(t.Text).Color(fg)
	a.description.Style(t.Text).Color(t.Muted)
	parts := []ggui.Widget{a.title, a.description}
	if a.action != nil {
		parts = append(parts, a.action)
	}
	a.box = ggui.Box(ggui.Column(parts...).Gap(t.Space/2)).Pad(16).Fill(t.Surface).Border(1, border).Radius(t.Radius)
	return a.box.Layout(c, env)
}

// Paint implements ggui.Widget.
func (a *AlertWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(a.box, r) }
