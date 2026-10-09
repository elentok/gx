package server_test

import (
	"context"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/runnerfake"
)

func startAgentVerbs(t *testing.T) (*servertest.Harness, *runnerfake.Runner) {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", t.TempDir())
	fake := runnerfake.NewRunner()
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Runner = fake })
	return h, fake
}

func TestAgentVerbs_ReachTheLiveIteration(t *testing.T) {
	h, fake := startAgentVerbs(t)
	const label = "epic-a-iter-01"
	if _, err := fake.Start(agentrunner.StartOptions{Label: label, Epic: "epic-a", Cwd: t.TempDir(), Kind: "claude"}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if res, err := h.Client.AgentPrompt(ctx, "proj:epic-a/01", "keep going"); err != nil || res.Refused {
		t.Fatalf("prompt = %+v, %v", res, err)
	}
	if got := fake.Prompts(label); len(got) != 1 || got[len(got)-1] != "keep going" {
		t.Errorf("prompts = %v", got)
	}
	if res, err := h.Client.AgentInterrupt(ctx, "proj:epic-a/01"); err != nil || res.Refused {
		t.Fatalf("interrupt = %+v, %v", res, err)
	}

	// Nothing is blocked yet, so there is nothing to answer.
	res, err := h.Client.AgentAnswer(ctx, "proj:epic-a/01", "allow", "")
	if err != nil || !res.Refused || res.Reason != server.ReasonNotBlocked {
		t.Fatalf("answer while idle = %+v, %v; want refusal %s", res, err, server.ReasonNotBlocked)
	}

	fake.SetState(label, agentrunner.StateBlocked, "permission to use Bash")
	if res, err := h.Client.AgentPrompt(ctx, "proj:epic-a/01", "more"); err != nil || !res.Refused || res.Reason != server.ReasonAgentNotReady {
		t.Errorf("prompt while blocked = %+v, %v; want refusal %s", res, err, server.ReasonAgentNotReady)
	}
	if res, err := h.Client.AgentAnswer(ctx, "proj:epic-a/01", "deny", "no"); err != nil || res.Refused {
		t.Fatalf("answer = %+v, %v", res, err)
	}
	got := fake.Answers(label)
	if len(got) != 1 || got[0].Decision != agentrunner.DecisionDeny || got[0].Text != "no" {
		t.Errorf("answers = %+v", got)
	}
}

func TestAgentVerbs_Refusals(t *testing.T) {
	h, _ := startAgentVerbs(t)
	ctx := context.Background()
	for _, c := range []struct {
		name   string
		do     func() (server.QueueResult, error)
		reason string
	}{
		{"prompt without text", func() (server.QueueResult, error) { return h.Client.AgentPrompt(ctx, "proj:epic-a/01", " ") }, server.ReasonTextRequired},
		{"prompt unknown ticket", func() (server.QueueResult, error) { return h.Client.AgentPrompt(ctx, "proj:epic-a/77", "hi") }, server.ReasonUnknownTicket},
		{"prompt not live", func() (server.QueueResult, error) { return h.Client.AgentPrompt(ctx, "proj:epic-a/01", "hi") }, server.ReasonIterationNotLive},
		{"interrupt not live", func() (server.QueueResult, error) { return h.Client.AgentInterrupt(ctx, "proj:epic-a/01") }, server.ReasonIterationNotLive},
		{"answer not live", func() (server.QueueResult, error) { return h.Client.AgentAnswer(ctx, "proj:epic-a/01", "allow", "") }, server.ReasonIterationNotLive},
		{"answer bad decision", func() (server.QueueResult, error) { return h.Client.AgentAnswer(ctx, "proj:epic-a/01", "maybe", "") }, server.ReasonBadDecision},
	} {
		res, err := c.do()
		if err != nil || !res.Refused || res.Reason != c.reason {
			t.Errorf("%s: %+v, %v; want refusal %s", c.name, res, err, c.reason)
		}
	}
}
