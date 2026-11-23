// Package deliverguard holds the repo-wide guard against a second "type and
// press Enter" implementation. Text reaches a session through agentrunner/deliver
// only; any other non-test code that types, pastes or presses submit must sit in
// the allowlist below, with a reason.
//
// The scan is syntactic, plus type information for one case: a SendKeys call whose
// argument is not a literal. Only the packages holding such a call are type-checked
// (with go/types and the source importer), so a normal run stays fast while a
// `const submit = "\r"` in another file or package is still resolved.
package deliverguard

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil/repofiles"
)

const submitKey = "\r"

// allowed maps a file, or a directory ending in "/", to why it may type, paste or
// press submit. An entry that covers no flagged call fails: the list only shrinks.
var allowed = map[string]string{
	"nativerunner/holder/":           "the holder client and its server side",
	"agentrunner/deliver/":           "the one production Target over the holder client",
	"claudedoctor/imagepaste.go":     "the doctor's image-paste diagnostic, not a prompt send",
	"server/sessions/prompt/keys.go": "the person-input relay: a person's typed text through the web",
	"server/sessions/webattach/":     "the web attach relay",
	"ui/agents/terminal/attach.go":   "the TUI attach relay",
	"agentrunner/screen/screen.go":   "the emulator's own Paste, not a holder call",
	"harness/typing.go":              "the harness typing object; ClearKey delegates and sends nothing",
	"nativerunner/pty_relay.go":      "the PTY person-input relay clears a lone i the vim probe typed",
}

// One file set and importer for the whole run: the source importer caches the
// packages it has type-checked, so later packages import for free.
var (
	fset           = token.NewFileSet()
	sharedImporter = importer.ForCompiler(fset, "source", nil)
)

var typeCalls = map[string]bool{"TypeText": true, "TypeTextBy": true, "TypePersonTextBy": true, "Paste": true}

var keyCalls = map[string]bool{"SubmitKey": true, "ClearKey": true}

const selfFile = "deliverguard/deliverguard_test.go"

func skipped(p string) bool {
	return strings.HasSuffix(p, "_test.go") || p == selfFile || strings.HasPrefix(p, "testutil/") || strings.Contains(p, "/testutil/")
}

func calleeName(fun ast.Expr) (name string, isSel bool) {
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		return f.Sel.Name, true
	case *ast.Ident:
		return f.Name, false
	}
	return "", false
}

// scan reports the flagged calls in one source file, as a package of one file.
func scan(p, src string) ([]string, error) {
	got, err := scanFiles(map[string]string{p: src})
	return got[p], err
}

// scanFiles parses the files of one package together and returns the flagged
// calls per file path. Skipped paths are not read.
func scanFiles(files map[string]string) (map[string][]string, error) {
	parsed := map[string]*ast.File{}
	var order []string
	for p, src := range files {
		if skipped(p) {
			continue
		}
		f, err := parser.ParseFile(fset, p, src, 0)
		if err != nil {
			return nil, err
		}
		parsed[p] = f
		order = append(order, p)
	}
	sort.Strings(order)
	needTypes := false
	for _, p := range order {
		ast.Inspect(parsed[p], func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok && isSendKeys(call) {
				if _, lit := call.Args[0].(*ast.BasicLit); !lit {
					needTypes = true
				}
			}
			return true
		})
	}
	var info *types.Info
	if needTypes {
		info = &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
		list := make([]*ast.File, 0, len(order))
		for _, p := range order {
			list = append(list, parsed[p])
		}
		// Errors (build tags, cgo, a missing import) must not stop the scan:
		// the constants we need resolve without them.
		conf := types.Config{Importer: sharedImporter, Error: func(error) {}}
		_, _ = conf.Check("x", fset, list, info)
	}
	out := map[string][]string{}
	for _, p := range order {
		ast.Inspect(parsed[p], func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, isSel := calleeName(call.Fun)
			switch {
			case isSel && typeCalls[name], keyCalls[name]:
				out[p] = append(out[p], name)
			case isSendKeys(call) && sendsSubmit(call.Args[0], info):
				out[p] = append(out[p], `SendKeys("\r")`)
			}
			return true
		})
	}
	return out, nil
}

func isSendKeys(call *ast.CallExpr) bool {
	name, isSel := calleeName(call.Fun)
	return isSel && name == "SendKeys" && len(call.Args) == 1
}

func sendsSubmit(arg ast.Expr, info *types.Info) bool {
	if lit, ok := arg.(*ast.BasicLit); ok {
		s, err := strconv.Unquote(lit.Value)
		return lit.Kind == token.STRING && err == nil && s == submitKey
	}
	if info == nil {
		return false
	}
	tv, ok := info.Types[arg]
	return ok && tv.Value != nil && tv.Value.Kind() == constant.String && constant.StringVal(tv.Value) == submitKey
}

func covers(entry, file string) bool {
	if strings.HasSuffix(entry, "/") {
		return strings.HasPrefix(file, entry)
	}
	return entry == file
}

const allowlistHint = " The allowlist is var allowed in deliverguard/deliverguard_test.go. Send text through agentrunner/deliver instead."

// compare returns one problem per unlisted file with a flagged call, per stale
// entry and per entry with an empty reason.
func compare(found map[string][]string, allow map[string]string) []string {
	var problems []string
	for file, calls := range found {
		listed := false
		for e := range allow {
			listed = listed || covers(e, file)
		}
		if !listed {
			problems = append(problems, fmt.Sprintf("%s: calls %s outside the allowlist."+allowlistHint, file, strings.Join(calls, ", ")))
		}
	}
	for e, reason := range allow {
		if strings.TrimSpace(reason) == "" {
			problems = append(problems, fmt.Sprintf("%s: allowlist entry has no reason."+allowlistHint, e))
		}
		used := false
		for file := range found {
			used = used || covers(e, file)
		}
		if !used {
			problems = append(problems, fmt.Sprintf("%s: stale allowlist entry holds no flagged call; drop it."+allowlistHint, e))
		}
	}
	sort.Strings(problems)
	return problems
}

func scanTree(t *testing.T) map[string][]string {
	t.Helper()
	byDir := map[string]map[string]string{}
	for _, p := range repofiles.List(t, "*.go") {
		if skipped(p) {
			continue
		}
		src, err := os.ReadFile(filepath.Join("..", p))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		dir := path.Dir(p)
		if byDir[dir] == nil {
			byDir[dir] = map[string]string{}
		}
		byDir[dir][p] = string(src)
	}
	found := map[string][]string{}
	for _, files := range byDir {
		got, err := scanFiles(files)
		if err != nil {
			t.Fatal(err)
		}
		maps.Copy(found, got)
	}
	return found
}

func TestNoTypingOutsideDeliver(t *testing.T) {
	for _, p := range compare(scanTree(t), allowed) {
		t.Error(p)
	}
}

func TestScan(t *testing.T) {
	cases := []struct {
		name, body, decl string
		want             int
	}{
		{"TypeText", `h.TypeText("x")`, "", 1},
		{"TypeTextBy", `h.TypeTextBy("x", 1)`, "", 1},
		{"TypePersonTextBy", `h.TypePersonTextBy("x")`, "", 1},
		{"Paste", `h.Paste("x")`, "", 1},
		{"SubmitKey", `_ = t.SubmitKey(true)`, "", 1},
		{"ClearKey", `_ = t.ClearKey()`, "", 1},
		{"SendKeys literal CR", `h.SendKeys("\r")`, "", 1},
		{"SendKeys named constant", `h.SendKeys(submit)`, `const submit = "\r"`, 1},
		{"SendKeys escape", `h.SendKeys("\x1b")`, "", 0},
		{"SendKeys letter", `h.SendKeys("i")`, "", 0},
		{"SendKeys variable", `key := "\r"; h.SendKeys(key)`, "", 0},
		{"SendKeys other constant", `h.SendKeys(esc)`, `const esc = "\x1b"`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := "package x\n" + c.decl + "\nfunc f(h, t any) {\n\t" + c.body + "\n}\n"
			got, err := scan("x/x.go", src)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != c.want {
				t.Fatalf("found %v, want %d", got, c.want)
			}
		})
	}
}

func TestScan_ConstantInAnotherFile(t *testing.T) {
	got, err := scanFiles(map[string]string{
		"x/a.go": "package x\nconst submit = \"\\r\"\n",
		"x/b.go": "package x\nfunc f(h any) { h.SendKeys(submit) }\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got["x/b.go"]) != 1 {
		t.Fatalf("found %v, want one call in x/b.go", got)
	}
}

func TestScan_SkipsTestsAndTestutil(t *testing.T) {
	for _, p := range []string{"x/x_test.go", "testutil/agentfake/harness.go"} {
		got, err := scan(p, "package x\nfunc f(h any) { h.Paste(1) }\n")
		if err != nil || len(got) != 0 {
			t.Errorf("%s: got %v, %v; want skipped", p, got, err)
		}
	}
}

func TestCompare(t *testing.T) {
	found := map[string][]string{"a/b.go": {"Paste"}}
	cases := []struct {
		name  string
		found map[string][]string
		allow map[string]string
		want  string // empty means no problem
	}{
		{"listed file", found, map[string]string{"a/b.go": "r"}, ""},
		{"listed dir", found, map[string]string{"a/": "r"}, ""},
		{"unlisted", found, map[string]string{"c/": "r"}, "a/b.go: calls Paste outside the allowlist"},
		{"stale", found, map[string]string{"a/": "r", "c/": "r"}, "c/: stale"},
		{"empty reason", found, map[string]string{"a/": " "}, "a/: allowlist entry has no reason"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(compare(c.found, c.allow), "\n")
			if c.want == "" {
				if got != "" {
					t.Fatalf("unexpected problems: %s", got)
				}
				return
			}
			if !strings.Contains(got, c.want) || !strings.Contains(got, "var allowed in deliverguard/deliverguard_test.go") {
				t.Fatalf("problems %q do not contain %q and the hint", got, c.want)
			}
		})
	}
}
