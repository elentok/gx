package harness_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/harness"
)

func TestSkillCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		harness, skill, arg, want string
	}{
		{"claude", "gx-design", "x", "/gx-design x"},
		{"codex", "gx-design", "x", "$gx-design x"},
		{"fakeagent", "gx-design", "x", "/gx-design x"},
		{"", "gx-design", "x", "/gx-design x"},
		{"claude", "gx-resolving-merge-conflicts", "", "/gx-resolving-merge-conflicts"},
		{"codex", "gx-resolving-merge-conflicts", "", "$gx-resolving-merge-conflicts"},
	}
	for _, tc := range tests {
		if got := harness.SkillCommand(tc.harness, tc.skill, tc.arg); got != tc.want {
			t.Errorf("SkillCommand(%q, %q, %q) = %q, want %q", tc.harness, tc.skill, tc.arg, got, tc.want)
		}
	}
}

// literalExemptions are the non-test string literals that start with a skill
// prefix and are not typed into an agent, keyed by file (slash-separated, from
// the repo root).
var literalExemptions = map[string][]string{
	// A URL path of the root certificate download.
	"server/webroot.go": {"/gx-root.mobileconfig", "/gx-root.cer"},
	// The prefix gx looks for when it reads a command out of a Codex transcript.
	"codexsession/chat.go": {"$gx-"},
}

// skippedDirs hold code that may name a skill: the fake agents recognise both
// prefixes on purpose, and the rest is not Go source of the product.
var skippedDirs = map[string]bool{
	"testutil": true, "node_modules": true, "webui": true, ".git": true, ".scratch": true,
}

// A hardcoded skill literal skips the one function that picks the prefix per
// harness, so a Codex session would be typed a Claude command.
func TestNoHardcodedSkillLiteralInNonTestGoCode(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skippedDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Errorf("parse %s: %v", rel, perr)
			return nil
		}
		checked++
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			val, uerr := strconv.Unquote(lit.Value)
			if uerr != nil || !(strings.HasPrefix(val, "/gx-") || strings.HasPrefix(val, "$gx-")) {
				return true
			}
			for _, ok := range literalExemptions[rel] {
				if val == ok {
					return true
				}
			}
			t.Errorf("%s: hardcoded skill literal %s; build it with harness.SkillCommand", fset.Position(lit.Pos()), lit.Value)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 50 {
		t.Errorf("walked only %d Go files; the repo root is wrong", checked)
	}
}
