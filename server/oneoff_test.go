package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func startOneOffHarness(t *testing.T) (*servertest.Harness, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "root", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.PollInterval = 50 * time.Millisecond
	})
	if _, err := h.Client.QueuePause(context.Background()); err != nil {
		t.Fatal(err)
	}
	return h, repo
}

func TestOneOff_FromProjectRepoCreatesPromptTicketAtQueueTail(t *testing.T) {
	h, repo := startOneOffHarness(t)
	ctx := context.Background()
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", ""); err != nil || res.Refused {
		t.Fatalf("seed queue: %+v, %v", res, err)
	}

	res, err := h.Client.OneOff(ctx, server.OneOffRequest{Prompt: "fix the flaky test please", Cwd: repo})
	if err != nil || res.Refused {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
	if !strings.HasPrefix(res.Address, "proj:fix-the-flaky-test-") || !strings.HasSuffix(res.Address, "/01") {
		t.Fatalf("address = %q, want proj:fix-the-flaky-test-<suffix>/01", res.Address)
	}
	q, err := h.Client.QueueItems(ctx)
	if err != nil || len(q) != 2 || q[1].Address != res.Address || q[1].Agent != "claude" {
		t.Fatalf("queue = %+v, %v; want the one-off last under claude", q, err)
	}

	a, err := tickets.ParseAddress(res.Address, tickets.AddressContext{})
	if err != nil {
		t.Fatal(err)
	}
	epicDir := filepath.Join(h.TicketStore, a.Project, a.Epic)
	files, _ := filepath.Glob(filepath.Join(epicDir, "issues", "01-*.md"))
	if len(files) != 1 {
		t.Fatalf("ticket files = %v", files)
	}
	raw, _ := os.ReadFile(files[0])
	if !strings.Contains(string(raw), "type: prompt") || !strings.Contains(string(raw), "fix the flaky test please") {
		t.Fatalf("ticket = %s", raw)
	}
	log, _ := os.ReadFile(ralphloop.RunLogPath(filepath.Join(h.TicketStore, a.Project), a.Epic))
	if !strings.Contains(string(log), `"type":"submitted"`) || !strings.Contains(string(log), res.Address) {
		t.Fatalf("run log = %s", log)
	}
}

func TestOneOff_NoProjectLandsInScratchAndHonoursNameAndAgent(t *testing.T) {
	h, _ := startOneOffHarness(t)
	ctx := context.Background()

	res, err := h.Client.OneOff(ctx, server.OneOffRequest{Prompt: "anything", Cwd: t.TempDir(), Name: "my-job", Agent: "codex"})
	if err != nil || res.Refused {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
	if !strings.HasPrefix(res.Address, "scratch:my-job/") {
		t.Fatalf("address = %q, want scratch:my-job/…", res.Address)
	}
	q, _ := h.Client.QueueItems(ctx)
	if len(q) != 1 || q[0].Address != res.Address || q[0].Agent != "codex" {
		t.Fatalf("queue = %+v", q)
	}
}

func TestOneOff_ExplicitProjectBeatsCwd(t *testing.T) {
	h, _ := startOneOffHarness(t)
	res, err := h.Client.OneOff(context.Background(), server.OneOffRequest{Prompt: "x", Project: "proj", Cwd: t.TempDir()})
	if err != nil || res.Refused || !strings.HasPrefix(res.Address, "proj:") {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
}

func TestOneOff_RefusesEmptyPromptAndUnknownProject(t *testing.T) {
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
	files, _ := filepath.Glob(filepath.Join(h.TicketStore, a.Project, a.Epic, "issues", "01-*.md"))
	if len(files) != 1 {
		t.Fatalf("ticket files = %v", files)
	}
	raw, _ := os.ReadFile(files[0])
	return string(raw)
}

func TestOneOff_FrontQueuesAtHead(t *testing.T) {
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

func TestOneOff_CommitsOptionsLandInFrontmatter(t *testing.T) {
	h, repo := startOneOffHarness(t)
	res, err := h.Client.OneOff(context.Background(), server.OneOffRequest{
		Prompt: "build it", Cwd: repo, Commits: true, Base: "main",
		BlockedBy: []string{"proj:epic-a/01"}, ExpectedContextWindow: 40000,
	})
	if err != nil || res.Refused {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
	raw := readOneOffTicket(t, h, res.Address)
	for _, want := range []string{"type: implement", "base: main", "epic-a/01", "expected_context_window: 40000"} {
		if !strings.Contains(raw, want) {
			t.Errorf("ticket lacks %q:\n%s", want, raw)
		}
	}
}

func TestOneOff_FilePayloadSetsFieldsAndFlagsOverride(t *testing.T) {
	h, repo := startOneOffHarness(t)
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "payload.md")
	payload := "---\ntype: implement\nbase: main\nexpected_context_window: 40000\nname: from-file\n---\nbuild it from the file\n"
	if err := os.WriteFile(file, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.OneOff(ctx, server.OneOffRequest{File: file, Cwd: repo, ExpectedContextWindow: 90000})
	if err != nil || res.Refused {
		t.Fatalf("one-off: %+v, %v", res, err)
	}
	if res.Address != "proj:from-file/01" {
		t.Errorf("address = %q; want the frontmatter name", res.Address)
	}
	raw := readOneOffTicket(t, h, res.Address)
	for _, want := range []string{"type: implement", "base: main", "expected_context_window: 90000", "build it from the file"} {
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

func TestOneOff_RefusesBadOptionsAndCreatesNothing(t *testing.T) {
	h, repo := startOneOffHarness(t)
	ctx := context.Background()
	cases := []struct {
		name   string
		req    server.OneOffRequest
		reason string
	}{
		{"commits in a vcs none project", server.OneOffRequest{Prompt: "x", Commits: true}, server.ReasonNoCommits},
		{"base without commits", server.OneOffRequest{Prompt: "x", Cwd: repo, Base: "main"}, server.ReasonBadBase},
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
	if _, err := os.Stat(filepath.Join(h.TicketStore, "proj", "refused-job")); err == nil {
		t.Error("a refused submit left an epic behind")
	}
}

func TestOneOff_InvalidAgentLeavesNoTicketAndRetryIsNotDuplicateLive(t *testing.T) {
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
	if _, err := os.Stat(filepath.Join(h.TicketStore, "proj", "bad-agent-job")); err == nil {
		t.Error("a refused submit left an epic (and its submitted event) behind")
	}
	req.Agent = ""
	res, err = h.Client.OneOff(ctx, req)
	if err != nil || res.Refused {
		t.Errorf("retry = %+v, %v; want accepted, not duplicate-live", res, err)
	}
}

func TestOneOff_DuplicateLiveRefusedByFileKey(t *testing.T) {
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
