package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// startRecovery starts a server whose catalog has one test-only low-authority
// rule that relaunches a manually parked ticket, and reports each remedy result.
func startRecovery(t *testing.T, optOut bool) (*servertest.Harness, <-chan recovery.Result) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	if optOut {
		path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
		data, _ := os.ReadFile(path)
		if err := os.WriteFile(path, []byte(strings.Replace(string(data), "status: open\n", "status: open\nrecover: false\n", 1)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	results := make(chan recovery.Result, 4)
	cat := recovery.Catalog{Enabled: true, Entries: []recovery.Entry{{
		ID: "TEST", Type: events.NeedsRepair, Kind: events.ManualPark,
		Executor: recovery.ExecutorRule, Authority: recovery.AuthorityLow, Enabled: true,
		Remedy: func(f recovery.Failure, v recovery.Verbs) error {
			res, err := v.Relaunch(f.Address)
			results <- res
			return err
		},
	}}}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.Recovery = cat
	})
	registerLaunch(h)
	return h, results
}

func TestRecovery_MatchingParkRunsItsRemedyThroughAServerVerb(t *testing.T) {
	h, results := startRecovery(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketPark(ctx, "proj:epic-a/01", "broken"); err != nil || res.Refused {
		t.Fatalf("park = %+v, %v", res, err)
	}
	select {
	case res := <-results:
		if res.Actor != "recovery" || res.Via != "server" || res.Refused {
			t.Errorf("remedy result = %+v, want a served verb by actor recovery", res)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("remedy never ran")
	}
}

func TestRecovery_RecoveredParkLeavesMatchedAndAppliedWithKind(t *testing.T) {
	h, results := startRecovery(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketPark(ctx, "proj:epic-a/01", "broken"); err != nil || res.Refused {
		t.Fatalf("park = %+v, %v", res, err)
	}
	select {
	case <-results:
	case <-time.After(5 * time.Second):
		t.Fatal("remedy never ran")
	}

	var matched, applied *ralphloop.Event
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && (matched == nil || applied == nil) {
		log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
		matched, applied = nil, nil
		for i, ev := range log {
			switch events.Type(ev.Type) {
			case events.RecoveryMatched:
				matched = &log[i]
			case events.RecoveryApplied:
				applied = &log[i]
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if matched == nil || applied == nil {
		t.Fatalf("matched = %v, applied = %v, want both recorded", matched, applied)
	}
	for _, ev := range []*ralphloop.Event{matched, applied} {
		if ev.Ticket != "01" || ev.Kind != string(events.ManualPark) || ev.Reason != "TEST" {
			t.Errorf("%s = %+v, want ticket 01, kind manual-park, entry TEST", ev.Type, *ev)
		}
	}
	if applied.Outcome != "ok" {
		t.Errorf("applied outcome = %q, want ok", applied.Outcome)
	}
}

func TestRecovery_TicketOptOutNeverTriggers(t *testing.T) {
	h, results := startRecovery(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketPark(ctx, "proj:epic-a/01", "broken"); err != nil || res.Refused {
		t.Fatalf("park = %+v, %v", res, err)
	}
	select {
	case res := <-results:
		t.Fatalf("recover: false ticket ran a remedy: %+v", res)
	case <-time.After(500 * time.Millisecond):
	}
}
