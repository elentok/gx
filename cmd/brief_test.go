package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

// runBriefCmd drives `gx brief` through the root command and returns stdout
// and the exit code.
func runBriefCmd(t *testing.T, cwd string, args ...string) (string, int) {
	t.Helper()
	root := newRootCmd(deps{getwd: func() (string, error) { return cwd, nil }, env: envFor(t)})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"brief"}, args...))
	err := root.Execute()
	var ee *ExitError
	if errors.As(err, &ee) {
		return out.String(), ee.Code
	}
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	return out.String(), 0
}

// goModule writes a tiny module with passing tests and returns its dir.
func goModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testutil.WriteFile(t, dir, "go.mod", "module example.com/tiny\n\ngo 1.21\n")
	testutil.WriteFile(t, dir, "a_test.go", "package tiny\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\nfunc TestB(t *testing.T) {}\n")
	return dir
}

func lastLine(out string) string {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	return lines[len(lines)-1]
}

func TestBrief_GoTestPass(t *testing.T) {
	data := envValue(envFor(t), "XDG_DATA_HOME")
	mod := goModule(t)

	out, code := runBriefCmd(t, mod, "go-test", "./...")
	if code != 0 {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
	first := strings.SplitN(out, "\n", 2)[0]
	if !strings.HasPrefix(first, "PASS") || !strings.Contains(first, "2 tests in 1 packages") {
		t.Fatalf("first line = %q", first)
	}
	logPath := lastLine(out)
	if !strings.HasPrefix(logPath, filepath.Join(data, "gx", "brief")) {
		t.Fatalf("log path %q is outside the temp data dir", logPath)
	}

	// Seam 2: the log holds the complete raw output.
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"Action":"pass"`, `"Test":"TestA"`, `"Test":"TestB"`, "=== RUN"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("log lacks %q", want)
		}
	}

	// Seam 3: one index line with exactly the fixed fields.
	idx, err := os.ReadFile(filepath.Join(data, "gx", "brief", "index.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(idx), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("index lines = %d", len(lines))
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 9 {
		t.Errorf("index fields = %v", fields)
	}
	for _, k := range []string{"time", "tool", "args", "dir", "exit", "units", "duration_ms", "log", "tests"} {
		if _, ok := fields[k]; !ok {
			t.Errorf("index lacks %q", k)
		}
	}
	if fields["tool"] != "go-test" || fields["units"] != float64(2) || fields["log"] != logPath {
		t.Errorf("index = %v", fields)
	}
	want := []any{"example.com/tiny.TestA", "example.com/tiny.TestB"}
	if !reflect.DeepEqual(fields["tests"], want) {
		t.Errorf("tests = %v, want %v", fields["tests"], want)
	}
}

func TestBrief_IndexInRepoCarriesBranchTreeAndCheckRun(t *testing.T) {
	data := envValue(envFor(t), "XDG_DATA_HOME")
	setEnvFor(t, brief.EnvCheckRun, "run-1")
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "go.mod", "module example.com/tiny\n\ngo 1.21\n")
	testutil.WriteFile(t, repo, "a_test.go", "package tiny\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\nfunc TestB(t *testing.T) { t.Fail() }\n")

	if out, code := runBriefCmd(t, repo, "go-test", "./..."); code != 1 {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
	var e brief.IndexEntry
	if err := json.Unmarshal([]byte(indexLines(t, data)[0]), &e); err != nil {
		t.Fatal(err)
	}
	tree, err := git.TreeID(repo)
	if err != nil {
		t.Fatal(err)
	}
	if e.Branch != "main" || e.TreeID != tree || e.CheckRun != "run-1" {
		t.Errorf("branch %q, tree %q (want %q), check run %q", e.Branch, e.TreeID, tree, e.CheckRun)
	}
	if !reflect.DeepEqual(e.Tests, []string{"example.com/tiny.TestA", "example.com/tiny.TestB"}) ||
		!reflect.DeepEqual(e.Failed, []string{"example.com/tiny.TestB"}) {
		t.Errorf("tests %v, failed %v", e.Tests, e.Failed)
	}
}

func TestBrief_GoTestArgs(t *testing.T) {
	bin := t.TempDir()
	rec := filepath.Join(t.TempDir(), "args")
	script := "#!/bin/sh\npwd > '" + rec + "'\nfor a in \"$@\"; do echo \"$a\" >> '" + rec + "'; done\n"
	testutil.WriteFile(t, bin, "go", script)
	if err := os.Chmod(filepath.Join(bin, "go"), 0o755); err != nil {
		t.Fatal(err)
	}
	prependPath(t, bin)
	proj := evalSymlinks(t, t.TempDir())

	_, code := runBriefCmd(t, t.TempDir(), "-C", proj, "go-test", "-v", "-run", "TestX", "-count=1", "./a", "./b")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	raw, err := os.ReadFile(rec)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if evalSymlinks(t, got[0]) != proj {
		t.Errorf("ran in %q, want %q", got[0], proj)
	}
	want := []string{"test", "-json", "-v", "-run", "TestX", "-count=1", "./a", "./b"}
	if strings.Join(got[1:], " ") != strings.Join(want, " ") {
		t.Errorf("args = %v, want %v", got[1:], want)
	}
}

func evalSymlinks(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestBrief_NoTestsRan(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteFile(t, dir, "go.mod", "module example.com/empty\n\ngo 1.21\n")
	testutil.WriteFile(t, dir, "x.go", "package empty\n")

	out, code := runBriefCmd(t, dir, "go-test", "./...")
	if code != 0 || !strings.Contains(out, "no tests ran") {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
}

func TestBrief_GoMissingExits2(t *testing.T) {
	setEnvFor(t, "PATH", t.TempDir())

	out, code := runBriefCmd(t, t.TempDir(), "go-test")
	if code != 2 {
		t.Fatalf("exit = %d", code)
	}
	if n := len(strings.Split(strings.TrimRight(out, "\n"), "\n")); n != 1 {
		t.Errorf("want one line, got %d:\n%s", n, out)
	}
}

func TestBrief_UsageErrorsExit2WithOneLine(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"unknown tool", []string{"nosuch"}, "nosuch"},
		{"bare -C", []string{"-C", "dir"}, "missing tool"},
		{"bare -C glued", []string{"-Cdir"}, "missing tool"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runBriefCmd(t, t.TempDir(), tc.args...)
			if code != 2 {
				t.Fatalf("exit = %d, out:\n%s", code, out)
			}
			if n := len(strings.Split(strings.TrimRight(out, "\n"), "\n")); n != 1 {
				t.Errorf("want one line, got %d:\n%s", n, out)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("want %q in:\n%s", tc.want, out)
			}
		})
	}
}

func TestBrief_NoArgsAndHelpPrintHelp(t *testing.T) {
	for _, args := range [][]string{{}, {"-h"}} {
		out, code := runBriefCmd(t, t.TempDir(), args...)
		if code != 0 || !strings.Contains(out, "Exit 0 means pass") {
			t.Errorf("args %v: exit = %d, out:\n%s", args, code, out)
		}
	}
}

func TestBrief_UnwritableLogExits2(t *testing.T) {
	data := envValue(envFor(t), "XDG_DATA_HOME")
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(data, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(data, 0o700) })
	if f, err := os.Create(filepath.Join(data, "probe")); err == nil {
		f.Close()
		t.Skip("directory permissions are not enforced (running as root?)")
	}

	out, code := runBriefCmd(t, goModule(t), "go-test", "./...")
	if code != 2 {
		t.Fatalf("exit = %d", code)
	}
	if strings.Contains(out, ".log") || len(strings.Split(strings.TrimRight(out, "\n"), "\n")) != 1 {
		t.Errorf("want one line without a log path:\n%s", out)
	}
}

// failingModule has 7 failing tests in package a: one prints 30 lines, one a
// 1000-character line. Package bad does not build.
func failingModule(t *testing.T, withBuildError bool) string {
	t.Helper()
	dir := t.TempDir()
	testutil.WriteFile(t, dir, "go.mod", "module example.com/tiny\n\ngo 1.21\n")
	src := "package a\n\nimport (\n\t\"strings\"\n\t\"testing\"\n)\n\n" +
		"func TestLong(t *testing.T) {\n\tfor i := 0; i < 30; i++ {\n\t\tt.Logf(\"long line %d\", i)\n\t}\n\tt.Fail()\n}\n" +
		"func TestWide(t *testing.T) { t.Log(strings.Repeat(\"x\", 1000)); t.Fail() }\n"
	for _, n := range []string{"C", "D", "E", "F", "G"} {
		src += "func Test" + n + "(t *testing.T) { t.Error(\"boom " + n + "\") }\n"
	}
	for _, sub := range []string{"a", "bad"} {
		if err := os.Mkdir(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	testutil.WriteFile(t, dir, "a/a_test.go", src)
	if withBuildError {
		testutil.WriteFile(t, dir, "bad/b.go", "package bad\n\nfunc F() int { return \"x\" }\n")
		testutil.WriteFile(t, dir, "bad/b_test.go", "package bad\n\nimport \"testing\"\n\nfunc TestB(t *testing.T) {}\n")
	}
	return dir
}

func TestBrief_GoTestFailures(t *testing.T) {
	data := envValue(envFor(t), "XDG_DATA_HOME")
	mod := failingModule(t, false)

	out, code := runBriefCmd(t, mod, "go-test", "-count=1", "./...")
	if code != 1 {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}

	// Seam 1: limits, order and the cut notice.
	if !strings.Contains(out, "example.com/tiny/a\n") {
		t.Errorf("no package heading:\n%s", out)
	}
	if !strings.Contains(out, "long line 9") || strings.Contains(out, "long line 10") {
		t.Errorf("TestLong is not limited to 10 lines:\n%s", out)
	}
	if !strings.Contains(out, "more lines cut") {
		t.Errorf("no cut notice for lines:\n%s", out)
	}
	if strings.Contains(out, strings.Repeat("x", 301)) || !strings.Contains(out, "x[") || !strings.Contains(out, "more chars cut, search the log]") {
		t.Errorf("wide line not cut at 300:\n%s", out)
	}
	// Failures come in source order: Long, Wide, C, D, E are shown in full
	// (C, D, E have "boom" text); F and G are by name only.
	if n := strings.Count(out, "boom "); n != 3 {
		t.Errorf("units with boom text = %d, want 3 (7 failing, 5 in full, 2 by name):\n%s", n, out)
	}
	if !strings.Contains(out, "2 more failures are listed by name only") {
		t.Errorf("by-name notice missing:\n%s", out)
	}

	// Seam 3: the re-run line covers all 7 failing tests, anchored.
	var rerun string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "re-run only the failing tests: ") {
			rerun = l
		}
	}
	for _, want := range []string{"gx brief go-test -count=1 ./... -run '^(", "TestLong", "TestWide", "TestC", "TestG", ")$'"} {
		if !strings.Contains(rerun, want) {
			t.Errorf("re-run line lacks %q: %q", want, rerun)
		}
	}

	// Seam 5: the log path is last, holds the cut text, and the index counts units.
	logPath := lastLine(out)
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "long line 29") || !strings.Contains(string(raw), strings.Repeat("x", 1000)) {
		t.Errorf("log lacks the cut text")
	}
	idx, err := os.ReadFile(filepath.Join(data, "gx", "brief", "index.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(idx))), &fields); err != nil {
		t.Fatal(err)
	}
	if fields["exit"] != float64(1) || fields["units"] != float64(7) {
		t.Errorf("index = %v", fields)
	}
}

// Seam 2: a package that does not build is one unit named by the package, and
// the other package's failures are still reported.
func TestBrief_GoTestBuildErrorIsOneUnit(t *testing.T) {
	out, code := runBriefCmd(t, failingModule(t, true), "go-test", "-count=1", "./...")
	if code != 1 {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
	if !strings.Contains(out, "example.com/tiny/bad\n  build failed\n") || !strings.Contains(out, "cannot use") {
		t.Errorf("build unit missing:\n%s", out)
	}
	if !strings.Contains(out, "TestLong") || !strings.Contains(out, "8 failed") {
		t.Errorf("other package's failures lost:\n%s", out)
	}
	if !strings.Contains(out, "re-run only the failing tests: ") || strings.Contains(out, "|TestB)") || strings.Contains(out, "(TestB|") {
		t.Errorf("re-run line should cover tests only:\n%s", out)
	}
}

func TestBrief_GoTestRerunKeepsArgsAndC(t *testing.T) {
	mod := failingModule(t, false)

	out, code := runBriefCmd(t, t.TempDir(), "-C", mod, "go-test", "-run", "TestC|TestD", "-count=1", "./a")
	if code != 1 {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
	want := "re-run only the failing tests: gx brief -C " + mod + " go-test -count=1 ./a -run '^(TestC|TestD)$'"
	if !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
}

func TestBrief_GoTestBuildErrorOnlyHasNoRerun(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteFile(t, dir, "go.mod", "module example.com/tiny\n\ngo 1.21\n")
	testutil.WriteFile(t, dir, "b.go", "package bad\n\nfunc F() int { return \"x\" }\n")
	testutil.WriteFile(t, dir, "b_test.go", "package bad\n\nimport \"testing\"\n\nfunc TestB(t *testing.T) {}\n")

	out, code := runBriefCmd(t, dir, "go-test", "./...")
	if code != 1 || strings.Contains(out, "re-run") || !strings.Contains(out, "build failed") {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
}

func TestBrief_GoTestPanicBeforeAnyTest(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteFile(t, dir, "go.mod", "module example.com/tiny\n\ngo 1.21\n")
	testutil.WriteFile(t, dir, "p_test.go", "package p\n\nimport \"testing\"\n\nfunc TestMain(m *testing.M) { panic(\"boom\") }\nfunc TestP(t *testing.T) {}\n")

	out, code := runBriefCmd(t, dir, "go-test", "./...")
	if code != 1 {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
	if !strings.Contains(out, "panic: boom") || !strings.Contains(out, "package failed") ||
		strings.Contains(out, `{"Time"`) || strings.Contains(out, "re-run") {
		t.Errorf("unexpected report:\n%s", out)
	}
	if _, err := os.Stat(lastLine(out)); err != nil {
		t.Errorf("last line is not the log: %v", err)
	}
}
