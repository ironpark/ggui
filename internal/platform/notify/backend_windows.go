//go:build windows

package notify

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Windows shows a toast through WinRT's ToastNotificationManager, reached
// as the accessibility bridge reaches UI Automation: COM method tables
// called by index, and the one object Windows calls back, the Activated
// handler, laid out by hand in memory outside the Go heap.
//
// A toast is shown under an AppUserModelID. An app that is not packaged
// has its AUMID registered under HKCU\Software\Classes\AppUserModelId,
// with the name toasts show it by, which is what lets Windows 10 and 11
// show the toasts of an app that has no Start menu shortcut carrying the
// AUMID. The process's own AUMID is left alone: setting it would part the
// app's taskbar button from a shortcut pinned without it.
//
// Clicks are heard through each toast's Activated event while the app
// runs. A click on a toast after the app has exited would need a COM
// activator registered for the AUMID, which ggui does not register, so it
// does nothing.

// HRESULTs.
const (
	sOK          = 0
	eNoInterface = 0x80004002
	ePointer     = 0x80004003
)

// NotificationSetting values: Enabled; anything else is turned off, by the
// user, a policy or the manifest.
const settingEnabled = 0

func mustGUID(s string) windows.GUID {
	g, err := windows.GUIDFromString(s)
	if err != nil {
		panic(err)
	}
	return g
}

var (
	iidUnknown              = mustGUID("{00000000-0000-0000-C000-000000000046}")
	iidAgileObject          = mustGUID("{94EA2B94-E9CC-49E0-C0FF-EE64CA8F5B90}")
	iidManagerStatics       = mustGUID("{50AC103F-D235-4598-BBEF-98FE4D1A3AD4}")
	iidManagerStatics2      = mustGUID("{7AB93C52-0E48-4750-BA9D-1A4113981847}")
	iidToastFactory         = mustGUID("{04124B20-82C6-4229-B109-FD9ED4662B53}")
	iidToast2               = mustGUID("{9DFB9FD1-143A-490E-90BF-B9FBA1A85E9E}")
	iidXMLDocument          = mustGUID("{F7F3A506-1E87-42D6-BCFB-B8C809FA5494}")
	iidXMLDocumentIO        = mustGUID("{6CD0E74E-EE65-4489-9EBF-CA43E87BA637}")
	iidActivatedEventArgs   = mustGUID("{E3BF92F3-C197-436F-8265-0625824F8DAC}")
	iidActivatedHandlerType = mustGUID("{AB54DE2D-97D9-5528-B6AD-105AFE156530}") // TypedEventHandler<ToastNotification, Object>
)

// Method indices, counted from the start of each table: IUnknown's three
// and IInspectable's three come first.
const (
	mQueryInterface = 0
	mRelease        = 2

	mCreateNotifierWithID   = 7  // IToastNotificationManagerStatics
	mGetHistory             = 6  // IToastNotificationManagerStatics2
	mCreateToast            = 6  // IToastNotificationFactory
	mAddActivated           = 11 // IToastNotification
	mPutTag                 = 6  // IToastNotification2
	mPutGroup               = 8  // IToastNotification2
	mShow                   = 6  // IToastNotifier
	mGetSetting             = 8  // IToastNotifier
	mRemoveGroupedTagWithID = 8  // IToastNotificationHistory
	mLoadXML                = 6  // IXmlDocumentIO
	mGetArguments           = 6  // IToastActivatedEventArgs
)

var (
	combase                    = windows.NewLazySystemDLL("combase.dll")
	kernel32                   = windows.NewLazySystemDLL("kernel32.dll")
	procRoInitialize           = combase.NewProc("RoInitialize")
	procRoGetActivationFactory = combase.NewProc("RoGetActivationFactory")
	procRoActivateInstance     = combase.NewProc("RoActivateInstance")
	procWindowsCreateString    = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString    = combase.NewProc("WindowsDeleteString")
	procWindowsGetStringRaw    = combase.NewProc("WindowsGetStringRawBuffer")
	procGlobalAlloc            = kernel32.NewProc("GlobalAlloc")
)

// toasts is the state of the app's toasts. The objects are touched only
// on the posting goroutine; the mutex guards what check reads.
var toasts struct {
	sync.Mutex
	initErr error // why WinRT gave no notifier

	ready    bool
	aumid    string
	notifier uintptr // IToastNotifier
	factory  uintptr // IToastNotificationFactory
	history  uintptr // IToastNotificationHistory, or 0
	handler  uintptr // the Activated handler
	byTag    map[string]uintptr
	untagged []uintptr // the toasts shown without an ID, the latest last
}

// toastGroup is the group of every toast the app shows: a toast is known
// by its tag and group together, and a group of its own for each
// Notification.Group would make the same ID a different toast in another
// group, and leave RemoveNotification not knowing which to remove.
const toastGroup = "ggui"

// keepUntagged is how many toasts shown without an ID are kept, for their
// Activated events: Windows raises a toast's events only while the object
// is alive, and a toast without an ID is never replaced or removed.
const keepUntagged = 32

func setup() {}

func check() error {
	toasts.Lock()
	ready, err := toasts.ready, toasts.initErr
	toasts.Unlock()
	switch {
	case err != nil:
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	case ready && !enabled():
		return errDenied
	}
	return nil
}

var errDenied = fmt.Errorf("%w in Settings, System, Notifications", ErrDenied)

// enabled reports whether the app's toasts are turned on, as the notifier
// says at the moment. The notifier lives in the process's multithreaded
// apartment, which any thread may call it from; it is read only once
// made.
func enabled() bool {
	var setting int32
	return failed(call(toasts.notifier, mGetSetting, uintptr(unsafe.Pointer(&setting)))) || setting == settingEnabled
}

// failed reports whether hr is a failure.
func failed(hr uintptr) bool { return int32(hr) < 0 }

// hresult is hr as an error.
func hresult(what string, hr uintptr) error {
	return fmt.Errorf("%s: HRESULT %#08x", what, uint32(hr))
}

// call calls the method at index of the COM object obj.
func call(obj uintptr, index int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(index)*unsafe.Sizeof(uintptr(0))))
	hr, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return hr
}

func release(obj uintptr) {
	if obj != 0 {
		call(obj, mRelease)
	}
}

// query is obj's interface iid, or 0.
func query(obj uintptr, iid *windows.GUID) uintptr {
	var out uintptr
	if failed(call(obj, mQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))) {
		return 0
	}
	return out
}

// hstring is s as an HSTRING, which the caller deletes.
func hstring(s string) (uintptr, error) {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return 0, err
	}
	var h uintptr
	hr, _, _ := procWindowsCreateString.Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&h)))
	if failed(hr) {
		return 0, hresult("WindowsCreateString", hr)
	}
	return h, nil
}

func deleteHString(h uintptr) {
	if h != 0 {
		procWindowsDeleteString.Call(h)
	}
}

// goHString is the text of an HSTRING.
func goHString(h uintptr) string {
	if h == 0 {
		return ""
	}
	var n uint32
	p, _, _ := procWindowsGetStringRaw.Call(h, uintptr(unsafe.Pointer(&n)))
	if p == 0 || n == 0 {
		return ""
	}
	return windows.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(p)), n))
}

// factoryOf is the activation factory of the runtime class name, as the
// interface iid.
func factoryOf(name string, iid *windows.GUID) (uintptr, error) {
	h, err := hstring(name)
	if err != nil {
		return 0, err
	}
	defer deleteHString(h)
	var f uintptr
	hr, _, _ := procRoGetActivationFactory.Call(h, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&f)))
	if failed(hr) {
		return 0, hresult("RoGetActivationFactory "+name, hr)
	}
	return f, nil
}

// start makes the notifier, on the posting goroutine, once.
func start() error {
	toasts.Lock()
	ready, err := toasts.ready, toasts.initErr
	toasts.Unlock()
	if ready || err != nil {
		return err
	}
	err = initToasts()
	toasts.Lock()
	toasts.ready, toasts.initErr = err == nil, err
	toasts.Unlock()
	return err
}

func initToasts() error {
	// RO_INIT_MULTITHREADED. The posting goroutine keeps its thread, so
	// the apartment stays the one the objects were made in; a thread that
	// has one already is fine as it is.
	procRoInitialize.Call(1)
	o := options()
	id := aumid(o.AppID)
	name := o.AppName
	if name == "" {
		name = o.AppID
	}
	if err := registerAUMID(id, name); err != nil {
		return fmt.Errorf("registering the AppUserModelID %s: %w", id, err)
	}
	statics, err := factoryOf("Windows.UI.Notifications.ToastNotificationManager", &iidManagerStatics)
	if err != nil {
		return err
	}
	defer release(statics)
	h, err := hstring(id)
	if err != nil {
		return err
	}
	defer deleteHString(h)
	var notifier uintptr
	if hr := call(statics, mCreateNotifierWithID, h, uintptr(unsafe.Pointer(&notifier))); failed(hr) {
		return hresult("CreateToastNotifierWithId", hr)
	}
	factory, err := factoryOf("Windows.UI.Notifications.ToastNotification", &iidToastFactory)
	if err != nil {
		release(notifier)
		return err
	}
	var history uintptr
	if s2 := query(statics, &iidManagerStatics2); s2 != 0 {
		if failed(call(s2, mGetHistory, uintptr(unsafe.Pointer(&history)))) {
			history = 0
		}
		release(s2)
	}
	toasts.aumid, toasts.notifier, toasts.factory, toasts.history = id, notifier, factory, history
	toasts.handler = newActivatedHandler()
	toasts.byTag = map[string]uintptr{}
	return nil
}

// registerAUMID registers the AUMID id, shown as name, for the user,
// writing only what is not there already.
func registerAUMID(id, name string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\AppUserModelId\`+id, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if have, _, err := k.GetStringValue("DisplayName"); err == nil && have == name {
		return nil
	}
	return k.SetStringValue("DisplayName", name)
}

func post(n Notification) error {
	if err := start(); err != nil {
		return err
	}
	if !enabled() {
		return errDenied
	}
	doc, err := loadXML(toastXML(n))
	if err != nil {
		return err
	}
	defer release(doc)
	var toast uintptr
	if hr := call(toasts.factory, mCreateToast, doc, uintptr(unsafe.Pointer(&toast))); failed(hr) {
		return hresult("CreateToastNotification", hr)
	}
	tag := ""
	if n.ID != "" {
		tag = toastTag(n.ID)
		if err := setTag(toast, tag); err != nil {
			release(toast)
			return err
		}
	}
	var token int64
	call(toast, mAddActivated, toasts.handler, uintptr(unsafe.Pointer(&token)))
	if hr := call(toasts.notifier, mShow, toast); failed(hr) {
		release(toast)
		return hresult("IToastNotifier.Show", hr)
	}
	if tag != "" {
		release(toasts.byTag[tag])
		toasts.byTag[tag] = toast
	} else {
		toasts.untagged = append(toasts.untagged, toast)
		if len(toasts.untagged) > keepUntagged {
			release(toasts.untagged[0])
			toasts.untagged = toasts.untagged[1:]
		}
	}
	return nil
}

// loadXML is an XmlDocument, as its IXmlDocument, holding s.
func loadXML(s string) (uintptr, error) {
	name, err := hstring("Windows.Data.Xml.Dom.XmlDocument")
	if err != nil {
		return 0, err
	}
	defer deleteHString(name)
	var inst uintptr
	hr, _, _ := procRoActivateInstance.Call(name, uintptr(unsafe.Pointer(&inst)))
	if failed(hr) {
		return 0, hresult("RoActivateInstance XmlDocument", hr)
	}
	defer release(inst)
	io := query(inst, &iidXMLDocumentIO)
	if io == 0 {
		return 0, errors.New("XmlDocument has no IXmlDocumentIO")
	}
	defer release(io)
	h, err := hstring(s)
	if err != nil {
		return 0, err
	}
	defer deleteHString(h)
	if hr := call(io, mLoadXML, h); failed(hr) {
		return 0, hresult("XmlDocument.LoadXml", hr)
	}
	doc := query(inst, &iidXMLDocument)
	if doc == 0 {
		return 0, errors.New("XmlDocument has no IXmlDocument")
	}
	return doc, nil
}

// setTag gives toast its tag and the app's group, which together are what
// a later toast replaces it by and what removes it.
func setTag(toast uintptr, tag string) error {
	t2 := query(toast, &iidToast2)
	if t2 == 0 {
		return errors.New("ToastNotification has no IToastNotification2")
	}
	defer release(t2)
	for _, f := range []struct {
		method int
		value  string
	}{{mPutTag, tag}, {mPutGroup, toastGroup}} {
		h, err := hstring(f.value)
		if err != nil {
			return err
		}
		hr := call(t2, f.method, h)
		deleteHString(h)
		if failed(hr) {
			return hresult("IToastNotification2", hr)
		}
	}
	return nil
}

func remove(id string) {
	if start() != nil || toasts.history == 0 {
		return
	}
	tag := toastTag(id)
	hs := make([]uintptr, 3)
	for i, s := range []string{tag, toastGroup, toasts.aumid} {
		h, err := hstring(s)
		if err != nil {
			return
		}
		defer deleteHString(h)
		hs[i] = h
	}
	call(toasts.history, mRemoveGroupedTagWithID, hs[0], hs[1], hs[2])
	release(toasts.byTag[tag])
	delete(toasts.byTag, tag)
}

func permission(fn func(bool)) {
	err := start()
	go fn(err == nil && enabled())
}

// The Activated handler is a TypedEventHandler<ToastNotification, Object>:
// IUnknown and Invoke. There is one, shared by every toast, made once and
// never freed, so its reference count is not kept. It is agile, as WinRT
// asks of a handler it calls from a thread pool.

// handlerObject is the layout of the handler's memory.
type handlerObject struct {
	vtbl uintptr
}

func newActivatedHandler() uintptr {
	size := unsafe.Sizeof(uintptr(0))
	table := winAlloc(4 * size)
	methods := []uintptr{
		syscall.NewCallback(handlerQueryInterface),
		syscall.NewCallback(handlerAddRef),
		syscall.NewCallback(handlerRelease),
		syscall.NewCallback(handlerInvoke),
	}
	copy(unsafe.Slice((*uintptr)(unsafe.Pointer(table)), len(methods)), methods)
	obj := winAlloc(unsafe.Sizeof(handlerObject{}))
	(*handlerObject)(unsafe.Pointer(obj)).vtbl = table
	return obj
}

// winAlloc takes fixed, zeroed memory outside the Go heap.
func winAlloc(size uintptr) uintptr {
	// GMEM_FIXED | GMEM_ZEROINIT.
	p, _, _ := procGlobalAlloc.Call(0x0040, size)
	if p == 0 {
		panic("ggui: GlobalAlloc failed")
	}
	return p
}

func handlerQueryInterface(this, riid, ppv uintptr) uintptr {
	if ppv == 0 {
		return ePointer
	}
	id := *(*windows.GUID)(unsafe.Pointer(riid))
	if id != iidUnknown && id != iidAgileObject && id != iidActivatedHandlerType {
		*(*uintptr)(unsafe.Pointer(ppv)) = 0
		return eNoInterface
	}
	*(*uintptr)(unsafe.Pointer(ppv)) = this
	return sOK
}

func handlerAddRef(uintptr) uintptr  { return 2 }
func handlerRelease(uintptr) uintptr { return 1 }

// handlerInvoke is a click: args is the ToastActivatedEventArgs, whose
// arguments are the payload of the toast or of the button clicked.
func handlerInvoke(_, _, args uintptr) uintptr {
	if args == 0 {
		return sOK
	}
	ea := query(args, &iidActivatedEventArgs)
	if ea == 0 {
		return sOK
	}
	defer release(ea)
	var h uintptr
	if failed(call(ea, mGetArguments, uintptr(unsafe.Pointer(&h)))) {
		return sOK
	}
	s := goHString(h)
	deleteHString(h)
	if p, ok := decodePayload(s); ok {
		respond(Response{ID: p.ID, Data: p.Data, Action: p.Action, Foreground: len(p.Foreground) > 0})
	}
	return sOK
}
