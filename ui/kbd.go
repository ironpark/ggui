package ui

import "github.com/ironpark/ggui"

// KbdWidget displays a keyboard shortcut without registering an input handler.
type KbdWidget struct {
	text *ggui.TextWidget
	box  *ggui.BoxWidget
}

// Kbd creates a key cap. Compose multiple caps with Row for a chord.
func Kbd(label string) *KbdWidget {
	text := ggui.Text(label).NoWrap()
	return &KbdWidget{text: text, box: ggui.Box(text)}
}

// Layout implements ggui.Widget.
func (k *KbdWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	k.text.Style(t.Caption).Color(t.Fg)
	k.box.Pad(2, t.Space*.75).Fill(t.Surface).Border(1, t.Border).Radius(t.Radius / 2)
	return k.box.Layout(c, env)
}

// Paint implements ggui.Widget.
func (k *KbdWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(k.box, r) }
