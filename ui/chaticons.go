package ui

import (
	"image/color"
	"sync"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	"github.com/ironpark/ggui/ui/icons/lucide"
)

// Roles the chat widgets draw that ui/icons does not name yet. An icon set
// in the Env or the theme that resolves them is used; otherwise the Lucide
// drawings below are.
const (
	iconCopy      icons.Role = "copy"
	iconArrowDown icons.Role = "arrow-down"
)

const chatIconOpen = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">`

// chatIcons holds Lucide's copy and arrow-down (ISC, as ui/icons/lucide),
// parsed once.
var chatIcons = sync.OnceValue(func() icons.Map {
	set := icons.Map{}
	for role, body := range map[icons.Role]string{
		iconCopy:      `<rect width="14" height="14" x="8" y="8" rx="2" ry="2" /><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" />`,
		iconArrowDown: `<path d="M12 5v14" /><path d="m19 12-7 7-7-7" />`,
	} {
		asset, err := icons.Parse([]byte(chatIconOpen + body + `</svg>`))
		if err != nil {
			panic(err)
		}
		set[role] = asset
	}
	return set
})

// paintChatIcon is paintIcon for a role ui/icons may not name.
func paintChatIcon(dst *ggui.Canvas, env ggui.Env, role icons.Role, rect ggui.Rect, col color.Color, rotation float64) {
	asset := icons.Resolve(env, lucide.Set(), role)
	if asset == nil {
		asset = chatIcons()[role]
	}
	asset.Draw(dst, rect, col, rotation)
}
