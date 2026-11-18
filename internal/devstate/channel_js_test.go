//go:build js && wasm

package devstate

import (
	"encoding/json"
	"syscall/js"
	"testing"
)

// ggui dev's page hands the state over in gguiDev.state and takes it back
// from gguiDev.save before it reloads.
func TestPage(t *testing.T) {
	js.Global().Set("gguiDev", js.ValueOf(map[string]any{"state": `{"n":41,"other":"x"}`}))
	defer js.Global().Delete("gguiDev")
	s := connect()
	if s == nil {
		t.Fatal("no session on ggui dev's page")
	}
	s.mu.Lock()
	delete(s.handed, "n")
	s.entries["n"] = &entry{save: func() ([]byte, error) { return []byte("42"), nil }}
	s.mu.Unlock()
	saved := js.Global().Get("gguiDev").Call("save").String()
	var st State
	if err := json.Unmarshal([]byte(saved), &st); err != nil {
		t.Fatal(err)
	}
	if string(st["n"]) != "42" || string(st["other"]) != `"x"` {
		t.Errorf("save() = %s, want n=42 and the unclaimed other", saved)
	}
}

// A page that is not ggui dev's has no gguiDev, and there is no session.
func TestNoPage(t *testing.T) {
	if s := connect(); s != nil {
		t.Error("a session without ggui dev's page")
	}
}
