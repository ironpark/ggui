//go:build !ggui_debug

package ggui

// storeChecks is what a ggui_debug build checks a Store with; nothing here.
//
//lint:ignore U1000 only a ggui_debug build checks a store
type storeChecks struct{}

func watchStore[M any](*Store[M]) {}

func watchSelect[M, T any](*Store[M], *DerivedValue[T], func(M) T) {}

func checkStores() {}
