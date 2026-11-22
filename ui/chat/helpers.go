// Package chat provides conversation and AI chat widgets for ggui:
// Message, Bubble, Marker, Attachment, AttachmentGroup, MessageScroller,
// Questionnaire, and Trace.
//
// Controls inherit design tokens from ui/theme (including Theme.Chat geometry)
// and localized strings from ui.ProvideStrings / ui.BindStrings.
package chat

import (
	"cmp"
	"image/color"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/gem"
	"github.com/ironpark/ggui/ui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// loopStep is how often an endless ambient animation (such as text shimmer)
// steps, matching ui's spinner/skeleton loop cadence so concurrent loops share
// the same frames.
const loopStep = time.Second / 30

func pick[T any](cond bool, a, b T) T { return gem.Pick(cond, a, b) }

func bounded(v, fallback float64) float64 { return gem.Bounded(v, fallback) }

func clamp[T cmp.Ordered](v, lo, hi T) T { return gem.Clamp(v, lo, hi) }

func fade(c color.Color, f float64) color.Color { return uitheme.Fade(c, f) }

func mix(a, b color.Color, amount float64) color.Color { return uitheme.Mix(a, b, amount) }

// childKey identifies a control a composite builds for itself by the
// composite's identity and the part it plays.
type childKey struct {
	owner, part any
}

// ownerOf is the identity a composite's parts are keyed under.
func ownerOf(i *ggui.Interactive, self any) any {
	if id := i.HitID(); id != nil {
		return id
	}
	return self
}

// traceStatus is how s words the status of a Trace step.
func traceStatus(s *ui.Strings, status TraceStatus) string {
	s = s.OrDefault()
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
func attachmentState(s *ui.Strings, state AttachmentState) string {
	s = s.OrDefault()
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

// localized lays its child out after apply has worded it in the Strings of the
// Env it is laid out in.
type localized struct {
	child ggui.Widget
	apply func(*ui.Strings)
}

// localize returns child, worded by apply at each layout.
func localize(child ggui.Widget, apply func(*ui.Strings)) *localized {
	return &localized{child: child, apply: apply}
}

func (l *localized) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	l.apply(ui.StringsOf(env))
	return l.child.Layout(c, env)
}

func (l *localized) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(l.child, r) }

// Baseline implements ggui.Baseliner: the child's.
func (l *localized) Baseline() (float64, bool) { return ggui.BaselineOf(l.child) }
