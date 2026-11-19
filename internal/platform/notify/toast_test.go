package notify

import (
	"encoding/xml"
	"reflect"
	"strings"
	"testing"
)

// toast is the shape of the XML toastXML writes.
type toast struct {
	Launch     string `xml:"launch,attr"`
	Activation string `xml:"activationType,attr"`
	Binding    struct {
		Template string   `xml:"template,attr"`
		Texts    []string `xml:"text"`
	} `xml:"visual>binding"`
	Actions []struct {
		Content    string `xml:"content,attr"`
		Arguments  string `xml:"arguments,attr"`
		Activation string `xml:"activationType,attr"`
	} `xml:"actions>action"`
	Audio *struct {
		Silent string `xml:"silent,attr"`
	} `xml:"audio"`
}

func parseToast(t *testing.T, s string) toast {
	t.Helper()
	var x toast
	if err := xml.Unmarshal([]byte(s), &x); err != nil {
		t.Fatalf("toastXML is not XML: %v\n%s", err, s)
	}
	return x
}

func TestToastXMLEscapesAndCarriesThePayload(t *testing.T) {
	n := Notification{
		ID: "chat-42", Title: `<b>Ada & "Bob"</b>`, Subtitle: "it's", Body: "line one\nline two \x01",
		Data: `{"chat":42}`,
		Actions: []Action{
			{ID: "reply", Title: "Reply <now>", Foreground: true},
			{ID: "read", Title: "Mark & read"},
		},
	}
	s := toastXML(n)
	x := parseToast(t, s)
	if x.Binding.Template != "ToastGeneric" || x.Activation != "foreground" {
		t.Errorf("toast %+v", x)
	}
	want := []string{`<b>Ada & "Bob"</b>`, "it's", "line one\nline two �"}
	if !reflect.DeepEqual(x.Binding.Texts, want) {
		t.Errorf("texts %q, want %q", x.Binding.Texts, want)
	}
	p, ok := decodePayload(x.Launch)
	if !ok || p.ID != "chat-42" || p.Data != `{"chat":42}` || p.Action != "" || len(p.Foreground) == 0 {
		t.Errorf("launch payload %+v from %q", p, x.Launch)
	}
	if len(x.Actions) != 2 {
		t.Fatalf("actions %+v", x.Actions)
	}
	for i, a := range x.Actions {
		if a.Content != n.Actions[i].Title || a.Activation != "foreground" {
			t.Errorf("action %d: %+v", i, a)
		}
		p, ok := decodePayload(a.Arguments)
		if !ok || p.ID != n.ID || p.Data != n.Data || p.Action != n.Actions[i].ID || (len(p.Foreground) > 0) != n.Actions[i].Foreground {
			t.Errorf("action %d payload %+v", i, p)
		}
	}
	if x.Audio == nil || x.Audio.Silent != "true" {
		t.Errorf("a notification without Sound is not silent: %s", s)
	}
}

func TestToastXMLLimits(t *testing.T) {
	var actions []Action
	for i := range 7 {
		actions = append(actions, Action{ID: string(rune('a' + i)), Title: "x"})
	}
	x := parseToast(t, toastXML(Notification{Body: "only a body", Sound: true, Actions: actions}))
	if len(x.Actions) != maxToastActions {
		t.Errorf("%d buttons, want %d", len(x.Actions), maxToastActions)
	}
	if !reflect.DeepEqual(x.Binding.Texts, []string{"only a body"}) {
		t.Errorf("texts %q", x.Binding.Texts)
	}
	if x.Audio != nil {
		t.Errorf("a notification with Sound is silenced")
	}
}

func TestToastTagFitsWindows(t *testing.T) {
	if got := toastTag("chat-42"); got != "chat-42" {
		t.Errorf("toastTag kept a short id as %q", got)
	}
	long := strings.Repeat("é", 65)
	a, b := toastTag(long), toastTag(long+"x")
	if len([]rune(a)) > 64 || a == b || a != toastTag(long) {
		t.Errorf("toastTag of long ids: %q %q", a, b)
	}
	if got := toastTag(strings.Repeat("é", 64)); got != strings.Repeat("é", 64) {
		t.Errorf("64 characters were replaced: %q", got)
	}
}

func TestAUMID(t *testing.T) {
	for id, want := range map[string]string{
		"com.example.editor": "com.example.editor",
		"My Editor":          "My.Editor",
		"tab\tand\nline":     "tab.and.line",
		"":                   "ggui",
	} {
		if got := aumid(id); got != want {
			t.Errorf("aumid(%q) = %q, want %q", id, got, want)
		}
	}
	if got := aumid(strings.Repeat("a", 200)); len(got) != 128 {
		t.Errorf("aumid kept %d characters", len(got))
	}
}
