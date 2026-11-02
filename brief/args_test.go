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
	if got := (Vitest{}).NarrowArgs(nil, "a"); got != nil {
		t.Errorf("Vitest.NarrowArgs = %v, want nil", got)
	}
}
