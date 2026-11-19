package ggui

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/ironpark/ggui/internal/platform/notify"
)

// Notification is a desktop notification, which App.Notify shows the
// platform's way: a banner and an entry in Notification Center on macOS,
// a toast in Windows' notification center, a notification of the
// desktop's notification service on Linux and the BSDs, and a Notification
// in the browser.
//
//	app.Notify(ggui.Notification{
//		ID:    "chat-42",
//		Title: "Ada Lovelace",
//		Body:  "Are the engines ready?",
//		Sound: true,
//		Actions: []ggui.NotificationAction{
//			{ID: "reply", Title: "Reply", Foreground: true},
//			{ID: "read", Title: "Mark as read"},
//		},
//		Data: "chat/42",
//	})
type Notification struct {
	// ID names the notification: a later one with the same ID replaces
	// it where it is shown, and RemoveNotification withdraws it by it.
	// With "", every notification is a new one.
	ID string
	// Title is the first line, in bold where the platform makes it so.
	Title string
	// Subtitle is a line between the title and the body. macOS shows it
	// as one and Windows as the toast's second line; Linux, the BSDs and
	// the browser have no place for it and show it on a line of its own
	// above the body.
	Subtitle string
	// Body is the text of the notification.
	Body string
	// Sound plays the platform's notification sound with it; without it
	// the notification is silent.
	Sound bool
	// Group gathers the notifications of one conversation or topic, as
	// macOS stacks those with the same thread identifier. Elsewhere it is
	// ignored: a Windows toast's group is part of what it is known by, not
	// how it is shown, and the freedesktop service has no such thing.
	Group string
	// Actions are buttons on the notification, where the platform shows
	// them: macOS in the notification's Options menu or as buttons,
	// Windows up to five, Linux where the notification service shows
	// actions, which GNOME and KDE do. The browser shows none.
	Actions []NotificationAction
	// Data is handed back in the NotificationResponse of a click, for the
	// app to tell what the notification was about. On macOS and Windows
	// it travels with the notification, so the response to one shown by
	// an earlier run of the app has it too.
	Data string
}

// NotificationAction is a button on a Notification.
//
//	ggui.NotificationAction{ID: "reply", Title: "Reply", Foreground: true}
type NotificationAction struct {
	// ID is the NotificationResponse.Action of a click on the button.
	ID string
	// Title is the button's label.
	Title string
	// Foreground brings the app forward when the button is clicked, as a
	// click on the notification itself always does: for a button that
	// opens a window, such as Reply, not one done in place, such as Mark
	// as read.
	Foreground bool
}

// NotificationResponse is a click on one of the app's notifications,
// which App.OnNotification's handlers receive.
//
//	app.OnNotification(func(r ggui.NotificationResponse) {
//		switch r.Action {
//		case "": // the notification itself
//			m.openChat(r.Data)
//		case "read":
//			m.markRead(r.Data)
//		}
//	})
type NotificationResponse struct {
	// ID is the Notification's ID, "" for one shown without.
	ID string
	// Data is the Notification's Data.
	Data string
	// Action is the ID of the button clicked, or "" for a click on the
	// notification itself.
	Action string
}

// The errors App.Notify returns when it knows at once that a notification
// cannot be shown. Anything the platform says later, it writes once on
// stderr.
var (
	// ErrNotificationsUnavailable is the platform having no notifications
	// for the app: Linux or a BSD with no D-Bus session bus, a browser
	// without the Notification API, iOS, Android and the like.
	//
	//	if err := app.Notify(n); errors.Is(err, ggui.ErrNotificationsUnavailable) {
	//		status.Set(n.Body) // say it in the window instead
	//	}
	ErrNotificationsUnavailable = notify.ErrUnavailable

	// ErrNotificationsDenied is the user having turned the app's
	// notifications off: in System Settings on macOS, Settings on Windows,
	// or the page's permission in a browser. It is returned once the
	// platform has said so, which on macOS and in a browser is after the
	// first notification asked.
	//
	//	if errors.Is(app.Notify(n), ggui.ErrNotificationsDenied) {
	//		banner.Set("Turn on notifications to hear of new messages")
	//	}
	ErrNotificationsDenied = notify.ErrDenied
)

// errEmptyNotification is what Notify returns for a notification with
// nothing to show.
var errEmptyNotification = errors.New("ggui: a notification needs a title or a body")

// toPlatform is n as the platform package takes it.
func (n Notification) toPlatform() (notify.Notification, error) {
	if n.Title == "" && n.Subtitle == "" && n.Body == "" {
		return notify.Notification{}, errEmptyNotification
	}
	p := notify.Notification{ID: n.ID, Title: n.Title, Subtitle: n.Subtitle, Body: n.Body, Sound: n.Sound, Group: n.Group, Data: n.Data}
	for _, a := range n.Actions {
		if a.ID == "" {
			return notify.Notification{}, fmt.Errorf("ggui: the notification action %q has no ID", a.Title)
		}
		p.Actions = append(p.Actions, notify.Action{ID: a.ID, Title: a.Title, Foreground: a.Foreground})
	}
	return p, nil
}

// clone is n with a slice of its own.
func (n Notification) clone() Notification {
	n.Actions = slices.Clone(n.Actions)
	return n
}

// appNotifications is the App's side of its notifications: the handlers
// OnNotification registered and the clicks that came before there was a
// handler, or a running app, to hand them to.
type appNotifications struct {
	start sync.Once
	ready atomic.Bool // the windows are open, so clicks may be handed on

	mu       sync.Mutex
	handlers []func(NotificationResponse)
	pending  []pendingClick
}

// pendingClick is a click waiting for a handler: fg says it should still
// bring the app forward.
type pendingClick struct {
	r  NotificationResponse
	fg bool
}

// maxPendingClicks is how many clicks wait for a handler; older ones are
// dropped.
const maxPendingClicks = 16

// Notify shows n, and returns at once: the platform's work runs off the
// calling goroutine, which may be any, the UI thread included. It returns
// an error when it knows at once that n cannot be shown:
// ErrNotificationsUnavailable or ErrNotificationsDenied, wrapped, or an
// error for a notification with no text or an action with no ID. What the
// platform says later, it writes once on stderr.
//
//	ui.Button("Export", func() {
//		go func() {
//			export()
//			app.Notify(ggui.Notification{Title: "Export finished", Body: name})
//		}()
//	})
//
// On macOS the first notification asks the user to allow them, unless
// RequestNotificationPermission asked before. Notifications are posted
// through UserNotifications by an app run from a bundle, as ggui build
// makes one; an executable run on its own, by go run, go build or ggui
// dev, has no bundle to post for and shows them through osascript, with
// no buttons, no replacing by ID and no response to a click, as it says
// once on stderr. On Windows the toasts are shown under the app's id,
// registered for the user as an AppUserModelID named for the main
// window's title. On Linux and the BSDs they go to the desktop's
// notification service on the session bus, which finds the app's icon
// by the desktop file named for its id. In a browser the first one asks
// for the page's permission.
//
// A click calls the OnNotification handlers. See docs/windows-and-platform.md
// for what each platform shows.
func (a *App) Notify(n Notification) error {
	p, err := n.toPlatform()
	if err != nil {
		return err
	}
	a.startNotifications()
	return notify.Post(p)
}

// RemoveNotification withdraws the notification with id, from the screen
// and from the platform's list of notifications, where the platform lets
// an app: macOS and Windows withdraw one an earlier run of the app showed
// too, Linux and the browser only one this run showed, and an app outside
// a bundle on macOS none. It may be called from any goroutine.
//
//	app.OnNotification(func(r ggui.NotificationResponse) { ... })
//	m.read.Watch(func(chat string) { app.RemoveNotification("chat-" + chat) })
func (a *App) RemoveNotification(id string) {
	a.startNotifications()
	notify.Remove(id)
}

// OnNotification registers fn to run, on the UI thread, with every click
// on one of the app's notifications or on a button of one. It may be
// called from any goroutine, before Run too.
//
//	app.OnNotification(func(r ggui.NotificationResponse) {
//		if r.Action == "" {
//			m.openChat(r.Data)
//		}
//	})
//
// A click on the notification itself, and on a button marked Foreground,
// also brings the app forward, as ShowFromTray does, before fn runs: the
// windows HideToTray hid are shown again, or else the window that last
// had the focus, and the app comes to the front. The platform brings the
// app forward for such a click itself on macOS and Windows, which is why
// a handler is not asked first.
//
// A click before the app's windows are open, as when a click on a
// notification launched the app on macOS, or before any handler is
// registered, waits for both. To hear of the click that launched it, an
// app registers its handler before Run.
func (a *App) OnNotification(fn func(NotificationResponse)) {
	n := &a.notes
	n.mu.Lock()
	n.handlers = append(n.handlers, fn)
	n.mu.Unlock()
	a.startNotifications()
	if n.ready.Load() {
		a.deliverPending()
	}
}

// RequestNotificationPermission asks the user to allow the app's
// notifications, where the platform asks and they have not answered yet,
// and calls fn on the UI thread with whether they are allowed. It is for
// asking at a moment of the app's choosing, as when the user turns on a
// setting that sends notifications, rather than at the first Notify, which
// asks otherwise; and, once asked, for showing the answer.
//
//	ui.Switch(ggui.Bind(notifyOn.Get, func(on bool) {
//		if !on {
//			notifyOn.Set(false)
//			return
//		}
//		app.RequestNotificationPermission(notifyOn.Set)
//	}), "Notify me of new messages")
//
// macOS asks once, in a notification of its own, and later reports what
// the user said there or has since set in System Settings; fn waits for
// the answer. A browser asks once, and may ask only in answer to a click.
// Windows, Linux and the BSDs do not ask: fn is told whether
// notifications are available and, on Windows, turned on. An app outside
// a bundle on macOS, which posts through osascript, is told true.
func (a *App) RequestNotificationPermission(fn func(granted bool)) {
	a.startNotifications()
	notify.RequestPermission(func(granted bool) {
		w := a.activeWindow()
		if w == nil {
			w = a.Window
		}
		w.post(func() { fn(granted) })
	})
}

// startNotifications readies the platform's notifications for the app,
// once, when it first posts, removes, asks or registers a handler.
func (a *App) startNotifications() {
	a.notes.start.Do(func() {
		id, src := appID()
		icon := ""
		if src == idBuild {
			// ggui build's Linux package installs the icon under the id.
			icon = id
		}
		name := a.title
		if name == "" {
			name = id
		}
		notify.Start(notify.Options{AppName: name, AppID: id, Icon: icon, Respond: a.notificationClicked})
	})
}

// notificationClicked takes a click from the platform, on whatever
// goroutine it calls on.
func (a *App) notificationClicked(r notify.Response) {
	click := pendingClick{r: NotificationResponse{ID: r.ID, Data: r.Data, Action: r.Action}, fg: r.Foreground}
	n := &a.notes
	n.mu.Lock()
	if len(n.pending) == maxPendingClicks {
		n.pending = n.pending[1:]
	}
	n.pending = append(n.pending, click)
	n.mu.Unlock()
	if n.ready.Load() {
		a.deliverPending()
	}
}

// notificationsReady is told, on the UI thread, that the windows are open,
// and hands on the clicks that waited for them.
func (a *App) notificationsReady() {
	a.notes.ready.Store(true)
	a.deliverPending()
}

// deliverPending hands the clicks waiting to the UI thread: each brings
// the app forward if it should, and goes to the handlers, if there are
// any yet; without them it waits on, having brought the app forward.
func (a *App) deliverPending() {
	w := a.activeWindow()
	if w == nil {
		return
	}
	n := &a.notes
	n.mu.Lock()
	clicks := n.pending
	handlers := slices.Clone(n.handlers)
	if len(handlers) > 0 {
		n.pending = nil
	} else {
		n.pending = slices.Clone(clicks)
		for i := range n.pending {
			n.pending[i].fg = false
		}
	}
	n.mu.Unlock()
	if len(clicks) == 0 {
		return
	}
	w.post(func() {
		for _, c := range clicks {
			if c.fg {
				a.ShowFromTray()
			}
			for _, fn := range handlers {
				fn(c.r)
			}
		}
	})
}

// Notify shows n, as App.Notify does: the window's app's notifications are
// the app's.
//
//	ggui.UseHost().Notify(ggui.Notification{Title: "Saved", Body: path})
func (w *Window) Notify(n Notification) error {
	if w.app == nil {
		return ErrNotificationsUnavailable
	}
	return w.app.Notify(n)
}

// RemoveNotification withdraws the notification with id, as
// App.RemoveNotification does.
//
//	ggui.UseHost().RemoveNotification("chat-42")
func (w *Window) RemoveNotification(id string) {
	if w.app != nil {
		w.app.RemoveNotification(id)
	}
}

// OnNotification registers fn for the clicks on the app's notifications,
// as App.OnNotification does: the handlers are the app's, whichever window
// registered them.
//
//	ggui.UseHost().OnNotification(func(r ggui.NotificationResponse) { m.open(r.Data) })
func (w *Window) OnNotification(fn func(NotificationResponse)) {
	if w.app != nil {
		w.app.OnNotification(fn)
	}
}

// probeNotifications are a Probe's notifications: shown nowhere, recorded
// for the test to read and to click.
type probeNotifications struct {
	mu       sync.Mutex
	shown    []Notification
	handlers []func(NotificationResponse)
}

// Notify records n among the probe's notifications, which Notifications
// returns, replacing the one with its ID if it has one; nothing is shown.
// It returns the error App.Notify returns for a notification with no text
// or an action with no ID, and nil otherwise. It may be called from any
// goroutine.
//
//	p.Tap("Send")
//	got := p.Notifications() // the notification the app posted
func (p *Probe) Notify(n Notification) error {
	if _, err := n.toPlatform(); err != nil {
		return err
	}
	ns := &p.notes
	ns.mu.Lock()
	defer ns.mu.Unlock()
	if n.ID != "" {
		ns.shown = slices.DeleteFunc(ns.shown, func(s Notification) bool { return s.ID == n.ID })
	}
	ns.shown = append(ns.shown, n.clone())
	return nil
}

// RemoveNotification takes the notification with id out of the probe's.
//
//	p.RemoveNotification("chat-42")
func (p *Probe) RemoveNotification(id string) {
	ns := &p.notes
	ns.mu.Lock()
	defer ns.mu.Unlock()
	ns.shown = slices.DeleteFunc(ns.shown, func(s Notification) bool { return s.ID == id })
}

// OnNotification registers fn for the clicks ClickNotification makes, as
// App.OnNotification does for the user's.
//
//	p.OnNotification(func(r ggui.NotificationResponse) { got = r })
func (p *Probe) OnNotification(fn func(NotificationResponse)) {
	ns := &p.notes
	ns.mu.Lock()
	defer ns.mu.Unlock()
	ns.handlers = append(ns.handlers, fn)
}

// Notifications returns the notifications the probe shows: those Notify
// recorded, in the order they were first shown, less those replaced,
// removed or clicked.
//
//	if n := p.Notifications(); len(n) != 1 || n[0].Title != "Export finished" {
//		t.Fatalf("notifications: %+v", n)
//	}
func (p *Probe) Notifications() []Notification {
	ns := &p.notes
	ns.mu.Lock()
	defer ns.mu.Unlock()
	out := make([]Notification, len(ns.shown))
	for i, n := range ns.shown {
		out[i] = n.clone()
	}
	return out
}

// ClickNotification clicks the notification with id, the latest shown of
// those with it, as the user would: on the notification itself for action
// "", or on its button with that ID. The notification goes, as a clicked
// one does, and the OnNotification handlers run in a frame, on the UI
// thread. It panics when the probe shows no such notification or button.
//
//	p.ClickNotification("chat-42", "read")
//	// the handler marked chat 42 read
func (p *Probe) ClickNotification(id, action string) {
	ns := &p.notes
	ns.mu.Lock()
	i := len(ns.shown) - 1
	for i >= 0 && ns.shown[i].ID != id {
		i--
	}
	if i < 0 {
		ns.mu.Unlock()
		panic(fmt.Sprintf("ggui: Probe.ClickNotification: no notification with ID %q", id))
	}
	n := ns.shown[i]
	if action != "" && !slices.ContainsFunc(n.Actions, func(a NotificationAction) bool { return a.ID == action }) {
		ns.mu.Unlock()
		panic(fmt.Sprintf("ggui: Probe.ClickNotification: notification %q has no action %q", id, action))
	}
	ns.shown = slices.Delete(ns.shown, i, i+1)
	handlers := slices.Clone(ns.handlers)
	ns.mu.Unlock()
	r := NotificationResponse{ID: n.ID, Data: n.Data, Action: action}
	p.Post(func() {
		for _, fn := range handlers {
			fn(r)
		}
	})
	p.Frame()
}
