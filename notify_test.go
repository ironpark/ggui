package ggui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ironpark/ggui/internal/platform/notify"
)

func TestProbeRecordsNotifications(t *testing.T) {
	t.Parallel()
	p := NewProbe(sized(10, 10), Sz(10, 10))
	defer p.Close()
	var h Host = p
	actions := []NotificationAction{{ID: "read", Title: "Mark as read"}}
	if err := h.Notify(Notification{ID: "a", Title: "first", Actions: actions}); err != nil {
		t.Fatal(err)
	}
	actions[0].Title = "changed after" // Notify keeps a copy
	if err := h.Notify(Notification{Title: "untagged"}); err != nil {
		t.Fatal(err)
	}
	if err := h.Notify(Notification{ID: "a", Title: "second", Actions: []NotificationAction{{ID: "read", Title: "Mark as read"}}}); err != nil {
		t.Fatal(err)
	}
	got := p.Notifications()
	if len(got) != 2 || got[0].Title != "untagged" || got[1].Title != "second" {
		t.Fatalf("Notifications() = %+v", got)
	}
	got[1].Actions[0].Title = "x" // Notifications returns copies
	if p.Notifications()[1].Actions[0].Title != "Mark as read" {
		t.Fatal("Notifications handed out the probe's own slice")
	}
	h.RemoveNotification("a")
	if got := p.Notifications(); len(got) != 1 || got[0].Title != "untagged" {
		t.Fatalf("after RemoveNotification: %+v", got)
	}
}

func TestNotifyRejectsWhatCannotBeShown(t *testing.T) {
	t.Parallel()
	p := NewProbe(sized(10, 10), Sz(10, 10))
	defer p.Close()
	if err := p.Notify(Notification{ID: "x", Data: "d"}); !errors.Is(err, errEmptyNotification) {
		t.Errorf("Notify with no text = %v", err)
	}
	if err := p.Notify(Notification{Title: "t", Actions: []NotificationAction{{Title: "No id"}}}); err == nil || !strings.Contains(err.Error(), "No id") {
		t.Errorf("Notify with an action without an id = %v", err)
	}
	if len(p.Notifications()) != 0 {
		t.Error("a rejected notification was recorded")
	}
}

func TestProbeClickNotificationRunsHandlersOnTheUIThread(t *testing.T) {
	t.Parallel()
	read := State("")
	var got []NotificationResponse
	p := ProbeBuilder(func() Widget {
		UseHost().OnNotification(func(r NotificationResponse) {
			got = append(got, r)
			read.Set(r.Data) // a write a handler off the UI thread could not make
		})
		return sized(10, 10)
	}, Sz(10, 10))
	defer p.Close()
	p.Frame()
	_ = p.Notify(Notification{ID: "chat-42", Title: "Ada", Data: "chat/42", Actions: []NotificationAction{{ID: "read", Title: "Mark as read"}}})
	_ = p.Notify(Notification{ID: "chat-43", Title: "Bob", Data: "chat/43"})
	p.ClickNotification("chat-42", "read")
	p.ClickNotification("chat-43", "")
	want := []NotificationResponse{{ID: "chat-42", Data: "chat/42", Action: "read"}, {ID: "chat-43", Data: "chat/43"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("responses %+v, want %+v", got, want)
	}
	if Untrack(read.Get) != "chat/43" {
		t.Fatalf("the handler's write is %q", Untrack(read.Get))
	}
	if len(p.Notifications()) != 0 {
		t.Fatalf("clicked notifications stay: %+v", p.Notifications())
	}
	for name, click := range map[string]func(){
		"a notification it does not show": func() { p.ClickNotification("chat-42", "") },
		"a button it does not have":       func() { _ = p.Notify(Notification{ID: "b", Title: "b"}); p.ClickNotification("b", "nope") },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("clicking %s did not panic", name)
				}
			}()
			click()
		}()
	}
}

func TestNotificationToPlatform(t *testing.T) {
	t.Parallel()
	n := Notification{ID: "i", Title: "t", Subtitle: "s", Body: "b", Sound: true, Group: "g", Data: "d",
		Actions: []NotificationAction{{ID: "a", Title: "A", Foreground: true}}}
	got, err := n.toPlatform()
	want := notify.Notification{ID: "i", Title: "t", Subtitle: "s", Body: "b", Sound: true, Group: "g", Data: "d",
		Actions: []notify.Action{{ID: "a", Title: "A", Foreground: true}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("toPlatform = %+v, %v", got, err)
	}
}

// A click that comes before the windows are open, as one that launched the
// app does, waits for them and for a handler; it brings the app forward
// once, and reaches the handler registered after.
func TestAppHoldsClicksUntilItRunsAndHasAHandler(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return sized(0, 0) })
	defer a.Close()
	a.Window.Hide()
	a.notificationClicked(notify.Response{ID: "x", Data: "d", Foreground: true})
	if a.Window.hasPosted() {
		t.Fatal("a click was handed on before the windows opened")
	}
	a.notificationsReady()
	if !a.Window.hasPosted() {
		t.Fatal("the app was not brought forward once its windows opened")
	}
	a.Window.runPosted()
	if !a.Window.Visible() {
		t.Fatal("a click on a notification left the app's window hidden")
	}

	var got []NotificationResponse
	a.OnNotification(func(r NotificationResponse) { got = append(got, r) })
	a.Window.Hide()
	a.Window.runPosted()
	if !reflect.DeepEqual(got, []NotificationResponse{{ID: "x", Data: "d"}}) {
		t.Fatalf("the handler got %+v", got)
	}
	if a.Window.Visible() {
		t.Fatal("the click brought the app forward a second time, for the handler")
	}

	// A button not marked Foreground leaves the app where it is.
	a.notificationClicked(notify.Response{ID: "y", Action: "read"})
	a.Window.runPosted()
	if len(got) != 2 || got[1].Action != "read" || a.Window.Visible() {
		t.Fatalf("background action: got %+v, visible %v", got, a.Window.Visible())
	}
}

func TestPendingClicksAreBounded(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return sized(0, 0) })
	defer a.Close()
	for i := range maxPendingClicks + 5 {
		a.notificationClicked(notify.Response{ID: string(rune('a' + i))})
	}
	if n := len(a.notes.pending); n != maxPendingClicks {
		t.Fatalf("%d clicks wait", n)
	}
	if a.notes.pending[0].r.ID != string(rune('a'+5)) {
		t.Fatalf("the oldest clicks were not the ones dropped: %q", a.notes.pending[0].r.ID)
	}
}
