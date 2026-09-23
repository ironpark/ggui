package main

import (
	"log"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/fonts/remote"
)

func setupEmojiFont() {
	font := ggui.Resource(func() string {
		return "assets/NotoColorEmoji.ttf?v=15671215ab76"
	}, remote.Load)
	ggui.Effect(func() ggui.Cleanup {
		state := font.Get()
		switch state.Status {
		case ggui.Ready:
			ggui.SetEmojiFont(state.Value)
		case ggui.Failed:
			log.Printf("load emoji font: %v", state.Err)
		}
		return nil
	})
}
