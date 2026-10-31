package agentlog_test

import (
	"bufio"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/elentok/gx/agentlog"
)

func renderRollout(t *testing.T, name string) []string {
	t.Helper()
	f, err := os.Open("../docs/specs/codex-support-fixtures/rollouts/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for sc.Scan() {
		out = append(out, agentlog.RenderCodex(sc.Bytes())...)
	}
	return out
}

func TestRenderCodex_PlainTurnShowsPromptAndReplyOnly(t *testing.T) {
	want := []string{"[prompt] Reply with exactly the word: pong", "[text] pong"}
	if got := renderRollout(t, "plain-turn.jsonl"); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderCodex_ApprovalShowsTheToolCall(t *testing.T) {
	got := renderRollout(t, "approval.jsonl")
	var tool string
	for _, l := range got {
		if strings.HasPrefix(l, "[tool] exec: ") {
			tool = l
		}
		if strings.Contains(l, "You are Codex") || strings.Contains(l, "AGENTS.md") {
			t.Errorf("hidden line rendered: %q", l)
		}
	}
	if !strings.Contains(tool, "touch ~/gx-codex-approval-probe.txt") {
		t.Errorf("no custom_tool_call line in %q", got)
	}
}

func TestRenderCodex_UnknownLinesRenderToNil(t *testing.T) {
	for _, raw := range []string{`not json`, `{"type":"event_msg"}`, `{"type":"response_item","payload":{"type":"reasoning"}}`} {
		if got := agentlog.RenderCodex([]byte(raw)); got != nil {
			t.Errorf("%s rendered %q", raw, got)
		}
	}
}
