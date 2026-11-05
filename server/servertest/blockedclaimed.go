package servertest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
)

// HerdrHosted is a runner that reports itself as herdr-hosted.
type HerdrHosted struct{ *HeadlessFake }

func (HerdrHosted) Name() string { return string(agentrunner.ChoiceHerdr) }

// MarkStarted gives the staged run a start time, which makes the ticket active.
func MarkStarted(t *testing.T, h *Harness) {
	t.Helper()
	path := filepath.Join(h.StateDir, "runs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs []map[string]any
	if err := json.Unmarshal(data, &runs); err != nil {
		t.Fatal(err)
	}
	runs[0]["started_at"] = time.Now()
	if data, err = json.Marshal(runs); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// StartBlockedClaimed starts a claimed ticket whose live agent is blocked on p.
func StartBlockedClaimed(t *testing.T, herdr bool, p *agentrunner.Pending) (*Harness, *HeadlessFake) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	SetProjectRepo(t, store, "proj", repo)
	fake := NewHeadlessFake(nativerunner.VerdictLive)
	var runner agentrunner.Runner = fake
	if herdr {
		runner = HerdrHosted{fake}
	}
	// The scaled-down dwell would pause the blocked ticket within a loaded
	// machine's lag, before the test reads the card.
	hold, release := GateDwell()
	t.Cleanup(release)
	h := StartWithStore(t, store, WithRunner(runner), hold, func(c *server.Config) {
		c.BudgetPollInterval = time.Hour
		c.WebAddr = "127.0.0.1:0"
	})
	wtPath, _ := LeaveIteration(t, h, store, repo, "herdr")
	if _, err := fake.Start(agentrunner.StartOptions{Label: "epic-a-iter-01", Cwd: wtPath}); err != nil {
		t.Fatal(err)
	}
	// A new fake session is idle, which the restarted server reads as a finished
	// agent: it merges the ticket and stops the session before SetPending. A
	// restart also closes an idle (still launching) agent and reopens its claim,
	// so the agent must already be mid-turn when the server comes back.
	fake.SetState("epic-a-iter-01", agentrunner.StateWorking, "")
	AsHeadlessRun(t, h)
	MarkStarted(t, h)
	h.Restart(t)
	Await(t, func() bool { return len(h.Server.Runs()) == 1 })
	if res, err := h.Client.Queue.Add(context.Background(), AnswerAddr, "claude"); err != nil || res.Refused {
		t.Fatalf("queue add = %+v, %v", res, err)
	}
	// A block raised while the launch still waits for the session to be ready fails
	// the launch instead of showing the card.
	WaitFor(t, time.Minute, "the launch finishes", func() bool {
		return len(h.Server.Runs()) == 1 && h.Server.Budget().LaunchingRuns() == 0
	})
	fake.SetPending("epic-a-iter-01", p)
	return h, fake
}
