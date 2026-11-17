package sessions

import (
	"testing"

	"github.com/elentok/gx/commandlist"
)

func TestDefaultModel(t *testing.T) {
	t.Parallel()
	claude := []commandlist.Model{{Value: "opus", ID: "claude-opus-4-5"}, {Value: "default", ID: "claude-sonnet-4-5"}}
	if id, eff := defaultModel(claude, false); id != "claude-sonnet-4-5" || eff != "" {
		t.Fatalf("claude = %q, %q", id, eff)
	}
	codex := []commandlist.Model{{Value: "a", ID: "gpt-a"}, {Value: "b", ID: "gpt-b", IsDefault: true, DefaultEffort: "medium"}}
	if id, eff := defaultModel(codex, true); id != "gpt-b" || eff != "medium" {
		t.Fatalf("codex = %q, %q", id, eff)
	}
	if id, eff := defaultModel(nil, false); id != "" || eff != "" {
		t.Fatalf("none = %q, %q", id, eff)
	}
}
