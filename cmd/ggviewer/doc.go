// Command ggviewer shows the window of a ggui app that runs in another
// process, so that the app can be rebuilt and restarted while its window
// stays: the dev mode of the ggui command. It is built from the module of
// the app it shows, so that it speaks the same protocol:
//
//	go build github.com/ironpark/ggui/cmd/ggviewer
//
// The viewer listens on a unix socket. An app built with -tags ggwd_remote
// and started with GGWD_REMOTE_SOCKET set to that socket opens no window
// and no GPU device of its own: it draws through a remote driver, whose
// batches the viewer replays into its window, and gets its input from the
// viewer's window (see github.com/ironpark/ggwd/remote). When the app ends,
// the viewer keeps its last frame; the next app that connects takes the
// window over, and its first frame replaces the last one. The window opens
// with the first app's options and takes the title, size limits,
// resizability, decoration and floating of every app after it; a new size
// only when an app asks for another than the one before. The title bar's
// hiding and transparency are the first app's. With -project, the window's
// position and size are kept for the project under the user's cache
// directory, and a later session opens the window there.
//
// Usage:
//
//	ggviewer -socket path [-project key] [-debug]
//
// # Control channel
//
// The launcher controls the viewer through its stdin and hears from it on
// its stdout, a JSON object a line each way. The viewer exits when its
// stdin closes, so it does not outlive a launcher that dies. Logs go to
// stderr.
//
// Commands, on stdin:
//
//	{"type":"building"}
//		A build runs: the window notes it in a corner.
//	{"type":"build-failed","output":"..."}
//		The build failed: the window shows the compiler's output over the
//		lower part of the app's last frame, scrolled with the wheel.
//	{"type":"app-exited","status":"exit status 2","output":"..."}
//		The app ended on its own: the window says how, with what it printed.
//	{"type":"clear"}
//		Removes what the window shows of these.
//	{"type":"quit"}
//		Closes the window and ends the viewer.
//
// An app's first frame removes what the window shows of them too. A
// build-failed or app-exited before any app connected opens a window to
// show it in. With -debug, the viewer also takes these, for tests:
//
//	{"type":"debug-dump","path":"/tmp/w.png"}
//		Writes what the window shows to a PNG, then says dumped.
//	{"type":"debug-window","action":"hide"}
//		hide, show, minimize, restore, focus, or resize with "width" and
//		"height".
//	{"type":"debug-input","input":{"kind":2,"x":10,"y":20,"pressed":true}}
//		Sends the app an Input of package ggwd/remote as if the window got
//		it; after the first, the window's own input goes nowhere.
//
// Events, on stdout:
//
//	{"type":"ready","socket":"/abs/path"}       listening; start apps
//	{"type":"app-attached","pid":123}           an app connected
//	{"type":"first-frame","pid":123}            its first frame shows
//	{"type":"app-detached","pid":123}           it went away
//	{"type":"window-closed"}                    the window closed; the viewer ends
//	{"type":"dumped","path":"/tmp/w.png"}       a debug-dump is written
//
// Closing the window asks the app to close, which it may refuse; when it
// closes, or when no app is attached, the window closes and the viewer
// ends. The viewer runs on unix systems only.
package main
