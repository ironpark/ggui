//go:build !js

package main

import "github.com/ironpark/ggui/fonts/notoemoji"

func setupEmojiFont() { notoemoji.Enable() }
