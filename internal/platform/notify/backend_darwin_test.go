//go:build darwin && !ios

package notify

import (
	"testing"

	"github.com/ebitengine/purego/objc"
)

// The framework opens and every class, selector and protocol the backend
// sends to is there, and the delegate class registers with the protocol's
// methods. Nothing is posted: a test binary has no bundle, and asking it
// for the notification center would end the process.
func TestUserNotificationsResolve(t *testing.T) {
	if !loadFramework() {
		t.Fatal("UserNotifications did not open")
	}
	c := lookupClasses()
	for name, id := range map[string]objc.ID{
		"UNUserNotificationCenter":     c.center,
		"UNMutableNotificationContent": c.content,
		"UNNotificationRequest":        c.request,
		"UNNotificationSound":          c.sound,
		"UNNotificationAction":         c.action,
		"UNNotificationCategory":       c.category,
	} {
		if id == 0 {
			t.Errorf("class %s is missing", name)
		}
	}
	class, err := delegateClass()
	if err != nil || class == 0 {
		t.Fatalf("delegate class: %v", err)
	}
	responds := objc.RegisterName("instancesRespondToSelector:")
	for _, sel := range []objc.SEL{selWillPresent, selDidReceive} {
		if !objc.Send[bool](objc.ID(class), responds, sel) {
			t.Errorf("the delegate does not respond to a method of the protocol")
		}
	}
	for _, check := range []struct {
		class objc.ID
		sel   objc.SEL
	}{
		{c.center, selCurrentCenter},
		{c.request, selRequestWith},
		{c.sound, selDefaultSound},
		{c.action, selActionWith},
		{c.category, selCategoryWith},
	} {
		if !objc.Send[bool](check.class, objc.RegisterName("respondsToSelector:"), check.sel) {
			t.Errorf("a class does not respond to a selector the backend sends it")
		}
	}
	instance := objc.RegisterName("instancesRespondToSelector:")
	for _, sel := range []objc.SEL{selSetTitle, selSetSubtitle, selSetBody, selSetSound, selSetUserInfo, selSetThreadID, selSetCategoryID} {
		if !objc.Send[bool](c.content, instance, sel) {
			t.Errorf("UNMutableNotificationContent does not respond to a setter the backend sends")
		}
	}
	for _, sel := range []objc.SEL{selSetDelegate, selRequestAuth, selAddRequest, selRemoveDelivered, selRemovePending, selSetCategories, selGetCategories} {
		if !objc.Send[bool](c.center, instance, sel) {
			t.Errorf("UNUserNotificationCenter does not respond to a method the backend sends")
		}
	}
}

// A test binary is not bundled, so it posts through osascript, and
// UserNotifications is never asked for its center.
func TestUnbundledIsNotBundled(t *testing.T) {
	if bundled() {
		t.Skip("the test runs from a bundle")
	}
	setupOnce.Do(setup)
	if center != 0 {
		t.Error("an unbundled process asked for the notification center")
	}
	if err := check(); err != nil {
		t.Errorf("check = %v for the osascript path", err)
	}
}
