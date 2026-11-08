package prompt_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// mapCase is one Design map in the readiness store: its design step, the
// status of its impl epic and a tweak made once the server is up.
type mapCase struct {
	name, step, implStatus string
	base                   string
	setup                  func(t *testing.T, h *servertest.Harness, store string)
	wantReason             string // substring of the reason; "" is ready
}

func writeMapCase(t *testing.T, store string, c mapCase) {
	t.Helper()
	dir := filepath.Join(store, "proj", c.name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ticket.md"), []byte("---\nkind: map\ndesign_step: "+c.step+"\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.WriteTicketWith(t, store, "proj", c.name+"-impl", "01", "first", servertest.TicketOpts{Base: c.base})
	implMD := "---\nstatus: " + c.implStatus + "\n---\n\n# " + c.name + "-impl\n"
	if err := os.WriteFile(filepath.Join(store, "proj", c.name+"-impl", "ticket.md"), []byte(implMD), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAutoQueueNotReady_OneReasonPerFailingCondition(t *testing.T) {
	t.Parallel()
	cases := []mapCase{
		{name: "ready", step: "branch", implStatus: "open"},
		{name: "based", step: "branch", implStatus: "open", base: "other", wantReason: ""},
		{name: "done-step", step: "done", implStatus: "open", wantReason: "not at the Branch step"},
		{name: "spec-step", step: "spec", implStatus: "open", wantReason: "not at the Branch step"},
		{name: "held", step: "branch", implStatus: "open", wantReason: "held",
			setup: func(t *testing.T, h *servertest.Harness, _ string) {
				if err := h.Server.Workspaces().Hold("proj:held"); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "drafty", step: "branch", implStatus: "draft", wantReason: "is a draft"},
		{name: "closed", step: "branch", implStatus: "done", wantReason: "is not open"},
		{name: "queued", step: "branch", implStatus: "open", wantReason: "already queued",
			setup: func(t *testing.T, h *servertest.Harness, _ string) {
				if res, err := h.Server.Prompt().QueueEpic("proj", "queued-impl"); err != nil || res.Refused {
					t.Fatalf("queue = %+v, %v", res, err)
				}
			}},
		{name: "ran", step: "branch", implStatus: "open", wantReason: "already run",
			setup: func(t *testing.T, _ *servertest.Harness, store string) {
				ev := runlog.Event{Time: time.Now(), Type: string(events.IterationStarted), Ticket: "01"}
				if err := runlog.AppendEvent(filepath.Join(store, "proj"), "ran-impl", ev); err != nil {
					t.Fatal(err)
				}
			}},
	}
	store := t.TempDir()
	for _, c := range cases {
		writeMapCase(t, store, c)
	}
	h := servertest.StartWithStore(t, store)
	for _, c := range cases {
		if c.setup != nil {
			c.setup(t, h, store)
		}
	}
	for _, c := range cases {
		got, err := h.Server.Prompt().AutoQueueNotReady("proj", c.name)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if (c.wantReason == "") != (got == "") || !strings.Contains(got, c.wantReason) {
			t.Errorf("%s: reason = %q, want it to contain %q", c.name, got, c.wantReason)
		}
	}
}

func TestQueueEpic_QueuesThroughTheQueueAndReturnsItsRefusal(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	writeMapCase(t, store, mapCase{name: "ok", step: "branch", implStatus: "open"})
	writeMapCase(t, store, mapCase{name: "drafty", step: "branch", implStatus: "draft"})
	h := servertest.StartWithStore(t, store)
	p := h.Server.Prompt()

	if res, err := p.QueueEpic("proj", "drafty-impl"); err != nil || !res.Refused || res.Reason != wire.ReasonEpicDraft {
		t.Errorf("queue draft = %+v, %v, want the queue's draft refusal", res, err)
	}
	if res, err := p.QueueEpic("proj", "ok-impl"); err != nil || res.Refused {
		t.Fatalf("queue ok = %+v, %v", res, err)
	}
	if got, _ := p.AutoQueueNotReady("proj", "ok"); !strings.Contains(got, "already queued") {
		t.Errorf("after queuing, reason = %q, want already queued", got)
	}
}

func TestNewestDesignSession_TheNewestOneDecidesLiveOrExited(t *testing.T) {
	t.Parallel()
	h, ctx, _, first, _, _ := startGrillingSessionIn(t, "proj:map", storeWithMapStep(t, "spec"))
	p := h.Server.Prompt()
	newest := func() string {
		t.Helper()
		s, ok, err := p.NewestDesignSession("proj", "map")
		if err != nil || !ok {
			t.Fatalf("newest = %+v, %v, %v", s, ok, err)
		}
		return s.ID
	}
	res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: "proj:map"})
	if err != nil || res.Session == nil {
		t.Fatalf("second create = %+v, %v", res, err)
	}
	second := res.Session.ID
	if got := newest(); got != second {
		t.Errorf("both live: newest = %s, want the second %s", got, second)
	}
	if r, err := h.Client.Sessions.Stop(ctx, second); err != nil || r.Refused {
		t.Fatalf("stop second = %+v, %v", r, err)
	}
	if got := newest(); got != first {
		t.Errorf("second exited: newest = %s, want the live first %s", got, first)
	}
	if r, err := h.Client.Sessions.Stop(ctx, first); err != nil || r.Refused {
		t.Fatalf("stop first = %+v, %v", r, err)
	}
	if got := newest(); got != first {
		t.Errorf("both exited: newest = %s, want the last to exit, %s", got, first)
	}
}
