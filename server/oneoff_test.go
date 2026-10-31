package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func startOneOffHarness(t *testing.T, opts ...func(*server.Config)) (*servertest.Harness, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "root", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, append(opts, func(c *server.Config) {
		c.PollInterval = 50 * time.Millisecond
	})...)
	if _, err := h.Client.QueuePause(context.Background()); err != nil {
		t.Fatal(err)
	}
	return h, repo
}

func TestOneOff_FromProjectRepoCreatesPromptTicketAtQueueTail(t *testing.T) {
	t.Parallel()
	h, repo := startOneOffHarness(t)
	ctx := context.Background()
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", ""); err != nil || res.Refused {
		t.Fatalf("seed queue: %+v, %v", res, err)
	}

	res, err := h.Client.OneOff(ctx, server.OneOffRequest{Prompt: "fix the flaky test please", Cwd: repo})
	if err != nil || res.Refused {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
	if res.Address != "proj:one-offs/01" {
		t.Fatalf("address = %q, want proj:one-offs/01", res.Address)
	}
	q, err := h.Client.QueueItems(ctx)
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
	log, _ := os.ReadFile(ralphloop.RunLogPath(filepath.Join(h.TicketStore, a.Project), a.Epic))
	if !strings.Contains(string(log), `"type":"submitted"`) || !strings.Contains(string(log), res.Address) {
		t.Fatalf("run log = %s", log)
	}
}

func TestOneOff_NoProjectMergesInScratchAndHonoursNameAndAgent(t *testing.T) {
	t.Parallel()
	h, _ := startOneOffHarness(t)
	ctx := context.Background()

	res, err := h.Client.OneOff(ctx, server.OneOffRequest{Prompt: "anything", Cwd: t.TempDir(), Name: "my-job", Agent: "codex"})
	if err != nil || res.Refused {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
	if res.Address != "scratch:one-offs/01" {
		t.Fatalf("address = %q, want scratch:one-offs/01", res.Address)
	}
	if files, _ := filepath.Glob(filepath.Join(h.TicketStore, "scratch", "one-offs", "issues", "01-my-job.md")); len(files) != 1 {
		t.Fatalf("ticket files = %v, want 01-my-job.md", files)
	}
	q, _ := h.Client.QueueItems(ctx)
	if len(q) != 1 || q[0].Address != res.Address || q[0].Agent != "codex" {
		t.Fatalf("queue = %+v", q)
	}
}

func TestOneOff_ExplicitProjectBeatsCwd(t *testing.T) {
	t.Parallel()
	h, _ := startOneOffHarness(t)
	res, err := h.Client.OneOff(context.Background(), server.OneOffRequest{Prompt: "x", Project: "proj", Cwd: t.TempDir()})
	if err != nil || res.Refused || !strings.HasPrefix(res.Address, "proj:") {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
}

func TestOneOff_RefusesEmptyPromptAndUnknownProject(t *testing.T) {
	t.Parallel()
	h, _ := startOneOffHarness(t)
	ctx := context.Background()
	if res, err := h.Client.OneOff(ctx, server.OneOffRequest{Prompt: "  "}); err != nil || !res.Refused {
		t.Fatalf("empty prompt: %+v, %v", res, err)
	}
	if res, err := h.Client.OneOff(ctx, server.OneOffRequest{Prompt: "x", Project: "nope"}); err != nil || !res.Refused {
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
	h, repo := startOneOffHarness(t)
	ctx := context.Background()
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", ""); err != nil || res.Refused {
		t.Fatalf("seed queue: %+v, %v", res, err)
	}
	res, err := h.Client.OneOff(ctx, server.OneOffRequest{Prompt: "urgent", Cwd: repo, Front: true})
	if err != nil || res.Refused {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
	q, _ := h.Client.QueueItems(ctx)
	if len(q) != 2 || q[0].Address != res.Address {
		t.Fatalf("queue = %+v; want the one-off first", q)
	}
}

func TestOneOff_OptionsMergeInFrontmatter(t *testing.T) {
	t.Parallel()
	h, repo := startOneOffHarness(t)
	res, err := h.Client.OneOff(context.Background(), server.OneOffRequest{
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

func TestOneOff_FileTypeKeyIsIgnored(t *testing.T) {
	t.Parallel()
	h, repo := startOneOffHarness(t)
	file := filepath.Join(t.TempDir(), "payload.md")
	if err := os.WriteFile(file, []byte("---\ntype: implement\n---\njust a prompt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.OneOff(context.Background(), server.OneOffRequest{File: file, Cwd: repo})
	if err != nil || res.Refused {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
	if raw := readOneOffTicket(t, h, res.Address); !strings.Contains(raw, "type: prompt") {
		t.Errorf("type key changed the ticket type:\n%s", raw)
	}
}

func TestOneOff_FilePayloadSetsFieldsAndFlagsOverride(t *testing.T) {
	t.Parallel()
	h, repo := startOneOffHarness(t)
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "payload.md")
	payload := "---\nexpected_context_window: 40000\nname: from-file\n---\nbuild it from the file\n"
	if err := os.WriteFile(file, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.OneOff(ctx, server.OneOffRequest{File: file, Cwd: repo, ExpectedContextWindow: 90000})
	if err != nil || res.Refused {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
	if res.Address != "proj:one-offs/01" {
		t.Errorf("address = %q; want proj:one-offs/01", res.Address)
	}
	if files, _ := filepath.Glob(filepath.Join(h.TicketStore, "proj", "one-offs", "issues", "01-from-file.md")); len(files) != 1 {
		t.Errorf("ticket files = %v; want the frontmatter name as slug", files)
	}
	raw := readOneOffTicket(t, h, res.Address)
	for _, want := range []string{"type: prompt", "expected_context_window: 90000", "build it from the file"} {
		if !strings.Contains(raw, want) {
			t.Errorf("ticket lacks %q:\n%s", want, raw)
		}
	}
	if strings.Contains(raw, "40000") {
		t.Errorf("flag should override the frontmatter window:\n%s", raw)
	}

	res, err = h.Client.OneOff(ctx, server.OneOffRequest{File: filepath.Join(t.TempDir(), "missing.md"), Cwd: repo})
	if err != nil || !res.Refused || res.Reason != server.ReasonBadFile {
		t.Errorf("missing file: %+v, %v; want %s refusal", res, err, server.ReasonBadFile)
	}
}

// Only an explicit choice (request or payload frontmatter) pins an agent; with
// the default on codex a plain submit pins nothing and queues an empty agent.
func TestOneOff_AgentIsPinnedOnlyByAnExplicitChoice(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	payload := filepath.Join(t.TempDir(), "payload.md")
	if err := os.WriteFile(payload, []byte("---\nagent: codex\n---\nfrom the file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name      string
		req       server.OneOffRequest
		wantAgent string
	}{
		{"plain", server.OneOffRequest{Prompt: "plain job", Name: "plain-job"}, ""},
		{"flag", server.OneOffRequest{Prompt: "flag job", Name: "flag-job", Agent: "claude"}, "claude"},
		{"payload", server.OneOffRequest{File: payload, Name: "payload-job"}, "codex"},
	}
	for _, c := range cases {
		// A harness per case: the epic's root is queued once, by its first one-off.
		h, repo := startOneOffHarness(t, func(c *server.Config) { c.AgentsDefaultHarness = "codex" })
		c.req.Cwd = repo
		res, err := h.Client.OneOff(ctx, c.req)
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
		q, _ := h.Client.QueueItems(ctx)
		var got *server.QueueItem
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
	h, repo := startOneOffHarness(t)
	ctx := context.Background()
	cases := []struct {
		name   string
		req    server.OneOffRequest
		reason string
	}{
		{"unknown blocker", server.OneOffRequest{Prompt: "x", Cwd: repo, BlockedBy: []string{"proj:epic-a/09"}}, server.ReasonBadBlocker},
		{"cross-project blocker", server.OneOffRequest{Prompt: "x", Cwd: repo, BlockedBy: []string{"scratch:e/01"}}, server.ReasonBadBlocker},
	}
	for _, c := range cases {
		c.req.Name = "refused-job"
		res, err := h.Client.OneOff(ctx, c.req)
		if err != nil || !res.Refused || res.Reason != c.reason {
			t.Errorf("%s: %+v, %v; want refusal %s", c.name, res, err, c.reason)
		}
	}
	if q, _ := h.Client.QueueItems(ctx); len(q) != 0 {
		t.Errorf("queue = %+v; want empty", q)
	}
	if files, _ := filepath.Glob(filepath.Join(h.TicketStore, "proj", "one-offs", "issues", "*refused-job*")); len(files) != 0 {
		t.Errorf("a refused submit left %v behind", files)
	}
}

func TestOneOff_InvalidAgentLeavesNoTicketAndRetryIsNotDuplicateLive(t *testing.T) {
	t.Parallel()
	h, repo := startOneOffHarness(t)
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "job.md")
	if err := os.WriteFile(file, []byte("do the thing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := server.OneOffRequest{File: file, Cwd: repo, Name: "bad-agent-job", Agent: "gpt"}
	res, err := h.Client.OneOff(ctx, req)
	if err != nil || !res.Refused || res.Reason != server.ReasonInvalidAgent {
		t.Fatalf("got %+v, %v; want refusal %s", res, err, server.ReasonInvalidAgent)
	}
	if files, _ := filepath.Glob(filepath.Join(h.TicketStore, "proj", "one-offs", "issues", "*bad-agent-job*")); len(files) != 0 {
		t.Errorf("a refused submit left %v behind", files)
	}
	req.Agent = ""
	res, err = h.Client.OneOff(ctx, req)
	if err != nil || res.Refused {
		t.Errorf("retry = %+v, %v; want accepted, not duplicate-live", res, err)
	}
}

func TestOneOff_RetroSubmitCreatesEvolveTicketAndRefusesALiveDuplicate(t *testing.T) {
	t.Parallel()
	h, _ := startOneOffHarness(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 10, 14, 5, 0, 0, time.UTC)
	req := server.RetroSubmit("proj", "epic-a", at)

	first, err := h.Client.OneOff(ctx, req)
	if err != nil || first.Refused {
		t.Fatalf("retro: %+v, %v", first, err)
	}
	if first.Address != "proj:one-offs/01" {
		t.Errorf("address = %q, want proj:one-offs/01", first.Address)
	}
	files, _ := filepath.Glob(filepath.Join(h.TicketStore, "proj", "one-offs", "issues", "01-evolve-epic-a-20261010-1405.md"))
	if len(files) != 1 {
		t.Fatalf("ticket files = %v", files)
	}
	raw, _ := os.ReadFile(files[0])
	for _, want := range []string{"type: evolve", "notify: true", "unique: evolve:proj/epic-a"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("ticket lacks %q:\n%s", want, raw)
		}
	}
	if strings.Contains(string(raw), "agent:") {
		t.Errorf("retro ticket pins an agent:\n%s", raw)
	}

	again := server.RetroSubmit("proj", "epic-a", at.Add(time.Hour))
	res, err := h.Client.OneOff(ctx, again)
	if err != nil || !res.Refused || res.Reason != server.ReasonDuplicateLive || res.Address != first.Address {
		t.Errorf("second retro: %+v, %v; want duplicate-live naming %s", res, err, first.Address)
	}
}

func TestOneOff_NumbersAboveTheArchiveAndQueuesTheRootOnce(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "root", "")
	servertest.WriteTicket(t, store, "proj", server.OneOffsArchivedEpic, "07", "old", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.PollInterval = 50 * time.Millisecond })
	ctx := context.Background()
	if _, err := h.Client.QueuePause(ctx); err != nil {
		t.Fatal(err)
	}

	first, err := h.Client.OneOff(ctx, server.OneOffRequest{Prompt: "one", Cwd: repo, Name: "a"})
	if err != nil || first.Refused || first.Address != "proj:one-offs/08" {
		t.Fatalf("first = %+v, %v; want proj:one-offs/08", first, err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", ""); err != nil || res.Refused {
		t.Fatalf("queue epic-a: %+v, %v", res, err)
	}
	second, err := h.Client.OneOff(ctx, server.OneOffRequest{Prompt: "two", Cwd: repo, Name: "b"})
	if err != nil || second.Refused || second.Address != "proj:one-offs/09" {
		t.Fatalf("second = %+v, %v; want proj:one-offs/09", second, err)
	}
	if q, _ := h.Client.QueueItems(ctx); len(q) != 2 || q[0].Address != first.Address {
		t.Fatalf("queue = %+v; want the one-offs root queued once, still first", q)
	}
}

func TestOneOff_FrontMovesAQueuedRootToTheHead(t *testing.T) {
	t.Parallel()
	h, repo := startOneOffHarness(t)
	ctx := context.Background()
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", ""); err != nil || res.Refused {
		t.Fatalf("queue epic-a: %+v, %v", res, err)
	}
	first, err := h.Client.OneOff(ctx, server.OneOffRequest{Prompt: "one", Cwd: repo})
	if err != nil || first.Refused {
		t.Fatalf("first = %+v, %v", first, err)
	}
	if res, err := h.Client.OneOff(ctx, server.OneOffRequest{Prompt: "two", Cwd: repo, Front: true}); err != nil || res.Refused {
		t.Fatalf("front = %+v, %v", res, err)
	}
	if q, _ := h.Client.QueueItems(ctx); len(q) != 2 || q[0].Address != first.Address || q[1].Address != "proj:epic-a/01" {
		t.Fatalf("queue = %+v; want one-offs then epic-a", q)
	}
}

func TestOneOff_RefusesBadTypes(t *testing.T) {
	t.Parallel()
	h, repo := startOneOffHarness(t)
	for name, req := range map[string]server.OneOffRequest{
		"unknown type": {Prompt: "x", Cwd: repo, Type: "research"},
		"implement":    {Prompt: "x", Cwd: repo, Type: "implement"},
	} {
		res, err := h.Client.OneOff(context.Background(), req)
		if err != nil || !res.Refused || res.Reason != server.ReasonBadType {
			t.Errorf("%s: %+v, %v; want invalid-type", name, res, err)
		}
	}
}

func TestOneOff_DuplicateLiveRefusedByFileKey(t *testing.T) {
	t.Parallel()
	h, repo := startOneOffHarness(t)
	ctx := context.Background()
	dir := t.TempDir()
	file := filepath.Join(dir, "payload.md")
	if err := os.WriteFile(file, []byte("do the thing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	first, err := h.Client.OneOff(ctx, server.OneOffRequest{File: file, Cwd: repo})
	if err != nil || first.Refused {
		t.Fatalf("first: %+v, %v", first, err)
	}
	for name, f := range map[string]string{"same file": file, "symlink": link} {
		res, err := h.Client.OneOff(ctx, server.OneOffRequest{File: f, Cwd: repo})
		if err != nil || !res.Refused || res.Reason != server.ReasonDuplicateLive || res.Address != first.Address {
			t.Errorf("%s: %+v, %v; want duplicate-live naming %s", name, res, err, first.Address)
		}
	}
	if res, err := h.Client.OneOff(ctx, server.OneOffRequest{File: file, Cwd: repo, NoUnique: true}); err != nil || res.Refused {
		t.Errorf("--no-unique: %+v, %v; want accepted", res, err)
	}
	if res, err := h.Client.OneOff(ctx, server.OneOffRequest{Prompt: "plain", Cwd: repo}); err != nil || res.Refused {
		t.Errorf("plain prompt: %+v, %v; want accepted", res, err)
	}
	evs, err := os.ReadFile(filepath.Join(h.TicketStore, "proj", strings.Split(strings.TrimPrefix(first.Address, "proj:"), "/")[0], "run-log.jsonl"))
	if err != nil || !strings.Contains(string(evs), `"type":"submit-refused"`) || !strings.Contains(string(evs), `"kind":"duplicate-live"`) {
		t.Errorf("run log lacks the submit-refused event: %v\n%s", err, evs)
	}
}
