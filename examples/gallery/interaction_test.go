package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

func galleryProbe(size ggui.Size) *ggui.Probe {
	var p *ggui.Probe
	dispose := ggui.Root(func() {
		build, setup, commands := newGallery()
		p = ggui.ProbeBuilder(build, size).Setup(setup)
		p.Shortcut("cmd+k", commands)
	})
	p.Setup(func() { ggui.OnCleanup(dispose) })
	p.Frame()
	return p
}
func searchGallery(p *ggui.Probe, query string) {
	p.Tap("All")
	p.Tap("Search components")
	p.Type(ggui.Mods{Meta: true}, ggui.KeyA)
	pasteText(p, query)
	p.Frame()
}
func TestGalleryRealInteractions(t *testing.T) {
	p := galleryProbe(ggui.Sz(1180, 820))
	defer p.Close()
	for _, size := range []ggui.Size{ggui.Sz(1600, 1000), ggui.Sz(640, 700), ggui.Sz(480, 640), ggui.Sz(360, 480), ggui.Sz(480, 320), ggui.Sz(1180, 820)} {
		p.Resize(size)
		p.Frame()
		for _, label := range []string{"Search components", "Commands", "Dark mode", "Layout", "Data"} {
			r, ok := p.Find(label)
			if !ok || r.Rect.Empty() || r.Rect.Origin.X < 0 || r.Rect.Origin.Y < 0 || r.Rect.Origin.X+r.Rect.Size.W > size.W || r.Rect.Origin.Y+r.Rect.Size.H > size.H {
				t.Fatalf("%v: %s not visible: %+v", size, label, r)
			}
		}
		p.Tap("Dark mode")
	}
	searchGallery(p, "Choices")
	p.Tap("Send notifications")
	p.Tap("pro")
	p.Tap("Choose fruit")
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	searchGallery(p, "Accordion")
	p.Tap("Keyboard controls")
	p.Type(ggui.Mods{}, ggui.KeyArrowUp, ggui.KeyEnter)
	searchGallery(p, "Resizable")
	handles := p.FindAll(ggui.RoleSeparator)
	if len(handles) != 2 {
		t.Fatalf("nested split: %d handles", len(handles))
	}
	for _, h := range handles {
		p.Click(h.Center())
		p.Type(ggui.Mods{}, ggui.KeyArrowRight, ggui.KeyArrowDown)
	}
	p.Type(ggui.Mods{Meta: true}, ggui.KeyK)
	if _, ok := p.Find("Search commands"); !ok {
		t.Fatal("command palette missing")
	}
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	searchGallery(p, "Dialog")
	p.Tap("Reset form…")
	p.Tap("Cancel")
	searchGallery(p, "Pagination")
	p.Tap("Next")
	searchGallery(p, "Text")
	p.Tap("Type here, IME works")
	pasteText(p, "Layout QA")
	p.Resize(ggui.Sz(480, 640))
	p.Frame()
	p.Resize(ggui.Sz(1180, 820))
	p.Frame()
	searchGallery(p, "xyz no match")
	p.Tap("Clear filters")
}

// The native editor takes text commits through its IME; test paste through
// the same editing path without reading or writing the system clipboard.
func pasteText(p *ggui.Probe, text string) {
	clip := &ggui.MemoryClipboard{}
	clip.Write(text)
	ggui.SetClipboard(clip)
	p.Type(ggui.Mods{Meta: true}, ggui.KeyV)
}

func TestGallerySmallWindowPopups(t *testing.T) {
	for _, size := range []ggui.Size{ggui.Sz(360, 480), ggui.Sz(480, 320), ggui.Sz(640, 480)} {
		t.Run(fmt.Sprintf("%gx%g", size.W, size.H), func(t *testing.T) {
			p := galleryProbe(size)
			defer p.Close()
			p.Tap("Commands")
			search, ok := p.Find("Search commands")
			if !ok || search.Rect.Empty() {
				t.Fatal("palette search is not reachable")
			}
			for _, r := range p.FindAll(ggui.RoleMenuItem) {
				if r.Rect.Origin.Y+r.Rect.Size.H > size.H || r.Rect.Origin.X+r.Rect.Size.W > size.W {
					t.Fatalf("command outside window: %+v", r)
				}
			}
			p.Type(ggui.Mods{}, ggui.KeyEscape)
			searchGallery(p, "Dialog")
			// Tab reveals controls that start below the viewport on short windows.
			revealGallery(p, "Reset form…")
			p.Tap("Reset form…")
			for _, label := range []string{"Cancel", "Reset"} {
				r, ok := p.Find(label)
				if !ok || r.Rect.Empty() || r.Rect.Origin.Y+r.Rect.Size.H > size.H {
					t.Fatalf("dialog control not reachable: %s %+v", label, r)
				}
			}
			p.Tap("Cancel")
		})
	}
}
func revealGallery(p *ggui.Probe, label string) {
	for range 60 {
		r, ok := p.Find(label)
		if ok && !r.Rect.Empty() {
			return
		}
		p.Type(ggui.Mods{}, ggui.KeyTab)
	}
}

func TestGalleryControlFlows(t *testing.T) {
	p := galleryProbe(ggui.Sz(1180, 820))
	defer p.Close()
	searchGallery(p, "Buttons")
	p.Tap("Save changes")
	if _, ok := p.Find("Dismiss Saved"); !ok {
		t.Fatal("save feedback missing")
	}
	p.Tap("Dismiss Saved")
	if _, ok := p.Find("Disabled"); ok {
		t.Fatal("disabled button accepts input")
	}
	searchGallery(p, "Search and commands")
	p.Tap("Search fruit")
	p.Tap("Search options")
	pasteText(p, "Mango")
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if _, ok := p.Find("Search options"); ok {
		t.Fatal("combobox did not close after selection")
	}
	searchGallery(p, "Toast")
	p.Tap("Notify with action")
	p.Tap("Undo")
	if _, ok := p.Find("Dismiss Restored"); !ok {
		t.Fatal("toast undo failed")
	}
	p.Tap("Dismiss Restored")
	p.Tap("Persistent error")
	p.Tap("Dismiss Upload failed")
	searchGallery(p, "Tabs")
	p.Tap("Details")
	p.Tap("More options")
	p.Type(ggui.Mods{}, ggui.KeyTab)
	p.Tap("Overview")
	p.Tap("+10%")
	p.Tap("Reset")
	searchGallery(p, "Transition")
	p.Tap("Show details")
	p.Advance(300 * time.Millisecond)
	p.Tap("Show details")
	searchGallery(p, "Wrap")
	p.Tap("go  ×")
	if _, ok := p.Find("go  ×"); ok {
		t.Fatal("removed tag still visible")
	}
	searchGallery(p, "Table")
	p.Tap("Ada")
	p.Tap("×")
	if _, ok := p.Find("Ada"); ok {
		t.Fatal("deleted table row remains visible")
	}

	searchGallery(p, "Resizable")
	handles := p.FindAll(ggui.RoleSeparator)
	first := handles[len(handles)-1]
	p.Press(first.Center())
	p.Move(first.Center().Add(ggui.Pt(80, 0)))
	p.Release(first.Center().Add(ggui.Pt(80, 0)))
	moved := p.FindAll(ggui.RoleSeparator)
	if moved[len(moved)-1].Rect.Origin.X == first.Rect.Origin.X {
		t.Fatal("splitter drag did not move")
	}
}

// TestGalleryNavigationAndOverlays drives the previews added for the
// navigation, grouping and composition components in the real tree, where
// they share a window with everything else.
func TestGalleryNavigationAndOverlays(t *testing.T) {
	p := galleryProbe(ggui.Sz(1180, 820))
	defer p.Close()

	searchGallery(p, "Sidebar")
	p.Tap("Sent")
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	p.Tap("Home")

	searchGallery(p, "Groups and addons")
	p.Tap("right")
	if node, ok := p.Semantics().Find(ggui.RoleRadio, "right"); !ok || node.Checked != ggui.TriOn {
		t.Fatal("the toggle group did not take the click")
	}
	p.Tap("Cut")
	p.Frame()

	searchGallery(p, "Sheets and drawers")
	p.Tap("Open filters")
	p.Advance(400 * time.Millisecond)
	if _, ok := p.Find("Apply"); !ok {
		t.Fatal("the filters sheet did not open")
	}
	p.Tap("Apply")
	p.Advance(400 * time.Millisecond)
	if _, ok := p.Find("Apply"); ok {
		t.Fatal("the filters sheet is still taking input after it left")
	}

	searchGallery(p, "Dialog")
	p.Tap("Remove row…")
	p.Frame()
	p.Click(ggui.Pt(8, 8))
	if _, ok := p.Find("Remove"); !ok {
		t.Fatal("a click beside the confirmation dismissed it")
	}
	p.Tap("Remove")
	if _, ok := p.Find("Remove"); ok {
		t.Fatal("the confirmation stayed open after it was answered")
	}
}

func TestGallerySupplementedComponents(t *testing.T) {
	p := galleryProbe(ggui.Sz(1180, 820))
	defer p.Close()

	searchGallery(p, "Progress")
	for range 4 {
		p.Tap("Advance download")
		p.Frame()
	}
	p.Advance(time.Second)
	if n, ok := p.Semantics().Find(ggui.RoleProgress, ""); !ok || n.Now != 1 {
		t.Fatalf("download did not reach completion: found=%v node=%+v", ok, n)
	}
	if _, ok := p.Find("Advance download"); ok {
		t.Fatal("completed download still accepts advances")
	}
	p.Tap("Restart download")
	p.Frame()
	p.Advance(time.Second)
	if n, ok := p.Semantics().Find(ggui.RoleProgress, ""); !ok || n.Now != 0 {
		t.Fatal("download did not restart")
	}

	searchGallery(p, "Collapsible")
	p.Tap("Delivery preferences")
	p.Advance(400 * time.Millisecond)
	p.Tap("Email delivery updates")
	p.Tap("Delivery preferences")
	p.Advance(400 * time.Millisecond)
	if _, ok := p.Find("Email delivery updates"); ok {
		t.Fatal("collapsed content still accepts input")
	}
	p.Tap("Delivery preferences")
	p.Advance(400 * time.Millisecond)
	if n, ok := p.Semantics().Find(ggui.RoleCheckbox, "Email delivery updates"); !ok || n.Checked != ggui.TriOff {
		t.Fatal("delivery preference was lost on reopen")
	}

	searchGallery(p, "ToggleGroup")
	p.Tap("right")
	p.Tap("Lock alignment")
	p.Frame()
	if _, ok := p.Find("left"); ok {
		t.Fatal("locked alignment remains interactive")
	}
	p.Tap("Lock alignment")
	p.Frame()
	if n, ok := p.Semantics().Find(ggui.RoleRadio, "right"); !ok || n.Checked != ggui.TriOn {
		t.Fatal("alignment was lost when unlocking")
	}
	p.Tap("Cut")
	p.Frame()
	if _, ok := p.Semantics().Find(ggui.RoleText, "Cut selected"); !ok {
		t.Fatal("button group action feedback missing")
	}
	p.Tap("Site")
	pasteText(p, "example.com")
	p.Tap("Go")
	p.Frame()
	if _, ok := p.Semantics().Find(ggui.RoleText, "Preview: https://example.com"); !ok {
		t.Fatal("input group preview missing")
	}
}

func TestGalleryChatComponentFlows(t *testing.T) {
	p := galleryProbe(ggui.Sz(1180, 900))
	defer p.Close()
	searchGallery(p, "Attachment")
	p.Tap("Cancel upload")
	if _, ok := p.Find("Cancel upload"); ok {
		t.Fatal("cancelled upload still visible")
	}
	p.Tap("Remove source attachment")
	if _, ok := p.Find("Remove source attachment"); ok {
		t.Fatal("removed source still visible")
	}
	revealGallery(p, "Retry upload")
	p.Tap("Retry upload")
	if n, ok := p.Semantics().Find(ggui.RoleGroup, "design-system.zip"); !ok || n.Description != "uploading" {
		t.Fatal("attachment lifecycle is not bound")
	}
	revealGallery(p, "Remove brief.pdf")
	p.Tap("Remove brief.pdf")
	if _, ok := p.Find("Remove brief.pdf"); ok {
		t.Fatal("attachment removal failed")
	}
	revealGallery(p, "Restore attachments")
	p.Tap("Restore attachments")
	searchGallery(p, "Bubble")
	revealGallery(p, "More bubble variants")
	p.Tap("More bubble variants")
	p.Frame()
	p.Advance(ggui.DefaultTheme().MotionFast)
	revealGallery(p, "Choose suggestion")
	p.Tap("Choose suggestion")
	if _, ok := p.Semantics().Find(ggui.RoleText, "Suggestion selected."); !ok {
		t.Fatal("suggestion action feedback missing")
	}
	searchGallery(p, "Message Scroller")
	p.Tap("Send turn")
	p.Tap("Stream reply")
	p.Tap("Load earlier")
	p.Tap("Save position")
	p.Tap("Restore position")
	searchGallery(p, "Questionnaire")
	revealGallery(p, "Next")
	p.Tap("Next")
	if _, ok := p.Semantics().Find(ggui.RoleText, "Choose an answer to continue."); !ok {
		t.Fatal("questionnaire validation missing")
	}
	p.Tap("Tool call timeline")
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	p.Tap("Skip")
	p.Tap("Audience")
	pasteText(p, "Support team")
	p.Tap("Save answers")
	if _, ok := p.Semantics().Find(ggui.RoleText, "Saved 2 answers. Audience: Support team"); !ok {
		t.Fatal("questionnaire submission failed")
	}
	p.Tap("Reset questionnaire")
	if _, ok := p.Find("Tool call timeline"); !ok {
		t.Fatal("questionnaire reset failed")
	}
}

func TestGalleryEmojiEditingAndThemePresets(t *testing.T) {
	p := galleryProbe(ggui.Sz(1180, 900))
	defer p.Close()
	searchGallery(p, "Emoji")
	p.Tap("Emoji text")
	p.Type(ggui.Mods{Meta: true}, ggui.KeyA)
	pasteText(p, "Hello 👩🏽‍💻")
	p.Type(ggui.Mods{}, ggui.KeyBackspace)
	if _, ok := p.Semantics().Find(ggui.RoleText, "Hello "); !ok {
		t.Fatal("emoji backspace split or lost text")
	}
	p.Tap("Insert emoji sequence")
	if _, ok := p.Semantics().Find(ggui.RoleText, "Hello  🏳️‍🌈 👨‍👩‍👧‍👦"); !ok {
		t.Fatal("emoji sequence insertion did not update text")
	}
	searchGallery(p, "Theme presets")
	p.Tap("Theme base")
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	p.Frame()
	want := ggui.ThemePreset{Base: ggui.BaseStone, Accent: ggui.AccentBlue, Style: ggui.StyleRhea}.Light()
	if ggui.UseTheme().Bg != want.Bg || ggui.UseTheme().Muted != want.Muted {
		t.Fatal("base palette not applied")
	}
	p.Tap("Theme style")
	p.Type(ggui.Mods{}, ggui.KeyArrowUp, ggui.KeyEnter)
	p.Frame()
	if ggui.UseTheme().Chat.BubbleRadius != 12 {
		t.Fatal("Nova geometry not applied")
	}
	p.Tap("Dark mode")
	p.Frame()
	want = ggui.ThemePreset{Base: ggui.BaseStone, Accent: ggui.AccentBlue, Style: ggui.StyleNova}.Dark()
	if ggui.UseTheme().Bg != want.Bg || ggui.UseTheme().Primary != want.Primary {
		t.Fatal("preset was lost during dark mode switch")
	}
}

func TestGalleryIconOverrideSurvivesThemeChange(t *testing.T) {
	p := galleryProbe(ggui.Sz(1180, 900))
	defer p.Close()
	searchGallery(p, "Icons")
	p.Tap("Replace check with plus")
	p.Tap("Dark mode")
	p.Frame()
	if n, ok := p.Semantics().Find(ggui.RoleSwitch, "Replace check with plus"); !ok || n.Checked != ggui.Tri(true) {
		t.Fatal("icon override reset on theme change")
	}
	p.Tap("Scoped icon override")
	if n, ok := p.Semantics().Find(ggui.RoleCheckbox, "Scoped icon override"); !ok || n.Checked != ggui.Tri(false) {
		t.Fatal("custom icons changed checkbox interaction")
	}
}
