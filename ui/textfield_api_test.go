package ui_test

import (
	"reflect"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// TextField wraps the editor and forwards its fluent setters one by one.
// This keeps the two from drifting: every chainable method on the editor
// must have a namesake on the field with the same parameters.
func TestTextFieldForwardsEditorSetters(t *testing.T) {
	editor := reflect.TypeFor[*ggui.TextInputWidget]()
	field := reflect.TypeFor[*ui.TextFieldWidget]()
	for i := range editor.NumMethod() {
		m := editor.Method(i)
		if m.Type.NumOut() != 1 || m.Type.Out(0) != editor {
			continue // not a fluent setter
		}
		fm, ok := field.MethodByName(m.Name)
		if !ok {
			t.Errorf("TextInputWidget.%s has no TextFieldWidget counterpart", m.Name)
			continue
		}
		if fm.Type.NumIn() != m.Type.NumIn() || fm.Type.Out(0) != field {
			t.Errorf("TextFieldWidget.%s: got %v, want the editor's parameters and *TextFieldWidget", m.Name, fm.Type)
			continue
		}
		for j := 1; j < m.Type.NumIn(); j++ {
			if fm.Type.In(j) != m.Type.In(j) {
				t.Errorf("TextFieldWidget.%s parameter %d: got %v, want %v", m.Name, j, fm.Type.In(j), m.Type.In(j))
			}
		}
	}
}

// The field's Key reaches the editor, whose regions carry the identity.
func TestTextFieldKeyReachesEditor(t *testing.T) {
	f := ui.TextField(ggui.State("")).Key("k")
	if f.Input().HitID() != "k" {
		t.Fatalf("editor id = %v", f.Input().HitID())
	}
}
