package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

// runBriefCmd drives `gx brief` through the root command and returns stdout
// and the exit code.
func runBriefCmd(t *testing.T, cwd string, args ...string) (string, int) {
	t.Helper()
	root := newRootCmd(deps{getwd: func() (string, error) { return cwd, nil }})
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
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
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
	if len(fields) != 8 {
		t.Errorf("index fields = %v", fields)
	}
	for _, k := range []string{"time", "tool", "args", "dir", "exit", "units", "duration_ms", "log"} {
		if _, ok := fields[k]; !ok {
			t.Errorf("index lacks %q", k)
		}
	}
	if fields["tool"] != "go-test" || fields["units"] != float64(2) || fields["log"] != logPath {
		t.Errorf("index = %v", fields)
	}
}

func TestBrief_GoTestArgs(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	bin := t.TempDir()
	rec := filepath.Join(t.TempDir(), "args")
	script := "#!/bin/sh\npwd > '" + rec + "'\nfor a in \"$@\"; do echo \"$a\" >> '" + rec + "'; done\n"
	testutil.WriteFile(t, bin, "go", script)
	if err := os.Chmod(filepath.Join(bin, "go"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
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
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	testutil.WriteFile(t, dir, "go.mod", "module example.com/empty\n\ngo 1.21\n")
	testutil.WriteFile(t, dir, "x.go", "package empty\n")

	out, code := runBriefCmd(t, dir, "go-test", "./...")
	if code != 0 || !strings.Contains(out, "no tests ran") {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
}

func TestBrief_GoMissingExits2(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	out, code := runBriefCmd(t, t.TempDir(), "go-test")
	if code != 2 {
		t.Fatalf("exit = %d", code)
	}
	if n := len(strings.Split(strings.TrimRight(out, "\n"), "\n")); n != 1 {
		t.Errorf("want one line, got %d:\n%s", n, out)
	}
}

func TestBrief_UnwritableLogExits2(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
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
