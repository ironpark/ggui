package chartdemo

import (
	"embed"
	"github.com/ironpark/ggui/icons"
)

//go:embed icons/*.svg
var iconFiles embed.FS
var chartIcons = func() icons.Map {
	out := icons.Map{}
	for role, file := range map[icons.Role]string{"chart-trending-up": "trending-up", "chart-trending-down": "trending-down", "chart-arrow-up": "arrow-up-from-line", "chart-arrow-down": "arrow-down-from-line", "chart-footprints": "footprints", "chart-waves": "waves", "chart-commit": "git-commit-vertical"} {
		svg, err := icons.Load(iconFiles, "icons/"+file+".svg")
		if err != nil {
			panic(err)
		}
		out[role] = svg
	}
	return out
}()
