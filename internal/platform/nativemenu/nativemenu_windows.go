//go:build windows

package nativemenu

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"strings"
	"sync"
	"unsafe"

	"github.com/ironpark/ggwd"
	"golang.org/x/image/draw"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// A tray is a notification-area icon of shell32, owned by one hidden
// window of the package's own that receives its clicks. The window lives
// on the main thread, whose message loop is ggwd's, so its procedure runs
// there, as AppKit's actions do on macOS. A tray's menu is a popup menu
// built when it opens, from the items SetTray was given and the states
// SetEnabled and SetChecked have set since; there is no menu bar.

var (
	dllUser32  = windows.NewLazySystemDLL("user32.dll")
	dllGdi32   = windows.NewLazySystemDLL("gdi32.dll")
	dllShell32 = windows.NewLazySystemDLL("shell32.dll")
	dllKernel  = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW       = dllUser32.NewProc("RegisterClassExW")
	procCreateWindowExW        = dllUser32.NewProc("CreateWindowExW")
	procDefWindowProcW         = dllUser32.NewProc("DefWindowProcW")
	procRegisterWindowMessageW = dllUser32.NewProc("RegisterWindowMessageW")
	procCreatePopupMenu        = dllUser32.NewProc("CreatePopupMenu")
	procAppendMenuW            = dllUser32.NewProc("AppendMenuW")
	procTrackPopupMenuEx       = dllUser32.NewProc("TrackPopupMenuEx")
	procDestroyMenu            = dllUser32.NewProc("DestroyMenu")
	procSetForegroundWindow    = dllUser32.NewProc("SetForegroundWindow")
	procPostMessageW           = dllUser32.NewProc("PostMessageW")
	procGetSystemMetrics       = dllUser32.NewProc("GetSystemMetrics")
	procCreateIconIndirect     = dllUser32.NewProc("CreateIconIndirect")
	procDestroyIcon            = dllUser32.NewProc("DestroyIcon")
	procLoadIconW              = dllUser32.NewProc("LoadIconW")
	procCreateDIBSection       = dllGdi32.NewProc("CreateDIBSection")
	procCreateBitmap           = dllGdi32.NewProc("CreateBitmap")
	procDeleteObject           = dllGdi32.NewProc("DeleteObject")
	procShellNotifyIconW       = dllShell32.NewProc("Shell_NotifyIconW")
	procExtractIconExW         = dllShell32.NewProc("ExtractIconExW")
	procGetModuleHandleW       = dllKernel.NewProc("GetModuleHandleW")
)

// Shell_NotifyIconW messages and flags, from shellapi.h.
const (
	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nimSetVersion = 4

	nifMessage  = 0x01
	nifIcon     = 0x02
	nifTip      = 0x04
	nifShowTip  = 0x80
	nifVersion4 = 4

	ninSelect    = 0x400 // WM_USER
	ninKeySelect = 0x401
)

// Window messages, from winuser.h.
const (
	wmNull          = 0x0000
	wmSettingChange = 0x001A
	wmContextMenu   = 0x007B
	wmApp           = 0x8000

	// wmTray is the message the shell sends the window about its icons.
	wmTray = wmApp + 1
)

// Menu flags, from winuser.h.
const (
	mfString    = 0x0000
	mfGrayed    = 0x0001
	mfChecked   = 0x0008
	mfPopup     = 0x0010
	mfSeparator = 0x0800

	tpmRightAlign  = 0x0008
	tpmBottomAlign = 0x0020
	tpmNoNotify    = 0x0080
	tpmReturnCmd   = 0x0100
	tpmRightButton = 0x0002

	smCxSmIcon          = 49
	smMenuDropAlignment = 40

	wsPopup        = 0x80000000
	wsExToolWindow = 0x00000080
	idiApplication = 32512
	dibRGBColors   = 0
	biRGB          = 0
)

// notifyIconData is NOTIFYICONDATAW, laid out as the shell reads it.
type notifyIconData struct {
	size            uint32
	wnd             windows.HWND
	id              uint32
	flags           uint32
	callbackMessage uint32
	icon            windows.Handle
	tip             [128]uint16
	state           uint32
	stateMask       uint32
	info            [256]uint16
	version         uint32 // a union with uTimeout
	infoTitle       [64]uint16
	infoFlags       uint32
	guidItem        windows.GUID
	balloonIcon     windows.Handle
}

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   windows.Handle
	icon       windows.Handle
	cursor     windows.Handle
	background windows.Handle
	menuName   *uint16
	className  *uint16
	iconSm     windows.Handle
}

type bitmapInfoHeader struct {
	size          uint32
	width         int32
	height        int32
	planes        uint16
	bitCount      uint16
	compression   uint32
	sizeImage     uint32
	xPelsPerMeter int32
	yPelsPerMeter int32
	clrUsed       uint32
	clrImportant  uint32
}

type iconInfo struct {
	icon    int32 // BOOL
	xHot    uint32
	yHot    uint32
	mask    windows.Handle
	bmColor windows.Handle
}

var (
	mu       sync.Mutex
	onSelect func(id int)
	enabled  = map[int]bool{} // states set since a tray was given its menu
	checked  = map[int]bool{}

	// Main-thread state, or the state of the thread that ends the app.
	hwnd           windows.HWND
	taskbarCreated uint32
	fallbackIcon   windows.Handle
	trays          = map[int]*trayIcon{} // by the caller's key
)

// trayIcon is one tray as the shell shows it.
type trayIcon struct {
	cfg   Tray
	icon  windows.Handle // the icon made from cfg.Icon; zero when shared
	added bool           // whether the shell has it
}

// Supported reports whether the platform shows Set's menus: Windows has no
// app menu bar of its own.
func Supported() bool { return false }

// TraySupported reports whether the platform shows SetTray's icons.
func TraySupported() bool { return true }

// Handle sets the function called with an action's id when it is chosen,
// in a menu or by a tray's click. It runs on the main thread and must hand
// the work on.
func Handle(selected func(id int)) {
	mu.Lock()
	onSelect = selected
	mu.Unlock()
}

// Set does nothing: Windows has no app menu bar.
func Set([]Item) {}

// SetEnabled enables or disables the action with the id in the tray menus
// built from now on.
func SetEnabled(id int, on bool) {
	mu.Lock()
	enabled[id] = on
	mu.Unlock()
}

// SetChecked puts a check mark by the action with the id in the tray menus
// built from now on, or takes it away.
func SetChecked(id int, on bool) {
	mu.Lock()
	checked[id] = on
	mu.Unlock()
}

// SetTray shows t in the notification area as the icon named key, making
// it or replacing what it showed. It needs a running app.
func SetTray(key int, t Tray) {
	ggwd.RunOnMainThread(func() {
		if !ensureWindow() {
			return
		}
		s := trays[key]
		if s == nil {
			s = &trayIcon{}
			trays[key] = s
		}
		forgetStates(s.cfg.Menu)
		s.cfg = t
		install(key, s)
	})
}

// RemoveTray takes the icon named key out of the notification area. As the
// app ends, when the main thread no longer runs calls, it removes the icon
// where it is called: the shell takes the call from any thread, and an
// icon left behind would stay in the tray until the pointer passes over it.
func RemoveTray(key int) {
	ran := false
	ggwd.RunOnMainThread(func() { ran = true; removeTray(key) })
	if !ran {
		removeTray(key)
	}
}

func removeTray(key int) {
	s := trays[key]
	if s == nil {
		return
	}
	delete(trays, key)
	forgetStates(s.cfg.Menu)
	if s.added {
		nid := notifyIconData{wnd: hwnd, id: uint32(key)}
		nid.size = uint32(unsafe.Sizeof(nid))
		notify(nimDelete, &nid)
	}
	if s.icon != 0 {
		procDestroyIcon.Call(uintptr(s.icon))
	}
}

// ContextSupported reports whether native popup menus are available.
func ContextSupported() bool { return false }

// Popup does nothing: context menus are drawn by ggui on Windows.
func Popup(uintptr, float64, float64, []Item) int { return 0 }

// forgetStates drops the states set for the actions of entries, whose menu
// is going away.
func forgetStates(entries []Item) {
	mu.Lock()
	defer mu.Unlock()
	var walk func([]Item)
	walk = func(entries []Item) {
		for _, e := range entries {
			delete(enabled, e.ID)
			delete(checked, e.ID)
			walk(e.Items)
		}
	}
	walk(entries)
}

// ensureWindow makes the hidden window the icons belong to. It is a
// top-level window rather than a message-only one, which would miss the
// TaskbarCreated broadcast that says Explorer restarted and lost them. It
// runs on the main thread.
func ensureWindow() bool {
	if hwnd != 0 {
		return true
	}
	instance, _, _ := procGetModuleHandleW.Call(0)
	class, _ := windows.UTF16PtrFromString("GguiTrayWindow")
	wc := wndClassEx{
		wndProc:   windows.NewCallback(wndProc),
		instance:  windows.Handle(instance),
		className: class,
	}
	wc.size = uint32(unsafe.Sizeof(wc))
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	h, _, _ := procCreateWindowExW.Call(wsExToolWindow, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(class)),
		wsPopup, 0, 0, 0, 0, 0, 0, instance, 0)
	if h == 0 {
		return false
	}
	name, _ := windows.UTF16PtrFromString("TaskbarCreated")
	m, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(name)))
	hwnd = windows.HWND(h)
	taskbarCreated = uint32(m)
	return true
}

// install hands the shell s's icon and tooltip, adding the icon when the
// shell does not have it. It runs on the main thread.
func install(key int, s *trayIcon) {
	old := s.icon
	s.icon = makeIcon(s.cfg.Icon, s.cfg.Template)
	icon := s.icon
	if icon == 0 {
		icon = appIcon()
	}
	nid := notifyIconData{
		wnd:             hwnd,
		id:              uint32(key),
		flags:           nifMessage | nifIcon | nifTip | nifShowTip,
		callbackMessage: wmTray,
		icon:            icon,
	}
	nid.size = uint32(unsafe.Sizeof(nid))
	tip := s.cfg.Tooltip
	if tip == "" {
		// The notification area shows no text beside an icon.
		tip = s.cfg.Title
	}
	copy(nid.tip[:len(nid.tip)-1], windows.StringToUTF16(tip))
	nid.tip[len(nid.tip)-1] = 0
	if !s.added || !notify(nimModify, &nid) {
		s.added = notify(nimAdd, &nid)
		if s.added {
			nid.version = nifVersion4
			notify(nimSetVersion, &nid)
		}
	}
	if old != 0 {
		procDestroyIcon.Call(uintptr(old))
	}
}

func notify(msg uintptr, nid *notifyIconData) bool {
	r, _, _ := procShellNotifyIconW.Call(msg, uintptr(unsafe.Pointer(nid)))
	return r != 0
}

// wndProc is the hidden window's procedure. It runs on the main thread.
func wndProc(wnd windows.HWND, msg uint32, wparam, lparam uintptr) uintptr {
	switch {
	case msg == wmTray:
		key := int(lparam >> 16 & 0xFFFF)
		event := uint32(lparam & 0xFFFF)
		x, y := int32(int16(wparam&0xFFFF)), int32(int16(wparam>>16&0xFFFF))
		clicked(key, event, x, y)
		return 0
	case msg == taskbarCreated && taskbarCreated != 0:
		// Explorer restarted, and its tray with no icon of ours.
		for key, s := range trays {
			s.added = false
			install(key, s)
		}
		return 0
	case msg == wmSettingChange:
		// A template icon follows the taskbar between light and dark.
		for key, s := range trays {
			if s.cfg.Template && len(s.cfg.Icon) > 0 {
				install(key, s)
			}
		}
	}
	r, _, _ := procDefWindowProcW.Call(uintptr(wnd), uintptr(msg), wparam, lparam)
	return r
}

// clicked answers an event about the icon named key: a menu opens on a
// click of either button, as the menu bar's does on macOS; an icon without
// one runs its action on the primary button or the Enter key.
func clicked(key int, event uint32, x, y int32) {
	s := trays[key]
	if s == nil {
		return
	}
	switch event {
	case ninSelect, ninKeySelect, wmContextMenu:
	default:
		return
	}
	var id int
	if len(s.cfg.Menu) > 0 {
		id = track(s.cfg.Menu, x, y)
	} else if event != wmContextMenu {
		id = s.cfg.ClickID
	}
	if id == 0 {
		return
	}
	mu.Lock()
	fn := onSelect
	mu.Unlock()
	if fn != nil {
		fn(id)
	}
}

// track opens a menu of entries at the screen point and returns the id of
// the action chosen, or zero when it is dismissed.
func track(entries []Item, x, y int32) int {
	m := popupMenu(entries)
	if m == 0 {
		return 0
	}
	defer procDestroyMenu.Call(m)
	// Without the foreground, the menu would stay open after a click
	// elsewhere; the posted message is the documented way to close it
	// properly on a second click of the icon.
	procSetForegroundWindow.Call(uintptr(hwnd))
	flags := uintptr(tpmReturnCmd | tpmNoNotify | tpmRightButton | tpmBottomAlign)
	if right, _, _ := procGetSystemMetrics.Call(smMenuDropAlignment); right != 0 {
		flags |= tpmRightAlign
	}
	id, _, _ := procTrackPopupMenuEx.Call(m, flags, uintptr(x), uintptr(y), uintptr(hwnd), 0)
	procPostMessageW.Call(uintptr(hwnd), wmNull, 0, 0)
	return int(id)
}

// popupMenu builds a popup menu of entries, owned by the caller.
func popupMenu(entries []Item) uintptr {
	m, _, _ := procCreatePopupMenu.Call()
	if m == 0 {
		return 0
	}
	mu.Lock()
	defer mu.Unlock()
	var fill func(m uintptr, entries []Item)
	fill = func(m uintptr, entries []Item) {
		for _, e := range entries {
			if e.Separator {
				procAppendMenuW.Call(m, mfSeparator, 0, 0)
				continue
			}
			title := menuTitle(e.Title)
			if len(e.Items) > 0 {
				sub, _, _ := procCreatePopupMenu.Call()
				if sub == 0 {
					continue
				}
				fill(sub, e.Items)
				procAppendMenuW.Call(m, mfPopup|mfString, sub, uintptr(unsafe.Pointer(title)))
				continue
			}
			on, ok := enabled[e.ID]
			if !ok {
				on = e.Enabled
			}
			check, ok := checked[e.ID]
			if !ok {
				check = e.Checked
			}
			flags := uintptr(mfString)
			if !on {
				flags |= mfGrayed
			}
			if check {
				flags |= mfChecked
			}
			procAppendMenuW.Call(m, flags, uintptr(e.ID), uintptr(unsafe.Pointer(title)))
		}
	}
	fill(m, entries)
	return m
}

// menuTitle is title as a menu shows it: an ampersand marks the mnemonic
// there, so a literal one is doubled.
func menuTitle(title string) *uint16 {
	p, _ := windows.UTF16PtrFromString(strings.ReplaceAll(strings.ReplaceAll(title, "\x00", ""), "&", "&&"))
	return p
}

// appIcon is the icon of an icon-less tray: the executable's own, or the
// system's application icon. It is shared, never destroyed.
func appIcon() windows.Handle {
	if fallbackIcon != 0 {
		return fallbackIcon
	}
	if exe, err := os.Executable(); err == nil {
		if p, err := windows.UTF16PtrFromString(exe); err == nil {
			var small windows.Handle
			if n, _, _ := procExtractIconExW.Call(uintptr(unsafe.Pointer(p)), 0, 0, uintptr(unsafe.Pointer(&small)), 1); n > 0 && small != 0 {
				fallbackIcon = small
				return fallbackIcon
			}
		}
	}
	h, _, _ := procLoadIconW.Call(0, idiApplication)
	fallbackIcon = windows.Handle(h)
	return fallbackIcon
}

// makeIcon makes an icon of the notification area's size from a PNG,
// fitted within the square and centred. A template is drawn from its alpha
// alone, in white on a dark taskbar and black on a light one. It returns
// zero for no PNG, or one that does not decode.
func makeIcon(data []byte, template bool) windows.Handle {
	if len(data) == 0 {
		return 0
	}
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return 0
	}
	size := 16
	if n, _, _ := procGetSystemMetrics.Call(smCxSmIcon); n > 0 {
		size = int(n)
	}
	sb := src.Bounds()
	if sb.Dx() <= 0 || sb.Dy() <= 0 {
		return 0
	}
	w, h := size, size
	if sb.Dx() > sb.Dy() {
		h = max(1, size*sb.Dy()/sb.Dx())
	} else {
		w = max(1, size*sb.Dx()/sb.Dy())
	}
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	at := image.Rect((size-w)/2, (size-h)/2, (size-w)/2+w, (size-h)/2+h)
	draw.CatmullRom.Scale(dst, at, src, sb, draw.Src, nil)
	if template {
		var ink uint8 = 0xFF
		if lightTaskbar() {
			ink = 0
		}
		for i := 0; i < len(dst.Pix); i += 4 {
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2] = ink, ink, ink
		}
	}

	hdr := bitmapInfoHeader{width: int32(size), height: -int32(size), planes: 1, bitCount: 32, compression: biRGB}
	hdr.size = uint32(unsafe.Sizeof(hdr))
	var bits unsafe.Pointer
	color, _, _ := procCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&hdr)), dibRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if color == 0 || bits == nil {
		return 0
	}
	defer procDeleteObject.Call(color)
	// The icon's colour is straight BGRA; its alpha is what masks it.
	px := unsafe.Slice((*byte)(bits), size*size*4)
	for i := 0; i < len(px); i += 4 {
		px[i], px[i+1], px[i+2], px[i+3] = dst.Pix[i+2], dst.Pix[i+1], dst.Pix[i], dst.Pix[i+3]
	}
	// An all-clear mask: the alpha alone says what shows. Its rows are
	// whole 16-bit words.
	maskBits := make([]byte, (size+15)/16*2*size)
	mask, _, _ := procCreateBitmap.Call(uintptr(size), uintptr(size), 1, 1, uintptr(unsafe.Pointer(&maskBits[0])))
	if mask == 0 {
		return 0
	}
	defer procDeleteObject.Call(mask)
	info := iconInfo{icon: 1, mask: windows.Handle(mask), bmColor: windows.Handle(color)}
	icon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&info)))
	return windows.Handle(icon)
}

// lightTaskbar reports whether the taskbar is drawn light, as Windows 10
// and later allow; it is dark by default.
func lightTaskbar() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("SystemUsesLightTheme")
	return err == nil && v != 0
}
