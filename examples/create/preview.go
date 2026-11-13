package main

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// previewModel owns sample data and controls independently of the theme editor.
// build returns the same widget tree, so changing the surrounding theme never
// resets form drafts, selections, focus or the preview's scroll position.
type previewModel struct {
	view                                            ggui.Widget
	cards                                           []ggui.Widget
	scroll, pan                                     ggui.ScrollController
	threshold                                       *ggui.StateValue[float64]
	payoutCurrency, notes, payoutFeedback           *ggui.StateValue[string]
	emailUpdates, compactView                       *ggui.StateValue[bool]
	preferredCurrency, preferencesFeedback          *ggui.StateValue[string]
	retirement, property, balance                   *ggui.StateValue[float64]
	savingsFeedback, balanceFeedback, chartFeedback *ggui.StateValue[string]
	securityAlerts, productUpdates, weeklySummary   *ggui.StateValue[bool]
	notificationsFeedback                           *ggui.StateValue[string]
	selectedDate                                    *ggui.StateValue[time.Time]
	showAllTransactions                             *ggui.StateValue[bool]
	feedbackOpen, savingsOpen, releaseOpen          *ggui.StateValue[bool]
	feedback, releaseName, releaseDraft             *ggui.StateValue[string]
}

func newPreview() *previewModel {
	p := &previewModel{
		threshold: ggui.State(2500.0), payoutCurrency: ggui.State("USD"), notes: ggui.State(""),
		payoutFeedback: ggui.State(""),
		emailUpdates:   ggui.State(true), compactView: ggui.State(false), preferredCurrency: ggui.State("USD"), preferencesFeedback: ggui.State(""),
		retirement: ggui.State(273000.0), property: ggui.State(27200.0), balance: ggui.State(0.0),
		savingsFeedback: ggui.State(""), balanceFeedback: ggui.State(""), chartFeedback: ggui.State(""),
		securityAlerts: ggui.State(true), productUpdates: ggui.State(false), weeklySummary: ggui.State(true), notificationsFeedback: ggui.State(""),
		selectedDate: ggui.State(time.Date(2026, time.June, 18, 0, 0, 0, 0, time.Local)), showAllTransactions: ggui.State(false),
		feedbackOpen: ggui.State(false), savingsOpen: ggui.State(false), releaseOpen: ggui.State(false),
		feedback: ggui.State(""), releaseName: ggui.State(""), releaseDraft: ggui.State(""),
	}
	p.cards = []ggui.Widget{
		p.contributionsCard(), p.payoutCard(), p.savingsCard(),
		p.distributeCard(), p.balanceCard(), p.transactionsCard(),
		p.preferencesCard(), p.calendarCard(), p.notificationsCard(),
	}
	grid := func(columns int, pad float64) ggui.Widget {
		// Independent stacks keep the dense collage from growing empty bands
		// below short cards. The same cards survive every breakpoint.
		stacks := make([]ggui.Widget, columns)
		for column := range columns {
			var cards []ggui.Widget
			for i := column; i < len(p.cards); i += columns {
				cards = append(cards, p.cards[i])
			}
			stacks[column] = ggui.Expanded(ggui.Column(cards...).Gap(32).Stretch())
		}
		gap := 32.0
		if columns == 3 {
			gap = 40
		}
		return ggui.Padding(ggui.Row(stacks...).Gap(gap).Align(ggui.AlignStart), pad)
	}
	scroller := ggui.Scroll(ggui.Responsive(1000, grid(3, 40), ggui.Responsive(650, grid(2, 24), grid(1, 16)))).
		Controller(&p.scroll).Key(p.key("preview-scroll"))
	wide := ggui.Scroll(ggui.Box(scroller).Width(1288)).Horizontal().Controller(&p.pan).Key(p.key("preview-pan"))
	viewport := ggui.Responsive(1000, wide, scroller)
	p.view = ggui.Stack(viewport, p.feedbackDialog(), p.savingsDialog(), p.releaseDialog())
	return p
}

func (p *previewModel) build() ggui.Widget { return p.view }

func (p *previewModel) key(name string) any {
	return struct {
		model *previewModel
		name  string
	}{p, name}
}

func previewHeader(title, description string) ggui.Widget {
	return ggui.Column(ui.Heading(title).Size(16).Weight(ggui.WeightMedium), previewDescription(description)).Gap(5).Stretch()
}

func previewDescription(text string) *ggui.TextWidget {
	return ggui.Text(text).Size(14).LineHeight(1.5).Color(uitheme.MutedFg)
}

func previewCard(title, description string, body ggui.Widget) ggui.Widget {
	return ui.Card(ggui.Column(previewHeader(title, description), body).Gap(24).Stretch()).Pad(16)
}

// Both colors stay live when the surrounding theme changes.
var previewMuted = uitheme.Mix(uitheme.Card, uitheme.Muted, .6)

func previewFooterCard(header, body, footer ggui.Widget) ggui.Widget {
	content := ggui.Box(ggui.Column(header, body).Gap(20).Stretch()).Pad(16).Fill(uitheme.Card)
	card := ui.Card(ggui.Column(content, ui.Divider(), ggui.Padding(ggui.Column(footer).Stretch(), 16)).Gap(0).Stretch()).Fill(previewMuted).Pad(0)
	var radius, borderWidth float64
	var border color.Color
	clippedCard := ggui.FromFuncs(func(cs ggui.Constraints, env ggui.Env) ggui.Size {
		t := uitheme.Of(env)
		radius, borderWidth, border = t.RadiusLg, t.BorderWidth, uitheme.Fade(t.Fg, .1)
		return card.Layout(cs, env)
	}, func(dst *ggui.Canvas, r ggui.Rect) {
		dst.ClipRoundRect(r, radius, func(clipped *ggui.Canvas) {
			clipped.Paint(card, r)
			clipped.StrokeRoundRect(r, radius, borderWidth, border)
		})
	})
	// A dark theme's Input is translucent, so the forms sit on a slightly
	// raised surface over the card without a token of their own.
	return clippedCard
}

func previewTile(content ggui.Widget) ggui.Widget {
	return ggui.Box(content).Pad(12).Fill(previewMuted).Radius(8)
}

func previewFeedback(value ggui.Readable[string]) ggui.Widget {
	return ui.CaptionOf(value).Role(ggui.RoleStatus)
}

func (p *previewModel) announce(value *ggui.StateValue[string], message string) {
	value.Set(message)
	p.feedback.Set(message)
	p.feedbackOpen.Set(true)
}

func (p *previewModel) feedbackDialog() ggui.Widget {
	return ui.Dialog(p.feedbackOpen, ggui.Column(
		ggui.TextOf(p.feedback).Role(ggui.RoleStatus).Size(14),
		ui.Button("Done", func() { p.feedbackOpen.Set(false) }).Key(p.key("feedback-done")),
	).Gap(20).Stretch()).Title("Preview update").Width(420)
}

func previewMoney(currency string, value float64) string {
	symbol := map[string]string{"USD": "$", "EUR": "€", "GBP": "£"}[currency]
	if symbol == "" {
		symbol = currency + " "
	}
	n := fmt.Sprintf("%.2f", value)
	parts := strings.SplitN(n, ".", 2)
	whole := parts[0]
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "," + whole[i:]
	}
	return symbol + whole + "." + parts[1]
}

func (p *previewModel) contributionsCard() ggui.Widget {
	months := []string{"Dec", "Jan", "Feb", "Mar", "Apr", "May"}
	values := []float64{1700, 2350, 1925, 2775, 1600, 3000}
	data := make([]ui.ChartDatum, len(months))
	for i, month := range months {
		data[i] = ui.ChartDatum{Label: month, Values: map[string]float64{"contributions": values[i]}}
	}
	chart := ui.BarChart(data, ui.ChartConfig{{Key: "contributions", Label: "Contributions", ColorIndex: 2}}).
		Name("Monthly contributions").Key(p.key("contributions-chart")).Height(172).Grid(false).Axes(false, false).CornerRadius(5).
		Tooltip(ui.ChartTooltipOptions{FormatValue: func(value float64, _ ui.ChartSeries, _ ui.ChartDatum) string { return previewMoney("USD", value) }}).
		OnSelect(func(_ int, datum ui.ChartDatum) {
			p.announce(p.chartFeedback, datum.Label+" contribution: "+previewMoney("USD", datum.Values["contributions"]))
		})
	labels := make([]ggui.Widget, len(months))
	for i, month := range months {
		labels[i] = ggui.Expanded(ggui.Center(ui.Caption(month).Size(12)))
	}
	chartWithLabels := ggui.Column(chart, ggui.Padding(ggui.Row(labels...), 0, 12)).Gap(0).Stretch()
	tile := func(label, title, description string) ggui.Widget {
		return previewTile(ggui.Column(ui.Caption(label).Size(12), ggui.Text(title).Size(16).Weight(ggui.WeightMedium), ui.Caption(description).Size(14)).Gap(8).Stretch())
	}
	return previewFooterCard(previewHeader("Contribution History", "Last 6 months of activity"),
		ggui.Column(chartWithLabels, ggui.Row(ggui.Expanded(tile("UPCOMING", "May 25, 2024", "$1,000 scheduled")), ggui.Expanded(tile("AUTO-SAVE PLAN", "Accelerated", "Recurring weekly"))).Gap(12)).Gap(24).Stretch(),
		ui.Button("View Full Report", func() {
			p.announce(p.chartFeedback, "December–May contributions: "+previewMoney("USD", 13350)+". Highest month: May, $3,000.00.")
		}).Key(p.key("full-report")))
}

func (p *previewModel) savingsCard() ggui.Widget {
	goal := func(name string, value *ggui.StateValue[float64], target float64) ggui.Widget {
		return previewTile(ggui.Column(
			ui.Caption(name).Size(12),
			ggui.Text(strings.TrimSuffix(previewMoney("USD", target), ".00")).Size(28).Weight(ggui.WeightMedium),
			ui.Progress(ggui.Map(value, func(v float64) float64 { return v / target })).Height(4),
			ggui.Row(ggui.Expanded(ui.CaptionOf(ggui.Map(value, func(v float64) string { return fmt.Sprintf("%.0f%% achieved", v/target*100) })).Size(14)),
				ui.CaptionOf(ggui.Map(value, func(v float64) string { return strings.TrimSuffix(previewMoney("USD", v), ".00") })).Size(14)).Gap(8),
		).Gap(14).Stretch())
	}
	header := ggui.Row(ggui.Expanded(previewHeader("Savings Targets", "Active milestones for 2024")),
		ui.Button("Manage", func() { p.savingsOpen.Set(true) }).Name("Manage savings targets").Key(p.key("manage-savings")).Outline().Size(ui.ButtonSmall)).Gap(8).Align(ggui.AlignStart)
	return previewFooterCard(header, ggui.Column(goal("RETIREMENT", p.retirement, 420000), goal("REAL ESTATE", p.property, 85000)).Gap(16).Stretch(),
		ggui.TextOf(ggui.Combine(p.retirement, p.property, func(retirement, property float64) string {
			if retirement >= 420000 && property >= 85000 {
				return "You have met your targets for this year."
			}
			return "You have not met your targets for this year."
		})).Size(14).LineHeight(1.5).Color(uitheme.MutedFg))
}

func (p *previewModel) savingsDialog() ggui.Widget {
	add := func(name string, value *ggui.StateValue[float64], target float64) ggui.Widget {
		return ui.Button("Add $1,000 to "+name, func() {
			next := min(target, ggui.Peek(value)+1000)
			value.Set(next)
			p.savingsFeedback.Set(name + ": " + previewMoney("USD", next) + " saved in this preview.")
		}).Key(p.key("save-" + name)).Outline()
	}
	return ui.Dialog(p.savingsOpen, ggui.Column(previewDescription("Explore contributions with local sample data. No money is moved."),
		add("Retirement", p.retirement, 420000), add("Real Estate", p.property, 85000), previewFeedback(p.savingsFeedback),
		ui.Button("Close savings", func() { p.savingsOpen.Set(false) }).Key(p.key("close-savings")),
	).Gap(16).Stretch()).Title("Manage savings targets").Width(420)
}

func previewCurrency(code string) string {
	return map[string]string{"USD": "USD — United States Dollar", "EUR": "EUR — Euro", "GBP": "GBP — British Pound"}[code]
}

func (p *previewModel) payoutCard() ggui.Widget {
	currency := ui.Select(p.payoutCurrency).Options([]string{"USD", "EUR", "GBP"}).Format(previewCurrency).Name("Payout currency").Key(p.key("payout-currency"))
	slider := ui.Slider(p.threshold, 50, 10000).Step(25).LargeStep(500).Name("Payout amount").Key(p.key("payout-amount"))
	amount := ggui.Combine(p.payoutCurrency, p.threshold, func(currency string, value float64) string {
		return strings.ReplaceAll(previewMoney(currency, value), ",", "")
	})
	// Stack the label and value on small cards; neither should squeeze the form.
	amountLabel := ggui.Text("Minimum Payout Amount").Size(14).Weight(ggui.WeightMedium)
	amountValue := ggui.TextOf(amount).Size(24).Weight(ggui.WeightMedium)
	amountRow := ggui.Responsive(330, ggui.Row(amountLabel, ggui.Spacer(), amountValue).Gap(8).Align(ggui.AlignCenter), ggui.Column(amountLabel, amountValue).Gap(8))
	return previewFooterCard(previewHeader("Payout Threshold", "Set the minimum balance required before a payout is triggered."), ggui.Column(
		ui.Field("Preferred Currency", currency),
		ggui.Column(amountRow, slider, ggui.Row(ggui.Expanded(ui.Caption("$50 (MIN)").Size(12)), ui.Caption("$10,000 (MAX)").Size(12))).Gap(12).Stretch(),
		ui.Field("Notes", ui.TextField(p.notes).Name("Payout notes").Key(p.key("payout-notes")).Lines(4).MinWidth(0).Placeholder("Add any notes for this payout configuration…")),
	).Gap(24).Stretch(), ui.Button("Save Threshold", func() {
		message := "Threshold saved: " + previewMoney(ggui.Peek(p.payoutCurrency), ggui.Peek(p.threshold)) + "."
		if strings.TrimSpace(ggui.Peek(p.notes)) != "" {
			message += " Note included."
		}
		p.announce(p.payoutFeedback, message)
	}).Key(p.key("save-threshold")))
}

func (p *previewModel) distributeCard() ggui.Widget {
	return ui.Card(ggui.Column(
		ggui.Center(ggui.Box(ui.Icon(icons.Plus).Size(20)).Pad(12).Fill(previewMuted).Radius(8)),
		ui.Heading("Distribute Track").Size(16).Weight(ggui.WeightMedium).Align(.5),
		previewDescription("Upload your first master to start reaching listeners on Spotify, Apple Music, and more.").Align(.5),
		ggui.Center(ui.Button("Create Release", func() { p.releaseOpen.Set(true) }).Key(p.key("create-release"))),
	).Gap(16).Stretch()).Pad(32, 24)
}

func (p *previewModel) releaseDialog() ggui.Widget {
	return ui.Dialog(p.releaseOpen, ggui.Column(
		previewDescription("Create a local release draft. Distribution and file uploads are not connected in this preview."),
		ui.Field("Release title", ui.TextField(p.releaseName).Name("Release title").MinWidth(0).Key(p.key("release-title"))),
		ui.Button("Save release draft", func() {
			name := strings.TrimSpace(ggui.Peek(p.releaseName))
			if name == "" {
				p.releaseDraft.Set("Enter a release title to save your draft.")
				return
			}
			p.releaseDraft.Set("Draft saved locally: " + name + ".")
		}).Key(p.key("save-release")), previewFeedback(p.releaseDraft),
		ui.Button("Close release", func() { p.releaseOpen.Set(false) }).Outline().Key(p.key("close-release")),
	).Gap(16).Stretch()).Title("Create Release").Width(420)
}

func (p *previewModel) preferencesCard() ggui.Widget {
	switchRow := func(title, description string, state *ggui.StateValue[bool]) ggui.Widget {
		control := ui.Switch(state, "").Key(p.key(title))
		control.SetName(title)
		return ggui.Row(ggui.Expanded(ggui.Column(ggui.Text(title).Weight(ggui.WeightMedium), ui.Caption(description)).Gap(4)), control).Gap(12).Align(ggui.AlignCenter)
	}
	return previewCard("Preferences", "A few details, just the way you like them.", ggui.Column(
		switchRow("Email updates", "Receive account activity by email.", p.emailUpdates),
		switchRow("Compact view", "Use a denser transaction list.", p.compactView),
		ui.Field("Preferred currency", ui.Select(p.preferredCurrency).Options([]string{"USD", "EUR", "GBP"}).Name("Preferred currency").Key(p.key("preferred-currency"))),
		ggui.Wrap(
			ui.Button("Save preferences", func() { p.announce(p.preferencesFeedback, "Preferences saved in this preview.") }).Key(p.key("save-preferences")),
			ui.Button("Reset", func() {
				p.emailUpdates.Set(true)
				p.compactView.Set(false)
				p.preferredCurrency.Set("USD")
				p.announce(p.preferencesFeedback, "Default preferences restored.")
			}).Name("Reset preferences").Key(p.key("reset-preferences")).Outline(),
		).Gap(8),
	).Gap(18).Stretch())
}

func (p *previewModel) transactionsCard() ggui.Widget {
	// Small decorative merchant icons are local monochrome artwork, rendered
	// through the same tinting and scaling infrastructure as the control icons.
	rows := []struct{ name, note, amount, path string }{
		{"Blue Bottle Coffee", "Food & Drink", "−$6.50", `<path d="M4 8h12v8a4 4 0 0 1-4 4H8a4 4 0 0 1-4-4zM16 9h2a3 3 0 0 1 0 6h-2M7 3v2m4-2v2m4-2v2"/>`},
		{"Whole Foods Market", "Groceries", "−$84.32", `<path d="M3 3h2l3 13h11l2-9H6M9 20h.01M18 20h.01"/>`},
		{"Stripe Payout", "Income", "+$1,250.00", `<rect x="3" y="6" width="18" height="14" rx="2"/><path d="M3 9V5l14-2v3m0 7h4m-4 0v3h4"/>`},
		{"Uber Technologies", "Transport", "−$24.00", `<path d="m5 9 2-5h10l2 5M3 10h18v9H3zM6 14h2m8 0h2M5 19v2m14-2v2"/>`},
		{"Netflix Subscription", "Entertainment", "−$15.99", `<rect x="3" y="7" width="18" height="14" rx="2"/><path d="m8 2 4 5 4-5"/>`},
		{"Figma", "Software", "−$15.00", `<path d="M8 3h8v18H8zM8 9h8M8 15h8"/>`},
		{"Apple Music", "Entertainment", "−$10.99", `<path d="M9 18V5l11-2v13M9 8l11-2"/><circle cx="6" cy="18" r="3"/><circle cx="17" cy="16" r="3"/>`},
	}
	widgets := make([]ggui.Widget, 0, len(rows)*2)
	for i, row := range rows {
		asset, err := icons.Parse([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">` + row.path + `</svg>`))
		if err != nil {
			panic(err)
		} // All paths above are fixed, trusted source assets.
		item := ggui.Row(ggui.Box(icons.New(asset).Size(20)).Pad(10).Fill(previewMuted).Radius(8),
			ggui.Expanded(ggui.Column(ggui.Text(row.name).Size(14).Weight(ggui.WeightMedium).NoWrap().Ellipsis(), ui.Caption(row.note).Size(14)).Gap(4)),
			ggui.Text(row.amount).Size(13).NoWrap().Weight(ggui.WeightMedium),
		).Gap(12).Align(ggui.AlignCenter)
		if i > 0 && i != 5 {
			widgets = append(widgets, ui.Divider())
		}
		widgets = append(widgets, item)
	}
	list := ggui.Column(widgets[:9]...).Stretch()
	density := ggui.FromFuncs(func(cs ggui.Constraints, env ggui.Env) ggui.Size {
		gap := 16.0
		if p.compactView.Get() {
			gap = 8
		}
		return list.Gap(gap).Layout(cs, env)
	}, func(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(list, r) })
	more := ggui.Column(widgets[9:]...).Gap(16).Stretch()
	return previewCard("Recent Transactions", "Your latest account activity.", ggui.Column(
		density, ggui.If(p.showAllTransactions, func() ggui.Widget { return more }),
		ui.ButtonOf(ggui.TextOf(ggui.Map(p.showAllTransactions, func(all bool) string {
			if all {
				return "Show fewer transactions"
			}
			return "View all transactions"
		})), func() {
			p.showAllTransactions.Set(!ggui.Peek(p.showAllTransactions))
		}).BindName(ggui.Map(p.showAllTransactions, func(all bool) string {
			if all {
				return "Show fewer transactions"
			}
			return "View all transactions"
		})).Key(p.key("transactions-toggle")).Outline(),
	).Gap(16).Stretch())
}

func (p *previewModel) calendarCard() ggui.Widget {
	return previewCard("Schedule a Payout", "Pick the day that works for you.", ggui.Column(
		ui.Calendar(p.selectedDate).Name("Payout calendar").Key(p.key("payout-calendar")).WeekStartsOn(time.Monday),
		ui.Divider(),
		previewFeedback(ggui.Map(p.selectedDate, func(date time.Time) string { return "Selected: " + date.Format("Monday, January 2") })),
	).Gap(12).Stretch())
}

func (p *previewModel) notificationsCard() ggui.Widget {
	return previewCard("Notifications", "Keep the important things in sight.", ggui.Column(
		ggui.Column(
			ui.Checkbox(p.securityAlerts, "Security alerts").Key(p.key("security-alerts")), ui.Caption("Sign-ins and changes to your account."),
		).Gap(5).Stretch(),
		ggui.Column(
			ui.Checkbox(p.productUpdates, "Product updates").Key(p.key("product-updates")), ui.Caption("New features and improvements."),
		).Gap(5).Stretch(),
		ggui.Column(
			ui.Checkbox(p.weeklySummary, "Weekly summary").Key(p.key("weekly-summary")), ui.Caption("A weekly snapshot of your activity."),
		).Gap(5).Stretch(),
		ui.Button("Save notifications", func() {
			count := 0
			for _, enabled := range []bool{ggui.Peek(p.securityAlerts), ggui.Peek(p.productUpdates), ggui.Peek(p.weeklySummary)} {
				if enabled {
					count++
				}
			}
			p.announce(p.notificationsFeedback, fmt.Sprintf("Saved: %d of 3 notifications enabled.", count))
		}).Key(p.key("save-notifications")),
	).Gap(16).Stretch())
}

func (p *previewModel) balanceCard() ggui.Widget {
	row := func(label, value string) ggui.Widget {
		return ggui.Row(ggui.Expanded(previewDescription(label)), ggui.Text(value).Size(14)).Gap(8)
	}
	return previewFooterCard(ggui.Column(
		ui.Heading("Claimable Balance").Size(14).Weight(ggui.WeightMedium).Color(uitheme.MutedFg),
		ggui.TextOf(ggui.Map(p.balance, func(v float64) string { return previewMoney("USD", v) })).Size(36).Weight(ggui.WeightMedium),
		ggui.Row(ggui.Box(ggui.Row(ggui.Box().Size(7, 7).Fill(color.NRGBA{R: 234, G: 179, B: 8, A: 255}).Radius(4), ggui.Text("Pending Setup").Size(12)).Gap(5).Align(ggui.AlignCenter)).Pad(2, 7).Border(1, uitheme.Border).Radius(20)),
	).Gap(12).Stretch(), previewTile(ggui.Column(row("Net Royalties", "$0.00"), row("Processing Fee", "−$0.00"), ui.Divider(), row("Total Ready to Claim", "$0.00 USD")).Gap(16).Stretch()),
		ggui.Column(previewDescription("Once your bank is connected, balances over $10.00 are automatically eligible for monthly distribution on the 15th of each month."),
			ui.Button("Connect Bank", func() {
				p.announce(p.balanceFeedback, "Bank connections are not available in this local preview. No account was connected and no funds were transferred.")
			}).Key(p.key("connect-bank")),
		).Gap(16).Stretch())
}
