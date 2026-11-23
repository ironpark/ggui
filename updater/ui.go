package updater

import (
	"errors"
	"net/url"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/runtime"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

type panelSnapshot struct {
	phase      Phase
	release    *Release
	downloaded *DownloadedUpdate
	errText    string
}

// Panel builds the reactive Sparkle-inspired software update panel bound to ctrl.
// Embed it in a custom container, wrap it in Dialog(ctrl), or display it in a
// standalone window with ShowWindow.
func Panel(ctrl *Controller) ggui.Widget {
	if ctrl == nil {
		panic("updater: Panel requires a non-nil *Controller")
	}
	if h, ok := ggui.LookupHost(); ok {
		ctrl.ensurePost(h.Post)
	}

	viewState := ggui.Derived(func() panelSnapshot {
		return panelSnapshot{
			phase:      ctrl.Phase.Get(),
			release:    ctrl.Release.Get(),
			downloaded: ctrl.Downloaded.Get(),
			errText:    ctrl.ErrText.Get(),
		}
	})

	return ggui.View(viewState, func(s panelSnapshot) ggui.Widget {
		switch s.phase {
		case PhaseChecking:
			return buildCheckingPanel(ctrl)
		case PhaseUpToDate:
			return buildUpToDatePanel(ctrl)
		case PhaseAvailable:
			return buildAvailablePanel(ctrl, s.release)
		case PhaseDownloading:
			return buildDownloadingPanel(ctrl, s.release)
		case PhaseReadyToInstall:
			return buildReadyPanel(ctrl, s.release, s.downloaded)
		case PhaseInstalling:
			return buildInstallingPanel(ctrl)
		case PhaseError:
			return buildErrorPanel(ctrl, s.errText)
		default:
			return buildIdlePanel(ctrl)
		}
	})
}

type dialogDismissSync struct {
	ctrl *Controller
	dlg  *ui.DialogWidget
}

func (s dialogDismissSync) Get() string {
	p := s.ctrl.Phase.Get()
	canDismiss := p != PhaseDownloading && p != PhaseInstalling
	s.dlg.Dismissible(canDismiss)
	s.dlg.CloseButton(canDismiss)
	return s.ctrl.tr("Software Update")
}

// Dialog wraps Panel(ctrl) in a modal ui.Dialog bound to ctrl.Open.
// The dialog is dismissible except while downloading or installing an update.
func Dialog(ctrl *Controller) *ui.DialogWidget {
	if ctrl == nil {
		panic("updater: Dialog requires a non-nil *Controller")
	}
	if h, ok := ggui.LookupHost(); ok {
		ctrl.ensurePost(h.Post)
	}
	openBinding := ggui.Bind(ctrl.Open.Get, func(open bool) {
		if !open {
			switch ggui.Peek(ctrl.Phase) {
			case PhaseDownloading, PhaseInstalling:
				return
			}
		}
		ctrl.Open.Set(open)
	})

	dlg := ui.Dialog(openBinding, Panel(ctrl)).
		Width(500).
		Compact().
		Name(ctrl.tr("Software Update")).
		OnClose(ctrl.Dismiss)

	syncDismissible := func(p Phase) {
		canDismiss := p != PhaseDownloading && p != PhaseInstalling
		dlg.Dismissible(canDismiss)
		dlg.CloseButton(canDismiss)
	}

	if ggui.UseScope() != (ggui.Scope{}) {
		ggui.Observe(func() {
			syncDismissible(ctrl.Phase.Get())
		})
	} else {
		syncDismissible(ggui.Peek(ctrl.Phase))
		dlg.BindName(dialogDismissSync{ctrl: ctrl, dlg: dlg})
	}

	return dlg
}

// ShowWindow wires app.Post and app.Quit into ctrl (without overwriting
// existing callbacks) and opens a non-resizable 520x420 standalone window
// titled "Software Update" displaying Panel(ctrl). The window closes
// automatically when ctrl.Open becomes false.
func ShowWindow(app *ggui.App, ctrl *Controller) (*ggui.Window, error) {
	if app == nil {
		return nil, errors.New("updater: ShowWindow requires a non-nil *ggui.App")
	}
	if ctrl == nil {
		return nil, errors.New("updater: ShowWindow requires a non-nil *Controller")
	}

	ctrl.ensurePost(app.Post)
	ctrl.ensureQuit(app.Quit)

	win, err := app.OpenWindow(ggui.Config{
		Title:     ctrl.tr("Software Update"),
		Width:     520,
		Height:    420,
		Resizable: false,
	}, func() ggui.Widget {
		w := ggui.UseWindow()
		ggui.Watch(ctrl.Open, func(open bool) {
			if !open && !w.Closed() {
				w.Close()
			}
		})
		w.OnCloseRequest(func() bool {
			if ggui.Peek(ctrl.Phase) == PhaseInstalling {
				return false
			}
			ctrl.Dismiss()
			return true
		})
		return widget.Box(widget.Scroll(Panel(ctrl))).Fill(uitheme.Bg)
	})
	if err != nil {
		return nil, err
	}
	ctrl.Open.Set(true)
	return win, nil
}

func panelFrame(body, footer ggui.Widget) ggui.Widget {
	parts := []ggui.Widget{
		widget.Box(body).Pad(20, 24),
	}
	if footer != nil {
		parts = append(parts,
			ui.Divider(),
			widget.Box(footer).Pad(14, 24),
		)
	}
	return widget.Column(parts...).Align(widget.AlignStretch)
}

func iconTile(icon ggui.Widget) ggui.Widget {
	return widget.Box(widget.Center(icon)).
		Size(40, 40).
		Radius(10).
		Fill(uitheme.Muted)
}

func appDisplayName(ctrl *Controller) string {
	cfg := ctrl.updater.Config()
	if name := strings.TrimSpace(cfg.AppName); name != "" {
		return name
	}
	if id := strings.TrimSpace(cfg.AppID); id != "" {
		return id
	}
	return ctrl.tr("Application")
}

func buildIdlePanel(ctrl *Controller) ggui.Widget {
	appName := appDisplayName(ctrl)
	body := widget.Row(
		iconTile(ui.Icon(icons.Download).Size(20)),
		widget.Expanded(widget.Column(
			ui.Heading(ctrl.tr("Software Update")),
			ui.Caption(ctrl.tr("Check whether a newer version of %s is available.", appName)),
		).Space(0.5).Align(widget.AlignStretch)),
	).Space(1.5).Align(widget.AlignCenter)

	footer := widget.Row(
		widget.Spacer(),
		ui.Button(ctrl.tr("Close"), ctrl.Dismiss).Outline(),
		ui.Button(ctrl.tr("Check for Updates"), ctrl.CheckNow),
	).Space(1)

	return panelFrame(body, footer)
}

func buildCheckingPanel(ctrl *Controller) ggui.Widget {
	appName := appDisplayName(ctrl)
	body := widget.Row(
		iconTile(ui.Spinner().Size(20).Name("")),
		widget.Expanded(widget.Column(
			ui.Heading(ctrl.tr("Checking for updates…")),
			ui.Caption(ctrl.tr("Looking for the latest version of %s.", appName)),
		).Space(0.5).Align(widget.AlignStretch)),
	).Space(1.5).Align(widget.AlignCenter)

	footer := widget.Row(
		widget.Spacer(),
		ui.Button(ctrl.tr("Cancel"), ctrl.Dismiss).Outline(),
	).Space(1)

	return panelFrame(body, footer)
}

func buildUpToDatePanel(ctrl *Controller) ggui.Widget {
	cfg := ctrl.updater.Config()
	appName := appDisplayName(ctrl)
	curVer := strings.TrimSpace(cfg.CurrentVersion)
	desc := ctrl.tr("%s is currently the newest version available.", appName)
	if curVer != "" {
		desc = ctrl.tr("%s %s is currently the newest version available.", appName, curVer)
	}

	body := widget.Row(
		iconTile(ui.Icon(icons.Check).Size(20).Color(uitheme.Primary)),
		widget.Expanded(widget.Column(
			ui.Heading(ctrl.tr("You're up to date!")),
			ui.Caption(desc),
		).Space(0.5).Align(widget.AlignStretch)),
	).Space(1.5).Align(widget.AlignCenter)

	footer := widget.Row(
		widget.Spacer(),
		ui.Button(ctrl.tr("OK"), ctrl.Dismiss),
	).Space(1)

	return panelFrame(body, footer)
}

func buildAvailablePanel(ctrl *Controller, rel *Release) ggui.Widget {
	if rel == nil {
		return buildIdlePanel(ctrl)
	}
	cfg := ctrl.updater.Config()
	appName := appDisplayName(ctrl)
	curVer := strings.TrimSpace(cfg.CurrentVersion)
	newVer := rel.DisplayVersion()
	critical := rel.IsCriticalFor(curVer)
	ext := ctrl.externalUpdate(rel)

	titleParts := []ggui.Widget{
		ui.Heading(ctrl.tr("A new version of %s is available!", appName)),
	}
	if critical {
		titleParts = append(titleParts, ui.Badge(ctrl.tr("Critical")).Destructive())
	}

	var subtitle string
	if curVer != "" {
		subtitle = ctrl.tr("%s %s is now available — you have %s.", appName, newVer, curVer)
	} else {
		subtitle = ctrl.tr("%s %s is now available.", appName, newVer)
	}

	header := widget.Box(widget.Row(
		iconTile(ui.Icon(icons.Download).Size(20).Color(uitheme.Primary)),
		widget.Expanded(widget.Column(
			widget.Row(titleParts...).Space(1).Align(widget.AlignCenter),
			ui.Caption(subtitle),
		).Space(0.5).Align(widget.AlignStretch)),
	).Space(1.5).Align(widget.AlignStart)).Pad(0, 20, 0, 0)

	openLink := makeSafeLinkHandler(ctrl.linkHandler(), cfg.AllowInsecureHTTP)
	notes := strings.TrimSpace(rel.Item.ReleaseNotes)
	var notesWidget ggui.Widget
	if notes != "" {
		md := ui.Markdown(notes).OnLink(openLink)
		notesWidget = widget.Box(
			widget.Scroll(widget.Box(md).Pad(12)),
		).Height(180).Border(1, uitheme.Border).Radius(6).Fill(uitheme.Bg).Clip()
	} else {
		notesWidget = widget.Box(
			ui.Caption(ctrl.tr("No release notes were provided for this version.")),
		).Pad(12).Border(1, uitheme.Border).Radius(6).Fill(uitheme.Muted)
	}

	bodyItems := []ggui.Widget{
		header,
		notesWidget,
	}

	if ext != nil {
		if msg := strings.TrimSpace(ext.Message); msg != "" {
			bodyItems = append(bodyItems, ui.Caption(msg))
		}
		if cmd := strings.TrimSpace(ext.Command); cmd != "" {
			bodyItems = append(bodyItems, ui.CodeBlock(cmd).Small().CopyButton(true))
		}
	}

	if link := strings.TrimSpace(rel.Item.ReleaseNotesLink); link != "" {
		if parsed, err := validateURLString(link, cfg.AllowInsecureHTTP); err == nil {
			safeURL := parsed.String()
			bodyItems = append(bodyItems, widget.Row(
				ui.Button(ctrl.tr("View full release notes"), func() { openLink(safeURL) }).
					Link().
					Pad(2, 0),
				widget.Spacer(),
			))
		}
	}

	bodyItems = append(bodyItems,
		ui.Checkbox(ctrl.AutoCheck, ctrl.tr("Automatically check for updates")).
			OnChange(func(enabled bool) { ctrl.ToggleAutoCheck(enabled) }),
	)

	body := widget.Column(bodyItems...).Space(1.25).Align(widget.AlignStretch)

	var footerItems []ggui.Widget
	if !critical {
		footerItems = append(footerItems, ui.Button(ctrl.tr("Skip This Version"), ctrl.Skip).Outline())
	}
	footerItems = append(footerItems,
		widget.Spacer(),
		ui.Button(ctrl.tr("Remind Me Later"), ctrl.RemindLater).Outline(),
	)
	if ext != nil {
		label := strings.TrimSpace(ext.ActionLabel)
		if label == "" {
			if strings.TrimSpace(ext.Command) != "" {
				label = ctrl.tr("Copy command")
			} else {
				label = ctrl.tr("Open release page")
			}
		}
		onAction := ext.OnAction
		if onAction == nil {
			onAction = ctrl.Dismiss
		}
		footerItems = append(footerItems, ui.Button(label, onAction))
	} else {
		footerItems = append(footerItems, ui.Button(ctrl.tr("Install Update"), ctrl.Download))
	}
	footer := widget.Row(footerItems...).Space(1)

	return panelFrame(body, footer)
}

func buildDownloadingPanel(ctrl *Controller, rel *Release) ggui.Widget {
	appName := appDisplayName(ctrl)
	heading := ctrl.tr("Downloading %s…", appName)
	if rel != nil && rel.DisplayVersion() != "" {
		heading = ctrl.tr("Downloading %s %s…", appName, rel.DisplayVersion())
	}

	body := widget.Column(
		ui.Heading(heading),
		ui.Progress(ctrl.Progress).Height(8).ShowValue(),
		ui.CaptionOf(ctrl.ProgressText),
	).Space(1.25).Align(widget.AlignStretch)

	footer := widget.Row(
		widget.Spacer(),
		ui.Button(ctrl.tr("Cancel"), ctrl.CancelDownload).Outline(),
	).Space(1)

	return panelFrame(body, footer)
}

func buildReadyPanel(ctrl *Controller, rel *Release, upd *DownloadedUpdate) ggui.Widget {
	appName := appDisplayName(ctrl)
	ver := ""
	if rel != nil {
		ver = rel.DisplayVersion()
	} else if upd != nil {
		ver = upd.Release.DisplayVersion()
	}

	titleParts := []ggui.Widget{
		ui.Heading(ctrl.tr("Ready to Install")),
	}
	if upd != nil && upd.Verified {
		titleParts = append(titleParts, ui.Badge(ctrl.tr("Verified")).Secondary())
	}

	var desc string
	verified := upd != nil && upd.Verified
	switch {
	case ver != "" && verified:
		desc = ctrl.tr("%s %s has been downloaded and verified. Click below to install and relaunch.", appName, ver)
	case ver != "":
		desc = ctrl.tr("%s %s has been downloaded. Click below to install and relaunch.", appName, ver)
	case verified:
		desc = ctrl.tr("The update for %s has been downloaded and verified. Click below to install and relaunch.", appName)
	default:
		desc = ctrl.tr("The update for %s has been downloaded. Click below to install and relaunch.", appName)
	}

	body := widget.Row(
		iconTile(ui.Icon(icons.Check).Size(20).Color(uitheme.Primary)),
		widget.Expanded(widget.Column(
			widget.Row(titleParts...).Space(1).Align(widget.AlignCenter),
			ui.Caption(desc),
		).Space(0.5).Align(widget.AlignStretch)),
	).Space(1.5).Align(widget.AlignCenter)

	footer := widget.Row(
		widget.Spacer(),
		ui.Button(ctrl.tr("Later"), ctrl.Dismiss).Outline(),
		ui.Button(ctrl.tr("Install and Relaunch"), ctrl.Install),
	).Space(1)

	return panelFrame(body, footer)
}

func buildInstallingPanel(ctrl *Controller) ggui.Widget {
	appName := appDisplayName(ctrl)
	body := widget.Row(
		iconTile(ui.Spinner().Size(20).Name("")),
		widget.Expanded(widget.Column(
			ui.Heading(ctrl.tr("Installing update…")),
			ui.Caption(ctrl.tr("%s will restart shortly.", appName)),
		).Space(0.5).Align(widget.AlignStretch)),
	).Space(1.5).Align(widget.AlignCenter)

	return panelFrame(body, nil)
}

func buildErrorPanel(ctrl *Controller, errText string) ggui.Widget {
	if strings.TrimSpace(errText) == "" {
		errText = ctrl.tr("An unexpected error occurred while updating.")
	}
	body := widget.Column(
		ui.Alert(ctrl.tr("Update Failed"), errText).Destructive().Icon(icons.Alert),
	).Align(widget.AlignStretch)

	footer := widget.Row(
		widget.Spacer(),
		ui.Button(ctrl.tr("Close"), ctrl.Dismiss).Outline(),
		ui.Button(ctrl.tr("Retry"), ctrl.Retry),
	).Space(1)

	return panelFrame(body, footer)
}

func safeLinkURL(raw string, allowInsecureHTTP, allowMailto bool) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		if u.Host == "" {
			return "", false
		}
		return u.String(), true
	case "http":
		if !allowInsecureHTTP || u.Host == "" {
			return "", false
		}
		return u.String(), true
	case "mailto":
		if !allowMailto || u.Opaque == "" || u.Host != "" || strings.HasPrefix(u.Opaque, "-") {
			return "", false
		}
		return u.String(), true
	default:
		return "", false
	}
}

func makeSafeLinkHandler(onLink func(string), allowInsecureHTTP bool) func(string) {
	return func(raw string) {
		safe, ok := safeLinkURL(raw, allowInsecureHTTP, onLink != nil)
		if !ok {
			return
		}
		if onLink != nil {
			onLink(safe)
			return
		}
		_ = runtime.OpenURL(safe)
	}
}
