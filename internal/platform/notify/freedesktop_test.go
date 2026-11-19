package notify

import (
	"reflect"
	"testing"
)

func TestNotifyArgs(t *testing.T) {
	n := Notification{
		ID: "x", Title: "Build <done>", Subtitle: "ggui", Body: "a & b",
		Actions: []Action{{ID: "default", Title: "Not the default"}, {ID: "open", Title: "Open log"}},
	}
	args := notifyArgs(n, fdOptions{appName: "Editor", appID: "com.example.editor", icon: "com.example.editor", replaces: 7, markup: true})
	got := roundTrip(t, fdNotify, args)
	want := []any{
		"Editor", uint32(7), "com.example.editor", "Build <done>", "ggui\na &amp; b",
		[]any{"default", "Open", "ggui.default", "Not the default", "ggui.open", "Open log"},
		[]any{
			dictEntry{"desktop-entry", variant{"s", "com.example.editor"}},
			dictEntry{"suppress-sound", variant{"b", true}},
		},
		int32(-1),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Notify arguments\n%#v\nwant\n%#v", got, want)
	}

	plain := roundTrip(t, fdNotify, notifyArgs(Notification{Title: "T", Body: "a & b", Sound: true}, fdOptions{appName: "app"}))
	if plain[4] != "a & b" {
		t.Errorf("the body was escaped for a server without markup: %q", plain[4])
	}
	if hints := plain[6].([]any); !reflect.DeepEqual(hints, []any{dictEntry{"sound-name", variant{"s", "message-new-instant"}}}) {
		t.Errorf("hints without an id, with sound: %#v", hints)
	}
}

func TestFdAction(t *testing.T) {
	for key, want := range map[string]struct {
		id string
		ok bool
	}{
		"default":      {"", true},
		"ggui.default": {"default", true},
		"ggui.read":    {"read", true},
		"read":         {"", false},
	} {
		id, ok := fdAction(key)
		if id != want.id || ok != want.ok {
			t.Errorf("fdAction(%q) = %q, %v; want %q, %v", key, id, ok, want.id, want.ok)
		}
	}
}

func TestSessionBusPath(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	exists := func(p string) bool { return p == "/run/user/1000/bus" }
	for _, c := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"DBUS_SESSION_BUS_ADDRESS": "unix:path=/run/user/1000/bus"}, "/run/user/1000/bus"},
		{map[string]string{"DBUS_SESSION_BUS_ADDRESS": "unix:abstract=/tmp/dbus-x,guid=1234"}, "@/tmp/dbus-x"},
		{map[string]string{"DBUS_SESSION_BUS_ADDRESS": "tcp:host=h,port=1;unix:guid=1,path=/tmp/a%20b"}, "/tmp/a b"},
		{map[string]string{"DBUS_SESSION_BUS_ADDRESS": "autolaunch:", "XDG_RUNTIME_DIR": "/run/user/1000"}, ""},
		{map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000"}, "/run/user/1000/bus"},
		{map[string]string{"XDG_RUNTIME_DIR": "/run/user/1001"}, ""},
		{map[string]string{}, ""},
	} {
		if got := sessionBusPath(env(c.env), exists); got != c.want {
			t.Errorf("sessionBusPath(%v) = %q, want %q", c.env, got, c.want)
		}
	}
}
