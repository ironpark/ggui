# Linux

[Documentation](README.md) · [The ggui command](cli.md)

A ggui app builds for Linux from any machine, without cgo or a C
toolchain, and `ggui build --target linux` packages it for Linux
desktops: an install tree, a tarball of it, and Debian and RPM packages.
This page is about what those contain and what the app needs on the
machine it runs on. [The Linux packages](cli.md#the-linux-packages) in the
command's reference has the flags and the file names.

```sh
GOARCH=amd64 ggui build --target linux --format tar,deb,rpm
```

## What runs

The program is built with `CGO_ENABLED=0` for `amd64`, `arm64` or
`riscv64`, as `GOARCH` says. It links only glibc's `libc.so.6`, `libdl.so.2`
and `libpthread.so.0` and its dynamic loader, which purego needs to load
the rest; everything else ggwd and ggfx use is opened with `dlopen` when
the app starts:

| Library | Package (Debian, Ubuntu) | Package (Fedora) | Needed for |
| --- | --- | --- | --- |
| `libX11.so.6` | `libx11-6` | `libX11` | Windows: without it none opens. |
| `libXrandr.so.2`, `libXi.so.6`, `libXcursor.so.1`, `libXinerama.so.1`, `libXext.so.6`, `libXrender.so.1` | `libxrandr2`, `libxi6`, `libxcursor1`, `libxinerama1`, `libxext6`, `libxrender1` | `libXrandr`, `libXi`, `libXcursor`, `libXinerama`, `libXext`, `libXrender` | Monitors, input, cursors and transparency; without one that part is limited. |
| `libGL.so.1` with GLX, or `libGLESv2.so.2` with `libEGL.so.1` | `libgl1`, `libegl1` | `libglvnd-glx`, `libglvnd-egl` | Drawing. ggfx uses OpenGL ES when `libGLESv2` is there and OpenGL otherwise, and ggwd makes the context with GLX or EGL. The driver must offer OpenGL 3.3 or OpenGL ES 3.0. |

A package manager cannot see what is opened at run time, so the packages
name these as dependencies: `linux.depends_deb` and `linux.depends_rpm`
in `ggui.toml`, which default to the packages above. `libgl1` and
`libegl1` are the GL vendor-neutral dispatch libraries every desktop has;
Debian's `libgl1` brings `libglx0`, and with it Mesa's drivers, and
Fedora's `libglvnd-glx` and `libglvnd-egl` require `mesa-libGL` and
`mesa-libEGL`. A GPU vendor's driver, such as NVIDIA's, provides the same
libraries. For another family, openSUSE for one, set `depends_rpm` to its
names, or to the libraries themselves, such as `libX11.so.6()(64bit)`,
which any RPM distribution resolves.

The app also runs programs for some features, and does without the
feature when one is missing: `fc-match` (fontconfig) to find the system's
UI fonts, `zenity` or `kdialog` for file and message dialogs, and
`xdg-open` for `runtime.OpenURL`. The packages do not depend on them;
desktops have them. `ggui doctor`, run on the Linux machine, checks for
all of this.

### X11 and Wayland

ggwd draws through X11 and has no Wayland backend. Under a Wayland
session the app runs through XWayland, which GNOME, KDE Plasma and most
other compositors start on demand; `DISPLAY` must be set, as those
sessions set it. The window is then an X11 window to the compositor.

### musl

The program needs glibc's dynamic loader, `/lib64/ld-linux-x86-64.so.2`
(or `ld-linux-aarch64.so.1`). On a musl distribution, such as Alpine,
install `gcompat`, which provides it over musl, along with the X11 and
Mesa packages (`libx11`, `libxrandr`, `libxi`, `libxcursor`,
`libxinerama`, `libxext`, `libxrender`, `mesa-gl`, `mesa-egl`). The
packages ggui makes are for glibc distributions; on Alpine use the
tarball.

## What the packages contain

```
/usr/bin/hello-world                                              the program, 0755
/usr/share/applications/com.example.hello.desktop                 the menu entry
/usr/share/icons/hicolor/{16,24,32,48,64,128,256,512}x…/apps/com.example.hello.png
/usr/share/icons/hicolor/scalable/apps/com.example.hello.svg      when app.icon is an SVG
/usr/share/metainfo/com.example.hello.metainfo.xml                for software centres
```

The program's name is the package name, made from `app.name`; the other
files are named for `app.id`. The desktop entry's `StartupWMClass` is the
ID too, and the program sets it as its window class (ggui build passes it
with `-X github.com/ironpark/ggui.buildID`), which is how GNOME Shell, KDE
Plasma and other docks put the app's windows under its entry and icon. So
set `app.id` before shipping, and keep it.

After installing and removing, the packages refresh the desktop entry and
icon caches with `update-desktop-database` and `gtk-update-icon-cache`
where those are installed; the scripts never fail the installation.

```sh
sudo apt install ./hello-world_1.2.0_amd64.deb
sudo dnf install ./hello-world-1.2.0-1.x86_64.rpm
```

Installing a local file this way resolves its dependencies; `dpkg -i`
alone does not, and `apt -f install` then fetches them.

The packages are not signed. A repository signs its own index, as
`reprepro`, `aptly` or `createrepo` with `gpg` do; for a package handed
out alone, publish its checksum.

## The tarball, and installing for one user

`hello-world-1.2.0-linux-amd64.tar.gz` holds the same tree, its paths
starting at `usr/`. Unpacked with `--strip-components=1`, `usr/bin` and
`usr/share` land as `bin` and `share` in the directory given, which suits
the two places a desktop looks besides `/usr`:

```sh
# for every user, beside what the package manager installs
sudo tar -xzf hello-world-1.2.0-linux-amd64.tar.gz -C /usr/local --strip-components=1

# for yourself alone, without root
tar -xzf hello-world-1.2.0-linux-amd64.tar.gz -C ~/.local --strip-components=1
```

The second puts the program in `~/.local/bin` and the rest in
`~/.local/share`, which is `$XDG_DATA_HOME` by default, where menus find
the desktop entry and the icons. The entry starts the program by its name,
so `~/.local/bin` must be on the `PATH` of the desktop session. Debian's
and Ubuntu's `~/.profile` add it when the directory exists at login, so
log out and in once after creating it; elsewhere, add it in your profile. Unpacking with `-C /` installs into `/usr`
as the packages do, but behind the package manager's back; prefer a
package, or `/usr/local`.

To remove a per-user install, delete the files the tarball lists:

```sh
tar -tzf hello-world-1.2.0-linux-amd64.tar.gz | grep -v '/$' | sed 's|^usr/|'"$HOME"'/.local/|' | xargs rm -f
```

## Not yet on Linux

- **Tray icons.** `app.AddTray` shows no icon on Linux yet; see
  [Platform support](windows-and-platform.md#platform-support).
- **Screen readers.** The accessibility bridge reaches VoiceOver on macOS
  and UI Automation on Windows; there is no AT-SPI bridge yet, so Orca does
  not see the app. See [Accessibility](accessibility.md).
- **Wayland.** See [X11 and Wayland](#x11-and-wayland).
- **Flatpak, Snap and AppImage.** ggui makes none of them; the install tree
  is a starting point for each.
