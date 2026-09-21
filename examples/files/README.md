# Files

Dropped files and native file dialogs in one small window: drop anything
from the desktop onto the card to list it, or press a button to open the
platform's own Open, Save or folder dialog through the host's `Dialogs`.

```sh
go run ./examples/files
```

The dialogs are macOS's `NSOpenPanel`/`NSSavePanel`, Windows' common
dialogs, and `zenity` or `kdialog` elsewhere; a platform with none of them
reports `runtime.ErrUnsupported`. The test answers them with the
`runtime.StubFilePicker` a `Probe` carries and drops in-memory files through
`Probe.Drop`, so it runs headlessly:

```sh
go test ./examples/files
```
