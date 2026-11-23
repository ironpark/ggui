# GGUI documentation

[Project README](../README.md) · [Runnable counter](../README.md#quick-start) · [Examples](examples/README.md) · [Component gallery](examples/gallery) · [Theme playground](examples/create)

Build a Go UI by composing widgets, binding controls to state, and styling the
result with a theme. These guides explain the current repository API and require
Go 1.27 or newer.

## Start here

1. Run the [counter quick start](../README.md#quick-start), or create an
   app with [`ggui init`](cli.md#ggui-init).
2. Learn [app setup and configuration](getting-started.md).
3. Read [state and components](reactivity.md) before adding reactive behavior.
4. Choose [layouts](layout.md) and [form controls](forms.md) for your screen.
5. Add [headless interaction tests](testing.md).

Examples are focused snippets unless they contain a `package` declaration.
They use the `ggui`, `widget` (`github.com/ironpark/ggui/widget`), `ui`, and
`theme` (`github.com/ironpark/ggui/ui/theme`) packages; application callbacks,
data, and colors such as `save`, `todos`, and `accent` are supplied by your app. Run shell commands
from the repository root unless a guide says otherwise.

> [!IMPORTANT]
> APIs are under active development. Native accessibility currently supports
> **macOS and Windows**; the Linux bridge is not implemented. Native file
> dialogs run on macOS, Windows, and desktops with `zenity` or `kdialog`
> on `PATH`; elsewhere they report `runtime.ErrUnsupported`. App menus in the
> menu bar are **macOS** only; elsewhere menus are drawn with `ui.AppMenubar`.
> Tray icons show on **macOS and Windows**; elsewhere a tray does nothing.

## Find a topic

### Getting started and core

| I want to… | Read |
| --- | --- |
| Configure a window and app lifecycle | [Getting started](getting-started.md) |
| Browse runnable applications | [Examples](examples/README.md) |
| Understand signals, derived values, ownership, and cleanup | [State and components](reactivity.md) |
| Update the UI from background work | [Threads](reactivity.md#threads) · [Resources](reactivity.md#resources-and-await-blocks) |
| Render changing or large collections | [Keyed lists](reactivity.md#keyed-lists) · [Tables](data-and-navigation.md#tables) |
| Route URLs to pages and persistent layouts | [Routing](routing.md) |
| Handle pointer input, shortcuts, scrolling, and focus | [Input and focus](input.md) |
| Accept dropped files or open a native file dialog | [Drag and drop](input.md#drag-and-drop) · [File dialogs](input.md#native-file-dialogs) |

### UI and styling

| I want to… | Read |
| --- | --- |
| Arrange text, images, and overlays | [Widgets and layout](layout.md) |
| Build forms, field sets and validation states | [Forms and controls](forms.md) · [Input OTP](carousel-and-otp.md#input-otp) |
| Add menus, dates and date ranges, search, a sidebar, or navigation | [Data and navigation](data-and-navigation.md) |
| Show notices, toasts, dialogs, sheets, cards, and loading states | [Feedback and composition](feedback-and-composition.md) |
| Build swipeable slides | [Carousel](carousel-and-otp.md#carousel) |
| Visualize data | [Charts](charts.md) |
| Build chat screens or questionnaires | [Chat components and questionnaires](chat.md) |
| Render Markdown, such as a streaming chat reply | [Markdown](chat.md#markdown) |
| Compare component coverage and behavior with shadcn/ui | [shadcn/ui comparison](shadcn-comparison.md) · [66-component catalog coverage](shadcn-implementation.md) |
| Customize colors, spacing, shadows, and themes, or offer light, dark and system modes | [Styling and themes](styling.md) · [Theme recipes and modes](../ui/theme/README.md) |
| Load fonts, render emoji, or change icon sets | [Fonts, emoji, and icons](fonts.md) |
| Show rich, selectable or right-to-left text | [Rich text](layout.md#rich-text) · [Selectable text](layout.md#selectable-text) · [Right-to-left text](fonts.md#right-to-left-text) |
| Animate values or entering/leaving content | [Animation](animation.md) |

### Desktop, localization, and distribution

| I want to… | Read |
| --- | --- |
| Open several windows, move and resize them, or follow their state | [Windows and the desktop](windows-and-platform.md) |
| Add app menus, a tray icon, message boxes, or keep one instance running | [Windows and the desktop](windows-and-platform.md) |
| Support assistive technology | [Accessibility](accessibility.md) |
| Translate the words controls show and say, such as "Close" and "No results" | [Localization](accessibility.md#localization) |
| Translate an app's own text, with plurals, dates and numbers | [Translating an app](i18n.md) |
| Create an app, run live rebuilds, bundle for macOS, Windows, Linux (`.tar.gz`, `.deb`, `.rpm`) or web, check translations, record GIFs, or inspect machine setup | [The ggui command](cli.md) |
| Write a custom Windows/macOS installer in Go with custom steps, hooks, and live preview | [Writing an installer in Go](installer.md) |
| Add Sparkle 2-compatible automatic updates with Ed25519 signing and built-in update UI | [Auto-updating an app](updater.md) |
| Package and install apps on Linux (`.deb`, `.rpm`, tarball, runtime libraries) | [Linux](linux.md) |
| Publish an app to GitHub, Homebrew and Chocolatey | [Releasing an app](releasing.md) |

### Testing and internals

| I want to… | Read |
| --- | --- |
| Test and inspect a UI | [Testing](testing.md) · [Widget inspector](inspector.md) |
| Make an animated GIF of the app for a README, with no screen capture | [Recording a tour](recording.md) |
| Write a custom widget or understand rendering | [Custom widgets and rendering](rendering.md) |
| Understand API conventions and runtime setters | [Public API conventions](api-conventions.md) |
| Build examples, run checks, or navigate the source | [Development](development.md) |

## Package and asset references

Package-specific READMEs remain beside their code and assets:

- [Icon sets, custom SVGs, precedence, and caching](../ui/icons/README.md)
- [Bundled Lucide icons](../ui/icons/lucide/README.md), [Tabler icons](../ui/icons/tabler/README.md), and [Heroicons](../ui/icons/heroicons/README.md)
- [Full icon library modules: Lucide](../icons/lucide/README.md), [Tabler](../icons/tabler/README.md), [Heroicons](../icons/heroicons/README.md)
- [Optional Noto Color Emoji font, a module of its own](../fonts/notoemoji/README.md)
- [Optional Go fonts, with bold, for every platform](../fonts/gofont/README.md)
- [Optional Inter font](../fonts/inter/README.md)
- [Optional Geist and Geist Mono fonts](../fonts/geist/README.md)
- [Gallery chat assets and licenses](examples/gallery/assets/chat/README.md)
- [Charts subpackage](../ui/chart/README.md)
- [Chat subpackage](../ui/chat/README.md)
- [Theme package](../ui/theme/README.md)
- [Vendored theme palettes and sources](../ui/theme/README.md#palette-sources)
