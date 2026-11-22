# Conversation and AI chat widgets

`ui/chat` provides GGUI's conversation and AI workflow widgets: messages,
bubbles, status markers, file attachments, transcript viewports, interactive
questionnaires, and nested execution traces. It builds on `ggui`, `widget`,
`ui`, `ui/theme`, and `ui/icons`.

```go
import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/chat"
	"github.com/ironpark/ggui/widget"
)

rows := ggui.State([]chat.MessageEntry{
	{ID: "date", Content: chat.Marker(widget.Text("Today")).Separator()},
	{ID: "prompt", Anchor: true, Content: chat.Message(
		chat.Bubble(widget.Text("Summarize the diff.")).End(),
	).End()},
	{ID: "reply", Content: chat.Message(
		chat.Bubble(ui.Markdown("**Ready** to review.")).Ghost(),
	).Avatar(ui.Avatar("Assistant").Size(32))},
})

transcript := chat.MessageScroller(rows).Height(320).AutoScroll(true)
```

## Components

| Widget | Purpose |
| --- | --- |
| `Message`, `MessageGroup` | Conversation rows with optional avatar, header, footer, and start/end alignment |
| `Bubble`, `BubbleGroup` | Chat surfaces in seven variants (`default`, `Secondary`, `Muted`, `Tinted`, `Outline`, `Ghost`, `Destructive`), actions/links, and edge reactions |
| `Marker` | Muted status notes, labeled separators (`Separator`), and bordered updates (`Border`) |
| `Attachment`, `AttachmentAction`, `AttachmentGroup` | File/image cards with upload lifecycle states, sizes, independent actions, and snapping horizontal strips |
| `MessageScroller` | Transcript viewport with turn anchors, reading-position preservation on history prepends, save/restore, and animated scroll-to-edge buttons |
| `Questionnaire` | Multi-step single/multi-choice and freeform prompts with skips, validation, keyboard shortcuts, and controlled active steps |
| `Trace` | Nested execution step disclosures with status styling, duration captions, running shimmer, and expandable details |

## Theming and localization

Chat geometry and corner radii follow `theme.Of(env).Chat` (`BubblePadding`,
`BubbleRadius`, `AttachmentRadius`, `AttachmentXSRadius`, `QuestionGap`,
`QuestionRadius`, `QuestionChoiceGap`, `QuestionTextGap`,
`QuestionChoicePadding`, `QuestionInputRadius`). All built-in labels and
screen-reader descriptions follow `ui.ProvideStrings` and `ui.BindStrings`.

## Migration

Chat widgets moved from `github.com/ironpark/ggui/ui` to
`github.com/ironpark/ggui/ui/chat` without aliases:

| Before (`ui`) | Now (`chat`) |
| --- | --- |
| `ui.Bubble`, `ui.BubbleWidget`, `ui.BubbleGroup` | `chat.Bubble`, `chat.BubbleWidget`, `chat.BubbleGroup` |
| `ui.Message`, `ui.MessageWidget`, `ui.MessageGroup` | `chat.Message`, `chat.MessageWidget`, `chat.MessageGroup` |
| `ui.Marker`, `ui.MarkerWidget` | `chat.Marker`, `chat.MarkerWidget` |
| `ui.Attachment`, `ui.AttachmentWidget`, `ui.AttachmentAction`, `ui.AttachmentState`, `ui.AttachmentSize`, `ui.AttachmentGroup`, `ui.AttachmentGroupWidget` | Same names in `chat` |
| `ui.MessageScroller`, `ui.MessageScrollerWidget`, `ui.MessageEntry`, `ui.ScrollPosition`, `ui.ScrollAlignment`, `ui.TranscriptPosition`, `ui.TranscriptVisibility` | Same names in `chat` |
| `ui.Questionnaire`, `ui.QuestionnaireWidget`, `ui.Question`, `ui.QuestionOption`, `ui.QuestionAnswer`, `ui.QuestionAnswers`, `ui.QuestionStatus`, `ui.QuestionnaireShortcuts` | Same names in `chat` |
| `ui.Trace`, `ui.TraceWidget`, `ui.TraceStep`, `ui.TraceStatus` | Same names in `chat` |
