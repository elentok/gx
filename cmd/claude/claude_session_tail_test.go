package claude

import (
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdtest"
)

func tailFixture(t *testing.T, home string) {
	t.Helper()
	cmdtest.WriteSessionFixture(t, home, "-Users-david-dev-proj", "tail-id", []string{
		`{"type":"user","uuid":"u1","sessionId":"tail-id","message":{"role":"user","content":"first question"}}`,
		`{"type":"assistant","uuid":"a1","message":{"role":"assistant","content":[{"type":"text","text":"first answer"}]}}`,
		`{"type":"user","uuid":"u2","message":{"role":"user","content":"second question"}}`,
		`{"type":"assistant","uuid":"a2","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}`,
		`{"type":"assistant","uuid":"a3","message":{"role":"assistant","content":[{"type":"text","text":"second answer"}]}}`,
	})
}

func TestRunClaudeSessionTail_PrintsLastTurnsWithoutMetadata(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	tailFixture(t, home)

	out := &strings.Builder{}
	if err := runClaudeSessionTail(homeDeps(out, nil, home), "tail-id", 2); err != nil {
		t.Fatalf("runClaudeSessionTail: %v", err)
	}

	got := out.String()
	for _, want := range []string{"person: second question", "agent: second answer"} {
		if !strings.Contains(got, want) {
			t.Errorf("output = %q, want it to contain %q", got, want)
		}
	}
	for _, unwanted := range []string{"first", "uuid", "sessionId", "Bash", `{"`} {
		if strings.Contains(got, unwanted) {
			t.Errorf("output = %q, should not contain %q", got, unwanted)
		}
	}
}

func TestRunClaudeSessionTail_FewerTurnsThanRequested(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	tailFixture(t, home)

	out := &strings.Builder{}
	if err := runClaudeSessionTail(homeDeps(out, nil, home), "tail-id", 50); err != nil {
		t.Fatalf("runClaudeSessionTail: %v", err)
	}
	if strings.Count(out.String(), "\n") < 4 || !strings.Contains(out.String(), "first question") {
		t.Errorf("output = %q, want all four turns", out.String())
	}
}

func TestRunClaudeSessionTail_NoMatchIsError(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if err := runClaudeSessionTail(homeDeps(&strings.Builder{}, nil, home), "missing-id", 3); err == nil {
		t.Fatal("expected an error for no matching transcript, got nil")
	}
}
