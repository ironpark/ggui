package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// The reader and format forms of the text helpers follow their values and
// keep the role the plain forms give.
func TestTextHelpersFollowValues(t *testing.T) {
	status, count := ggui.State("Saved"), ggui.State(3)
	p := ggui.NewProbe(ggui.Column(ui.CaptionOf(status), ui.Titlef("%d left", count)), ggui.Sz(200, 80))
	defer p.Close()
	p.Frame()
	status.Set("Saving…")
	count.Set(2)
	p.Frame()
	if _, ok := p.Semantics().Find(ggui.RoleText, "Saving…"); !ok {
		t.Error("CaptionOf did not follow its value")
	}
	if _, ok := p.Semantics().Find(ggui.RoleHeading, "2 left"); !ok {
		t.Error("Titlef did not follow its argument as a heading")
	}
}
