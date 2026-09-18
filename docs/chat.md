# Chat components and questionnaires

[Documentation](README.md) · [Project README](../README.md)

Build attachments, conversations, transcript scrolling, and validated questionnaires.

Examples use `ggui` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

The chat controls follow the [shadcn/ui chat components](https://ui.shadcn.com/docs/changelog/2026-06-chat-components)
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
rendering. The gallery uses ThemePreset for its palette and geometry; library
widgets continue to use the host theme and font. Sources and licenses are listed in
[the asset notes](../examples/gallery/assets/chat/README.md).

## On this page

- [Attachments](#attachments)
- [Conversation surfaces](#conversation-surfaces)
- [Transcript scrolling](#transcript-scrolling)
- [Questionnaires](#questionnaires)

## Attachments

```go
state := ggui.State(ui.AttachmentIdle)
file := ui.Attachment("report.pdf", "PDF · 2.4 MB").
    Media(ggui.Text("PDF").Size(11)).StateOf(state).
    Trigger("Preview report.pdf", previewReport).
    Actions(ui.AttachmentAction("Remove report.pdf", ggui.Text("×"), removeReport))
```

`State` sets a fixed state and replaces `StateOf`. The five states are `AttachmentIdle`
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
support `Disabled` and `DisabledWhen`.

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

## Transcript scrolling

```go
rows := ggui.State([]ui.MessageEntry{
    {ID: "question", Content: ui.Message(ui.Bubble(ggui.Text("Review this?"))).End(), Anchor: true},
    {ID: "reply", Content: ui.Message(ui.Bubble(ggui.Text("Reviewing…")).Secondary())},
})
transcript := ui.MessageScroller(rows).Height(320).AutoScroll(true)
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
answers := ggui.State(ui.QuestionAnswers{})
form := ui.Questionnaire(answers,
    ui.Question{
        Name: "scope", Title: "What should we build?", Required: true,
        Choices: []ui.QuestionOption{
            {Value: "small", Label: "Small change", Description: "Keep the scope focused."},
            {Value: "full", Label: "Complete feature"},
        },
        InputLabel: "Another answer", Placeholder: "Describe your idea…",
    },
    ui.Question{Name: "notes", Title: "Any constraints?", InputLabel: "Notes"},
).Shortcuts(ui.QuestionNumbers).OnSubmit(func(values ui.QuestionAnswers) {
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
