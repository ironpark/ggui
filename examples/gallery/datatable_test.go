package main

import (
	"github.com/ironpark/ggui"
	"testing"
)

func TestPaymentTableFiltersAndRestores(t *testing.T) {
	p := ggui.ProbeBuilder(newPaymentTable, ggui.Sz(640, 600))
	defer p.Close()
	set := func(value string) {
		t.Helper()
		n, ok := p.Semantics().Find(ggui.RoleTextField, "Filter rows")
		if !ok {
			t.Fatal("missing filter")
		}
		p.Perform(n.ID, ggui.Action{Kind: ggui.ActionSetValue, Text: value})
		p.Frame()
	}
	set("missing")
	if n := len(p.FindAll(ggui.RoleRow)); n != 0 {
		t.Fatalf("empty %d", n)
	}
	set("example")
	if n := len(p.FindAll(ggui.RoleRow)); n != 5 {
		t.Fatalf("restored %d", n)
	}
}
