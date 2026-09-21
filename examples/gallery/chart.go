package main

import "github.com/ironpark/ggui/ui"

// chartPreview adapts the shadcn/ui gradient area chart (see CHART-NOTICE).
func chartPreview() *ui.ChartWidget {
	data := []ui.ChartDatum{
		{Label: "January", Values: map[string]float64{"desktop": 186, "mobile": 80}},
		{Label: "February", Values: map[string]float64{"desktop": 305, "mobile": 200}},
		{Label: "March", Values: map[string]float64{"desktop": 237, "mobile": 120}},
		{Label: "April", Values: map[string]float64{"desktop": 73, "mobile": 190}},
		{Label: "May", Values: map[string]float64{"desktop": 209, "mobile": 130}},
		{Label: "June", Values: map[string]float64{"desktop": 214, "mobile": 140}},
	}
	opacity := .4
	config := ui.ChartConfig{
		{Key: "mobile", Label: "Mobile", ColorIndex: 2, FillOpacity: &opacity},
		{Key: "desktop", Label: "Desktop", ColorIndex: 1, FillOpacity: &opacity},
	}
	return ui.ChartContainer(ui.ChartArea, data, config).
		Name("Area Chart - Gradient").Height(240).
		TickFormatter(func(s string) string {
			if len(s) > 3 {
				return s[:3]
			}
			return s
		}).
		Stack(ui.ChartStacked).Gradient(true).Legend(true).
		TooltipCursor(false).Tooltip(ui.ChartTooltipOptions{Indicator: ui.ChartIndicatorLine})
}
