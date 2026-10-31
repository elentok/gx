package server_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
)

// namedFake is a fake that reports itself as the runner called name.
type namedFake struct {
	*runnerfake.Runner
	name string
}

func (n namedFake) Name() string { return n.name }

const resumeLabel = "epic-a-iter-01"

// claimOpen puts ticket 01 in the queue as a ticket that has run before
// (session sess-old) and was paused, then waits for its next claim to launch.
// answered says whether the pause ended with a person's answer or a plain resume.
// rollout says whether Codex holds a rollout for sess-old in the claim's worktree.
func claimOpen(t *testing.T, runner string, agent string, answered, rollout bool, fake *runnerfake.Runner) []ralphloop.Event {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	codexHome := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	withSession := strings.Replace(string(body), "type: implement\n", "type: implement\nsession_ids: [\"sess-old\"]\n", 1)
	if err := os.WriteFile(path, []byte(withSession), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := tickets.AppendLog(path, tickets.LogEvent{Type: tickets.EventPauseQuestion, By: "agent", Summary: "which one?", Handoff: "half done"}); err != nil {
		t.Fatal(err)
	}
	end := tickets.LogEvent{Type: tickets.EventPauseClosed, Question: 1, Reason: "resume"}
	if answered {
		end = tickets.LogEvent{Type: tickets.EventPauseAnswer, Question: 1, Source: "web", Choice: "a"}
	}
	if _, err := tickets.AppendLog(path, end); err != nil {
		t.Fatal(err)
	}

	h := servertest.StartWithStore(t, store, servertest.WithRunner(namedFake{fake, runner}), servertest.WithEnv("CODEX_HOME="+codexHome))
	if rollout {
		_, _, wt := ralphloop.IterationIdentity("epic-a", "01", h.Server.WorktreeDir("proj"))
		meta := `{"type":"session_meta","payload":{"id":"sess-old","cwd":` + strconv.Quote(wt) + `}}` + "\n"
		dir := filepath.Join(codexHome, "sessions", "2026", "10", "30")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "rollout-2026-10-30T00-00-00-sess-old.jsonl"), []byte(meta), 0o644); err != nil {
			t.Fatal(err)
		}
	}
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
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", agent); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	for ev := range evs {
		if ev.Type == server.EventIterationStarted {
			break
		}
	}
	t.Cleanup(func() { fake.SetState(resumeLabel, agentrunner.StateDone, "") })
	logged, _, err := ralphloop.ReadEvents(filepath.Join(store, "proj"), "epic-a")
	if err != nil {
		t.Fatal(err)
	}
	return slices.DeleteFunc(logged, func(ev ralphloop.Event) bool { return ev.Type != string(events.IterationStarted) })
}

func TestClaim_ResumesOrColdStarts(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		runner    string
		agent     string
		answered  bool
		rollout   bool
		resumeErr error
		wantIDs   []string
		wantPath  string
	}{
		{"answered pty resumes", "pty", "claude", true, false, nil, []string{"sess-old"}, "resumed"},
		{"failed resume falls back cold", "pty", "claude", true, false, errors.New("no conversation found"), []string{"sess-old", ""}, "cold-start"},
		{"plain resume cold-starts", "pty", "claude", false, false, nil, []string{""}, "cold-start"},
		{"herdr cold-starts", "herdr", "claude", true, false, nil, []string{""}, "cold-start"},
		{"headless cold-starts", "headless", "claude", true, false, nil, []string{""}, "cold-start"},
		{"codex resumes", "pty", "codex", true, true, nil, []string{"sess-old"}, "resumed"},
		{"codex without rollout cold-starts", "pty", "codex", true, false, nil, []string{""}, "cold-start"},
		{"codex plain resume cold-starts", "pty", "codex", false, true, nil, []string{""}, "cold-start"},
		{"codex herdr cold-starts", "herdr", "codex", true, true, nil, []string{""}, "cold-start"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := runnerfake.NewRunner()
			fake.ResumeErr = c.resumeErr
			logged := claimOpen(t, c.runner, c.agent, c.answered, c.rollout, fake)
			if got := fake.ResumeIDs(resumeLabel); !slices.Equal(got, c.wantIDs) {
				t.Errorf("start attempts carried resume ids %q, want %q", got, c.wantIDs)
			}
			if len(logged) != 1 || logged[0].Path != c.wantPath {
				t.Errorf("iteration-started events = %+v, want one with path %q", logged, c.wantPath)
			}
		})
	}
}
