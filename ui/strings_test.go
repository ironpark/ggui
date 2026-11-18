package ui_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

// pseudo is DefaultStrings with every word in guillemets: a translation
// that leaves no English unnoticed, and keeps the formats' verbs where they
// were.
func pseudo() ui.Strings {
	s := ui.DefaultStrings()
	v := reflect.ValueOf(&s).Elem()
	for i := range v.NumField() {
		f := v.Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString(tr(f.String()))
		case reflect.Array:
			for j := range f.Len() {
				f.Index(j).SetString(tr(f.Index(j).String()))
			}
		}
	}
	return s
}

// tr is s as pseudo translates it.
func tr(s string) string { return "«" + s + "»" }

// said reports whether p shows or says text anywhere: as the label a probe
// finds a control by, or as a node's name, value or description.
func said(p *ggui.Probe, text string) bool {
	if _, ok := p.Find(text); ok {
		return true
	}
	for _, n := range p.Semantics().All() {
		if n.Name == text || n.Value == text || n.Description == text {
			return true
		}
	}
	return false
}

// open is an open binding for an overlay under test.
func open() *ggui.StateValue[bool] { return ggui.State(true) }

// localized are the controls of the package with what each says by itself,
// as pseudo words it.
var localized = []struct {
	name  string
	build func() ggui.Widget
	act   func(p *ggui.Probe, say func(string) string) // after the first frame, to show what the control says later; say words a pseudo word in the probe's language
	want  []string
}{
	{name: "dialog", build: func() ggui.Widget { return ui.Dialog(open(), widget.Text("body")).Title("Title") },
		want: []string{tr("Close")}},
	{name: "sheet", build: func() ggui.Widget { return ui.Sheet(open(), widget.Text("body")).Title("Title") },
		want: []string{tr("Close")}},
	{name: "command", build: func() ggui.Widget {
		return ui.Command(ggui.State("zzz"), ui.CommandItem("Open", nil)).Hints()
	}, want: []string{tr("Search commands"), tr("No results"), "↑↓ " + tr("Navigate") + "   ↵ " + tr("Select")}},
	{name: "command dialog", build: func() ggui.Widget {
		return ui.CommandDialog(open(), ui.Command(ggui.State(""), ui.CommandItem("Open", nil)))
	}, want: []string{tr("Commands")}},
	{name: "combobox", build: func() ggui.Widget {
		return ui.Combobox(ggui.State(1)).Options([]int{1, 2}).ClearButton(true)
	}, want: []string{tr("Choose option"), tr("Clear")}},
	{name: "combobox chevron", build: func() ggui.Widget {
		return ui.Combobox(ggui.State(0)).Options([]int{1, 2})
	}, want: []string{tr("Show options")}},
	{name: "combobox multiple", build: func() ggui.Widget {
		return ui.ComboboxMultiple(ggui.State([]string{"a"})).Options([]string{"a", "b"})
	}, want: []string{tr("Choose options"), fmt.Sprintf(tr("Remove %s"), "a")}},
	{name: "data table", build: func() ggui.Widget {
		return ui.DataTable(dataModel(people()))
	}, want: []string{tr("Filter rows"), tr("Columns"), fmt.Sprintf(tr("Sort %s"), "Name"), tr("Select page"),
		fmt.Sprintf(tr("Select row %v"), 1), fmt.Sprintf(tr("%d of %d row(s) selected."), 0, 3),
		fmt.Sprintf(tr("Page %d of %d"), 1, 1), tr("Rows per page"), tr("Previous"), tr("Next")}},
	{name: "data table empty", build: func() ggui.Widget {
		m := dataModel(people())
		m.SetQuery("nobody")
		return ui.DataTable(m)
	}, want: []string{tr("No results.")}},
	{name: "pagination", build: func() ggui.Widget {
		return ui.Pagination(ggui.State(1), ggui.State(10))
	}, want: []string{tr("Previous"), tr("Next"), fmt.Sprintf(tr("Page %d"), 2), tr("Pagination")}},
	{name: "calendar", build: func() ggui.Widget {
		return ui.Calendar(ggui.State(time.Date(2026, time.November, 18, 0, 0, 0, 0, time.Local)))
	}, want: []string{tr("Calendar"), tr("Previous month"), tr("Next month"), tr("Su"), fmt.Sprintf(tr("%s %d"), tr("November"), 2026)}},
	{name: "calendar dropdown", build: func() ggui.Widget {
		return ui.Calendar(ggui.State(time.Date(2026, time.November, 18, 0, 0, 0, 0, time.Local))).CaptionDropdown(true)
	}, want: []string{tr("Month"), tr("Year")}},
	{name: "date picker", build: func() ggui.Widget { return ui.DatePicker(ggui.State(time.Time{})) },
		want: []string{tr("Choose date")}},
	{name: "date range picker", build: func() ggui.Widget { return ui.DateRangePicker(ggui.State(ui.DateRange{})) },
		want: []string{tr("Choose dates")}},
	{name: "carousel", build: func() ggui.Widget {
		return ui.Carousel(ggui.State(0), widget.Text("one"), widget.Text("two"))
	}, want: []string{tr("Carousel"), tr("Previous slide"), tr("Next slide"), fmt.Sprintf(tr("Slide %d of %d"), 1, 2)}},
	{name: "code block", build: func() ggui.Widget { return ui.CodeBlock("x := 1") },
		want: []string{tr("Copy code"), tr("Code content"), tr("Code")}},
	{name: "questionnaire", build: func() ggui.Widget {
		return ui.Questionnaire(ggui.State(ui.QuestionAnswers{}),
			ui.Question{Name: "a", Title: "A", Choices: []ui.QuestionOption{{Value: "x", Label: "X"}}},
			ui.Question{Name: "b", Title: "B", Required: true, Choices: []ui.QuestionOption{{Value: "y", Label: "Y"}}})
	}, act: func(p *ggui.Probe, say func(string) string) { p.Tap(say(tr("Next"))) },
		want: []string{tr("Questionnaire"), fmt.Sprintf(tr("Question %d of %d"), 1, 2), tr("Skip"), tr("Next"),
			tr("Choose an answer or skip this question."), tr("Questionnaire progress")}},
	{name: "questionnaire submit", build: func() ggui.Widget {
		return ui.Questionnaire(ggui.State(ui.QuestionAnswers{}),
			ui.Question{Name: "a", Title: "A", Required: true, Choices: []ui.QuestionOption{{Value: "x", Label: "X"}}})
	}, want: []string{tr("Submit")}},
	{name: "no questions", build: func() ggui.Widget { return ui.Questionnaire(ggui.State(ui.QuestionAnswers{})) },
		want: []string{tr("No questions"), tr("There are no enabled questions.")}},
	{name: "alert dialog", build: func() ggui.Widget { return ui.AlertDialog(open(), "Delete?", "Gone for good.") },
		want: []string{tr("Continue"), tr("Cancel")}},
	{name: "toast", build: func() ggui.Widget {
		toaster := ui.NewToaster()
		toaster.Push(ui.Toast("Saved", "").Duration(0))
		return toaster
	}, want: []string{fmt.Sprintf(tr("Dismiss %s"), "Saved")}},
	{name: "number input", build: func() ggui.Widget { return ui.NumberInput(ggui.State(1.0)) },
		want: []string{tr("Decrement"), tr("Increment")}},
	{name: "message scroller", build: func() ggui.Widget {
		return ui.MessageScroller(ggui.State([]ui.MessageEntry{{ID: "1", Content: widget.Text("hi")}}))
	}, want: []string{tr("Conversation")}},
	{name: "theme switch", build: func() ggui.Widget { return ui.ThemeSwitch(ggui.State(false)) },
		want: []string{tr("Dark mode")}},
	{name: "theme mode", build: func() ggui.Widget { return ui.ThemeMode(ggui.State(uitheme.ModeSystem)) },
		want: []string{tr("Theme"), tr("Light"), tr("Dark"), tr("System")}},
	{name: "spinner", build: func() ggui.Widget { return ui.Spinner() }, want: []string{tr("Loading")}},
	{name: "sidebar trigger", build: func() ggui.Widget { return ui.SidebarTrigger(ggui.State(false)) },
		want: []string{tr("Toggle Sidebar")}},
	{name: "groups", build: func() ggui.Widget {
		return widget.Column(
			ui.ButtonGroup(ui.Button("A", nil)),
			ui.ToggleGroup(ggui.State(1)).Options([]int{1, 2}),
			ui.Breadcrumb(ui.Crumb("Home", nil), ui.Crumb("Here", nil)),
		)
	}, want: []string{tr("Actions"), tr("Options"), tr("Breadcrumb")}},
	{name: "unnamed controls", build: func() ggui.Widget {
		return widget.Column(
			ui.Accordion(ggui.State([]string(nil)), ui.AccordionItem("a", "Section", widget.Text("body"))),
			ui.ContextMenu(widget.Box().Size(40, 20), ui.MenuItem("Copy", nil)),
			ui.Menubar(ui.Menu("File", ui.MenuItem("New", nil))),
			ui.NavigationMenu(ui.NavigationMenuLink("Home", nil)),
			ui.InputOTP(ggui.State(""), 4),
			ui.AttachmentGroup(ui.Attachment("a.txt", "")),
			ui.Trace(ui.TraceStep{ID: "run", Title: "Run", Status: ui.TraceRunning}),
		)
	}, want: []string{tr("Accordion"), tr("Context menu"), tr("Menubar"), tr("Navigation"), tr("One-time password"),
		fmt.Sprintf(tr("%d-character code"), 4), tr("Attachments"), tr("done"), tr("Execution trace"), "Run: " + tr("running")}},
	{name: "chart", build: func() ggui.Widget {
		return ui.BarChart([]ui.ChartDatum{{Label: "a", Values: map[string]float64{"v": 1}}}, ui.ChartConfig{{Key: "v"}})
	}, want: []string{tr("Chart"), tr("Chart. Use arrow keys to explore values; Enter selects a category.")}},
	{name: "range slider", build: func() ggui.Widget { return ui.RangeSlider(ggui.State([]float64{20, 60}), 0, 100) },
		want: []string{fmt.Sprintf(tr("%s %d"), tr("Value"), 1)}},
	{name: "resizable", build: func() ggui.Widget { return ui.Resizable(ggui.State(.5), widget.Box(), widget.Box()) },
		want: []string{tr("Resize panels")}},
	{name: "editor tab", build: func() ggui.Widget { return ui.EditorTab("a.go", nil).OnClose(func() {}) },
		want: []string{fmt.Sprintf(tr("Close %s"), "a.go")}},
	{name: "avatar group", build: func() ggui.Widget {
		return ui.AvatarGroup(ui.Avatar("Ann"), ui.Avatar("Bo"), ui.Avatar("Cy")).Max(1)
	}, want: []string{fmt.Sprintf(tr("%d more"), 2)}},
}

// TestStringsWordTheControls provides a translation and finds every control
// by what it says in it: no English is left where a word was replaced.
func TestStringsWordTheControls(t *testing.T) {
	t.Parallel()
	for _, tc := range localized {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := ggui.NewProbe(ui.ProvideStrings(pseudo(), tc.build()), ggui.Sz(900, 700))
			defer p.Close()
			p.Frame()
			if tc.act != nil {
				tc.act(p, func(s string) string { return s })
			}
			for _, want := range tc.want {
				if !said(p, want) {
					t.Errorf("nothing says %q in\n%s", want, p.Semantics())
				}
			}
		})
	}
}

// plain is a pseudo word in the English it stands for.
func plain(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "«", ""), "»", "") }

// TestStringsAreEnglishWithoutAProvider shows the same controls under no
// Strings, and under empty ones, which fall back field by field: both say
// what DefaultStrings says.
func TestStringsAreEnglishWithoutAProvider(t *testing.T) {
	t.Parallel()
	for _, provided := range []bool{false, true} {
		for _, tc := range localized {
			t.Run(tc.name+strconv.FormatBool(provided), func(t *testing.T) {
				t.Parallel()
				root := tc.build()
				if provided {
					root = ui.ProvideStrings(ui.Strings{}, root)
				}
				p := ggui.NewProbe(root, ggui.Sz(900, 700))
				defer p.Close()
				p.Frame()
				if tc.act != nil {
					tc.act(p, plain)
				}
				for _, want := range tc.want {
					if !said(p, plain(want)) {
						t.Errorf("nothing says %q in\n%s", plain(want), p.Semantics())
					}
				}
			})
		}
	}
}

// TestStringsFallBackFieldByField provides a few words and keeps the rest
// English.
func TestStringsFallBackFieldByField(t *testing.T) {
	t.Parallel()
	var got ui.Strings
	probe := ggui.FromFuncs(func(c ggui.Constraints, env ggui.Env) ggui.Size {
		got = ui.StringsFrom(env)
		return ggui.Size{}
	}, func(*ggui.Canvas, ggui.Rect) {})
	weekdays := [7]string{"일", "월", "화", "수", "목", "금", ""}
	p := ggui.NewProbe(ui.ProvideStrings(ui.Strings{Next: "다음", Weekdays: weekdays}, probe), ggui.Sz(10, 10))
	defer p.Close()
	p.Frame()
	want := ui.DefaultStrings()
	want.Next = "다음"
	want.Weekdays = [7]string{"일", "월", "화", "수", "목", "금", "Sa"}
	if got != want {
		t.Fatalf("StringsFrom = %+v\nwant %+v", got, want)
	}
	if ui.StringsFrom(ggui.Env{}) != ui.DefaultStrings() {
		t.Fatal("an Env with no Strings is not worded in DefaultStrings")
	}
}

// TestStringsLoseToSetters gives controls words of their own under a
// translation: each keeps its own.
func TestStringsLoseToSetters(t *testing.T) {
	t.Parallel()
	date := time.Date(2026, time.November, 18, 0, 0, 0, 0, time.Local)
	cases := []struct {
		name  string
		build func() ggui.Widget
		want  []string
		not   []string
	}{
		{"pagination labels", func() ggui.Widget {
			return ui.Pagination(ggui.State(1), ggui.State(3)).Labels("Back", "Forward")
		}, []string{"Back", "Forward"}, []string{tr("Previous"), tr("Next")}},
		{"command", func() ggui.Widget {
			return ui.Command(ggui.State("zzz")).Name("Find").Empty("Nothing").HintLabels("Move", "Pick").Hints()
		}, []string{"Find", "Nothing", "↑↓ Move   ↵ Pick"}, []string{tr("Search commands"), tr("No results")}},
		{"combobox", func() ggui.Widget {
			return ui.Combobox(ggui.State(0)).Options([]int{1}).Name("Size")
		}, []string{"Size"}, []string{tr("Choose option")}},
		{"calendar", func() ggui.Widget {
			return ui.Calendar(ggui.State(date)).Name("Due").
				WeekdayLabels([7]string{"S", "M", "T", "W", "T", "F", "S"}).
				MonthLabel(func(t time.Time) string { return t.Format("01/2006") })
		}, []string{"Due", "M", "11/2026"}, []string{tr("Calendar"), tr("Mo")}},
		{"date picker", func() ggui.Widget {
			return ui.DatePicker(ggui.State(time.Time{})).Name("When")
		}, []string{"When"}, []string{tr("Choose date")}},
		{"questionnaire", func() ggui.Widget {
			return ui.Questionnaire(ggui.State(ui.QuestionAnswers{}),
				ui.Question{Name: "a", Title: "A", Required: true, Choices: []ui.QuestionOption{{Value: "x", Label: "X"}}}).
				SubmitLabel("Send").Name("Survey")
		}, []string{"Send", "Survey"}, []string{tr("Submit"), tr("Questionnaire")}},
		{"alert dialog", func() ggui.Widget {
			return ui.AlertDialog(open(), "Delete?", "").Confirm("Delete", nil).Cancel("Keep")
		}, []string{"Delete", "Keep"}, []string{tr("Continue"), tr("Cancel")}},
		{"carousel", func() ggui.Widget {
			return ui.Carousel(ggui.State(0), widget.Text("one")).Name("Slides")
		}, []string{"Slides"}, []string{tr("Carousel")}},
		{"editor tab", func() ggui.Widget {
			return ui.EditorTab("a.go", nil).OnClose(func() {}).CloseName("Shut a.go")
		}, []string{"Shut a.go"}, []string{fmt.Sprintf(tr("Close %s"), "a.go")}},
		{"spinner", func() ggui.Widget { return ui.Spinner().Name("Saving") },
			[]string{"Saving"}, []string{tr("Loading")}},
		{"theme mode", func() ggui.Widget {
			return ui.ThemeMode(ggui.State(uitheme.ModeLight)).Name("Look").Format(func(m uitheme.Mode) string { return "M:" + string(m) })
		}, []string{"Look", "M:light"}, []string{tr("Theme"), tr("Light")}},
		{"groups", func() ggui.Widget {
			return widget.Column(ui.ButtonGroup(ui.Button("A", nil)).Name("Tools"), ui.Breadcrumb(ui.Crumb("Home", nil)).Name("Path"),
				ui.Trace(ui.TraceStep{ID: "run", Title: "Run"}).Name("Log"))
		}, []string{"Tools", "Path", "Log"}, []string{tr("Actions"), tr("Breadcrumb"), tr("Execution trace")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := ggui.NewProbe(ui.ProvideStrings(pseudo(), tc.build()), ggui.Sz(900, 700))
			defer p.Close()
			for _, want := range tc.want {
				if !said(p, want) {
					t.Errorf("nothing says %q in\n%s", want, p.Semantics())
				}
			}
			for _, not := range tc.not {
				if said(p, not) {
					t.Errorf("the Strings' %q beat the control's own word", not)
				}
			}
		})
	}
}

// TestBindStringsSwitchesAMountedControl switches the language of a
// mounted tree: the controls say the new words at the next frame, with
// nothing rebuilt.
func TestBindStringsSwitchesAMountedControl(t *testing.T) {
	t.Parallel()
	lang := ggui.State(ui.DefaultStrings())
	builds := 0
	p := ggui.ProbeBuilder(func() ggui.Widget {
		builds++
		return ui.BindStrings(lang, widget.Column(
			ui.Pagination(ggui.State(1), ggui.State(3)),
			ui.Dialog(open(), widget.Text("body")).Title("Title"),
			ui.Calendar(ggui.State(time.Date(2026, time.November, 18, 0, 0, 0, 0, time.Local))),
		))
	}, ggui.Sz(900, 700))
	defer p.Close()
	for _, want := range []string{"Next", "Close", "November 2026"} {
		if !said(p, want) {
			t.Fatalf("nothing says %q before the switch", want)
		}
	}
	ko := ui.Strings{Next: "다음", Close: "닫기", MonthTitleFormat: "%[2]d년 %[1]s",
		Months: [12]string{"1월", "2월", "3월", "4월", "5월", "6월", "7월", "8월", "9월", "10월", "11월", "12월"}}
	lang.Set(ko)
	p.Frame()
	for _, want := range []string{"다음", "닫기", "2026년 11월"} {
		if !said(p, want) {
			t.Errorf("nothing says %q after the switch in\n%s", want, p.Semantics())
		}
	}
	for _, gone := range []string{"Next", "Close", "November 2026"} {
		if said(p, gone) {
			t.Errorf("%q is still said after the switch", gone)
		}
	}
	lang.Set(ui.DefaultStrings())
	p.Frame()
	if !said(p, "Next") || said(p, "다음") {
		t.Error("switching back did not bring the English back")
	}
	if builds != 1 {
		t.Errorf("the tree was built %d times; a switch should only lay it out again", builds)
	}
}

// TestNoBuiltInWordOutsideTheStrings keeps the words of the package in
// strings.go: a string literal with a letter in it, handed to a setter or
// a constructor that puts it on the screen or in a screen reader's ear,
// is English no translation reaches.
func TestNoBuiltInWordOutsideTheStrings(t *testing.T) {
	t.Parallel()
	// The calls whose first argument is shown or said, and the fields of a
	// node that are.
	says := []string{"Name", "SetName", "Placeholder", "Alt", "Empty", "Labels", "HintLabels", "SubmitLabel",
		"Content", "Text", "Caption", "Heading", "Button", "MenuItem", "Menu", "Checkbox", "Switch", "Tooltip", "Toast", "relabel"}
	fields := []string{"Name", "Description", "Value"}
	hasLetter := func(lit *ast.BasicLit) bool {
		if lit.Kind != token.STRING {
			return false
		}
		s, err := strconv.Unquote(lit.Value)
		return err == nil && strings.IndexFunc(s, unicode.IsLetter) >= 0
	}
	fset := token.NewFileSet()
	for name, file := range parseSources(t, fset, parser.SkipObjectResolution, false) {
		if name == "strings.go" {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				var callee string
				switch f := n.Fun.(type) {
				case *ast.Ident:
					callee = f.Name
				case *ast.SelectorExpr:
					callee = f.Sel.Name
				}
				if !slices.Contains(says, callee) {
					return true
				}
				for _, arg := range n.Args {
					if lit, ok := arg.(*ast.BasicLit); ok && hasLetter(lit) {
						t.Errorf("%s: %s(%s) says English the Strings do not hold; add a field to ui.Strings", fset.Position(lit.Pos()), callee, lit.Value)
					}
				}
			case *ast.KeyValueExpr:
				key, ok := n.Key.(*ast.Ident)
				if !ok || !slices.Contains(fields, key.Name) {
					return true
				}
				if lit, ok := n.Value.(*ast.BasicLit); ok && hasLetter(lit) {
					t.Errorf("%s: %s: %s says English the Strings do not hold; add a field to ui.Strings", fset.Position(lit.Pos()), key.Name, lit.Value)
				}
			}
			return true
		})
	}
}
