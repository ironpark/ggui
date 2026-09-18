package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// These previews keep transcript and questionnaire instances alive while the
// gallery filters or theme change. No network or file access is needed.
func newChatPreviews() func() []ggui.Widget {
	upload := ggui.State(ui.AttachmentIdle)
	states := []ui.AttachmentState{ui.AttachmentIdle, ui.AttachmentUploading, ui.AttachmentProcessing, ui.AttachmentError, ui.AttachmentDone}
	files := ggui.State([]string{"brief.pdf", "customers.csv", "renderer.go"})
	attachmentAction := ggui.State("Open a file preview or remove an attachment.")
	thumb := attachmentThumbnail()
	ggui.OnCleanup(thumb.Deallocate)
	reaction := ggui.State(0)
	expanded := ggui.State(false)
	bubbleAction := ggui.State("Choose a suggested reply.")
	rows := ggui.State([]ui.MessageEntry{
		{ID: "welcome", Content: ui.Marker(ggui.Text("Today")).Separator()},
		{ID: "hello", Content: ui.Message(ui.Bubble(ggui.Text("Can you review this patch?")).End()).End(), Anchor: true},
		{ID: "reply", Content: ui.Message(ui.Bubble(ggui.Text("I will check the implementation and tests.")).Secondary()).Avatar(ui.Avatar("Reviewer").Size(32))},
	})
	scroller := ui.MessageScroller(rows).Height(260).AutoScroll(true).Named("Preview conversation")
	sequence, history := 0, 0
	var saved ui.TranscriptPosition
	scrollNote := ggui.State("Scroll up to pause following. Jump to the end to resume.")
	answers := ggui.State(ui.QuestionAnswers{})
	submission := ggui.State("Your answers stay local to this preview.")
	questionnaire := ui.Questionnaire(answers,
		ui.Question{Name: "scope", Title: "What should we build next?", Description: "Choose a direction or describe another feature.", Required: true,
			Choices: []ui.QuestionOption{{Value: "timeline", Label: "Activity timeline", Description: "Show actions and their results."}, {Value: "approvals", Label: "Approval checkpoints", Description: "Review important actions before they run."}, {Value: "unavailable", Label: "Coming soon", Disabled: true}}, InputLabel: "Another feature", Placeholder: "Describe another feature…"},
		ui.Question{Name: "updates", Title: "What should updates include?", Description: "Select multiple items, or explicitly skip this step.", Multiple: true,
			Choices: []ui.QuestionOption{{Value: "progress", Label: "Progress updates"}, {Value: "decisions", Label: "Decisions"}, {Value: "risks", Label: "Risks"}}},
		ui.Question{Name: "context", Title: "Who will use this?", Description: "Add a short audience description.", Required: true, InputLabel: "Audience", Placeholder: "For example, our support team", Validate: func(a ui.QuestionAnswer) string {
			if len([]rune(a.Text)) < 3 {
				return "Please use at least three characters."
			}
			return ""
		}},
	).Shortcuts(ui.QuestionNumbers).SubmitLabel("Save answers").OnSubmit(func(a ui.QuestionAnswers) {
		submission.Set(fmt.Sprintf("Saved %d answers. Audience: %s", len(a), a["context"].Text))
	})

	return func() []ggui.Widget {
		t := ggui.UseTheme()
		return []ggui.Widget{
			preview("Attachment", ggui.Column(
				ui.Select(upload, states).Named("Upload state"),
				ui.Attachment("design-system.zip", "Choose an upload state above").Media(ggui.Text("ZIP").Size(11)).StateOf(upload).Width(300).
					Trigger("Preview design system", func() { attachmentAction.Set("Preview opened: design-system.zip") }).
					Actions(ui.AttachmentAction("Retry upload", ggui.Text("↻"), func() { upload.Set(ui.AttachmentUploading) })),
				ggui.Wrap(ui.Attachment("Default", "PDF · 2.4 MB"), ui.Attachment("Small", "CSV · 18 KB").Size(ui.AttachmentSmall), ui.Attachment("Extra small", "").Size(ui.AttachmentExtraSmall)).Gap(8),
				ggui.View(files, func(names []string) *ui.AttachmentGroupWidget {
					cards := []*ui.AttachmentWidget{ui.Attachment("workspace.png", "PNG · 820 KB").Image(thumb, "Illustrated workspace").Vertical().Width(140).Trigger("Preview workspace", func() { attachmentAction.Set("Preview opened: workspace.png") })}
					for _, name := range names {
						cards = append(cards, ui.Attachment(name, "Ready to upload").State(ui.AttachmentIdle).Width(200).Media(ggui.Text("File").Size(11)).Trigger("Preview "+name, func() { attachmentAction.Set("Preview opened: " + name) }).Actions(ui.AttachmentAction("Remove "+name, ggui.Text("×"), func() { ggui.Remove(files, func(v string) bool { return v == name }) })))
					}
					return ui.AttachmentGroup(cards...).Named("Attached files")
				}),
				ggui.TextOf(attachmentAction).AsCaption(),
				ui.Button("Restore attachments", func() { files.Set([]string{"brief.pdf", "customers.csv", "renderer.go"}) }).Outline(),
			).Gap(12).Align(ggui.AlignStretch)),
			preview("Bubble", ggui.Column(
				ui.Bubble(ggui.Text("Primary, aligned to the end.")).End(),
				ui.Bubble(ggui.Text("Secondary conversation surface.")).Secondary(),
				ui.Bubble(ggui.Text("Muted supporting content.")).Muted(),
				ui.Bubble(ggui.Text("A subtle primary tint.")).Tinted(),
				ui.Bubble(ggui.Text("Choose this suggestion")).Outline().Action("Choose suggestion", func() { bubbleAction.Set("Suggestion selected.") }),
				ui.Bubble(ggui.Text("Upload failed. Please retry.")).Destructive(),
				ui.Bubble(ggui.Text("Ghost content has no frame and can use the full width.")).Ghost(),
				ui.Bubble(ui.Collapsible(expanded, "Show more", ggui.Text("Long content composes with Collapsible without losing its state."))).Secondary().Reactions(ui.ButtonOf(ggui.Textf("Like · %d", reaction), func() { ggui.Add(reaction, 1) }).Named("Like bubble").Ghost().Pad(2, 6)),
				ggui.Padding(ggui.TextOf(bubbleAction).AsCaption(), 16, 0, 0, 0),
			).Gap(14).Align(ggui.AlignStretch)),
			preview("Message", ui.MessageGroup(
				ui.Message(ui.Bubble(ggui.Text("The report is ready to review.")).Secondary()).Avatar(ui.Avatar("Ada Lovelace").Size(32)).Header(ggui.Text("Ada Lovelace")).Footer(ggui.Caption("10:42 · Delivered")),
				ui.Message(ui.Bubble(ggui.Text("Thanks! I will take a look.")).End()).End().Avatar(ui.Avatar("You").Size(32)).Header(ggui.Text("You")).Footer(ui.Button("Acknowledge", func() { bubbleAction.Set("Message acknowledged.") }).Ghost().Pad(2, 6)),
				ui.Message(ui.Attachment("report.pdf", "PDF · 2.4 MB").Media(ggui.Text("PDF").Size(11))).Header(ggui.Text("Shared attachment")),
			)),
			preview("Marker", ggui.Column(
				ui.Marker(ggui.Text("Reviewing the transcript…")).Icon(ui.Spinner().Size(16)),
				ui.Marker(ggui.Text("Yesterday")).Separator(),
				ui.Marker(ggui.Text("A reviewer joined the conversation.")).Border(),
				ui.Marker(ui.Button("View review details", func() { bubbleAction.Set("Review details opened.") }).Ghost().Pad(0)),
			).Gap(20).Align(ggui.AlignStretch)),
			preview("Message Scroller", ggui.Column(
				ggui.Box(scroller).Border(1, t.Border).Radius(t.Radius),
				ggui.Wrap(
					ui.Button("Send turn", func() {
						sequence++
						id := fmt.Sprintf("turn-%d", sequence)
						next := slices.Clone(rows.Peek())
						next = append(next, ui.MessageEntry{ID: id, Content: ui.Message(ui.Bubble(ggui.Text(fmt.Sprintf("Review request %d", sequence))).End()).End(), Anchor: true}, ui.MessageEntry{ID: id + "-reply", Content: ui.Message(ui.Bubble(ggui.Text("Starting the review…")).Secondary())})
						rows.Set(next)
					}),
					ui.Button("Stream reply", func() {
						next := slices.Clone(rows.Peek())
						if len(next) == 0 {
							return
						}
						sequence++
						i := len(next) - 1
						next[i].Content = ui.Message(ui.Bubble(ggui.Text(fmt.Sprintf("Review in progress. Pass %d. ", sequence) + repeatReview(sequence))).Secondary())
						rows.Set(next)
					}).Outline(),
					ui.Button("Load earlier", func() {
						history++
						rows.Set(append([]ui.MessageEntry{{ID: fmt.Sprintf("history-%d", history), Content: ui.Marker(ggui.Text(fmt.Sprintf("Earlier note %d: the reader stays in place.", history))).Border()}}, rows.Peek()...))
					}).Outline(),
					ui.Button("Save position", func() { saved = scroller.Save(); scrollNote.Set("Saved the current reading position.") }).Outline(),
					ui.Button("Restore position", func() { scroller.Restore(saved); scrollNote.Set("Restored the saved reading position.") }).Outline(),
				).Gap(8), ggui.TextOf(scrollNote).AsCaption(),
			).Gap(12).Align(ggui.AlignStretch)),
			preview("Questionnaire", ggui.Column(questionnaire, ggui.TextOf(submission).AsCaption(), ui.Button("Reset questionnaire", func() { questionnaire.Reset(); submission.Set("Your answers stay local to this preview.") }).Outline()).Gap(16).Align(ggui.AlignStretch)),
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
func attachmentThumbnail() *ebiten.Image {
	img := image.NewRGBA(image.Rect(0, 0, 160, 120))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{224, 232, 240, 255}), image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(18, 20, 142, 100), image.NewUniform(color.RGBA{58, 76, 100, 255}), image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(24, 26, 136, 84), image.NewUniform(color.RGBA{168, 196, 212, 255}), image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(50, 104, 110, 110), image.NewUniform(color.RGBA{58, 76, 100, 255}), image.Point{}, draw.Src)
	return ebiten.NewImageFromImage(img)
}
