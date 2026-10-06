//go:build !ggui_debug

package ggui

func watchStore[M any](*Store[M]) {}

func checkStores() {}
