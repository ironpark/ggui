package ggui

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestEnginePackagesStayBelowTheUI keeps the engines ggui is built on free
// of what ggui itself is: widgets, the reactive runtime and the frame. An
// engine that needs one of them has its boundary in the wrong place, and
// loses the benchmarks and tests that run it alone.
func TestEnginePackagesStayBelowTheUI(t *testing.T) {
	if testing.Short() {
		t.Skip("lists the module's packages")
	}
	t.Parallel()
	// The Taskfile's bench default, ENGINES, is these with reactive, less
	// gem, which has nothing to measure.
	engines := []string{"internal/fontsys", "internal/textlayout", "internal/paragraph", "internal/imgcache", "internal/render", "internal/region", "internal/framestats", "internal/textedit", "internal/editlayout", "pkg/kinetics", "pkg/virtual", "pkg/geom", "pkg/gem", "pkg/layout"}
	forbidden := []string{"github.com/ironpark/ggui", "github.com/ironpark/ggui/internal/reactive", "github.com/ironpark/ggui/internal/property"}
	for _, pkg := range engines {
		out, err := exec.Command("go", "list", "-deps", "./"+pkg).Output()
		if err != nil {
			t.Fatalf("go list %s: %v", pkg, err)
		}
		deps := strings.Fields(string(out))
		for _, f := range forbidden {
			if slices.Contains(deps, f) {
				t.Errorf("%s depends on %s", pkg, f)
			}
		}
	}
}

// The root package is two things in one directory: the kernel, which is
// the frame loop, input, painting, layout caching and the rest that every
// widget stands on, and the built-in widgets written on it. The widgets are
// to be written on the kernel's public API alone, as a widget of another
// package is, so that they can leave the root; until then they share it,
// and nothing but this test keeps a widget file from reaching a name the
// kernel keeps to itself, or the kernel from reaching into a widget.
//
// kernelFiles and widgetFiles say which root file is which, by name.
var (
	kernelFiles = []string{"anim", "app", "boundary", "canvas", "damage", "drag", "effects", "environment", "font", "input", "inspector", "keys", "keys_gen", "loop", "menu", "overlay", "paint", "probe", "reactivity", "semantics", "widget", "window", "windowprobe"}
	widgetFiles = []string{"dragdrop", "for", "for_sticky", "popup", "richtext", "scroll", "text", "transition", "widgets"}
)

// privateAcross is every unexported name a widget file still takes from a
// kernel file, or a kernel file from a widget file, as "file name": a
// package-level name, or Type.field or Type.method. Each waits for the
// public protocol its group names; once a file no longer uses a name, it
// comes off the list, and nothing goes on it.
var privateAcross = []string{
	// Layout boundary: a Scroll is one, and holds the layouts asked for
	// inside it in its own viewport, which Invalidate cannot say. Its
	// moves mark its own cache stale, not the cache above, as Invalidate
	// would.
	"boundary.go ScrollWidget.hold",
	"boundary.go scrollPortKey",
	"scroll.go layoutBoundary",
	"scroll.go layoutBoundary.begin",
	"scroll.go layoutBoundary.end",
	"scroll.go layoutBoundary.paint",
	"scroll.go layoutBoundary.placed",
	"scroll.go layoutBoundary.win",
	"scroll.go noteContained",
	"scroll.go cacheOwner",
	"scroll.go CachedWidget.invalidate",

	// Each's layout requests: a list whose items changed, or that moved in
	// its Scroll, asks for a whole layout, where Invalidate would hold it
	// in the Scroll around it and check that Scroll's size by it.
	"for.go cacheOwner",
	"for.go CachedWidget.invalidate",

	// Keyed mounting: Each's rows and Switcher's subtrees are mounted
	// under keys, with identities of their own, and Switcher marks its own
	// component's cache stale when a key mounts.
	"for.go remembering",
	"transition.go mountKeyed",
	"transition.go observeKey",
	"transition.go ComponentWidget.cw",
	"transition.go CachedWidget.invalidate",

	// A Motion moved along with the content it scrolls, keeping its
	// timing, which no public method does.
	"scroll.go Motion.from",
	"scroll.go Motion.to",
}

// TestWidgetFilesUseThePublicKernel type-checks the root package and fails
// when a widget file uses an unexported name of a kernel file, or a kernel
// file one of a widget file, that privateAcross does not list, or when it
// lists one no longer used. A root file in neither kernelFiles nor
// widgetFiles fails it too: a new file says which it is.
func TestWidgetFilesUseThePublicKernel(t *testing.T) {
	if testing.Short() {
		t.Skip("type-checks the package and its imports")
	}
	t.Parallel()
	side := map[string]string{}
	for _, f := range kernelFiles {
		side[f+".go"] = "kernel"
	}
	for _, f := range widgetFiles {
		side[f+".go"] = "widget"
	}
	all, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range all {
		if !strings.HasSuffix(f, "_test.go") && side[f] == "" {
			t.Errorf("%s is in neither kernelFiles nor widgetFiles", f)
		}
	}

	bp, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, n := range bp.GoFiles {
		f, err := parser.ParseFile(fset, n, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	// The imports are read from the export data the build cache holds,
	// which go list hands over, building what it lacks: type-checking
	// them from source takes several times as long.
	out, err := exec.Command("go", "list", "-export", "-deps", "-f", "{{.ImportPath}}\t{{.Export}}", ".").Output()
	if err != nil {
		t.Fatalf("go list -export: %v", err)
	}
	exports := map[string]string{}
	for line := range strings.Lines(string(out)) {
		path, file, _ := strings.Cut(strings.TrimSpace(line), "\t")
		exports[path] = file
	}
	lookup := func(path string) (io.ReadCloser, error) {
		if exports[path] == "" {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(exports[path])
	}
	info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
	conf := types.Config{Importer: importer.ForCompiler(fset, "gc", lookup)}
	pkg, err := conf.Check(bp.ImportPath, fset, files, info)
	if err != nil {
		t.Fatal(err)
	}

	// A field is named by the type it is declared in.
	owner := map[*types.Var]string{}
	for _, n := range pkg.Scope().Names() {
		tn, ok := pkg.Scope().Lookup(n).(*types.TypeName)
		if !ok || tn.IsAlias() {
			continue
		}
		if st, ok := tn.Type().Underlying().(*types.Struct); ok {
			for i := range st.NumFields() {
				owner[st.Field(i)] = tn.Name()
			}
		}
	}
	name := func(obj types.Object) string {
		switch o := obj.(type) {
		case *types.Var:
			if o.IsField() {
				return owner[o.Origin()] + "." + o.Name()
			}
		case *types.Func:
			if recv := o.Signature().Recv(); recv != nil {
				rt := recv.Type()
				if p, ok := rt.(*types.Pointer); ok {
					rt = p.Elem()
				}
				if nt, ok := rt.(*types.Named); ok {
					return nt.Obj().Name() + "." + o.Name()
				}
			}
		}
		return obj.Name()
	}

	used := map[string]string{} // "file name" -> the file it is declared in
	for id, obj := range info.Uses {
		if obj.Pkg() != pkg || obj.Exported() {
			continue
		}
		switch o := obj.(type) {
		case *types.Var:
			if !o.IsField() && o.Parent() != pkg.Scope() {
				continue
			}
		case *types.Func:
			if o.Signature().Recv() == nil && o.Parent() != pkg.Scope() {
				continue
			}
		case *types.TypeName, *types.Const:
			if obj.Parent() != pkg.Scope() {
				continue
			}
		default:
			continue
		}
		from, to := filepath.Base(fset.Position(id.Pos()).Filename), filepath.Base(fset.Position(obj.Pos()).Filename)
		if side[from] != "" && side[to] != "" && side[from] != side[to] {
			used[from+" "+name(obj)] = to
		}
	}
	for use, to := range used {
		if !slices.Contains(privateAcross, use) {
			from, n, _ := strings.Cut(use, " ")
			t.Errorf("%s uses %s of %s, which is not exported: use the public API, or export what a widget of another package needs too", from, n, to)
		}
	}
	for _, use := range privateAcross {
		if _, ok := used[use]; !ok {
			t.Errorf("privateAcross lists %q, which is no longer used: take it off", use)
		}
	}
}
