package ui_test

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"testing"
)

func TestSelectPopupStaysWithinTriggerWidth(t *testing.T) {
	value := ggui.State("Alpha")
	selectBox := ui.Select(value, []string{"Alpha", "Beta"}).Named("Choose option")
	p := ggui.NewProbe(ggui.Box(selectBox).Width(180), ggui.Sz(900, 400))
	defer p.Close()
	p.Tap("Choose option")
	trigger, ok := p.Find("Choose option")
	if !ok {
		t.Fatal("missing trigger")
	}
	option, ok := p.Find("Beta")
	if !ok {
		t.Fatal("missing option")
	}
	if option.Rect.Size.W > trigger.Rect.Size.W {
		t.Fatalf("popup expanded to viewport: option %v, trigger %v", option.Rect, trigger.Rect)
	}
}
