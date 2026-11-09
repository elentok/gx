package brief

import (
	"fmt"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestBrief_FailuresListsEveryFailureOfTheLatestFailedRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	testutil.WriteFile(t, dir, "go.mod", "module example.com/tiny\n\ngo 1.21\n")
	var src strings.Builder
	src.WriteString("package tiny\n\nimport \"testing\"\n\n")
	const n = 7 // more than the run report shows with text
	for i := range n {
		fmt.Fprintf(&src, "func TestBad%d(t *testing.T) { t.Errorf(\"cause %d\") }\n", i, i)
	}
	testutil.WriteFile(t, dir, "a_test.go", src.String())

	runOut, code := runBriefCmd(t, dir, "go-test", "./...")
	if code != 1 || !strings.Contains(runOut, "more failures are listed by name only") {
		t.Fatalf("run exit = %d, out:\n%s", code, runOut)
	}

	out, code := runBriefCmd(t, dir, "failures")
	if code != 1 {
		t.Fatalf("failures exit = %d, out:\n%s", code, out)
	}
	if !strings.HasPrefix(out, fmt.Sprintf("%d failed", n)) || strings.Contains(out, "listed by name only") {
		t.Errorf("want all %d failures with text, got:\n%s", n, out)
	}
	for i := range n {
		if want := fmt.Sprintf("cause %d", i); !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if lastLine(out) != lastLine(runOut) {
		t.Errorf("last line = %q, want the run's log %q", lastLine(out), lastLine(runOut))
	}

	again, code := runBriefCmd(t, dir, "failures", lastLine(runOut))
	if code != 1 || again != out {
		t.Errorf("failures <log> = %d, want the same report:\n%s", code, again)
	}
}

func TestBrief_FailuresWithoutAFailedRunCannotRun(t *testing.T) {
	t.Parallel()
	out, code := runBriefCmd(t, t.TempDir(), "failures")
	if code != 2 || !strings.Contains(out, "no failed brief run") {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
}
