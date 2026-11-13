# shadcn/ui defaults

[Documentation](README.md) · [Gallery](../examples/gallery)

The default visual reference is **Base UI / base-nova / Neutral**, inspected on
2026-11-13 through the [current component docs](https://ui.shadcn.com/docs/components)
and each component's Manual installation source. The old New York registry is
not the reference for these defaults. The gallery now starts in Neutral/Nova,
with accent and Rhea customization still available.

## Implemented defaults

| Area | Default behavior and appearance |
| --- | --- |
| Theme | Neutral light/dark semantic palette, 14px body type, 10px control radius, 8px inner radius, 14px large radius. |
| [Button](https://ui.shadcn.com/docs/components/base/button) | 24/28/32/36px text and icon variants; 32px default with 10px horizontal padding; no default shadow; primary hover at 80%; muted outline/ghost hover; subtle destructive surface with red text; expanded trigger styling; 1px pressed translation for non-popup actions; default arrow cursor. |
| [Input](https://ui.shadcn.com/docs/components/base/input) / [Textarea](https://ui.shadcn.com/docs/components/base/textarea) | 32px input minimum; 64px multiline minimum with content growth; dark input surface and disabled fill; shared focus/invalid halo. Explicit line counts remain supported. |
| [Card](https://ui.shadcn.com/docs/components/base/card) | 16px padding, foreground/10 outline, 14px default radius, no default shadow. |
| [Badge](https://ui.shadcn.com/docs/components/base/badge) | Primary default, 20px height, 12px text, 8px horizontal padding, subtle destructive surface. `.Secondary()` selects the former default. |
| [Switch](https://ui.shadcn.com/docs/components/base/switch) | 32 × 18.4px track and 16px thumb; dark unchecked track and thumb colors follow the reference. |
| [Checkbox](https://ui.shadcn.com/docs/components/base/checkbox) / [Radio](https://ui.shadcn.com/docs/components/base/radio-group) | 16px glyph; checkbox 4px radius and 14px check; selected radio fills primary with an 8px contrasting dot. |
| [Slider](https://ui.shadcn.com/docs/components/base/slider) | 4px track, 12px white thumb with ring-colored outline, 3px hover/focus halo; Home/End, PageUp/PageDown and Shift-arrow larger steps. |
| [Progress](https://ui.shadcn.com/docs/components/base/progress) | 4px muted track; primary indicator animates while accessibility reports the actual value immediately. |
| [Accordion](https://ui.shadcn.com/docs/components/base/accordion) | 10px vertical header padding and content bottom padding, independent enabled header Tab stops, 200ms disclosure, up/down icon substitution. Existing arrow navigation remains available. |
| [Tabs](https://ui.shadcn.com/docs/components/base/tabs) | Compact 32px list geometry; background-colored active segment with dark input surface and border. |
| [Dialog](https://ui.shadcn.com/docs/components/base/dialog) | 384px maximum default width, 16px outer margin/padding, 100ms fade/95% scale, black/10 scrim with 4px backdrop blur, default close button. |
| [Sheet](https://ui.shadcn.com/docs/components/base/sheet) | Right-side default, 75% viewport width capped at 384px, 200ms ease-in-out, fade and 40px entrance translation, default close button. Explicit `.Size()` overrides remain. |
| [Tooltip](https://ui.shadcn.com/docs/components/base/tooltip) | No default opening delay, top/center placement with 4px gap and collision fallback, 320px max width, 8px radius, 12/6px horizontal/vertical padding. |
| [Hover Card](https://ui.shadcn.com/docs/components/base/hover-card) | 256px width, 10px radius/padding, 600ms opening delay, 300ms closing grace period, 100ms transition. |
| [Alert](https://ui.shadcn.com/docs/components/base/alert) | 10/8px horizontal/vertical padding, 2px row gap, medium title; destructive changes text while retaining the neutral border. |
| [Skeleton](https://ui.shadcn.com/docs/components/base/skeleton) | 8px corners, muted fill, existing 2-second opacity pulse. |
| Floating menus and shared controls | 4px popup padding, 6/4px item padding, foreground/10 popup outline, 100ms popup duration, soft 3px focus halos. |

Dimensions are logical pixels at normal text scale. Buttons allow larger text
to grow instead of clipping it. Explicit `Pad`, `Radius`, theme overrides,
custom variants, and reduced-motion preferences remain supported.

## Compatibility and verification

Default sizes and colors intentionally change existing applications. Use
`theme.Override`, variant restyling, and explicit padding to preserve a custom
look. `DestructiveFg` remains available for custom solid variants; built-in
subtle destructive buttons/badges use `Destructive` for the text. `MotionFast`
is 150ms, `MotionSlow` 200ms, and the new `MotionOverlay` 100ms. Setting motion
tokens to zero or providing `ReducedMotionKey` disables their animations.

Dialogs and sheets expose `.CloseButton(false)` for surfaces that supply their
own closing UI. AlertDialog and CommandDialog omit this button by default.
HoverCard exposes `.CloseDelay(...)`; Tooltip's existing `.Delay(...)` can
restore delayed opening. Accordion's group is a semantic container; its
individual headers now own focus and keep identity across keyed rebuilds.

Checks include exact control sizes in default/preset light and dark themes,
text scaling, independent accordion tab stops, close actions, retained pointer
identity, overlay keyboard flow, reduced motion and progress semantics. Visual
captures use the actual gallery in both themes, Nova/Rhea, and 320/640px widths.

```sh
go test ./...
go vet ./...
go run ./examples/gallery -render-dir /tmp/ggui-base-nova-final \
  -render-examples 'Buttons,Text,Choices,Accordion,Dialog,Sheets and drawers,Tabs,Control states,Notices and empty states,Progress,Profile and media'
```

## Follow-up implementation

The subsequent multi-agent implementation adds the missing catalog APIs and
behavioral modes. See the [66-component audit, API examples and remaining
boundaries](shadcn-implementation.md). This includes Popover, Navigation Menu,
Code Block, Trace, Label, Toggle and Native Select; multi-thumb sliders,
calendar selection modes, multiple-choice comboboxes and toggles, vertical/manual
Tabs, selected-item-aligned Select, tooltip arrows, and draggable Drawer snaps.

The default Tabs keyboard behavior is now manual activation: arrows move focus,
Space or Enter changes the panel. `.ActivateOnFocus(true)` restores automatic
activation. Each tab and multiple-toggle item is a real accessibility focus
target, with one roving Tab stop per group. Slider step defaults are now 1 and
10 for normal/large movement; `.Step(0)` retains continuous pointer adjustment.

This is a native adaptation, not React/DOM API compatibility. Native font metrics,
text shaping and color interpolation can differ from CSS. The implementation
audit separately records unsupported advanced browser behaviors rather than
claiming pixel-for-pixel or prop-for-prop equivalence.
