package brief

import (
	"slices"
	"testing"
)

func TestStripFlag(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		takesValue bool
		flags      []string
		want       []string
		wantFound  bool
	}{
		{"bare boolean", []string{"-v", "./..."}, false, []string{"v"}, []string{"./..."}, true},
		{"boolean with value", []string{"-json=true", "./..."}, false, []string{"json"}, []string{"./..."}, true},
		{"value as next arg", []string{"--run", "TestX", "./..."}, true, []string{"run"}, []string{"./..."}, true},
		{"value after equals", []string{"-run=TestX", "./..."}, true, []string{"run"}, []string{"./..."}, true},
		{"either alias", []string{"-t", "a", "--testNamePattern=b", "x"}, true, []string{"t", "testNamePattern"}, []string{"x"}, true},
		{"similar names stay", []string{"-vet=off", "-count=1"}, false, []string{"v"}, []string{"-vet=off", "-count=1"}, false},
		{"positional stays", []string{"v"}, false, []string{"v"}, []string{"v"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := stripFlag(tt.args, tt.takesValue, tt.flags...)
			if !slices.Equal(got, tt.want) || found != tt.wantFound {
				t.Fatalf("stripFlag() = %v, %v; want %v, %v", got, found, tt.want, tt.wantFound)
			}
		})
	}
}

func TestSplitArgs(t *testing.T) {
	rest, tail := splitArgs([]string{"./...", "-args", "-v"})
	if !slices.Equal(rest, []string{"./..."}) || !slices.Equal(tail, []string{"-args", "-v"}) {
		t.Fatalf("splitArgs() = %v, %v", rest, tail)
	}
	rest, tail = splitArgs([]string{"./..."})
	if !slices.Equal(rest, []string{"./..."}) || tail != nil {
		t.Fatalf("splitArgs() without -args = %v, %v", rest, tail)
	}
}

func TestGoTestNarrowArgs(t *testing.T) {
	for _, c := range []struct {
		name string
		args []string
		test string
		want []string
	}{
		{"plain", []string{"./..."}, "TestA", []string{"./...", "-run", "^TestA$"}},
		{"subtest", []string{"./..."}, "TestA/sub_case", []string{"./...", "-run", "^TestA$/^sub_case$"}},
		{"replaces -run, keeps -args tail", []string{"-run", "X", "./a", "-args", "-v"}, "TestA", []string{"./a", "-run", "^TestA$", "-args", "-v"}},
		{"quotes metacharacters", nil, "TestA/a.b", []string{"-run", `^TestA$/^a\.b$`}},
	} {
		if got := (GoTest{}).NarrowArgs(c.args, c.test); !slices.Equal(got, c.want) {
			t.Errorf("%s: NarrowArgs = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSplitTestName(t *testing.T) {
	for _, c := range []struct {
		tool      Tool
		name      string
		loc, test string
	}{
		{GoTest{}, "github.com/a/b.c.TestX/sub", "github.com/a/b.c", "TestX/sub"},
		{GoTest{}, "no tests here", "", ""},
		{Vitest{}, "src/a.test.ts.suite a", "src/a.test.ts", "suite a"},
		{Playwright{}, "disk.e2e.ts.1. Send", "disk.e2e.ts", "1. Send"},
		{Tsc{}, "src/a.ts.x", "", ""},
	} {
		if loc, test := c.tool.SplitTestName(c.name); loc != c.loc || test != c.test {
			t.Errorf("%s %q: got (%q, %q), want (%q, %q)", c.tool.Name(), c.name, loc, test, c.loc, c.test)
		}
	}
}

func TestToolByName(t *testing.T) {
	for _, name := range []string{"go-test", "go-build", "go-vet", "vitest", "tsc", "playwright"} {
		if tool, ok := ToolByName(name); !ok || tool.Name() != name {
			t.Errorf("ToolByName(%q) = %v, %v", name, tool, ok)
		}
	}
	if _, ok := ToolByName("go"); ok {
		t.Error("ToolByName(go) found a tool; only brief names resolve")
	}
}

func TestNodeNarrowArgs(t *testing.T) {
	for _, c := range []struct {
		name string
		tool Tool
		args []string
		test string
		want []string
	}{
		{"vitest anchors", Vitest{}, []string{"src"}, "suite a.b", []string{"src", "-t", `^suite a\.b$`}},
		{"vitest replaces -t", Vitest{}, []string{"-t", "x", "src"}, "a", []string{"src", "-t", "^a$"}},
		{"playwright full title, no ^", Playwright{}, []string{"disk.e2e.ts"}, "153. Send (x)", []string{"disk.e2e.ts", "-g", `› 153\. Send \(x\)$`}},
		{"playwright replaces -g", Playwright{}, []string{"--grep=foo", "f.ts"}, "a", []string{"f.ts", "-g", "› a$"}},
		{"playwright drops flow numbers", Playwright{}, []string{"disk.e2e.ts", "69", "70"}, "69. A", []string{"disk.e2e.ts", "-g", `› 69\. A$`}},
	} {
		if got := c.tool.NarrowArgs(c.args, c.test); !slices.Equal(got, c.want) {
			t.Errorf("%s: NarrowArgs = %v, want %v", c.name, got, c.want)
		}
	}
}
