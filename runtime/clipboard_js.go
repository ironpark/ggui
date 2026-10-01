package runtime

// nativeClipboard is an in-process copy: the browser hands out its clipboard
// only asynchronously and to a page the user has interacted with.
func nativeClipboard() Clipboard { return &MemoryClipboard{} }
