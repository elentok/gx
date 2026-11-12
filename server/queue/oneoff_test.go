package queue_test

import (
	"context"
	"encoding/json"
	"github.com/elentok/gx/server/servertest/rigs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestOneOff_FromProjectRepoCreatesPromptTicketAtQueueTail(t *testing.T) {
	t.Parallel()
	h, repo := rigs.StartOneOffHarness(t)
	ctx := context.Background()
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", ""); err != nil || res.Refused {
		t.Fatalf("seed queue: %+v, %v", res, err)
	}

	res, err := h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "fix the flaky test please", Cwd: repo})
	if err != nil || res.Refused {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
	if res.Address != "proj:one-offs/01" {
		t.Fatalf("address = %q, want proj:one-offs/01", res.Address)
	}
	q, err := h.Client.Queue.Items(ctx)
	if err != nil || len(q) != 2 || q[1].Address != res.Address || q[1].Agent != "" {
		t.Fatalf("queue = %+v, %v; want the one-off last with an empty agent", q, err)
	}

	a, err := tickets.ParseAddress(res.Address, tickets.AddressContext{})
	if err != nil {
		t.Fatal(err)
	}
	epicDir := filepath.Join(h.TicketStore, a.Project, a.Epic)
	files, _ := filepath.Glob(filepath.Join(epicDir, "issues", "01-fix-the-flaky-test.md"))
	if len(files) != 1 {
		t.Fatalf("ticket files = %v", files)
	}
	raw, _ := os.ReadFile(files[0])
	if !strings.Contains(string(raw), "type: prompt") || !strings.Contains(string(raw), "fix the flaky test please") || strings.Contains(string(raw), "agent:") {
		t.Fatalf("ticket = %s", raw)
	}
	if !strings.Contains(string(raw), "created_at:") || !strings.Contains(string(raw), "original_address: "+res.Address+"\n") {
		t.Fatalf("one-off ticket lacks created_at or original_address %s:\n%s", res.Address, raw)
	}
	if epicMD, err := os.ReadFile(filepath.Join(epicDir, "ticket.md")); err != nil || !strings.Contains(string(epicMD), "status: draft") {
		t.Fatalf("epic ticket.md = %q, %v; want status: draft", epicMD, err)
	}
	log, _ := os.ReadFile(runlog.RunLogPath(filepath.Join(h.TicketStore, a.Project), a.Epic))
	if !strings.Contains(string(log), `"type":"submitted"`) || !strings.Contains(string(log), res.Address) {
		t.Fatalf("run log = %s", log)
	}
}

func TestOneOff_NoProjectMergesInScratchAndHonoursNameAndAgent(t *testing.T) {
	t.Parallel()
	h, _ := rigs.StartOneOffHarness(t)
	ctx := context.Background()

	res, err := h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "anything", Cwd: t.TempDir(), Name: "my-job", Agent: "codex"})
	if err != nil || res.Refused {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
	if res.Address != "scratch:one-offs/01" {
		t.Fatalf("address = %q, want scratch:one-offs/01", res.Address)
	}
	if files, _ := filepath.Glob(filepath.Join(h.TicketStore, "scratch", "one-offs", "issues", "01-my-job.md")); len(files) != 1 {
		t.Fatalf("ticket files = %v, want 01-my-job.md", files)
	}
	q, _ := h.Client.Queue.Items(ctx)
	if len(q) != 1 || q[0].Address != res.Address || q[0].Agent != "codex" {
		t.Fatalf("queue = %+v", q)
	}
}

func TestOneOff_ExplicitProjectBeatsCwd(t *testing.T) {
	t.Parallel()
	h, _ := rigs.StartOneOffHarness(t)
	res, err := h.Client.Queue.OneOff(context.Background(), wire.OneOffRequest{Prompt: "x", Project: "proj", Cwd: t.TempDir()})
	if err != nil || res.Refused || !strings.HasPrefix(res.Address, "proj:") {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
}

func TestOneOff_RefusesEmptyPromptAndUnknownProject(t *testing.T) {
	t.Parallel()
	h, _ := rigs.StartOneOffHarness(t)
	ctx := context.Background()
	if res, err := h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "  "}); err != nil || !res.Refused {
		t.Fatalf("empty prompt: %+v, %v", res, err)
	}
	if res, err := h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "x", Project: "nope"}); err != nil || !res.Refused {
		t.Fatalf("unknown project: %+v, %v", res, err)
	}
}

func readOneOffTicket(t *testing.T, h *servertest.Harness, addr string) string {
	t.Helper()
	a, err := tickets.ParseAddress(addr, tickets.AddressContext{})
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(h.TicketStore, a.Project, a.Epic, "issues", a.ID+"-*.md"))
	if len(files) != 1 {
		t.Fatalf("ticket files = %v", files)
	}
	raw, _ := os.ReadFile(files[0])
	return string(raw)
}

func TestOneOff_FrontQueuesAtHead(t *testing.T) {
	t.Parallel()
	h, repo := rigs.StartOneOffHarness(t)
	ctx := context.Background()
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", ""); err != nil || res.Refused {
		t.Fatalf("seed queue: %+v, %v", res, err)
	}
	res, err := h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "urgent", Cwd: repo, Front: true})
	if err != nil || res.Refused {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
	q, _ := h.Client.Queue.Items(ctx)
	if len(q) != 2 || q[0].Address != res.Address {
		t.Fatalf("queue = %+v; want the one-off first", q)
	}
}

func TestOneOff_OptionsMergeInFrontmatter(t *testing.T) {
	t.Parallel()
	h, repo := rigs.StartOneOffHarness(t)
	res, err := h.Client.Queue.OneOff(context.Background(), wire.OneOffRequest{
		Prompt: "build it", Cwd: repo,
		BlockedBy: []string{"proj:epic-a/01"}, ExpectedContextWindow: 40000,
	})
	if err != nil || res.Refused {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
	raw := readOneOffTicket(t, h, res.Address)
	for _, want := range []string{"type: prompt", "epic-a/01", "expected_context_window: 40000"} {
		if !strings.Contains(raw, want) {
			t.Errorf("ticket lacks %q:\n%s", want, raw)
		}
	}
}

func TestOneOff_NameBecomesTheTicketSlug(t *testing.T) {
	t.Parallel()
	h, repo := rigs.StartOneOffHarness(t)
	res, err := h.Client.Queue.OneOff(context.Background(), wire.OneOffRequest{
		Prompt: "build it from the request", Name: "from-request", Cwd: repo, ExpectedContextWindow: 90000,
	})
	if err != nil || res.Refused {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
	if res.Address != "proj:one-offs/01" {
		t.Errorf("address = %q; want proj:one-offs/01", res.Address)
	}
	if files, _ := filepath.Glob(filepath.Join(h.TicketStore, "proj", "one-offs", "issues", "01-from-request.md")); len(files) != 1 {
		t.Errorf("ticket files = %v; want the name as slug", files)
	}
	raw := readOneOffTicket(t, h, res.Address)
	for _, want := range []string{"type: prompt", "expected_context_window: 90000", "build it from the request"} {
		if !strings.Contains(raw, want) {
			t.Errorf("ticket lacks %q:\n%s", want, raw)
		}
	}
}

// A name becomes a filename, so only lowercase letters, digits and dashes pass.
func TestOneOff_NameThatIsNotASlugIsRefusedAndNothingIsWritten(t *testing.T) {
	t.Parallel()
	h, repo := rigs.StartOneOffHarness(t)
	ctx := context.Background()
	for _, name := range []string{"Upper", "with_underscore", "dotted.name", "..", "../escape", "a/b", "with space", `back\slash`} {
		res, err := h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "x", Name: name, Cwd: repo})
		if err != nil || !res.Refused || res.Reason != wire.ReasonInvalidAddress {
			t.Errorf("name %q: %+v, %v; want refusal %s", name, res, err, wire.ReasonInvalidAddress)
		}
	}
	if files, _ := filepath.Glob(filepath.Join(h.TicketStore, "proj", "one-offs", "issues", "*")); len(files) != 0 {
		t.Errorf("a refused submit left %v behind", files)
	}
}

// Only an explicit choice (request or payload frontmatter) pins an agent; with
// the default on codex a plain submit pins nothing and queues an empty agent.
func TestOneOff_AgentIsPinnedOnlyByAnExplicitChoice(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cases := []struct {
		name      string
		req       wire.OneOffRequest
		wantAgent string
	}{
		{"plain", wire.OneOffRequest{Prompt: "plain job", Name: "plain-job"}, ""},
		{"flag", wire.OneOffRequest{Prompt: "flag job", Name: "flag-job", Agent: "claude"}, "claude"},
		{"payload", wire.OneOffRequest{Prompt: "from the file", Name: "payload-job", Agent: "codex"}, "codex"},
	}
	for _, c := range cases {
		// A harness per case: the epic's root is queued once, by its first one-off.
		h, repo := rigs.StartOneOffHarness(t, func(c *server.Config) { c.AgentsDefaultHarness = "codex" })
		c.req.Cwd = repo
		res, err := h.Client.Queue.OneOff(ctx, c.req)
		if err != nil || res.Refused {
			t.Fatalf("%s: %+v, %v", c.name, res, err)
		}
		raw := readOneOffTicket(t, h, res.Address)
		if c.wantAgent == "" && strings.Contains(raw, "agent:") {
			t.Errorf("%s: ticket pins an agent:\n%s", c.name, raw)
		}
		if c.wantAgent != "" && !strings.Contains(raw, "agent: "+c.wantAgent+"\n") {
			t.Errorf("%s: ticket lacks agent: %s:\n%s", c.name, c.wantAgent, raw)
		}
		q, _ := h.Client.Queue.Items(ctx)
		var got *wire.QueueItem
		for i := range q {
			if q[i].Address == res.Address {
				got = &q[i]
			}
		}
		if got == nil || got.Agent != c.wantAgent {
			t.Errorf("%s: queue item = %+v, want agent %q", c.name, got, c.wantAgent)
		}
	}
}

func TestOneOff_RefusesBadOptionsAndCreatesNothing(t *testing.T) {
	t.Parallel()
	h, repo := rigs.StartOneOffHarness(t)
	ctx := context.Background()
	cases := []struct {
		name   string
		req    wire.OneOffRequest
		reason string
	}{
		{"unknown blocker", wire.OneOffRequest{Prompt: "x", Cwd: repo, BlockedBy: []string{"proj:epic-a/09"}}, wire.ReasonBadBlocker},
		{"cross-project blocker", wire.OneOffRequest{Prompt: "x", Cwd: repo, BlockedBy: []string{"scratch:e/01"}}, wire.ReasonBadBlocker},
	}
	for _, c := range cases {
		c.req.Name = "refused-job"
		res, err := h.Client.Queue.OneOff(ctx, c.req)
		if err != nil || !res.Refused || res.Reason != c.reason {
			t.Errorf("%s: %+v, %v; want refusal %s", c.name, res, err, c.reason)
		}
	}
	if q, _ := h.Client.Queue.Items(ctx); len(q) != 0 {
		t.Errorf("queue = %+v; want empty", q)
	}
	if files, _ := filepath.Glob(filepath.Join(h.TicketStore, "proj", "one-offs", "issues", "*refused-job*")); len(files) != 0 {
		t.Errorf("a refused submit left %v behind", files)
	}
}

func TestOneOff_InvalidAgentLeavesNoTicketAndRetryIsNotDuplicateLive(t *testing.T) {
	t.Parallel()
	h, repo := rigs.StartOneOffHarness(t)
	ctx := context.Background()
	req := wire.OneOffRequest{Prompt: "do the thing", Unique: "job-key", Cwd: repo, Name: "bad-agent-job", Agent: "gpt"}
	res, err := h.Client.Queue.OneOff(ctx, req)
	if err != nil || !res.Refused || res.Reason != wire.ReasonInvalidAgent {
		t.Fatalf("got %+v, %v; want refusal %s", res, err, wire.ReasonInvalidAgent)
	}
	if files, _ := filepath.Glob(filepath.Join(h.TicketStore, "proj", "one-offs", "issues", "*bad-agent-job*")); len(files) != 0 {
		t.Errorf("a refused submit left %v behind", files)
	}
	req.Agent = ""
	res, err = h.Client.Queue.OneOff(ctx, req)
	if err != nil || res.Refused {
		t.Errorf("retry = %+v, %v; want accepted, not duplicate-live", res, err)
	}
}

func TestOneOff_NumbersAboveTheArchiveAndQueuesTheRootOnce(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "root", "")
	servertest.WriteTicket(t, store, "proj", queue.OneOffsArchivedEpic, "07", "old", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.PollInterval = 50 * time.Millisecond })
	ctx := context.Background()
	if _, err := h.Client.Queue.Pause(ctx); err != nil {
		t.Fatal(err)
	}

	first, err := h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "one", Cwd: repo, Name: "a"})
	if err != nil || first.Refused || first.Address != "proj:one-offs/08" {
		t.Fatalf("first = %+v, %v; want proj:one-offs/08", first, err)
	}
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", ""); err != nil || res.Refused {
		t.Fatalf("queue epic-a: %+v, %v", res, err)
	}
	second, err := h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "two", Cwd: repo, Name: "b"})
	if err != nil || second.Refused || second.Address != "proj:one-offs/09" {
		t.Fatalf("second = %+v, %v; want proj:one-offs/09", second, err)
	}
	if q, _ := h.Client.Queue.Items(ctx); len(q) != 2 || q[0].Address != first.Address {
		t.Fatalf("queue = %+v; want the one-offs root queued once, still first", q)
	}
}

func TestOneOff_FrontMovesAQueuedRootToTheHead(t *testing.T) {
	t.Parallel()
	h, repo := rigs.StartOneOffHarness(t)
	ctx := context.Background()
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", ""); err != nil || res.Refused {
		t.Fatalf("queue epic-a: %+v, %v", res, err)
	}
	first, err := h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "one", Cwd: repo})
	if err != nil || first.Refused {
		t.Fatalf("first = %+v, %v", first, err)
	}
	if res, err := h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "two", Cwd: repo, Front: true}); err != nil || res.Refused {
		t.Fatalf("front = %+v, %v", res, err)
	}
	if q, _ := h.Client.Queue.Items(ctx); len(q) != 2 || q[0].Address != first.Address || q[1].Address != "proj:epic-a/01" {
		t.Fatalf("queue = %+v; want one-offs then epic-a", q)
	}
}

func TestOneOff_RefusesBadTypes(t *testing.T) {
	t.Parallel()
	h, repo := rigs.StartOneOffHarness(t)
	for name, req := range map[string]wire.OneOffRequest{
		"unknown type": {Prompt: "x", Cwd: repo, Type: "research"},
		"implement":    {Prompt: "x", Cwd: repo, Type: "implement"},
	} {
		res, err := h.Client.Queue.OneOff(context.Background(), req)
		if err != nil || !res.Refused || res.Reason != wire.ReasonBadType {
			t.Errorf("%s: %+v, %v; want invalid-type", name, res, err)
		}
	}
}

func TestOneOff_DuplicateLiveRefusedByUniqueKey(t *testing.T) {
	t.Parallel()
	h, repo := rigs.StartOneOffHarness(t)
	ctx := context.Background()
	first, err := h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "do the thing", Unique: "payload-key", Cwd: repo})
	if err != nil || first.Refused {
		t.Fatalf("first: %+v, %v", first, err)
	}
	res, err := h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "do the thing", Unique: "payload-key", Cwd: repo})
	if err != nil || !res.Refused || res.Reason != wire.ReasonDuplicateLive || res.Address != first.Address {
		t.Errorf("same key: %+v, %v; want duplicate-live naming %s", res, err, first.Address)
	}
	if res, err := h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "do the thing", Unique: "payload-key", Cwd: repo, NoUnique: true}); err != nil || res.Refused {
		t.Errorf("--no-unique: %+v, %v; want accepted", res, err)
	}
	if res, err := h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "plain", Cwd: repo}); err != nil || res.Refused {
		t.Errorf("plain prompt: %+v, %v; want accepted", res, err)
	}
	evs, err := os.ReadFile(filepath.Join(h.TicketStore, "proj", strings.Split(strings.TrimPrefix(first.Address, "proj:"), "/")[0], "run-log.jsonl"))
	if err != nil || !strings.Contains(string(evs), `"type":"submit-refused"`) || !strings.Contains(string(evs), `"kind":"duplicate-live"`) {
		t.Errorf("run log lacks the submit-refused event: %v\n%s", err, evs)
	}
}

func oneOffTicketFile(t *testing.T, h *servertest.Harness, addr string) string {
	t.Helper()
	a, err := tickets.ParseAddress(addr, tickets.AddressContext{})
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(h.TicketStore, a.Project, a.Epic, "issues", a.ID+"-*.md"))
	if len(files) != 1 {
		t.Fatalf("ticket files for %s = %v", addr, files)
	}
	return servertest.ReadFile(t, files[0])
}

func TestOneOff_SubmitScheduledWritesScheduledForAndLaterCreatedAt(t *testing.T) {
	t.Parallel()
	h, repo := rigs.StartOneOffHarness(t)
	set := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	res, err := h.Server.OneOffs().SubmitScheduled(wire.OneOffRequest{Prompt: "nightly", Cwd: repo}, set)
	if err != nil || res.Refused {
		t.Fatalf("submit scheduled: %+v, %v", res, err)
	}
	raw := oneOffTicketFile(t, h, res.Address)
	if !strings.Contains(raw, "scheduled_for: "+set.Format(time.RFC3339)+"\n") {
		t.Errorf("ticket lacks scheduled_for %s:\n%s", set.Format(time.RFC3339), raw)
	}
	tk, err := schema.ParseTicketFromRaw(raw, "01-nightly.md")
	if err != nil {
		t.Fatal(err)
	}
	if !tk.ScheduledFor.Equal(set) || !tk.CreatedAt.After(tk.ScheduledFor) {
		t.Errorf("scheduled_for = %v, created_at = %v; want created_at after %v", tk.ScheduledFor, tk.CreatedAt, set)
	}
}

func TestOneOff_RunNowAndRawBodyWriteNoScheduledFor(t *testing.T) {
	t.Parallel()
	h, repo := rigs.StartOneOffHarness(t)
	res, err := h.Client.Queue.OneOff(context.Background(), wire.OneOffRequest{Prompt: "now", Cwd: repo})
	if err != nil || res.Refused {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
	if raw := oneOffTicketFile(t, h, res.Address); strings.Contains(raw, "scheduled_for") {
		t.Errorf("plain one-off wrote scheduled_for:\n%s", raw)
	}
	// The route decodes the body into wire.OneOffRequest, so an extra key is dropped.
	var req wire.OneOffRequest
	body := `{"prompt":"sneaky","cwd":` + strconv.Quote(repo) + `,"scheduled_for":"2020-01-01T00:00:00Z"}`
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	res, err = h.Server.OneOffs().Submit(req)
	if err != nil || res.Refused {
		t.Fatalf("submit: %+v, %v", res, err)
	}
	if raw := oneOffTicketFile(t, h, res.Address); strings.Contains(raw, "scheduled_for") {
		t.Errorf("raw body set scheduled_for:\n%s", raw)
	}
}
