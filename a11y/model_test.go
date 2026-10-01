package a11y

import "testing"

func TestTriAndExpandableEncodeTheirStates(t *testing.T) {
	t.Parallel()
	if Tri(true) != TriOn || Tri(false) != TriOff {
		t.Errorf("Tri(true), Tri(false) = %v, %v; want TriOn, TriOff", Tri(true), Tri(false))
	}
	open, closed := Expandable(true), Expandable(false)
	if open == nil || !*open || closed == nil || *closed {
		t.Error("Expandable should point at the value it was given")
	}
	if Expandable(true) == open {
		t.Error("Expandable should return a fresh pointer each call, so nodes never share one")
	}
}

func TestActionSetHasRequiresEveryBit(t *testing.T) {
	t.Parallel()
	s := ActionPress | ActionFocus
	cases := []struct {
		b    ActionSet
		want bool
	}{
		{ActionPress, true},
		{ActionFocus, true},
		{ActionPress | ActionFocus, true},
		{ActionPress | ActionExpand, false},
		{ActionSetSelection, false},
		{0, true}, // the empty set is in every set
	}
	for _, c := range cases {
		if got := s.Has(c.b); got != c.want {
			t.Errorf("(%b).Has(%b) = %v, want %v", s, c.b, got, c.want)
		}
	}
}

func TestTextRunAtClampsAndSnapsToRuneStarts(t *testing.T) {
	t.Parallel()
	// "aé" laid out from byte 10: a stop at 10, 11 and 13, since é is two bytes.
	r := TextRun{Start: 10, End: 13, Stops: []TextStop{{10, 0}, {11, 7}, {13, 15}}}
	cases := []struct {
		b    int
		want float64
	}{
		{0, 0},   // before the run: its start
		{10, 0},  // on a stop
		{11, 7},  // on a stop
		{12, 7},  // inside é: where é starts
		{13, 15}, // the end
		{99, 15}, // past the run: its end
	}
	for _, c := range cases {
		if got := r.At(c.b); got != c.want {
			t.Errorf("At(%d) = %g, want %g", c.b, got, c.want)
		}
	}
	if got := (TextRun{}).At(5); got != 0 {
		t.Errorf("a run with no stops placed byte 5 at %g, want 0", got)
	}
}

func TestKeyRejectsUncomparableHandlers(t *testing.T) {
	t.Parallel()
	type handle struct{ id int }
	p := &handle{1}
	cases := []struct {
		name string
		h    any
		ok   bool
	}{
		{"nil", nil, false},
		{"slice", []int{1}, false},
		{"map", map[string]int{}, false},
		{"func", func() {}, false},
		{"struct holding a slice", struct{ s []int }{}, false},
		{"string", "id", true},
		{"pointer", p, true},
		{"comparable struct", handle{2}, true},
	}
	for _, c := range cases {
		got := Key(c.h)
		if (got != nil) != c.ok {
			t.Errorf("Key(%s) = %v, want a key: %v", c.name, got, c.ok)
			continue
		}
		if c.ok {
			// The point of a key is that a map lookup with it cannot panic.
			m := map[any]bool{got: true}
			if !m[Key(c.h)] {
				t.Errorf("Key(%s) is not stable across calls", c.name)
			}
		}
	}
}

func TestControlSeparatesInputRolesFromContent(t *testing.T) {
	t.Parallel()
	controls := []Role{RoleButton, RoleCheckbox, RoleRadio, RoleSwitch, RoleSlider, RoleTextField,
		RoleSelect, RoleOption, RoleMenu, RoleMenuItem, RoleTab, RoleDisclosure, RoleCombobox, RoleLink}
	content := []Role{RoleTabs, RoleDialog, RoleRow, RoleAccordion, RoleSeparator, RoleText, RoleHeading,
		RoleImage, RoleList, RoleListItem, RoleGroup, RoleProgress, RoleToolbar, RoleStatus, RoleWindow, ""}
	for _, r := range controls {
		if !Control(r) {
			t.Errorf("Control(%q) = false, want true: the user acts on it", r)
		}
	}
	for _, r := range content {
		if Control(r) {
			t.Errorf("Control(%q) = true, want false: it is content or structure", r)
		}
	}
}

func TestGeometryConstructorsForwardToGeom(t *testing.T) {
	t.Parallel()
	r := Rct(Pt(1, 2), Sz(3.5, 4))
	if r.Origin.X != 1 || r.Origin.Y != 2 || r.Size.W != 3.5 || r.Size.H != 4 {
		t.Errorf("Rct(Pt(1, 2), Sz(3.5, 4)) = %+v", r)
	}
}
