# Chat components and questionnaires

[Documentation](README.md) · [Project README](../README.md)

Build attachments, conversations, Markdown replies, transcript scrolling, and validated questionnaires.

Examples use `ggui`, `widget`, `ui` and `chat` (`github.com/ironpark/ggui/ui/chat`) imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

The chat controls live in `github.com/ironpark/ggui/ui/chat` and follow the [shadcn/ui chat components](https://ui.shadcn.com/docs/changelog/2026-06-chat-components)
and [Questionnaire](https://ui.shadcn.com/docs/components/base/questionnaire)
contracts with native Go widgets, bindings and callbacks. Surface colors come
from the current theme; typography uses the application's native font. The
gallery has separate Attachment, Bubble, Message, Marker, Message Scroller and
Questionnaire previews in both themes. The chat surfaces follow the official
Rhea demos (24px bubble corners, 16px attachment corners); the questionnaire
follows the Nova demo. Bubble groups use 8px spacing, separate turns use 24–32px,
and reaction rings sit outside the pill. Native text metrics are accounted for
in the bubble padding.

The gallery's reference scenes bundle Geist, the original demo photos/avatars,
and the optional Noto Color Emoji font for consistent offline/native/WASM
rendering. The gallery uses theme.Preset for its palette and geometry; library
widgets continue to use the host theme and font. Sources and licenses are listed in
[the asset notes](../examples/gallery/assets/chat/README.md).

## On this page

- [Attachments](#attachments)
- [Conversation surfaces](#conversation-surfaces)
- [Markdown](#markdown)
- [Transcript scrolling](#transcript-scrolling)
- [Questionnaires](#questionnaires)

## Attachments

```go
state := ggui.State(chat.AttachmentIdle)
file := chat.Attachment("report.pdf", "PDF · 2.4 MB").
    Media(widget.Text("PDF").Size(11)).BindState(state).
    Trigger("Preview report.pdf", previewReport).
    Actions(chat.AttachmentAction("Remove report.pdf", widget.Text("×"), removeReport))
```

`State` sets a fixed state and replaces `BindState`. The five states are `AttachmentIdle`
(dashed border), `AttachmentUploading` and `AttachmentProcessing` (title shimmer),
`AttachmentError` (destructive border/media/metadata), and `AttachmentDone` (default).
Put an explicit failure reason in the description. Uploading is presentation
only: the host owns files, progress, retries, transport and image lifetimes.

`Image(img, alt)` displays a rounded cover crop; unfinished previews are dimmed.
`Vertical()` moves media above the metadata and overlays actions at the top right.
`Size(AttachmentSmall)` and `Size(AttachmentExtraSmall)` select compact geometry;
`Width(px)` sets the requested width within parent constraints. Long filenames
and metadata truncate visually but retain their complete accessible text.
Card triggers and actions remain separate keyboard and pointer targets.

`AttachmentGroup(files...)` supplies horizontal scrolling, 12px gaps, settling
snap points and edge fades. Its named group supports Left/Right, Home/End and
PageUp/PageDown. Tab brings offscreen actions into view. Keep the group instance
or assign a stable `Key` when preserving scroll across rebuilds.

## Conversation surfaces

`Bubble(content)` is a primary surface, limited to 80% of its available width.
`Secondary`, `Muted`, `Tinted`, `Outline`, `Ghost` and `Destructive` select the
other treatments. Ghost has no frame or padding and can use the full width.
`End()` aligns a bubble to the trailing side. `Action(name, fn)` or `Link(name, fn)`
provides a focusable button/link surface; the host owns opening a URL. Both
support `Disabled` and `BindDisabled`.

`Reactions(widget)` overlaps the bottom end edge. `ReactionsTop()` and
`ReactionsStart()` move it. Leave vertical space between rows; reactions can
contain independently named buttons. Compose `ui.Collapsible` inside the bubble
for show-more behavior. `BubbleGroup` stacks consecutive bubbles with an 8px gap.

`Message(content).Avatar(widget).Header(widget).Footer(widget)` arranges a
conversation row. `End()` reverses the avatar side and aligns its metadata and
nested bubbles. The avatar sits above the footer. `MessageGroup` stacks rows;
avatars, links, attachments and content are supplied by the caller.

`Marker(content)` displays muted status content. `Icon(widget)` supplies a
16px decorative slot; `Separator()` adds rules around a centered label and
`Border()` adds a bottom rule. Compose a spinner, text or links as needed.

## Markdown

A reply is usually written in Markdown. `ui.Markdown(text)` renders it in
the theme's typography:

```go
reply := ggui.State("")
streaming := ggui.State(true)
answer := chat.Bubble(ui.Markdown("").BindContent(reply).BindStreaming(streaming)).Ghost()
// As each piece arrives: reply.Set(ggui.Peek(reply) + piece)
// When the stream ends:  streaming.Set(false)
```

It reads CommonMark with GitHub's extensions: ATX and setext headings,
paragraphs with emphasis, strong emphasis, `~~strikethrough~~`, code spans,
links, images and autolinks (`<https://…>`, and bare `www.` and
`https://` addresses); bullet, numbered, nested and task lists; block
quotes; fenced and indented code; thematic breaks; and tables, aligned by
their delimiter row, with `\|` for a pipe inside a cell. Raw HTML shows as
the text it is, and a reference link (`[text][ref]`) as written: nothing in
a message is interpreted beyond what it shows. An image shows as a link
named by its description; the image is not fetched.

| Block | Rendered as |
| --- | --- |
| Heading | Semibold text, larger by level, described as a heading of its level |
| Paragraph | Selectable rich text; a link is a focusable link span |
| List | Bullets that change shape with nesting, numbers, or a read-only check box for a task item; described as a list of positioned items |
| Block quote | Muted text inset from a bar |
| Code | A `CodeBlock` labelled with its language, with its copy action, as tall as its code |
| Table | A grid in the theme's border color, the heading row in semibold and every other row on the muted surface; described as a table of column headings and cells |
| Rule | A `Divider`, described as a separator |

`Content(s)` replaces the source; `BindContent(r)` follows a reader. When
the source changes, only the blocks that changed are rebuilt: as a reply
grows, that is its last block, which grows in place, and the source is
parsed again only from that block on. `Streaming(true)` (or
`BindStreaming`) says the text was cut off mid-stream, so emphasis, a code
span or a link opened in the last block and not yet closed show as closed
rather than as their syntax; turn it off when the stream ends, so the
finished text reads as written. An unclosed code fence shows the code so
far either way.

`OnLink(fn)` receives the destination of a tapped link, whatever its
scheme. Without it, http, https and mailto links open with
`runtime.OpenURL` and every other scheme is ignored, so a `javascript:` or
`file:` link in a message does nothing. `Size(px)` sets the body text size
(headings and code scale with it), `Color(c)` the body text color, and
`Selectable(false)` turns off selection, which otherwise runs across the
blocks as on a page. A short reply is as wide as its text; one holding
code, a quote, a table or a rule fills the width it is given.

The parser is the [`markdown`](../pkg/markdown/markdown.go) package, which
depends on nothing of ggui: `markdown.Parse` returns the blocks and inline
spans, `ParseStream` reads a source cut off mid-stream, and `Reparse`
parses a grown source again from its last blocks.

## Transcript scrolling

```go
rows := ggui.State([]chat.MessageEntry{
    {ID: "question", Content: chat.Message(chat.Bubble(widget.Text("Review this?"))).End(), Anchor: true},
    {ID: "reply", Content: chat.Message(chat.Bubble(widget.Text("Reviewing…")).Secondary())},
})
transcript := chat.MessageScroller(rows).Height(320).AutoScroll(true)
```

### Update transcript rows

Each row needs a unique, nonempty ID and nonnil content. Keep the scroller alive
while updating its bound rows. Replace a row's content for streamed updates, or
bind its text directly. The scroller does not own transport or message storage.

### Control scrolling

- `Opening(ScrollStart|ScrollEnd|ScrollLastAnchor)` chooses the first nonempty
  transcript's position. End is the default; last-anchor falls back to end when
  that turn fits. Opening is applied before the first paint.
- `AutoScroll` defaults to false. When enabled, it follows output at the live
  edge; wheel, pointer presses and keyboard input inside the viewport pause it,
  including events consumed by child controls. `ScrollToEnd()` resumes following.
- Appending an `Anchor` row starts a turn near the top, preserving a 64px peek
  of its predecessor. `PreviousPeek`, `ScrollMargin` and `Gap` configure spacing.
- Prepending history or changing measured row heights preserves the first visible
  stable row and the offset within it. `Save()`/`Restore()` persist that reading
  position. Restore waits if its row has not been loaded yet.
- `ScrollToMessage(id, alignment)` supports start, center, end and nearest;
  missing rows remain queued until available. Later jump commands replace them.
  `ScrollToStart()` and `ScrollToEnd()` target the edges.
- `Animation(duration)` controls programmatic scrolling; reduced motion jumps
  immediately. `Visibility()` and `OnVisibility` expose visible IDs, the current
  anchor and scrollable edges. Callbacks run on the UI thread and should avoid
  changing layout in a feedback loop.

The viewport is keyboard scrollable, reveals focused descendants, clips input
along with content and shows start/end controls only when useful. Large histories
are measured, not virtualized; paginate the bound collection for very long chats.

## Questionnaires

```go
answers := ggui.State(chat.QuestionAnswers{})
form := chat.Questionnaire(answers,
    chat.Question{
        Name: "scope", Title: "What should we build?", Required: true,
        Choices: []chat.QuestionOption{
            {Value: "small", Label: "Small change", Description: "Keep the scope focused."},
            {Value: "full", Label: "Complete feature"},
        },
        InputLabel: "Another answer", Placeholder: "Describe your idea…",
    },
    chat.Question{Name: "notes", Title: "Any constraints?", InputLabel: "Notes"},
).Shortcuts(chat.QuestionNumbers).OnSubmit(func(values chat.QuestionAnswers) {
    // Persist or send the validated answers here.
})
```

### Define questions and answers

Question names and option values must be unique and nonempty. Questions are
optional by default, but moving forward requires either an answer or explicit
Skip. `Required` prevents skipping; `Multiple` permits several fixed answers.
Freeform input replaces a fixed answer on single-choice steps and can accompany
fixed answers on multiple-choice steps. Whitespace alone is not an answer.
Disabled steps are excluded from progress/navigation/submission and disabled
choices are not selectable or serialized.

The answer binding stores `Values`, `Text` and `Skipped` per question.
`OnSubmit` receives a deep copy containing enabled answered questions; skipped
questions are omitted. `Status(name)` and `OnStatusChange` distinguish unanswered,
answered and skipped. `Active(binding)` controls the active question by name;
`OnItemChange` observes navigation. `SetItems` supports conditional collections.
`Reset()` restores initial answers and navigation, clearing skips and errors.

### Validate and navigate

`Question.Validate` returns an error string for custom validation. `SetError`
returns to a step with a host-provided error; editing clears it. Forward navigation
validates the active question, while Submit validates all enabled questions and
focuses the first invalid answer. Previous does not validate or discard answers.
Buttons remain enabled so an attempted action can explain what is missing.

### Keyboard interaction

Tab visits answers and visible actions. Arrows move between fixed answers;
radio movement also selects. Enter on a selected answer continues; Cmd/Ctrl+Enter
validates and continues from any answer or action. Letter or number shortcuts
select enabled answers without advancing and never intercept text editing or
IME composition. Progress and validation are exposed to assistive technology.
The surrounding app/card/dialog owns cancellation, persistence and branching.
