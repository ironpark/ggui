package ggui

import "syscall/js"

// systemLanguages is the browser's navigator.languages, or its one
// navigator.language where the list is missing.
func systemLanguages() []string {
	nav := js.Global().Get("navigator")
	if !nav.Truthy() {
		return nil
	}
	var out []string
	if list := nav.Get("languages"); list.Truthy() {
		for i := range list.Length() {
			if v := list.Index(i); v.Type() == js.TypeString {
				out = append(out, v.String())
			}
		}
	}
	if len(out) == 0 {
		if v := nav.Get("language"); v.Type() == js.TypeString {
			out = append(out, v.String())
		}
	}
	return out
}
