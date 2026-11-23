package installer

import (
	"fmt"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

// StepKind identifies special lifecycle steps in a setup wizard.
type StepKind int

const (
	// StepNormal is a standard navigable step (Welcome, License, Location, custom).
	StepNormal StepKind = iota
	// StepProgress executes Install or Uninstall while displaying progress.
	StepProgress
	// StepFinish displays completion status and optional launch checkbox.
	StepFinish
)

// Step describes one screen in the setup or uninstall wizard.
// Step is a configuration value: fluent setters return a modified copy.
type Step struct {
	ID          string
	Title       string
	Kind        StepKind
	Build       func(ctx *Context) ggui.Widget
	Footer      func(ctx *Context) ggui.Widget
	CanContinue func(ctx *Context) ggui.Readable[bool]
	NextLabel   func(ctx *Context) string
	OnEnter     func(ctx *Context)
	Skip        func(ctx *Context) bool
}

// WithTitle sets the step's display title.
func (s Step) WithTitle(title string) Step { s.Title = title; return s }

// WithFooter overrides the step's bottom action bar.
func (s Step) WithFooter(fn func(ctx *Context) ggui.Widget) Step { s.Footer = fn; return s }

// WithCanContinue binds the enabled state of the default primary button.
func (s Step) WithCanContinue(fn func(ctx *Context) ggui.Readable[bool]) Step {
	s.CanContinue = fn
	return s
}

// WithNextLabel overrides the default primary button text.
func (s Step) WithNextLabel(fn func(ctx *Context) string) Step { s.NextLabel = fn; return s }

// WithOnEnter registers a callback invoked whenever the wizard enters this step.
func (s Step) WithOnEnter(fn func(ctx *Context)) Step { s.OnEnter = fn; return s }

// SkipWhen skips this step during wizard navigation when fn returns true.
func (s Step) SkipWhen(fn func(ctx *Context) bool) Step { s.Skip = fn; return s }

// DefaultSteps returns the standard install wizard flow:
// [LicenseStep, LocationStep, ProgressStep, FinishStep].
func DefaultSteps() []Step {
	return []Step{
		LicenseStep(),
		LocationStep(),
		ProgressStep(),
		FinishStep(),
	}
}

// DefaultUninstallSteps returns the standard uninstall wizard flow:
// [UninstallConfirmStep, ProgressStep, FinishStep].
func DefaultUninstallSteps() []Step {
	return []Step{
		UninstallConfirmStep(),
		ProgressStep(),
		FinishStep(),
	}
}

// CustomStep creates a custom navigable step with an optional heading and a
// ggui widget builder. The wizard automatically provides Back, Cancel, and
// Next (or Install/Update when followed by ProgressStep) buttons unless
// overridden via WithFooter.
func CustomStep(id, title string, build func(ctx *Context) ggui.Widget) Step {
	return Step{
		ID:    id,
		Title: title,
		Kind:  StepNormal,
		Build: func(ctx *Context) ggui.Widget {
			var body ggui.Widget
			if build != nil {
				body = build(ctx)
			}
			if title == "" {
				return body
			}
			if body == nil {
				return ui.Heading(title)
			}
			return widget.Column(
				ui.Heading(title),
				body,
			).Space(1.25).Align(widget.AlignStretch)
		},
	}
}

// WelcomeStep creates an introductory welcome screen. Empty title or body
// falls back to sensible defaults derived from ctx.Meta().
func WelcomeStep(title, body string) Step {
	return Step{
		ID:    "welcome",
		Title: title,
		Kind:  StepNormal,
		Build: func(ctx *Context) ggui.Widget {
			meta := ctx.Meta()
			h := title
			if h == "" {
				h = "Welcome to " + meta.Name + " Setup"
			}
			msg := body
			if msg == "" {
				if meta.Description != "" {
					msg = meta.Description
				} else {
					msg = fmt.Sprintf("This wizard will guide you through installing %s %s on your computer.", meta.Name, meta.Version)
				}
			}
			return widget.Column(
				ui.Heading(h),
				widget.Text(msg),
			).Space(1.25).Align(widget.AlignStretch)
		},
	}
}

// LicenseStep creates the scrollable license agreement screen. It is
// automatically skipped when the installer metadata has no license text.
func LicenseStep() Step {
	return Step{
		ID:    "license",
		Title: "License Agreement",
		Kind:  StepNormal,
		Skip: func(ctx *Context) bool {
			return strings.TrimSpace(ctx.Meta().License) == ""
		},
		CanContinue: func(ctx *Context) ggui.Readable[bool] {
			return ctx.LicenseAccepted
		},
		Build: func(ctx *Context) ggui.Widget {
			meta := ctx.Meta()
			return widget.Column(
				ui.Heading("License Agreement"),
				ui.Caption("Please read and accept the license agreement before installing."),
				widget.Expanded(
					widget.Box(
						widget.Scroll(widget.Box(ui.Mono(meta.License)).Pad(12)),
					).Border(1, uitheme.Border).Radius(6),
				),
				ui.Checkbox(ctx.LicenseAccepted, "I accept the terms in the License Agreement"),
			).Space(1).Align(widget.AlignStretch)
		},
	}
}

// LocationStep creates the standard install folder and shortcut options screen.
func LocationStep() Step {
	return Step{
		ID:    "location",
		Title: "Install Location",
		Kind:  StepNormal,
		CanContinue: func(ctx *Context) ggui.Readable[bool] {
			return ctx.InstallDir.Map(func(s string) bool { return strings.TrimSpace(s) != "" })
		},
		Build: func(ctx *Context) ggui.Widget {
			meta := ctx.Meta()
			heading := "Install " + meta.Name
			if ctx.IsUpgrade() {
				heading = "Update " + meta.Name
				if prev := ctx.PreviousVersion(); prev != "" && prev != meta.Version {
					heading = fmt.Sprintf("Update %s (%s → %s)", meta.Name, prev, meta.Version)
				}
			}
			scopeHint := "Installs for your user account (no administrator prompt)."
			if meta.NormalizedScope() == ScopeMachine {
				scopeHint = "Installs for all users on this computer."
			}
			pathRow := widget.Row(
				widget.Expanded(ui.TextField(ctx.InstallDir).Placeholder("Install folder")),
				ui.Button("Browse…", ctx.BrowseFolder).Outline(),
			).Space(1).Align(widget.AlignCenter)

			items := []ggui.Widget{ui.Heading(heading)}
			if meta.Description != "" {
				items = append(items, ui.Caption(meta.Description))
			}
			items = append(items,
				ui.Field("Install location", pathRow).Help(scopeHint),
				widget.Column(
					ui.Checkbox(ctx.StartMenu, "Create Start Menu shortcut"),
					ui.Checkbox(ctx.Desktop, "Create Desktop shortcut"),
				).Space(0.75),
			)
			return widget.Column(items...).Space(1.25).Align(widget.AlignStretch)
		},
	}
}

// UninstallConfirmStep creates the standard uninstall confirmation screen.
func UninstallConfirmStep() Step {
	return Step{
		ID:    "uninstall-confirm",
		Title: "Uninstall",
		Kind:  StepNormal,
		Build: func(ctx *Context) ggui.Widget {
			meta := ctx.Meta()
			return widget.Column(
				ui.Heading("Uninstall "+meta.Name),
				ui.Caption("This will remove "+meta.Name+" and its shortcuts from your computer."),
				ui.Field("Installed location", ui.Mono(ggui.Peek(ctx.InstallDir))),
			).Space(1.25).Align(widget.AlignStretch)
		},
		Footer: func(ctx *Context) ggui.Widget {
			return widget.Row(
				ui.Button("Cancel", ctx.Close).Outline(),
				widget.Spacer(),
				ui.Button("Uninstall", ctx.Next).Destructive(),
			).Space(1)
		},
	}
}

// ProgressStep creates the installation/uninstallation progress screen.
// Entering this step automatically triggers ctx.StartInstall or ctx.StartUninstall.
func ProgressStep() Step {
	return Step{
		ID:    "progress",
		Title: "Progress",
		Kind:  StepProgress,
		OnEnter: func(ctx *Context) {
			if ctx.IsUninstall() {
				ctx.StartUninstall()
				return
			}
			ctx.StartInstall()
		},
		Build: func(ctx *Context) ggui.Widget {
			meta := ctx.Meta()
			heading := "Installing " + meta.Name + "…"
			if ctx.IsUninstall() {
				heading = "Uninstalling " + meta.Name + "…"
			}
			return widget.Column(
				ui.Heading(heading),
				ui.Progress(ctx.Progress).Height(8).ShowValue(),
				ui.CaptionOf(ctx.Status),
			).Space(1.25).Align(widget.AlignStretch)
		},
		Footer: func(ctx *Context) ggui.Widget {
			return widget.Row(
				widget.Spacer(),
				ui.Button("Cancel", func() {}).Outline().Disabled(true),
			)
		},
	}
}

// FinishStep creates the completion screen with optional "Launch <Name>" checkbox.
func FinishStep() Step {
	return Step{
		ID:    "finish",
		Title: "Complete",
		Kind:  StepFinish,
		Build: func(ctx *Context) ggui.Widget {
			meta := ctx.Meta()
			title := meta.Name + " has been installed."
			sub := "Installed to " + ggui.Peek(ctx.InstallDir)
			if ctx.IsUninstall() {
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
			if !ctx.IsUninstall() {
				items = append(items, ui.Checkbox(ctx.LaunchAfter, "Launch "+meta.Name))
			}
			return widget.Column(items...).Space(1.25).Align(widget.AlignStretch)
		},
		Footer: func(ctx *Context) ggui.Widget {
			return widget.Row(
				widget.Spacer(),
				ui.Button("Finish", ctx.Finish),
			)
		},
	}
}
