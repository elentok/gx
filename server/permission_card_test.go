package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

// herdrHosted is a runner that reports itself as herdr-hosted.
type herdrHosted struct{ *headlessFake }

func (herdrHosted) Name() string { return string(agentrunner.ChoiceHerdr) }

// markStarted gives the staged run a start time, which makes the ticket active.
func markStarted(t *testing.T, h *servertest.Harness) {
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

// startBlockedClaimed starts a claimed ticket whose live agent is blocked on p.
func startBlockedClaimed(t *testing.T, herdr bool, p *agentrunner.Pending) (*servertest.Harness, *headlessFake) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := &headlessFake{Runner: runnerfake.NewRunner(), verdict: nativerunner.VerdictLive}
	var runner agentrunner.Runner = fake
	if herdr {
		runner = herdrHosted{fake}
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.RunnerFor = func(_, _ string) agentrunner.Runner { return runner }
		c.BudgetPollInterval = time.Hour
	})
	wtPath, _ := leaveIteration(t, h, store, repo)
	if _, err := fake.Start(agentrunner.StartOptions{Label: "epic-a-iter-01", Cwd: wtPath}); err != nil {
		t.Fatal(err)
	}
	asHeadlessRun(t, h)
	markStarted(t, h)
	h.Restart(t)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	if res, err := h.Client.QueueAdd(context.Background(), answerAddr, "claude"); err != nil || res.Refused {
		t.Fatalf("queue add = %+v, %v", res, err)
	}
	fake.SetPending("epic-a-iter-01", p)
	return h, fake
}

func activeNeedsApproval(t *testing.T, h *servertest.Harness) bool {
	t.Helper()
	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range snap.Epics {
		for _, tk := range e.Active {
			if tk.Address == answerAddr {
				return tk.NeedsApproval
			}
		}
	}
	t.Fatalf("no active %s in %+v", answerAddr, snap.Epics)
	return false
}

func TestPermissionCard_AppearsForBlockedAgentAndVanishesAfterAnswer(t *testing.T) {
	h, fake := startBlockedClaimed(t, false, &agentrunner.Pending{
		ID: "req-1", Tool: "Bash", Input: json.RawMessage(`{"command":"ls"}`), Description: "List files",
	})
	ctx := context.Background()

	d, err := h.Client.Detail(ctx, answerAddr)
	if err != nil {
		t.Fatal(err)
	}
	want := server.PermissionCard{RequestID: "req-1", Tool: "Bash", Input: `{"command":"ls"}`, Description: "List files"}
	if d.Permission == nil || *d.Permission != want {
		t.Fatalf("permission = %+v, want %+v", d.Permission, want)
	}
	if !slices.Contains(d.Actions, server.ActionApprove) {
		t.Errorf("actions = %v, want approve", d.Actions)
	}
	if !activeNeedsApproval(t, h) {
		t.Error("snapshot does not flag the blocked ticket")
	}

	if res, err := h.Client.AgentAnswer(ctx, answerAddr, "allow", ""); err != nil || res.Refused {
		t.Fatalf("answer = %+v, %v", res, err)
	}
	d, err = h.Client.Detail(ctx, answerAddr)
	if err != nil || d.Permission != nil || slices.Contains(d.Actions, server.ActionApprove) {
		t.Errorf("after answer: permission %+v, actions %v, err %v", d.Permission, d.Actions, err)
	}
	if activeNeedsApproval(t, h) {
		t.Error("snapshot still flags the ticket after the decision")
	}
	if got := fake.Answers("epic-a-iter-01"); len(got) != 1 || got[0].Decision != agentrunner.DecisionAllow {
		t.Errorf("answers = %+v", got)
	}
}

// awaitTicketChanged waits for a ticket-changed event naming addr.
func awaitTicketChanged(t *testing.T, ctx context.Context, evs <-chan server.Event, addr, why string) {
	t.Helper()
	for {
		select {
		case ev := <-evs:
			if ev.Type == server.EventTicketChanged && ev.Address == addr {
				return
			}
		case <-ctx.Done():
			t.Fatalf("no ticket-changed for %s %s", addr, why)
		}
	}
}

// An open page learns of a prompt only from an event: the card and the
// "needs approval" row are read when a page fetches, and nothing else makes it
// fetch.
func TestPermissionCard_BlockAndClearPublishTicketChanged(t *testing.T) {
	h, fake := startBlockedClaimed(t, false, nil)
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

	fake.SetPending("epic-a-iter-01", &agentrunner.Pending{ID: "req-1", Tool: "Bash"})
	awaitTicketChanged(t, ctx, evs, answerAddr, "when the agent blocked")

	// Answered in the TUI: the server sends nothing, the agent just moves on.
	fake.SetState("epic-a-iter-01", agentrunner.StateWorking, "")
	awaitTicketChanged(t, ctx, evs, answerAddr, "when the block cleared")
}

func TestPermissionCard_NotApprovablePromptShowsTheHint(t *testing.T) {
	h, _ := startBlockedClaimed(t, false, &agentrunner.Pending{ID: "req-2", Tool: "ExitPlanMode", NotApprovable: true})
	d, err := h.Client.Detail(context.Background(), answerAddr)
	if err != nil {
		t.Fatal(err)
	}
	if d.Permission == nil || d.Permission.Hint != "Approve it in the TUI" || d.Permission.RequestID != "" || d.Permission.Tool != "ExitPlanMode" {
		t.Errorf("permission = %+v", d.Permission)
	}
	if slices.Contains(d.Actions, server.ActionApprove) {
		t.Errorf("actions = %v, want no approve", d.Actions)
	}
	if !activeNeedsApproval(t, h) {
		t.Error("snapshot does not flag the blocked ticket")
	}
}

func TestPermissionCard_HerdrHostedShowsTheHint(t *testing.T) {
	h, _ := startBlockedClaimed(t, true, &agentrunner.Pending{ID: "req-3", Tool: "Bash"})
	d, err := h.Client.Detail(context.Background(), answerAddr)
	if err != nil {
		t.Fatal(err)
	}
	if d.Permission == nil || d.Permission.Hint != "Approve it in the TUI" || d.Permission.RequestID != "" {
		t.Errorf("permission = %+v", d.Permission)
	}
	if slices.Contains(d.Actions, server.ActionApprove) {
		t.Errorf("actions = %v, want no approve", d.Actions)
	}
}

func TestPermissionCard_NoCardWhenNotBlocked(t *testing.T) {
	h, _ := startBlockedClaimed(t, false, nil)
	d, err := h.Client.Detail(context.Background(), answerAddr)
	if err != nil || d.Permission != nil {
		t.Errorf("permission = %+v, err %v; want none", d.Permission, err)
	}
	if activeNeedsApproval(t, h) {
		t.Error("snapshot flags a ticket that is not blocked")
	}
}

func TestWeb_AgentAnswerNeedsRequestIDAndOrigin(t *testing.T) {
	h, base := startWeb(t)
	js := map[string]string{"Origin": base, "Content-Type": "application/json"}
	post := func(headers map[string]string, body string) (int, string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, base+"/v1/agents/answer", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(login(t, h.StateDir, base))
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var res server.QueueResult
		_ = json.NewDecoder(resp.Body).Decode(&res)
		return resp.StatusCode, res.Reason
	}

	if code, _ := post(map[string]string{"Origin": "http://evil.example", "Content-Type": "application/json"}, `{"address":"proj:epic-a/01","decision":"allow","request_id":"r"}`); code != http.StatusForbidden {
		t.Errorf("wrong Origin = %d, want 403", code)
	}
	code, reason := post(js, `{"address":"proj:epic-a/01","decision":"allow"}`)
	if code != http.StatusOK || reason != server.ReasonRequestIDRequired {
		t.Errorf("no request_id = %d %q, want 200 %q", code, reason, server.ReasonRequestIDRequired)
	}
	// With a request ID the call goes through to the verb (and refuses: nothing is live).
	code, reason = post(js, `{"address":"proj:epic-a/01","decision":"allow","request_id":"r"}`)
	if code != http.StatusOK || reason == server.ReasonRequestIDRequired || reason == "" {
		t.Errorf("with request_id = %d %q, want the verb's own refusal", code, reason)
	}
}
