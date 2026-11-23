package installer

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/runtime"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

type setupStep int

const (
	stepLicense setupStep = iota
	stepConfig
	stepUninstallConfirm
	stepProgress
	stepDone
	stepError
)

// Main parses os.Args, runs the ggui setup wizard (or silent install/uninstall),
// and exits with status 1 on error.
func Main(payload []byte) {
	if err := Run(payload, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Run executes the installer or uninstaller described by payload and args.
func Run(payloadBytes []byte, args []string) error {
	return runWithBackend(payloadBytes, args, defaultBackend())
}

func runWithBackend(payloadBytes []byte, args []string, b Backend) error {
	opts, err := ParseArgs(args)
	if err != nil {
		return err
	}
	payload, err := ParsePayload(payloadBytes)
	if err != nil {
		return err
	}
	if opts.ShowHelp {
		printHelp(payload.Meta)
		return nil
	}
	if opts.CleanupSelf {
		if self, err := b.SelfExe(); err == nil {
			defer b.ScheduleTempDelete(self)
		}
	}

	if opts.Uninstall {
		dir := resolveUninstallDir(payload.Meta, opts, b)
		if opts.FromDir == "" {
			if relaunched, err := b.RelaunchForUninstall(dir, opts.Silent); err != nil {
				return err
			} else if relaunched {
				return nil
			}
		}
		if opts.Silent {
			return Uninstall(context.Background(), payload.Meta, dir, b, nil)
		}
		return runGUI(payload, opts, dir, b)
	}

	installDir := resolveInstallDir(payload.Meta, opts, b)
	if opts.Silent {
		iopts := resolveInstallOptions(payload.Meta, opts, installDir)
		return Install(context.Background(), payload, iopts, b, nil)
	}
	return runGUI(payload, opts, installDir, b)
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

// setupModel holds all reactive UI state for the ggui setup wizard so it can
// be driven both by a real App window and headlessly by ggtest.Probe.
type setupModel struct {
	payload     *Payload
	backend     Backend
	uninstall   bool
	isUpgrade   bool
	prevVersion string
	iconTex     *ggfx.Image

	Step            *ggui.StateValue[setupStep]
	InstallDir      *ggui.StateValue[string]
	StartMenu       *ggui.StateValue[bool]
	Desktop         *ggui.StateValue[bool]
	LaunchAfter     *ggui.StateValue[bool]
	LicenseAccepted *ggui.StateValue[bool]
	Progress        *ggui.StateValue[float64]
	Status          *ggui.StateValue[string]
	ErrText         *ggui.StateValue[string]
}

func newSetupModel(payload *Payload, opts CLIOptions, targetDir string, b Backend) *setupModel {
	meta := payload.Meta
	iopts := resolveInstallOptions(meta, opts, targetDir)
	launchAfter := meta.LaunchAfter
	if opts.LaunchSet {
		launchAfter = !opts.NoLaunch
	}
	_, prevVer, isUpgrade := b.ExistingInstall(meta)

	initialStep := stepConfig
	if opts.Uninstall {
		initialStep = stepUninstallConfirm
	} else if strings.TrimSpace(meta.License) != "" {
		initialStep = stepLicense
	}

	return &setupModel{
		payload:         payload,
		backend:         b,
		uninstall:       opts.Uninstall,
		isUpgrade:       isUpgrade,
		prevVersion:     prevVer,
		Step:            ggui.State(initialStep),
		InstallDir:      ggui.State(iopts.InstallDir),
		StartMenu:       ggui.State(iopts.StartMenuShortcut),
		Desktop:         ggui.State(iopts.DesktopShortcut),
		LaunchAfter:     ggui.State(launchAfter),
		LicenseAccepted: ggui.State(false),
		Progress:        ggui.State(0.0),
		Status:          ggui.State(""),
		ErrText:         ggui.State(""),
	}
}

func (m *setupModel) ensureIcon() *ggfx.Image {
	if m.iconTex == nil && m.payload.Icon != nil {
		m.iconTex = ggfx.NewImageFromImage(m.payload.Icon)
	}
	return m.iconTex
}

func (m *setupModel) browse(post func(func()), dialogs runtime.Dialogs) {
	if dialogs == nil {
		return
	}
	cur := ggui.Peek(m.InstallDir)
	go func() {
		picked, err := dialogs.PickFolder(runtime.FileDialog{
			Title:     "Choose install folder",
			Directory: cur,
		})
		if err != nil || strings.TrimSpace(picked) == "" {
			return
		}
		// Append app name if user picked a parent directory that does not already end with it.
		if !strings.EqualFold(filepath.Base(picked), m.payload.Meta.Name) {
			picked = filepath.Join(picked, m.payload.Meta.Name)
		}
		post(func() { m.InstallDir.Set(picked) })
	}()
}

func (m *setupModel) startInstall(post func(func())) {
	dir := strings.TrimSpace(ggui.Peek(m.InstallDir))
	if dir == "" {
		m.ErrText.Set("Please choose an install location.")
		m.Step.Set(stepError)
		return
	}
	iopts := InstallOptions{
		InstallDir:        dir,
		StartMenuShortcut: ggui.Peek(m.StartMenu),
		DesktopShortcut:   ggui.Peek(m.Desktop),
	}
	m.Progress.Set(0)
	m.Status.Set("Preparing installation…")
	m.ErrText.Set("")
	m.Step.Set(stepProgress)

	go func() {
		err := Install(context.Background(), m.payload, iopts, m.backend, func(p Progress) {
			post(func() {
				m.Progress.Set(p.Fraction)
				m.Status.Set(p.Message)
			})
		})
		post(func() {
			if err != nil {
				m.ErrText.Set(err.Error())
				m.Step.Set(stepError)
				return
			}
			m.Progress.Set(1.0)
			m.Status.Set("Installation complete.")
			m.Step.Set(stepDone)
		})
	}()
}

func (m *setupModel) startUninstall(post func(func())) {
	dir := strings.TrimSpace(ggui.Peek(m.InstallDir))
	m.Progress.Set(0)
	m.Status.Set("Removing files…")
	m.ErrText.Set("")
	m.Step.Set(stepProgress)

	go func() {
		err := Uninstall(context.Background(), m.payload.Meta, dir, m.backend, func(p Progress) {
			post(func() {
				m.Progress.Set(p.Fraction)
				m.Status.Set(p.Message)
			})
		})
		post(func() {
			if err != nil {
				m.ErrText.Set(err.Error())
				m.Step.Set(stepError)
				return
			}
			m.Progress.Set(1.0)
			m.Status.Set("Uninstall complete.")
			m.Step.Set(stepDone)
		})
	}()
}

func (m *setupModel) finish(closeWin func()) {
	if !m.uninstall && ggui.Peek(m.LaunchAfter) {
		exe := filepath.Join(ggui.Peek(m.InstallDir), filepath.FromSlash(m.payload.Meta.Executable))
		_ = m.backend.Launch(exe)
	}
	closeWin()
}

func (m *setupModel) build(post func(func()), dialogs runtime.Dialogs, closeWin func()) ggui.Widget {
	meta := m.payload.Meta
	subtitle := "Version " + meta.Version
	if meta.Publisher != "" {
		subtitle += " · " + meta.Publisher
	}
	if m.uninstall {
		subtitle = "Uninstall · " + subtitle
	}

	var headerParts []ggui.Widget
	if tex := m.ensureIcon(); tex != nil {
		headerParts = append(headerParts, widget.Image(tex).Size(40, 40).Fit(ggui.FitContain))
	} else {
		headerParts = append(headerParts, widget.Box(widget.Center(ui.Icon(icons.Download).Size(22))).Size(40, 40).Radius(8).Fill(uitheme.Muted))
	}
	headerParts = append(headerParts, widget.Column(
		ui.Title(meta.Name),
		ui.Caption(subtitle),
	).Space(0.25))

	header := widget.Box(
		widget.Row(headerParts...).Space(1.5).Align(widget.AlignCenter),
	).Pad(20, 24, 16, 24)

	body := ggui.View(m.Step, func(st setupStep) ggui.Widget {
		return m.buildStep(st, post, dialogs, closeWin)
	})

	return widget.Column(
		header,
		ui.Divider(),
		widget.Expanded(body),
	).Align(widget.AlignStretch)
}

func (m *setupModel) buildStep(st setupStep, post func(func()), dialogs runtime.Dialogs, closeWin func()) ggui.Widget {
	meta := m.payload.Meta
	switch st {
	case stepLicense:
		canContinue := ggui.Not(m.LicenseAccepted)
		content := widget.Column(
			ui.Heading("License Agreement"),
			ui.Caption("Please read and accept the license agreement before installing."),
			widget.Expanded(
				widget.Box(
					widget.Scroll(widget.Box(ui.Mono(meta.License)).Pad(12)),
				).Border(1, uitheme.Border).Radius(6),
			),
			ui.Checkbox(m.LicenseAccepted, "I accept the terms in the License Agreement"),
		).Space(1).Align(widget.AlignStretch)
		footer := widget.Row(
			ui.Button("Cancel", closeWin).Outline(),
			widget.Spacer(),
			ui.Button("Next", func() { m.Step.Set(stepConfig) }).BindDisabled(canContinue),
		).Space(1)
		return m.page(content, footer)

	case stepConfig:
		heading := "Install " + meta.Name
		cta := "Install"
		if m.isUpgrade {
			heading = "Update " + meta.Name
			cta = "Update"
			if m.prevVersion != "" && m.prevVersion != meta.Version {
				heading = fmt.Sprintf("Update %s (%s → %s)", meta.Name, m.prevVersion, meta.Version)
			}
		}
		scopeHint := "Installs for your user account (no administrator prompt)."
		if meta.NormalizedScope() == ScopeMachine {
			scopeHint = "Installs for all users on this computer."
		}
		dirEmpty := m.InstallDir.Map(func(s string) bool { return strings.TrimSpace(s) == "" })

		pathRow := widget.Row(
			widget.Expanded(ui.TextField(m.InstallDir).Placeholder("Install folder")),
			ui.Button("Browse…", func() { m.browse(post, dialogs) }).Outline(),
		).Space(1).Align(widget.AlignCenter)

		items := []ggui.Widget{
			ui.Heading(heading),
		}
		if meta.Description != "" {
			items = append(items, ui.Caption(meta.Description))
		}
		items = append(items,
			ui.Field("Install location", pathRow).Help(scopeHint),
			widget.Column(
				ui.Checkbox(m.StartMenu, "Create Start Menu shortcut"),
				ui.Checkbox(m.Desktop, "Create Desktop shortcut"),
			).Space(0.75),
		)
		content := widget.Column(items...).Space(1.25).Align(widget.AlignStretch)

		var left []ggui.Widget
		if strings.TrimSpace(meta.License) != "" {
			left = append(left, ui.Button("Back", func() { m.Step.Set(stepLicense) }).Outline())
		}
		left = append(left, ui.Button("Cancel", closeWin).Outline())
		footer := widget.Row(append(left,
			widget.Spacer(),
			ui.Button(cta, func() { m.startInstall(post) }).BindDisabled(dirEmpty),
		)...).Space(1)
		return m.page(content, footer)

	case stepUninstallConfirm:
		content := widget.Column(
			ui.Heading("Uninstall "+meta.Name),
			ui.Caption("This will remove "+meta.Name+" and its shortcuts from your computer."),
			ui.Field("Installed location", ui.Mono(ggui.Peek(m.InstallDir))),
		).Space(1.25).Align(widget.AlignStretch)
		footer := widget.Row(
			ui.Button("Cancel", closeWin).Outline(),
			widget.Spacer(),
			ui.Button("Uninstall", func() { m.startUninstall(post) }).Destructive(),
		).Space(1)
		return m.page(content, footer)

	case stepProgress:
		heading := "Installing " + meta.Name + "…"
		if m.uninstall {
			heading = "Uninstalling " + meta.Name + "…"
		}
		content := widget.Column(
			ui.Heading(heading),
			ui.Progress(m.Progress).Height(8).ShowValue(),
			ui.CaptionOf(m.Status),
		).Space(1.25).Align(widget.AlignStretch)
		footer := widget.Row(
			widget.Spacer(),
			ui.Button("Cancel", func() {}).Outline().Disabled(true),
		)
		return m.page(content, footer)

	case stepDone:
		title := meta.Name + " has been installed."
		sub := "Installed to " + ggui.Peek(m.InstallDir)
		if m.uninstall {
			title = meta.Name + " has been uninstalled."
			sub = "Application files and shortcuts were removed."
		}
		items := []ggui.Widget{
			widget.Row(
				ui.Icon(icons.Check).Size(20),
				ui.Heading(title),
			).Space(1).Align(widget.AlignCenter),
			ui.Caption(sub),
		}
		if !m.uninstall {
			items = append(items, ui.Checkbox(m.LaunchAfter, "Launch "+meta.Name))
		}
		content := widget.Column(items...).Space(1.25).Align(widget.AlignStretch)
		footer := widget.Row(
			widget.Spacer(),
			ui.Button("Finish", func() { m.finish(closeWin) }),
		)
		return m.page(content, footer)

	default: // stepError
		title := "Installation failed"
		retry := func() { m.startInstall(post) }
		if m.uninstall {
			title = "Uninstall failed"
			retry = func() { m.startUninstall(post) }
		}
		content := widget.Column(
			ui.Alert(title, ggui.Peek(m.ErrText)).Destructive().Icon(icons.Alert),
		).Space(1).Align(widget.AlignStretch)
		footer := widget.Row(
			ui.Button("Close", closeWin).Outline(),
			widget.Spacer(),
			ui.Button("Retry", retry),
		).Space(1)
		return m.page(content, footer)
	}
}

func (m *setupModel) page(content, footer ggui.Widget) ggui.Widget {
	return widget.Column(
		widget.Expanded(widget.Box(content).Pad(20, 24)),
		ui.Divider(),
		widget.Box(footer).Pad(14, 24),
	).Align(widget.AlignStretch)
}

func runGUI(payload *Payload, opts CLIOptions, targetDir string, b Backend) error {
	m := newSetupModel(payload, opts, targetDir, b)
	title := payload.Meta.Name + " Setup"
	if opts.Uninstall {
		title = "Uninstall " + payload.Meta.Name
	}
	var iconImages []image.Image
	if payload.Icon != nil {
		iconImages = []image.Image{payload.Icon}
	}
	var app *ggui.App
	app = ggui.New(ggui.Config{
		Title:     title,
		Width:     540,
		Height:    390,
		Resizable: false,
		Icon:      iconImages,
	}, func() ggui.Widget {
		return m.build(app.Post, app.Dialogs(), app.Close)
	})
	app.Setup(func() {
		uitheme.Bind(ggui.SystemDark(), uitheme.Dark(), uitheme.Default())
	})
	return app.Run()
}
