package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/theme"
)

// Title draws a heading using the inherited theme's title style.
func Title(s string) *ggui.TextWidget {
	return ggui.Text(s).StyleKey(theme.TitleKey, theme.Default().Title).Role(ggui.RoleHeading)
}

// Caption draws text using the inherited theme's caption style.
func Caption(s string) *ggui.TextWidget {
	return ggui.Text(s).StyleKey(theme.CaptionKey, theme.Default().Caption)
}
