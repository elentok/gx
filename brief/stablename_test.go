package brief

import (
	"bytes"
	"reflect"
	"testing"
)

// outcomeTool is a shell tool whose Parse returns a fixed Outcome.
type outcomeTool struct {
	shellTool
	outcome Outcome
}

func (o outcomeTool) Parse([]byte, int) Outcome { return o.outcome }

func runIndexLine(t *testing.T, tool Tool, dir string) IndexEntry {
	t.Helper()
	data := t.TempDir()
	var out bytes.Buffer
	RunOutcome(tool, Options{Dir: dir, DataDir: data, Out: &out})
	lines, err := ReadIndex(data)
	if err != nil || len(lines) != 1 {
		t.Fatalf("index = %v, %v", lines, err)
	}
	return lines[0]
}

func TestStableName_RulesPerTool(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, tool, dir, in, want string }{
		{"vitest drops the run dir", "vitest", "/w/webui", "/w/webui/src/x.test.tsx.a b", "src/x.test.tsx.a b"},
		{"vitest keeps a path outside the dir", "vitest", "/w/webui", "/w/other/x.test.tsx.a", "/w/other/x.test.tsx.a"},
		{"playwright drops e2e and the rule", "playwright", "/w", "e2e/a.e2e.ts.2. Epic ─────", "a.e2e.ts.2. Epic"},
		{"playwright key is unchanged", "playwright", "/w", "a.e2e.ts.2. Epic ", "a.e2e.ts.2. Epic"},
		{"go is unchanged", "go-test", "/w", "example.com/tiny.TestSlow", "example.com/tiny.TestSlow"},
	}
	for _, c := range cases {
		if got := StableName(c.tool, c.dir, c.in); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestStableName_IndexLineUsesOneNamePerTest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	abs := dir + "/src/x.test.tsx"
	fake := outcomeTool{shellTool{script: "exit 1", name: "vitest"}, Outcome{
		Units:         1,
		Failures:      []Unit{{Location: abs, Name: "boom"}},
		Tests:         []string{abs + ".boom"},
		TestDurations: map[string]int64{abs + ".boom": 5},
	}}
	e := runIndexLine(t, fake, dir)
	want := []string{"src/x.test.tsx.boom"}
	if !reflect.DeepEqual(e.Tests, want) || !reflect.DeepEqual(e.Failed, want) {
		t.Errorf("tests = %v, failed = %v, want %v", e.Tests, e.Failed, want)
	}
	if !reflect.DeepEqual(e.TestDurations, map[string]int64{"src/x.test.tsx.boom": 5}) {
		t.Errorf("durations = %v", e.TestDurations)
	}

	pw := outcomeTool{shellTool{script: "exit 1", name: "playwright"}, Outcome{
		Units:         1,
		Failures:      []Unit{{Location: "e2e/a.e2e.ts", Name: "2. Epic"}},
		Tests:         []string{"e2e/a.e2e.ts.2. Epic"},
		TestDurations: map[string]int64{"a.e2e.ts.2. Epic": 7},
	}}
	e = runIndexLine(t, pw, t.TempDir())
	want = []string{"a.e2e.ts.2. Epic"}
	if !reflect.DeepEqual(e.Tests, want) || !reflect.DeepEqual(e.Failed, want) || e.TestDurations["a.e2e.ts.2. Epic"] != 7 {
		t.Errorf("tests = %v, failed = %v, durations = %v", e.Tests, e.Failed, e.TestDurations)
	}
}

func TestStableName_SilentVitestRedIsOneFailedEntry(t *testing.T) {
	t.Parallel()
	fake := outcomeTool{shellTool{script: "exit 1", name: "vitest"}, Outcome{
		Units:     3,
		Summary:   "3 tests",
		Unhandled: "1 unhandled error\nError: boom\n at x.ts:1",
	}}
	e := runIndexLine(t, fake, t.TempDir())
	if !reflect.DeepEqual(e.Failed, []string{"vitest unhandled errors"}) {
		t.Errorf("failed = %v", e.Failed)
	}
	if e.Signature != "Error: boom" {
		t.Errorf("signature = %q", e.Signature)
	}
}
