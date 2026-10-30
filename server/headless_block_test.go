package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

// startBlockable starts proj:epic-a/01 on a runner named runnerName whose agent
// keeps working until the test blocks it, and returns once the session is live.
func startBlockable(t *testing.T, runnerName string, opts ...func(*server.Config)) (*servertest.Harness, string, *turnRunner) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	ticketPath := store + "/proj/epic-a/issues/01-first.md"
	turn := newTurnRunner(func(_, _ string) {})
	turn.PromptState = agentrunner.StateWorking
	var fake agentrunner.Runner = codexNamed{turn}
	switch runnerName {
	case "herdr":
		fake = herdrNamed{turn}
	case "pty":
		fake = ptyNamed{turn}
	}
	h := servertest.StartWithStore(t, store, append([]func(*server.Config){servertest.WithRunner(fake)}, opts...)...)
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	waitFor(t, func() bool { _, ok, _ := turn.Find("epic-a-iter-01"); return ok })
	return h, ticketPath, turn
}

func TestHeadlessBlock_PausesAtOnce(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		block func(*turnRunner)
		want  []string
	}{
		{"permission request", func(r *turnRunner) {
			r.SetPending("epic-a-iter-01", &agentrunner.Pending{ID: "r1", Tool: "Bash", Input: []byte(`{"command":"ls"}`)})
		}, []string{"Bash", `"command":"ls"`, "gx server agents answer"}},
		{"bare requires_action", func(r *turnRunner) {
			r.SetState("epic-a-iter-01", agentrunner.StateBlocked, "waiting on a prompt")
		}, []string{"waiting on a prompt", "gx server agents interrupt"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, ticketPath, turn := startBlockable(t, "headless")
			c.block(turn)
			waitStatus(t, ticketPath, schema.StatusLegacyAnswer)
			tk, err := schema.ParseTicket(ticketPath)
			if err != nil {
				t.Fatal(err)
			}
			assertPaused(t, tk, schema.PauseClassAnswer, schema.PauseBlockedPane)
			// The pause hands the run to the waiting-run machinery.
			waitFor(t, func() bool {
				data, _ := os.ReadFile(filepath.Join(h.StateDir, "runs.json"))
				return strings.Contains(string(data), `"waiting_since"`)
			})
			if _, ok, err := turn.Find("epic-a-iter-01"); err != nil || !ok {
				t.Errorf("session not live: ok=%v err=%v", ok, err)
			}
			var reason string
			for _, ev := range runLog(t, h) {
				if ev.Type == "paused" && ev.Kind == "blocked-pane" {
					reason = ev.Reason
				}
			}
			for _, w := range c.want {
				if !strings.Contains(reason, w) {
					t.Errorf("pause reason %q lacks %q", reason, w)
				}
			}
		})
	}
}

// ptyNamed is a turnRunner that answers to "pty", so its runs are pty-hosted.
type ptyNamed struct{ *turnRunner }

func (ptyNamed) Name() string { return "pty" }

// A herdr or pty hosted agent that stays blocked past the dwell is paused by
// the server wait loop, like a headless one.
func TestHeadlessBlock_NonHeadlessPausesAfterDwell(t *testing.T) {
	t.Parallel()
	for _, runner := range []string{"herdr", "pty"} {
		t.Run(runner, func(t *testing.T) {
			_, ticketPath, turn := startBlockable(t, runner)
			turn.SetState("epic-a-iter-01", agentrunner.StateBlocked, "waiting on a prompt")
			waitStatus(t, ticketPath, schema.StatusLegacyAnswer)
		})
	}
}

// The pause raises the chat notification, not only the paused run-log event.
func TestHeadlessBlock_PauseRaisesNotification(t *testing.T) {
	t.Parallel()
	chat, count := chatRecorder(t)
	_, ticketPath, turn := startBlockable(t, "headless", chat)
	turn.SetPending("epic-a-iter-01", &agentrunner.Pending{ID: "r1", Tool: "Bash", Input: []byte(`{"command":"ls"}`)})
	waitStatus(t, ticketPath, schema.StatusLegacyAnswer)
	if got := waitForChat(count, "blocked-pane"); got != 1 {
		t.Errorf("blocked-pane notification sent %d times, want once", got)
	}
}

// answer on a ticket that is not paused blocked-pane leaves its status alone.
func TestHeadlessBlock_AnswerOnClaimedLeavesStatus(t *testing.T) {
	t.Parallel()
	h, ticketPath, _ := startBlockable(t, "headless")
	// The agent is working and the ticket claimed; the answer may be refused, but must not move it.
	_, _ = h.Client.AgentAnswerRequest(context.Background(), server.QueueRequest{Address: answerAddr, Decision: "allow", RequestID: "r1"})
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusClaimed {
		t.Errorf("status = %q, want claimed", tk.Status)
	}
}

// pauseBlocked blocks the session of a headless run on a permission request
// and returns once the ticket is paused and the run waits.
func pauseBlocked(t *testing.T) (*servertest.Harness, string, *turnRunner) {
	t.Helper()
	h, ticketPath, turn := startBlockable(t, "headless")
	turn.SetPending("epic-a-iter-01", &agentrunner.Pending{ID: "r1", Tool: "Bash", Input: []byte(`{"command":"ls"}`)})
	waitStatus(t, ticketPath, schema.StatusLegacyAnswer)
	waitFor(t, func() bool { return runsJSONHas(h, `"waiting_since"`) })
	return h, ticketPath, turn
}

func runsJSONHas(h *servertest.Harness, s string) bool {
	data, _ := os.ReadFile(filepath.Join(h.StateDir, "runs.json"))
	return strings.Contains(string(data), s)
}

func TestHeadlessBlock_AnswerReturnsToClaimed(t *testing.T) {
	t.Parallel()
	h, ticketPath, turn := pauseBlocked(t)
	res, err := h.Client.AgentAnswerRequest(context.Background(), server.QueueRequest{Address: answerAddr, Decision: "allow", RequestID: "r1"})
	if err != nil || res.Refused {
		t.Fatalf("answer = %+v, %v", res, err)
	}
	waitStatus(t, ticketPath, schema.StatusClaimed)
	// The wait is back on and the run takes its slot back.
	waitFor(t, func() bool { return !runsJSONHas(h, `"waiting_since"`) })
	if got := turn.Answers("epic-a-iter-01"); len(got) != 1 {
		t.Errorf("answers = %+v, want one", got)
	}
}

// A new block after the answer pauses again.
func TestHeadlessBlock_PausesAgainAfterAnswer(t *testing.T) {
	t.Parallel()
	h, ticketPath, turn := pauseBlocked(t)
	if res, err := h.Client.AgentAnswerRequest(context.Background(), server.QueueRequest{Address: answerAddr, Decision: "allow", RequestID: "r1"}); err != nil || res.Refused {
		t.Fatalf("answer = %+v, %v", res, err)
	}
	waitStatus(t, ticketPath, schema.StatusClaimed)
	turn.SetPending("epic-a-iter-01", &agentrunner.Pending{ID: "r2", Tool: "Bash", Input: []byte(`{"command":"pwd"}`)})
	waitStatus(t, ticketPath, schema.StatusLegacyAnswer)
}

// Every way out of blocked clears the pause the same way.
func TestHeadlessBlock_AnyExitReturnsToClaimed(t *testing.T) {
	t.Parallel()
	exits := map[string]func(*servertest.Harness, *turnRunner){
		"interrupt": func(h *servertest.Harness, _ *turnRunner) {
			if res, err := h.Client.AgentInterrupt(context.Background(), answerAddr); err != nil || res.Refused {
				t.Errorf("interrupt = %+v, %v", res, err)
			}
		},
		"cancel request": func(_ *servertest.Harness, r *turnRunner) { r.SetState("epic-a-iter-01", agentrunner.StateWorking, "") },
		"running":        func(_ *servertest.Harness, r *turnRunner) { r.SetState("epic-a-iter-01", agentrunner.StateWorking, "") },
		"idle":           func(_ *servertest.Harness, r *turnRunner) { r.SetState("epic-a-iter-01", agentrunner.StateIdle, "") },
		"result":         func(_ *servertest.Harness, r *turnRunner) { r.SetState("epic-a-iter-01", agentrunner.StateDone, "") },
	}
	for name, exit := range exits {
		t.Run(name, func(t *testing.T) {
			h, ticketPath, turn := pauseBlocked(t)
			exit(h, turn)
			waitFor(t, func() bool {
				tk, err := schema.ParseTicket(ticketPath)
				return err == nil && tk.Status != schema.StatusLegacyAnswer
			})
			waitFor(t, func() bool { return !runsJSONHas(h, `"waiting_since"`) })
		})
	}
}

func TestHeadlessBlock_ResumeRefusedWhileLiveAllowedOnceGone(t *testing.T) {
	t.Parallel()
	h, ticketPath, turn := pauseBlocked(t)
	req := server.RepairRequest{Address: answerAddr}
	res, err := h.Server.ResumeAs(req, server.OriginCLI)
	if err != nil || !res.Refused || res.Reason != server.ReasonAgentWaiting || !strings.Contains(res.Message, "gx server agents answer") {
		t.Fatalf("resume live = %+v, %v; want %s naming the answer verb", res, err, server.ReasonAgentWaiting)
	}
	sess, ok, err := turn.Find("epic-a-iter-01")
	if err != nil || !ok {
		t.Fatalf("Find = %v, %v", ok, err)
	}
	if err := turn.Stop(sess); err != nil {
		t.Fatal(err)
	}
	res, err = h.Server.ResumeAs(req, server.OriginCLI)
	if err != nil || res.Refused {
		t.Fatalf("resume gone = %+v, %v", res, err)
	}
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status == schema.StatusLegacyAnswer {
		t.Errorf("still paused after resume of a gone agent")
	}
}
