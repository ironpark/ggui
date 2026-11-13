# shadcn/ui implementation audit

Reference: [current Base UI component catalog](https://ui.shadcn.com/docs/components),
inspected 2026-11-13. This inventory records the gaps after the default-style
migration; it is not a claim of React prop-for-prop compatibility.

## Gaps identified before implementation

| Area | Missing native behavior | Implementation owner |
| --- | --- | --- |
| Popover | Themed public anchored surface, controlled open state, dismissal and focus lifecycle | Overlay agent |
| Navigation Menu | Navigation links and expandable content with keyboard navigation | Overlay agent |
| Tooltip | Direction-aware arrow attached to the floating surface | Overlay agent |
| Select | Position the selected option over the trigger by default | Overlay agent |
| Slider | Multiple independently focusable thumbs, constrained range, minimum separation, RTL direction | Range/Drawer agent |
| Drawer | Drag gesture, dismissal threshold and snap points | Range/Drawer agent |
| Code Block | Code presentation, language/header, line numbers and clipboard action | Code/Trace agent |
| Trace | Expandable execution steps, statuses, durations and details | Code/Trace agent |
| Direction | Mirrored layout and horizontal keyboard movement in hand-painted controls | Integration agent |
| Toggle / Toggle Group | Standalone pressed button and multiple-selection binding | Integration agent |
| Label | Standalone associated label with focus/activation | Integration agent |
| Native Select | Platform menu invocation with a themed fallback | Overlay agent |
| Tabs | Vertical list, manual activation default, per-page disabled state | Integration agent |
| Calendar | Date range and multiple-date selection | Range/Drawer agent |
| Combobox | Multiple selection with removable chips | Overlay agent |

## Complete catalog mapping

The live catalog has 66 entries. “Existing” means a native implementation or an
explicit composition already exists, not that every optional web prop is covered.
“Added” identifies a new API in this implementation round.

| shadcn/ui component | GGUI API | Coverage |
| --- | --- | --- |
| [Accordion](https://ui.shadcn.com/docs/components/base/accordion) | `Accordion` | Existing |
| [Alert](https://ui.shadcn.com/docs/components/base/alert) | `Alert` | Existing |
| [Alert Dialog](https://ui.shadcn.com/docs/components/base/alert-dialog) | `AlertDialog` | Existing |
| [Aspect Ratio](https://ui.shadcn.com/docs/components/base/aspect-ratio) | `AspectRatio` | Existing |
| [Attachment](https://ui.shadcn.com/docs/components/base/attachment) | `Attachment / AttachmentGroup` | Existing |
| [Avatar](https://ui.shadcn.com/docs/components/base/avatar) | `Avatar` | Existing |
| [Badge](https://ui.shadcn.com/docs/components/base/badge) | `Badge` | Existing |
| [Breadcrumb](https://ui.shadcn.com/docs/components/base/breadcrumb) | `Breadcrumb / Crumb` | Existing |
| [Bubble](https://ui.shadcn.com/docs/components/base/bubble) | `Bubble` | Existing |
| [Button](https://ui.shadcn.com/docs/components/base/button) | `Button / ButtonOf` | Existing |
| [Button Group](https://ui.shadcn.com/docs/components/base/button-group) | `ButtonGroup` | Existing |
| [Calendar](https://ui.shadcn.com/docs/components/base/calendar) | `Calendar / CalendarRange / CalendarMultiple` | Existing; selection modes added |
| [Card](https://ui.shadcn.com/docs/components/base/card) | `Card` | Existing |
| [Carousel](https://ui.shadcn.com/docs/components/base/carousel) | `Carousel` | Existing |
| [Chart](https://ui.shadcn.com/docs/components/base/chart) | `AreaChart / BarChart / LineChart / PieChart / RadarChart / RadialChart` | Existing |
| [Checkbox](https://ui.shadcn.com/docs/components/base/checkbox) | `Checkbox` | Existing |
| [Code Block](https://ui.shadcn.com/docs/components/base/code-block) | `CodeBlock / CodeBlockOf` | Added |
| [Collapsible](https://ui.shadcn.com/docs/components/base/collapsible) | `Collapsible` | Existing |
| [Combobox](https://ui.shadcn.com/docs/components/base/combobox) | `Combobox / ComboboxMultiple` | Existing; multiple selection added |
| [Command](https://ui.shadcn.com/docs/components/base/command) | `Command / CommandDialog` | Existing |
| [Context Menu](https://ui.shadcn.com/docs/components/base/context-menu) | `ContextMenu / NativeContextMenu` | Existing |
| [Data Table](https://ui.shadcn.com/docs/components/base/data-table) | `DataTable` | Existing |
| [Date Picker](https://ui.shadcn.com/docs/components/base/date-picker) | `DatePicker` | Existing |
| [Dialog](https://ui.shadcn.com/docs/components/base/dialog) | `Dialog` | Existing |
| [Direction](https://ui.shadcn.com/docs/components/base/direction) | `ggui.Directionality` | Existing; controls expanded |
| [Drawer](https://ui.shadcn.com/docs/components/base/drawer) | `Drawer` | Existing; drag/snap added |
| [Dropdown Menu](https://ui.shadcn.com/docs/components/base/dropdown-menu) | `Menu` | Existing |
| [Empty](https://ui.shadcn.com/docs/components/base/empty) | `Empty` | Existing |
| [Field](https://ui.shadcn.com/docs/components/base/field) | `Field` | Existing; associated label added |
| [Hover Card](https://ui.shadcn.com/docs/components/base/hover-card) | `HoverCard` | Existing |
| [Input](https://ui.shadcn.com/docs/components/base/input) | `TextField` | Existing |
| [Input Group](https://ui.shadcn.com/docs/components/base/input-group) | `InputGroup` | Existing |
| [Input OTP](https://ui.shadcn.com/docs/components/base/input-otp) | `InputOTP` | Existing |
| [Item](https://ui.shadcn.com/docs/components/base/item) | `Item` | Existing |
| [Kbd](https://ui.shadcn.com/docs/components/base/kbd) | `Kbd` | Existing |
| [Label](https://ui.shadcn.com/docs/components/base/label) | `Label` | Added |
| [Marker](https://ui.shadcn.com/docs/components/base/marker) | `Marker` | Existing |
| [Menubar](https://ui.shadcn.com/docs/components/base/menubar) | `Menubar / AppMenubar` | Existing |
| [Message](https://ui.shadcn.com/docs/components/base/message) | `Message` | Existing |
| [Message Scroller](https://ui.shadcn.com/docs/components/base/message-scroller) | `MessageScroller` | Existing |
| [Native Select](https://ui.shadcn.com/docs/components/base/native-select) | `NativeSelect` | Added |
| [Navigation Menu](https://ui.shadcn.com/docs/components/base/navigation-menu) | `NavigationMenu` | Added |
| [Pagination](https://ui.shadcn.com/docs/components/base/pagination) | `Pagination` | Existing |
| [Popover](https://ui.shadcn.com/docs/components/base/popover) | `Popover / PopoverOf` | Added |
| [Progress](https://ui.shadcn.com/docs/components/base/progress) | `Progress / ProgressRing` | Existing |
| [Questionnaire](https://ui.shadcn.com/docs/components/base/questionnaire) | `Questionnaire` | Existing |
| [Radio Group](https://ui.shadcn.com/docs/components/base/radio-group) | `Radios / Radio` | Existing |
| [Resizable](https://ui.shadcn.com/docs/components/base/resizable) | `Resizable` | Existing |
| [Scroll Area](https://ui.shadcn.com/docs/components/base/scroll-area) | `ggui.Scroll` | Existing core widget |
| [Select](https://ui.shadcn.com/docs/components/base/select) | `Select` | Existing; alignment added |
| [Separator](https://ui.shadcn.com/docs/components/base/separator) | `Divider` | Existing |
| [Sheet](https://ui.shadcn.com/docs/components/base/sheet) | `Sheet` | Existing |
| [Sidebar](https://ui.shadcn.com/docs/components/base/sidebar) | `Sidebar` | Existing |
| [Skeleton](https://ui.shadcn.com/docs/components/base/skeleton) | `Skeleton` | Existing |
| [Slider](https://ui.shadcn.com/docs/components/base/slider) | `Slider / RangeSlider` | Existing; multiple thumbs added |
| [Spinner](https://ui.shadcn.com/docs/components/base/spinner) | `Spinner` | Existing |
| [Switch](https://ui.shadcn.com/docs/components/base/switch) | `Switch` | Existing |
| [Table](https://ui.shadcn.com/docs/components/base/table) | `Table` | Existing |
| [Tabs](https://ui.shadcn.com/docs/components/base/tabs) | `Tabs` | Existing; vertical/manual/disabled/RTL added |
| [Textarea](https://ui.shadcn.com/docs/components/base/textarea) | `TextField(...).Multiline()` | Existing |
| [Toast](https://ui.shadcn.com/docs/components/base/toast) | `NewToaster / Toast` | Existing |
| [Toggle](https://ui.shadcn.com/docs/components/base/toggle) | `Toggle` | Added |
| [Toggle Group](https://ui.shadcn.com/docs/components/base/toggle-group) | `ToggleGroup / ToggleGroupMultiple` | Existing; multiple selection added |
| [Tooltip](https://ui.shadcn.com/docs/components/base/tooltip) | `Tooltip` | Existing; arrow added |
| [Trace](https://ui.shadcn.com/docs/components/base/trace) | `Trace` | Added |
| [Typography](https://ui.shadcn.com/docs/components/base/typography) | `ggui.Text / RichText, Title / Heading / Caption / Mono` | Existing composition |

## Native APIs implemented

| Area | Public API and behavior |
| --- | --- |
| Anchored content | `Popover` / `PopoverOf`, controlled `BindOpen`, `Trigger`, `Title` / `Description`, logical `Align`, 288px default panel, outside/Escape dismissal and focus restoration, including nested and text-only panels. Nonmodal by default; `Modal(true)` opts into trapping. |
| Navigation | `NavigationMenu`, `NavigationMenuItem`, `NavigationMenuLink` / `NavigationMenuLinkOf`; real link semantics, active destinations, shared rich panel, keyboard navigation and RTL order. |
| Tooltip | Matching arrow with collision-side placement; `Arrow(false)` opt-out, Escape dismissal, `Disabled` / `BindDisabled` independent from the trigger. |
| Select | Selected-item alignment by default; `AlignItemWithTrigger(false)` restores edge placement. `NativeSelect` / `Native(true)` attempts a platform menu and falls back to the themed popup. |
| Sliders | `RangeSlider` accepts any number of values with independent focus/accessible ranges, `MinGap`, named thumbs and copied callbacks; scalar/range sliders support `Vertical`, RTL, `Step`, `LargeStep` and commits. |
| Drawers | Bottom-handle drag, positional snap heights, controlled `BindSnapPoint`, `OnSnapChange`, dismissal and keyboard/accessibility resizing. |
| Code | `CodeBlock` / `CodeBlockOf`, title/language, exact copy/selection, line numbers, `StartLine`, `Wrap`, `LineStyles`, bounded keyboard scrolling and LTR code inside RTL interfaces. |
| Trace | Hierarchical keyed `TraceStep` rows with status, description, duration, details, children and leaf actions; controlled steps/expansion and retained focus across reorders. |
| Buttons/forms | `Toggle` binds pressed state; `ToggleGroupMultiple` binds selected values; `Label.For` focuses or activates its associated control. `Field` uses associated labels. |
| Tabs | `Vertical`, `Tab.Disabled`, manual activation default and `ActivateOnFocus(true)` opt-in; independent accessible targets with a roving Tab stop. |
| Combobox | `ComboboxMultiple`, searchable checked rows, removable chips, keyboard toggles/backspace, live options and copied selection callbacks. |
| Calendars | `CalendarRange` / `CalendarMultiple`, inclusive civil dates, required/count/night limits, disabled-interior policy, controlled rejection, copied/sorted selection callbacks. |
| Direction | Tabs, button/toggle groups, toggle glyph/label placement, sliders, calendar, pagination icons, sidebar labels/border and resizable panes honor `Directionality`. Code content deliberately stays LTR. |

Examples live in the gallery: **Popover**, **Navigation menu**, **Code block**,
**Trace**, **Range slider**, **Toggle and label**, **Native select**,
**Drawer snapping**, **RTL controls**, **Tabs modes**, **Calendar modes**, and **Multiple combobox**.
See [forms](forms.md), [navigation](data-and-navigation.md), and
[feedback](feedback-and-composition.md) for composition examples.

## Compatibility changes

- Tabs arrows now move focus without activating by default. Use
  `.ActivateOnFocus(true)` for the previous behavior.
- Slider defaults are `Step(1)` and `LargeStep(10)`. Fractional ranges should set
  an explicit fractional step; `Step(0)` enables continuous pointer movement.
- Select opens with its selected row over the trigger where the viewport allows;
  opt out with `.AlignItemWithTrigger(false)`.
- Field labels now focus/activate their controls. Label interaction adds no Tab stop.

## Remaining boundaries

This audit covers catalog components and the major documented interaction modes;
it is not an exhaustive copy of every Base UI/React prop or example composition.

- Drawer gesture support is for the bottom handle with logical-pixel snap heights.
  Velocity/sequential snaps, whole-panel scroll handoff, nested drawer scaling,
  viewport-fraction snap syntax and mobile virtual-keyboard providers remain absent.
- Calendar uses one month at a time and DatePicker remains single-date. Month/year
  dropdowns, multi-month views, week-number columns and non-Gregorian calendars
  are not supplied by the new selection APIs.
- Code renders plain source without a syntax tokenizer. A caller can compose a
  wrap/show-more toolbar using the public APIs; no built-in toolbar preset is added.
- Navigation Menu destinations use caller callbacks and panels open by activation;
  automatic hover-open and browser routing are not supplied. Disabled entries are
  skipped during keyboard traversal. Links stay open unless `CloseOnClick(true)`
  is set.
- RTL support applies to the controls named above and existing core layout/text.
  It is not a guarantee that every custom widget, chart or table mirrors itself.
- Native Select's OS menu is platform-dependent; headless probes verify the themed
  fallback. Clipboard writes have no success/error result in the current native
  API, so copy feedback reports invocation rather than OS delivery confirmation.
- DOM form submission, browser-only responsive utilities and every upstream ARIA
  attribute are outside GGUI's native API. Font metrics and rendering can differ.

## Verification

Verified on 2026-11-13:

- `go test ./...` passed, including the gallery and workspace examples.
- `go vet ./...` and `git diff --check` passed.
- The 12 gallery examples above rendered 384 PNGs: Nova/Rhea, light/dark,
  320/640px widths, and initial/active/settled/exit states. Representative frames
  were visually inspected; Popover and Navigation Menu were rendered again after
  the final focus fixes. Local output: `/tmp/ggui-shadcn-implemented`.
- Regression coverage includes nested/text-only popup dismissal and focus return,
  roving keyboard focus, disabled/manual tabs, rejected controlled updates,
  live combobox options, range constraints, calendar selection and drawer snapping.

The render command is reproducible from the repository root:

```sh
go run ./examples/gallery -render-dir /tmp/ggui-shadcn-implemented -render-examples 'Popover,Navigation menu,Code block,Trace,Range slider,Toggle and label,Native select,Drawer snapping,RTL controls,Tabs modes,Calendar modes,Multiple combobox'
```
