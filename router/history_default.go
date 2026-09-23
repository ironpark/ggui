//go:build !(js && wasm)

package router

func defaultHistory() History { return Memory("/") }
