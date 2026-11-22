package main

import "github.com/ironpark/ggui/ui/chart"

// chartPreview adapts the shadcn/ui gradient area chart (see CHART-NOTICE).
func chartPreview() *chart.Widget {
	data := []chart.Datum{
		{Label: "January", Values: map[string]float64{"desktop": 186, "mobile": 80}},
		{Label: "February", Values: map[string]float64{"desktop": 305, "mobile": 200}},
		{Label: "March", Values: map[string]float64{"desktop": 237, "mobile": 120}},
		{Label: "April", Values: map[string]float64{"desktop": 73, "mobile": 190}},
		{Label: "May", Values: map[string]float64{"desktop": 209, "mobile": 130}},
		{Label: "June", Values: map[string]float64{"desktop": 214, "mobile": 140}},
	}
	opacity := .4
	config := chart.Config{
		{Key: "mobile", Label: "Mobile", ColorIndex: 2, FillOpacity: &opacity},
		{Key: "desktop", Label: "Desktop", ColorIndex: 1, FillOpacity: &opacity},
	}
	return chart.Container(chart.KindArea, data, config).
		Name("Area Chart - Gradient").Height(240).
		TickFormatter(func(s string) string {
			if len(s) > 3 {
				return s[:3]
			}
			return s
		}).
		Stack(chart.StackStacked).Gradient(true).Legend(true).
		TooltipCursor(false).Tooltip(chart.TooltipOptions{Indicator: chart.IndicatorLine})
}
