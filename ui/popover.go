package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// PopoverWidget opens a themed, focusable panel from a button. The content can
// contain any widgets. Tab can leave the panel; Escape and outside presses close
// it, and focus returns to its trigger. Build one with Popover or PopoverOf.
type PopoverWidget struct {
	props                        property.Owner
	button                       *ButtonWidget
	popup                        *ggui.PopupWidget
	panel                        *ggui.BoxWidget
	width                        float64
	pad                          ggui.EdgeInsets
	padded                       bool
	content                      ggui.Widget
	body                         ggui.Widget
	title, description           string
	builtTitle, builtDescription string
}

// Popover creates a button that toggles content in a 288px floating panel.
// Content can close the panel through PopupOf(env), or through Hide.
func Popover(label string, content ggui.Widget) *PopoverWidget {
	p := &PopoverWidget{width: 288}
	p.button = Button(label, p.Toggle).Outline()
	p.init(content)
	return p
}

// PopoverOf uses arbitrary content as the trigger's face. Name supplies its
// accessible label. The face is rendered inside a button; it is not a second
// interactive trigger.
func PopoverOf(face, content ggui.Widget) *PopoverWidget {
	p := &PopoverWidget{width: 288}
	p.button = ButtonOf(face, p.Toggle).Outline()
	p.init(content)
	return p
}

func (p *PopoverWidget) init(content ggui.Widget) {
	p.content = content
	p.panel = ggui.Box(ggui.Scroll(popoverBody{p}))
	p.popup = ggui.Popup(p.button, popoverPanel{p}).Owner(p.button).Modal(false).Align(ggui.PopupCenter)
	p.button.Expands(p.popup.IsOpen).Opens(p)
}

// Title sets the panel heading and accessible dialog name.
func (p *PopoverWidget) Title(v string) *PopoverWidget {
	defer property.Watch(&p.props, &p.title)()
	p.title = v
	return p
}

// Description adds supporting text and an accessible dialog description.
func (p *PopoverWidget) Description(v string) *PopoverWidget {
	defer property.Watch(&p.props, &p.description)()
	p.description = v
	return p
}

// Trigger returns the button for variant, size, icon and name customization.
func (p *PopoverWidget) Trigger() *ButtonWidget { return p.button }

// Popup returns the underlying popup for lower-level configuration.
func (p *PopoverWidget) Popup() *ggui.PopupWidget { return p.popup }

// Name names the trigger.
func (p *PopoverWidget) Name(name string) *PopoverWidget { p.button.Name(name); return p }

// BindName follows the trigger's accessible name.
func (p *PopoverWidget) BindName(r ggui.Readable[string]) *PopoverWidget {
	p.button.BindName(r)
	return p
}

// Key gives the trigger and popup stable identities across rebuilds.
func (p *PopoverWidget) Key(k any) *PopoverWidget {
	p.button.Key(k)
	p.popup.Key(childKey{owner: k, part: "popover"})
	return p
}

// Width sets the panel's width, constrained to the available room.
func (p *PopoverWidget) Width(w float64) *PopoverWidget {
	defer property.Watch(&p.props, &p.width)()
	p.width = max(0, w)
	return p
}

// Pad sets the panel padding, using the same arguments as ggui.Insets.
func (p *PopoverWidget) Pad(v ...float64) *PopoverWidget {
	defer property.Watch(&p.props, &p.pad)()
	defer property.Watch(&p.props, &p.padded)()
	p.pad, p.padded = ggui.Insets(v...), true
	return p
}

// Modal optionally traps Tab and consumes outside presses. The default is
// nonmodal, allowing keyboard and pointer access to surrounding content.
func (p *PopoverWidget) Modal(v bool) *PopoverWidget { p.popup.Modal(v); return p }

// Align chooses center (the default), logical start, or logical end alignment.
func (p *PopoverWidget) Align(v ggui.PopupAlignment) *PopoverWidget { p.popup.Align(v); return p }

// Gap sets the distance from the trigger to the panel (4px by default).
func (p *PopoverWidget) Gap(v float64) *PopoverWidget { p.popup.Gap(v); return p }

// BindOpen binds visibility, including changes from Escape and outside clicks.
func (p *PopoverWidget) BindOpen(v ggui.Binding[bool]) *PopoverWidget { p.popup.BindOpen(v); return p }

// Open sets visibility and detaches a previous visibility binding.
func (p *PopoverWidget) Open(v bool) *PopoverWidget { p.popup.Open(v && !p.button.IsInert()); return p }

// IsOpen reports whether the panel is open.
func (p *PopoverWidget) IsOpen() bool { return p.popup.IsOpen() }

// Show opens the panel unless its trigger is disabled.
func (p *PopoverWidget) Show() {
	if !p.button.IsInert() {
		p.popup.Show()
	}
}

// Hide closes the panel.
func (p *PopoverWidget) Hide() { p.popup.Hide() }

// Toggle opens or closes the panel unless its trigger is disabled.
func (p *PopoverWidget) Toggle() {
	if !p.button.IsInert() {
		p.popup.Toggle()
	}
}

// OnClose runs after the popup closes through its API or user input.
func (p *PopoverWidget) OnClose(fn func()) *PopoverWidget { p.popup.OnClose(fn); return p }

// Disabled disables the trigger and closes the panel.
func (p *PopoverWidget) Disabled(v bool) *PopoverWidget {
	p.button.Disabled(v)
	if v {
		p.Hide()
	}
	return p
}

// BindDisabled follows a disabled-state reader and closes while disabled.
func (p *PopoverWidget) BindDisabled(v ggui.Readable[bool]) *PopoverWidget {
	p.button.BindDisabled(v)
	return p
}

// Act supports accessible expand and collapse actions on the trigger.
func (p *PopoverWidget) Act(a ggui.Action) bool {
	if p.button.IsInert() {
		return false
	}
	switch a.Kind {
	case ggui.ActionExpand:
		p.Show()
	case ggui.ActionCollapse:
		p.Hide()
	default:
		return false
	}
	return true
}

// Layout implements ggui.Widget.
func (p *PopoverWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer p.props.Layout().EndLayout()
	p.button.Sync()
	if p.button.IsInert() {
		p.Hide()
	}
	t := uitheme.Of(env)
	pad := ggui.Insets(t.Space * 2)
	if p.padded {
		pad = p.pad
	}
	panelBox(p.panel, t).Width(p.width).Padding(pad)
	return p.popup.Layout(c, env)
}

// Paint implements ggui.Widget.
func (p *PopoverWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(p.popup, r) }

// SetName names the trigger for Field.
func (p *PopoverWidget) SetName(name string) { p.button.SetName(name) }

// HasName reports whether the trigger has an accessible name.
func (p *PopoverWidget) HasName() bool { return p.button.HasName() }

// popoverPanel names the floating surface separately from its trigger.
type popoverPanel struct{ p *PopoverWidget }

func (p popoverPanel) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return p.p.panel.Layout(c, env)
}
func (p popoverPanel) Paint(dst *ggui.Canvas, r ggui.Rect) {
	name := p.p.title
	if name == "" {
		name = p.p.button.name()
	}
	dst.Node(r, ggui.Node{Role: ggui.RoleDialog, Name: name, Description: p.p.description}, func(dst *ggui.Canvas) { dst.Paint(p.p.panel, r) })
}

type popoverBody struct{ p *PopoverWidget }

func (b popoverBody) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	p := b.p
	if p.body == nil || p.builtTitle != p.title || p.builtDescription != p.description {
		children := []ggui.Widget{}
		if p.title != "" {
			children = append(children, Heading(p.title))
		}
		if p.description != "" {
			children = append(children, Caption(p.description))
		}
		children = append(children, p.content)
		p.body = ggui.Column(children...).Gap(uitheme.Of(env).Space).Align(ggui.AlignStretch)
		p.builtTitle, p.builtDescription = p.title, p.description
	}
	return p.body.Layout(c, env)
}
func (b popoverBody) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(b.p.body, r) }
