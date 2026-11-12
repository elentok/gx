package brief

import (
	"slices"
	"testing"
)

func TestGoTestRerunArgs_KeepsFlagsReplacesPackagesWithFailingOnes(t *testing.T) {
	t.Parallel()
	failures := []Unit{
		{Location: "ex.com/b", Name: "TestB/sub", Kind: KindTest},
		{Location: "ex.com/b", Name: "TestB2", Kind: KindTest},
	}
	args := []string{"-count", "1", "-race", "./a", "./b", "./c", "-run", "X", "-args", "-flag"}
	got := GoTest{}.RerunArgs(args, failures)
	want := []string{"-count", "1", "-race", "ex.com/b", "-run", "^(TestB|TestB2)$", "-args", "-flag"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestVitestRerunArgs_RelatedBecomesFailingFiles(t *testing.T) {
	t.Parallel()
	failures := []Unit{
		{Location: "src/page.test.ts", Name: "a b", Kind: KindTest},
		{Location: "src/page.test.ts", Name: "c", Kind: KindTest},
	}
	got := Vitest{}.RerunArgs([]string{"related", "src/connection.ts", "-t", "old"}, failures)
	want := []string{"src/page.test.ts", "-t", "^(a b|c)$"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
