package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/theme"
)

// The theme's named text styles, each applied to a Text built any of the
// ways ggui builds one: from a string, a reader (Of) or a format (f).

func title(t *ggui.TextWidget) *ggui.TextWidget {
	return t.StyleKey(theme.TitleKey, theme.Default().Title).Role(ggui.RoleHeading)
}

func heading(t *ggui.TextWidget) *ggui.TextWidget {
	return t.StyleKey(theme.HeadingKey, theme.Default().Heading).Role(ggui.RoleHeading)
}

func caption(t *ggui.TextWidget) *ggui.TextWidget {
	return t.StyleKey(theme.CaptionKey, theme.Default().Caption)
}

func mono(t *ggui.TextWidget) *ggui.TextWidget {
	return t.StyleKey(theme.MonoKey, theme.Default().Mono)
}

// Title draws a page heading using the inherited theme's title style.
func Title(s string) *ggui.TextWidget { return title(ggui.Text(s)) }

// TitleOf is Title following r, as ggui.TextOf follows it.
func TitleOf(r ggui.Readable[string]) *ggui.TextWidget { return title(ggui.TextOf(r)) }

// Titlef is Title formatting its arguments, as ggui.Textf does.
func Titlef(format string, args ...any) *ggui.TextWidget { return title(ggui.Textf(format, args...)) }

// Heading draws the title of a dialog, sheet or card using the inherited
// theme's heading style.
func Heading(s string) *ggui.TextWidget { return heading(ggui.Text(s)) }

// HeadingOf is Heading following r, as ggui.TextOf follows it.
func HeadingOf(r ggui.Readable[string]) *ggui.TextWidget { return heading(ggui.TextOf(r)) }

// Headingf is Heading formatting its arguments, as ggui.Textf does.
func Headingf(format string, args ...any) *ggui.TextWidget {
	return heading(ggui.Textf(format, args...))
}

// Caption draws text using the inherited theme's caption style.
func Caption(s string) *ggui.TextWidget { return caption(ggui.Text(s)) }

// CaptionOf is Caption following r, as ggui.TextOf follows it.
func CaptionOf(r ggui.Readable[string]) *ggui.TextWidget { return caption(ggui.TextOf(r)) }

// Captionf is Caption formatting its arguments, as ggui.Textf does.
func Captionf(format string, args ...any) *ggui.TextWidget {
	return caption(ggui.Textf(format, args...))
}

// Mono draws code using the inherited theme's monospaced style.
func Mono(s string) *ggui.TextWidget { return mono(ggui.Text(s)) }

// MonoOf is Mono following r, as ggui.TextOf follows it.
func MonoOf(r ggui.Readable[string]) *ggui.TextWidget { return mono(ggui.TextOf(r)) }

// Monof is Mono formatting its arguments, as ggui.Textf does.
func Monof(format string, args ...any) *ggui.TextWidget { return mono(ggui.Textf(format, args...)) }
