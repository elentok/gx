package chat_test

import (
	"slices"
	"testing"

	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

func TestContinueAction_ByFlowAndAgent(t *testing.T) {
	t.Parallel()
	design := chat.FlowRef{Flow: tickets.FlowDesign, Key: "proj:map/03"}
	triage := chat.FlowRef{Flow: tickets.FlowTriage, Key: "proj"}
	for _, tc := range []struct {
		name  string
		ref   chat.FlowRef
		agent string
		want  []string
	}{
		{"design claude", design, "claude", []string{"/clear", "/gx-design map"}},
		{"design codex", design, "codex", []string{"/new", "$gx-design map"}},
		{"triage claude", triage, "claude", []string{"/clear", "/gx-triage-follow-ups --project proj"}},
		{"triage codex", triage, "codex", []string{"/new", "$gx-triage-follow-ups --project proj"}},
		{"no flow", chat.FlowRef{}, "claude", nil},
	} {
		if got := chat.ContinueAction(tc.ref, tc.agent); !slices.Equal(got, tc.want) {
			t.Errorf("%s = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestRunEndNext_FinalHasTheLineButNoContinue(t *testing.T) {
	t.Parallel()
	ref := chat.FlowRef{Flow: tickets.FlowTriage, Key: "proj"}
	next, ok := chat.RunEndNext(agentfake.DesignRunEnd, ref, "claude", false)
	if !ok || !slices.Equal(next.Action, []string{"/clear", "/gx-triage-follow-ups --project proj"}) {
		t.Errorf("non-final = %+v, %v; want the triage Continue", next, ok)
	}
	final, ok := chat.RunEndNext(agentfake.DesignRunEnd, ref, "claude", true)
	if !ok || final.Action != nil || final.Reason != agentfake.DesignRunEnd || !final.Derived {
		t.Errorf("final = %+v, %v; want the line only", final, ok)
	}
}
