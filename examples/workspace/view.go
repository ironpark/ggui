package main

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func build(m *model) ggui.Widget {
	toasts := ui.NewToaster().Limit(1)
	ggui.OnCleanup(toasts.Close)

	content := ggui.Column(
		header(m),
		metrics(m),
		ggui.Column(
			ggui.Row(
				ggui.Caption("RELEASE PROGRESS"),
				ggui.Spacer(),
				ggui.Textf("%d of %d complete",
					m.Summary.Map(func(s summary) int { return s.Done }),
					m.Summary.Map(func(s summary) int { return s.Total }),
				).AsCaption(),
			),
			ui.Progress(m.Progress).Height(5),
		).Gap(8).Align(ggui.AlignStretch),
		ui.Tabs(m.Tab,
			ui.Tab("Tasks", taskList(m)),
			ui.Tab("Insights", insights(m)),
			ui.Tab("Settings", settings(m)),
		).Line(),
		ggui.Caption("⌘/Ctrl+N  New task · F1  Inspector · Changes last until the window closes."),
	).Gap(16).Align(ggui.AlignStretch)

	return ggui.Column(
		ggui.Expanded(ggui.Scroll(ggui.Padding(ggui.Align(ggui.Box(content).Width(1120)).Top(), 24))),
		taskEditor(m, func() {
			if m.save() {
				toasts.Push(ui.Toast("Task saved", "The list and insights are up to date."))
			}
		}),
		ui.AlertDialog(m.Confirm, "Delete selected task?", "This removes the task from this workspace.").
			Confirm("Delete task", func() {
				m.delete()
				toasts.Push(ui.Toast("Task deleted", "The remaining tasks are unchanged."))
			}).Destructive(),
		toasts,
	).Align(ggui.AlignStretch)
}

func header(m *model) ggui.Widget {
	title := ggui.Column(
		ggui.Row(ggui.Caption("PROJECT / LAUNCH"), ui.Badge("Local demo")).Gap(12),
		ggui.Title("Launch workspace").Size(30),
		ggui.Caption("A clear view of the work ahead."),
	).Gap(8)
	actions := ggui.Row(
		ui.ThemeSwitch(m.Dark),
		ui.Tooltip(ui.Button("New task", m.create), "Create a task (⌘/Ctrl+N)"),
	).Gap(12).Align(ggui.AlignCenter)
	return &adaptive{
		breakpoint: 680,
		wide:       ggui.Row(ggui.Expanded(title), actions).Gap(24).Align(ggui.AlignCenter),
		narrow:     ggui.Column(title, actions).Gap(16).Align(ggui.AlignStretch),
	}
}

func metrics(m *model) ggui.Widget {
	card := func(label string, value ggui.Readable[int]) ggui.Widget {
		return ggui.Box(ui.Card(ggui.Column(
			ggui.Caption(label),
			ggui.Textf("%d", value).AsTitle().Size(30),
		).Gap(6)).Pad(16)).BindWidth(m.CardWidth)
	}
	cards := []ggui.Widget{
		card("All tasks", m.Summary.Map(func(s summary) int { return s.Total })),
		card("In progress", m.Summary.Map(func(s summary) int { return s.Active })),
		card("Completed", m.Summary.Map(func(s summary) int { return s.Done })),
	}
	return &adaptive{
		breakpoint: 980,
		wide:       ggui.Wrap(cards...).Gap(12),
		narrow:     ggui.Grid(3, cards...).Gap(8),
	}
}

func taskList(m *model) ggui.Widget {
	noSelection := m.Selected.Map(func(id int) bool { return id == 0 })
	selectedTitle := ggui.Combine(m.Tasks, m.Selected, func(tasks []task, id int) string {
		if item, ok := findTask(tasks, id); ok {
			return "Selected: " + item.Title
		}
		return "Select a row to edit or delete it."
	})
	table := ui.Table(m.Visible, func(item task) int { return item.ID },
		ui.TextCol("Task", func(item task) string { return item.Title }).Grow(3),
		ui.TextCol("Assignee", func(item task) string { return item.Assignee }),
		ui.TextCol("Status", func(item task) string { return item.Status }),
	).BindSelected(m.Selected).RowName(func(item task) string { return item.Title }).Height(210)

	compactList := ggui.EachKeyed(m.Visible, func(item task) int { return item.ID },
		func(row ggui.EachItem[task]) ggui.Widget {
			id := row.Value.Get().ID
			label := ggui.Map(row.Value, func(item task) string { return item.Title })
			detail := ggui.Map(row.Value, func(item task) string { return item.Assignee + " · " + item.Status })
			return ui.ButtonOf(ggui.Column(
				ggui.TextOf(label),
				ggui.TextOf(detail).AsCaption(),
			).Gap(4).Align(ggui.AlignStretch), func() { m.Selected.Set(id) }).
				BindName(label).Outline().Pad(12)
		},
	).Gap(8)
	rows := &adaptive{breakpoint: 620, wide: table, narrow: compactList}
	search := ui.TextField(m.Query).Name("Search tasks").Placeholder("Search titles or people…")
	filters := ui.ToggleGroup(m.Filter).
		Options([]string{"All", "Planned", "In progress", "Done"}).Name("Task filter")
	toolbar := &adaptive{
		breakpoint: 760,
		wide:       ggui.Row(ggui.Expanded(search), filters).Gap(16).Align(ggui.AlignCenter),
		narrow:     ggui.Column(search, ggui.Align(filters).Left()).Gap(12).Align(ggui.AlignStretch),
	}
	selection := ggui.TextOf(selectedTitle).AsCaption()
	actions := ggui.Wrap(
		ui.Button("Edit selected", m.edit).Outline().BindDisabled(noSelection),
		ui.Button("Delete selected", m.askDelete).Outline().BindDisabled(noSelection),
	).Gap(8)
	selectionBar := &adaptive{
		breakpoint: 760,
		wide:       ggui.Row(ggui.Expanded(selection), actions).Gap(16).Align(ggui.AlignCenter),
		narrow:     ggui.Column(selection, actions).Gap(12).Align(ggui.AlignStretch),
	}
	return ui.Card(ggui.Column(
		ggui.Row(
			ggui.Title("Tasks").Size(20),
			ggui.Spacer(),
			ggui.Textf("%d results", m.Visible.Map(func(tasks []task) int { return len(tasks) })).AsCaption(),
		).Align(ggui.AlignCenter),
		toolbar,
		selectionBar,
		ui.Divider(),
		ggui.If(m.Visible.Map(func(tasks []task) bool { return len(tasks) != 0 }),
			func() ggui.Widget { return rows },
		).Else(func() ggui.Widget {
			return ui.Empty("No matching tasks", "Try another search or create a task.").
				Action(ui.Button("Clear filters", func() {
					m.Query.Set("")
					m.Filter.Set("All")
				}).Outline())
		}),
	).Gap(16).Align(ggui.AlignStretch)).Pad(20)
}

func taskEditor(m *model, save func()) ggui.Widget {
	title := m.Draft.Field(func(item *task) *string { return &item.Title })
	assignee := m.Draft.Field(func(item *task) *string { return &item.Assignee })
	status := m.Draft.Field(func(item *task) *string { return &item.Status })
	invalid := m.Validation.Map(func(message string) bool { return message != "" })

	return ui.Dialog(m.Editing, ggui.Column(
		ui.Field("Task title", ui.TextField(title).Name("Task title").OnSubmit(func(string) { save() })).
			BindError(m.Validation),
		// Both controls borrow the same options source. Changing the team in
		// Settings updates this list without rebuilding the editor.
		ui.Field("Assignee", ui.Combobox(assignee).BindOptions(m.Team).Name("Task assignee")),
		ui.Field("Status", ui.Select(status).Options(statuses).Name("Task status")),
		ggui.Row(
			ui.Button("Cancel edit", func() { m.Editing.Set(false) }).Outline(),
			ui.Button("Save task", save).BindDisabled(invalid),
		).Gap(8).Justify(ggui.JustifyEnd),
	).Gap(16).Align(ggui.AlignStretch)).Title("Task details").Width(440)
}

func insights(m *model) ggui.Widget {
	chart := ggui.View(m.Summary, func(s summary) ggui.Widget {
		return ui.BarChart([]ui.ChartDatum{
			{Label: "Planned", Values: map[string]float64{"tasks": float64(s.Total - s.Active - s.Done)}},
			{Label: "In progress", Values: map[string]float64{"tasks": float64(s.Active)}},
			{Label: "Done", Values: map[string]float64{"tasks": float64(s.Done)}},
		}, ui.ChartConfig{{Key: "tasks", Label: "Tasks"}}).Height(220)
	})
	activity := ggui.View(m.Activity, func(entries []string) ggui.Widget {
		return ggui.List(entries, func(entry string) ggui.Widget {
			return ggui.Text(entry)
		}).Gap(8)
	})
	return ggui.Column(
		ui.Card(ggui.Column(
			ggui.Title("Work by status").Size(20),
			ggui.Caption("All tasks in this workspace, independent of the current filter."),
			chart,
		).Gap(12).Align(ggui.AlignStretch)),
		ui.Card(ui.Collapsible(ggui.State(true), "Recent activity", activity)),
	).Gap(16).Align(ggui.AlignStretch)
}

func settings(m *model) ggui.Widget {
	previewAssignee := ggui.State("Ada")
	return ui.Card(ggui.Column(
		ggui.Title("Workspace preferences").Size(20),
		ui.Switch(m.Extended, "Include extended team"),
		ui.Field("Available people", ui.Select(previewAssignee).BindOptions(m.Team).Name("Available people")).
			Help("This list and the task editor share a live options source. Existing assignments are preserved."),
		ui.Field("Metric card width", ui.Slider(m.CardWidth, 200, 360).Step(10).Name("Metric card width")).
			Help("Adjust the desktop cards. Narrow windows use three compact, equal columns."),
		ggui.Textf("Card width: %.0f px", m.CardWidth).AsCaption(),
		ui.Alert("Try the inspector", "Press F1 to explore layout, hit regions, and the widget tree."),
	).Gap(16).Align(ggui.AlignStretch))
}
