package ggui

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"maps"
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
// to leave the root for a package of their own, and their tests with them,
// while the kernel's tests stay behind; until then they share it, and
// nothing but this test keeps either side from leaning on the other in a
// way that move would break.
//
// kernelFiles and widgetFiles say which root source file is which, and
// kernelTests and widgetTests which test file, each by name without .go.
var (
	kernelFiles = []string{"anim", "app", "boundary", "canvas", "damage", "drag", "effects", "environment", "font", "input", "inspector", "keys", "keys_gen", "loop", "menu", "overlay", "paint", "probe", "reactivity", "semantics", "widget", "window", "windowprobe"}
	widgetFiles = []string{"composite", "dragdrop", "for", "for_sticky", "interact", "popup", "richtext", "scroll", "text", "transition", "widgets"}
	kernelTests = []string{"anim_test", "boundary_test", "canvas_test", "damage_test", "debug_test", "effects_test", "environment_test", "fixtures_test", "font_test", "frame_alloc_test", "holding_test", "input_test", "inspector_test", "keys_gen_test", "loop_test", "menu_test", "overlay_test", "paint_test", "reactivity_test", "semantics_test", "widget_test", "window_test"}
	widgetTests = []string{"cached_test", "composite_test", "for_test", "interact_test", "linebox_test", "popup_side_test", "popup_test", "properties_test", "richtext_test", "scroll_damage_test", "scroll_test", "styles_test", "text_test", "tracking_test", "transition_test", "widget_size_test", "widgets_test", "window_neighbour_test"}
)

// guardTags are the build tags the root's files are type-checked under:
// every tag some root file is built under alone, so that each file is
// seen. The packages the root imports are read as an untagged build
// exports them; no tag changes what of theirs a root file can see.
var guardTags = []string{"ggui_debug"}

// TestKernelAndWidgetFilesStayApart type-checks the root package with its
// tests and fails when a file reaches across the line the move will draw:
//
//   - a kernel file, source or test, uses any name a widget file declares,
//     since the kernel will not import the widgets, and its tests cannot
//     without a cycle;
//   - a widget file, source or test, uses an unexported name of a kernel
//     source file, which another package cannot, or any name a kernel test
//     file declares, which no other package can see.
//
// A widget that needs more of the kernel than it exports gets it exported,
// as one of another package would need it; a kernel test that needs a
// widget to stand on takes one of fixtures_test.go, built on the kernel's
// widget protocol. There is no list of exceptions. A root file in none of
// the four lists fails it too, and so does one that no tag of guardTags
// brings into the check.
func TestKernelAndWidgetFilesStayApart(t *testing.T) {
	if testing.Short() {
		t.Skip("type-checks the package and its imports")
	}
	t.Parallel()
	type side struct{ widget, test bool }
	sides := map[string]side{}
	for _, list := range []struct {
		names []string
		side  side
	}{{kernelFiles, side{}}, {widgetFiles, side{widget: true}}, {kernelTests, side{test: true}}, {widgetTests, side{widget: true, test: true}}} {
		for _, n := range list.names {
			if _, dup := sides[n+".go"]; dup {
				t.Errorf("%s.go is listed twice", n)
			}
			sides[n+".go"] = list.side
		}
	}
	all, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range all {
		if _, ok := sides[f]; !ok {
			t.Errorf("%s is in none of kernelFiles, widgetFiles, kernelTests and widgetTests", f)
		}
	}
	for f := range sides {
		if !slices.Contains(all, f) {
			t.Errorf("%s is listed but is not in the package: take it off", f)
		}
	}

	ctx := build.Default
	ctx.BuildTags = guardTags
	bp, err := ctx.ImportDir(".", 0)
	if err != nil {
		t.Fatal(err)
	}
	names := slices.Concat(bp.GoFiles, bp.TestGoFiles)
	for _, f := range all {
		if !slices.Contains(names, f) {
			t.Errorf("%s is not type-checked under the tags %v: add the tag it is built under to guardTags", f, guardTags)
		}
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, n := range names {
		f, err := parser.ParseFile(fset, n, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	// The imports are read from the export data the build cache holds,
	// which go list hands over, building what it lacks: type-checking
	// them from source takes several times as long. The tests' imports
	// are listed rather than the package as its tests build it, which
	// would compile the tests.
	args := slices.Concat([]string{"list", "-export", "-deps", "-f", "{{.ImportPath}}\t{{.Export}}"}, bp.Imports, bp.TestImports)
	out, err := exec.Command("go", args...).Output()
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
			if n := owner[o.Origin()]; o.IsField() && n != "" {
				return n + "." + o.Name()
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

	// Each name a file reaches across for is reported once, at its first
	// use there.
	type use struct {
		from, name, to string
		exported       bool
	}
	first := map[use]token.Position{}
	for id, obj := range info.Uses {
		if obj.Pkg() != pkg {
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
		at := fset.Position(id.Pos())
		from, to := filepath.Base(at.Filename), filepath.Base(fset.Position(obj.Pos()).Filename)
		if sides[from].widget == sides[to].widget {
			continue
		}
		u := use{from, name(obj), to, obj.Exported()}
		if was, ok := first[u]; !ok || at.Offset < was.Offset {
			first[u] = at
		}
	}
	uses := slices.SortedFunc(maps.Keys(first), func(a, b use) int {
		return cmp.Or(cmp.Compare(a.from, b.from), cmp.Compare(first[a].Offset, first[b].Offset))
	})
	for _, u := range uses {
		at := fmt.Sprintf("%s:%d", u.from, first[u].Line)
		switch ts := sides[u.to]; {
		case ts.widget:
			t.Errorf("%s uses %s of %s: the kernel stands on no widget, and a kernel test takes a fixture of fixtures_test.go instead", at, u.name, u.to)
		case ts.test:
			t.Errorf("%s uses %s of %s, a kernel test file, which no other package can see", at, u.name, u.to)
		case !u.exported:
			t.Errorf("%s uses %s of %s, which is not exported: use the public API, or export what a widget of another package needs too", at, u.name, u.to)
		}
	}
}
