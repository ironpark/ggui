package ui

import (
	"reflect"
	"sync"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/property"
)

// Strings is every word the controls of this package put on the screen or
// hand a screen reader by themselves: a dialog's "Close", a pager's
// "Previous" and "Next", a command list's "No results", the calendar's
// month and weekday names. ProvideStrings or BindStrings gives a subtree,
// or the whole app, its own, in any language; a control reads them from its
// Env as it lays out, so a change reaches it without a rebuild.
//
// A field left empty keeps the English of DefaultStrings, so an app can
// replace a few words and leave the rest. A setter on a control, such as
// Pagination's Labels or Command's Empty, still wins over the Strings around
// it.
//
// Fields ending in Format are fmt formats. Their arguments are given in
// each field's comment, and an explicit index such as %[2]d puts them in
// the order a language needs: "%[2]d년 %[1]s" for a Korean month title.
//
//	ko := ui.Strings{Close: "닫기", Previous: "이전", Next: "다음", NoResults: "결과 없음"}
//	root := ui.ProvideStrings(ko, app)
//
// Strings carries words only. The language text is shaped in is
// ggui.LanguageKey's, and the direction it reads in ggui.Directionality's;
// an app in Arabic provides all three.
type Strings struct {
	// Shared by several controls.

	Close    string // a dialog's, sheet's or drawer's close button: "Close"
	Previous string // Pagination, DataTable and Questionnaire: "Previous"
	Next     string // Pagination, DataTable and Questionnaire: "Next"
	Loading  string // a Spinner's name: "Loading"

	// Overlays and notices.

	DismissFormat    string // a toast's close button; %s is its title: "Dismiss %s"
	CloseTabFormat   string // an editor tab's close button; %s is its label: "Close %s"
	AlertContinue    string // an AlertDialog's confirm button without Confirm: "Continue"
	AlertCancel      string // an AlertDialog's cancel button: "Cancel"
	DrawerSize       string // a Drawer's size handle: "Drawer size"
	DrawerSizeFormat string // a named Drawer's size handle; %s is its name: "%s size"

	// Command and Combobox.

	SearchCommands      string // a Command's search field: "Search commands"
	SearchCommandsHint  string // its placeholder: "Search commands…"
	Commands            string // a CommandDialog: "Commands"
	NoResults           string // a Command with nothing matching: "No results"
	HintNavigate        string // a Command's Hints footer: "Navigate"
	HintSelect          string // a Command's Hints footer: "Select"
	ComboboxPlaceholder string // an empty Combobox: "Select…"
	ChooseOption        string // an unnamed Combobox: "Choose option"
	ChooseOptions       string // an unnamed ComboboxMultiple: "Choose options"
	ShowOptions         string // a Combobox's chevron: "Show options"
	ClearValue          string // a clearable Combobox's clear button: "Clear"
	SearchOptions       string // a Combobox's list: "Search options"
	NoItemsFound        string // a Combobox with nothing matching: "No items found."
	RemoveFormat        string // a ComboboxMultiple chip's remove button; %s is its label: "Remove %s"

	// What controls are called when not given a name of their own.

	ThemeModeName    string // ThemeMode's group: "Theme"
	ToggleGroupName  string // a ToggleGroup: "Options"
	ButtonGroupName  string // a ButtonGroup: "Actions"
	ContextMenuName  string // a ContextMenu: "Context menu"
	MenubarName      string // a Menubar: "Menubar"
	NavigationName   string // a NavigationMenu: "Navigation"
	BreadcrumbName   string // a Breadcrumb: "Breadcrumb"
	SidebarName      string // a Sidebar: "Sidebar"
	AccordionName    string // an Accordion: "Accordion"
	ResizablePanels  string // a Resizable: "Resize panels"
	TraceName        string // a Trace: "Execution trace"
	AttachmentsName  string // an AttachmentGroup: "Attachments"
	ConversationName string // a MessageScroller: "Conversation"
	CarouselName     string // a Carousel: "Carousel"
	ChartName        string // a chart: "Chart"
	OneTimePassword  string // an InputOTP: "One-time password"
	DarkMode         string // a ThemeSwitch: "Dark mode"
	SliderValue      string // a RangeSlider: "Value"

	// The other controls' words.

	ThemeLight            string // ThemeMode's light option: "Light"
	ThemeDark             string // ThemeMode's dark option: "Dark"
	ThemeSystem           string // ThemeMode's system option: "System"
	BreadcrumbMore        string // a Breadcrumb's collapsed steps: "More"
	ToggleSidebar         string // a SidebarTrigger and the rail: "Toggle Sidebar"
	SliderThumbFormat     string // a RangeSlider's thumb; %[1]s is the slider's name, %[2]d the thumb's number: "%s %d"
	CodeLengthFormat      string // an InputOTP's description; %d is its length: "%d-character code"
	InvalidCode           string // added to it while the code is invalid: "; invalid code"
	AvatarMoreFormat      string // an AvatarGroup's overflow; %d is how many are hidden: "%d more"
	ScrollToStart         string // a MessageScroller's button: "Scroll to start"
	ScrollToEnd           string // a MessageScroller's button: "Scroll to end"
	NumberDecrement       string // a NumberInput's − button: "Decrement"
	NumberIncrement       string // a NumberInput's + button: "Increment"
	PreviousSlide         string // a Carousel's previous button: "Previous slide"
	NextSlide             string // a Carousel's next button: "Next slide"
	SlideFormat           string // a slide; %[1]d is its number, %[2]d how many there are: "Slide %d of %d"
	ChartHelp             string // a chart's description: "Chart. Use arrow keys to explore values; Enter selects a category."
	ChartTotal            string // a tooltip's total row without TotalLabel: "Total"
	ChartNoData           string // a chart with no data: "No data"
	CodeContent           string // a CodeBlock's viewport, unless named: "Code content"
	CodeScrollHelp        string // its description: "Use arrow keys or Page Up and Page Down to scroll"
	CodeBlockName         string // a CodeBlock's description: "Code block"
	CodeName              string // a CodeBlock with no title or language: "Code"
	CopyCode              string // a CodeBlock's copy button: "Copy code"
	CodeCopied            string // what is read once it copied: "Code copied"
	HighlightedLineFormat string // a highlighted line; %d is its number: "Highlighted line %d"
	AddedLineFormat       string // an added line; %d is its number: "Added line %d"
	RemovedLineFormat     string // a removed line; %d is its number: "Removed line %d"

	// A Trace step's status and an Attachment's upload state, as a screen
	// reader hears them. A status of the app's own is read as it is.

	TraceStatusPending         string // "pending"
	TraceStatusRunning         string // "running"
	TraceStatusCompleted       string // "completed"
	TraceStatusFailed          string // "failed"
	TraceStatusError           string // "error"
	TraceStatusSkipped         string // "skipped"
	AttachmentStatusIdle       string // "idle"
	AttachmentStatusUploading  string // "uploading"
	AttachmentStatusProcessing string // "processing"
	AttachmentStatusError      string // "error"
	AttachmentStatusDone       string // "done"

	// Pagination and DataTable.

	PaginationName     string // an unnamed Pagination: "Pagination"
	PageFormat         string // a page's button; %d is its number: "Page %d"
	MorePages          string // the gaps between pages: "More pages"
	FilterRows         string // a DataTable's search field: "Filter rows"
	FilterRowsHint     string // its placeholder: "Filter rows..."
	Columns            string // a DataTable's column menu: "Columns"
	ShowColumnFormat   string // a column in it; %s is the column's title: "Show %s"
	SortFormat         string // a sortable heading; %s is its title: "Sort %s"
	SortAscending      string // a heading sorting up: "ascending"
	SortDescending     string // a heading sorting down: "descending"
	SelectRowFormat    string // a row's checkbox; %v is the row's key: "Select row %v"
	SelectPage         string // the heading checkbox: "Select page"
	NoTableResults     string // a DataTable with nothing matching: "No results."
	RowsSelectedFormat string // %[1]d selected of %[2]d rows: "%d of %d row(s) selected."
	PageOfFormat       string // %[1]d is the page, %[2]d how many: "Page %d of %d"
	RowsPerPage        string // the page size select and its label: "Rows per page"

	// Calendar and DatePicker.

	CalendarName  string     // an unnamed Calendar: "Calendar"
	PreviousMonth string     // "Previous month"
	NextMonth     string     // "Next month"
	MonthSelect   string     // the caption dropdown's month select: "Month"
	YearSelect    string     // the caption dropdown's year select: "Year"
	Weekdays      [7]string  // the weekday columns, Sunday first: "Su" … "Sa"
	Months        [12]string // the months in a caption, January first: "January" … "December"
	MonthsShort   [12]string // the months in the caption dropdown: "Jan" … "Dec"
	// MonthTitleFormat is a month's caption; %[1]s is its name from Months
	// and %[2]d its year: "%s %d".
	MonthTitleFormat string
	ChooseDate       string // an empty DatePicker: "Choose date"
	ChooseDates      string // an empty DateRangePicker: "Choose dates"
	// DateLayout is how a DatePicker shows the date chosen, a layout for
	// time.Time.Format: "2006-01-02".
	DateLayout string

	// Questionnaire.

	QuestionnaireName     string // an unnamed Questionnaire: "Questionnaire"
	QuestionFormat        string // %[1]d is the question, %[2]d how many: "Question %d of %d"
	QuestionnaireProgress string // the progress it reports: "Questionnaire progress"
	Submit                string // the last step's button without SubmitLabel: "Submit"
	Skip                  string // an optional step's button: "Skip"
	AnswerRequired        string // a required step left unanswered: "Choose an answer to continue."
	AnswerOrSkip          string // an optional one: "Choose an answer or skip this question."
	NoQuestions           string // with no enabled questions: "No questions"
	NoQuestionsDetail     string // and under it: "There are no enabled questions."
}

// DefaultStrings returns the English the controls use where no Strings
// were provided.
func DefaultStrings() Strings { return *english() }

// english is DefaultStrings made once, shared by every Env that has no
// Strings of its own.
var english = sync.OnceValue(func() *Strings {
	return &Strings{
		Close:    "Close",
		Previous: "Previous",
		Next:     "Next",
		Loading:  "Loading",

		DismissFormat:    "Dismiss %s",
		CloseTabFormat:   "Close %s",
		AlertContinue:    "Continue",
		AlertCancel:      "Cancel",
		DrawerSize:       "Drawer size",
		DrawerSizeFormat: "%s size",

		SearchCommands:      "Search commands",
		SearchCommandsHint:  "Search commands…",
		Commands:            "Commands",
		NoResults:           "No results",
		HintNavigate:        "Navigate",
		HintSelect:          "Select",
		ComboboxPlaceholder: "Select…",
		ChooseOption:        "Choose option",
		ChooseOptions:       "Choose options",
		ShowOptions:         "Show options",
		ClearValue:          "Clear",
		SearchOptions:       "Search options",
		NoItemsFound:        "No items found.",
		RemoveFormat:        "Remove %s",

		ThemeModeName:    "Theme",
		ToggleGroupName:  "Options",
		ButtonGroupName:  "Actions",
		ContextMenuName:  "Context menu",
		MenubarName:      "Menubar",
		NavigationName:   "Navigation",
		BreadcrumbName:   "Breadcrumb",
		SidebarName:      "Sidebar",
		AccordionName:    "Accordion",
		ResizablePanels:  "Resize panels",
		TraceName:        "Execution trace",
		AttachmentsName:  "Attachments",
		ConversationName: "Conversation",
		CarouselName:     "Carousel",
		ChartName:        "Chart",
		OneTimePassword:  "One-time password",
		DarkMode:         "Dark mode",
		SliderValue:      "Value",

		ThemeLight:            "Light",
		ThemeDark:             "Dark",
		ThemeSystem:           "System",
		BreadcrumbMore:        "More",
		ToggleSidebar:         "Toggle Sidebar",
		SliderThumbFormat:     "%s %d",
		CodeLengthFormat:      "%d-character code",
		InvalidCode:           "; invalid code",
		AvatarMoreFormat:      "%d more",
		ScrollToStart:         "Scroll to start",
		ScrollToEnd:           "Scroll to end",
		NumberDecrement:       "Decrement",
		NumberIncrement:       "Increment",
		PreviousSlide:         "Previous slide",
		NextSlide:             "Next slide",
		SlideFormat:           "Slide %d of %d",
		ChartHelp:             "Chart. Use arrow keys to explore values; Enter selects a category.",
		ChartTotal:            "Total",
		ChartNoData:           "No data",
		CodeContent:           "Code content",
		CodeScrollHelp:        "Use arrow keys or Page Up and Page Down to scroll",
		CodeBlockName:         "Code block",
		CodeName:              "Code",
		CopyCode:              "Copy code",
		CodeCopied:            "Code copied",
		HighlightedLineFormat: "Highlighted line %d",
		AddedLineFormat:       "Added line %d",
		RemovedLineFormat:     "Removed line %d",

		TraceStatusPending:         "pending",
		TraceStatusRunning:         "running",
		TraceStatusCompleted:       "completed",
		TraceStatusFailed:          "failed",
		TraceStatusError:           "error",
		TraceStatusSkipped:         "skipped",
		AttachmentStatusIdle:       "idle",
		AttachmentStatusUploading:  "uploading",
		AttachmentStatusProcessing: "processing",
		AttachmentStatusError:      "error",
		AttachmentStatusDone:       "done",

		PaginationName:     "Pagination",
		PageFormat:         "Page %d",
		MorePages:          "More pages",
		FilterRows:         "Filter rows",
		FilterRowsHint:     "Filter rows...",
		Columns:            "Columns",
		ShowColumnFormat:   "Show %s",
		SortFormat:         "Sort %s",
		SortAscending:      "ascending",
		SortDescending:     "descending",
		SelectRowFormat:    "Select row %v",
		SelectPage:         "Select page",
		NoTableResults:     "No results.",
		RowsSelectedFormat: "%d of %d row(s) selected.",
		PageOfFormat:       "Page %d of %d",
		RowsPerPage:        "Rows per page",

		CalendarName:  "Calendar",
		PreviousMonth: "Previous month",
		NextMonth:     "Next month",
		MonthSelect:   "Month",
		YearSelect:    "Year",
		Weekdays:      [7]string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"},
		Months: [12]string{"January", "February", "March", "April", "May", "June",
			"July", "August", "September", "October", "November", "December"},
		MonthsShort:      [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
		MonthTitleFormat: "%s %d",
		ChooseDate:       "Choose date",
		ChooseDates:      "Choose dates",
		DateLayout:       "2006-01-02",

		QuestionnaireName:     "Questionnaire",
		QuestionFormat:        "Question %d of %d",
		QuestionnaireProgress: "Questionnaire progress",
		Submit:                "Submit",
		Skip:                  "Skip",
		AnswerRequired:        "Choose an answer to continue.",
		AnswerOrSkip:          "Choose an answer or skip this question.",
		NoQuestions:           "No questions",
		NoQuestionsDetail:     "There are no enabled questions.",
	}
})

// withDefaults returns s with every empty string, a field's or an array
// element's, taken from DefaultStrings.
func (s Strings) withDefaults() Strings {
	out, def := reflect.ValueOf(&s).Elem(), reflect.ValueOf(english()).Elem()
	for i := range out.NumField() {
		f := out.Field(i)
		switch f.Kind() {
		case reflect.String:
			if f.String() == "" {
				f.Set(def.Field(i))
			}
		case reflect.Array:
			for j := range f.Len() {
				if f.Index(j).String() == "" {
					f.Index(j).Set(def.Field(i).Index(j))
				}
			}
		}
	}
	return s
}

// stringsKey holds the provided Strings, defaults filled in, by pointer:
// a control keeps the pointer from Layout to Paint rather than a copy.
// Strings compare by what they hold, so providing equal ones again gives
// back the Env it gave before, and the caches under it hold.
var stringsKey = ggui.NewEnvKeyFunc("ui strings", func(a, b *Strings) bool { return a == b || *a == *b })

// ProvideStrings gives the subtree under child the words s holds in place
// of the English ones; fields s leaves empty stay English. Put it around
// the root for the whole app: popups, dialogs and toasts take the Strings
// of where they are placed in the tree.
//
//	ui.ProvideStrings(ui.Strings{Close: "닫기", Previous: "이전", Next: "다음"}, root)
func ProvideStrings(s Strings, child ggui.Widget) *ggui.EnvWidget {
	p := new(Strings)
	*p = s.withDefaults()
	return ggui.Provide(stringsKey, p, child)
}

// BindStrings is ProvideStrings following r: the subtree lays out again in
// the new words when r changes, with nothing rebuilt.
//
//	lang := ggui.State(ui.DefaultStrings())
//	root := ui.BindStrings(lang, app) // lang.Set(korean) switches it
func BindStrings(r ggui.Readable[Strings], child ggui.Widget) *ggui.EnvWidget {
	property.Require(r, "BindStrings")
	var last *Strings
	return ggui.WithEnv(func(env ggui.Env) ggui.Env {
		s := r.Get().withDefaults()
		if last == nil || *last != s {
			last = new(Strings)
			*last = s
		}
		return env.With(stringsKey, last)
	}, child)
}

// StringsFrom returns the Strings the tree under env is worded in: those
// provided above it, defaults filled in, or DefaultStrings. A control of
// your own reads its words here in Layout.
func StringsFrom(env ggui.Env) Strings { return *stringsOf(env) }

// stringsOf is StringsFrom without the copy, for the controls of this
// package. Treat what it returns as read-only.
func stringsOf(env ggui.Env) *Strings {
	if s, ok := env.Get(stringsKey); ok {
		return s
	}
	return english()
}

// traceStatus is how s words the status of a Trace step.
func (s *Strings) traceStatus(status TraceStatus) string {
	switch status {
	case TracePending:
		return s.TraceStatusPending
	case TraceRunning:
		return s.TraceStatusRunning
	case TraceCompleted:
		return s.TraceStatusCompleted
	case TraceFailed:
		return s.TraceStatusFailed
	case TraceError:
		return s.TraceStatusError
	case TraceSkipped:
		return s.TraceStatusSkipped
	}
	return string(status)
}

// attachmentState is how s words the upload state of an Attachment.
func (s *Strings) attachmentState(state AttachmentState) string {
	switch state {
	case AttachmentIdle:
		return s.AttachmentStatusIdle
	case AttachmentUploading:
		return s.AttachmentStatusUploading
	case AttachmentProcessing:
		return s.AttachmentStatusProcessing
	case AttachmentError:
		return s.AttachmentStatusError
	case AttachmentDone:
		return s.AttachmentStatusDone
	}
	return string(state)
}

// orDefault returns s, or the English Strings when s is nil: the words of
// a control not yet laid out.
func (s *Strings) orDefault() *Strings {
	if s == nil {
		return english()
	}
	return s
}

// localized lays its child out after apply has worded it in the Strings of
// the Env it is laid out in: for the parts a control builds before it has
// an Env, such as a DataTable's toolbar or a Questionnaire's buttons. It
// is otherwise its child.
type localized struct {
	child ggui.Widget
	apply func(*Strings)
}

// localize returns child, worded by apply at each layout.
func localize(child ggui.Widget, apply func(*Strings)) *localized {
	return &localized{child: child, apply: apply}
}

func (l *localized) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	l.apply(stringsOf(env))
	return l.child.Layout(c, env)
}

func (l *localized) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(l.child, r) }

// Baseline implements ggui.Baseliner: the child's.
func (l *localized) Baseline() (float64, bool) { return ggui.BaselineOf(l.child) }
