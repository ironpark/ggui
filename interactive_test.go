package ggui

import "testing"

// A direct write to Inert, bypassing SetInert, still asks for a layout
// once Sync sees it.
func TestInteractiveDirectInertWriteRequestsLayout(t *testing.T) {
	var s Interactive
	s.Sync()
	before := layoutGen.Load()
	s.SetInert(true)
	s.Sync()
	if layoutGen.Load() == before {
		t.Fatal("direct Inert write not noticed by Sync")
	}
	before = layoutGen.Load()
	s.Sync()
	if layoutGen.Load() != before {
		t.Fatal("Sync requested a layout with nothing changed")
	}
}

func TestConfigInspectorChord(t *testing.T) {
	a := New(Config{Inspector: "f1"}, func() Widget { return Box() })
	if a.inspectChord.Key != KeyF1 {
		t.Fatalf("chord key = %v", a.inspectChord.Key)
	}
	if New(Config{}, func() Widget { return Box() }).inspectChord.Key != 0 {
		t.Fatal("empty Inspector parsed a chord")
	}
}
