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
	body                         *ggui.ColumnWidget
	header                       *ggui.ColumnWidget
	heading, text                *ggui.TextWidget
	title, description           string
	builtTitle, builtDescription string
}

// popoverWidth is a popover's width until Width says otherwise: shadcn's
// w-72.
const popoverWidth = 288

// Popover creates a button that toggles content in a 288px floating panel.
// Content can close the panel through PopupOf(env), or through Hide.
func Popover(label string, content ggui.Widget) *PopoverWidget {
	p := &PopoverWidget{width: popoverWidth}
	p.button = Button(label, p.Toggle).Outline()
	p.init(content)
	return p
}

// PopoverOf uses arbitrary content as the trigger's face. Name supplies its
// accessible label. The face is rendered inside a button; it is not a second
// interactive trigger.
func PopoverOf(face, content ggui.Widget) *PopoverWidget {
	p := &PopoverWidget{width: popoverWidth}
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

// Align chooses center (the default), logical start, or logical end
// alignment along the trigger's side.
func (p *PopoverWidget) Align(v ggui.PopupAlignment) *PopoverWidget { p.popup.Align(v); return p }

// Side sets the side of the trigger the panel opens on, ggui.PopupBottom by
// default. It opens on the opposite side when only that one has the room.
func (p *PopoverWidget) Side(v ggui.PopupSide) *PopoverWidget { p.popup.Side(v); return p }

// AlignOffset moves the panel along the trigger's side, away from the edge
// Align lines it up with; see ggui.PopupWidget.AlignOffset.
func (p *PopoverWidget) AlignOffset(v float64) *PopoverWidget { p.popup.AlignOffset(v); return p }

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
	pad := ggui.Insets(t.Space * 1.25)
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
	t := uitheme.Of(env)
	if p.body == nil || p.builtTitle != p.title || p.builtDescription != p.description {
		// The title and the description are one block, closer to each
		// other than the block is to the content.
		var lines, children []ggui.Widget
		p.heading, p.text, p.header = nil, nil, nil
		if p.title != "" {
			p.heading = ggui.Text(p.title).Role(ggui.RoleHeading)
			lines = append(lines, p.heading)
		}
		if p.description != "" {
			p.text = ggui.Text(p.description)
			lines = append(lines, p.text)
		}
		if len(lines) > 0 {
			p.header = ggui.Column(lines...).Align(ggui.AlignStretch)
			children = append(children, p.header)
		}
		children = append(children, p.content)
		p.body = ggui.Column(children...).Align(ggui.AlignStretch)
		p.builtTitle, p.builtDescription = p.title, p.description
	}
	if p.heading != nil {
		title := t.Text
		title.Weight, title.Color = ggui.WeightMedium, t.PopoverFg
		p.heading.Style(title)
	}
	if p.text != nil {
		p.text.Style(mutedText(t))
	}
	if p.header != nil {
		p.header.Gap(t.Space / 4)
	}
	p.body.Gap(t.Space * 1.25)
	return p.body.Layout(c, env)
}
func (b popoverBody) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(b.p.body, r) }
