# GGUI documentation

[Project README](../README.md) · [Runnable counter](../README.md#quick-start) · [Component gallery](../examples/gallery)

Build a Go UI by composing widgets, binding controls to state, and styling the
result with a theme. These guides explain the current repository API and require
Go 1.27 or newer.

## Start here

1. Run the [counter quick start](../README.md#quick-start).
2. Learn [app setup and configuration](getting-started.md).
3. Read [state and components](reactivity.md) before adding reactive behavior.
4. Choose [layouts](layout.md) and [form controls](forms.md) for your screen.
5. Add [headless interaction tests](testing.md).

Examples are focused snippets unless they contain a `package` declaration.
They use the `ggui`, `ui`, and `theme` (`github.com/ironpark/ggui/ui/theme`)
packages; application callbacks, data, and colors
such as `save`, `todos`, and `accent` are supplied by your app. Run shell commands
from the repository root unless a guide says otherwise.

> [!IMPORTANT]
> APIs are under active development. Native accessibility currently supports
> **macOS and Windows**; the Linux bridge is not implemented. Native file
> dialogs run on macOS, Windows, and desktops with `zenity` or `kdialog`
> on `PATH`; elsewhere they report `runtime.ErrUnsupported`.

## Find a topic

| I want to… | Read |
| --- | --- |
| Configure a window and app lifecycle | [Getting started](getting-started.md) |
| Understand signals, derived values, ownership, and cleanup | [State and components](reactivity.md) |
| Update the UI from background work | [Threads](reactivity.md#threads) · [Resources](reactivity.md#resources-and-await-blocks) |
| Render changing or large collections | [Keyed lists](reactivity.md#keyed-lists) · [Tables](data-and-navigation.md#tables) |
| Arrange text, images, and overlays | [Widgets and layout](layout.md) |
| Build forms and show validation | [Forms and controls](forms.md) · [Input OTP](carousel-and-otp.md#input-otp) |
| Add menus, dates, search, or navigation | [Data and navigation](data-and-navigation.md) |
| Show notices, dialogs, sheets, and loading states | [Feedback and composition](feedback-and-composition.md) |
| Build swipeable slides | [Carousel](carousel-and-otp.md#carousel) |
| Visualize data | [Charts](charts.md) |
| Build chat screens or questionnaires | [Chat components and questionnaires](chat.md) |
| Customize colors, spacing, shadows, and themes | [Styling and themes](styling.md) |
| Load fonts, render emoji, or change icon sets | [Fonts, emoji, and icons](fonts.md) |
| Animate values or entering/leaving content | [Animation](animation.md) |
| Handle pointer input, shortcuts, scrolling, and focus | [Input and focus](input.md) |
| Accept dropped files or open a native file dialog | [Drag and drop](input.md#drag-and-drop) · [File dialogs](input.md#native-file-dialogs) |
| Support assistive technology | [Accessibility](accessibility.md) |
| Test and inspect a UI | [Testing](testing.md) · [Widget inspector](inspector.md) |
| Write a custom widget or understand rendering | [Custom widgets and rendering](rendering.md) |
| Understand API conventions and runtime setters | [Public API conventions](api-conventions.md) |
| Build examples, run checks, or navigate the source | [Development](development.md) |

## Package and asset references

Package-specific READMEs remain beside their code and assets:

- [Icon sets, custom SVGs, precedence, and caching](../ui/icons/README.md)
- [Bundled Lucide icons](../ui/icons/lucide/README.md), [Tabler icons](../ui/icons/tabler/README.md), and [Heroicons](../ui/icons/heroicons/README.md)
- [Optional Noto Color Emoji font](../fonts/notoemoji/README.md)
- [Gallery chat assets and licenses](../examples/gallery/assets/chat/README.md)
- [Theme package and migration](../ui/theme/README.md)
- [Vendored theme palettes and sources](../internal/themedata/README.md)
