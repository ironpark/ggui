package ui

import (
	"fmt"
	"image/color"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// ChartTooltipContent is reusable tooltip content configured for a chart. It
// uses the same data/config labels, colors, icons and formatters as hover.
func ChartTooltipContent(chart *ChartWidget, index int) ggui.Widget {
	return &chartContent{chart: chart, index: index}
}

// ChartTooltip is the standalone form of ChartTooltipContent. The chart's
// Tooltip setter controls the built-in floating tooltip.
func ChartTooltip(chart *ChartWidget, index int) ggui.Widget {
	return ChartTooltipContent(chart, index)
}

// ChartLegendContent renders the chart's series or category legend on its own.
func ChartLegendContent(chart *ChartWidget) ggui.Widget {
	return &chartContent{chart: chart, legend: true}
}
func ChartLegend(chart *ChartWidget) ggui.Widget { return ChartLegendContent(chart) }

type chartContent struct {
	chart   *ChartWidget
	index   int
	legend  bool
	context ChartWidget
	rows    []chartContentRow
}

func (w *chartContent) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	// The legend and tooltip renderers are ChartWidget methods, but they read
	// only the fields below. Listing them beats copying the whole widget: the
	// standalone content then shares no interaction, animation or geometry
	// state with the live chart, and a new ChartWidget field cannot silently
	// join the copy.
	c := w.chart
	w.context = ChartWidget{
		kind:           c.kind,
		data:           c.data,
		config:         c.config,
		tooltip:        c.tooltip,
		categoryColors: c.categoryColors,
		active:         c.active,
		env:            env,
		theme:          uitheme.Of(env),
		// The labels measured at the last layout carry over, to re-measure
		// under this one as Layout has them do on the chart itself.
		textCache:  w.context.textCache,
		textLayout: w.context.textLayout + 1,
	}
	if w.legend {
		return cs.Constrain(ggui.Sz(bounded(cs.MaxW, 240), w.context.legendHeight(bounded(cs.MaxW, 240))))
	}
	rows, size := w.context.tooltipRows(w.index)
	w.rows = rows
	return cs.Constrain(size)
}
func (w *chartContent) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if w.legend {
		w.context.paintLegend(dst, r)
	} else {
		w.context.paintTooltipContent(dst, r, w.index, w.rows)
	}
}

// categoryLegend reports whether the legend and tooltip name categories rather
// than series, which pie and radial charts do when colored per category.
func (c *ChartWidget) categoryLegend() bool {
	return c.categoryColors && (c.kind == ChartPie || c.kind == ChartRadial)
}

type chartContentRow struct {
	label, value string
	col          color.Color
	icon         icons.Role
}

func (c *ChartWidget) legendRows() []chartContentRow {
	rows := make([]chartContentRow, 0, len(c.config))
	if c.categoryLegend() {
		for i, d := range c.data {
			if len(c.config) > 0 {
				rows = append(rows, chartContentRow{label: d.Label, col: c.pointColor(i, 0), icon: d.Icon})
			}
		}
	} else {
		for j, s := range c.config {
			rows = append(rows, chartContentRow{label: seriesLabel(s), col: c.seriesColor(j), icon: s.Icon})
		}
	}
	return rows
}
func (c *ChartWidget) legendHeight(width float64) float64 {
	x := 0.
	lines := 1
	for _, row := range c.legendRows() {
		w := c.measuredText(row.label, 12, c.theme.Fg).size.W + chartLegendItem
		if x > 0 && x+w > width {
			lines++
			x = 0
		}
		x += w
	}
	return chartLegendInset + float64(lines)*chartLegendPitch - (chartLegendPitch - chartLine)
}

// The metrics of shadcn's chart legend and tooltip, which are set in 12px
// text on 16px lines whatever the theme's body text is.
const (
	chartLine        = 16. // a line of text-xs
	chartLegendInset = 12. // pt-3 between the plot and the legend
	chartLegendPitch = 24. // from one wrapped line of the legend to the next
	chartLegendItem  = 30. // an 8px swatch, 6px to its label, and gap-4 after it
	chartTipPadX     = 10. // px-2.5
	chartTipPadY     = 6.  // py-1.5
	chartTipGap      = 6.  // gap-1.5 between the heading and the rows
	chartTipRow      = 12. // a row is one leading-none line
	chartTipMinW     = 128.
)

// chartIndicatorWidth is how wide the mark before a tooltip row is.
func chartIndicatorWidth(kind ChartIndicator, icon bool) float64 {
	switch {
	case icon:
		return 10
	case kind == ChartIndicatorLine:
		return 4
	case kind == ChartIndicatorDashed:
		return 3
	}
	return 10
}
func (c *ChartWidget) paintLegend(dst *ggui.Canvas, r ggui.Rect) {
	rows := c.legendRows()
	widths := make([]float64, len(rows))
	for i, row := range rows {
		widths[i] = c.measuredText(row.label, 12, c.theme.Fg).size.W + chartLegendItem
	}
	y := r.Origin.Y + chartLegendInset + chartLine/2 // the middle of the first line
	for start := 0; start < len(rows); {
		end, total := start, 0.
		for end < len(rows) && (end == start || total+widths[end] <= r.Size.W) {
			total += widths[end]
			end++
		}
		x := r.Origin.X + max(0, (r.Size.W-total)/2)
		for i := start; i < end; i++ {
			row := rows[i]
			if row.icon != "" {
				paintIcon(dst, c.env, row.icon, ggui.Rct(ggui.Pt(x, y-6), ggui.Sz(12., 12.)), row.col, 0)
			} else {
				dst.FillRoundRect(ggui.Rct(ggui.Pt(x, y-4), ggui.Sz(8., 8.)), 2, row.col)
			}
			c.textMiddle(dst, row.label, ggui.Pt(x+14, y), chartRegular, c.theme.Fg, 0)
			x += widths[i]
		}
		y += chartLegendPitch
		start = end
	}
}
func (c *ChartWidget) tooltipRows(index int) ([]chartContentRow, ggui.Size) {
	if index < 0 || index >= len(c.data) {
		return nil, ggui.Size{}
	}
	rows := make([]chartContentRow, 0, len(c.config))
	width := chartTipMinW
	for j, s := range c.config {
		v, ok := c.value(index, j)
		if !ok {
			continue
		}
		label := seriesLabel(s)
		icon := s.Icon
		if c.categoryLegend() {
			label = c.data[index].Label
			icon = c.data[index].Icon
		}
		if c.tooltip.FormatName != nil {
			label = c.tooltip.FormatName(s, c.data[index])
		}
		value := c.formatValue(index, j, v)
		rows = append(rows, chartContentRow{label, value, c.pointColor(index, j), icon})
		w := c.measuredText(label, 12, c.theme.MutedFg).size.W + 2*c.theme.Space + c.measuredFace(value, 12, c.theme.Fg, chartMono).size.W
		if !c.tooltip.HideIndicator {
			w += chartIndicatorWidth(c.tooltip.Indicator, icon != "") + c.theme.Space
		}
		width = max(width, w+2*chartTipPadX)
	}
	// Rows of one line each with a gap between, inside the padding; the
	// heading is a full line above them, the total a row under a rule.
	height := 2*chartTipPadY + float64(len(rows))*(chartTipRow+chartTipGap) - chartTipGap
	if c.tooltip.ShowTotal {
		height += 2*chartTipGap + chartTipRow
	}
	if !c.tooltip.HideLabel {
		width = max(width, c.measuredFace(c.tooltipLabel(index), 12, c.theme.Fg, chartMedium).size.W+2*chartTipPadX)
		height += chartLine + chartTipGap
	}
	return rows, ggui.Sz(width, height)
}
func (c *ChartWidget) tooltipLabel(index int) string {
	label := c.data[index].Label
	if c.tooltip.Label != "" {
		label = c.tooltip.Label
	}
	if c.tooltip.FormatLabel != nil {
		label = c.tooltip.FormatLabel(label)
	}
	return label
}

// paintTooltipContent draws rows, which the caller sized with tooltipRows.
// Taking them as a parameter keeps that measuring pass to once per frame.
func (c *ChartWidget) paintTooltipContent(dst *ggui.Canvas, r ggui.Rect, index int, rows []chartContentRow) {
	if len(rows) == 0 {
		return
	}
	t := c.theme
	// shadcn's rounded-lg border-border/50 bg-background shadow-xl.
	paintShadows(dst, r, t.Radius, t.PanelShadow, t.PanelContact)
	dst.FillRoundRect(r, t.Radius, t.Bg)
	dst.StrokeRoundRect(r, t.Radius, t.BorderWidth, fade(t.Border, .5))
	left, right := r.Origin.X+chartTipPadX, r.Origin.X+r.Size.W-chartTipPadX
	y := r.Origin.Y + chartTipPadY // the top of the next line
	if !c.tooltip.HideLabel {
		c.textMiddle(dst, c.tooltipLabel(index), ggui.Pt(left, y+chartLine/2), chartMedium, t.Fg, 0)
		y += chartLine + chartTipGap
	}
	for _, row := range rows {
		x, mid := left, y+chartTipRow/2
		if !c.tooltip.HideIndicator {
			w := chartIndicatorWidth(c.tooltip.Indicator, row.icon != "")
			switch {
			case row.icon != "":
				paintIcon(dst, c.env, row.icon, ggui.Rct(ggui.Pt(x, mid-w/2), ggui.Sz(w, w)), row.col, 0)
			case c.tooltip.Indicator == ChartIndicatorLine:
				dst.FillRoundRect(ggui.Rct(ggui.Pt(x, y), ggui.Sz(w, chartTipRow)), 2, row.col)
			case c.tooltip.Indicator == ChartIndicatorDashed:
				for k := 0; k < 3; k++ {
					dst.FillRect(ggui.Rct(ggui.Pt(x, y+float64(k)*4.5), ggui.Sz(w, 3.)), row.col)
				}
			default:
				dst.FillRoundRect(ggui.Rct(ggui.Pt(x, mid-w/2), ggui.Sz(w, w)), 2, row.col)
			}
			x += w + t.Space
		}
		c.textMiddle(dst, row.label, ggui.Pt(x, mid), chartRegular, t.MutedFg, 0)
		c.textMiddle(dst, row.value, ggui.Pt(right, mid), chartMono, t.Fg, 1)
		y += chartTipRow + chartTipGap
	}
	if c.tooltip.ShowTotal {
		dst.StrokeLine(ggui.Pt(left, y), ggui.Pt(right, y), t.BorderWidth, fade(t.Border, .5))
		y += chartTipGap
		total := 0.
		for j := range c.config {
			if v, ok := c.value(index, j); ok {
				total += v
			}
		}
		label := c.tooltip.TotalLabel
		if label == "" {
			label = "Total"
		}
		value := fmt.Sprintf("%g", total)
		if len(c.config) > 0 {
			value = c.formatValue(index, 0, total)
		}
		c.textMiddle(dst, label, ggui.Pt(left, y+chartTipRow/2), chartMedium, t.Fg, 0)
		c.textMiddle(dst, value, ggui.Pt(right, y+chartTipRow/2), chartMono, t.Fg, 1)
	}

}
func (c *ChartWidget) paintTooltip(dst *ggui.Canvas, r ggui.Rect) {
	index := c.active
	rows, size := c.tooltipRows(index)
	var custom ggui.Widget
	if c.tooltip.Content != nil {
		custom = c.tooltip.Content(ChartTooltipPayload{index, c.data[index], append(ChartConfig(nil), c.config...)})
		if custom != nil {
			size = custom.Layout(ggui.Loose(dst.Size()), c.env)
		}
	}
	if len(rows) == 0 {
		return
	}
	p := c.plot.Center()
	if point, ok := dst.Pointer(); ok && c.Hovered {
		p = point
	} else if !c.kind.polar() && len(c.geometry.series) > 0 {
		p = chartPosition(c.plot, c.geometry.series[0].top[index])
	}
	bounds := dst.Size()
	if bounds.W <= 0 || bounds.H <= 0 {
		bounds = r.Size
	}
	x := p.X + 14
	if x+size.W > bounds.W-8 {
		x = p.X - size.W - 14
	}
	x = max(4, min(x, bounds.W-size.W-4))
	y := max(4, min(p.Y-size.H/2, bounds.H-size.H-4))
	box := ggui.Rct(ggui.Pt(x, y), size)
	dst.Overlay(func(overlay *ggui.Canvas) {
		if custom != nil {
			overlay.Paint(custom, box)
		} else {
			c.paintTooltipContent(overlay, box, index, rows)
		}
	})
}
