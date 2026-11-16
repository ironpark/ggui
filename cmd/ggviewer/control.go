//go:build unix && !ios && !android

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/ironpark/ggwd/remote"
)

// The control channel is newline-delimited JSON: commands on the
// viewer's stdin, events on its stdout. The package doc describes it.

// Commands the launcher sends.
const (
	cmdBuilding    = "building"
	cmdBuildFailed = "build-failed"
	cmdAppExited   = "app-exited"
	cmdClear       = "clear"
	cmdQuit        = "quit"

	// Accepted with -debug only.
	cmdDebugDump   = "debug-dump"
	cmdDebugWindow = "debug-window"
	cmdDebugInput  = "debug-input"
)

// Events the viewer sends.
const (
	evReady        = "ready"
	evAppAttached  = "app-attached"
	evFirstFrame   = "first-frame"
	evAppDetached  = "app-detached"
	evWindowClosed = "window-closed"
	evDumped       = "dumped"
)

// command is one line of the control channel.
type command struct {
	Type string `json:"type"`
	// Output is the compiler's output of a build-failed, or what the app
	// printed before an app-exited.
	Output string `json:"output,omitempty"`
	// Status is how the app ended, as the launcher puts it, such as
	// "exit status 2" or "signal: killed".
	Status string `json:"status,omitempty"`

	// Path is where debug-dump writes the window's picture, a PNG.
	Path string `json:"path,omitempty"`
	// Action is what debug-window does: hide, show, minimize, restore, focus or
	// resize the client area to Width by Height.
	Action string  `json:"action,omitempty"`
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
	// Input is the input debug-input sends the app as if the window got it.
	// From the first one on, the window's own input is not forwarded.
	Input *remote.Input `json:"input,omitempty"`
}

var windowActions = []string{"hide", "show", "minimize", "restore", "focus", "close", "resize"}

// parseCommand parses one line of the control channel. debug allows the
// debug commands.
func parseCommand(line []byte, debug bool) (command, error) {
	var c command
	if err := json.Unmarshal(line, &c); err != nil {
		return command{}, fmt.Errorf("a control line that is not a JSON object: %w", err)
	}
	switch c.Type {
	case cmdBuilding, cmdBuildFailed, cmdAppExited, cmdClear, cmdQuit:
		return c, nil
	case cmdDebugDump, cmdDebugWindow, cmdDebugInput:
		if !debug {
			return command{}, fmt.Errorf("%s needs -debug", c.Type)
		}
	case "":
		return command{}, errors.New("a control line with no type")
	default:
		return command{}, fmt.Errorf("an unknown control command %q", c.Type)
	}
	switch c.Type {
	case cmdDebugDump:
		if c.Path == "" {
			return command{}, errors.New("debug-dump with no path")
		}
	case cmdDebugWindow:
		known := false
		for _, a := range windowActions {
			known = known || a == c.Action
		}
		if !known {
			return command{}, fmt.Errorf("debug-window: an action of %q, not one of %s", c.Action, strings.Join(windowActions, ", "))
		}
		if c.Action == "resize" && (c.Width < 1 || c.Height < 1) {
			return command{}, errors.New("debug-window: resize to no size")
		}
	case cmdDebugInput:
		if c.Input == nil {
			return command{}, errors.New("debug-input with no input")
		}
	}
	return c, nil
}

// event is one line the viewer writes.
type event struct {
	Type   string `json:"type"`
	Socket string `json:"socket,omitempty"`
	PID    int    `json:"pid,omitempty"`
	Path   string `json:"path,omitempty"`
}

// events writes events to the launcher, one JSON object a line. Writes
// from several goroutines do not interleave; a launcher that is gone is
// not an error.
type events struct {
	mu sync.Mutex
	w  io.Writer
}

func (e *events) send(ev event) {
	b, err := json.Marshal(ev)
	if err != nil {
		panic(err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, _ = e.w.Write(append(b, '\n'))
}
