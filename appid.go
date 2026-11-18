package ggui

import (
	"path/filepath"
	"strings"

	"github.com/ironpark/ggwd"
)

// buildID is the app's reverse-DNS id, "com.example.editor", which ggui
// build links into every app it builds with
//
//	-ldflags "-X github.com/ironpark/ggui.buildID=<id>"
//
// It is the same id as the macOS bundle identifier and the name of the
// Linux package's desktop file. Outside such a build it is "". The linker's
// -X sets only a package-level string variable that has no initializer
// other than "", so buildID must stay one: no constant, no other type, and
// no value computed at init.
var buildID string

// idSource is where the app's id came from, from the weakest to the
// strongest claim to name the app.
type idSource uint8

const (
	idExecutable idSource = iota // the executable's name
	idInstance                   // the id App.SingleInstance claimed
	idBuild                      // buildID, linked in by ggui build
	idBundle                     // the macOS bundle identifier
)

// AppID returns the id that names the app to the system: the first of
//
//   - on macOS, the bundle identifier of the app bundle it runs from;
//   - the id ggui build linked into the executable, the app's reverse-DNS
//     id from its project, which is also the bundle identifier of the
//     bundle ggui build makes;
//   - the id given to App.SingleInstance, once it has been called;
//   - the running executable's name, links resolved, without its
//     extension, or "ggui" where it has none, as in a browser.
//
// A binary ggui build made so returns its project's id everywhere, from
// the start of main. A dev run, go run or go build, outside a bundle, has
// no linked id: it returns SingleInstance's id after that call, and the
// executable's name before it or without it, which for go run is a
// temporary name, often the package's directory. An app that keeps files
// of its own under the id, in os.UserConfigDir say, should therefore call
// SingleInstance first or expect a different name in development.
//
// The id is returned as it was given. A reverse-DNS id is a valid file
// name everywhere; an id of the app's own making may need its slashes and
// colons replaced before it names a file. It names the app's login item
// (SetOpenAtLogin), the directory its windows remember their placements in,
// and on X11 its windows' class, which a desktop file's StartupWMClass is
// matched against. It may be called from any goroutine.
func AppID() string {
	id, _ := appID()
	return id
}

// appID is the app's id and where it came from.
func appID() (string, idSource) {
	exe, err := loginExecutable()
	if err != nil {
		exe = ""
	}
	return appIDOf(exe)
}

// appIDOf is the app's id, should it be running exe.
func appIDOf(exe string) (string, idSource) {
	instance := ""
	if p := instanceName.Load(); p != nil {
		instance = *p
	}
	return chooseAppID(bundleID(), buildID, instance, exe)
}

// chooseAppID is the first set of the bundle identifier, the linked build
// id and the instance id, or else the name of exe without its extension,
// or "ggui" when exe is "".
//
// When an app is built and bundled by ggui build the bundle identifier and
// the build id are the same. They can differ only when someone else made
// the bundle or edited its Info.plist; then macOS knows the app by the
// bundle identifier, so that one wins.
func chooseAppID(bundle, build, instance, exe string) (string, idSource) {
	switch {
	case bundle != "":
		return bundle, idBundle
	case build != "":
		return build, idBuild
	case instance != "":
		return instance, idInstance
	case exe != "":
		base := filepath.Base(exe)
		if name := strings.TrimSuffix(base, filepath.Ext(base)); name != "" {
			return name, idExecutable
		}
	}
	return "ggui", idExecutable
}

// runOptions are the options App.Run starts ggwd with, given the app's id
// and where it came from.
//
// Holding a key on macOS pops up the accent menu, as it does in every text
// field on the platform; text editing relies on it.
//
// On X11 a dock or task bar finds a window's desktop file by matching the
// res_class of the window's WM_CLASS against the file's StartupWMClass
// (GNOME Shell tries the res_name first, then the res_class, then a
// desktop file named for either; KDE matches the res_class). The Linux
// package ggui build makes installs <id>.desktop with StartupWMClass=<id>,
// so an id the app was given, linked in or claimed by SingleInstance, is
// the class. The instance name stays ggwd's default, the executable's
// name, the program name the ICCCM asks for. An id that is only the
// executable's name leaves both to ggwd, which capitalises the instance
// name for the class, as it did before the app had an id.
func runOptions(id string, src idSource) *ggwd.RunOptions {
	o := &ggwd.RunOptions{MacOS: ggwd.MacOSOptions{PressAndHold: true}}
	if src >= idInstance {
		o.X11.ClassName = id
	}
	return o
}
