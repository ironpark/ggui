package ui

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
