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

// Codex runs on pty under every config but herdr, which hosts it only when
// configured; Claude stays on the configured runner.
func TestLaunchRunner_CodexRunsOnPtyUnlessHerdrIsConfigured(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ configured, codexOn string }{
		{"pty", "pty"},
		{"headless", "pty"},
		{"herdr", "herdr"},
	} {
		t.Run(tc.configured, func(t *testing.T) {
			t.Parallel()
			store, repo := t.TempDir(), testutil.TempRepo(t)
			servertest.WriteTicket(t, store, "proj", "epic-a", "01", "codex one", "")
			servertest.WriteTicket(t, store, "proj", "epic-b", "01", "claude one", "")
			servertest.SetProjectRepo(t, store, "proj", repo)
			fakes := map[string]namedFake{}
			for _, name := range []string{"pty", "headless", "herdr"} {
				fakes[name] = namedFake{runnerfake.NewRunner(), name}
			}
			configured := fakes[tc.configured]
			h := servertest.StartWithStore(t, store, servertest.WithRunnerFor(configured, func(name, _ string) agentrunner.Runner {
				if f, ok := fakes[name]; ok {
					return f
				}
				return nil
			}))
			t.Cleanup(func() {
				fakes[tc.codexOn].SetState("epic-a-iter-01", agentrunner.StateDone, "")
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
			if runners["proj:epic-a/01"] != tc.codexOn || runners["proj:epic-b/01"] != tc.configured {
				t.Errorf("run runners = %v, want codex on %s and claude on %s", runners, tc.codexOn, tc.configured)
			}
			if got := strings.Join(fakes[tc.codexOn].Prompts("epic-a-iter-01"), " "); !strings.HasPrefix(got, "$gx-") {
				t.Errorf("codex prompt on %s = %q, want a $gx- prefix", tc.codexOn, got)
			}
			if got := configured.Prompts("epic-b-iter-01"); len(got) == 0 {
				t.Error("claude ticket did not run on the configured runner")
			}
		})
	}
}
