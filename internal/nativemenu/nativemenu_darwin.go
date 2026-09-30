//go:build darwin && !ios

package nativemenu

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego/objc"
	"github.com/ironpark/ggfx"

	"github.com/ironpark/ggui/internal/platform/cocoa"
)

// The app's menus go between the two the engine puts in the menu bar: the
// application menu, with About, Hide and Quit, stays first, and the Window
// menu stays last. A tray is an NSStatusItem. Every action, in either, is
// one method of one target object, which reads the sender's tag and hands
// it to the callback Handle was given. All of it runs on the main thread,
// which is AppKit's.

var (
	selMainMenu             = objc.RegisterName("mainMenu")
	selSetMainMenu          = objc.RegisterName("setMainMenu:")
	selNumberOfItems        = objc.RegisterName("numberOfItems")
	selSetSubmenu           = objc.RegisterName("setSubmenu:")
	selInsertItemAtIndex    = objc.RegisterName("insertItem:atIndex:")
	selAddItem              = objc.RegisterName("addItem:")
	selRemoveItem           = objc.RegisterName("removeItem:")
	selAlloc                = objc.RegisterName("alloc")
	selInit                 = objc.RegisterName("init")
	selInitWithTitle        = objc.RegisterName("initWithTitle:")
	selInitItem             = objc.RegisterName("initWithTitle:action:keyEquivalent:")
	selRelease              = objc.RegisterName("release")
	selRetain               = objc.RegisterName("retain")
	selAutorelease          = objc.RegisterName("autorelease")
	selSeparatorItem        = objc.RegisterName("separatorItem")
	selSetTarget            = objc.RegisterName("setTarget:")
	selSetAction            = objc.RegisterName("setAction:")
	selSetTag               = objc.RegisterName("setTag:")
	selTag                  = objc.RegisterName("tag")
	selSetEnabled           = objc.RegisterName("setEnabled:")
	selSetState             = objc.RegisterName("setState:")
	selSetAutoenablesItems  = objc.RegisterName("setAutoenablesItems:")
	selSetKeyEquivalentMask = objc.RegisterName("setKeyEquivalentModifierMask:")
	selSelect               = objc.RegisterName("gguiMenuSelect:")

	selSystemStatusBar  = objc.RegisterName("systemStatusBar")
	selStatusItem       = objc.RegisterName("statusItemWithLength:")
	selRemoveStatusItem = objc.RegisterName("removeStatusItem:")
	selButton           = objc.RegisterName("button")
	selSetMenu          = objc.RegisterName("setMenu:")
	selSetTitle         = objc.RegisterName("setTitle:")
	selSetImage         = objc.RegisterName("setImage:")
	selSetImagePosition = objc.RegisterName("setImagePosition:")
	selSetToolTip       = objc.RegisterName("setToolTip:")
	selDataWithBytes    = objc.RegisterName("dataWithBytes:length:")
	selInitWithData     = objc.RegisterName("initWithData:")
	selSetTemplate      = objc.RegisterName("setTemplate:")
	selSetSize          = objc.RegisterName("setSize:")
	selSize             = objc.RegisterName("size")

	classNSMenu      = objc.ID(objc.GetClass("NSMenu"))
	classNSMenuItem  = objc.ID(objc.GetClass("NSMenuItem"))
	classNSStatusBar = objc.ID(objc.GetClass("NSStatusBar"))
	classNSData      = objc.ID(objc.GetClass("NSData"))
	classNSImage     = objc.ID(objc.GetClass("NSImage"))
)

// NSEventModifierFlags.
const (
	modShift = 1 << 17
	modCtrl  = 1 << 18
	modAlt   = 1 << 19
	modCmd   = 1 << 20
)

const (
	nsVariableStatusItemLength = -1
	nsImageLeft                = 2
	// trayIconSize is the height of a status item's icon, in points: what
	// fits the menu bar.
	trayIconSize = 18.0
)

type nsSize struct{ W, H float64 }

var (
	mu       sync.Mutex
	onSelect func(id int)
	items    = map[int]objc.ID{} // the action items made, by id

	// Main-thread state.
	target  objc.ID
	barMenu []objc.ID               // the menu bar items Set inserted
	barIDs  []int                   // the actions among them
	trays   = map[int]*statusItem{} // by the caller's key
)

// statusItem is one tray's NSStatusItem and the actions of its menu.
type statusItem struct {
	item objc.ID
	ids  []int
}

// targetClass is the class of target, the one object every action is
// sent to.
var targetClass = sync.OnceValue(func() objc.Class {
	c, err := objc.RegisterClass("GguiMenuTarget", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{{
		Cmd: selSelect,
		Fn: func(_ objc.ID, _ objc.SEL, sender objc.ID) {
			id := objc.Send[int](sender, selTag)
			mu.Lock()
			fn := onSelect
			mu.Unlock()
			if fn != nil {
				fn(id)
			}
		},
	}})
	if err != nil {
		panic("ggui: " + err.Error())
	}
	return c
})

// Supported reports whether the platform shows Set's menus.
func Supported() bool { return true }

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

// Set replaces the app's menus in the menu bar with menus, each a titled
// item whose Items are its entries. It needs a running app: before the
// engine has a window it does nothing.
func Set(menus []Item) {
	ggfx.RunOnMainThread(func() { install(menus) })
}

// install swaps the menus in. It runs on the main thread.
func install(menus []Item) {
	app := cocoa.App()
	bar := app.Send(selMainMenu)
	if bar == 0 {
		bar = classNSMenu.Send(selAlloc).Send(selInit)
		app.Send(selSetMainMenu, bar)
		bar.Send(selRelease)
	}
	for _, it := range barMenu {
		bar.Send(selRemoveItem, it)
	}
	barMenu = barMenu[:0]
	forget(barIDs)
	barIDs = barIDs[:0]

	// After the application menu, and before the Window menu if the bar
	// ends with it.
	at := min(1, objc.Send[int](bar, selNumberOfItems))
	for _, m := range menus {
		it := submenuItem(m, &barIDs)
		bar.Send(selInsertItemAtIndex, it, at)
		it.Send(selRelease)
		barMenu = append(barMenu, it)
		at++
	}
}

// menu makes an NSMenu of entries, owned by the caller, recording the ids
// of its actions in ids.
func menu(title string, entries []Item, ids *[]int) objc.ID {
	m := classNSMenu.Send(selAlloc).Send(selInitWithTitle, cocoa.String(title))
	// The items' enabled state is ggui's to say, not a responder chain's.
	m.Send(selSetAutoenablesItems, false)
	for _, e := range entries {
		child := entry(e, ids)
		m.Send(selAddItem, child)
		if !e.Separator {
			child.Send(selRelease)
		}
	}
	return m
}

// submenuItem makes the menu bar or menu item that opens m's entries.
func submenuItem(m Item, ids *[]int) objc.ID {
	it := classNSMenuItem.Send(selAlloc).Send(selInitItem, cocoa.String(m.Title), objc.SEL(0), cocoa.String(""))
	sub := menu(m.Title, m.Items, ids)
	it.Send(selSetSubmenu, sub)
	sub.Send(selRelease)
	return it
}

// entry makes one menu item. A separator is shared and autoreleased; every
// other item is owned by the caller.
func entry(e Item, ids *[]int) objc.ID {
	if e.Separator {
		return classNSMenuItem.Send(selSeparatorItem)
	}
	if len(e.Items) > 0 {
		return submenuItem(e, ids)
	}
	if target == 0 {
		target = objc.ID(targetClass()).Send(selAlloc).Send(selInit)
	}
	it := classNSMenuItem.Send(selAlloc).Send(selInitItem, cocoa.String(e.Title), selSelect, cocoa.String(e.Key))
	if e.Key != "" {
		var mask uint
		if e.Cmd {
			mask |= modCmd
		}
		if e.Shift {
			mask |= modShift
		}
		if e.Alt {
			mask |= modAlt
		}
		if e.Ctrl {
			mask |= modCtrl
		}
		it.Send(selSetKeyEquivalentMask, mask)
	}
	it.Send(selSetTarget, target)
	it.Send(selSetTag, e.ID)
	it.Send(selSetEnabled, e.Enabled)
	it.Send(selSetState, state(e.Checked))
	mu.Lock()
	items[e.ID] = it
	mu.Unlock()
	*ids = append(*ids, e.ID)
	return it
}

// forget drops the action items with the ids, whose menu is going away.
func forget(ids []int) {
	mu.Lock()
	for _, id := range ids {
		delete(items, id)
	}
	mu.Unlock()
}

func state(checked bool) int {
	if checked {
		return 1 // NSControlStateValueOn
	}
	return 0
}

// SetEnabled enables or disables the action item with the id.
func SetEnabled(id int, on bool) {
	ggfx.RunOnMainThread(func() {
		if it := item(id); it != 0 {
			it.Send(selSetEnabled, on)
		}
	})
}

// SetChecked puts a check mark by the action item with the id, or takes it
// away.
func SetChecked(id int, on bool) {
	ggfx.RunOnMainThread(func() {
		if it := item(id); it != 0 {
			it.Send(selSetState, state(on))
		}
	})
}

func item(id int) objc.ID {
	mu.Lock()
	defer mu.Unlock()
	return items[id]
}

// SetTray shows t in the status area as the icon named key, making it or
// replacing what it showed. It needs a running app, as Set does.
func SetTray(key int, t Tray) {
	ggfx.RunOnMainThread(func() { installTray(key, t) })
}

// installTray makes or updates a status item. It runs on the main thread.
func installTray(key int, t Tray) {
	s := trays[key]
	if s == nil {
		item := classNSStatusBar.Send(selSystemStatusBar).Send(selStatusItem, float64(nsVariableStatusItemLength))
		item.Send(selRetain)
		s = &statusItem{item: item}
		trays[key] = s
	}
	forget(s.ids)
	s.ids = s.ids[:0]
	if target == 0 {
		target = objc.ID(targetClass()).Send(selAlloc).Send(selInit)
	}
	button := s.item.Send(selButton)
	button.Send(selSetTitle, cocoa.String(t.Title))
	button.Send(selSetToolTip, cocoa.String(t.Tooltip))
	button.Send(selSetImage, trayImage(t.Icon, t.Template))
	if len(t.Icon) > 0 && t.Title != "" {
		button.Send(selSetImagePosition, nsImageLeft)
	}
	if len(t.Menu) > 0 {
		m := menu("", t.Menu, &s.ids)
		s.item.Send(selSetMenu, m)
		m.Send(selRelease)
		return
	}
	s.item.Send(selSetMenu, objc.ID(0))
	button.Send(selSetTarget, target)
	button.Send(selSetAction, selSelect)
	button.Send(selSetTag, t.ClickID)
}

// trayImage is an autoreleased NSImage of png, sized to the menu bar, or
// nil for no png.
func trayImage(png []byte, template bool) objc.ID {
	if len(png) == 0 {
		return 0
	}
	data := classNSData.Send(selDataWithBytes, unsafe.Pointer(&png[0]), uint(len(png)))
	img := classNSImage.Send(selAlloc).Send(selInitWithData, data)
	if img == 0 {
		return 0
	}
	size := objc.Send[nsSize](img, selSize)
	if size.H > 0 {
		img.Send(selSetSize, nsSize{W: size.W * trayIconSize / size.H, H: trayIconSize})
	}
	img.Send(selSetTemplate, template)
	// The button takes its own reference when it is handed the image.
	return img.Send(selAutorelease)
}

// RemoveTray takes the icon named key out of the status area.
func RemoveTray(key int) {
	ggfx.RunOnMainThread(func() {
		s := trays[key]
		if s == nil {
			return
		}
		forget(s.ids)
		classNSStatusBar.Send(selSystemStatusBar).Send(selRemoveStatusItem, s.item)
		s.item.Send(selRelease)
		delete(trays, key)
	})
}
