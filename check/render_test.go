package check

import (
	"strings"
	"testing"
)

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
