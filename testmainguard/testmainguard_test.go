// Package testmainguard holds the repo-wide guard for test packages that use
// the servertest harness. The harness runs the test binary as the fake herdr
// and as the session holder; a package without both TestMain hooks re-runs its
// whole suite in every child, without end.
package testmainguard

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The import path is split so this file does not match its own scan.
const servertestImport = `"github.com/elentok/gx/server/` + `servertest"`

// hooks are the calls a TestMain needs; cmdtest.Main and cmdtest.MainWithGx
// make both.
var hooks = []string{"RunSessionProcesses(", "RunHelperProcess("}

var wrappers = []string{"cmdtest.Main(", "cmdtest.MainWithGx("}

// problems returns one message per package dir that imports servertest in a
// test file but has no TestMain calling both hooks. files maps a test file
// path to its source.
func problems(files map[string]string) []string {
	importers := map[string]bool{}
	mains := map[string][]string{}
	for path, src := range files {
		dir := filepath.Dir(path)
		if strings.Contains(src, servertestImport) {
			importers[dir] = true
		}
		if strings.Contains(src, "func TestMain(") {
			mains[dir] = append(mains[dir], src)
		}
	}
	var out []string
	for dir := range importers {
		ok := false
		for _, src := range mains[dir] {
			if hasHooks(src) {
				ok = true
			}
		}
		if !ok {
			out = append(out, dir+": tests import server/servertest, so its TestMain must call servertest.RunSessionProcesses() and herdrfake.RunHelperProcess() (or cmdtest.Main)")
		}
	}
	sort.Strings(out)
	return out
}

func hasHooks(src string) bool {
	for _, w := range wrappers {
		if strings.Contains(src, w) {
			return true
		}
	}
	for _, h := range hooks {
		if !strings.Contains(src, h) {
			return false
		}
	}
	return true
}

func TestEveryServertestImporterHasTheTestMainHooks(t *testing.T) {
	out, err := exec.Command("git", "-C", "..", "ls-files", "*_test.go").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	files := map[string]string{}
	for path := range strings.FieldsSeq(string(out)) {
		src, err := os.ReadFile(filepath.Join("..", path))
		if os.IsNotExist(err) {
			continue // tracked but deleted in the working tree
		}
		if err != nil {
			t.Fatal(err)
		}
		files[path] = string(src)
	}
	for _, p := range problems(files) {
		t.Error(p)
	}
}

func TestProblems(t *testing.T) {
	imp := "package a\nimport " + servertestImport + "\n"
	both := "func TestMain(m *testing.M) {\nservertest.RunSessionProcesses()\nherdrfake.RunHelperProcess()\n}\n"
	cases := []struct {
		name  string
		files map[string]string
		want  int
	}{
		{"no import", map[string]string{"a/x_test.go": "package a\n"}, 0},
		{"import without TestMain", map[string]string{"a/x_test.go": imp}, 1},
		{"both hooks in another file", map[string]string{"a/x_test.go": imp, "a/main_test.go": both}, 0},
		{"one hook only", map[string]string{"a/x_test.go": imp + "func TestMain(m *testing.M) {\nherdrfake.RunHelperProcess()\n}\n"}, 1},
		{"cmdtest.Main", map[string]string{"a/x_test.go": imp + "func TestMain(m *testing.M) { cmdtest.Main(m) }\n"}, 0},
		{"TestMain in another dir", map[string]string{"a/x_test.go": imp, "b/main_test.go": both}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := problems(c.files); len(got) != c.want {
				t.Errorf("problems = %v, want %d", got, c.want)
			}
		})
	}
}
