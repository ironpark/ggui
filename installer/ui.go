package installer

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/runtime"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

// Main executes the installer described by app using os.Args and exits with
// status 1 on error. Passing nil runs the default standard setup wizard.
//
// When GGUI_INSTALLER_DESCRIBE is set by `ggui build` or `ggui release`, Main
// writes the build-time specification JSON to that path and returns cleanly
// without opening a window.
func Main(app *App) {
	if err := Run(app, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Run executes the installer or uninstaller described by app and args.
func Run(app *App, args []string) error {
	if app == nil {
		app = &App{}
	}
	if descPath := os.Getenv(DescribeEnv); descPath != "" {
		return writeDescription(descPath, app)
	}
	return runWithBackend(app, args, nil)
}

func runWithBackend(app *App, args []string, b Backend) error {
	if app == nil {
		app = &App{}
	}
	opts, err := ParseArgs(args)
	if err != nil {
		return err
	}
	payload, isPreview, err := resolvePayload(app)
	if err != nil {
		return err
	}
	if opts.ShowHelp {
		printHelp(payload.Meta)
		return nil
	}
	var previewCloser func() error
	if b == nil {
		if isPreview {
			pb := NewPreviewBackend()
			b = pb
			previewCloser = pb.Close
		} else {
			b = defaultBackend()
		}
	}
	if previewCloser != nil {
		defer previewCloser()
	}

	if opts.CleanupSelf {
		if self, err := b.SelfExe(); err == nil {
			defer b.ScheduleTempDelete(self)
		}
	}

	if opts.Uninstall {
		dir := resolveUninstallDir(payload.Meta, opts, b)
		if opts.FromDir == "" && !isPreview {
			if relaunched, err := b.RelaunchForUninstall(dir, opts.Silent); err != nil {
				return err
			} else if relaunched {
				return nil
			}
		}
		ic := newContext(app, payload, opts, dir, isPreview, b)
		if opts.Silent {
			return uninstall(context.Background(), payload.Meta, dir, b, ic, nil)
		}
		return runGUI(ic)
	}

	installDir := resolveInstallDir(payload.Meta, opts, b)
	ic := newContext(app, payload, opts, installDir, isPreview, b)
	if opts.Silent {
		iopts := resolveInstallOptions(payload.Meta, opts, installDir)
		return install(context.Background(), payload, iopts, b, ic, nil)
	}
	return runGUI(ic)
}

func resolveInstallDir(meta Metadata, opts CLIOptions, b Backend) string {
	if opts.InstallDir != "" {
		return opts.InstallDir
	}
	if existing, _, ok := b.ExistingInstall(meta); ok {
		return existing
	}
	return b.DefaultInstallDir(meta)
}

func resolveUninstallDir(meta Metadata, opts CLIOptions, b Backend) string {
	if opts.FromDir != "" {
		return opts.FromDir
	}
	if opts.InstallDir != "" {
		return opts.InstallDir
	}
	if existing, _, ok := b.ExistingInstall(meta); ok {
		return existing
	}
	if self, err := b.SelfExe(); err == nil {
		return filepath.Dir(self)
	}
	return b.DefaultInstallDir(meta)
}

func resolveInstallOptions(meta Metadata, opts CLIOptions, dir string) InstallOptions {
	startMenu := meta.StartMenuShortcut
	if opts.StartMenuSet {
		startMenu = !opts.NoStartMenu
	}
	desktop := meta.DesktopShortcut
	if opts.DesktopSet {
		desktop = !opts.NoDesktop
	}
	return InstallOptions{
		InstallDir:        dir,
		StartMenuShortcut: startMenu,
		DesktopShortcut:   desktop,
	}
}

func printHelp(meta Metadata) {
	fmt.Printf("%s %s Setup\n\n", meta.Name, meta.Version)
	fmt.Println("Usage:")
	fmt.Println("  --silent, /S              Install or uninstall without opening a window")
	fmt.Println("  --dir PATH, /D=PATH       Install into PATH")
	fmt.Println("  --uninstall               Uninstall the application")
	fmt.Println("  --no-start-menu           Do not create a Start Menu shortcut")
	fmt.Println("  --no-desktop-shortcut     Do not create a Desktop shortcut")
	fmt.Println("  --no-launch               Do not launch the app after installing")
}

// BuildWidget attaches UI runtime hooks (post, dialogs, closeWin) to c and
// builds the root ggui.Widget for the installer window. Useful for headless
// UI testing with ggui.Probe.
func (c *Context) BuildWidget(post func(func()), dialogs runtime.Dialogs, closeWin func()) ggui.Widget {
	c.post = post
	c.dialogs = dialogs
	c.closeWin = closeWin

	if c.app != nil && c.app.BuildUI != nil {
		return c.app.BuildUI(c)
	}

	var header ggui.Widget
	if c.app != nil && c.app.Header != nil {
		header = c.app.Header(c)
	} else {
		header = c.defaultHeader()
	}

	viewState := ggui.Combine(c.StepIndex, c.Failed, func(idx int, failed bool) wizardView {
		return wizardView{index: idx, failed: failed}
	})
	body := ggui.View(viewState, func(v wizardView) ggui.Widget {
		if v.failed {
			return c.buildErrorPage()
		}
		if v.index < 0 || v.index >= len(c.steps) {
			return widget.Box()
		}
		st := c.steps[v.index]
		var content ggui.Widget
		if st.Build != nil {
			content = st.Build(c)
		}
		var footer ggui.Widget
		if st.Footer != nil {
			footer = st.Footer(c)
		} else {
			footer = c.defaultFooter(st)
		}
		return c.page(content, footer)
	})

	return widget.Column(
		header,
		ui.Divider(),
		widget.Expanded(body),
	).Align(widget.AlignStretch)
}

type wizardView struct {
	index  int
	failed bool
}

func (c *Context) defaultHeader() ggui.Widget {
	meta := c.Meta()
	subtitle := "Version " + meta.Version
	if meta.Publisher != "" {
		subtitle += " · " + meta.Publisher
	}
	if c.IsUninstall() {
		subtitle = "Uninstall · " + subtitle
	}
	if c.IsPreview() {
		subtitle += " · Preview"
	}

	var headerParts []ggui.Widget
	if tex := c.Icon(); tex != nil {
		headerParts = append(headerParts, widget.Image(tex).Size(40, 40).Fit(ggui.FitContain))
	} else {
		headerParts = append(headerParts, widget.Box(widget.Center(ui.Icon(icons.Download).Size(22))).Size(40, 40).Radius(8).Fill(uitheme.Muted))
	}
	headerParts = append(headerParts, widget.Column(
		ui.Title(meta.Name),
		ui.Caption(subtitle),
	).Space(0.25))

	return widget.Box(
		widget.Row(headerParts...).Space(1.5).Align(widget.AlignCenter),
	).Pad(20, 24, 16, 24)
}

func (c *Context) defaultFooter(st Step) ggui.Widget {
	var left []ggui.Widget
	if c.HasPrev() {
		left = append(left, ui.Button("Back", c.Back).Outline())
	}
	left = append(left, ui.Button("Cancel", c.Close).Outline())

	label := "Next"
	if st.NextLabel != nil {
		if custom := st.NextLabel(c); custom != "" {
			label = custom
		}
	} else if c.NextIsProgress() {
		if c.IsUninstall() {
			label = "Uninstall"
		} else if c.IsUpgrade() {
			label = "Update"
		} else {
			label = "Install"
		}
	}

	nextBtn := ui.Button(label, c.Next)
	if st.CanContinue != nil {
		if ok := st.CanContinue(c); ok != nil {
			nextBtn = nextBtn.BindDisabled(ggui.Not(ok))
		}
	}
	return widget.Row(append(left,
		widget.Spacer(),
		nextBtn,
	)...).Space(1)
}

func (c *Context) buildErrorPage() ggui.Widget {
	title := "Installation failed"
	if c.IsUninstall() {
		title = "Uninstall failed"
	}
	content := widget.Column(
		ui.Alert(title, ggui.Peek(c.ErrText)).Destructive().Icon(icons.Alert),
	).Space(1).Align(widget.AlignStretch)
	footer := widget.Row(
		ui.Button("Close", c.Close).Outline(),
		widget.Spacer(),
		ui.Button("Retry", c.Retry),
	).Space(1)
	return c.page(content, footer)
}

func (c *Context) page(content, footer ggui.Widget) ggui.Widget {
	return widget.Column(
		widget.Expanded(widget.Box(content).Pad(20, 24)),
		ui.Divider(),
		widget.Box(footer).Pad(14, 24),
	).Align(widget.AlignStretch)
}

func runGUI(c *Context) error {
	meta := c.Meta()
	title := c.app.Title
	if title == "" {
		title = meta.Name + " Setup"
		if c.IsUninstall() {
			title = "Uninstall " + meta.Name
		}
		if c.IsPreview() {
			title += " (Preview)"
		}
	}
	width := c.app.Width
	if width <= 0 {
		width = 540
	}
	height := c.app.Height
	if height <= 0 {
		height = 390
	}
	var iconImages []image.Image
	if c.payload.Icon != nil {
		iconImages = []image.Image{c.payload.Icon}
	}

	light := c.app.Theme
	if light == nil {
		light = uitheme.Default()
	}
	dark := c.app.DarkTheme
	if dark == nil {
		if c.app.Theme != nil {
			dark = c.app.Theme
		} else {
			dark = uitheme.Dark()
		}
	}

	var app *ggui.App
	app = ggui.New(ggui.Config{
		Title:     title,
		Width:     width,
		Height:    height,
		Resizable: c.app.Resizable,
		Icon:      iconImages,
	}, func() ggui.Widget {
		return c.BuildWidget(app.Post, app.Dialogs(), app.Close)
	})
	app.Setup(func() {
		uitheme.Bind(ggui.SystemDark(), dark, light)
	})
	return app.Run()
}
