# shadcn/ui defaults

[Documentation](README.md) · [Gallery](../examples/gallery)

The default visual reference is **Base UI / base-nova / Neutral**, inspected on
2026-11-13 through the [current component docs](https://ui.shadcn.com/docs/components)
and each component's Manual installation source. The old New York registry is
not the reference for these defaults. `theme.Default()` and `theme.Dark()` are
exactly `Preset{}`, base-nova on Neutral; the gallery starts there, with the
other bases, accents, styles, radii, menu colors and modes a choice away.

## Implemented defaults

| Area | Default behavior and appearance |
| --- | --- |
| Theme | Neutral light/dark semantic palette with chart colors, 14px body type, 16px medium headings, a 10px `--radius` with 6/8/14/18px steps, a translucent `Input` (clear in light, input/30 in dark), `shadow-lg` overlays. Radius choices 0, 7.2, 10 and 14; styles Nova, Vega, Maia, Lyra, Mira, Luma and Rhea. |
| [Button](https://ui.shadcn.com/docs/components/base/button) | 24/28/32/36px text and icon variants; 32px default with 10px horizontal padding; leading and trailing icons at the size's icon size, narrowing the padding on their side; no default shadow; primary hover at 80%; muted outline/ghost hover; subtle destructive surface with red text; expanded trigger styling; 1px pressed translation for non-popup actions; focus ring easing over 150ms; a link's underline 4px under its baseline; default arrow cursor; `.Invalid(v)` gives any variant a destructive border and 3px halo, and a toggle the border of its outline variant and a destructive focus ring. |
| [Button Group](https://ui.shadcn.com/docs/components/base/button-group) / [Toggle Group](https://ui.shadcn.com/docs/components/base/toggle-group) | Joined children with squared inner corners and shared borders, buttons, text fields and selects alike, the focused child's ring on top; group text and separators. Toggle groups have no track, a 2-unit gap by default and join at `Spacing(0)`. |
| [Input](https://ui.shadcn.com/docs/components/base/input) / [Textarea](https://ui.shadcn.com/docs/components/base/textarea) | 32px input minimum; 64px multiline minimum with content growth; translucent input surface over whatever it sits on and disabled fill; shared focus/invalid halo easing over 150ms. Explicit line counts remain supported. Input groups are 32px, with rows above and below the editor; a single line keeps its 32px there and moves its text 4px away from the row, as `pt-3` and `pb-3` do. |
| [Card](https://ui.shadcn.com/docs/components/base/card) | 16px padding, foreground/10 outline, 14px default radius, no default shadow; header, content and a muted footer band as sections, and a small size. |
| [Badge](https://ui.shadcn.com/docs/components/base/badge) | Primary default, 20px height, 12px text, 8px horizontal padding, subtle destructive surface; ghost and link variants and icons. `.Secondary()` selects the former default. |
| [Switch](https://ui.shadcn.com/docs/components/base/switch) | 32 × 18.4px track and 16px thumb, three quarters of that with `.Small()`; dark unchecked track and thumb colors follow the reference. |
| [Checkbox](https://ui.shadcn.com/docs/components/base/checkbox) / [Radio](https://ui.shadcn.com/docs/components/base/radio-group) | 16px glyph; checkbox 4px radius and 14px check; selected radio fills primary with an 8px contrasting dot; checked fills ease over 150ms. A radio group stacks its options 8px apart and is one Tab stop, its choice moved by the arrow keys. All take an invalid state. Like a switch, a slider and a select with its options, they keep the default arrow cursor. A checkbox, a radio and a switch take the pointer 12px to either side of the glyph and 8px above and below it, and a slider's thumb 8px around it, at no cost in layout (`Interactive.HitReach`). |
| [Avatar](https://ui.shadcn.com/docs/components/base/avatar) | 32px default, 24 small and 40 large; groups overlap with a background ring and count the rest. |
| [Spinner](https://ui.shadcn.com/docs/components/base/spinner) / [Empty](https://ui.shadcn.com/docs/components/base/empty) | The spinner is the theme's 16px icon size in the text color around it. An empty state has no border until `.Outline()`. |
| [Sidebar](https://ui.shadcn.com/docs/components/base/sidebar) | 256px wide, a 48px icon rail, 32px rows with icons, badges, actions and folding sub-menus; animated collapse; scrolling groups between a fixed header and footer. |
| [Toast](https://ui.shadcn.com/docs/components/base/toast) | Stacked notices that spread on hover, typed icons, six positions, 500ms motion; no countdown bar unless `.Countdown()`. |
| [Slider](https://ui.shadcn.com/docs/components/base/slider) | 4px track, 12px white thumb with ring-colored outline, 3px hover/focus halo; Home/End, PageUp/PageDown and Shift-arrow larger steps. |
| [Progress](https://ui.shadcn.com/docs/components/base/progress) | 4px muted track; a square-ended primary indicator that the track's round ends cut, running from the reading edge, animates while accessibility reports the actual value immediately. |
| [Accordion](https://ui.shadcn.com/docs/components/base/accordion) | 10px vertical header padding inside the one-pixel border focus colors, so a closed section is 42px above its one-pixel rule; 10px content bottom padding, independent enabled header Tab stops, 200ms disclosure, up/down icon substitution. Existing arrow navigation remains available. |
| [Tabs](https://ui.shadcn.com/docs/components/base/tabs) | Compact 32px list geometry; background-colored active segment with dark input surface and border, fading in where it stands over 150ms while the one left fades out, as a line strip's bar does. |
| [Dialog](https://ui.shadcn.com/docs/components/base/dialog) | 384px maximum default width, 16px outer margin/padding, 100ms fade/95% scale, black/10 scrim with 4px backdrop blur, default close button. |
| [Sheet](https://ui.shadcn.com/docs/components/base/sheet) | Right-side default, 75% viewport width capped at 384px, 200ms ease-in-out, fade and 40px entrance translation, default close button, description and footer. Explicit `.Size()` overrides remain. |
| [Drawer](https://ui.shadcn.com/docs/components/base/drawer) | As tall as its content, rounded top corners, 450ms motion; the handle bar is opt-in with `.Handle(true)`, and the drag works without it. No close button until `.CloseButton(true)`. |
| [Tooltip](https://ui.shadcn.com/docs/components/base/tooltip) | No default opening delay, top/center placement with 4px gap and collision fallback, any side on request, 320px max width, 8px radius, 12/6px horizontal/vertical padding; key hints as key caps. |
| [Hover Card](https://ui.shadcn.com/docs/components/base/hover-card) | 256px width, 10px radius/padding, 600ms opening delay, 300ms closing grace period, 100ms transition. |
| [Alert](https://ui.shadcn.com/docs/components/base/alert) | 10/8px horizontal/vertical padding, 2px row gap, medium title, optional leading icon and top-right action; destructive changes text while retaining the neutral border. |
| [Skeleton](https://ui.shadcn.com/docs/components/base/skeleton) | 8px corners, muted fill, existing 2-second opacity pulse. |
| Floating menus and shared controls | 4px popup padding, 6/4px item padding, foreground/10 popup outline, 100ms popup duration entering 8px from the anchor's side, soft 3px focus halos; labels, icons, right-hand checks and submenus beside their row; menus drawn in the theme's menu colors. A menu starts on its first row when the keyboard opened it and on none when the pointer did, and typing moves to the row that starts with what was typed; its rows keep the arrow cursor. A select list with more options than fit shows a chevron over the edge there is more beyond, which scrolls a row every 40ms while the pointer moved onto it stays; the keyboard's row is shown clear of it. |
| [Resizable](https://ui.shadcn.com/docs/components/base/resizable) / [Navigation Menu](https://ui.shadcn.com/docs/components/base/navigation-menu) | A hairline divider that takes no room. Navigation triggers are 36px with a rotating chevron and open on a 200ms hover. |

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
own closing UI. AlertDialog, CommandDialog and Drawer omit this button by default.
HoverCard exposes `.CloseDelay(...)`; Tooltip's existing `.Delay(...)` can
restore delayed opening. Accordion's group is a semantic container; its
individual headers now own focus and keep identity across keyed rebuilds.

Checks include exact control sizes in default/preset light and dark themes,
text scaling, independent accordion tab stops, close actions, retained pointer
identity, overlay keyboard flow, reduced motion and progress semantics. Visual
captures use the actual gallery in both themes, Nova/Rhea, and 320/640px widths.
The parity round's previews render the same way:

```sh
go run ./examples/gallery -render-dir /tmp/ggui-parity \
  -render-examples 'Theme presets,Buttons,Groups and addons,Card,Form layout,Validation states,Dropdown menu,Input group,Tooltip,Alert dialog,Sidebar,Toast,Table,Date picker,Calendar modes'
```

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
The parity round after it brings the parts shadcn composes its components from:
icon buttons, joined button groups, field sets, card sections, menu labels and
submenus, typed toasts, a sidebar with an icon rail, multi-month calendars and a
date-range picker, and the theme builder's radius, menu and mode choices.

The default Tabs keyboard behavior is now manual activation: arrows move focus,
Space or Enter changes the panel. `.ActivateOnFocus(true)` restores automatic
activation. Each tab and multiple-toggle item is a real accessibility focus
target, with one roving Tab stop per group. Slider step defaults are now 1 and
10 for normal/large movement; `.Step(0)` retains continuous pointer adjustment.

This is a native adaptation, not React/DOM API compatibility. Native font metrics,
text shaping and color interpolation can differ from CSS. The implementation
audit separately records unsupported advanced browser behaviors rather than
claiming pixel-for-pixel or prop-for-prop equivalence.
