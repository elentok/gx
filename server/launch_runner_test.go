package server_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

func TestLaunchRunner_CodexGoesToHerdrWhateverNativeRunnerIsConfigured(t *testing.T) {
	t.Parallel()
	for _, native := range []string{"pty", "headless"} {
		t.Run(native, func(t *testing.T) {
			t.Parallel()
			store, repo := t.TempDir(), testutil.TempRepo(t)
			servertest.WriteTicket(t, store, "proj", "epic-a", "01", "codex one", "")
			servertest.WriteTicket(t, store, "proj", "epic-b", "01", "claude one", "")
			servertest.SetProjectRepo(t, store, "proj", repo)
			configured := namedFake{runnerfake.NewRunner(), native}
			herdr := namedFake{runnerfake.NewRunner(), "herdr"}
			h := servertest.StartWithStore(t, store, servertest.WithRunnerFor(configured, func(name, _ string) agentrunner.Runner {
				switch name {
				case native:
					return configured
				case "herdr":
					return herdr
				}
				return nil
			}))
			t.Cleanup(func() {
				herdr.SetState("epic-a-iter-01", agentrunner.StateDone, "")
				configured.SetState("epic-b-iter-01", agentrunner.StateDone, "")
			})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			snap, err := h.Client.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			evs, err := h.Client.Events(ctx, snap.Seq)
			if err != nil {
				t.Fatal(err)
			}
			if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "codex"); err != nil || res.Refused {
				t.Fatalf("add codex: %+v, %v", res, err)
			}
			if res, err := h.Client.QueueAdd(ctx, "proj:epic-b/01", "claude"); err != nil || res.Refused {
				t.Fatalf("add claude: %+v, %v", res, err)
			}
			for started := 0; started < 2; {
				if (<-evs).Type == server.EventIterationStarted {
					started++
				}
			}

			runners := map[string]string{}
			for _, r := range h.Server.Runs() {
				runners[r.Address] = r.Runner
			}
			if runners["proj:epic-a/01"] != "herdr" || runners["proj:epic-b/01"] != native {
				t.Errorf("run runners = %v, want codex on herdr and claude on %s", runners, native)
			}
			if got := strings.Join(herdr.Prompts("epic-a-iter-01"), " "); !strings.HasPrefix(got, "$gx-") {
				t.Errorf("codex prompt on herdr = %q, want a $gx- prefix", got)
			}
			if got := configured.Prompts("epic-b-iter-01"); len(got) == 0 {
				t.Error("claude ticket did not run on the configured runner")
			}
		})
	}
}
