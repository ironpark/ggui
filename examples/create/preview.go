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
	emailUpdates, publicStats                       *ggui.StateValue[bool]
	preferredCurrency, preferencesFeedback          *ggui.StateValue[string]
	retirement, property, balance                   *ggui.StateValue[float64]
	savingsFeedback, balanceFeedback, chartFeedback *ggui.StateValue[string]
	notificationsFeedback                           *ggui.StateValue[string]
	selectedDate                                    *ggui.StateValue[time.Time]
	showAllTransactions                             *ggui.StateValue[bool]
	feedbackOpen, savingsOpen, releaseOpen          *ggui.StateValue[bool]
	feedback, releaseName, releaseDraft             *ggui.StateValue[string]
}

func newPreview() *previewModel {
	p := &previewModel{
		threshold: ggui.State(3000.0), payoutCurrency: ggui.State("USD"), notes: ggui.State(""),
		payoutFeedback: ggui.State(""),
		emailUpdates:   ggui.State(true), publicStats: ggui.State(true), preferredCurrency: ggui.State("USD"), preferencesFeedback: ggui.State(""),
		retirement: ggui.State(273000.0), property: ggui.State(27200.0), balance: ggui.State(0.0),
		savingsFeedback: ggui.State(""), balanceFeedback: ggui.State(""), chartFeedback: ggui.State(""),
		notificationsFeedback: ggui.State(""),
		selectedDate:          ggui.State(time.Date(2026, time.June, 18, 0, 0, 0, 0, time.Local)), showAllTransactions: ggui.State(false),
		feedbackOpen: ggui.State(false), savingsOpen: ggui.State(false), releaseOpen: ggui.State(false),
		feedback: ggui.State(""), releaseName: ggui.State(""), releaseDraft: ggui.State(""),
	}
	contributions, payout, savings, buy := p.contributionsCard(), p.payoutCard(), p.savingsCard(), p.buyCard()
	distribute, balance, transactions := p.distributeCard(), p.balanceCard(), p.transactionsCard()
	scan, preferences, nav, faq := p.scanCard(), p.preferencesCard(), p.navCards(), p.faqCard()
	dividends, goal, shortcuts, door := p.dividendsCard(), p.goalCard(), p.shortcutsCard(), p.doorCard()
	kitchen, holdings, access, summary := p.kitchenCard(), p.holdingsCard(), p.accessCard(), p.summaryCards()
	transfer, method, power, stock := p.transferCard(), p.methodCard(), p.powerCard(), p.stockCard()
	catalog, bank, milestone, cover := p.catalogCard(), p.bankCard(), p.milestoneCard(), p.coverCard()
	upcoming, social, averaging, syncing := p.upcomingCard(), p.socialCard(), p.averagingCard(), p.syncingCard()
	shades, notifications, loading := p.shadesCard(), p.notificationsCard(), p.loadingCard()
	// The narrow layouts deal these out in order, so the first cards are the
	// top of the collage's first columns.
	p.cards = []ggui.Widget{
		contributions, payout, savings, buy, distribute, balance, transactions,
		scan, preferences, nav, faq, dividends, goal, shortcuts, door,
		kitchen, holdings, access, summary, transfer, method, power, stock,
		catalog, bank, milestone, cover, upcoming, social, averaging, syncing,
		shades, notifications, loading,
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
		return ggui.Padding(ggui.Row(stacks...).Gap(32).Align(ggui.AlignStart), pad)
	}
	// The desktop collage is the reference's: five single columns around a
	// double one, whose wide cards span it and whose others pair up.
	stack := func(cards ...ggui.Widget) ggui.Widget { return ggui.Column(cards...).Gap(previewGap).Stretch() }
	pair := func(a, b ggui.Widget) ggui.Widget {
		return ggui.Row(ggui.Expanded(a), ggui.Expanded(b)).Gap(previewGap).Align(ggui.AlignStart)
	}
	single := func(cards ...ggui.Widget) ggui.Widget { return ggui.Box(stack(cards...)).Width(previewColumn) }
	collage := ggui.Padding(ggui.Row(
		single(contributions, distribute, scan, dividends, averaging, syncing),
		single(payout, balance, preferences, goal, kitchen),
		ggui.Box(stack(pair(savings, buy), transactions, pair(stack(nav, faq), stack(shortcuts, door)), holdings)).Width(2*previewColumn+previewGutter),
		single(access, summary, transfer, cover, loading),
		single(method, power, bank, upcoming, shades),
		single(stock, catalog, milestone, social, notifications),
	).Gap(previewGutter).Align(ggui.AlignStart), previewMargin)
	narrow := ggui.Scroll(ggui.Responsive(650, grid(2, 24), grid(1, 16))).Controller(&p.scroll).Key(p.key("preview-scroll"))
	desktop := ggui.Scroll(ggui.Box(collage).Width(previewWidth)).Controller(&p.scroll).Key(p.key("preview-scroll"))
	wide := ggui.Scroll(desktop).Horizontal().Controller(&p.pan).Key(p.key("preview-pan"))
	viewport := ggui.Responsive(1000, wide, narrow)
	p.view = ggui.Stack(viewport, p.feedbackDialog(), p.savingsDialog(), p.releaseDialog())
	return p
}

func (p *previewModel) build() ggui.Widget { return p.view }

// The desktop collage's measures, the reference's: 375 pixel columns 48
// apart, cards 40 apart, inside a 44 pixel margin.
const (
	previewColumn = 375.0
	previewGutter = 48.0
	previewGap    = 40.0
	previewMargin = 44.0
	previewWidth  = 2*previewMargin + 7*previewColumn + 6*previewGutter
)

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

// previewHeaderAction is a header with a control at its top right corner.
func previewHeaderAction(title, description string, action ggui.Widget) ggui.Widget {
	return ggui.Row(ggui.Expanded(previewHeader(title, description)), action).Gap(8).Align(ggui.AlignStart)
}

func previewLabel(text string) *ggui.TextWidget {
	return ggui.Text(text).Size(14).Weight(ggui.WeightMedium)
}

func previewCard(title, description string, body ggui.Widget) ggui.Widget {
	return previewPlainCard(previewHeader(title, description), body)
}

// previewPlainCard is a card of sections 16 apart, with no footer.
func previewPlainCard(sections ...ggui.Widget) ggui.Widget {
	return ui.Card(ggui.Column(sections...).Gap(16).Stretch()).Pad(16)
}

// previewEmpty is a card with nothing in it yet: a tile of media over a
// title, what to do about it, and the button that does.
func previewEmpty(media ggui.Widget, title, description string, action ggui.Widget) ggui.Widget {
	return ui.Card(ggui.Column(
		ggui.Center(ggui.Box(media).Pad(8).Fill(previewMuted).Radius(8)),
		ggui.Column(
			ui.Heading(title).Size(14).Weight(ggui.WeightMedium).Align(.5),
			previewDescription(description).Align(.5),
		).Gap(8).Stretch(),
		ggui.Center(action),
	).Gap(16).Stretch()).Pad(32, 24)
}

// previewValue is a row naming a value at its right edge.
func previewValue(label, value string) ggui.Widget {
	return ggui.Row(ggui.Expanded(previewDescription(label)), ggui.Text(value).Size(14).Weight(ggui.WeightMedium).NoWrap()).Gap(8).Align(ggui.AlignCenter)
}

// Both colors stay live when the surrounding theme changes.
var previewMuted = uitheme.Mix(uitheme.Card, uitheme.Muted, .6)

func previewFooterCard(header, body, footer ggui.Widget) ggui.Widget {
	var sections []ggui.Widget
	for _, section := range []ggui.Widget{header, body} {
		if section != nil {
			sections = append(sections, section)
		}
	}
	content := ggui.Box(ggui.Column(sections...).Gap(16).Stretch()).Pad(16).Fill(uitheme.Card)
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
	p.say(message)
}

// say reports what a sample action did, in the preview's one dialog.
func (p *previewModel) say(message string) {
	p.feedback.Set(message)
	p.feedbackOpen.Set(true)
}

// dismiss is the close button at a card's corner. The collage keeps its
// cards, so it only says so.
func (p *previewModel) dismiss(card string) ggui.Widget {
	return ui.ButtonOf(ui.Icon(icons.Close), func() { p.say(card + " stays open in this preview.") }).
		Secondary().Size(ui.ButtonIconSmall).Name("Dismiss " + card).Key(p.key("dismiss-" + card))
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
		Name("Monthly contributions").Key(p.key("contributions-chart")).Height(172).Grid(true).Axes(false, false).Domain(0, 3000).CornerRadius(5).
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
	return previewFooterCard(previewHeaderAction("Contribution History", "Last 6 months of activity", ui.Badge("+12% vs last month").Secondary()),
		ggui.Column(chartWithLabels, ggui.Row(ggui.Expanded(tile("UPCOMING", "May 25, 2024", "$1,000 scheduled")), ggui.Expanded(tile("AUTO-SAVE PLAN", "Accelerated", "Recurring weekly"))).Gap(12)).Gap(16).Stretch(),
		ui.Button("View Full Report", func() {
			p.announce(p.chartFeedback, "December–May contributions: "+previewMoney("USD", 13350)+". Highest month: May, $3,000.00.")
		}).Key(p.key("full-report")))
}

func (p *previewModel) savingsCard() ggui.Widget {
	goal := func(name string, value *ggui.StateValue[float64], target float64) ggui.Widget {
		return previewTile(ggui.Column(
			ui.Caption(name).Size(12),
			ggui.Text(strings.TrimSuffix(previewMoney("USD", target), ".00")).Size(30).Weight(ggui.WeightSemibold),
			ui.Progress(ggui.Map(value, func(v float64) float64 { return v / target })).Height(4),
			ggui.Row(ggui.Expanded(ui.CaptionOf(ggui.Map(value, func(v float64) string { return fmt.Sprintf("%.0f%% achieved", v/target*100) })).Size(14)),
				ui.CaptionOf(ggui.Map(value, func(v float64) string { return strings.TrimSuffix(previewMoney("USD", v), ".00") })).Size(14)).Gap(8),
		).Gap(14).Stretch())
	}
	header := previewHeaderAction("Savings Targets", "Active milestones for 2024",
		ui.Button("New Goal", func() { p.savingsOpen.Set(true) }).Name("Manage savings targets").Key(p.key("manage-savings")).Outline().Size(ui.ButtonSmall))
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
	amountValue := ggui.TextOf(amount).Size(24).Weight(ggui.WeightSemibold)
	amountRow := ggui.Responsive(330, ggui.Row(amountLabel, ggui.Spacer(), amountValue).Gap(8).Align(ggui.AlignCenter), ggui.Column(amountLabel, amountValue).Gap(8))
	return previewFooterCard(previewHeaderAction("Payout Threshold", "Set the minimum balance required before a payout is triggered.", p.dismiss("Payout Threshold")), ggui.Column(
		ui.Field("Preferred Currency", currency),
		ggui.Column(amountRow, slider, ggui.Row(ggui.Expanded(ui.Caption("$50 (MIN)").Size(12)), ui.Caption("$10,000 (MAX)").Size(12))).Gap(12).Stretch(),
		ui.Field("Notes", ui.TextField(p.notes).Name("Payout notes").Key(p.key("payout-notes")).Lines(4).MinWidth(0).Placeholder("Add any notes for this payout configuration…")),
	).Gap(20).Stretch(), ui.Button("Save Threshold", func() {
		message := "Threshold saved: " + previewMoney(ggui.Peek(p.payoutCurrency), ggui.Peek(p.threshold)) + "."
		if strings.TrimSpace(ggui.Peek(p.notes)) != "" {
			message += " Note included."
		}
		p.announce(p.payoutFeedback, message)
	}).Key(p.key("save-threshold")))
}

func (p *previewModel) distributeCard() ggui.Widget {
	return previewEmpty(ui.Icon(icons.Plus), "Distribute Track",
		"Upload your first master to start reaching listeners on Spotify, Apple Music, and more.",
		ui.Button("Create Release", func() { p.releaseOpen.Set(true) }).Key(p.key("create-release")))
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

func (p *previewModel) balanceCard() ggui.Widget {
	return previewFooterCard(ggui.Column(
		ui.Heading("Claimable Balance").Size(14).Weight(ggui.WeightMedium).Color(uitheme.MutedFg),
		ggui.TextOf(ggui.Map(p.balance, func(v float64) string { return previewMoney("USD", v) })).Size(36).Weight(ggui.WeightSemibold),
		ggui.Row(ggui.Box(ggui.Row(ggui.Box().Size(7, 7).Fill(color.NRGBA{R: 234, G: 179, B: 8, A: 255}).Radius(4), ggui.Text("Pending Setup").Size(12)).Gap(5).Align(ggui.AlignCenter)).Pad(2, 7).Border(1, uitheme.Border).Radius(20)),
	).Gap(12).Stretch(), previewTile(ggui.Column(previewValue("Net Royalties", "$0.00"), previewValue("Processing Fee", "−$0.00"), ui.Divider(), previewValue("Total Ready to Claim", "$0.00 USD")).Gap(16).Stretch()),
		previewDescription("Once your bank is connected, balances over $10.00 are automatically eligible for monthly distribution on the 15th of each month."))
}
