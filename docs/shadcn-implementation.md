# shadcn/ui component coverage

[Documentation](README.md) · [shadcn/ui defaults](shadcn-comparison.md)

Reference: [current Base UI component catalog](https://ui.shadcn.com/docs/components),
inspected 2026-11-13. This reference maps the catalog to native GGUI APIs and
records remaining boundaries; it is not a claim of React prop-for-prop
compatibility.

## Complete catalog mapping

The live catalog has 66 entries. Each entry maps to a native widget or
composition in `ui` (or the named subpackage), rather than promising every
optional web prop.

| shadcn/ui component | GGUI API | Coverage |
| --- | --- | --- |
| [Accordion](https://ui.shadcn.com/docs/components/base/accordion) | `Accordion` | Native |
| [Alert](https://ui.shadcn.com/docs/components/base/alert) | `Alert` | Native; supports leading icon and top-right action |
| [Alert Dialog](https://ui.shadcn.com/docs/components/base/alert-dialog) | `AlertDialog` | Native; supports `Small` and `Media` |
| [Aspect Ratio](https://ui.shadcn.com/docs/components/base/aspect-ratio) | `AspectRatio` | Native |
| [Attachment](https://ui.shadcn.com/docs/components/base/attachment) | `chat.Attachment / chat.AttachmentGroup` (`ui/chat`) | Native |
| [Avatar](https://ui.shadcn.com/docs/components/base/avatar) | `Avatar / AvatarGroup` | Native; sizes, badges and groups |
| [Badge](https://ui.shadcn.com/docs/components/base/badge) | `Badge` | Native; ghost/link variants, icons and `OnTap` |
| [Breadcrumb](https://ui.shadcn.com/docs/components/base/breadcrumb) | `Breadcrumb / Crumb` | Native; chevron separator and `SeparatorOf` |
| [Bubble](https://ui.shadcn.com/docs/components/base/bubble) | `chat.Bubble` (`ui/chat`) | Native |
| [Button](https://ui.shadcn.com/docs/components/base/button) | `Button / ButtonOf` | Native; leading and trailing icons |
| [Button Group](https://ui.shadcn.com/docs/components/base/button-group) | `ButtonGroup / ButtonGroupText / ButtonGroupSeparator` | Native; joined children, text and separators |
| [Calendar](https://ui.shadcn.com/docs/components/base/calendar) | `Calendar / CalendarRange / CalendarMultiple` | Native; selection modes, `Months(n)` and caption dropdowns |
| [Card](https://ui.shadcn.com/docs/components/base/card) | `Card / CardMedia / CardHeader / CardContent / CardFooter` | Native; sections, `CardMedia`, small size and `OnTap` |
| [Carousel](https://ui.shadcn.com/docs/components/base/carousel) | `Carousel` | Native |
| [Chart](https://ui.shadcn.com/docs/components/base/chart) | `chart.Area / chart.Bar / chart.Line / chart.Pie / chart.Radar / chart.Radial` (`ui/chart`) | Native |
| [Checkbox](https://ui.shadcn.com/docs/components/base/checkbox) | `Checkbox` | Native |
| [Code Block](https://ui.shadcn.com/docs/components/base/code-block) | `CodeBlock / CodeBlockOf` | Native |
| [Collapsible](https://ui.shadcn.com/docs/components/base/collapsible) | `Collapsible / CollapsibleOf` | Native; supports custom trigger |
| [Combobox](https://ui.shadcn.com/docs/components/base/combobox) | `Combobox / ComboboxMultiple` | Native; multiple selection, editable input trigger and clear button |
| [Command](https://ui.shadcn.com/docs/components/base/command) | `Command / CommandDialog` | Native; groups, separators and icons |
| [Context Menu](https://ui.shadcn.com/docs/components/base/context-menu) | `ContextMenu / NativeContextMenu` | Native |
| [Data Table](https://ui.shadcn.com/docs/components/base/data-table) | `DataTable` | Native |
| [Date Picker](https://ui.shadcn.com/docs/components/base/date-picker) | `DatePicker / DateRangePicker` | Native; single date and date ranges |
| [Dialog](https://ui.shadcn.com/docs/components/base/dialog) | `Dialog` | Native; title, description and footer |
| [Direction](https://ui.shadcn.com/docs/components/base/direction) | `ggui.Directionality` | Native; honored by core layout/text and UI controls |
| [Drawer](https://ui.shadcn.com/docs/components/base/drawer) | `Drawer` | Native; handle drag, snap points and opt-in handle bar |
| [Dropdown Menu](https://ui.shadcn.com/docs/components/base/dropdown-menu) | `Menu / MenuLabel / MenuCheckbox / MenuRadioGroup / MenuSub` | Native; labels, icons, check/radio rows, destructive rows and submenus |
| [Empty](https://ui.shadcn.com/docs/components/base/empty) | `Empty` | Native; media icon and outline |
| [Field](https://ui.shadcn.com/docs/components/base/field) | `Field / FieldGroup / FieldSet / FieldSeparator / FieldCard / FieldContent / FieldTitle / FieldDescription` | Native; associated label, horizontal layout, field sets, choice cards and error lists |
| [Hover Card](https://ui.shadcn.com/docs/components/base/hover-card) | `HoverCard` | Native; side, alignment and delays |
| [Input](https://ui.shadcn.com/docs/components/base/input) | `TextField` | Native |
| [Input Group](https://ui.shadcn.com/docs/components/base/input-group) | `InputGroup` | Native; leading/trailing/top/bottom addons and multiline |
| [Input OTP](https://ui.shadcn.com/docs/components/base/input-otp) | `InputOTP` | Native |
| [Item](https://ui.shadcn.com/docs/components/base/item) | `Item / ItemGroup / ItemSeparator` | Native; media icons, header/footer, sizes, `OnTap` and groups |
| [Kbd](https://ui.shadcn.com/docs/components/base/kbd) | `Kbd / KbdGroup` | Native; key caps and groups |
| [Label](https://ui.shadcn.com/docs/components/base/label) | `Label` | Native |
| [Marker](https://ui.shadcn.com/docs/components/base/marker) | `chat.Marker` (`ui/chat`) | Native |
| [Menubar](https://ui.shadcn.com/docs/components/base/menubar) | `Menubar / AppMenubar` | Native |
| [Message](https://ui.shadcn.com/docs/components/base/message) | `chat.Message` (`ui/chat`) | Native |
| [Message Scroller](https://ui.shadcn.com/docs/components/base/message-scroller) | `chat.MessageScroller` (`ui/chat`) | Native |
| [Native Select](https://ui.shadcn.com/docs/components/base/native-select) | `NativeSelect` | Native; platform menu with themed fallback |
| [Navigation Menu](https://ui.shadcn.com/docs/components/base/navigation-menu) | `NavigationMenu` | Native; links, dropdowns and hover-open |
| [Pagination](https://ui.shadcn.com/docs/components/base/pagination) | `Pagination` | Native |
| [Popover](https://ui.shadcn.com/docs/components/base/popover) | `Popover / PopoverOf` | Native; sides, alignment and controlled open state |
| [Progress](https://ui.shadcn.com/docs/components/base/progress) | `Progress / ProgressRing` | Native; label, value and ring variants |
| [Questionnaire](https://ui.shadcn.com/docs/components/base/questionnaire) | `chat.Questionnaire` (`ui/chat`) | Native |
| [Radio Group](https://ui.shadcn.com/docs/components/base/radio-group) | `Radios / Radio` | Native; vertical default, horizontal option and invalid state |
| [Resizable](https://ui.shadcn.com/docs/components/base/resizable) | `Resizable` | Native |
| [Scroll Area](https://ui.shadcn.com/docs/components/base/scroll-area) | `widget.Scroll` | Core widget |
| [Select](https://ui.shadcn.com/docs/components/base/select) | `Select` | Native; selected-item alignment, placeholder, groups and disabled options |
| [Separator](https://ui.shadcn.com/docs/components/base/separator) | `Divider` | Native |
| [Sheet](https://ui.shadcn.com/docs/components/base/sheet) | `Sheet` | Native; edge placement, description and footer |
| [Sidebar](https://ui.shadcn.com/docs/components/base/sidebar) | `Sidebar / SidebarTrigger / SidebarInset` | Native; icons, badges, actions, sub-menus, icon rail, trigger, rail and variants |
| [Skeleton](https://ui.shadcn.com/docs/components/base/skeleton) | `Skeleton` | Native |
| [Slider](https://ui.shadcn.com/docs/components/base/slider) | `Slider / RangeSlider` | Native; single and multi-thumb ranges |
| [Spinner](https://ui.shadcn.com/docs/components/base/spinner) | `Spinner` | Native |
| [Switch](https://ui.shadcn.com/docs/components/base/switch) | `Switch` | Native; small size and invalid state |
| [Table](https://ui.shadcn.com/docs/components/base/table) | `Table` | Native; selection, footer and caption |
| [Tabs](https://ui.shadcn.com/docs/components/base/tabs) | `Tabs` | Native; vertical/horizontal, manual/auto activation, disabled tabs and RTL |
| [Textarea](https://ui.shadcn.com/docs/components/base/textarea) | `TextField(...).Multiline()` | Native |
| [Toast](https://ui.shadcn.com/docs/components/base/toast) | `NewToaster / Toast` | Native; typed notices, loading updates and six positions |
| [Toggle](https://ui.shadcn.com/docs/components/base/toggle) | `Toggle / ToggleOf` | Native; label and custom/icon content |
| [Toggle Group](https://ui.shadcn.com/docs/components/base/toggle-group) | `ToggleGroup / ToggleGroupMultiple` | Native; single/multiple selection, outline and joined spacing |
| [Tooltip](https://ui.shadcn.com/docs/components/base/tooltip) | `Tooltip` | Native; arrow, sides and shortcut hints |
| [Trace](https://ui.shadcn.com/docs/components/base/trace) | `chat.Trace` (`ui/chat`) | Native |
| [Typography](https://ui.shadcn.com/docs/components/base/typography) | `widget.Text / RichText, Title / Heading / Caption / Mono` | Core and UI composition |

## Native APIs implemented

| Area | Public API and behavior |
| --- | --- |
| Anchored content | `Popover` / `PopoverOf`, controlled `BindOpen`, `Trigger`, `Title` / `Description`, logical `Align`, 288px default panel, outside/Escape dismissal and focus restoration, including nested and text-only panels. Nonmodal by default; `Modal(true)` opts into trapping. |
| Navigation | `NavigationMenu`, `NavigationMenuItem`, `NavigationMenuLink` / `NavigationMenuLinkOf`; real link semantics, active destinations, shared rich panel, keyboard navigation and RTL order. |
| Tooltip | Matching arrow with collision-side placement; `Arrow(false)` opt-out, Escape dismissal, `Disabled` / `BindDisabled` independent from the trigger. |
| Select | Selected-item alignment by default; `AlignItemWithTrigger(false)` restores edge placement. Arrows open the closed list, Home/End reach its ends and typed letters search the options. `NativeSelect` / `Native(true)` attempts a platform menu and falls back to the themed popup. |
| Sliders | `RangeSlider` accepts any number of values with independent focus/accessible ranges, `MinGap`, named thumbs and copied callbacks; scalar/range sliders support `Vertical`, RTL, `Step`, `LargeStep` and commits. |
| Drawers | Bottom-handle drag, positional snap heights, controlled `BindSnapPoint`, `OnSnapChange`, dismissal and keyboard/accessibility resizing. |
| Code | `CodeBlock` / `CodeBlockOf`, title/language, exact copy/selection, line numbers, `StartLine`, `Wrap`, `LineStyles`, bounded keyboard scrolling and LTR code inside RTL interfaces. |
| Trace | `chat.Trace` (`ui/chat`): hierarchical keyed `chat.TraceStep` rows with an icon, status, description, duration, details, children and leaf actions; controlled steps/expansion and retained focus across reorders. |
| Buttons/forms | `Toggle` binds pressed state; `ToggleGroupMultiple` binds selected values; `Label.For` focuses or activates its associated control. `Field` uses associated labels. |
| Tabs | `Vertical`, `Tab.Disabled`, manual activation default and `ActivateOnFocus(true)` opt-in; independent accessible targets with a roving Tab stop. |
| Combobox | An editable input as the trigger that filters as it is typed in, with focus kept there; `ClearButton`, `Empty`; `ComboboxMultiple` with its search inline after removable chips, checked rows, keyboard toggles/backspace, live options and copied selection callbacks. |
| Calendars | `CalendarRange` / `CalendarMultiple`, inclusive civil dates, required/count/night limits, disabled-interior policy, controlled rejection, copied/sorted selection callbacks. |
| Direction | Tabs, button/toggle groups, toggle glyph/label placement, sliders, calendar, pagination icons, sidebar labels/border and resizable panes honor `Directionality`. Code content deliberately stays LTR. |
| Buttons and groups | `Button.Icon` / `TrailingIcon`; `ToggleOf`; `ButtonGroup` joins its children with squared inner corners and shared borders and takes `ButtonGroupText` / `ButtonGroupTextOf` and `ButtonGroupSeparator`, while text fields and input groups in it take the spare width and a `Select` joins as a button does; `ToggleGroup.Outline` / `Size` / `Spacing(0)`; `KbdGroup`. |
| Fields | `Checkbox` / `Radio` / `Radios` / `Switch` / `Select` / `Button` / `Toggle` / `InputOTP` `.Invalid(v)` (all except `Select` also support `.BindInvalid(reader)`); `Switch.Small`; `Field.Horizontal` / `ControlFirst` / `Card` / `Errors` / `BindErrors`; `FieldCard(control, content).ControlFirst`, `FieldContent`, `FieldTitle`, `FieldDescription`; `FieldGroup`, `FieldSet(...).Legend / Description`, `FieldSeparator().Label`; `InputGroup.Top` / `Bottom` / `Multiline`; `Select.Placeholder` / `GroupBy` / `DisabledOption` / `Small`. |
| Overlays | `PopupWidget.Side` / `AlignOffset` / `MatchAnchorWidth` / `FocusOnOpen`; `Dialog.Description` / `Footer` / `TopAt`; `Sheet.Description` / `Footer` / `Handle`; `AlertDialog.Small` / `Media`; `Popover`, `HoverCard` and `Tooltip` `.Side` / `Align` / `AlignOffset` over `widget.PopupSide`. |
| Menus | `MenuLabel`, `MenuSub`, `MenuCheckbox`, `MenuRadioGroup`; `MenuItem.Icon` / `Inset` / `Destructive`; `CommandSeparator`, `CommandEntry.Icon`; menus draw in `Theme.Menu()`. |
| Cards and feedback | `Card` sections (`CardMedia`, `CardHeader(...).Action`, `CardContent`, `CardFooter(...).Justify`), `Card.Small` / `OnTap`; toast types, `ToastMessage.Loading` / `Countdown`, `Toaster.Position` / `Update`; `AvatarGroup`, `Avatar.Small` / `Large` / `Badge` / `BadgeIcon`; `ItemGroup`, `ItemSeparator`, `Item.MediaIcon` / `Header` / `Footer` / `Muted` / `Small` / `OnTap`; `Empty.MediaIcon` / `Outline`; `Alert.Icon`; `Badge.Ghost` / `Link` / `Icon` / `OnTap`; `Progress.Label` / `ShowValue` / `FormatValue`; `Spinner.Color`. |
| Data | `Calendar.Months(n)`, `CaptionDropdown`, `DropdownYears`, `MonthNames`; `DateRangePicker` with `FormatRange` / `OnRangeChange`; `Table.Footer` / `Caption`. |
| Sidebar | `SidebarEntry.Icon` / `Badge` / `Action` / `Sub` / `Collapsible` / `Tooltip` and the rest; `SidebarSeparator`, `SidebarSkeleton`, `SidebarCustom`; `Sidebar.Collapsible(SidebarCollapseIcon)`, `Variant`, `Side`, `Rail`, `Shortcut`, `HeaderItems` / `FooterItems`; `SidebarTrigger`, `SidebarInset`. |
| Theme | `theme.Config` with `Radius`, `MenuAccent`, `MenuColor`, `Chart`, fonts and `Icons`; `theme.Mode`, `BindMode` and `ui.ThemeMode`; the Vega, Maia, Lyra, Mira and Luma styles beside Nova and Rhea. |

Examples live in the gallery: **Popover**, **Navigation menu**, **Code block**,
**Trace**, **Range slider**, **Toggle and label**, **Native select**,
**Drawer snapping**, **RTL controls**, **Tabs modes**, **Calendar modes**,
**Multiple combobox**, **Buttons**, **Groups and addons**, **Card**,
**Form layout**, **Validation states**, **Dropdown menu**, **Context menu**,
**Input group**, **Tooltip**, **Dialog**, **Alert dialog**,
**Sheets and drawers**, **Notices and empty states**, **Loading and shortcuts**,
**Progress**, **Toast**, **Profile and media**, **Table**, **Calendar**,
**Date picker**, **Sidebar**, and **Theme presets**.
See [forms](forms.md), [navigation](data-and-navigation.md), and
[feedback](feedback-and-composition.md) for composition examples.

## Key behavioral conventions

- `Tabs` arrows move focus without activating by default; `.ActivateOnFocus(true)`
  opts into automatic activation on focus.
- `Slider` and `RangeSlider` default to `Step(1)` and `LargeStep(10)`. Fractional
  ranges should set an explicit fractional step; `Step(0)` enables continuous
  pointer movement.
- `Select` opens with its selected row aligned over the trigger where the
  viewport allows; `.AlignItemWithTrigger(false)` uses edge placement instead.
  Up and Down on a closed `Select` open its popup on the chosen option, Home and
  End move through the open list, and typed letters select matching options.
- `Field` labels focus or activate their associated controls without adding a
  Tab stop.
- `Radios` stacks options vertically, 8 logical pixels apart, as shadcn's
  RadioGroup does (`.Horizontal()` lays them out in a row). The group is one
  roving Tab stop on the chosen option, navigated with the arrow keys; a
  standalone `Radio` is its own Tab stop.
- `Drawer` sizes to its content (up to 96px short of the window) with rounded
  top corners and `MotionDrawer` (450ms) animation. Dragging works by default;
  `.Handle(true)` draws the handle bar and `.CloseButton(true)` adds a close
  button.
- `Avatar` defaults to 32px, `Spinner` uses the theme's 16px `IconSize` in the
  inherited text color, and `Empty` has no border until `.Outline()`.
- Toasts stack with 500ms motion and show no countdown bar unless `.Countdown()`
  is set.
- `Sidebar` defaults to 256px wide.
- `Resizable` draws a zero-width hairline divider (`.WithHandle()` adds a grip).
- `NavigationMenu` opens dropdowns after a 50ms hover (with a 50ms close
  linger); `.OpenOnHover(false)` restricts opening to explicit activation.
- `Menu`, `ContextMenu`, and `Menubar` menus opened with the pointer start with
  no row highlighted, so Enter runs nothing until keyboard navigation selects a
  row; opened from the keyboard, all three start on their first enabled row.
- `Command` stops at the first and last match (`.Loop(true)` enables wrapping),
  leads its search field with its icon, and ranks fuzzy/abbreviation matches
  ahead of source order.
- `Theme.Heading` defaults to 16px medium, and `Theme.Input` is translucent
  (clear in light Nova, `--input` at 30% in dark Nova), painted over whatever
  surface sits behind a field.
- `Combobox` uses an editable input directly as its trigger and keeps focus in
  the input while filtering the open list.
- `ToggleGroup` segments draw without a background track; `ButtonGroup` joins
  its children with squared inner corners and shared borders (best paired with
  outline buttons).

## Remaining boundaries

This reference covers catalog components and the major documented interaction
modes; it is not an exhaustive copy of every Base UI/React prop or example
composition.

- Drawer gesture support is for the bottom handle with logical-pixel snap heights.
  Velocity/sequential snaps, whole-panel scroll handoff, nested drawer scaling,
  viewport-fraction snap syntax and mobile virtual-keyboard providers remain absent.
- Week-number columns and non-Gregorian calendars are not supplied; a date
  picker's text cannot be typed.
- Code renders plain source without a syntax tokenizer. A caller can compose a
  wrap/show-more toolbar using the public APIs; no built-in toolbar preset is added.
- Navigation Menu destinations use caller callbacks; browser routing is not
  supplied. Disabled entries are skipped during keyboard traversal. Links stay
  open unless `CloseOnClick(true)` is set.
- Menus draw opaque: shadcn's translucent, blurred menu color is not offered.
  The bundled `ui/icons/{lucide,tabler,heroicons}` packages hold the 23 semantic
  control roles; for the complete libraries, use the standalone tree-shaken
  `icons/{lucide,tabler,heroicons}` modules (see [Icons](fonts.md#icons)).
- RTL support applies to the controls named above and existing core layout/text.
  It is not a guarantee that every custom widget, chart or table mirrors itself.
- Native Select's OS menu is platform-dependent; headless probes verify the themed
  fallback. Clipboard writes have no success/error result in the current native
  API, so copy feedback reports invocation rather than OS delivery confirmation.
- DOM form submission, browser-only responsive utilities and every upstream ARIA
  attribute are outside GGUI's native API. Font metrics and rendering can differ.

## Visual verification

Automated tests (`go test ./...`) cover nested and text-only popup dismissal,
focus return, roving keyboard focus, disabled and manual tabs, rejected
controlled updates, live combobox options, range constraints, calendar
selection, and drawer snapping.

To render gallery previews offscreen with the GPU renderer across Nova and Rhea
styles, light and dark themes, and 320/640px widths, run from the repository
root:

```sh
go run ./docs/examples/gallery -render-dir /tmp/ggui-gallery-renders \
  -render-examples 'Popover,Navigation menu,Code block,Trace,Range slider,Toggle and label,Native select,Drawer snapping,RTL controls,Tabs modes,Calendar modes,Multiple combobox'

go run ./docs/examples/gallery -render-dir /tmp/ggui-gallery-renders \
  -render-examples 'Theme presets,Buttons,Groups and addons,Card,Form layout,Validation states,Dropdown menu,Context menu,Input group,Tooltip,Alert dialog,Dialog,Sheets and drawers,Notices and empty states,Loading and shortcuts,Progress,Toast,Profile and media,Table,Calendar,Date picker,Calendar modes,Popover,Drawer snapping,Sidebar'
```

Each example is rendered at rest, opened by the control listed in `actionNames`
in `docs/examples/gallery/render.go` (or hovered, for `hoverNames`), settled,
and dismissed with Escape.
