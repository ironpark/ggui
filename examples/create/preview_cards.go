package main

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// previewGain is the green of money coming in, legible on both themes.
var previewGain = color.NRGBA{R: 34, G: 197, B: 94, A: 255}

// previewBars is a bar chart of one series over labels, with the reference's
// faint grid and its category names under the plot.
func (p *previewModel) previewBars(name string, height float64, labels []string, values []float64) ggui.Widget {
	data := make([]ui.ChartDatum, len(labels))
	names := make([]ggui.Widget, len(labels))
	for i, label := range labels {
		data[i] = ui.ChartDatum{Label: label, Values: map[string]float64{"value": values[i]}}
		names[i] = ggui.Expanded(ggui.Center(ui.Caption(label).Size(12)))
	}
	chart := ui.BarChart(data, ui.ChartConfig{{Key: "value", Label: name, ColorIndex: 2}}).
		Name(name).Key(p.key(name)).Height(height).Grid(true).Axes(false, false).CornerRadius(4).Domain(0, slices.Max(values))
	return ggui.Column(chart, ggui.Padding(ggui.Row(names...), 0, 12)).Gap(0).Stretch()
}

func (p *previewModel) scanCard() ggui.Widget {
	return previewFooterCard(nil, ggui.Column(
		ggui.Center(previewQR(194)),
		ggui.Column(
			ui.Heading("Scan to connect your mobile device").Size(16).Weight(ggui.WeightMedium).Align(.5),
			previewDescription("Open the Ledger mobile app and scan this code to link your device.").Align(.5),
		).Gap(4).Stretch(),
	).Gap(20).Stretch(), ui.Button("Got it", func() {
		p.say("No device was linked: the code is a sample.")
	}).Secondary().Key(p.key("scan-done")))
}

// previewQR draws a sample code, size pixels square: the three finder
// patterns of a QR code around modules that encode nothing.
func previewQR(size float64) ggui.Widget {
	const modules, quiet = 25, 22.0
	finder := func(x, y int) (inside, dark bool) {
		for _, at := range [][2]int{{0, 0}, {modules - 7, 0}, {0, modules - 7}} {
			dx, dy := x-at[0], y-at[1]
			if dx < -1 || dx > 7 || dy < -1 || dy > 7 {
				continue
			}
			ring := max(max(dx, 6-dx)-3, max(dy, 6-dy)-3) // 3 at the rim, 0 in the middle
			return true, dx >= 0 && dx <= 6 && dy >= 0 && dy <= 6 && ring != 2
		}
		return false, false
	}
	return ggui.FromFuncs(func(cs ggui.Constraints, _ ggui.Env) ggui.Size {
		return cs.Constrain(ggui.Sz(size, size))
	}, func(dst *ggui.Canvas, r ggui.Rect) {
		dst.FillRoundRect(r, 14, color.White)
		cell := (r.Size.W - 2*quiet) / modules
		seed := uint32(0x2f6e2b1)
		for y := range modules {
			for x := range modules {
				seed = seed*1664525 + 1013904223
				inside, dark := finder(x, y)
				if !inside {
					dark = seed>>16&3 < 2
				}
				if dark {
					at := ggui.Pt(r.Origin.X+quiet+float64(x)*cell, r.Origin.Y+quiet+float64(y)*cell)
					dst.FillRect(ggui.Rect{Origin: at, Size: ggui.Sz(cell+.25, cell+.25)}, color.Black)
				}
			}
		}
	})
}

func (p *previewModel) dividendsCard() ggui.Widget {
	holding := func(name, shares, amount string, quarters ...float64) ggui.Widget {
		return previewTile(ggui.Row(
			ggui.Expanded(ggui.Column(previewLabel(name).NoWrap().Ellipsis(), ui.Caption(shares).Size(14)).Gap(6)),
			previewSpark(quarters...),
			ggui.Text(amount).Size(14).Weight(ggui.WeightSemibold).NoWrap(),
		).Gap(12).Align(ggui.AlignCenter))
	}
	return previewPlainCard(
		previewHeaderAction("Q2 Dividend Income", "Quarterly dividend payouts across your portfolio holdings.", p.dismiss("Q2 Dividend Income")),
		ggui.Column(
			holding("Vanguard VIG", "450 Shares", "$1,842.10", 2.8, 3, 2.9, 5),
			holding("S&P 500 VOO", "112 Shares", "$928.40", 2.8, 3.2, 5, 3.4),
			holding("Apple AAPL", "85 Shares", "$340.00", 2.8, 3.2, 5, 3.8),
			holding("Realty Income", "320 Shares", "$1,139.50", 3.3, 3.5, 3.7, 5),
		).Gap(12).Stretch(),
	)
}

// previewSpark is a chart too small to read values from: a bar a value,
// over the faint lines of a grid.
func previewSpark(values ...float64) ggui.Widget {
	const width, height, bar, gap = 92.0, 34.0, 17.0, 5.0
	var fill, line color.Color
	return ggui.FromFuncs(func(cs ggui.Constraints, env ggui.Env) ggui.Size {
		t := uitheme.Of(env)
		fill, line = uitheme.Fade(t.Fg, .42), uitheme.Fade(t.Fg, .08)
		return cs.Constrain(ggui.Sz(width, height))
	}, func(dst *ggui.Canvas, r ggui.Rect) {
		for i := range 4 {
			y := r.Origin.Y + float64(i)*r.Size.H/3
			dst.StrokeLine(ggui.Pt(r.Origin.X, y), ggui.Pt(r.Origin.X+r.Size.W, y), 1, line)
		}
		most := slices.Max(values)
		x := r.Origin.X + (r.Size.W-float64(len(values))*(bar+gap)+gap)/2
		for _, v := range values {
			h := r.Size.H * v / most
			dst.FillRoundRect(ggui.Rect{Origin: ggui.Pt(x, r.Origin.Y+r.Size.H-h), Size: ggui.Sz(bar, h)}, 3, fill)
			x += bar + gap
		}
	})
}

func (p *previewModel) averagingCard() ggui.Widget {
	return previewCard("Dollar-Cost Averaging", "A strategy for building wealth over time.",
		ggui.Text("Over time, this smooths out the average cost of your investments. When prices drop, your fixed amount buys more shares. When prices rise, you buy fewer. The result is a lower average cost per share compared to lump-sum investing during volatile periods.").
			Size(14).LineHeight(1.65).Color(uitheme.MutedFg))
}

func (p *previewModel) syncingCard() ggui.Widget {
	return previewEmpty(ui.Spinner().Size(16), "Syncing your accounts",
		"We're pulling in your latest transactions. This usually takes a few seconds.",
		ui.Button("Cancel", func() { p.say("Nothing is syncing: the preview has no accounts.") }).Outline().Name("Cancel sync").Key(p.key("cancel-sync")))
}

func (p *previewModel) preferencesCard() ggui.Widget {
	toggle := func(title, description string, state *ggui.StateValue[bool]) ggui.Widget {
		control := ui.Switch(state, "").Key(p.key(title))
		control.SetName(title)
		return ggui.Row(ggui.Expanded(ggui.Column(previewLabel(title), previewDescription(description)).Gap(2)), control).Gap(12).Align(ggui.AlignStart)
	}
	return previewFooterCard(
		previewHeaderAction("Preferences", "Manage your account settings and notifications.", p.dismiss("Preferences")),
		ggui.Column(
			ui.Field("Default Currency", ui.Select(p.preferredCurrency).Options([]string{"USD", "EUR", "GBP"}).Format(previewCurrency).Name("Default currency").Key(p.key("preferred-currency"))),
			ui.Divider(),
			toggle("Public Statistics", "Allow others to see your total stream count and listening activity", p.publicStats),
			ui.Divider(),
			toggle("Email Notifications", "Monthly royalty reports and distribution updates", p.emailUpdates),
		).Gap(14).Stretch(),
		ggui.Row(
			ui.Button("Reset", func() {
				p.emailUpdates.Set(true)
				p.publicStats.Set(true)
				p.preferredCurrency.Set("USD")
				p.announce(p.preferencesFeedback, "Default preferences restored.")
			}).Name("Reset preferences").Key(p.key("reset-preferences")).Outline(),
			ggui.Spacer(),
			ui.Button("Save Preferences", func() { p.announce(p.preferencesFeedback, "Preferences saved in this preview.") }).Key(p.key("save-preferences")),
		).Gap(8).Align(ggui.AlignCenter))
}

func (p *previewModel) goalCard() ggui.Widget {
	ring := ui.ProgressRing(ggui.State(.8)).Size(168).Stroke(16)
	figure := ggui.Box(ggui.Stack(ring, ggui.Center(ggui.Column(
		ggui.Text("$24,000").Size(24).Weight(ggui.WeightSemibold).Align(.5),
		ui.Caption("80% of $30,000").Size(12).Align(.5),
	).Gap(2)))).Size(168, 168)
	return previewFooterCard(nil, ggui.Padding(ggui.Center(figure), 12, 0),
		ggui.Column(
			previewValue("Projected Finish", "October 2024"), ui.Divider(),
			previewValue("Monthly Average", "$1,250"), ui.Divider(),
			previewValue("Top Contributor", "Auto-Transfer"),
		).Gap(12).Stretch())
}

func (p *previewModel) kitchenCard() ggui.Widget {
	on, scene := ggui.State(true), ggui.State("Cooking")
	power := ui.Switch(on, "").Key(p.key("kitchen-power"))
	power.SetName("Kitchen Island power")
	level := func(icon, name string, value float64) ggui.Widget {
		slider := ui.Slider(ggui.State(value), 0, 100).Step(5).Name(name).Key(p.key("kitchen-" + name)).BindDisabled(ggui.Map(on, func(on bool) bool { return !on }))
		return ggui.Box(ggui.Row(glyph(icon), ggui.Box(previewLabel(name)).Width(112), ggui.Expanded(slider)).Gap(10).Align(ggui.AlignCenter)).
			Pad(10, 12).Border(1, uitheme.Border).Radius(8)
	}
	return previewPlainCard(
		previewHeaderAction("Kitchen Island", "Hue Color Ambient", power),
		ui.ToggleGroup(scene).Options([]string{"Cooking", "Dining", "Nightlight", "Focus"}).Outline().Name("Scenes").Key(p.key("kitchen-scene")),
		ggui.Column(
			level("brightness", "Brightness", 85), level("thermometer", "Color Temp", 60),
			level("volume", "Volume", 30), level("timer", "Fade", 0),
		).Gap(10).Stretch(),
	)
}

// previewAmount is a money field, its currency sign leading the digits.
func (p *previewModel) previewAmount(value *ggui.StateValue[string], name string) ggui.Widget {
	return ui.InputGroup(ggui.TextInput(value).MinWidth(0).Key(p.key(name))).
		Leading(ggui.Text("$").Size(14).Color(uitheme.MutedFg)).Name(name)
}

// previewNumber reads an amount as it is typed, with its thousands commas.
func previewNumber(s string) float64 {
	v, _ := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 64)
	return v
}

func (p *previewModel) buyCard() ggui.Widget {
	amount, order := ggui.State("1,000.00"), ggui.State("Market Order")
	help := map[string]string{
		"Market Order": "Market orders execute at the current price.",
		"Limit Order":  "Limit orders execute at your price or better.",
		"Stop Order":   "Stop orders trigger once the price is reached.",
	}
	shares := ggui.Map(amount, func(s string) string { return fmt.Sprintf("%.2f", previewNumber(s)/512.82) })
	value := func(label string, value ggui.Widget) ggui.Widget {
		return ggui.Row(ggui.Expanded(previewDescription(label)), value).Gap(8).Align(ggui.AlignCenter)
	}
	return previewFooterCard(ui.Heading("Buy Investment").Size(16).Weight(ggui.WeightMedium), ggui.Column(
		ui.Field("Amount to Invest", p.previewAmount(amount, "Amount to invest")),
		ggui.Column(
			ui.Field("Order Type", ui.Select(order).Options([]string{"Market Order", "Limit Order", "Stop Order"}).Name("Order type").Key(p.key("order-type"))),
			ggui.TextOf(ggui.Map(order, func(o string) string { return help[o] })).Size(14).LineHeight(1.5).Color(uitheme.MutedFg),
		).Gap(8).Stretch(),
		ggui.Column(
			value("Estimated Shares", ggui.TextOf(shares).Size(14).Weight(ggui.WeightMedium)),
			value("Buying Power", previewLabel("$12,450.00")),
		).Gap(10).Stretch(),
	).Gap(16).Stretch(), ggui.Column(
		ui.Button("Review Order", func() {
			p.say(fmt.Sprintf("%s for $%s: about %s shares. No order was placed.", ggui.Peek(order), ggui.Peek(amount), ggui.Peek(shares)))
		}).Key(p.key("review-order")),
		ui.Caption("Trades are typically executed within minutes during market hours.").Size(12).Align(.5),
	).Gap(12).Stretch())
}

func (p *previewModel) transactionsCard() ggui.Widget {
	rows := []struct {
		name, note, when, amount, icon string
		income                         bool
	}{
		{"Blue Bottle Coffee", "Food & Drink", "Today, 10:24 AM", "−$6.50", "coffee", false},
		{"Whole Foods Market", "Groceries", "Yesterday", "−$142.30", "cart", false},
		{"Stripe Payout", "Income", "Oct 12", "+$4,200.00", "wallet", true},
		{"Uber Technologies", "Transport", "Oct 11", "−$24.10", "car", false},
		{"Netflix Subscription", "Entertainment", "Oct 10", "−$19.99", "tv", false},
		{"Figma", "Software", "Oct 9", "−$15.00", "figma", false},
		{"Apple Music", "Entertainment", "Oct 8", "−$10.99", "music", false},
	}
	const shown = 5
	items := make([]ggui.Widget, len(rows))
	for i, row := range rows {
		amount := ggui.Text(row.amount).Size(14).NoWrap().Weight(ggui.WeightSemibold).Align(1)
		if row.income {
			amount.Color(previewGain)
		}
		media := ggui.Box(glyph(row.icon).Size(18)).Pad(11).Fill(previewMuted).Radius(8)
		title := ggui.Column(previewLabel(row.name).NoWrap().Ellipsis(), ui.Caption(row.note).Size(14)).Gap(2)
		more := ui.ButtonOf(ui.Icon(icons.MoreHorizontal), func() {
			p.say(row.name + " · " + row.note + " · " + row.when + " · " + row.amount)
		}).Ghost().Size(ui.ButtonIconSmall).Name("More about " + row.name).Key(p.key("more-" + row.name))
		// A wide card is a table with a date column and a menu; a narrow one
		// keeps the merchant and the amount.
		var item ggui.Widget = ggui.Responsive(520,
			ggui.Row(media, ggui.Flex(title, 5), ggui.Flex(ui.Caption(row.when).Size(14).NoWrap(), 4), ggui.Box(amount).Width(104), more).Gap(16).Align(ggui.AlignCenter),
			ggui.Row(media, ggui.Expanded(title), amount).Gap(12).Align(ggui.AlignCenter))
		if i > 0 {
			item = ggui.Column(ui.Divider(), item).Gap(8).Stretch()
		}
		items[i] = ggui.Padding(item, 0, 8)
	}
	label := ggui.Map(p.showAllTransactions, func(all bool) string {
		if all {
			return "Show fewer transactions"
		}
		return "View all transactions"
	})
	toggle := ui.ButtonOf(ggui.TextOf(ggui.Map(p.showAllTransactions, func(all bool) string {
		if all {
			return "Show Less"
		}
		return "View All"
	})), func() { p.showAllTransactions.Set(!ggui.Peek(p.showAllTransactions)) }).
		BindName(label).Key(p.key("transactions-toggle")).Outline().Size(ui.ButtonSmall)
	more := ggui.Column(items[shown:]...).Gap(8).Stretch()
	return previewPlainCard(
		previewHeaderAction("Recent Transactions", "Your latest account activity.", toggle),
		ggui.Column(
			ggui.Column(items[:shown]...).Gap(8).Stretch(),
			ggui.If(p.showAllTransactions, func() ggui.Widget { return more }),
		).Gap(8).Stretch(),
	)
}

// navCards is the two menu cards side by side, a selection each.
func (p *previewModel) navCards() ggui.Widget {
	type entry struct{ icon, label string }
	menu := func(name, selected string, sections ...[]entry) ggui.Widget {
		current := ggui.State(selected)
		var rows []ggui.Widget
		for i, section := range sections {
			if i > 0 {
				rows = append(rows, ggui.Padding(ui.Divider(), 4, 0))
			}
			rows = append(rows, ggui.Padding(ui.Caption(section[0].label).Size(12).Weight(ggui.WeightMedium), 6, 8))
			for _, e := range section[1:] {
				rows = append(rows, previewNavItem(current, e.icon, e.label).Key(p.key("nav-"+e.label)))
			}
		}
		return ui.Card(ggui.Column(rows...).Gap(2).Stretch()).Pad(8)
	}
	return ggui.Row(
		ggui.Expanded(menu("Overview", "Dashboard",
			[]entry{{"", "Overview"}, {"dashboard", "Dashboard"}, {"transfer", "Transactions"}, {"trending", "Investments"}, {"bank", "Accounts"}, {"pie", "Spending"}},
			[]entry{{"", "Planning"}, {"target", "Goals"}, {"wallet", "Budget"}, {"report", "Reports"}, {"document", "Documents"}})),
		ggui.Expanded(menu("Account", "Billing",
			[]entry{{"", "Account"}, {"user", "Profile"}, {"card", "Billing"}, {"bell", "Notifications"}, {"shield", "Security"}, {"brush", "Appearance"}},
			[]entry{{"", "Support"}, {"help", "Help Center"}, {"message", "Contact Us"}, {"book", "Documentation"}, {"activity", "Status"}})),
	).Gap(24).Align(ggui.AlignStart)
}

// previewNavItem is a menu row that selects itself in current: a ghost
// button whose label starts at its leading edge, filled while it is chosen.
func previewNavItem(current *ggui.StateValue[string], icon, label string) *ui.ButtonWidget {
	face := ggui.Row(glyph(icon), ggui.Expanded(ggui.Text(label).Size(14).NoWrap().Ellipsis())).Gap(8).Align(ggui.AlignCenter)
	var fill color.Color
	var radius float64
	chosen := ggui.FromFuncs(func(cs ggui.Constraints, env ggui.Env) ggui.Size {
		fill, radius = nil, uitheme.Of(env).RadiusSm
		if current.Get() == label {
			fill = ggui.ResolveColor(uitheme.Accent, env)
		}
		return face.Layout(cs, env)
	}, func(dst *ggui.Canvas, r ggui.Rect) {
		if fill != nil {
			dst.FillRoundRect(ggui.Rect{Origin: ggui.Pt(r.Origin.X-8, r.Origin.Y-6), Size: ggui.Sz(r.Size.W+16, r.Size.H+12)}, radius, fill)
		}
		dst.Paint(face, r)
	})
	return ui.ButtonOf(chosen, func() { current.Set(label) }).Ghost().Size(ui.ButtonSmall).Name(label)
}

func (p *previewModel) faqCard() ggui.Widget {
	answers := func(key string, items ...[2]string) ggui.Widget {
		sections := make([]ui.AccordionSection, len(items))
		for i, item := range items {
			sections[i] = ui.AccordionItem(item[0], item[0], ggui.Text(item[1]).Size(14).LineHeight(1.5))
		}
		return ui.Accordion(ggui.State([]string{items[0][0]}), sections...).Name(key + " questions").Key(p.key("faq-" + key))
	}
	tabs := ui.Tabs(ggui.State(0),
		ui.Tab("General", answers("General",
			[2]string{"How secure is my financial data with Ledger?", "We use bank-level AES-256 encryption, SOC 2 Type II certified infrastructure, and never store your credentials. All connections use read-only access tokens. We are a SEC registered investment advisor."},
			[2]string{"How do I connect my bank or investment accounts?", "Open Accounts, choose Add Account and sign in with your institution. Balances appear within a minute."},
			[2]string{"Can I export my data for tax purposes?", "Yes. Reports exports every transaction and dividend as CSV or PDF, by tax year."})),
		ui.Tab("Billing", answers("Billing",
			[2]string{"When am I charged?", "Plans renew on the day of the month you subscribed. Invoices are emailed and kept under Billing."},
			[2]string{"Can I change plans mid-cycle?", "Yes. Upgrades apply at once and are prorated; downgrades apply at the next renewal."},
			[2]string{"Which payment methods are accepted?", "All major cards, and bank debit in supported regions."})),
		ui.Tab("Goals", answers("Goals",
			[2]string{"How are goal projections calculated?", "From your average monthly contribution over the last six months, without assuming market returns."},
			[2]string{"Can a goal draw from several accounts?", "Yes. Link any number of accounts to a goal and weight each one's share."},
			[2]string{"What happens when I reach a goal?", "It is marked complete and its automatic transfers pause until you choose what comes next."})),
	).Name("Help topics").Key(p.key("faq-tabs"))
	return previewFooterCard(nil, tabs, ui.Button("Contact Support", func() {
		p.say("Support is not connected in this preview.")
	}).Outline().Key(p.key("contact-support")))
}

func (p *previewModel) shortcutsCard() ggui.Widget {
	shortcut := func(icon, title, description string) ggui.Widget {
		open := ui.ButtonOf(ui.Icon(icons.ChevronRight), func() { p.say(title + ": " + description) }).
			Ghost().Size(ui.ButtonIconSmall).Name(title).Key(p.key("shortcut-" + title))
		return previewTile(ggui.Row(
			ggui.Padding(glyph(icon), 2, 0, 0, 0),
			ggui.Expanded(ggui.Column(previewLabel(title), previewDescription(description)).Gap(2)),
			open,
		).Gap(10).Align(ggui.AlignStart))
	}
	trail := ui.Breadcrumb(
		ui.Crumb("Home", func() { p.say("Home is outside this preview.") }),
		ui.Crumb("Account", func() { p.say("Account is outside this preview.") }),
		ui.Crumb("Settings", func() { p.say("Settings is outside this preview.") }),
		ui.Crumb("Payments", nil),
	).Max(3).Name("Location")
	return previewPlainCard(trail, ggui.Column(
		shortcut("gauge", "Change transfer limit", "Adjust how much you can send from your balance."),
		shortcut("calendar", "Scheduled transfers", "Set up a transfer to send at a later date."),
		shortcut("repeat", "Direct Debits", "Set up and manage regular payments."),
		shortcut("refresh", "Recurring card payments", "Manage your repeated card transactions."),
	).Gap(12).Stretch())
}

func (p *previewModel) doorCard() ggui.Widget {
	status := ggui.Row(ggui.Text("Locked").Size(14).Color(uitheme.MutedFg), glyph("lock").Color(uitheme.MutedFg)).Gap(6).Align(ggui.AlignCenter)
	feed := ggui.Stack(previewStripes(156), ggui.Align(ggui.Padding(ui.Badge("Live").Destructive(), 8)).Right().Top())
	return previewPlainCard(previewHeaderAction("Front Door", "Smart Lock Pro", status), feed)
}

// previewStripes is a placeholder for a picture: a muted panel, height
// tall, hatched with diagonal lines.
func previewStripes(height float64) ggui.Widget {
	var fill, line color.Color
	var radius float64
	return ggui.FromFuncs(func(cs ggui.Constraints, env ggui.Env) ggui.Size {
		t := uitheme.Of(env)
		fill, line, radius = ggui.ResolveColor(previewMuted, env), uitheme.Fade(t.Fg, .07), t.RadiusSm
		w := cs.MaxW
		if math.IsInf(w, 1) {
			w = 2 * height
		}
		return cs.Constrain(ggui.Sz(w, height))
	}, func(dst *ggui.Canvas, r ggui.Rect) {
		dst.ClipRoundRect(r, radius, func(clipped *ggui.Canvas) {
			clipped.FillRect(r, fill)
			for x := -r.Size.H; x < r.Size.W; x += 12 {
				clipped.StrokeLine(ggui.Pt(r.Origin.X+x, r.Origin.Y), ggui.Pt(r.Origin.X+x+r.Size.H, r.Origin.Y+r.Size.H), 1, line)
			}
		})
	})
}

func (p *previewModel) holdingsCard() ggui.Widget {
	query, kinds := ggui.State(""), ggui.State([]string{})
	rows := []struct{ ticker, name, detail, kind, group, value string }{
		{"VOO", "Vanguard S&P 500 ETF", "112 SHARES · JAN 2021", "ETF", "ETFs", "$48,230.40"},
		{"VIG", "Vanguard Dividend Appreciation", "450 SHARES · MAR 2022", "ETF", "ETFs", "$26,033.79"},
		{"AAPL", "Apple Inc.", "85 SHARES · NOV 2020", "Stock", "Stocks", "$18,488.90"},
		{"O", "Realty Income Corp", "320 SHARES · JUN 2023", "REIT", "REITs", "$15,136.59"},
	}
	var items []ggui.Widget
	for _, row := range rows {
		item := previewTile(ggui.Row(
			ggui.Box(ggui.Center(ggui.Text(row.ticker).Size(13).Weight(ggui.WeightSemibold).NoWrap())).Size(48, 48).Border(1, uitheme.Border).Radius(8),
			ggui.Expanded(ggui.Column(previewLabel(row.name).NoWrap().Ellipsis(), ui.Caption(row.detail).Size(12).NoWrap().Ellipsis()).Gap(4)),
			ui.Badge(row.kind).Outline(),
			ggui.Column(ui.Caption("VALUE").Size(12).Align(1), ggui.Text(row.value).Size(14).Weight(ggui.WeightSemibold).NoWrap().Align(1)).Gap(2).Align(ggui.AlignEnd),
		).Gap(12).Align(ggui.AlignCenter))
		shown := ggui.Derived(func() bool {
			q, k := strings.ToLower(strings.TrimSpace(query.Get())), kinds.Get()
			named := strings.Contains(strings.ToLower(row.ticker+" "+row.name), q)
			return named && (len(k) == 0 || slices.Contains(k, row.group))
		})
		items = append(items, ggui.If(shown, func() ggui.Widget { return ggui.Padding(item, 0, 0, 12, 0) }))
	}
	search := ui.InputGroup(ggui.TextInput(query).MinWidth(0).Placeholder("Search holdings or tickers…").Key(p.key("holdings-search"))).
		Leading(ui.Icon(icons.Search).Color(uitheme.MutedFg)).Name("Search holdings")
	filter := ui.ToggleGroupMultiple(kinds).Options([]string{"Stocks", "ETFs", "REITs"}).Outline().Name("Holding kinds").Key(p.key("holdings-kinds"))
	none := ggui.Derived(func() bool {
		q, k := strings.ToLower(strings.TrimSpace(query.Get())), kinds.Get()
		return !slices.ContainsFunc(rows, func(row struct{ ticker, name, detail, kind, group, value string }) bool {
			return strings.Contains(strings.ToLower(row.ticker+" "+row.name), q) && (len(k) == 0 || slices.Contains(k, row.group))
		})
	})
	items = append(items, ggui.If(none, func() ggui.Widget {
		return ggui.Padding(previewDescription("No holdings match.").Align(.5), 12, 0, 24, 0)
	}))
	// The last row's own gap would double the card's padding.
	list := ggui.Padding(ggui.Column(items...).Stretch(), 0, 0, -12, 0)
	return previewPlainCard(
		ggui.Responsive(520, ggui.Row(ggui.Expanded(search), filter).Gap(12).Align(ggui.AlignCenter), ggui.Column(search, filter).Gap(12).Stretch()),
		list,
	)
}

func (p *previewModel) accessCard() ggui.Widget {
	email, password := ggui.State("artist@studio.inc"), ggui.State("correct horse")
	forgot := ui.Button("FORGOT?", func() { p.say("A reset link would be sent to " + ggui.Peek(email) + ".") }).
		Link().Size(ui.ButtonExtraSmall).Name("Forgot password").Key(p.key("forgot-password"))
	danger := previewTile(ggui.Row(
		ggui.Padding(glyph("circle-alert").Color(uitheme.Destructive), 2, 0, 0, 0),
		ggui.Expanded(ggui.Column(previewLabel("Danger Zone"), previewDescription("Archive account and remove catalog")).Gap(2)),
		ui.ButtonOf(glyph("arrow-right"), func() { p.say("Nothing was archived: this account is a sample.") }).
			Ghost().Size(ui.ButtonIconSmall).Name("Danger Zone").Key(p.key("danger-zone")),
	).Gap(10).Align(ggui.AlignCenter))
	return previewFooterCard(previewHeader("Account Access", "Update your credentials or re-authenticate."), ggui.Column(
		ui.Field("Email Address", ui.TextField(email).Name("Email address").MinWidth(0).Key(p.key("access-email"))),
		ggui.Column(
			ggui.Row(previewLabel("Current Password"), ggui.Spacer(), forgot).Align(ggui.AlignCenter),
			ui.TextField(password).Password().Name("Current password").MinWidth(0).Key(p.key("access-password")),
		).Gap(8).Stretch(),
	).Gap(16).Stretch(), ggui.Column(
		ui.Button("Update Security", func() { p.say("Credentials are not stored or sent by this preview.") }).Icon(glyph("lock")).Key(p.key("update-security")),
		danger,
	).Gap(12).Stretch())
}

// summaryCards is the card's balance beside its due date, over the year.
func (p *previewModel) summaryCards() ggui.Widget {
	figure := func(label, value string, rest ...ggui.Widget) ggui.Widget {
		return ui.Card(ggui.Column(append([]ggui.Widget{
			ggui.Text(label).Size(14).Color(uitheme.MutedFg), ggui.Text(value).Size(24).Weight(ggui.WeightMedium).NoWrap(),
		}, rest...)...).Gap(6).Stretch()).Pad(16)
	}
	months := []string{"J", "F", "M", "A", "M", "J", "J", "A", "S", "O", "N", "D"}
	spend := []float64{4, 5.5, 3.5, 6, 4.5, 5, 6.5, 4, 5.5, 7, 4.5, 8}
	return ggui.Column(
		ggui.Row(
			ggui.Expanded(figure("Card Balance", "US$12.94", previewDescription("US$11,337.06 Available"))),
			ggui.Expanded(figure("Payment Due", "1 Apr", ggui.Box().Height(2), ui.Button("Pay Early", func() {
				p.say("No payment was made: the card is a sample.")
			}).Outline().Size(ui.ButtonSmall).Key(p.key("pay-early")))),
		).Gap(12).Stretch(),
		ui.Card(ggui.Column(
			ggui.Row(ggui.Expanded(ggui.Text("Yearly Activity").Size(14).Color(uitheme.MutedFg)), ui.Badge("+US$0.25 Daily Cash").Secondary()).Gap(8).Align(ggui.AlignCenter),
			p.previewBars("Yearly activity", 64, months, spend),
		).Gap(10).Stretch()).Pad(16),
	).Gap(12).Stretch()
}

func (p *previewModel) transferCard() ggui.Widget {
	amount := ggui.State("1,200.00")
	from, to := ggui.State("Main Checking (··8402) — $12,450.00"), ggui.State("High Yield Savings (··1192) — $42,100.00")
	accounts := []string{"Main Checking (··8402) — $12,450.00", "High Yield Savings (··1192) — $42,100.00", "Brokerage (··5530) — $91,889.68"}
	total := ggui.Map(amount, func(s string) string { return previewMoney("USD", previewNumber(s)) })
	return previewFooterCard(
		previewHeaderAction("Transfer Funds", "Move money between your connected accounts.", p.dismiss("Transfer Funds")),
		ggui.Column(
			ui.Field("Amount to Transfer", p.previewAmount(amount, "Amount to transfer")),
			ui.Field("From Account", ui.Select(from).Options(accounts).Name("From account").Key(p.key("transfer-from"))),
			ui.Field("To Account", ui.Select(to).Options(accounts).Name("To account").Key(p.key("transfer-to"))),
			previewTile(ggui.Column(
				previewValue("Estimated arrival", "Today, Apr 14"), ui.Divider(),
				previewValue("Transaction fee", "$0.00"), ui.Divider(),
				ggui.Row(ggui.Expanded(previewLabel("Total amount")), ggui.TextOf(total).Size(14).Weight(ggui.WeightSemibold).NoWrap()).Gap(8),
			).Gap(12).Stretch()),
		).Gap(16).Stretch(),
		ui.Button("Confirm Transfer", func() {
			if ggui.Peek(from) == ggui.Peek(to) {
				p.say("Choose two different accounts.")
				return
			}
			p.say(ggui.Peek(total) + " would move today. No money was transferred.")
		}).Key(p.key("confirm-transfer")))
}

func (p *previewModel) coverCard() ggui.Widget {
	frame := ui.AspectRatio(1, ggui.Box(ggui.Center(glyph("image").Size(32).Color(uitheme.MutedFg))).Border(1, uitheme.Border).Radius(8))
	return previewFooterCard(ui.Caption("COVER ART").Size(12), frame, ggui.Column(
		ui.Button("Upload Artwork", func() { p.say("File uploads are not connected in this preview.") }).Secondary().Key(p.key("upload-artwork")),
		ui.Caption("Minimum 3000 × 3000px\nJPEG or PNG only").Size(12).Align(.5),
	).Gap(12).Stretch())
}

// loadingCard is a card whose content has not arrived.
func (p *previewModel) loadingCard() ggui.Widget {
	line := func(fraction, height float64) ggui.Widget {
		return ggui.Row(ggui.Flex(ui.Skeleton(0, height), fraction), ggui.Flex(ggui.Box(), 1-fraction))
	}
	return previewPlainCard(
		ggui.Column(line(.37, 20), line(.56, 16)).Gap(6).Stretch(),
		ui.Skeleton(0, 128),
		ggui.Column(line(1, 16), line(.75, 16), line(.5, 16)).Gap(8).Stretch(),
		ggui.Row(ggui.Expanded(ui.Skeleton(0, 36)), ggui.Expanded(ui.Skeleton(0, 36))).Gap(8),
	)
}

func (p *previewModel) methodCard() ggui.Widget {
	holder, iban, method := ggui.State("Synthetic Horizons Music LLC"), ggui.State(""), ggui.State("bank")
	option := func(value, title, description string) ggui.Widget {
		return ggui.Box(ggui.Column(
			ui.Radio(method, value, title).Key(p.key("method-"+value)),
			ggui.Padding(ui.Caption(description).Size(14), 0, 0, 0, 24),
		).Gap(4)).Pad(10, 12).Border(1, uitheme.Border).Radius(8)
	}
	header := ggui.Row(ggui.Expanded(ggui.Column(
		ggui.Text("Payout Preferences").Size(14).Color(uitheme.MutedFg),
		ui.Heading("Receiving Method").Size(16).Weight(ggui.WeightMedium),
	).Gap(4)), p.dismiss("Payout Preferences")).Gap(8).Align(ggui.AlignStart)
	return previewFooterCard(header, ggui.Column(
		ui.Field("Account Holder Name", ui.TextField(holder).Name("Account holder name").MinWidth(0).Key(p.key("method-holder"))),
		ggui.Column(previewLabel("Receiving Method"), ggui.Row(
			ggui.Expanded(option("bank", "Bank Transfer", "SWIFT / IBAN")),
			ggui.Expanded(option("paypal", "PayPal", "Instant Payout")),
		).Gap(12).Stretch()).Gap(8).Stretch(),
		ui.Field("IBAN / Account Number", ui.TextField(iban).Name("IBAN or account number").MinWidth(0).Placeholder("DE89 3704 0044 ….").Key(p.key("method-iban"))),
	).Gap(16).Stretch(), ui.Button("Save Payout Settings", func() {
		p.say("Payout settings saved in this preview for " + ggui.Peek(holder) + ".")
	}).BindDisabled(ggui.Combine(method, iban, func(method, iban string) bool {
		return method == "bank" && strings.TrimSpace(iban) == ""
	})).Key(p.key("save-method")))
}

func (p *previewModel) powerCard() ggui.Widget {
	hours := []string{"6a", "8a", "10a", "12p", "2p", "4p", "6p", "8p"}
	load := []float64{1.6, 3.8, 4.2, 3.2, 4.6, 3.9, 5.1, 4.3}
	figure := func(label, value string) ggui.Widget {
		return ggui.Expanded(ggui.Column(ggui.Text(label).Size(14).Color(uitheme.MutedFg), ggui.Text(value).Size(20).Weight(ggui.WeightSemibold)).Gap(4))
	}
	return previewFooterCard(previewHeader("Power Usage", "Whole Home"), ggui.Column(
		p.previewBars("Power usage", 150, hours, load),
		ui.Divider(),
		ggui.Row(figure("Currently Using", "3.4 kW"), figure("Solar Gen", "+1.2 kW")).Gap(12),
	).Gap(16).Stretch(), ggui.Column(
		ggui.Text("Battery Level").Size(14).Color(uitheme.MutedFg),
		ggui.Row(ggui.Expanded(ui.Progress(ggui.State(.85)).Height(4)), previewLabel("85%")).Gap(12).Align(ggui.AlignCenter),
	).Gap(8).Stretch())
}

func (p *previewModel) bankCard() ggui.Widget {
	return previewEmpty(glyph("card"), "Connect Bank",
		"Link your payout method to receive monthly royalty distributions automatically.",
		ui.Button("Set Up Payouts", func() {
			p.announce(p.balanceFeedback, "Bank connections are not available in this local preview. No account was connected and no funds were transferred.")
		}).Key(p.key("connect-bank")))
}

func (p *previewModel) upcomingCard() ggui.Widget {
	payment := func(name, date, amount string) ggui.Widget {
		return previewTile(ggui.Row(
			ggui.Expanded(ggui.Column(previewLabel(name).NoWrap().Ellipsis(), ui.Caption(date).Size(14)).Gap(4)),
			ui.Badge(amount).Secondary(),
		).Gap(12).Align(ggui.AlignCenter))
	}
	calendar := ui.Calendar(p.selectedDate).Name("Payout calendar").Key(p.key("payout-calendar"))
	return previewCard("Upcoming Payments", "Select a date to view scheduled payments.", ggui.Column(
		ggui.Box(ggui.Center(calendar)).Pad(8).Border(1, uitheme.Border).Radius(8),
		payment("Netflix Subscription", "Apr 15, 2024", "$19.99"),
		payment("Rent Payment", "Apr 1, 2024", "$2,400.00"),
		payment("Auto Insurance", "Apr 22, 2024", "$186.00"),
	).Gap(12).Stretch())
}

func (p *previewModel) shadesCard() ggui.Widget {
	closed := ggui.State(50.0) // how far down the shade is, in percent
	var shade, sill, border color.Color
	var radius, fraction float64
	window := ggui.FromFuncs(func(cs ggui.Constraints, env ggui.Env) ggui.Size {
		t := uitheme.Of(env)
		shade, sill, border = uitheme.Fade(t.Fg, .62), ggui.ResolveColor(uitheme.Muted, env), ggui.ResolveColor(uitheme.Border, env)
		radius, fraction = t.RadiusSm, closed.Get()/100
		return cs.Constrain(ggui.Sz(cs.MaxW, 128))
	}, func(dst *ggui.Canvas, r ggui.Rect) {
		dst.ClipRoundRect(r, radius, func(clipped *ggui.Canvas) {
			clipped.FillRect(r, sill)
			clipped.FillRect(ggui.Rect{Origin: r.Origin, Size: ggui.Sz(r.Size.W, r.Size.H*fraction)}, shade)
		})
		dst.StrokeRoundRect(r, radius, 1, border)
	})
	set := func(label string, to float64) ggui.Widget {
		return ggui.Expanded(ui.Button(label, func() { closed.Set(to) }).Outline().Name(label + " shades").Key(p.key("shades-" + label)))
	}
	return previewFooterCard(previewHeader("Living Room", "Roller Shades"), ggui.Column(
		window,
		ggui.Row(ui.Caption("OPEN").Size(12), ggui.Expanded(ui.Slider(closed, 0, 100).Step(5).Name("Shade position").Key(p.key("shades-position"))), ui.Caption("CLOSE").Size(12)).Gap(12).Align(ggui.AlignCenter),
	).Gap(16).Stretch(), ggui.Row(set("Open", 0), set("Half", 50), set("Closed", 100)).Gap(8))
}

func (p *previewModel) stockCard() ggui.Widget {
	months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun"}
	prices := map[string][]float64{
		"VOO":  {412, 438, 395, 446, 418, 460},
		"VIG":  {168, 172, 165, 176, 181, 179},
		"AAPL": {185, 172, 169, 190, 204, 213},
	}
	series := func(ticker string) []ui.ChartDatum {
		data := make([]ui.ChartDatum, len(months))
		for i, month := range months {
			data[i] = ui.ChartDatum{Label: month, Values: map[string]float64{"price": prices[ticker][i]}}
		}
		return data
	}
	ticker := ggui.State("VOO")
	chart := ui.AreaChart(series("VOO"), ui.ChartConfig{{Key: "price", Label: "Price", ColorIndex: 2}}).
		Name("Price history").Key(p.key("stock-chart")).Height(196).Grid(true).Axes(true, false).Domain(0, 480).FillOpacity(.6).
		Tooltip(ui.ChartTooltipOptions{FormatValue: func(value float64, _ ui.ChartSeries, _ ui.ChartDatum) string { return previewMoney("USD", value) }})
	picker := ui.Select(ticker).Options([]string{"VOO", "VIG", "AAPL"}).Name("Ticker").Key(p.key("stock-ticker")).
		OnChange(func(t string) { chart.Data(series(t)).Domain(0, 1.05*slices.Max(prices[t])) })
	return previewCard("Stock Performance", "6-month price history.", ggui.Column(
		ui.Field("Ticker", picker), ui.Divider(), chart,
	).Gap(16).Stretch())
}

func (p *previewModel) catalogCard() ggui.Widget {
	return previewEmpty(glyph("audio"), "Explore Catalog",
		"Check your ISRC codes, metadata, and visual assets before going live.",
		ui.Button("View Catalog", func() { p.say("The catalog is empty in this preview.") }).Key(p.key("view-catalog")))
}

func (p *previewModel) milestoneCard() ggui.Widget {
	name, amount, date := ggui.State(""), ggui.State("$15,000"), ggui.State("Dec 2025")
	return previewFooterCard(previewHeader("Set a new milestone", "Define your financial target and we'll help you pace your savings."), ggui.Column(
		ui.Field("Goal Name", ui.TextField(name).Name("Goal name").MinWidth(0).Placeholder("e.g. New Car, Home Downpayment").Key(p.key("milestone-name"))),
		ggui.Row(
			ggui.Expanded(ui.Field("Target Amount", ui.TextField(amount).Name("Target amount").MinWidth(0).Key(p.key("milestone-amount")))),
			ggui.Expanded(ui.Field("Target Date", ui.TextField(date).Name("Target date").MinWidth(0).Key(p.key("milestone-date")))),
		).Gap(12).Align(ggui.AlignStart),
	).Gap(16).Stretch(), ggui.Column(
		ui.Button("Create Goal", func() {
			goal := strings.TrimSpace(ggui.Peek(name))
			if goal == "" {
				p.say("Name the goal to create it.")
				return
			}
			p.say(goal + ": " + ggui.Peek(amount) + " by " + ggui.Peek(date) + ". Saved in this preview.")
		}).Key(p.key("create-goal")),
		ui.Button("Cancel", func() { name.Set(""); amount.Set("$15,000"); date.Set("Dec 2025") }).Outline().Name("Cancel goal").Key(p.key("cancel-goal")),
	).Gap(8).Stretch())
}

func (p *previewModel) socialCard() ggui.Widget {
	type link struct {
		value                   *ggui.StateValue[string]
		label, icon, hint, seed string
	}
	links := []link{
		{label: "Spotify Artist URL", icon: "circle-plus", hint: "spotify.com/artist/…", seed: "spotify.com/artist/3j…2k"},
		{label: "Instagram Handle", icon: "camera", hint: "@handle", seed: "@julianduryea_music"},
		{label: "SoundCloud URL", icon: "cloud", hint: "soundcloud.com/username"},
		{label: "Website", icon: "globe", hint: "https://yoursite.com"},
	}
	fields := make([]ggui.Widget, len(links))
	for i := range links {
		l := &links[i]
		l.value = ggui.State(l.seed)
		fields[i] = ui.Field(l.label, ui.InputGroup(ggui.TextInput(l.value).MinWidth(0).Placeholder(l.hint).Key(p.key("social-"+l.label))).
			Leading(glyph(l.icon).Color(uitheme.MutedFg)).Name(l.label))
	}
	return previewFooterCard(ui.Heading("Social Links").Size(16).Weight(ggui.WeightMedium), ggui.Column(fields...).Gap(16).Stretch(),
		ggui.Row(ggui.Spacer(),
			ui.Button("Discard", func() {
				for _, l := range links {
					l.value.Set(l.seed)
				}
			}).Secondary().Name("Discard social links").Key(p.key("discard-social")),
			ui.Button("Save Changes", func() {
				n := 0
				for _, l := range links {
					if strings.TrimSpace(ggui.Peek(l.value)) != "" {
						n++
					}
				}
				p.say(fmt.Sprintf("Saved %d of %d links in this preview.", n, len(links)))
			}).Key(p.key("save-social")),
		).Gap(8).Align(ggui.AlignCenter))
}

func (p *previewModel) notificationsCard() ggui.Widget {
	type topic struct {
		on                 *ggui.StateValue[bool]
		title, description string
	}
	topics := []topic{
		{ggui.State(true), "Transaction alerts", "Deposits, withdrawals, and transfers."},
		{ggui.State(true), "Security alerts", "Login attempts and account changes."},
		{ggui.State(false), "Goal milestones", "Updates at 25%, 50%, 75%, and 100%."},
		{ggui.State(false), "Market updates", "Daily portfolio summary and price alerts."},
	}
	count := func(read func(ggui.Readable[bool]) bool) int {
		n := 0
		for _, t := range topics {
			if read(t.on) {
				n++
			}
		}
		return n
	}
	// Select all is every topic at once: checked when all are, a dash when
	// only some are.
	all := ggui.State(false)
	some := ggui.Derived(func() bool {
		n := count(func(s ggui.Readable[bool]) bool { return s.Get() })
		return n > 0 && n < len(topics)
	})
	rows := []ggui.Widget{ui.Checkbox(all, "Select all").BindIndeterminate(some).Key(p.key("notify-all")).OnChange(func(on bool) {
		for _, t := range topics {
			t.on.Set(on)
		}
	})}
	for _, t := range topics {
		rows = append(rows, ggui.Column(
			ui.Checkbox(t.on, t.title).Key(p.key("notify-"+t.title)).OnChange(func(bool) {
				all.Set(count(ggui.Peek[bool]) == len(topics))
			}),
			ggui.Padding(ui.Caption(t.description).Size(14), 0, 0, 0, 24),
		).Gap(4).Stretch())
	}
	return previewFooterCard(previewHeader("Notifications", "Choose what you want to be notified about."),
		ggui.Column(rows...).Gap(14).Stretch(),
		ui.Button("Save Preferences", func() {
			p.announce(p.notificationsFeedback, fmt.Sprintf("Saved: %d of %d notifications enabled.", count(ggui.Peek[bool]), len(topics)))
		}).Name("Save notifications").Key(p.key("save-notifications")))
}
