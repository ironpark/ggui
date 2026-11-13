package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// LabelWidget provides a themed label and an optional association with a control.
// Associated labels name unnamed controls and activate or focus them on click.
type LabelWidget struct {
	ggui.Interactive
	props   property.Owner
	text    *ggui.TextWidget
	content string
	target  ggui.Widget
	pending bool
	theme   *uitheme.Theme
}

// Label creates a medium-weight form label. For associates it with a control.
func Label(text string) *LabelWidget {
	l := &LabelWidget{text: ggui.Text(text), content: text}
	l.Role = ggui.RoleText
	l.SetName(text)
	l.AutoKey()
	return l
}

// For associates the label with a control without adding another Tab stop.
// The target should be painted in the same frame as this label.
func (l *LabelWidget) For(target ggui.Widget) *LabelWidget {
	l.target = target
	if n, ok := target.(Named); ok && !n.HasName() {
		n.SetName(l.content)
	}
	l.props.Changed()
	return l
}

// Disabled dims the label and prevents activation.
func (l *LabelWidget) Disabled(v bool) *LabelWidget { l.SetInert(v); return l }

// BindDisabled follows the disabled state of a form.
func (l *LabelWidget) BindDisabled(r ggui.Readable[bool]) *LabelWidget { l.BindInert(r); return l }

func (l *LabelWidget) disabled() bool {
	if l.IsInert() {
		return true
	}
	if n, ok := l.target.(interface{ IsInert() bool }); ok {
		return n.IsInert()
	}
	return false
}

// Layout implements ggui.Widget.
func (l *LabelWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer l.props.Layout().EndLayout()
	l.Sync()
	l.theme = uitheme.Of(env)
	l.text.Style(l.theme.Text.Merge(l.theme.Label)).Color(pick(l.disabled(), l.theme.MutedFg, l.theme.Fg))
	return l.text.Layout(c, env)
}

// Baseline implements ggui.Baseliner.
func (l *LabelWidget) Baseline() (float64, bool) { return l.text.Baseline() }

func (l *LabelWidget) focusTarget() ggui.KeyHandler {
	switch target := l.target.(type) {
	case *TextFieldWidget:
		return target.input
	case *NumberInputWidget:
		return target.input
	case *InputGroupWidget:
		if h, ok := target.input.(ggui.KeyHandler); ok {
			return h
		}
	}
	h, _ := l.target.(ggui.KeyHandler)
	return h
}

// Paint implements ggui.Widget.
func (l *LabelWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	// Keep a single text node and no separate label Tab stop.
	dst.Paint(l.text, r)
	if l.target != nil && !l.disabled() {
		dst.HitPointer(r, l)
	}
	if l.pending {
		if !l.disabled() {
			if h := l.focusTarget(); h != nil {
				dst.RequestFocus(h)
			}
		}
		l.pending = false
	}
}

// HandlePointer activates a toggle control or focuses an editor.
func (l *LabelWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if l.disabled() || l.target == nil {
		return false
	}
	return l.Pointer(ev, func() {
		l.pending = true
		if d, ok := l.target.(ggui.Describer); ok {
			switch d.Describe().Role {
			case ggui.RoleCheckbox, ggui.RoleRadio, ggui.RoleSwitch, ggui.RoleSelect, ggui.RoleButton:
				if a, ok := l.target.(ggui.Actor); ok && a.Act(ggui.Action{Kind: ggui.ActionPress}) {
					return
				}
				if h := l.focusTarget(); h != nil {
					h.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeySpace})
				}
			}
		}
	})
}

// Key assigns input identity to an associated label across rebuilds.
func (l *LabelWidget) Key(key any) *LabelWidget { l.SetKey(key); return l }
