package ui

import (
	"image/color"
	"math"
	"slices"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// NavigationMenuItemWidget is a navigation link or dropdown trigger. Build it
// with NavigationMenuLink, NavigationMenuLinkOf, or NavigationMenuItem.
// Links can also be used inside arbitrary dropdown layouts.
type NavigationMenuItemWidget struct {
	*ButtonWidget
	content      ggui.Widget
	panel        *ggui.BoxWidget
	owner        *NavigationMenuWidget
	popup        *ggui.PopupWidget
	onNavigate   func()
	width        float64
	activeProp   property.Value[bool]
	closeOnClick bool
	rect         ggui.Rect
	inContent    bool                  // a link inside a dropdown's panel, as laid out
	panelSize    ggui.Size             // the dropdown's panel at its own size, as laid out
	menu         *NavigationMenuWidget // the menu whose panel this link is in, as laid out
	quiet        bool                  // focus came here with a panel the pointer opened
}

// navigationMenuOwnerKey tells a link which menu's panel it is laid out in.
var navigationMenuOwnerKey = ggui.NewEnvKey[*NavigationMenuWidget]("navigation menu")

const (
	navigationMenuChevron = 12.0                   // the trigger's chevron, shadcn's size-3
	navigationMenuTurn    = 300 * time.Millisecond // the chevron turning over
	navigationMenuMotion  = 350 * time.Millisecond // the panel arriving, moving and changing content
	navigationMenuExit    = 150 * time.Millisecond // the panel leaving
	navigationMenuFrom    = .9                     // the panel's scale as it arrives
	navigationMenuDelay   = 50 * time.Millisecond  // resting on a trigger before its dropdown opens: Base UI's delay
	navigationMenuLinger  = 50 * time.Millisecond  // off the menu before a dropdown hover opened closes: its closeDelay
)

// navigationMenuEase is the curve the panel moves along: shadcn's
// cubic-bezier(0.22, 1, 0.36, 1), an ease-out-quint.
var navigationMenuEase = ggui.CubicBezier(.22, 1, .36, 1)

var (
	navigationHoverSlot  = ggui.NewSlot[*ggui.Motion]("navigation menu hover")
	navigationActiveSlot = ggui.NewSlot[*ggui.Motion]("navigation menu active")
	navigationTurnSlot   = ggui.NewSlot[*ggui.Motion]("navigation menu chevron")
)

// NavigationMenuLink creates a real navigation link with a native navigation
// callback (for routing or opening a URL). CloseOnClick optionally dismisses its popup.
func NavigationMenuLink(label string, onNavigate func()) *NavigationMenuItemWidget {
	item := &NavigationMenuItemWidget{onNavigate: onNavigate}
	item.ButtonWidget = Button(label, item.activate).Ghost().Size(ButtonLarge)
	item.box = ggui.Box(&navigationMenuLinkFace{item: item, child: item.label})
	item.Role = ggui.RoleLink
	return item
}

// NavigationMenuLinkOf uses custom link content, such as a title and description.
// Supply its accessible label with Name.
func NavigationMenuLinkOf(content ggui.Widget, onNavigate func()) *NavigationMenuItemWidget {
	item := &NavigationMenuItemWidget{onNavigate: onNavigate}
	item.ButtonWidget = ButtonOf(content, item.activate).Ghost().Size(ButtonLarge)
	item.box = ggui.Box(&navigationMenuLinkFace{item: item, child: content})
	item.Role = ggui.RoleLink
	return item
}

// NavigationMenuItem creates a dropdown with arbitrary content. Compose its
// contents from NavigationMenuLink, text, columns, or other native widgets.
func NavigationMenuItem(label string, content ggui.Widget) *NavigationMenuItemWidget {
	item := &NavigationMenuItemWidget{content: content, width: 320}
	item.ButtonWidget = Button(label, item.activate).Ghost().Size(ButtonLarge)
	item.box = ggui.Box(&buttonContent{child: &navigationMenuFace{item: item}})
	item.panel = ggui.Box(ggui.Scroll(content))
	item.Expands(item.isOpen).Opens(item)
	return item
}

// Name sets the link or trigger's accessible name.
func (i *NavigationMenuItemWidget) Name(name string) *NavigationMenuItemWidget {
	i.ButtonWidget.Name(name)
	return i
}

// BindName follows the link or trigger's accessible name.
func (i *NavigationMenuItemWidget) BindName(r ggui.Readable[string]) *NavigationMenuItemWidget {
	i.ButtonWidget.BindName(r)
	return i
}

// Key gives this link or trigger a stable input identity.
func (i *NavigationMenuItemWidget) Key(k any) *NavigationMenuItemWidget {
	i.ButtonWidget.Key(k)
	return i
}

// Disabled disables this entry and closes its dropdown if open.
func (i *NavigationMenuItemWidget) Disabled(v bool) *NavigationMenuItemWidget {
	i.ButtonWidget.Disabled(v)
	if v && i.isOpen() {
		i.owner.popup.Hide()
	}
	return i
}

// BindDisabled follows a disabled-state reader.
func (i *NavigationMenuItemWidget) BindDisabled(v ggui.Readable[bool]) *NavigationMenuItemWidget {
	i.ButtonWidget.BindDisabled(v)
	return i
}

// Active marks a link as the current destination, visually and semantically.
func (i *NavigationMenuItemWidget) Active(v bool) *NavigationMenuItemWidget {
	property.Set(&i.ButtonWidget.props, &i.activeProp, v)
	i.selected = v
	return i
}

// BindActive follows whether this link is the current destination.
func (i *NavigationMenuItemWidget) BindActive(r ggui.Readable[bool]) *NavigationMenuItemWidget {
	property.Bind(&i.ButtonWidget.props, &i.activeProp, r, "BindActive")
	return i
}

// Width sets this entry's dropdown width (320px by default).
func (i *NavigationMenuItemWidget) Width(w float64) *NavigationMenuItemWidget {
	defer property.Watch(&i.ButtonWidget.props, &i.width)()
	i.width = max(w, 0)
	return i
}

// CloseOnClick closes the enclosing dropdown when this link is activated.
// The default is false, matching navigation links that manage their own route.
func (i *NavigationMenuItemWidget) CloseOnClick(v bool) *NavigationMenuItemWidget {
	i.closeOnClick = v
	return i
}

// Trigger returns the button for visual customization.
func (i *NavigationMenuItemWidget) Trigger() *ButtonWidget { return i.ButtonWidget }

func (i *NavigationMenuItemWidget) isOpen() bool {
	return i.owner != nil && i.owner.popup.IsOpen() && i.owner.current() == i
}
func (i *NavigationMenuItemWidget) disabled() bool {
	return i.IsInert() || i.owner != nil && i.owner.IsInert()
}
func (i *NavigationMenuItemWidget) activate() { i.activateWith(true) }

// activateWith follows a link or toggles a dropdown. A dropdown opened from
// the keyboard takes focus into its panel; one opened with the pointer
// leaves focus on its trigger, as a browser's does.
func (i *NavigationMenuItemWidget) activateWith(focus bool) {
	if i.disabled() {
		return
	}
	if i.panel != nil {
		if i.owner != nil {
			if n := i.owner; i.isOpen() && !focus && n.hoverShown == slices.Index(n.items, i) {
				// The pointer opened it on its way here: the click keeps
				// it open, now until it is dismissed.
				n.hoverShown, n.hoverOpened, n.hoverLeft = -1, false, time.Time{}
			} else if i.isOpen() {
				i.owner.popup.Hide()
				if !focus {
					// Closed with a click: resting on it does not open it
					// again until the pointer has been off it.
					i.owner.hoverBlocked = slices.Index(i.owner.items, i)
				}
			} else {
				i.owner.open(i, focus)
			}
		}
		return
	}
	if i.closeOnClick {
		if i.popup != nil {
			i.popup.Hide()
		}
		if i.owner != nil {
			i.owner.popup.Hide()
		}
	}
	if i.onNavigate != nil {
		i.onNavigate()
	}
}

// Act implements accessible link activation and dropdown expand/collapse.
func (i *NavigationMenuItemWidget) Act(a ggui.Action) bool {
	if i.disabled() {
		return false
	}
	switch a.Kind {
	case ggui.ActionPress:
		i.activate()
	case ggui.ActionExpand:
		if i.owner == nil || i.panel == nil {
			return false
		}
		i.owner.open(i, true)
	case ggui.ActionCollapse:
		if i.owner == nil || i.panel == nil {
			return false
		}
		if i.isOpen() {
			i.owner.popup.Hide()
		}
	default:
		return false
	}
	return true
}

// Describe exposes native link/button semantics, current destination and
// expanded state, including a disabled parent navigation group.
func (i *NavigationMenuItemWidget) Describe() ggui.Node {
	node := i.ButtonWidget.Describe()
	node.Disabled = i.disabled()
	return node
}

// Layout implements ggui.Widget. A top-level entry is the 36px trigger of
// shadcn's navigation menu; a link inside a dropdown's panel is its plainer
// row, padded by one spacing unit, in the regular weight and at the smaller
// radius.
func (i *NavigationMenuItemWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	i.popup, _ = ggui.PopupOf(env)
	i.selected = i.activeProp.Get()
	i.inContent = i.owner == nil && i.popup != nil
	i.menu, _ = env.Get(navigationMenuOwnerKey)
	if i.inContent {
		t := uitheme.Of(env)
		i.ButtonWidget.Pad(t.Space).Radius(t.RadiusSm)
	}
	return i.ButtonWidget.Layout(c, env)
}

// Paint implements ggui.Widget.
func (i *NavigationMenuItemWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	i.rect = r
	// Register the entry itself as the sole focus target. Painting the nested
	// button would create an additional Tab stop with its own key handler.
	t := i.theme
	inert := i.disabled()
	// The muted surface under the pointer or the keyboard, and half of it
	// for the current destination and the open dropdown.
	focused := i.Focused && i.FocusVisible && !i.quiet
	at := i.Anchor(r)
	hover := ease(dst, i.theme, at, navigationHoverSlot, pick((i.Hovered || focused) && !inert, 1.0, 0.0), i.motion)
	on := ease(dst, i.theme, at, navigationActiveSlot, pick((i.selected || i.isOpen()) && !inert, 1.0, 0.0), i.motion)
	var fill color.Color
	if alpha := .5*on + (1-.5*on)*hover; alpha > 0 {
		fill = fade(t.Muted, alpha)
	}
	label := i.style.Label
	if inert {
		label = t.Disabled(label)
	}
	if i.label != nil {
		i.label.Color(label)
	}
	i.box.Fill(fill).Border(0, nil)
	if i.style.Border != nil {
		i.box.Border(t.BorderWidth, i.style.Border)
	}
	dst.Describe(r, i)
	if !inert {
		dst.HitPointer(r, i)
		dst.HitKey(r, i)
		dst.HitCursor(r, pick(i.panel == nil, ggui.CursorShapePointer, ggui.CursorShapeDefault))
	}
	dst.Paint(i.box, r)
	if !i.inContent {
		// A link in a panel shows focus as its surface alone.
		controlFocusRing(dst, &i.Interactive, t, r, pick(i.radius >= 0, i.radius, t.Radius), t.Ring)
	}
}

// ConsumesKey reserves horizontal navigation and dropdown opening keys.
func (i *NavigationMenuItemWidget) ConsumesKey(e ggui.KeyEvent) bool {
	return ggui.Activates(e) || i.owner != nil && (navConsumes(e, navHorizontal) || e.Kind == ggui.KeyPress && i.panel != nil && (e.Key == ggui.KeyArrowDown || e.Key == ggui.KeyArrowUp))
}

// HandleKey moves between top-level entries, or opens and activates an entry.
func (i *NavigationMenuItemWidget) HandleKey(e ggui.KeyEvent) {
	if i.disabled() {
		return
	}
	switch {
	case e.Kind == ggui.KeyFocus && i.menu != nil && i.menu.quietFocus:
		// The panel took focus as the pointer opened it; it shows no
		// focus until the keyboard moves it.
		i.quiet, i.menu.quietFocus = true, false
	case e.Kind == ggui.KeyFocus, e.Kind == ggui.KeyBlur, e.Kind == ggui.KeyPress:
		i.quiet = false
	}
	i.Keyboard(e, nil)
	if i.owner != nil && e.Kind == ggui.KeyPress {
		n := i.owner
		cur := slices.Index(n.items, i)
		if next, ok := navTarget(directionalKey(e, n.rtl).Key, navHorizontal, cur, len(n.items)); ok {
			next = max(0, min(next, len(n.items)-1))
			direction := 1
			if next < cur {
				direction = -1
			}
			for next >= 0 && next < len(n.items) && !n.enabled(next) {
				next += direction
			}
			if next < 0 || next >= len(n.items) {
				next = cur
			}
			if next >= 0 {
				n.pending = n.items[next]
			}
			n.popup.Hide()
			return
		}
		if i.panel != nil && (e.Key == ggui.KeyArrowDown || e.Key == ggui.KeyArrowUp) {
			n.open(i, true)
			return
		}
	}
	if ggui.Activates(e) {
		i.activate()
	}
}

// HandlePointer activates only enabled navigation entries.
func (i *NavigationMenuItemWidget) HandlePointer(e ggui.PointerEvent) bool {
	if i.disabled() {
		return false
	}
	return i.Pointer(e, func() { i.activateWith(false) })
}

// Adopt preserves the button's input state across keyed rebuilds.
func (i *NavigationMenuItemWidget) Adopt(prev any) { i.Interactive.Adopt(prev) }

// NavigationMenuWidget is a row of independently focusable navigation links and
// dropdowns, with one open panel at a time. It is a navigation group, not an
// application action menubar. Popup content uses normal Tab navigation.
type NavigationMenuWidget struct {
	props property.Owner
	ggui.Interactive
	items   []*NavigationMenuItemWidget
	popup   *ggui.PopupWidget
	row     *ggui.RowWidget
	active  int
	pending *NavigationMenuItemWidget
	rtl     bool

	chrome   *ggui.BoxWidget // the panel's surface, around whichever content shows
	theme    *uitheme.Theme
	env      ggui.Env
	motion   time.Duration // navigationMenuMotion, or zero under reduced motion
	shown    bool          // the panel was open when last painted
	leaving  int           // the entry whose content is sliding out, or -1
	travel   float64       // +1 when the new content arrives from the right, -1 from the left
	restart  bool          // a switch of entries is to start its slide at the next paint
	leftSize ggui.Size     // the leaving content's panel, as laid out

	noHover      bool      // dropdowns open only when pressed
	hoverItem    int       // the trigger the pointer rests on, or -1
	hoverSince   time.Time // when it came to rest there
	hoverBlocked int       // a trigger closed with a click, until the pointer leaves it
	hoverOpened  bool      // the open dropdown was opened by resting on its trigger
	hoverShown   int       // the trigger the pointer opened or switched to while on it, or -1
	hoverLeft    time.Time // when the pointer left the menu and its panel
	quietFocus   bool      // the panel opening takes focus but was opened with the pointer
	// The panel's motion: how far it has arrived, how far the content has
	// slid across, and where its corner and its size have got to.
	enter, slide, anchorX, width, height ggui.Motion
}

// NavigationMenu creates a native navigation group. Left/Right and Home/End
// move between enabled top-level entries; Enter/Space activates a link or
// toggles a dropdown, and Up/Down opens a dropdown. Escape or an outside press
// dismisses it and restores focus. Resting the pointer on a trigger opens its
// dropdown, and moving onto another trigger while one is open switches to it;
// see OpenOnHover. Entries belong to just one group.
func NavigationMenu(items ...*NavigationMenuItemWidget) *NavigationMenuWidget {
	n := &NavigationMenuWidget{items: slices.Clone(items), active: -1, leaving: -1, hoverItem: -1, hoverBlocked: -1, hoverShown: -1}
	n.Role = ggui.RoleGroup
	n.SetName("Navigation")
	n.AutoKey()
	children := make([]ggui.Widget, len(items))
	for j, item := range items {
		if item == nil {
			panic("ui: NavigationMenu entry must not be nil")
		}
		item.owner = n
		children[j] = item
	}
	n.row = ggui.Row(children...).Gap(0)
	n.chrome = ggui.Box(&navigationMenuViewport{n})
	n.popup = ggui.Popup(n.row, navigationMenuPanel{n}).Owner(n).Modal(false)
	return n
}

// Name names the navigation group.
func (n *NavigationMenuWidget) Name(name string) *NavigationMenuWidget { n.SetName(name); return n }

// BindName follows the navigation group's accessible name.
func (n *NavigationMenuWidget) BindName(r ggui.Readable[string]) *NavigationMenuWidget {
	n.Interactive.BindName(r)
	return n
}

// Key preserves the group and assigns stable positional identities to its entries.
func (n *NavigationMenuWidget) Key(k any) *NavigationMenuWidget {
	n.SetKey(k)
	n.popup.Key(childKey{owner: k, part: "navigation popup"})
	for j, item := range n.items {
		item.Key(childKey{owner: k, part: j})
	}
	return n
}

// OpenOnHover sets whether resting the pointer on a trigger for 200ms opens
// its dropdown, as shadcn's navigation menu does (true by default). A
// dropdown opened this way closes 150ms after the pointer leaves both the
// menu and the panel; one opened with a click or the keyboard stays open
// until it is dismissed, as does one the pointer opened and then clicked.
// While one is open, the pointer moving onto another trigger switches to it
// either way.
func (n *NavigationMenuWidget) OpenOnHover(v bool) *NavigationMenuWidget {
	n.noHover = !v
	return n
}

// Popup returns the shared popup for inspecting or dismissing the open panel.
func (n *NavigationMenuWidget) Popup() *ggui.PopupWidget { return n.popup }

// Disabled disables all entries and closes the open panel.
func (n *NavigationMenuWidget) Disabled(v bool) *NavigationMenuWidget {
	n.SetInert(v)
	if v {
		n.popup.Hide()
	}
	return n
}

// BindDisabled follows a disabled-state reader for the whole group.
func (n *NavigationMenuWidget) BindDisabled(v ggui.Readable[bool]) *NavigationMenuWidget {
	n.BindInert(v)
	return n
}
func (n *NavigationMenuWidget) enabled(j int) bool { return !n.IsInert() && !n.items[j].IsInert() }
func (n *NavigationMenuWidget) current() *NavigationMenuItemWidget {
	if n.active < 0 || n.active >= len(n.items) {
		return nil
	}
	return n.items[n.active]
}

// anchorPoint is where item's panel hangs: under its leading corner.
func (n *NavigationMenuWidget) anchorPoint(item *NavigationMenuItemWidget) ggui.Point {
	return item.rect.Origin.Add(ggui.Pt(pick(n.rtl, item.rect.Size.W, 0.0), item.rect.Size.H))
}

// open shows item's dropdown, moving focus into its panel when focus is set.
func (n *NavigationMenuWidget) open(item *NavigationMenuItemWidget, focus bool) {
	if item.disabled() || item.panel == nil {
		return
	}
	next := slices.Index(n.items, item)
	if prev := n.current(); n.popup.IsOpen() && prev != nil && prev != item {
		// Moving between dropdowns while one is open: the old content
		// slides out the way the new one came from.
		n.leaving, n.restart = n.active, true
		n.travel = pick(item.rect.Origin.X >= prev.rect.Origin.X, 1.0, -1.0)
	}
	n.active, n.hoverOpened, n.hoverLeft, n.quietFocus = next, false, time.Time{}, !focus
	n.popup.ShowAt(n.place(n.anchorPoint(item)))
	if focus {
		n.popup.FocusContent()
	}
}

// place is where the panel hangs this frame on its way to at: its corner
// eases across when the open dropdown changes, and is there at once when the
// panel opens.
func (n *NavigationMenuWidget) place(at ggui.Point) ggui.Point {
	now := ggui.FrameTime()
	n.anchorX.Easing(navigationMenuEase).MoveTo(at.X, now, pick(n.shown, n.motion, 0))
	at.X = n.anchorX.Value(now)
	return at
}

// Layout implements ggui.Widget.
func (n *NavigationMenuWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer n.props.Layout().EndLayout()
	n.Sync()
	n.rtl = env.Direction() == ggui.DirRTL
	t := uitheme.Of(env)
	n.theme, n.env, n.motion = t, env, env.Motion(navigationMenuMotion)
	for _, item := range n.items {
		item.Sync()
	}
	if item := n.current(); n.IsInert() || item == nil || item.IsInert() {
		n.popup.Hide()
	}
	// The popup fades its content in and out over the time it is given, and
	// the panel leaves in 150ms; the longer arrival is the panel's own.
	return n.popup.Gap(t.Space).Layout(c, env.With(ggui.PopupDurationKey, navigationMenuExit))
}

// Paint implements ggui.Widget.
func (n *NavigationMenuWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.HitPointer(r, n)
	open := n.popup.IsOpen()
	if open && !n.shown {
		// Opening: the panel arrives where it belongs at its own size,
		// with nothing sliding out of it.
		n.leaving, n.restart = -1, false
		n.enter, n.anchorX, n.width, n.height = ggui.Motion{}, ggui.Motion{}, ggui.Motion{}, ggui.Motion{}
		n.enter.Easing(navigationMenuEase).MoveTo(0, ggui.FrameTime(), 0)
	}
	dst.DescribeNode(r, n, func(dst *ggui.Canvas) { dst.Paint(n.popup, r) })
	if !n.noHover && !n.IsInert() {
		n.hover(dst, r)
		open = n.popup.IsOpen()
	}
	if item := n.current(); item != nil && open {
		n.popup.ShowAt(n.place(n.anchorPoint(item)))
	}
	if !open {
		n.quietFocus = false
	}
	n.shown = open
	if n.pending != nil {
		dst.RequestFocus(n.pending)
		n.pending = nil
	}
}

// hover opens a dropdown whose trigger the pointer has rested on, switches
// to another trigger's while one is open, and closes one it opened once the
// pointer has left the menu and its panel.
func (n *NavigationMenuWidget) hover(dst *ggui.Canvas, r ggui.Rect) {
	p, known := dst.Pointer()
	now := ggui.FrameTime()
	over := -1
	for j, item := range n.items {
		if known && item.panel != nil && n.enabled(j) && item.rect.Contains(p) {
			over = j
		}
	}
	if over != n.hoverBlocked {
		n.hoverBlocked = -1
	}
	if over != n.hoverItem {
		n.hoverItem, n.hoverSince, n.hoverShown = over, now, -1
	}
	open := n.popup.IsOpen()
	switch {
	case over < 0 || over == n.hoverBlocked:
	case open && over != n.active:
		hovered := n.hoverOpened
		n.open(n.items[over], false)
		n.hoverOpened, n.hoverShown = hovered, over
	case !open && dst.PointerPressed():
	case !open && now.Sub(n.hoverSince) >= navigationMenuDelay:
		n.open(n.items[over], false)
		n.hoverOpened, n.hoverShown = true, over
	case !open:
		ggui.WakeAt(n.hoverSince.Add(navigationMenuDelay))
	}
	if !n.popup.IsOpen() || !n.hoverOpened {
		n.hoverLeft = time.Time{}
		return
	}
	// The gap between the triggers and the panel counts as the menu, so
	// crossing it does not close the panel.
	if known && r.Union(n.popup.Rect()).Contains(p) {
		n.hoverLeft = time.Time{}
		return
	}
	if n.hoverLeft.IsZero() {
		n.hoverLeft = now
	}
	if now.Sub(n.hoverLeft) >= navigationMenuLinger {
		n.popup.Hide()
		n.hoverOpened, n.hoverLeft = false, time.Time{}
		return
	}
	ggui.WakeAt(n.hoverLeft.Add(navigationMenuLinger))
}

// HandlePointer registers the group's retained identity without consuming input.
func (n *NavigationMenuWidget) HandlePointer(ggui.PointerEvent) bool { return false }

// Adopt retains the active dropdown across a keyed rebuild.
func (n *NavigationMenuWidget) Adopt(prev any) {
	n.Interactive.Adopt(prev)
	p, ok := prev.(*NavigationMenuWidget)
	if ok {
		n.hoverItem, n.hoverSince, n.hoverBlocked, n.hoverShown = p.hoverItem, p.hoverSince, p.hoverBlocked, p.hoverShown
	}
	if ok && p.active >= 0 && p.active < len(n.items) {
		n.active = p.active
		n.shown, n.leaving, n.travel, n.restart, n.leftSize = p.shown, p.leaving, p.travel, p.restart, p.leftSize
		n.hoverOpened, n.hoverLeft = p.hoverOpened, p.hoverLeft
		n.enter, n.slide, n.anchorX, n.width, n.height = p.enter, p.slide, p.anchorX, p.width, p.height
		if n.leaving >= len(n.items) || n.leaving >= 0 && n.items[n.leaving].panel == nil {
			n.leaving = -1
		}
		if p.popup.IsOpen() && n.enabled(n.active) && n.items[n.active].panel != nil {
			n.popup.Show()
		}
	}
}

// navigationMenuPanel is the popup's content: the panel's surface around the
// open dropdown's content, scaling up from navigationMenuFrom as it arrives.
type navigationMenuPanel struct{ n *NavigationMenuWidget }

func (p navigationMenuPanel) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	n := p.n
	item := n.current()
	if item == nil || item.panel == nil {
		return ggui.Size{}
	}
	t := uitheme.Of(env)
	n.chrome.Shadow(t.PanelShadow).Fill(t.Popover).Border(t.BorderWidth, t.Border).Radius(t.Radius)
	// What is inside the panel opens its own popups in the theme's time.
	env = env.With(ggui.PopupDurationKey, n.env.PopupDuration())
	return n.chrome.Layout(c, env.With(navigationMenuOwnerKey, n))
}

func (p navigationMenuPanel) Paint(dst *ggui.Canvas, r ggui.Rect) {
	n := p.n
	if item := n.current(); item != nil && item.panel != nil {
		now := ggui.FrameTime()
		n.enter.MoveTo(1, now, n.motion)
		if e := n.enter.Value(now); e < 1 && n.popup.IsOpen() {
			// The popup scales its content up a little as it fades it in;
			// this takes the panel the rest of the way down to
			// navigationMenuFrom, about the trigger it hangs from, and
			// brings it back along the panel's own longer curve.
			const popIn = .95
			from := navigationMenuFrom / popIn
			about := ggui.Pt(clamp(item.rect.Center().X, r.Origin.X, r.Origin.X+r.Size.W), pick(r.Origin.Y >= item.rect.Origin.Y, r.Origin.Y, r.Origin.Y+r.Size.H))
			dst.Layer(ggui.LayerOptions{Scale: from + (1-from)*e, About: about}, func(layer *ggui.Canvas) { layer.Paint(n.chrome, r) })
		} else {
			dst.Paint(n.chrome, r)
		}
	}
	// Keep top-level triggers reachable above the popup scrim, so a click can
	// switch dropdowns or follow a link without first dismissing the old panel.
	for _, item := range n.items {
		if !item.disabled() {
			dst.HitPointer(item.rect, item)
		}
	}
}

// navigationMenuViewport is what the panel's surface holds: the open
// dropdown's content at the panel's padding. When the open dropdown changes
// it eases from the old content's size to the new one's, while the old
// content slides out and fades and the new one slides in from the side the
// new trigger is on.
type navigationMenuViewport struct{ n *NavigationMenuWidget }

func (v *navigationMenuViewport) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	n := v.n
	item := n.current()
	if item == nil || item.panel == nil {
		return ggui.Size{}
	}
	t := uitheme.Of(env)
	item.panelSize = item.panel.Padding(t.PanelPad).Width(item.width).Layout(c, env)
	if n.leaving >= 0 {
		left := n.items[n.leaving]
		n.leftSize = left.panel.Padding(t.PanelPad).Width(left.width).Layout(c, env)
	}
	// The size moves in layout, so each step of it is laid out again.
	w := n.width.Easing(navigationMenuEase).LayoutTo(env, item.panelSize.W, navigationMenuMotion)
	h := n.height.Easing(navigationMenuEase).LayoutTo(env, item.panelSize.H, navigationMenuMotion)
	return c.Constrain(ggui.Sz(w, h))
}

func (v *navigationMenuViewport) Paint(dst *ggui.Canvas, r ggui.Rect) {
	n := v.n
	item := n.current()
	if item == nil || item.panel == nil {
		return
	}
	now := ggui.FrameTime()
	if n.restart {
		n.restart = false
		n.slide = ggui.Motion{}
		n.slide.Easing(navigationMenuEase).MoveTo(0, now, 0)
	}
	across := 1.0
	if n.leaving >= 0 {
		n.slide.MoveTo(1, now, n.motion)
		if across = n.slide.Value(now); across >= 1 {
			n.leaving = -1
		}
	}
	if n.leaving < 0 {
		if r.Size == item.panelSize {
			dst.Paint(item.panel, r)
			return
		}
		// Still easing to its size: the content keeps its own and is cut
		// at the panel's edge.
		dst.Clip(r).Paint(item.panel, ggui.Rct(r.Origin, item.panelSize))
		return
	}
	// Each content moves half the panel's width, as shadcn's translate by
	// 50% does, the leaving one fading out as the arriving one fades in.
	left, shift := n.items[n.leaving], n.travel*r.Size.W/2
	dst.ClipRoundRect(r, n.theme.Radius, func(dst *ggui.Canvas) {
		dst.Inert().Layer(ggui.LayerOptions{Fade: across}, func(layer *ggui.Canvas) {
			layer.Paint(left.panel, ggui.Rct(r.Origin.Add(ggui.Pt(-shift*across, 0)), n.leftSize))
		})
		dst.Layer(ggui.LayerOptions{Fade: 1 - across}, func(layer *ggui.Canvas) {
			layer.Paint(item.panel, ggui.Rct(r.Origin.Add(ggui.Pt(shift*(1-across), 0)), item.panelSize))
		})
	})
}

// navigationMenuLinkFace is a link's content. At the top level it is centered
// in the trigger, as a button's is; inside a panel it starts at the leading
// edge in the regular weight, as shadcn's content links do.
type navigationMenuLinkFace struct {
	item  *NavigationMenuItemWidget
	child ggui.Widget
	size  ggui.Size
	rtl   bool
	h     float64
}

func (f *navigationMenuLinkFace) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	f.rtl = env.Direction() == ggui.DirRTL
	if f.item.inContent {
		env = env.WithText(ggui.TextStyle{Weight: ggui.WeightRegular})
	}
	f.size = f.child.Layout(c.Loosen(), env)
	out := c.Constrain(f.size)
	f.h = out.H
	return out
}

func (f *navigationMenuLinkFace) Paint(dst *ggui.Canvas, r ggui.Rect) {
	x := (r.Size.W - f.size.W) / 2
	if f.item.inContent {
		x = pick(f.rtl, r.Size.W-f.size.W, 0.0)
	}
	dst.Paint(f.child, ggui.Rct(r.Origin.Add(ggui.Pt(x, (r.Size.H-f.size.H)/2)), f.size))
}

func (f *navigationMenuLinkFace) Baseline() (float64, bool) {
	if child, ok := f.child.(ggui.Baseliner); ok {
		baseline, ok := child.Baseline()
		return baseline + (f.h-f.size.H)/2, ok
	}
	return 0, false
}

// navigationMenuFace keeps the disclosure chevron separate from the label so
// both retain native geometry and the chevron reflects the open state.
type navigationMenuFace struct {
	item      *NavigationMenuItemWidget
	env       ggui.Env
	labelSize ggui.Size
}

func (f *navigationMenuFace) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	f.env = env
	gap := f.item.theme.Space / 2
	inner := c.Loosen()
	inner.MaxW = max(0, inner.MaxW-navigationMenuChevron-gap)
	f.labelSize = f.item.label.Layout(inner, env)
	return c.Constrain(ggui.Sz(f.labelSize.W+navigationMenuChevron+gap, max(f.labelSize.H, navigationMenuChevron)))
}

func (f *navigationMenuFace) Paint(dst *ggui.Canvas, r ggui.Rect) {
	i := f.item
	x := pick(f.env.Direction() == ggui.DirRTL, r.Origin.X+navigationMenuChevron+i.theme.Space/2, r.Origin.X)
	dst.Paint(i.label, ggui.Rct(ggui.Pt(x, r.Origin.Y+(r.Size.H-f.labelSize.H)/2), f.labelSize))
	col := i.style.Label
	if i.disabled() {
		col = i.theme.Disabled(col)
	}
	// The chevron turns over as the dropdown opens, a pixel below center as
	// shadcn's top-px sets it.
	turn := ease(dst, i.theme, i.Anchor(r), navigationTurnSlot, pick(i.isOpen(), math.Pi, 0.0), f.env.Motion(navigationMenuTurn))
	cx := pick(f.env.Direction() == ggui.DirRTL, r.Origin.X+navigationMenuChevron/2, r.Origin.X+r.Size.W-navigationMenuChevron/2)
	paintIconAt(dst, f.env, icons.ChevronDown, ggui.Pt(cx, r.Center().Y+1), navigationMenuChevron, col, turn)
}
