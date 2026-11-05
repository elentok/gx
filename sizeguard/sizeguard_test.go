// Package sizeguard holds the repo-wide size guardrail (docs/specs/split-server-pkg.md,
// "Guardrail"). It fails when a package, a file or a type grows past its limit, so a
// large unit is split instead of growing further.
package sizeguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// Default limits.
const (
	maxPackageFiles   = 50
	maxSourceLines    = 4500
	maxTestLines      = 5000
	maxTestFuncs      = 180
	maxFileLines      = 1000
	maxMethodsPerType = 50
)

// Measures, as they appear in messages and in package exception keys.
const (
	measureFiles       = "files"
	measureSourceLines = "source lines"
	measureTestLines   = "test lines"
	measureTestFuncs   = "Test funcs"
	measureLines       = "lines"
	measureMethods     = "methods"
)

// Reasons are the only reasons an exception entry may give.
const (
	reasonGenerated  = "generated"
	reasonCohesive   = "cohesive type"
	reasonTableData  = "table data"
	splitInstruction = "Split it, do not raise the limit. See .ai/index.md (File size)."
)

var reasons = []string{reasonGenerated, reasonCohesive, reasonTableData}

// exception raises one limit. Entries only shrink: the limit must equal the current
// size, and the entry goes once the size is back under the default.
//
// Keys: a file path ("server/x.go"), a type ("server.Type", directory then name) or a
// package measure ("server:files", "server:source lines", "server:test lines",
// "server:Test funcs").
type exception struct {
	limit  int
	reason string
}

// exceptions starts empty and stays empty unless a split proves infeasible.
var exceptions = map[string]exception{}

type measurement struct {
	key, name, measure string
	size, def          int
}

func countLines(src string) int {
	n := strings.Count(src, "\n")
	if src != "" && !strings.HasSuffix(src, "\n") {
		n++
	}
	return n
}

func isTestFunc(fn *ast.FuncDecl) bool {
	if fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Type.Params == nil {
		return false
	}
	if len(fn.Type.Params.List) != 1 {
		return false
	}
	star, ok := fn.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "T" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "testing"
}

// receiverName unwraps pointer, paren and generic receivers to the type name.
func receiverName(e ast.Expr) string {
	for {
		switch x := e.(type) {
		case *ast.StarExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.IndexListExpr:
			e = x.X
		case *ast.Ident:
			return x.Name
		default:
			return ""
		}
	}
}

type pkgStats struct {
	files, sourceLines, testLines, testFuncs int
}

// measure sizes every package, file and type in files (tracked path -> source).
func measure(files map[string]string) ([]measurement, []string) {
	var problems []string
	pkgs := map[string]*pkgStats{}
	methods := map[string]int{}
	var out []measurement

	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		src := files[p]
		dir := path.Dir(p)
		isTest := strings.HasSuffix(p, "_test.go")
		lines := countLines(src)
		st := pkgs[dir]
		if st == nil {
			st = &pkgStats{}
			pkgs[dir] = st
		}
		st.files++
		if isTest {
			st.testLines += lines
		} else {
			st.sourceLines += lines
		}
		out = append(out, measurement{p, p, measureLines, lines, maxFileLines})

		f, err := parser.ParseFile(token.NewFileSet(), p, src, parser.SkipObjectResolution)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: cannot parse: %v", p, err))
			continue
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			switch {
			case isTest && isTestFunc(fn):
				st.testFuncs++
			case !isTest && fn.Recv != nil && len(fn.Recv.List) == 1:
				if name := receiverName(fn.Recv.List[0].Type); name != "" {
					methods[dir+"."+name]++
				}
			}
		}
	}

	for dir, st := range pkgs {
		for _, m := range []struct {
			measure   string
			size, def int
		}{
			{measureFiles, st.files, maxPackageFiles},
			{measureSourceLines, st.sourceLines, maxSourceLines},
			{measureTestLines, st.testLines, maxTestLines},
			{measureTestFuncs, st.testFuncs, maxTestFuncs},
		} {
			out = append(out, measurement{dir + ":" + m.measure, dir, m.measure, m.size, m.def})
		}
	}
	for key, n := range methods {
		out = append(out, measurement{key, key, measureMethods, n, maxMethodsPerType})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key+out[i].measure < out[j].key+out[j].measure })
	return out, problems
}

// check returns one problem per breach or bad exception entry.
func check(files map[string]string, allow map[string]exception) []string {
	ms, problems := measure(files)
	seen := map[string]bool{}
	for _, m := range ms {
		seen[m.key] = true
		limit := m.def
		if e, ok := allow[m.key]; ok {
			switch {
			case m.size <= m.def:
				problems = append(problems, fmt.Sprintf("%s: exception entry is not needed (%s is %d, default limit is %d): drop the entry.", m.key, m.measure, m.size, m.def))
				continue
			case e.limit > m.size:
				problems = append(problems, fmt.Sprintf("%s: exception limit %d is above the current %s %d: lower the limit.", m.key, e.limit, m.measure, m.size))
				continue
			}
			limit = e.limit
		}
		if m.size > limit {
			problems = append(problems, fmt.Sprintf("%s: %s is %d, limit is %d. %s", m.name, m.measure, m.size, limit, splitInstruction))
		}
	}
	keys := make([]string, 0, len(allow))
	for k := range allow {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !seen[k] {
			problems = append(problems, fmt.Sprintf("%s: exception entry matches nothing: drop the entry.", k))
		}
		if !slices.Contains(reasons, allow[k].reason) {
			problems = append(problems, fmt.Sprintf("%s: exception reason %q is not one of %v.", k, allow[k].reason, reasons))
		}
	}
	return problems
}

func trackedSources(t *testing.T) map[string]string {
	t.Helper()
	out, err := exec.Command("git", "-C", "..", "ls-files", "*.go").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	files := map[string]string{}
	for p := range strings.FieldsSeq(string(out)) {
		src, err := os.ReadFile(filepath.Join("..", p))
		if err != nil {
			continue // tracked but deleted in the working tree
		}
		files[p] = string(src)
	}
	return files
}

func TestSizeLimits(t *testing.T) {
	for _, p := range check(trackedSources(t), exceptions) {
		t.Error(p)
	}
}

func TestExceptionReasons(t *testing.T) {
	for k, e := range exceptions {
		if !slices.Contains(reasons, e.reason) {
			t.Errorf("%s: reason %q is not one of %v", k, e.reason, reasons)
		}
	}
}

// fixtures

func src(pkg string, lines int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "package %s\n", pkg)
	for range lines - 1 {
		b.WriteString("// filler\n")
	}
	return b.String()
}

func tests(pkg string, n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "package %s\nimport \"testing\"\n", pkg)
	for i := range n {
		fmt.Fprintf(&b, "func Test%d(t *testing.T) {}\n", i)
	}
	return b.String()
}

func methodsOf(recv string, n int) string {
	var b strings.Builder
	b.WriteString("package x\ntype T struct{}\n")
	for i := range n {
		fmt.Fprintf(&b, "func (%s) M%d() {}\n", recv, i)
	}
	return b.String()
}

func many(dir string, n, lines int, suffix string) map[string]string {
	files := map[string]string{}
	for i := range n {
		files[fmt.Sprintf("%s/f%d%s", dir, i, suffix)] = src("x", lines)
	}
	return files
}

func merge(parts ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range parts {
		maps.Copy(out, m)
	}
	return out
}

func TestCheck_Breaches(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  []string // every substring must appear in one message
	}{
		{"too many files", many("big", 51, 1, ".go"), []string{"big", "files", "limit is 50", "is 51"}},
		{"too many source lines", many("big", 6, 751, ".go"), []string{"big", "source lines", "limit is 4500", "is 4506"}},
		{"too many test lines", many("big", 6, 834, "_test.go"), []string{"big", "test lines", "limit is 5000", "is 5004"}},
		{"too many Test funcs", map[string]string{"big/a_test.go": tests("x", 181)}, []string{"big", "Test funcs", "limit is 180", "is 181"}},
		{"file too long", map[string]string{"p/long.go": src("x", 1001)}, []string{"p/long.go", "lines", "limit is 1000", "is 1001"}},
		{"test file too long", map[string]string{"p/long_test.go": src("x", 1001)}, []string{"p/long_test.go", "limit is 1000", "is 1001"}},
		{"type too many methods", map[string]string{"p/t.go": methodsOf("t *T", 51)}, []string{"p.T", "methods", "limit is 50", "is 51"}},
		{"value receiver", map[string]string{"p/t.go": methodsOf("t T", 51)}, []string{"p.T", "is 51"}},
		{"generic receiver", map[string]string{"p/t.go": strings.Replace(methodsOf("t *T[K]", 51), "type T struct{}", "type T[K any] struct{}", 1)}, []string{"p.T", "is 51"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			problems := check(c.files, nil)
			if len(problems) == 0 {
				t.Fatal("want a problem, got none")
			}
			for _, w := range append(c.want, "Split", "do not raise the limit", ".ai/index.md") {
				if !slices.ContainsFunc(problems, func(p string) bool { return strings.Contains(p, w) }) {
					t.Errorf("no problem contains %q:\n%s", w, strings.Join(problems, "\n"))
				}
			}
		})
	}
}

func TestCheck_WithinLimits(t *testing.T) {
	files := merge(
		many("a", 50, 1, ".go"),
		map[string]string{
			"b/a_test.go": tests("x", 180) + "func TestNot(t *testing.B) {}\nfunc Test(x int) {}\nfunc BenchmarkX(b *testing.B) {}\nfunc ExampleX() {}\n",
			"b/long.go":   src("x", 1000),
			"c/t.go":      methodsOf("t *T", 50),
			"c/t_test.go": "package x\nfunc (T) A() {}\nfunc (T) B() {}\n", // test-file methods do not count
			"d/m.go":      "package main\nfunc main() {}\n",
		},
	)
	if problems := check(files, nil); len(problems) != 0 {
		t.Fatalf("want none, got:\n%s", strings.Join(problems, "\n"))
	}
}

func TestCheck_Exceptions(t *testing.T) {
	over := map[string]string{"p/long.go": src("x", 1200)}
	cases := []struct {
		name  string
		files map[string]string
		allow map[string]exception
		want  []string // empty: no problems
	}{
		{"entry matching the size passes", over, map[string]exception{"p/long.go": {1200, reasonCohesive}}, nil},
		{"entry above the current size", over, map[string]exception{"p/long.go": {1300, reasonCohesive}}, []string{"p/long.go", "lower the limit", "1300", "1200"}},
		{"entry under the default", map[string]string{"p/short.go": src("x", 10)}, map[string]exception{"p/short.go": {1100, reasonCohesive}}, []string{"p/short.go", "drop the entry", "10", "1000"}},
		{"entry for nothing", over, map[string]exception{"gone/x.go": {1100, reasonCohesive}}, []string{"gone/x.go", "drop the entry"}},
		{"unknown reason", over, map[string]exception{"p/long.go": {1200, "because"}}, []string{"p/long.go", "reason"}},
		{"entry below the size still fails", over, map[string]exception{"p/long.go": {1100, reasonCohesive}}, []string{"p/long.go", "limit is 1100", "is 1200", "Split"}},
		{"package entry", many("big", 51, 1, ".go"), map[string]exception{"big:files": {51, reasonGenerated}}, nil},
		{"type entry", map[string]string{"p/t.go": methodsOf("t *T", 60)}, map[string]exception{"p.T": {60, reasonCohesive}}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			problems := check(c.files, c.allow)
			if len(c.want) == 0 {
				if len(problems) != 0 {
					t.Fatalf("want none, got:\n%s", strings.Join(problems, "\n"))
				}
				return
			}
			joined := strings.Join(problems, "\n")
			for _, w := range c.want {
				if !strings.Contains(joined, w) {
					t.Errorf("no problem contains %q:\n%s", w, joined)
				}
			}
		})
	}
}

func TestCheck_OneErrorPerProblem(t *testing.T) {
	files := merge(many("big", 51, 1, ".go"), map[string]string{"p/long.go": src("x", 1001)})
	if got := len(check(files, nil)); got != 2 {
		t.Fatalf("want 2 problems, got %d", got)
	}
}
