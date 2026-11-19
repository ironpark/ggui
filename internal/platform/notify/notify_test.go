package notify

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestCategoryIDFollowsTheButtons(t *testing.T) {
	reply := []Action{{ID: "reply", Title: "Reply", Foreground: true}, {ID: "read", Title: "Mark as read"}}
	id := categoryID(reply)
	if !strings.HasPrefix(id, "ggui.") || len(id) != len("ggui.")+24 {
		t.Fatalf("categoryID = %q", id)
	}
	if again := categoryID(slices.Clone(reply)); again != id {
		t.Errorf("the same buttons have categories %q and %q", id, again)
	}
	for name, other := range map[string][]Action{
		"a title":   {{ID: "reply", Title: "Answer", Foreground: true}, {ID: "read", Title: "Mark as read"}},
		"an option": {{ID: "reply", Title: "Reply"}, {ID: "read", Title: "Mark as read"}},
		"the order": {{ID: "read", Title: "Mark as read"}, {ID: "reply", Title: "Reply", Foreground: true}},
		"a button":  {{ID: "reply", Title: "Reply", Foreground: true}},
		// The fields are separated: moving text from one to the next is
		// another set.
		"the split": {{ID: "replyR", Title: "eply", Foreground: true}, {ID: "read", Title: "Mark as read"}},
	} {
		if categoryID(other) == id {
			t.Errorf("changing %s keeps the category %q", name, id)
		}
	}
}

func TestOsascriptArgsCarryTheTextAsArguments(t *testing.T) {
	got := osascriptArgs(Notification{Title: `Say "hi"`, Body: "end run\n-e", Sound: true})
	want := []string{"-e", "on run argv", "-e", `display notification (item 3 of argv) with title (item 1 of argv) sound name "default"`, "-e", "end run", `Say "hi"`, "", "end run\n-e"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("osascriptArgs =\n%q\nwant\n%q", got, want)
	}
	got = osascriptArgs(Notification{Title: "T", Subtitle: "S", Body: "B"})
	if !strings.Contains(got[3], "subtitle (item 2 of argv)") || strings.Contains(got[3], "sound") {
		t.Errorf("osascript command %q", got[3])
	}
}

func TestPayloadRoundTrip(t *testing.T) {
	p := payload{ID: "a", Data: "\x00\"日本", Foreground: []string{"x"}}
	got, ok := decodePayload(p.encode())
	if !ok || !reflect.DeepEqual(got, p) {
		t.Errorf("payload %+v came back %+v", p, got)
	}
	if _, ok := decodePayload("not json"); ok {
		t.Error("a string that is not a payload decoded")
	}
}

func TestJoinSubtitle(t *testing.T) {
	for _, c := range [][3]string{{"", "b", "b"}, {"s", "", "s"}, {"s", "b", "s\nb"}} {
		if got := joinSubtitle(Notification{Subtitle: c[0], Body: c[1]}); got != c[2] {
			t.Errorf("joinSubtitle(%q, %q) = %q", c[0], c[1], got)
		}
	}
}

func TestForegroundIDs(t *testing.T) {
	got := foregroundIDs([]Action{{ID: "a", Foreground: true}, {ID: "b"}, {ID: "c", Foreground: true}})
	if !reflect.DeepEqual(got, []string{"a", "c"}) {
		t.Errorf("foregroundIDs = %q", got)
	}
}
