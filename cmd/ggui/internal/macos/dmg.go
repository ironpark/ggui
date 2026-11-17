package macos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ironpark/zapp/pkg/dmg"
	"github.com/ironpark/zapp/pkg/udif"
)

// DMG describes a disk image to make around an app: the app and a link
// to /Applications, in a Finder window laid out to drag one onto the
// other.
type DMG struct {
	App   string // the .app bundle
	Out   string // the .dmg file, replaced if it exists
	Title string // the volume's name
	// Icon is the volume's icon, a PNG or .icns; "" uses the app's.
	Icon string
	// Background is an image drawn at the window's top left, or "".
	Background          string
	Width, Height       int
	IconSize, LabelSize int
	// AppPosition and ApplicationsPosition are [x, y] of each icon's
	// centre; nil places the icons side by side across the window.
	AppPosition, ApplicationsPosition []int
	FS, Format                        string
}

// layout returns where the two icons go: as given, or zapp's default of
// side by side across the middle, a third of the way in from each edge.
func (d DMG) layout() (app, link [2]int) {
	y := d.Height/2 - d.IconSize/2 + d.LabelSize
	app = [2]int{d.Width/3 - d.IconSize/2, y}
	link = [2]int{d.Width*2/3 + d.IconSize/2, y}
	if len(d.AppPosition) == 2 {
		app = [2]int{d.AppPosition[0], d.AppPosition[1]}
	}
	if len(d.ApplicationsPosition) == 2 {
		link = [2]int{d.ApplicationsPosition[0], d.ApplicationsPosition[1]}
	}
	return app, link
}

// MakeDMG writes the disk image. It is pure Go, so it works on any host.
func MakeDMG(ctx context.Context, d DMG) error {
	fs, err := dmg.ParseFileSystem(d.FS)
	if err != nil {
		return err
	}
	format, err := udif.ParseFormat(d.Format)
	if err != nil {
		return err
	}
	icon := d.Icon
	if icon == "" {
		// The app's own icon, which Assemble wrote; without one the
		// volume keeps Finder's generic disk icon.
		if p := filepath.Join(d.App, "Contents", "Resources", iconName+".icns"); exists(p) {
			icon = p
		}
	} else if strings.EqualFold(filepath.Ext(icon), ".png") {
		tmp, err := os.MkdirTemp("", "ggui-dmg-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		png := icon
		icon = filepath.Join(tmp, "VolumeIcon.icns")
		if _, err := WriteIcon(icon, png); err != nil {
			return fmt.Errorf("macos.dmg.icon %s: %w", png, err)
		}
	}
	app, link := d.layout()
	c := dmg.Config{
		FileName:         d.Out,
		Title:            d.Title,
		Icon:             icon,
		Background:       d.Background,
		WindowWidth:      d.Width,
		WindowHeight:     d.Height,
		ContentsIconSize: d.IconSize,
		LabelSize:        d.LabelSize,
		FileSystem:       fs,
		Format:           format,
		Contents: []dmg.Item{
			{Type: dmg.Dir, Path: d.App, X: app[0], Y: app[1]},
			{Type: dmg.Link, Path: "/Applications", X: link[0], Y: link[1]},
		},
	}
	if err := c.Validate(); err != nil {
		return fmt.Errorf("disk image: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(d.Out), 0o755); err != nil {
		return err
	}
	if err := dmg.CreateDMG(ctx, c); err != nil {
		return fmt.Errorf("disk image: %w", err)
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
