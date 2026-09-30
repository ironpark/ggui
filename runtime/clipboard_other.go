//go:build (!darwin && !windows) || ios

package runtime

func nativeClipboard() Clipboard { return &execClipboard{} }
