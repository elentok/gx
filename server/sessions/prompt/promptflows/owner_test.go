package promptflows_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// startFirstPromptSession starts a session with an input box and a first
// prompt, still waiting for its ready report.
func startFirstPromptSession(t *testing.T, prompt string) (*servertest.Harness, context.Context, <-chan wire.Event, string, string) {
	t.Helper()
	return servertest.StartFakeSessionWith(t, wire.CreateSessionRequest{Prompt: prompt}, func(c *server.Config) {
		c.PromptGrace = firstPromptGrace
	}, servertest.WithEnv(agentfake.ProgramInputEnv+"=1"))
}

// attachToSession attaches to the session's holder and returns the owner
// report that comes with it.
func attachToSession(t *testing.T, dir string) (*holder.HolderAttach, holder.AttachOwner) {
	t.Helper()
	c, err := holder.DialHolder(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, err := c.Attach()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a, nextOwner(t, a)
}

// nextOwner reads attach messages until an owner report.
func nextOwner(t *testing.T, a *holder.HolderAttach) holder.AttachOwner {
	t.Helper()
	for {
		m, err := a.Next()
		if err != nil {
			t.Fatalf("waiting for an owner report: %v", err)
		}
		if m.Type == holder.StreamOwner {
			return m.Owner
		}
	}
}

func TestOwner_ClientAttachedBeforeTheFirstPromptIsBlockedUntilItIsSent(t *testing.T) {
	t.Parallel()
	h, _, _, id, conv := startFirstPromptSession(t, "fix the bug")
	dir := filepath.Join(h.SessionsDir, id)
	waitForHolder(t, dir)
	a, owner := attachToSession(t, dir)
	if !owner.GX {
		t.Fatalf("owner report at attach = %+v; want gx", owner)
	}
	if err := a.SendKey(screen.Key{Code: 'z', Text: "z"}); err != nil {
		t.Fatal(err)
	}

	report(t, conv, agentfake.Control{State: "idle"})
	waitForSubmitted(t, conv, "\"fix the bug\"\n")
	if got := nextOwner(t, a); !got.Self {
		t.Errorf("owner report after the first prompt = %+v; want the client", got)
	}
}

func TestBatch_EachNudgeEnterIsItsOwnBatch(t *testing.T) {
	t.Parallel()
	h, _, _, id, conv := startFirstPromptSession(t, "fix the bug")
	dir := filepath.Join(h.SessionsDir, id)
	waitForHolder(t, dir)
	if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionSwallowEnter}); err != nil {
		t.Fatal(err)
	}
	a, _ := attachToSession(t, dir)

	report(t, conv, agentfake.Control{State: "idle"})
	// The launch hold ends with the first prompt's batch; the swallowed Enter
	// then gets a nudge batch of its own: client, gx, client.
	for i, want := range []string{"client", "gx", "client"} {
		o := nextOwner(t, a)
		if got := o.GX; got != (want == "gx") || o.Self != (want == "client") {
			t.Fatalf("owner report %d = %+v; want %s", i, o, want)
		}
	}
	waitForSubmitted(t, conv, "\"fix the bug\"\n")
}
