package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/theme"
)

// Title draws a page heading using the inherited theme's title style.
func Title(s string) *ggui.TextWidget {
	return ggui.Text(s).StyleKey(theme.TitleKey, theme.Default().Title).Role(ggui.RoleHeading)
}

// Heading draws the title of a dialog, sheet or card using the inherited
// theme's heading style.
func Heading(s string) *ggui.TextWidget {
	return ggui.Text(s).StyleKey(theme.HeadingKey, theme.Default().Heading).Role(ggui.RoleHeading)
}

// Caption draws text using the inherited theme's caption style.
func Caption(s string) *ggui.TextWidget {
	return ggui.Text(s).StyleKey(theme.CaptionKey, theme.Default().Caption)
}

// Mono draws code using the inherited theme's monospaced style.
func Mono(s string) *ggui.TextWidget {
	return ggui.Text(s).StyleKey(theme.MonoKey, theme.Default().Mono)
}
