package chat_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/chat"
	"github.com/ironpark/ggui/widget"
)

func TestTraceNestedDisclosuresAndAccessibleActions(t *testing.T) {
	t.Parallel()
	trace := chat.Trace(chat.TraceStep{ID: "run", Title: "Run", Status: chat.TraceRunning, Duration: 1500 * time.Millisecond,
		Details: widget.Text("Run details"), Children: []chat.TraceStep{
			{ID: "fetch", Title: "Fetch", Status: chat.TraceCompleted, Details: widget.Text("Fetched two files")},
			{ID: "save", Title: "Save", Status: chat.TracePending},
		}})
	p := ggui.NewProbe(widget.Column(trace), ggui.Sz(400, 400))
	defer p.Close()
	node(t, p.Semantics(), ggui.RoleList, "Execution trace")
	if n := node(t, p.Semantics(), ggui.RoleListItem, "Run"); n.Description != "running · 1.5s" {
		t.Fatal(n)
	}
	if _, ok := p.Semantics().Find(ggui.RoleListItem, "Fetch"); ok {
		t.Fatal("collapsed descendants are exposed")
	}
	run := node(t, p.Semantics(), ggui.RoleDisclosure, "Run")
	if run.Expanded != ggui.ExpandClosed {
		t.Fatal(run)
	}
	p.Perform(run.ID, ggui.Action{Kind: ggui.ActionExpand})
	p.Advance(300 * time.Millisecond)
	node(t, p.Semantics(), ggui.RoleText, "Run details")
	child := node(t, p.Semantics(), ggui.RoleDisclosure, "Fetch")
	p.Perform(child.ID, ggui.Action{Kind: ggui.ActionExpand})
	p.Advance(300 * time.Millisecond)
	node(t, p.Semantics(), ggui.RoleText, "Fetched two files")
	if _, ok := p.Semantics().Find(ggui.RoleDisclosure, "Save"); ok {
		t.Fatal("leaf without details is an empty disclosure")
	}
	p.Perform(run.ID, ggui.Action{Kind: ggui.ActionCollapse})
	// Closing content becomes inert immediately, including during its animation.
	if _, ok := p.Semantics().Find(ggui.RoleText, "Fetched two files"); ok {
		t.Fatal("closing details still exposed")
	}
	p.Advance(300 * time.Millisecond)
	p.Perform(node(t, p.Semantics(), ggui.RoleDisclosure, "Run").ID, ggui.Action{Kind: ggui.ActionExpand})
	p.Advance(300 * time.Millisecond)
	if node(t, p.Semantics(), ggui.RoleDisclosure, "Fetch").Expanded != ggui.ExpandOpen {
		t.Fatal("nested expansion was lost")
	}
}

func TestTraceKeyboardDetailsAndImmutableExpansionBinding(t *testing.T) {
	t.Parallel()
	initial := map[string]bool{"other": true}
	expanded := ggui.State(initial)
	var toggles []string
	clicked := 0
	trace := chat.Trace(
		chat.TraceStep{ID: "a", Title: "First", Details: ui.Button("Inspect", func() { clicked++ })},
		chat.TraceStep{ID: "b", Title: "Second", Details: widget.Text("Second details")},
	).BindExpanded(expanded).OnToggle(func(id string, open bool) { toggles = append(toggles, id) })
	p := ggui.NewProbe(widget.Column(trace), ggui.Sz(350, 300))
	defer p.Close()
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeyEnter)
	p.Advance(300 * time.Millisecond)
	if !ggui.Peek(expanded)["a"] || initial["a"] || !ggui.Peek(expanded)["other"] {
		t.Fatal("expansion mutated old map or dropped another key")
	}
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeySpace)
	if clicked != 1 {
		t.Fatal("expanded detail action not reachable by Tab")
	}
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeySpace)
	p.Advance(300 * time.Millisecond)
	if !ggui.Peek(expanded)["b"] || len(toggles) != 2 || toggles[0] != "a" || toggles[1] != "b" {
		t.Fatal("headers not independently focusable", toggles)
	}
	expanded.Set(map[string]bool{})
	p.Frame()
	if len(toggles) != 2 {
		t.Fatal("external updates triggered user callback")
	}
}

func TestTraceUpdatesKeepKeyedFocusAndExpansion(t *testing.T) {
	t.Parallel()
	steps := ggui.State([]chat.TraceStep{
		{ID: "a", Title: "Compile", Status: chat.TraceRunning, Details: widget.Text("Compiling")},
		{ID: "b", Title: "Test", Status: chat.TracePending, Details: widget.Text("Waiting")},
	})
	trace := chat.Trace().BindSteps(steps)
	p := ggui.NewProbe(widget.Column(trace), ggui.Sz(350, 300))
	defer p.Close()
	p.Tap("Compile")
	p.Advance(300 * time.Millisecond)
	before := node(t, p.Semantics(), ggui.RoleDisclosure, "Compile")
	steps.Set([]chat.TraceStep{
		{ID: "b", Title: "Test", Status: chat.TraceRunning, Details: widget.Text("Running tests")},
		{ID: "a", Title: "Compiled", Status: chat.TraceCompleted, Duration: 42 * time.Millisecond, Details: widget.Text("Finished")},
	})
	p.Frame()
	after := node(t, p.Semantics(), ggui.RoleDisclosure, "Compiled")
	if before.ID.ID != after.ID.ID || after.Expanded != ggui.ExpandOpen {
		t.Fatal("keyed disclosure identity or state lost")
	}
	if focused, ok := p.Semantics().Focused(); !ok || focused.Name != "Compiled" {
		t.Fatal("focus lost on reorder", focused)
	}
	node(t, p.Semantics(), ggui.RoleText, "Finished")
	if n := node(t, p.Semantics(), ggui.RoleListItem, "Compiled"); n.Description != "completed · 42ms" {
		t.Fatal("stale status or duration", n)
	}
	trace.Steps([]chat.TraceStep{{ID: "c", Title: "Skipped", Status: chat.TraceSkipped}})
	steps.Set(nil)
	p.Frame()
	node(t, p.Semantics(), ggui.RoleListItem, "Skipped")
	if _, ok := p.Semantics().Find(ggui.RoleListItem, "Compiled"); ok {
		t.Fatal("removed step still present")
	}
}

func TestTraceOwnsLiteralSlicesAndValidatesIDs(t *testing.T) {
	t.Parallel()
	steps := []chat.TraceStep{{ID: "root", Title: "Root", Children: []chat.TraceStep{{ID: "child", Title: "Child"}}}}
	trace := chat.Trace(steps...).Expanded(map[string]bool{"root": true})
	steps[0].Children[0].Title = "Mutated"
	p := ggui.NewProbe(widget.Column(trace), ggui.Sz(280, 200))
	node(t, p.Semantics(), ggui.RoleListItem, "Child")
	p.Close()
	for _, invalid := range [][]chat.TraceStep{
		{{Title: "Missing ID"}},
		{{ID: "same", Children: []chat.TraceStep{{ID: "same"}}}},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("invalid IDs were accepted")
				}
			}()
			chat.Trace(invalid...).Layout(ggui.Loose(ggui.Sz(300, 200)), ggui.Env{})
		}()
	}
}

func TestTraceEmptyAndNarrowLayout(t *testing.T) {
	t.Parallel()
	empty := chat.Trace().Name("Build trace")
	p := ggui.NewProbe(empty, ggui.Sz(200, 100))
	if n := node(t, p.Semantics(), ggui.RoleList, "Build trace"); n.Max != 0 {
		t.Fatal(n)
	}
	p.Close()
	trace := chat.Trace(chat.TraceStep{ID: "step", Title: "A long execution step title", Status: chat.TraceFailed, Duration: time.Second,
		Details: widget.Text("Long details that should wrap in a narrow view")}).Expanded(map[string]bool{"step": true})
	for _, size := range []ggui.Size{ggui.Sz(0, 0), ggui.Sz(50, 50), ggui.Sz(200, 300)} {
		got := trace.Layout(ggui.Loose(size), ggui.Env{})
		if got.W < 0 || got.H < 0 || got.W > size.W || got.H > size.H {
			t.Fatalf("size %v escaped %v", got, size)
		}
	}
}

func TestTraceDescriptionsAndActionRows(t *testing.T) {
	t.Parallel()
	activated := 0
	trace := chat.Trace(
		chat.TraceStep{ID: "plain", Title: "Complete", Description: "Two files written"},
		chat.TraceStep{ID: "error", Title: "Failed build", Description: "Open diagnostics", Status: chat.TraceError, OnActivate: func() { activated++ }},
	)
	p := ggui.NewProbe(widget.Column(trace), ggui.Sz(300, 200))
	defer p.Close()
	if n := node(t, p.Semantics(), ggui.RoleListItem, "Complete"); n.Description != "Two files written" {
		t.Fatal(n)
	}
	if n := node(t, p.Semantics(), ggui.RoleListItem, "Failed build"); n.Description != "error · Open diagnostics" {
		t.Fatal(n)
	}
	if _, ok := p.Semantics().Find(ggui.RoleButton, "Complete"); ok {
		t.Fatal("plain row became interactive")
	}
	p.Tap("Failed build")
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if activated != 2 {
		t.Fatal("action row did not activate by pointer and keyboard", activated)
	}
}
