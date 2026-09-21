package ui

import "github.com/ironpark/ggui"

// Key setters return the concrete control so identities can be assigned inside
// widget expressions. Custom controls use Interactive.SetKey to set identity.

// Key assigns this control an input identity. Configure it before mount.
func (w *AccordionWidget) Key(k any) *AccordionWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *AttachmentGroupWidget) Key(k any) *AttachmentGroupWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *BubbleWidget) Key(k any) *BubbleWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *ButtonWidget) Key(k any) *ButtonWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *CalendarWidget) Key(k any) *CalendarWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *CarouselWidget) Key(k any) *CarouselWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *CarouselNavigationWidget) Key(k any) *CarouselNavigationWidget {
	w.Interactive.SetKey(k)
	return w
}

// Key assigns this control an input identity. Configure it before mount.
func (w *ChartWidget) Key(k any) *ChartWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *CheckboxWidget) Key(k any) *CheckboxWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *CollapsibleWidget) Key(k any) *CollapsibleWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *ContextMenuWidget) Key(k any) *ContextMenuWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *ToggleGroupWidget[T]) Key(k any) *ToggleGroupWidget[T] { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *InputGroupWidget) Key(k any) *InputGroupWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *InputOTPWidget) Key(k any) *InputOTPWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *MenuItemWidget) Key(k any) *MenuItemWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *MenubarWidget) Key(k any) *MenubarWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *MessageScrollerWidget) Key(k any) *MessageScrollerWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *PaginationWidget) Key(k any) *PaginationWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *QuestionnaireWidget) Key(k any) *QuestionnaireWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *RadioWidget[T]) Key(k any) *RadioWidget[T] { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *RadiosWidget[T]) Key(k any) *RadiosWidget[T] { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *ResizableWidget) Key(k any) *ResizableWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *SelectWidget[T]) Key(k any) *SelectWidget[T] { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *SidebarWidget) Key(k any) *SidebarWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *SliderWidget) Key(k any) *SliderWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *SwitchWidget) Key(k any) *SwitchWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *TabsWidget) Key(k any) *TabsWidget { w.Interactive.SetKey(k); return w }

// Key assigns this control an input identity. Configure it before mount.
func (w *ThemeSwitchWidget) Key(k any) *ThemeSwitchWidget { w.Interactive.SetKey(k); return w }

// BindName follows a non-nil accessible-name reader.
func (w *AccordionWidget) BindName(r ggui.Readable[string]) *AccordionWidget {
	w.Interactive.BindName(r)
	return w
}

// BindName follows a non-nil accessible-name reader.
func (w *AttachmentGroupWidget) BindName(r ggui.Readable[string]) *AttachmentGroupWidget {
	w.Interactive.BindName(r)
	return w
}

// Name sets the accessible name.
func (w *BubbleWidget) Name(name string) *BubbleWidget { w.SetName(name); return w }

// BindName follows a non-nil accessible-name reader.
func (w *BubbleWidget) BindName(r ggui.Readable[string]) *BubbleWidget {
	w.Interactive.BindName(r)
	return w
}

// BindName follows a non-nil accessible-name reader.
func (w *CalendarWidget) BindName(r ggui.Readable[string]) *CalendarWidget {
	w.Interactive.BindName(r)
	return w
}

// BindName follows a non-nil accessible-name reader.
func (w *CarouselWidget) BindName(r ggui.Readable[string]) *CarouselWidget {
	w.Interactive.BindName(r)
	return w
}

// Name sets the accessible name.
func (w *CarouselNavigationWidget) Name(name string) *CarouselNavigationWidget {
	w.SetName(name)
	return w
}

// BindName follows a non-nil accessible-name reader.
func (w *CarouselNavigationWidget) BindName(r ggui.Readable[string]) *CarouselNavigationWidget {
	w.Interactive.BindName(r)
	return w
}

// BindName follows a non-nil accessible-name reader.
func (w *ChartWidget) BindName(r ggui.Readable[string]) *ChartWidget {
	w.Interactive.BindName(r)
	return w
}

// Name sets the accessible name.
func (w *CheckboxWidget) Name(name string) *CheckboxWidget { w.SetName(name); return w }

// BindName follows a non-nil accessible-name reader.
func (w *CheckboxWidget) BindName(r ggui.Readable[string]) *CheckboxWidget {
	w.Interactive.BindName(r)
	return w
}

// Name sets the accessible name.
func (w *CollapsibleWidget) Name(name string) *CollapsibleWidget { w.SetName(name); return w }

// BindName follows a non-nil accessible-name reader.
func (w *CollapsibleWidget) BindName(r ggui.Readable[string]) *CollapsibleWidget {
	w.Interactive.BindName(r)
	return w
}

// BindName follows a non-nil accessible-name reader.
func (w *ContextMenuWidget) BindName(r ggui.Readable[string]) *ContextMenuWidget {
	w.Interactive.BindName(r)
	return w
}

// BindName follows a non-nil accessible-name reader.
func (w *ToggleGroupWidget[T]) BindName(r ggui.Readable[string]) *ToggleGroupWidget[T] {
	w.Interactive.BindName(r)
	return w
}

// BindName follows a non-nil accessible-name reader.
func (w *InputGroupWidget) BindName(r ggui.Readable[string]) *InputGroupWidget {
	w.Interactive.BindName(r)
	if n, ok := w.input.(Named); ok && !n.HasName() {
		w.nameChild = true
	}
	return w
}

// Name sets the accessible name.
func (w *MenuItemWidget) Name(name string) *MenuItemWidget { w.SetName(name); return w }

// BindName follows a non-nil accessible-name reader.
func (w *MenuItemWidget) BindName(r ggui.Readable[string]) *MenuItemWidget {
	w.Interactive.BindName(r)
	return w
}

// BindName follows a non-nil accessible-name reader.
func (w *MenubarWidget) BindName(r ggui.Readable[string]) *MenubarWidget {
	w.Interactive.BindName(r)
	return w
}

// BindName follows a non-nil accessible-name reader.
func (w *MessageScrollerWidget) BindName(r ggui.Readable[string]) *MessageScrollerWidget {
	w.Interactive.BindName(r)
	return w
}

// Name sets the accessible name.
func (w *PaginationWidget) Name(name string) *PaginationWidget { w.SetName(name); return w }

// BindName follows a non-nil accessible-name reader.
func (w *PaginationWidget) BindName(r ggui.Readable[string]) *PaginationWidget {
	w.Interactive.BindName(r)
	return w
}

// BindName follows a non-nil accessible-name reader.
func (w *QuestionnaireWidget) BindName(r ggui.Readable[string]) *QuestionnaireWidget {
	w.Interactive.BindName(r)
	return w
}

// Name sets the accessible name.
func (w *RadioWidget[T]) Name(name string) *RadioWidget[T] { w.SetName(name); return w }

// BindName follows a non-nil accessible-name reader.
func (w *RadioWidget[T]) BindName(r ggui.Readable[string]) *RadioWidget[T] {
	w.Interactive.BindName(r)
	return w
}

// BindName follows a non-nil accessible-name reader.
func (w *RadiosWidget[T]) BindName(r ggui.Readable[string]) *RadiosWidget[T] {
	w.Interactive.BindName(r)
	return w
}

// BindName follows a non-nil accessible-name reader.
func (w *ResizableWidget) BindName(r ggui.Readable[string]) *ResizableWidget {
	w.Interactive.BindName(r)
	return w
}

// BindName follows a non-nil accessible-name reader.
func (w *SelectWidget[T]) BindName(r ggui.Readable[string]) *SelectWidget[T] {
	w.Interactive.BindName(r)
	return w
}

// BindName follows a non-nil accessible-name reader.
func (w *SidebarWidget) BindName(r ggui.Readable[string]) *SidebarWidget {
	w.Interactive.BindName(r)
	return w
}

// BindName follows a non-nil accessible-name reader.
func (w *SliderWidget) BindName(r ggui.Readable[string]) *SliderWidget {
	w.Interactive.BindName(r)
	return w
}

// Name sets the accessible name.
func (w *SwitchWidget) Name(name string) *SwitchWidget { w.SetName(name); return w }

// BindName follows a non-nil accessible-name reader.
func (w *SwitchWidget) BindName(r ggui.Readable[string]) *SwitchWidget {
	w.Interactive.BindName(r)
	return w
}

// Name sets the accessible name.
func (w *TabsWidget) Name(name string) *TabsWidget { w.SetName(name); return w }

// BindName follows a non-nil accessible-name reader.
func (w *TabsWidget) BindName(r ggui.Readable[string]) *TabsWidget {
	w.Interactive.BindName(r)
	return w
}

// BindName follows a non-nil accessible-name reader.
func (w *ThemeSwitchWidget) BindName(r ggui.Readable[string]) *ThemeSwitchWidget {
	w.Interactive.BindName(r)
	return w
}
