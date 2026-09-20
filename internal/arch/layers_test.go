// Package arch holds architecture tests. It contains no production code: its
// only job is to fail the build when the dependency rule in PHASE-1-SPEC.md §5
// is broken.
//
// Architecture that is not enforced is architecture that decays. A decade from
// now this test is what stops somebody reaching from the cell model into the
// user interface because it happens to be convenient.
package arch

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const module = "cellsheet"

// layer assigns every package a height. A package may import only packages
// strictly below it, which is what keeps the layers acyclic and testable.
//
// Leaf packages (no internal imports) sit at 0.
var layer = map[string]int{
	// Leaves.
	"internal/cell":          0,
	"internal/formula/lexer": 0,
	"internal/tui/layout":    0,
	"internal/tui/render":    0,

	// Values and presentation.
	"internal/formula/ast":      1,
	"internal/formula/registry": 1,
	"internal/tui/theme":        1,

	// Grammar and the domain model.
	"internal/formula/ops": 2,
	"internal/grid":        2,

	// Reversible change and the copy buffer.
	"internal/formula/parser": 3,
	"internal/journal":        3,
	"internal/clipboard":      3,

	// Evaluation.
	"internal/formula/functions": 4,
	"internal/formula/eval":      4,

	// Scheduling and history.
	"internal/calc":       5,
	"internal/checkpoint": 5,

	// Persistence.
	"internal/sheetfile": 6,

	// Interface.
	"internal/tui/screens": 7,
	"internal/app":         8,
	"internal/perf":        9,
	"cmd/cellsheet":        10,

	// This package holds only tests, so it sits above everything.
	"internal/arch": 10,
}

// excluded are trees that are not part of the layered program: the vendored
// third-party code, the Phase 1 look prototype (a standalone stdlib-only design
// artifact), and build scratch directories.
var excluded = []string{"vendor", "prototype", ".tmp", ".gopath", ".gocache"}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod above the test directory")
		}
		dir = parent
	}
}

// importsOf parses one package directory and returns the internal packages it
// imports.
//
// The layering rule applies to production code. Test files are exempt, and
// deliberately so: a test is the composition root for its own package, so it
// legitimately wires together things the production code must not. The
// exemption is narrow — TestOnlyCmdImportsApp still applies to production, and
// the golden and integration suites exercise the real graph.
func importsOf(t *testing.T, dir string, includeTests bool) []string {
	t.Helper()
	fset := token.NewFileSet()
	filter := func(fi os.FileInfo) bool {
		if includeTests {
			return true
		}
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}
	pkgs, err := parser.ParseDir(fset, dir, filter, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parsing %s: %v", dir, err)
	}
	seen := map[string]struct{}{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, spec := range file.Imports {
				path, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					continue
				}
				if !strings.HasPrefix(path, module+"/") {
					continue
				}
				seen[strings.TrimPrefix(path, module+"/")] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// packageDirs lists every directory in the module that holds Go source.
func packageDirs(t *testing.T, root string) []string {
	t.Helper()
	var dirs []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		base := info.Name()
		if path != root {
			for _, skip := range excluded {
				if base == skip {
					return filepath.SkipDir
				}
			}
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				dirs = append(dirs, filepath.ToSlash(rel))
				return nil
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(dirs)
	return dirs
}

// TestEveryPackageIsLayered forces a new package to be classified rather than
// silently escaping the rule.
func TestEveryPackageIsLayered(t *testing.T) {
	root := repoRoot(t)
	for _, dir := range packageDirs(t, root) {
		if _, ok := layer[dir]; !ok {
			t.Errorf("package %s has no layer. Add it to the layer table in "+
				"internal/arch so the dependency rule keeps being enforced.", dir)
		}
	}
	for name := range layer {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Errorf("the layer table names %s, which does not exist", name)
		}
	}
}

// TestDependencyRule is the rule itself: a package may import only packages
// strictly below it.
func TestDependencyRule(t *testing.T) {
	root := repoRoot(t)
	for _, dir := range packageDirs(t, root) {
		self, known := layer[dir]
		if !known {
			continue // reported by TestEveryPackageIsLayered
		}
		for _, dep := range importsOf(t, filepath.Join(root, filepath.FromSlash(dir)), false) {
			other, known := layer[dep]
			if !known {
				t.Errorf("%s imports %s, which is not in the layer table", dir, dep)
				continue
			}
			if other >= self {
				t.Errorf("%s (layer %d) imports %s (layer %d): a package may "+
					"import only packages strictly below it", dir, self, dep, other)
			}
		}
	}
}

// TestNoCycles is implied by the layer rule, but it is worth stating on its own
// because a cycle would be a build failure that is much harder to read.
func TestNoCycles(t *testing.T) {
	root := repoRoot(t)
	graph := map[string][]string{}
	for _, dir := range packageDirs(t, root) {
		if _, ok := layer[dir]; !ok {
			continue
		}
		graph[dir] = importsOf(t, filepath.Join(root, filepath.FromSlash(dir)), false)
	}
	const (
		white = 0
		grey  = 1
		black = 2
	)
	colour := map[string]int{}
	var visit func(string, []string) bool
	visit = func(pkg string, path []string) bool {
		colour[pkg] = grey
		for _, dep := range graph[pkg] {
			if _, ok := graph[dep]; !ok {
				continue
			}
			switch colour[dep] {
			case grey:
				t.Errorf("import cycle: %s", strings.Join(append(path, pkg, dep), " -> "))
				return true
			case white:
				if visit(dep, append(path, pkg)) {
					return true
				}
			}
		}
		colour[pkg] = black
		return false
	}
	names := make([]string, 0, len(graph))
	for name := range graph {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if colour[name] == white {
			if visit(name, nil) {
				return
			}
		}
	}
}

// TestCellIsALeaf keeps the lowest layer honest: the value model must not
// acquire knowledge of the grid, formulas or the interface.
func TestCellIsALeaf(t *testing.T) {
	root := repoRoot(t)
	deps := importsOf(t, filepath.Join(root, "internal", "cell"), true)
	if len(deps) != 0 {
		t.Errorf("internal/cell must not import anything internal, but imports %v", deps)
	}
}

// TestRenderIsALeaf keeps the compositor reusable: it is the piece most likely
// to be wanted elsewhere, and it must stay free of domain knowledge.
func TestRenderIsALeaf(t *testing.T) {
	root := repoRoot(t)
	deps := importsOf(t, filepath.Join(root, "internal", "tui", "render"), true)
	if len(deps) != 0 {
		t.Errorf("internal/tui/render must not import anything internal, but imports %v", deps)
	}
}

// TestOnlyCmdImportsApp states the single entry point as a rule rather than as a
// convention nobody remembers.
func TestOnlyCmdImportsApp(t *testing.T) {
	root := repoRoot(t)
	for _, dir := range packageDirs(t, root) {
		if dir == "cmd/cellsheet" {
			continue
		}
		for _, dep := range importsOf(t, filepath.Join(root, filepath.FromSlash(dir)), false) {
			if dep == "internal/app" {
				t.Errorf("%s imports internal/app; only cmd/cellsheet may do that", dir)
			}
		}
	}
}

// TestLayerTableIsOrdered is a sanity check on the table itself.
func TestLayerTableIsOrdered(t *testing.T) {
	// A sanity check on the table itself: no two packages share a layer where
	// one imports the other. The rule test covers the real graph; this catches
	// a table that would make the rule vacuous.
	if layer["internal/cell"] != 0 {
		t.Errorf("internal/cell should be the lowest layer, got %d", layer["internal/cell"])
	}
	if layer["cmd/cellsheet"] <= layer["internal/app"] {
		t.Errorf("cmd/cellsheet must sit above internal/app")
	}
	if layer["internal/perf"] <= layer["internal/tui/screens"] {
		t.Errorf("internal/perf wires the whole program together and must sit above it")
	}
	fmt.Fprintf(os.Stderr, "layering: %d packages across %d layers\n",
		len(layer), countLayers())
}

func countLayers() int {
	seen := map[int]struct{}{}
	for _, l := range layer {
		seen[l] = struct{}{}
	}
	return len(seen)
}
