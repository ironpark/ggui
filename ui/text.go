package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

// The theme's named text styles, each applied to a Text built any of the
// ways ggui builds one: from a string, a reader (Of) or a format (f).

func title(t *widget.TextWidget) *widget.TextWidget {
	return t.StyleKey(theme.TitleKey, theme.Default().Title).Role(ggui.RoleHeading)
}

func heading(t *widget.TextWidget) *widget.TextWidget {
	return t.StyleKey(theme.HeadingKey, theme.Default().Heading).Role(ggui.RoleHeading)
}

func caption(t *widget.TextWidget) *widget.TextWidget {
	return t.StyleKey(theme.CaptionKey, theme.Default().Caption)
}

func mono(t *widget.TextWidget) *widget.TextWidget {
	return t.StyleKey(theme.MonoKey, theme.Default().Mono)
}

// Title draws a page heading using the inherited theme's title style.
func Title(s string) *widget.TextWidget { return title(widget.Text(s)) }

// TitleOf is Title following r, as widget.TextOf follows it.
func TitleOf(r ggui.Readable[string]) *widget.TextWidget { return title(widget.TextOf(r)) }

// Titlef is Title formatting its arguments, as widget.Textf does.
func Titlef(format string, args ...any) *widget.TextWidget {
	return title(widget.Textf(format, args...))
}

// Heading draws the title of a dialog, sheet or card using the inherited
// theme's heading style.
func Heading(s string) *widget.TextWidget { return heading(widget.Text(s)) }

// HeadingOf is Heading following r, as widget.TextOf follows it.
func HeadingOf(r ggui.Readable[string]) *widget.TextWidget { return heading(widget.TextOf(r)) }

// Headingf is Heading formatting its arguments, as widget.Textf does.
func Headingf(format string, args ...any) *widget.TextWidget {
	return heading(widget.Textf(format, args...))
}

// Caption draws text using the inherited theme's caption style.
func Caption(s string) *widget.TextWidget { return caption(widget.Text(s)) }

// CaptionOf is Caption following r, as widget.TextOf follows it.
func CaptionOf(r ggui.Readable[string]) *widget.TextWidget { return caption(widget.TextOf(r)) }

// Captionf is Caption formatting its arguments, as widget.Textf does.
func Captionf(format string, args ...any) *widget.TextWidget {
	return caption(widget.Textf(format, args...))
}

// Mono draws code using the inherited theme's monospaced style.
func Mono(s string) *widget.TextWidget { return mono(widget.Text(s)) }

// MonoOf is Mono following r, as widget.TextOf follows it.
func MonoOf(r ggui.Readable[string]) *widget.TextWidget { return mono(widget.TextOf(r)) }

// Monof is Mono formatting its arguments, as widget.Textf does.
func Monof(format string, args ...any) *widget.TextWidget { return mono(widget.Textf(format, args...)) }
