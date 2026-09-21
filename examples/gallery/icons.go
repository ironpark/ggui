package main

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/icons"
	"github.com/ironpark/ggui/icons/heroicons"
	"github.com/ironpark/ggui/icons/lucide"
	"github.com/ironpark/ggui/icons/tabler"
	"github.com/ironpark/ggui/ui"
)

func iconPreview() ggui.Widget {
	selected := ggui.State("Lucide")
	alternate := ggui.State(false)
	checked := ggui.State(true)
	// Overrides only Check: every other role still falls back to Lucide.
	plus, err := lucide.Asset("plus")
	if err != nil {
		panic(err)
	}
	custom := icons.Map{icons.Check: plus}
	return ggui.Column(
		ui.Select(selected).Options([]string{"Lucide", "Tabler", "Heroicons"}).Name("Icon library"),
		// Key remounts the subtree when the library changes: the checkbox
		// inside is rebuilt on purpose, so it picks up the new check glyph.
		ggui.Key(selected, func(library string) ggui.Widget {
			var set icons.Set = lucide.Set()
			switch library {
			case "Tabler":
				set = tabler.Set()
			case "Heroicons":
				set = heroicons.Set()
			}
			return ggui.Provide(icons.SetKey, set, ggui.Column(
				ggui.Row(ui.Icon(icons.Check), ui.Icon(icons.Close), ui.Icon(icons.Search),
					ui.Icon(icons.Download), ui.Icon(icons.File), ui.Icon(icons.Sun), ui.Icon(icons.Moon)).Gap(16),
				ggui.Row(ui.Icon(icons.ChevronLeft), ui.Icon(icons.ChevronRight), ui.Icon(icons.ChevronUp),
					ui.Icon(icons.ChevronDown), ui.Icon(icons.Alert), ui.Icon(icons.Info), ui.Icon(icons.Plus),
					ui.Icon(icons.Minus), ui.Icon(icons.Grip), ui.Spinner()).Gap(12),
				ui.Checkbox(checked, "Selected library checkbox"),
			).Gap(14))
		}),
		ui.Switch(alternate, "Replace check with plus"),
		ggui.Key(alternate, func(replaced bool) ggui.Widget {
			var set icons.Set = lucide.Set()
			if replaced {
				set = custom
			}
			return ggui.Provide(icons.SetKey, set, ggui.Row(
				ui.Checkbox(checked, "Scoped icon override"), ui.Icon(icons.Close),
			).Gap(16))
		}),
		ggui.Caption("The same placeholders follow the theme or a local icon set. Missing roles keep their defaults."),
	).Gap(14)
}
