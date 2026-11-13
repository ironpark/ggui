package main

import (
	"image/color"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
)

// newPartPreviews shows the parts shadcn/ui composes its components from:
// card sections, field sets, menu rows, input-group addons, tooltip sides
// and the small alert dialog. Their state outlives a gallery rebuild.
func newPartPreviews(toasts *ui.ToasterWidget) func() []ggui.Widget {
	project, framework := ggui.State(""), ggui.State("")
	timezone := ggui.State("")
	marketing, security, compact := ggui.State(true), ggui.State(false), ggui.State(true)
	terms := ggui.State(false)
	tier := ggui.State("")
	shipping := ggui.State("none")
	alerts := ggui.State(false)
	compute, digest, password := ggui.State("kubernetes"), ggui.State(false), ggui.State("")
	showStatus, showPanel := ggui.State(true), ggui.State(false)
	sortBy := ggui.State("Newest")
	prompt, url, amount := ggui.State(""), ggui.State(""), ggui.State("")
	allowOpen, allowed := ggui.State(false), ggui.State("Not asked yet")
	frameworks := []string{"Next.js", "SvelteKit", "Nuxt", "Remix", "Astro"}
	zones := []string{"Eastern", "Central", "Pacific", "London", "Berlin", "Tokyo", "Seoul"}
	region := map[string]string{"Eastern": "North America", "Central": "North America", "Pacific": "North America", "London": "Europe", "Berlin": "Europe", "Tokyo": "Asia", "Seoul": "Asia"}
	allowDialog := ui.AlertDialog(allowOpen, "Allow notifications?", "The app will tell you when a build finishes.").
		Small().Media(ui.Icon(icons.Info)).Confirm("Allow", func() { allowed.Set("Notifications allowed") }).
		Cancel("Not now").OnCancel(func() { allowed.Set("Notifications declined") })
	menu := ui.Menu("Open menu",
		ui.MenuLabel("My account"),
		ui.MenuItem("Profile", nil).Icon(ui.Icon(icons.File)).Shortcut("cmd+p"),
		ui.MenuItem("Downloads", nil).Icon(ui.Icon(icons.Download)),
		ui.MenuSub("Invite users",
			ui.MenuItem("By email", func() { toasts.Push(ui.Toast("Invitation sent", "An email is on its way.").Success()) }),
			ui.MenuItem("By link", func() { toasts.Push(ui.Toast("Link copied", "Paste it anywhere.").Info()) }),
		).Icon(ui.Icon(icons.Plus)),
		ui.MenuDivider(),
		ui.MenuLabel("Appearance").Inset(),
		ui.MenuCheckbox("Status bar", showStatus).Inset(),
		ui.MenuCheckbox("Activity panel", showPanel).Inset(),
		ui.MenuDivider(),
		ui.MenuLabel("Sort by").Inset(),
		ui.MenuRadioGroup(sortBy, "Newest", "Oldest", "Name").Inset(),
		ui.MenuDivider(),
		ui.MenuItem("Log out", nil).Icon(ui.Icon(icons.Close)).Destructive().Shortcut("cmd+shift+q"),
	)
	return func() []ggui.Widget {
		return []ggui.Widget{
			preview("Card", ggui.Column(
				ui.Card(
					ui.CardHeader("Create project", "Deploy your new project in one click.").
						Action(ui.Badge("New").Secondary()),
					ui.CardContent(ui.FieldGroup(
						ui.Field("Name", ui.TextField(project).Placeholder("Name of your project")),
						ui.Field("Framework", ui.Select(framework).Options(frameworks).Placeholder("Select a framework")),
					)),
					ui.CardFooter(ui.Button("Cancel", func() { project.Set("") }).Outline(), ui.Button("Deploy", func() {
						toasts.Push(ui.Toast("Deploying", "Your project is on its way.").Success())
					})).Justify(ggui.SpaceBetween),
				),
				ui.Card(
					ui.CardHeader("Small card", "Tighter padding for dense layouts."),
					ui.CardContent(ui.Caption("The header, the content and the footer keep their spacing at either size.")),
				).Small(),
			).Gap(12).Stretch()),

			preview("Form layout", ui.FieldSet(
				ui.FieldGroup(
					ui.Field("Time zone", ui.Select(timezone).Options(zones).Placeholder("Select a time zone").
						GroupBy(func(z string) string { return region[z] }).
						DisabledOption(func(z string) bool { return z == "Berlin" })).
						Help("Grouped by region; Berlin is unavailable."),
					ui.FieldSeparator().Label("Notifications"),
					ui.Field("Marketing emails", ui.Switch(marketing, "")).Help("News about products and features.").Horizontal(),
					ui.Field("Security alerts", ui.Switch(security, "")).Help("Sign-ins from a new device.").Horizontal(),
					ui.Field("Compact rows", ui.Switch(compact, "").Small()).Horizontal(),
					ui.FieldSeparator(),
					ui.Field("Accept the terms", ui.Checkbox(terms, "")).Help("A checkbox before its label.").ControlFirst(),
				),
			).Legend("Preferences").Description("How the workspace reaches you.")),

			preview("Validation states", ggui.Column(
				ui.Checkbox(terms, "I accept the terms").BindInvalid(ggui.Not(terms)),
				ui.Field("Tier", ui.Radios(tier).Options([]string{"Starter", "Team", "Enterprise"}).Horizontal()).BindError(tier.Map(func(v string) string {
					if v == "" {
						return "Choose a tier."
					}
					return ""
				})),
				ui.Switch(alerts, "Required alerts").BindInvalid(ggui.Not(alerts)),
				ui.Field("Shipping", ui.Select(shipping).Options([]string{"none", "Standard", "Express"}).Format(func(v string) string {
					if v == "none" {
						return "Choose a method"
					}
					return v
				}).DisabledOption(func(v string) bool { return v == "none" })).BindError(shipping.Map(func(v string) string {
					if v == "none" {
						return "Choose how to ship the order."
					}
					return ""
				})),
				ui.Caption("Each control turns destructive until it holds a valid answer."),
			).Gap(12).Stretch()),

			preview("Choice cards", ggui.Column(
				ui.Field("Kubernetes", ui.Radio(compute, "kubernetes", "")).Help("Run GPU workloads on a K8s cluster.").Card(),
				ui.Field("Virtual machine", ui.Radio(compute, "vm", "")).Help("Access a cluster to run GPU workloads.").Card(),
				ui.FieldCard(ui.Checkbox(digest, ""), ui.FieldContent(
					ui.FieldTitle("Weekly digest"),
					ui.FieldDescription("A summary of activity, every Monday."),
				)).ControlFirst(),
				ui.Field("Airplane mode", ui.Switch(ggui.State(false), "").Disabled(true)).Card(),
				ui.Field("Password", ui.TextField(password).Placeholder("New password")).BindErrors(password.Map(passwordErrors)),
			).Gap(12).Stretch()),

			preview("Dropdown menu", ggui.Column(
				menu,
				ui.Captionf("Sorted by %s", sortBy),
			).Gap(8)),

			preview("Input group", ggui.Column(
				ui.InputGroup(ggui.TextInput(url).Placeholder("example.com").Name("Website")).
					Leading(ui.Icon(icons.Search)).
					Trailing(ui.Badge("12 results").Secondary()),
				ui.InputGroup(ggui.TextInput(amount).Placeholder("0.00").Name("Amount")).
					Leading(ggui.Text("$")).Trailing(ggui.Text("USD")),
				ui.InputGroup(ggui.TextInput(prompt).Placeholder("Ask, search or make anything…").Name("Prompt")).Multiline().
					Top(ui.Caption("Composer")).
					Bottom(ggui.Row(
						ui.ButtonOf(ui.Icon(icons.Plus), nil).Name("Attach").Size(ui.ButtonIconExtraSmall).Outline(),
						ggui.Spacer(),
						ui.Caption("52% used"),
						ui.Button("Send", func() { prompt.Set("") }).Size(ui.ButtonExtraSmall),
					).Gap(8)),
			).Gap(12).Stretch()),

			preview("Tooltip", ggui.Column(
				ggui.Wrap(
					ui.Tooltip(ui.Button("Top", nil).Outline(), "Opens above").Side(ggui.PopupTop),
					ui.Tooltip(ui.Button("Right", nil).Outline(), "Opens to the right").Side(ggui.PopupRight),
					ui.Tooltip(ui.Button("Bottom", nil).Outline(), "Opens below").Side(ggui.PopupBottom),
					ui.Tooltip(ui.Button("Left", nil).Outline(), "Opens to the left").Side(ggui.PopupLeft),
				).Gap(8),
				ui.Tooltip(ui.ButtonOf(ui.Icon(icons.Undo), nil).Name("Undo").Size(ui.ButtonIcon).Outline(), "Undo").Shortcut("cmd+z"),
			).Gap(12)),

			preview("Alert dialog", ggui.Column(
				ui.Button("Allow notifications…", func() { allowOpen.Set(true) }).Outline(),
				ui.CaptionOf(allowed),
				allowDialog,
			).Gap(8)),
		}
	}
}

// roundFrame clips child to a rounded rectangle and outlines it, so a
// child that fills its corners, such as a sidebar, keeps them round.
func roundFrame(child ggui.Widget, radius float64, border color.Color) ggui.Widget {
	return &roundFrameWidget{child: child, radius: radius, border: border}
}

type roundFrameWidget struct {
	child  ggui.Widget
	radius float64
	border color.Color
}

func (w *roundFrameWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return w.child.Layout(c, env)
}

func (w *roundFrameWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.ClipRoundRect(r, w.radius, func(dst *ggui.Canvas) { dst.Paint(w.child, r) })
	dst.StrokeRoundRect(r, w.radius, 1, w.border)
}

// colorChip paints c over the page background bg, as a control paints its
// surface: a translucent token such as Input shows as the tint it is.
func colorChip(c, bg, border color.Color, radius float64) ggui.Widget {
	return &colorChipWidget{c: c, bg: bg, border: border, radius: radius}
}

type colorChipWidget struct {
	c, bg, border color.Color
	radius        float64
}

func (w *colorChipWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return c.Constrain(ggui.Sz(c.MaxW, 28))
}

func (w *colorChipWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.FillRoundRect(r, w.radius, w.bg)
	dst.FillRoundRect(r, w.radius, w.c)
	dst.StrokeRoundRect(r, w.radius, 1, w.border)
}

// passwordErrors is every rule a new password breaks.
func passwordErrors(v string) []string {
	var out []string
	if len(v) < 8 {
		out = append(out, "Use at least 8 characters.")
	}
	if !strings.ContainsAny(v, "0123456789") {
		out = append(out, "Add a number.")
	}
	return out
}
