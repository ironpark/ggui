//go:build js

package runtime

import "errors"

// Launch is what a second launch of an app hands the instance already
// running; see ClaimInstance.
type Launch struct {
	Args []string
	Dir  string
}

// ErrRunning is returned by ClaimInstance when another process already
// runs the app.
var ErrRunning = errors.New("runtime: the app is already running")

// ClaimInstance always succeeds in a browser, where every tab is an
// instance of its own.
func ClaimInstance(string, func(Launch)) (func(), error) { return func() {}, nil }
