package harness_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

// An interactive session with no harness starts on the Default harness; an
// explicit harness wins and an unregistered default is refused.
func TestCreateSession_EmptyHarnessFollowsDefault(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder(), servertest.WithDefaultHarness(string(agentfake.HarnessKind)))
	res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir()})
	if err != nil || res.Refused || res.Session == nil || res.Session.Harness != string(agentfake.HarnessKind) {
		t.Fatalf("default create = %+v, %v; want the default harness", res, err)
	}
	res, err = h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: "no-such-harness"})
	if err != nil || !res.Refused || res.Reason != wire.ReasonSessionUnknownHarness {
		t.Fatalf("explicit create = %+v, %v; want the explicit harness refused", res, err)
	}
}

func TestCreateSession_ProjectOverrideBeatsDefault(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	if err := os.MkdirAll(filepath.Join(store, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := servertest.WriteProjectRepo(store, "proj", testutil.TempRepo(t), `,"agents":{"default-harness":"no-such-harness"}`); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), servertest.WithDefaultHarness(string(agentfake.HarnessKind)))
	res, err := h.Client.Sessions.Create(context.Background(), wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir()})
	if err != nil || !res.Refused || res.Reason != wire.ReasonSessionUnknownHarness {
		t.Fatalf("create = %+v, %v; want the project override used (and refused as unregistered)", res, err)
	}
}

// The Create form's tier models follow the harness the project's next launch
// uses.
func TestProjectWorktrees_TiersFollowProjectDefaultHarness(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	for name, extra := range map[string]string{
		"inherits": "",
		"claude":   `,"agents":{"default-harness":"claude"}`,
	} {
		if err := os.MkdirAll(filepath.Join(store, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := servertest.WriteProjectRepo(store, name, testutil.TempRepo(t), extra); err != nil {
			t.Fatal(err)
		}
	}
	h := servertest.StartWithStore(t, store, servertest.WithDefaultHarness("codex"), func(c *server.Config) {
		c.AgentsClaude = config.DefaultAgentsConfig().Claude
		c.AgentsCodex = config.DefaultAgentsConfig().Codex
	})
	list, err := h.Client.ProjectWorktrees(context.Background())
	if err != nil || len(list) != 2 {
		t.Fatalf("worktrees = %+v, %v", list, err)
	}
	for _, p := range list {
		agentKind := agent.AgentCodex
		if p.Project == "claude" {
			agentKind = agent.AgentClaude
		}
		for _, tier := range agent.AllTiers {
			want, _ := agent.ResolveTier(config.DefaultAgentsConfig(), agentKind, tier)
			if p.Tiers[string(tier)] != want {
				t.Errorf("%s %s model = %q, want %q", p.Project, tier, p.Tiers[string(tier)], want)
			}
		}
	}
}
