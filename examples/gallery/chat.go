package main

import (
	"fmt"
	"slices"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// These previews keep transcript and questionnaire instances alive while the
// gallery filters or theme change. No network or file access is needed.
func newChatPreviews() func() []ggui.Widget {
	upload := ggui.State(ui.AttachmentIdle)
	states := []ui.AttachmentState{ui.AttachmentIdle, ui.AttachmentUploading, ui.AttachmentProcessing, ui.AttachmentError, ui.AttachmentDone}
	files := ggui.State([]string{"brief.pdf", "customers.csv", "renderer.go"})
	attachmentAction := ggui.State("Open a file preview or remove an attachment.")
	thumb, desk, office := chatImage("workspace.jpg"), chatImage("desk.jpg"), chatImage("office.jpg")
	you, oliver := chatImage("avatar-you.png"), chatImage("avatar-oliver.png")
	variants := ggui.State(false)
	attachmentVariants := ggui.State(false)
	showUpload, showSource := ggui.State(true), ggui.State(true)
	reaction := ggui.State(0)
	expanded := ggui.State(false)
	bubbleAction := ggui.State("Choose a suggested reply.")
	rows := ggui.State([]ui.MessageEntry{
		{ID: "welcome", Content: ui.Marker(ggui.Text("Today")).Separator()},
		{ID: "hello", Content: ui.Message(ui.Bubble(ggui.Text("Can you review this patch?")).End()).End(), Anchor: true},
		{ID: "reply", Content: ui.Message(ui.Bubble(ggui.Text("I will check the implementation and tests.")).Secondary()).Avatar(ui.Avatar("Reviewer").Size(32))},
	})
	scroller := ui.MessageScroller(rows).Height(260).AutoScroll(true).Name("Preview conversation")
	sequence, history := 0, 0
	var saved ui.TranscriptPosition
	scrollNote := ggui.State("Scroll up to pause following. Jump to the end to resume.")
	answers := ggui.State(ui.QuestionAnswers{})
	submission := ggui.State("Your answers stay local to this preview.")
	questionnaire := ui.Questionnaire(answers,
		ui.Question{
			Name:        "scope",
			Title:       "What should the agent build next?",
			Description: "Choose a direction or describe another task.",
			Required:    true,
			Choices: []ui.QuestionOption{
				{
					Value:       "timeline",
					Label:       "Tool call timeline",
					Description: "Show what the agent ran and what came back.",
				},
				{
					Value:       "approvals",
					Label:       "Approval checkpoints",
					Description: "Ask before sensitive or destructive actions.",
				},
				{
					Value:       "handoffs",
					Label:       "Sub-agent handoffs",
					Description: "Make delegated work and results easier to follow.",
				},
			},
			InputLabel:  "Another feature",
			Placeholder: "Describe another feature…",
		},
		ui.Question{
			Name:        "updates",
			Title:       "What should updates include?",
			Description: "Select multiple items, or explicitly skip this step.",
			Multiple:    true,
			Choices: []ui.QuestionOption{
				{Value: "progress", Label: "Progress updates"},
				{Value: "decisions", Label: "Decisions"},
				{Value: "risks", Label: "Risks"},
			},
		},
		ui.Question{
			Name:        "context",
			Title:       "Who will use this?",
			Description: "Add a short audience description.",
			Required:    true,
			InputLabel:  "Audience",
			Placeholder: "EachKeyed example, our support team",
			Validate: func(a ui.QuestionAnswer) string {
				if len([]rune(a.Text)) < 3 {
					return "Please use at least three characters."
				}
				return ""
			},
		},
	).Shortcuts(ui.QuestionLetters).SubmitLabel("Save answers").OnSubmit(func(a ui.QuestionAnswers) {
		submission.Set(fmt.Sprintf("Saved %d answers. Audience: %s", len(a), a["context"].Text))
	})

	return func() []ggui.Widget {
		t := uitheme.Use()
		return []ggui.Widget{
			preview("Attachment", ggui.Column(
				chatSurface(ggui.Column(
					ui.AttachmentGroup(
						ui.Attachment("workspace.png", "PNG · 820 KB").Image(thumb, "Workspace").Vertical().Trigger("Preview workspace", func() { attachmentAction.Set("Preview opened: workspace.png") }),
						ui.Attachment("desk-reference.jpg", "JPG · 1.1 MB").Image(desk, "Desk").Vertical(),
						ui.Attachment("office-reference.jpg", "JPG · 940 KB").Image(office, "Office").Vertical(),
					).Name("Image attachments"),
					ggui.If(showUpload, func() ggui.Widget {
						return ui.Attachment("sales-dashboard.pdf", "Uploading · 64%").Media(ui.Spinner().Size(16)).State(ui.AttachmentUploading).
							Actions(ui.AttachmentAction("Cancel upload", ui.Icon(icons.Close), func() {
								showUpload.Set(false)
								attachmentAction.Set("Upload cancelled.")
							}))
					}),
					ggui.If(showSource, func() ggui.Widget {
						return ui.Attachment("message-renderer.tsx", "TypeScript · 12 KB").Media(ui.Icon(icons.File)).
							Actions(ui.AttachmentAction("Remove source attachment", ui.Icon(icons.Close), func() {
								showSource.Set(false)
								attachmentAction.Set("Source attachment removed.")
							}))
					}),
				).Gap(12).Align(ggui.AlignStretch)),
				ui.Select(upload).Options(states).Name("Upload state"),
				ui.Attachment("design-system.zip", "Choose an upload state above").Media(ggui.Text("ZIP").Size(11)).BindState(upload).
					Actions(ui.AttachmentAction("Retry upload", ggui.Text("↻"), func() { upload.Set(ui.AttachmentUploading) })),
				ggui.View(files, func(names []string) *ui.AttachmentGroupWidget {
					cards := []*ui.AttachmentWidget{}
					for _, name := range names {
						cards = append(cards, ui.Attachment(name, "Ready to upload").State(ui.AttachmentIdle).Width(180).Media(ui.Icon(icons.File)).
							Trigger("Preview "+name, func() { attachmentAction.Set("Preview opened: " + name) }).
							Actions(ui.AttachmentAction("Remove "+name, ui.Icon(icons.Close), func() { ggui.Remove(files, func(v string) bool { return v == name }) })))
					}
					return ui.AttachmentGroup(cards...).Name("Attached files")
				}),
				ggui.TextOf(attachmentAction).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption),
				ui.Button("Restore attachments", func() {
					showUpload.Set(true)
					showSource.Set(true)
					files.Set([]string{"brief.pdf", "customers.csv", "renderer.go"})
				}).Outline(),
				ui.Collapsible(attachmentVariants, "Attachment sizes", ggui.Wrap(
					ui.Attachment("Default", "PDF · 2.4 MB"),
					ui.Attachment("Small", "CSV · 18 KB").Size(ui.AttachmentSmall),
					ui.Attachment("Extra small", "").Size(ui.AttachmentExtraSmall),
				).Gap(8)),
			).Gap(12).Align(ggui.AlignStretch)),
			preview("Bubble", ggui.Column(
				chatSurface(ggui.Column(
					ui.Bubble(ggui.Text("Hey there! what's up?")).End(),
					ui.BubbleGroup(
						ui.Bubble(ggui.Text("Hey! Want to see chat bubbles?")).Muted(),
						ui.Bubble(ggui.Text("I can group messages, switch sides, and keep the whole thread easy to scan.")).Muted().Reactions(ggui.Text("👍").Size(20)),
					),
					ui.Bubble(ggui.Text("Sure. Hit me with your best demo.")).End(),
					ui.Bubble(ggui.Text("Yes. You are reading a demo that is demoing itself. Very meta. Very on-brand.")).Muted().Reactions(ggui.Row(ggui.Text("👍").Size(20), ggui.Text("🔥").Size(20), ggui.Text("👀").Size(20), ggui.Text("+2")).Gap(4)),
				).Gap(32).Align(ggui.AlignStretch)),
				ui.Collapsible(variants, "More bubble variants", ggui.Column(
					ui.Bubble(ggui.Text("Secondary conversation surface.")).Secondary(),
					ui.Bubble(ggui.Text("A subtle primary tint.")).Tinted(),
					ui.Bubble(ggui.Text("Choose this suggestion")).Outline().Action("Choose suggestion", func() { bubbleAction.Set("Suggestion selected.") }),
					ui.Bubble(ggui.Text("Upload failed. Please retry.")).Destructive(),
					ui.Bubble(ggui.Text("Ghost content uses the full width.")).Ghost(),
					ui.Bubble(ui.Collapsible(expanded, "Show more", ggui.Text("Long content keeps its state."))).Secondary().Reactions(ui.ButtonOf(ggui.Textf("Like · %d", reaction), func() { ggui.Add(reaction, 1) }).Name("Like bubble").Ghost().Pad(2, 6)),
					ggui.Padding(ggui.TextOf(bubbleAction).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption), 16, 0, 0, 0),
				).Gap(16).Align(ggui.AlignStretch)),
			).Gap(16).Align(ggui.AlignStretch)),
			preview("Message", chatSurface(ggui.Column(
				ui.Message(ui.Bubble(ggui.Text("Deploying to prod real quick."))).End().Avatar(ui.Avatar("You").Image(you).Size(32)),
				ui.Message(ui.Bubble(ggui.Text("It's 4:55 PM. On a Friday.")).Muted()).Avatar(ui.Avatar("Oliver").Image(oliver).Size(32)),
				ui.Message(ui.Bubble(ggui.Text("It's a one-line change."))).End().Avatar(ui.Avatar("You").Image(you).Size(32)).Footer(ggui.Text("Delivered")),
				ui.Message(ui.BubbleGroup(
					ui.Bubble(ggui.Text("It's always a one-line change 😭.")).Muted(),
					ui.Bubble(ggui.Text("Alright, let me take a look.")).Muted().Reactions(ggui.Text("👍").Size(20)),
				)).Avatar(ui.Avatar("Oliver").Image(oliver).Size(32)),
				ui.Marker(ggui.Text("Oliver is typing…")),
			).Gap(24).Align(ggui.AlignStretch))),
			preview("Marker", chatSurface(ggui.Column(
				ui.Marker(ggui.Text("Switched to a new branch")).Icon(&chatIcon{kind: "branch"}),
				ui.Marker(ggui.Text("Thinking…")).Icon(ui.Spinner().Size(16)),
				ui.Marker(ggui.Text("Conversation compacted")).Separator(),
				ui.Marker(ggui.Text("Explored 4 files")).Icon(ui.Icon(icons.Search)),
			).Gap(32).Align(ggui.AlignStretch))),
			preview("Message Scroller", ggui.Column(
				chatSurface(ggui.Box(scroller).Border(1, t.Border).Radius(t.Radius)),
				ggui.Wrap(
					ui.Button("Send turn", func() {
						sequence++
						id := fmt.Sprintf("turn-%d", sequence)
						ggui.Append(rows,
							ui.MessageEntry{ID: id, Content: ui.Message(ui.Bubble(ggui.Text(fmt.Sprintf("Review request %d", sequence))).End()).End(), Anchor: true},
							ui.MessageEntry{ID: id + "-reply", Content: ui.Message(ui.Bubble(ggui.Text("Starting the review…")).Secondary())})
					}),
					ui.Button("Stream reply", func() {
						if len(ggui.Untrack(rows.Get)) == 0 {
							return
						}
						sequence++
						// Update copies the slice first: State is shallow, so editing
						// the entry in place would not publish the change.
						rows.Update(func(prev []ui.MessageEntry) []ui.MessageEntry {
							next := slices.Clone(prev)
							next[len(next)-1].Content = ui.Message(ui.Bubble(ggui.Text(fmt.Sprintf("Review in progress. Pass %d. ", sequence) + repeatReview(sequence))).Secondary())
							return next
						})
					}).Outline(),
					ui.Button("Load earlier", func() {
						history++
						rows.Update(func(prev []ui.MessageEntry) []ui.MessageEntry {
							note := ui.MessageEntry{ID: fmt.Sprintf("history-%d", history), Content: ui.Marker(ggui.Text(fmt.Sprintf("Earlier note %d: the reader stays in place.", history))).Border()}
							return append([]ui.MessageEntry{note}, prev...)
						})
					}).Outline(),
					ui.Button("Save position", func() {
						saved = scroller.Save()
						scrollNote.Set("Saved the current reading position.")
					}).Outline(),
					ui.Button("Restore position", func() {
						scroller.Restore(saved)
						scrollNote.Set("Restored the saved reading position.")
					}).Outline(),
				).Gap(8), ggui.TextOf(scrollNote).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption),
			).Gap(12).Align(ggui.AlignStretch)),
			preview("Questionnaire", ggui.Column(
				chatSurface(questionnaire, 448),
				ggui.TextOf(submission).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption),
				ui.Button("Reset questionnaire", func() {
					questionnaire.Reset()
					submission.Set("Your answers stay local to this preview.")
				}).Outline(),
			).Gap(16).Align(ggui.AlignStretch)),
		}
	}
}
func repeatReview(n int) string {
	out := ""
	for range min(n, 20) {
		out += "The layout, keyboard behavior and state changes are being checked. "
	}
	return out
}
