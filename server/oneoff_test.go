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
