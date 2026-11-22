package check

import (
	"strings"
	"testing"
)

func TestRender_FailingPlaywrightFlowsOfACheckLogAreNamedInTheSummary(t *testing.T) {
	t.Parallel()
	names := checkLogFailures("../brief/testdata/check-test.log")
	out := Result{Exit: ExitFail, Kinds: []KindResult{{Kind: "test", Status: StatusFail, Log: "/tmp/log", Failures: names}}}.Render()
	for _, want := range []string{"e2e/codex.e2e.ts.117. Default harness setting", "e2e/united.e2e.ts.18. Joined card"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary lacks %q:\n%s", want, out)
		}
	}
}

func TestRender_ManyFailuresNameTheDominantError(t *testing.T) {
	t.Parallel()
	k := KindResult{Kind: "test", Status: StatusFail, Log: "/tmp/log"}
	const cause = "fork/exec /bin/x: resource temporarily unavailable"
	for i := range 72 {
		k.Failures = append(k.Failures, "pkg.T"+strings.Repeat("x", i))
		k.Causes = append(k.Causes, cause)
	}
	k.Causes[0] = "other"
	out := Result{Exit: ExitFail, Kinds: []KindResult{k}}.Render()
	if want := "most common error (71 of 72 failures): " + cause + "\n"; !strings.Contains(out, want) {
		t.Errorf("summary lacks %q:\n%s", want, out)
	}
	if n := strings.Count(out, cause); n != 1 {
		t.Errorf("cause printed %d times, want once", n)
	}
}

func TestRender_LongGuardLinePrintsInFull(t *testing.T) {
	t.Parallel()
	line := "sizeguard: " + strings.Repeat("g", 400)
	out := Result{Exit: ExitFail, Kinds: []KindResult{{Kind: "guards", Status: StatusFail, Log: "/tmp/log", Tail: []string{line}}}}.Render()
	if !strings.Contains(out, "  "+line+"\n") || strings.Contains(out, "cut") {
		t.Errorf("guard line cut:\n%s", out)
	}
}

func TestRender_FailureTailDropsLogNoiseAndStartsAtTheAssertion(t *testing.T) {
	t.Parallel()
	r := Result{Exit: ExitFail, Kinds: []KindResult{{
		Kind: "test", Status: StatusFail, Log: "/tmp/log",
		Tail: []string{
			`{"level":"info","msg":"request"}`,
			`{"level":"info","msg":"request"}`,
			"a_test.go:9: got 1, want 2",
			"ctx 1", "ctx 2", "ctx 3", "ctx 4",
		},
	}}}
	out := r.Render()
	if strings.Contains(out, `{"level"`) || strings.Contains(out, "ctx 4") {
		t.Errorf("tail not trimmed:\n%s", out)
	}
	if !strings.Contains(out, "\n  a_test.go:9: got 1, want 2\n  ctx 1\n  ctx 2\n  ctx 3\n") {
		t.Errorf("tail lacks the assertion and its context:\n%s", out)
	}
}
