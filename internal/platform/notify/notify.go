// Package notify posts the app's desktop notifications the platform's way:
// UserNotifications on macOS, toasts on Windows, the freedesktop
// notification service over D-Bus on Linux and the BSDs, and the
// Notification API in a browser. It reports the notification, or the
// button on one, the user clicks.
//
// The platform's work runs on a goroutine of the package's own, one call
// at a time and in the order asked, so that Post does not block the
// caller: only what is known at once, that the platform has no
// notifications or that the user turned them off, is returned; anything
// the platform says later is written once on stderr.
package notify

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"sync"
)

// The errors Post returns.
var (
	// ErrUnavailable is the platform having no notifications for the app:
	// no session bus on Linux, no Notification API in the browser, no
	// support at all elsewhere.
	ErrUnavailable = errors.New("ggui: notifications are unavailable")

	// ErrDenied is the user, or a policy, having turned the app's
	// notifications off.
	ErrDenied = errors.New("ggui: notifications are turned off for this app")
)

// Notification is what Post shows; see ggui.Notification, which it
// mirrors.
type Notification struct {
	ID       string
	Title    string
	Subtitle string
	Body     string
	Sound    bool
	Group    string
	Actions  []Action
	Data     string
}

// Action is a button on a notification.
type Action struct {
	ID         string
	Title      string
	Foreground bool
}

// Response is a click on a notification, Action "" for the notification
// itself. Foreground says whether the app should come forward: true for the
// notification itself and for an action marked so.
type Response struct {
	ID         string
	Data       string
	Action     string
	Foreground bool
}

// Options are what Start is given.
type Options struct {
	// AppName names the app where the platform shows a name beside the
	// notification it is not given otherwise: the freedesktop service's
	// app_name and the toast's registered display name on Windows.
	AppName string
	// AppID is the app's id, ggui.AppID: the AUMID the toasts of Windows
	// are shown under and the desktop entry the freedesktop service finds
	// the app's icon by.
	AppID string
	// Icon is the icon theme name the freedesktop service is given, or ""
	// for none: the app's id when a package installed its icon under it.
	Icon string
	// Respond is called with every click, on a goroutine of the
	// platform's.
	Respond func(Response)
}

var (
	// setupOnce runs the platform's setup, before the first Post on any
	// goroutine sees what it set.
	setupOnce sync.Once

	mu      sync.Mutex
	opts    Options
	started bool
	queue   []func()
	wake    = make(chan struct{}, 1)
)

// Start readies the platform's notifications and starts the goroutine that
// posts them. The first call does what must be done before the app
// finishes launching, which on macOS is to become the notification
// center's delegate, so that the click that launched the app is heard;
// later calls only replace the options. It may be called from any
// goroutine.
func Start(o Options) {
	mu.Lock()
	first := !started
	started = true
	opts = o
	mu.Unlock()
	if first {
		setupOnce.Do(setup)
		go work()
	}
}

// Post shows n, or returns ErrUnavailable or ErrDenied, wrapped, when it
// is known at once that it cannot be shown. It does not wait for the
// platform. Call Start first.
func Post(n Notification) error {
	setupOnce.Do(setup)
	if err := check(); err != nil {
		return err
	}
	n.Actions = slices.Clone(n.Actions)
	enqueue(func() {
		if err := post(n); err != nil {
			Report(err)
		}
	})
	return nil
}

// Remove withdraws the notification with id, shown or still to be shown,
// where the platform can. Call Start first.
func Remove(id string) {
	setupOnce.Do(setup)
	if id == "" || check() != nil {
		return
	}
	enqueue(func() { remove(id) })
}

// RequestPermission asks the user to allow the app's notifications where
// the platform asks, and the first time only, and calls fn, on a goroutine
// of the package's, with whether they are allowed. Where the platform does
// not ask, they are allowed when they are available. Call Start first.
func RequestPermission(fn func(granted bool)) {
	setupOnce.Do(setup)
	if err := check(); errors.Is(err, ErrUnavailable) {
		go fn(false)
		return
	}
	enqueue(func() { permission(fn) })
}

// Options returns the options Start was last given.
func options() Options {
	mu.Lock()
	defer mu.Unlock()
	return opts
}

// respond hands r to the Respond option, from whatever goroutine the
// platform called on.
func respond(r Response) {
	if f := options().Respond; f != nil {
		f(r)
	}
}

// enqueue has the posting goroutine run fn after the work before it.
func enqueue(fn func()) {
	mu.Lock()
	queue = append(queue, fn)
	mu.Unlock()
	select {
	case wake <- struct{}{}:
	default:
	}
}

// work runs the queued work, for the life of the process. It keeps its
// thread, which COM on Windows wants, and which an autorelease pool on
// macOS must be drained on.
func work() {
	runtime.LockOSThread()
	for range wake {
		for {
			mu.Lock()
			list := queue
			queue = nil
			mu.Unlock()
			if len(list) == 0 {
				break
			}
			for _, fn := range list {
				fn()
			}
		}
	}
}

// reported holds the messages Report wrote.
var reported sync.Map

// Report writes err on stderr, once for each distinct message: what the
// platform said of a notification after Post returned.
func Report(err error) {
	msg := err.Error()
	if _, dup := reported.LoadOrStore(msg, true); !dup {
		fmt.Fprintf(os.Stderr, "ggui: notification: %s\n", msg)
	}
}

// payload is what a notification carries to be handed back on a click,
// where the platform carries it with the notification rather than this
// process keeping it: in the user info of a macOS notification and in the
// arguments of a Windows toast and its buttons. It outlives the process
// there, as the notification does.
type payload struct {
	ID   string `json:"i,omitempty"`
	Data string `json:"d,omitempty"`
	// Action is the button a Windows toast's arguments are for.
	Action string `json:"a,omitempty"`
	// Foreground lists the buttons that bring the app forward (macOS), or
	// says that the one the arguments are for does (Windows: "1").
	Foreground []string `json:"f,omitempty"`
}

// encode is p as a string.
func (p payload) encode() string {
	b, err := json.Marshal(p)
	if err != nil {
		// Strings and slices of them always marshal.
		panic(err)
	}
	return string(b)
}

// decodePayload is the payload s holds, and false for a string that is not
// one, as an older or another app's notification may hold.
func decodePayload(s string) (payload, bool) {
	var p payload
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return payload{}, false
	}
	return p, true
}

// foregroundIDs are the ids of the actions that bring the app forward.
func foregroundIDs(actions []Action) []string {
	var ids []string
	for _, a := range actions {
		if a.Foreground {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// joinSubtitle is the body of a platform that has no subtitle: the
// subtitle, if any, on a line of its own above the body.
func joinSubtitle(n Notification) string {
	switch {
	case n.Subtitle == "":
		return n.Body
	case n.Body == "":
		return n.Subtitle
	}
	return n.Subtitle + "\n" + n.Body
}
