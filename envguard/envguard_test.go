// Package envguard holds the repo-wide guard against reading the process env
// for paths (docs/specs better-tests, Part B). Code takes HOME, PATH, the XDG
// dirs and the agent config dirs from config or its caller, so tests can hand
// it a poisoned env instead of changing the process's own.
package envguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Rules the scan counts.
const (
	ruleEnvRead   = "env-read"   // os.Getenv or os.LookupEnv of a path variable
	ruleHomeDir   = "home-dir"   // os.UserHomeDir()
	ruleChildEnv  = "child-env"  // exec.Command whose result never gets a .Env assignment
	ruleConfigEnv = "config-env" // a use of a config function that reads the process env
	selfFile      = "envguard/envguard_test.go"
	reasonConfig  = "config"
	reasonStartup = "startup"
	reasonChild   = "child program"
	reasonTool    = "user tool"
)

// reasons are the only permanent reasons an allowlist entry may give.
var reasons = []string{reasonConfig, reasonStartup, reasonChild, reasonTool}

type site struct {
	file, rule string
}

type entry struct {
	count  int
	reason string
}

// allowed is the seed generated from the tree by the scan below. A count only
// goes down: take the value from config or the caller, then lower the count.
var allowed = map[site]entry{
	{"server/server.go", ruleConfigEnv}:                   {1, reasonStartup},
	{"sound/sound.go", ruleConfigEnv}:                     {2, reasonStartup},
	{"harness/claudepid.go", ruleChildEnv}:                {1, reasonChild},
	{"harness/claudepid.go", ruleHomeDir}:                 {1, reasonChild},
	{"osnotify/osnotify.go", ruleChildEnv}:                {1, reasonTool},
	{"server/diskclean.go", ruleChildEnv}:                 {1, reasonTool},
	{"server/diskwatch.go", ruleChildEnv}:                 {1, reasonTool},
	{"sound/sound.go", ruleChildEnv}:                      {1, reasonTool},
	{"testutil/claudeenv/claudeenv.go", ruleChildEnv}:     {1, reasonTool},
	{"testutil/agentfake/design.go", ruleHomeDir}:         {1, reasonChild},
	{"testutil/agentfake/tui.go", ruleHomeDir}:            {1, reasonChild},
	{"testutil/agentfake/worker.go", ruleChildEnv}:        {1, reasonChild},
	{"testutil/gxfixture/main.go", ruleChildEnv}:          {1, reasonStartup},
	{"testutil/gxfixture/main.go", ruleEnvRead}:           {1, reasonStartup},
	{"testutil/gxfixture/main.go", ruleHomeDir}:           {1, reasonStartup},
	{"testutil/herdrfake/herdrfake.go", ruleEnvRead}:      {2, reasonChild},
	{"ui/history/conversations_actions.go", ruleChildEnv}: {1, reasonTool},
	{"ui/openurl.go", ruleChildEnv}:                       {2, reasonTool},
	{"ui/reword/reword.go", ruleChildEnv}:                 {1, reasonTool},
	{"ui/status/model_runtime.go", ruleChildEnv}:          {1, reasonTool},
	{"ui/terminalrun/terminalrun.go", ruleChildEnv}:       {3, reasonTool},
	{"ui/worktrees/kitty.go", ruleChildEnv}:               {1, reasonTool},
	{"ui/worktrees/kitty.go", ruleHomeDir}:                {1, reasonTool},
	{"ui/worktrees/lazygit.go", ruleChildEnv}:             {2, reasonTool},
	{"ui/worktrees/tmux.go", ruleChildEnv}:                {2, reasonTool},
}

func isPathVar(key string) bool {
	switch key {
	case "HOME", "PATH", "CLAUDE_CONFIG_DIR", "CODEX_HOME":
		return true
	}
	return strings.HasPrefix(key, "XDG_")
}

func selector(e ast.Expr, pkg string, names ...string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != pkg {
		return false
	}
	return slices.Contains(names, sel.Sel.Name)
}

// configEnvFuncs are config's package functions that resolve their dirs from
// the process env. Code outside config/ calls config.DirsFrom(env) instead.
var configEnvFuncs = []string{
	"DataDir", "StateDir", "UserStateDir", "UserConfigDir", "UserCacheDir",
	"FilePath", "Load", "LoadRunnerWait", "Init",
}

func isCommand(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	return ok && selector(call.Fun, "exec", "Command", "CommandContext")
}

// scan counts the banned reads in one source file, keyed by rule. path is the
// file's repo path: config/ itself may use its own functions.
func scan(path, src string) (map[string]int, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, err
	}
	inConfig := strings.HasPrefix(path, "config/")
	counts := map[string]int{}
	ast.Inspect(f, func(n ast.Node) bool {
		switch fn := n.(type) {
		case *ast.SelectorExpr:
			if !inConfig && selector(fn, "config", configEnvFuncs...) {
				counts[ruleConfigEnv]++
			}
		case *ast.FuncDecl:
			if fn.Body != nil {
				countChildren(fn.Body, counts)
			}
		case *ast.CallExpr:
			switch {
			case selector(fn.Fun, "os", "UserHomeDir"):
				counts[ruleHomeDir]++
			case selector(fn.Fun, "os", "Getenv", "LookupEnv") && len(fn.Args) == 1:
				if lit, ok := fn.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if key, err := strconv.Unquote(lit.Value); err == nil && isPathVar(key) {
						counts[ruleEnvRead]++
					}
				}
			}
		}
		return true
	})
	return counts, nil
}

// countChildren counts the exec.Command calls in one function (closures
// included) whose result is used without a `.Env =` assignment. An assignment
// of os.Environ() counts as explicit: the rule makes the choice visible, not
// the choice.
func countChildren(body *ast.BlockStmt, counts map[string]int) {
	named := map[ast.Expr]string{} // command call -> variable it is assigned to
	withEnv := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range s.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "Env" {
					if x, ok := sel.X.(*ast.Ident); ok {
						withEnv[x.Name] = true
					}
				}
				if id, ok := lhs.(*ast.Ident); ok && i < len(s.Rhs) && isCommand(s.Rhs[i]) {
					named[s.Rhs[i]] = id.Name
				}
			}
		case *ast.ValueSpec:
			for i, v := range s.Values {
				if isCommand(v) && i < len(s.Names) {
					named[v] = s.Names[i].Name
				}
			}
		}
		return true
	})
	ast.Inspect(body, func(n ast.Node) bool {
		if e, ok := n.(ast.Expr); ok && isCommand(e) {
			if name, bound := named[e]; !bound || !withEnv[name] {
				counts[ruleChildEnv]++
			}
		}
		return true
	})
}

func trackedFiles(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", "..", "ls-files", "*.go").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var files []string
	for p := range strings.FieldsSeq(string(out)) {
		if !strings.HasSuffix(p, "_test.go") {
			files = append(files, p)
		}
	}
	return files
}

func scanTree(t *testing.T) map[site]int {
	t.Helper()
	found := map[site]int{}
	for _, path := range trackedFiles(t) {
		src, err := os.ReadFile(filepath.Join("..", path))
		if os.IsNotExist(err) {
			continue // tracked but deleted in the working tree
		}
		if err != nil {
			t.Fatal(err)
		}
		counts, err := scan(path, string(src))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for rule, n := range counts {
			if n > 0 {
				found[site{path, rule}] = n
			}
		}
	}
	return found
}

// compare returns one problem per site whose count differs from its allowance.
func compare(found map[site]int, allow map[site]entry) []string {
	var problems []string
	for s, n := range found {
		if n > allow[s].count {
			problems = append(problems, fmt.Sprintf(
				"%s: rule %s allows %d, found %d. Take the value from config or the caller instead of the process env.",
				s.file, s.rule, allow[s].count, n))
		}
	}
	for s, want := range allow {
		got := found[s]
		switch {
		case got >= want.count:
		case got == 0:
			problems = append(problems, fmt.Sprintf("%s: rule %s allowlist says %d, found none; drop the entry", s.file, s.rule, want.count))
		default:
			problems = append(problems, fmt.Sprintf("%s: rule %s allowlist says %d, found %d; lower the count", s.file, s.rule, want.count, got))
		}
	}
	sort.Strings(problems)
	return problems
}

// badReasons returns one problem per entry whose reason is not permanent.
func badReasons(allow map[site]entry) []string {
	var problems []string
	for s, e := range allow {
		if !slices.Contains(reasons, e.reason) {
			problems = append(problems, fmt.Sprintf("%s: rule %s has reason %q, want one of %q", s.file, s.rule, e.reason, reasons))
		}
	}
	sort.Strings(problems)
	return problems
}

func TestNoEnvReads(t *testing.T) {
	for _, p := range compare(scanTree(t), allowed) {
		t.Error(p)
	}
	for _, p := range badReasons(allowed) {
		t.Error(p)
	}
}

func TestBadReasons_RejectsToRemove(t *testing.T) {
	a := site{"a.go", ruleHomeDir}
	if got := badReasons(map[site]entry{a: {1, reasonTool}}); len(got) != 0 {
		t.Errorf("permanent reason rejected: %v", got)
	}
	if got := badReasons(map[site]entry{a: {1, "to remove"}}); len(got) != 1 {
		t.Errorf("a %q entry gave %v, want one problem", "to remove", got)
	}
}

func TestScan(t *testing.T) {
	cases := []struct {
		name, body string
		rule       string
		want       int
	}{
		{"getenv HOME", `os.Getenv("HOME")`, ruleEnvRead, 1},
		{"lookupenv XDG", `os.LookupEnv("XDG_STATE_HOME")`, ruleEnvRead, 1},
		{"getenv other", `os.Getenv("GX_DEBUG")`, ruleEnvRead, 0},
		{"home dir", `os.UserHomeDir()`, ruleHomeDir, 1},
		{"command with env", "cmd := exec.Command(\"x\")\n\tcmd.Env = env\n\t_ = cmd.Run()", ruleChildEnv, 0},
		{"command with environ", "cmd := exec.Command(\"x\")\n\tcmd.Env = os.Environ()", ruleChildEnv, 0},
		{"command without env", "cmd := exec.Command(\"x\")\n\t_ = cmd.Run()", ruleChildEnv, 1},
		{"context command without env", "cmd := exec.CommandContext(ctx, \"x\")\n\t_ = cmd.Run()", ruleChildEnv, 1},
		{"inline", `exec.Command("x").Output()`, ruleChildEnv, 1},
		{"var decl with env", "var cmd = exec.Command(\"x\")\n\tcmd.Env = env", ruleChildEnv, 0},
		{"config call", `config.Load()`, ruleConfigEnv, 1},
		{"config function value", `x := config.StateDir; _ = x`, ruleConfigEnv, 1},
		{"method on Dirs", `cfg.Load()`, ruleConfigEnv, 0},
		{"Dirs from env", `config.DirsFrom(env).Load()`, ruleConfigEnv, 0},
		{"inside config", `config.Load()`, ruleConfigEnv, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := "package x\nfunc f() {\n\t" + c.body + "\n}\n"
			path := "x.go"
			if c.name == "inside config" {
				path = "config/x.go"
			}
			got, err := scan(path, src)
			if err != nil {
				t.Fatal(err)
			}
			if got[c.rule] != c.want {
				t.Fatalf("%s = %d, want %d (all: %v)", c.rule, got[c.rule], c.want, got)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	a := site{"a.go", ruleHomeDir}
	cases := []struct {
		name         string
		found        map[site]int
		allow        map[site]entry
		wantContains string // empty means no problem
	}{
		{"exact match", map[site]int{a: 2}, map[site]entry{a: {2, "x"}}, ""},
		{"over", map[site]int{a: 3}, map[site]entry{a: {2, "x"}}, "allows 2, found 3"},
		{"under", map[site]int{a: 1}, map[site]entry{a: {2, "x"}}, "lower the count"},
		{"gone", map[site]int{}, map[site]entry{a: {2, "x"}}, "drop the entry"},
		{"not listed", map[site]int{a: 1}, map[site]entry{}, "allows 0, found 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(compare(c.found, c.allow), "\n")
			if c.wantContains == "" {
				if got != "" {
					t.Fatalf("unexpected problems: %s", got)
				}
				return
			}
			if !strings.Contains(got, c.wantContains) {
				t.Fatalf("problems %q do not contain %q", got, c.wantContains)
			}
		})
	}
}
