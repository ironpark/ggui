//go:build darwin && !ios

package notify

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/ironpark/ggui/internal/platform/cocoa"
)

// macOS posts through UserNotifications, reached with purego through the
// Objective-C runtime so that the project stays free of cgo. The framework
// posts only for an app with a bundle: an executable run on its own, by go
// run, go build or ggui dev, shows its notifications through osascript
// instead, which says nothing of a click.
//
// The notification center holds its delegate weakly, so the delegate this
// package makes is kept for the life of the process; it shows the app's
// notifications while the app is in front too, which the system would
// otherwise hold back, and hears of the clicks.

// UNNotificationPresentationOptions, UNAuthorizationOptions and
// UNNotificationActionOptions.
const (
	presentSound  = 1 << 1
	presentList   = 1 << 3
	presentBanner = 1 << 4
	authSound     = 1 << 1
	authAlert     = 1 << 2
	actionFG      = 1 << 2
)

// The action identifiers of a click on the notification itself and of its
// dismissal.
const (
	defaultActionID = "com.apple.UNNotificationDefaultActionIdentifier"
	dismissActionID = "com.apple.UNNotificationDismissActionIdentifier"
)

// payloadKey is the key of the user info that holds the payload.
const payloadKey = "ggui"

var (
	selAlloc             = objc.RegisterName("alloc")
	selInit              = objc.RegisterName("init")
	selRetain            = objc.RegisterName("retain")
	selRelease           = objc.RegisterName("release")
	selNew               = objc.RegisterName("new")
	selDrain             = objc.RegisterName("drain")
	selMainBundle        = objc.RegisterName("mainBundle")
	selBundleIdentifier  = objc.RegisterName("bundleIdentifier")
	selBundlePath        = objc.RegisterName("bundlePath")
	selUTF8String        = objc.RegisterName("UTF8String")
	selIsKindOfClass     = objc.RegisterName("isKindOfClass:")
	selLocalizedDesc     = objc.RegisterName("localizedDescription")
	selCurrentCenter     = objc.RegisterName("currentNotificationCenter")
	selSetDelegate       = objc.RegisterName("setDelegate:")
	selRequestAuth       = objc.RegisterName("requestAuthorizationWithOptions:completionHandler:")
	selSetTitle          = objc.RegisterName("setTitle:")
	selSetSubtitle       = objc.RegisterName("setSubtitle:")
	selSetBody           = objc.RegisterName("setBody:")
	selSetSound          = objc.RegisterName("setSound:")
	selDefaultSound      = objc.RegisterName("defaultSound")
	selSetUserInfo       = objc.RegisterName("setUserInfo:")
	selSetThreadID       = objc.RegisterName("setThreadIdentifier:")
	selSetCategoryID     = objc.RegisterName("setCategoryIdentifier:")
	selDictionaryWith    = objc.RegisterName("dictionaryWithObject:forKey:")
	selRequestWith       = objc.RegisterName("requestWithIdentifier:content:trigger:")
	selAddRequest        = objc.RegisterName("addNotificationRequest:withCompletionHandler:")
	selRemoveDelivered   = objc.RegisterName("removeDeliveredNotificationsWithIdentifiers:")
	selRemovePending     = objc.RegisterName("removePendingNotificationRequestsWithIdentifiers:")
	selActionWith        = objc.RegisterName("actionWithIdentifier:title:options:")
	selCategoryWith      = objc.RegisterName("categoryWithIdentifier:actions:intentIdentifiers:options:")
	selSetWithArray      = objc.RegisterName("setWithArray:")
	selSetCategories     = objc.RegisterName("setNotificationCategories:")
	selGetCategories     = objc.RegisterName("getNotificationCategoriesWithCompletionHandler:")
	selNotification      = objc.RegisterName("notification")
	selRequest           = objc.RegisterName("request")
	selContent           = objc.RegisterName("content")
	selUserInfo          = objc.RegisterName("userInfo")
	selObjectForKey      = objc.RegisterName("objectForKey:")
	selActionIdentifier  = objc.RegisterName("actionIdentifier")
	selWillPresent       = objc.RegisterName("userNotificationCenter:willPresentNotification:withCompletionHandler:")
	selDidReceive        = objc.RegisterName("userNotificationCenter:didReceiveNotificationResponse:withCompletionHandler:")
	userNotificationsLib = "/System/Library/Frameworks/UserNotifications.framework/UserNotifications"
)

// loadFramework opens UserNotifications, whose classes the runtime then
// knows, and reports whether it could.
var loadFramework = sync.OnceValue(func() bool {
	_, err := purego.Dlopen(userNotificationsLib, purego.RTLD_GLOBAL|purego.RTLD_NOW)
	return err == nil
})

// bundled reports whether the app runs from an app bundle with an
// identifier, which UserNotifications needs: asked for its center without
// one, it raises an exception that ends the process.
var bundled = sync.OnceValue(func() bool {
	var ok bool
	cocoa.WithAutoreleasePool(func() {
		b := objc.ID(objc.GetClass("NSBundle")).Send(selMainBundle)
		if b == 0 || b.Send(selBundleIdentifier) == 0 {
			return
		}
		ok = strings.HasSuffix(goString(b.Send(selBundlePath)), ".app") && loadFramework()
	})
	return ok
})

// classes are the framework's classes, looked up once it is open.
type classes struct {
	center, content, request, sound, action, category objc.ID
}

func lookupClasses() classes {
	return classes{
		center:   objc.ID(objc.GetClass("UNUserNotificationCenter")),
		content:  objc.ID(objc.GetClass("UNMutableNotificationContent")),
		request:  objc.ID(objc.GetClass("UNNotificationRequest")),
		sound:    objc.ID(objc.GetClass("UNNotificationSound")),
		action:   objc.ID(objc.GetClass("UNNotificationAction")),
		category: objc.ID(objc.GetClass("UNNotificationCategory")),
	}
}

// Authorization states.
const (
	authUnknown = iota
	authGranted
	authDenied
)

var (
	cls      classes
	center   objc.ID // the current notification center, retained
	delegate objc.ID // retained for the life of the process

	auth      atomic.Int32 // what the user last said, authUnknown before
	lastAsked atomic.Int64 // when a denial was last checked again, in Unix nanoseconds

	// Categories registered, by identifier, retained. Only the posting
	// goroutine touches them.
	categories = map[string]objc.ID{}
	nextID     atomic.Uint64
)

// delegateClass is the class of the delegate, registered once. The
// runtime knows the delegate protocol only once some image has used it,
// which the framework need not have; the center asks its delegate whether
// it responds to each method, not whether it conforms, so the class does
// without the protocol then.
var delegateClass = sync.OnceValues(func() (objc.Class, error) {
	var protocols []*objc.Protocol
	if p := objc.GetProtocol("UNUserNotificationCenterDelegate"); p != nil {
		protocols = append(protocols, p)
	}
	return objc.RegisterClass("GguiNotificationDelegate", objc.GetClass("NSObject"), protocols, nil,
		[]objc.MethodDef{
			{Cmd: selWillPresent, Fn: willPresent},
			{Cmd: selDidReceive, Fn: didReceive},
		})
})

// setup becomes the center's delegate. It must happen before the app
// finishes launching for the click that launched it to be heard, so it is
// done at once, from whichever goroutine, rather than on the posting one.
func setup() {
	if !bundled() {
		return
	}
	cocoa.WithAutoreleasePool(func() {
		class, err := delegateClass()
		if err != nil {
			Report(err)
			return
		}
		cls = lookupClasses()
		center = cls.center.Send(selCurrentCenter).Send(selRetain)
		delegate = objc.ID(class).Send(selAlloc).Send(selInit)
		center.Send(selSetDelegate, delegate)
	})
}

// recheck is how often a denial is asked about again: the user may allow
// notifications in System Settings while the app runs.
const recheck = 2 * time.Second

func check() error {
	if !bundled() || center == 0 || auth.Load() != authDenied {
		return nil
	}
	now := time.Now().UnixNano()
	if last := lastAsked.Load(); now-last > int64(recheck) && lastAsked.CompareAndSwap(last, now) {
		go askAuthorization()
	}
	return fmt.Errorf("%w in System Settings, Notifications", ErrDenied)
}

// authWaiting are the channels waiting for the answer to the request in
// flight, nil while none is: a second request while the user is being asked
// fails at once, so every caller waits on the one.
var (
	authMu      sync.Mutex
	authWaiting []chan bool
)

// askAuthorization asks for leave to post alerts with sound, which the
// first time puts the question to the user and later reports what they
// said, and returns a channel that delivers the answer.
func askAuthorization() <-chan bool {
	answer := make(chan bool, 1)
	authMu.Lock()
	inFlight := authWaiting != nil
	authWaiting = append(authWaiting, answer)
	authMu.Unlock()
	if inFlight {
		return answer
	}
	cocoa.WithAutoreleasePool(func() {
		block := objc.NewBlock(func(_ objc.Block, granted bool, err objc.ID) {
			switch {
			case granted:
				auth.Store(authGranted)
			case err != 0:
				// Not the user's answer but the system's refusal to ask, as
				// for an app it does not know: one run from a temporary
				// directory, say. Each post asks again, and fails.
				auth.Store(authUnknown)
				Report(errors.New("asking to allow notifications: " + goString(err.Send(selLocalizedDesc))))
			default:
				auth.Store(authDenied)
			}
			authMu.Lock()
			waiting := authWaiting
			authWaiting = nil
			authMu.Unlock()
			for _, w := range waiting {
				w <- granted
			}
		})
		center.Send(selRequestAuth, uint(authAlert|authSound), block)
		// The center copied the block; the reference NewBlock made is ours.
		block.Release()
	})
	return answer
}

// authWait is how long a post waits for the user to answer the question
// the first post asks. The question is a notification of its own, which
// the user may leave unanswered; the post is then tried anyway, and says
// what the system answered.
const authWait = 60 * time.Second

func post(n Notification) error {
	if !bundled() {
		return postScript(n)
	}
	if center == 0 {
		return ErrUnavailable
	}
	if auth.Load() != authGranted {
		select {
		case ok := <-askAuthorization():
			if !ok && auth.Load() == authDenied {
				return fmt.Errorf("%w in System Settings, Notifications", ErrDenied)
			}
			if !ok {
				return errors.New("macOS did not let the app post notifications")
			}
		case <-time.After(authWait):
		}
	}
	// An autorelease pool belongs to its thread, which the posting
	// goroutine keeps.
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(selNew)
	defer pool.Send(selDrain)
	content := cls.content.Send(selAlloc).Send(selInit)
	defer content.Send(selRelease)
	content.Send(selSetTitle, cocoa.String(n.Title))
	if n.Subtitle != "" {
		content.Send(selSetSubtitle, cocoa.String(n.Subtitle))
	}
	content.Send(selSetBody, cocoa.String(n.Body))
	if n.Sound {
		content.Send(selSetSound, cls.sound.Send(selDefaultSound))
	}
	if n.Group != "" {
		content.Send(selSetThreadID, cocoa.String(n.Group))
	}
	p := payload{ID: n.ID, Data: n.Data, Foreground: foregroundIDs(n.Actions)}
	content.Send(selSetUserInfo, objc.ID(objc.GetClass("NSDictionary")).Send(selDictionaryWith, cocoa.String(p.encode()), cocoa.String(payloadKey)))
	if len(n.Actions) > 0 {
		content.Send(selSetCategoryID, cocoa.String(category(n.Actions)))
	}
	// A request with the identifier of one delivered replaces it.
	id := n.ID
	if id == "" {
		id = "ggui." + strconv.FormatUint(nextID.Add(1), 10) + "." + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	request := cls.request.Send(selRequestWith, cocoa.String(id), content, objc.ID(0))
	failed := make(chan string, 1)
	block := objc.NewBlock(func(_ objc.Block, err objc.ID) {
		msg := ""
		if err != 0 {
			msg = goString(err.Send(selLocalizedDesc))
			if msg == "" {
				msg = "the system did not show the notification"
			}
		}
		failed <- msg
	})
	center.Send(selAddRequest, request, block)
	block.Release()
	select {
	case msg := <-failed:
		if msg != "" {
			return errors.New(msg)
		}
	case <-time.After(5 * time.Second):
	}
	return nil
}

// category registers, unless it is already, the category of a
// notification with actions, and returns its identifier. The center takes
// the app's whole set of categories at once, so the set is given again
// with the new one in it, and asked for back, which waits until the
// center has it: a notification posted under a category the center does
// not know yet shows no buttons.
func category(actions []Action) string {
	id := categoryID(actions)
	if _, ok := categories[id]; ok {
		return id
	}
	buttons := make([]objc.ID, len(actions))
	for i, a := range actions {
		opts := uint(0)
		if a.Foreground {
			opts = actionFG
		}
		buttons[i] = cls.action.Send(selActionWith, cocoa.String(a.ID), cocoa.String(a.Title), opts)
	}
	c := cls.category.Send(selCategoryWith, cocoa.String(id), cocoa.Array(buttons), cocoa.Array(nil), uint(0))
	categories[id] = c.Send(selRetain)
	all := make([]objc.ID, 0, len(categories))
	for _, c := range categories {
		all = append(all, c)
	}
	center.Send(selSetCategories, objc.ID(objc.GetClass("NSSet")).Send(selSetWithArray, cocoa.Array(all)))
	done := make(chan struct{}, 1)
	block := objc.NewBlock(func(objc.Block, objc.ID) { done <- struct{}{} })
	center.Send(selGetCategories, block)
	block.Release()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
	return id
}

func remove(id string) {
	if !bundled() || center == 0 {
		return
	}
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(selNew)
	defer pool.Send(selDrain)
	ids := cocoa.StringArray([]string{id})
	center.Send(selRemoveDelivered, ids)
	center.Send(selRemovePending, ids)
}

func permission(fn func(bool)) {
	if !bundled() {
		go fn(true)
		return
	}
	if center == 0 {
		go fn(false)
		return
	}
	// The answer may wait for the user, who need not answer soon: it is
	// not waited for here, where it would hold the posts behind it.
	answer := askAuthorization()
	go func() { fn(<-answer) }()
}

// notices holds the notices said.
var notices sync.Map

// notice says what once on stderr, as a line of ggui's own.
func notice(what string) {
	if _, dup := notices.LoadOrStore(what, true); !dup {
		fmt.Fprintf(os.Stderr, "ggui: %s\n", what)
	}
}

// postScript shows n through osascript, for an app outside a bundle.
func postScript(n Notification) error {
	notice("notifications show through osascript, without buttons or clicks, while the app runs outside an app bundle (go run, go build, ggui dev); a bundle ggui build makes posts them itself")
	out, err := exec.Command("osascript", osascriptArgs(n)...).CombinedOutput()
	if err != nil {
		return errors.New("osascript: " + strings.TrimSpace(string(out)) + ": " + err.Error())
	}
	return nil
}

// willPresent shows a notification while the app is in front as it is
// shown while the app is not: as a banner, in Notification Center, with
// its sound if it has one.
func willPresent(_ objc.ID, _ objc.SEL, _, _ objc.ID, done objc.Block) {
	callBlock(done, presentBanner|presentList|presentSound)
}

// didReceive hands a click on one of the app's notifications on.
func didReceive(_ objc.ID, _ objc.SEL, _, response objc.ID, done objc.Block) {
	defer callBlock(done)
	r, ok := responseOf(response)
	if ok {
		respond(r)
	}
}

// responseOf reads a UNNotificationResponse, and reports false for a
// dismissal and for a notification that is not the app's.
func responseOf(response objc.ID) (Response, bool) {
	action := goString(response.Send(selActionIdentifier))
	if action == dismissActionID {
		return Response{}, false
	}
	info := response.Send(selNotification).Send(selRequest).Send(selContent).Send(selUserInfo)
	if info == 0 {
		return Response{}, false
	}
	v := info.Send(selObjectForKey, cocoa.String(payloadKey))
	if v == 0 || !objc.Send[bool](v, selIsKindOfClass, objc.GetClass("NSString")) {
		return Response{}, false
	}
	p, ok := decodePayload(goString(v))
	if !ok {
		return Response{}, false
	}
	if action == defaultActionID {
		action = ""
	}
	return Response{ID: p.ID, Data: p.Data, Action: action, Foreground: action == "" || slices.Contains(p.Foreground, action)}, true
}

// goString copies an NSString's text.
func goString(s objc.ID) string {
	if s == 0 {
		return ""
	}
	p := objc.Send[*byte](s, selUTF8String)
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

// callBlock calls a block the framework handed over, a completion handler,
// through its invoke function, the third word of its layout.
func callBlock(block objc.Block, args ...uintptr) {
	if block == 0 {
		return
	}
	layout := *(**[3]uintptr)(unsafe.Pointer(&block))
	purego.SyscallN(layout[2], append([]uintptr{uintptr(block)}, args...)...)
	runtime.KeepAlive(block)
}
