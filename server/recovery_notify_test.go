package server_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

// startNotifyRecovery starts a server with a fake chat and one rule entry
// whose remedy is remedy; it returns the server and the chat bodies sent so far.
func startNotifyRecovery(t *testing.T, hold time.Duration, remedy recovery.Remedy) (*servertest.Harness, func() []string) {
	t.Helper()
	return startNotifyRecoveryWith(t, hold, recovery.Entry{
		ID: "TEST", Type: events.NeedsRepair, Kind: events.IterationError,
		Executor: recovery.ExecutorRule, Authority: recovery.AuthorityLow, Enabled: true, Remedy: remedy,
	})
}

func startNotifyRecoveryWith(t *testing.T, hold time.Duration, entry recovery.Entry) (*servertest.Harness, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var bodies []string
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
	}))
	t.Cleanup(hook.Close)
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.Recovery = recovery.Catalog{Enabled: true, Entries: []recovery.Entry{entry}}
		c.RecoverySettings.NotifyHold = hold
		c.Chat = ralphloop.ServerChatConfig{SlackWebhookURL: hook.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")}
	})
	return h, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), bodies...)
	}
}

// parkMessages are the sends that are about the park, not the server starting.
func parkMessages(sent []string) []string {
	return parkMessagesOf(sent, events.IterationError)
}

func parkMessagesOf(sent []string, kind events.Kind) []string {
	var out []string
	for _, b := range sent {
		if strings.Contains(b, string(kind)) {
			out = append(out, b)
		}
	}
	return out
}

// waitParkMessages polls until a park message of kind is sent, then waits long
// enough for a wrong second one to show.
func waitParkMessages(t *testing.T, sent func() []string, kind events.Kind) []string {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline) && len(parkMessagesOf(sent(), kind)) == 0; time.Sleep(50 * time.Millisecond) {
	}
	time.Sleep(1500 * time.Millisecond)
	return parkMessagesOf(sent(), kind)
}

// unpark moves ticket 01 on the way a remedy's relaunch would.
func unpark(t *testing.T, h *servertest.Harness) {
	t.Helper()
	path := filepath.Join(h.TicketStore, "proj", "epic-a", "issues", "01-first.md")
	if err := schema.UpdateTicket(path, func(tk *schema.Ticket) { tk.Status = schema.StatusClaimed }); err != nil {
		t.Error(err)
	}
}

// waitRecoveryState polls the snapshot until the parked ticket shows want.
func waitRecoveryState(t *testing.T, h *servertest.Harness, want string) {
	t.Helper()
	var got server.TicketInfo
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		snap, err := h.Client.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, ti := range snap.Tickets {
			if ti.Address == "proj:epic-a/01" {
				got = ti
			}
		}
		if got.Status == "needs-repair" && got.Recovery == want {
			return
		}
	}
	t.Fatalf("ticket = %s, recovery %q; want needs-repair, recovery %q", got.Status, got.Recovery, want)
}

func TestRecoveryNotify_SnapshotShowsTheHold(t *testing.T) {
	t.Run("escalated after a failed remedy", func(t *testing.T) {
		release := make(chan struct{})
		h, _ := startNotifyRecovery(t, time.Minute, func(recovery.Failure, recovery.Verbs) error {
			<-release
			return errors.New("remedy refused")
		})
		if err := h.Server.ParkTicket("proj", "epic-a", "01", events.IterationError, "boom"); err != nil {
			t.Fatal(err)
		}
		waitRecoveryState(t, h, server.RecoveryPending)
		close(release)
		waitRecoveryState(t, h, server.RecoveryEscalated)
	})
	t.Run("cleared when the hold expires", func(t *testing.T) {
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		h, _ := startNotifyRecovery(t, 300*time.Millisecond, func(recovery.Failure, recovery.Verbs) error {
			<-release
			return nil
		})
		if err := h.Server.ParkTicket("proj", "epic-a", "01", events.IterationError, "boom"); err != nil {
			t.Fatal(err)
		}
		waitRecoveryState(t, h, "")
	})
}

func TestRecoveryNotify_RecoveredParkSendsNothingAndIsCounted(t *testing.T) {
	done := make(chan struct{})
	var h *servertest.Harness
	h, sent := startNotifyRecovery(t, time.Minute, func(recovery.Failure, recovery.Verbs) error {
		unpark(t, h)
		close(done)
		return nil
	})
	if err := h.Server.ParkTicket("proj", "epic-a", "01", events.IterationError, "boom"); err != nil {
		t.Fatal(err)
	}
	<-done
	time.Sleep(1500 * time.Millisecond) // a wrongly sent message would show by now
	if got := parkMessages(sent()); len(got) != 0 {
		t.Fatalf("a recovered park sent %v", got)
	}
	if n := server.RecoveredCount(filepath.Join(h.TicketStore, "proj"), "epic-a"); n != 1 {
		t.Errorf("recovered count = %d, want 1", n)
	}
}

func TestRecoveryNotify_FailedRemedySendsOneMessageNamingTheEntry(t *testing.T) {
	h, sent := startNotifyRecovery(t, time.Minute, func(recovery.Failure, recovery.Verbs) error {
		return errors.New("remedy refused")
	})
	if err := h.Server.ParkTicket("proj", "epic-a", "01", events.IterationError, "boom"); err != nil {
		t.Fatal(err)
	}
	got := waitParkMessages(t, sent, events.IterationError)
	if len(got) != 1 || !strings.Contains(got[0], "TEST") || !strings.Contains(got[0], "boom") {
		t.Fatalf("sends = %v, want one park message naming entry TEST and the original reason", got)
	}
}

func TestRecoveryNotify_OkRemedyThatLeavesTheTicketParkedSendsTheMessage(t *testing.T) {
	h, sent := startNotifyRecovery(t, time.Minute, func(recovery.Failure, recovery.Verbs) error { return nil })
	if err := h.Server.ParkTicket("proj", "epic-a", "01", events.IterationError, "boom"); err != nil {
		t.Fatal(err)
	}
	got := waitParkMessages(t, sent, events.IterationError)
	if len(got) != 1 || !strings.Contains(got[0], "TEST left it parked") || !strings.Contains(got[0], "boom") {
		t.Fatalf("sends = %v, want one park message saying TEST left it parked", got)
	}
}

func TestRecoveryNotify_RecognizedParkStillNotifiesAndIsNotCounted(t *testing.T) {
	entries := recovery.Default().Entries
	r1 := entries[slices.IndexFunc(entries, func(e recovery.Entry) bool { return e.ID == "R1" })]
	h, sent := startNotifyRecoveryWith(t, time.Minute, r1)
	if err := h.Server.ParkTicket("proj", "epic-a", "01", events.Spinning, "parked and re-claimed 3 times within 5m0s"); err != nil {
		t.Fatal(err)
	}
	if got := waitParkMessages(t, sent, events.Spinning); len(got) != 1 {
		t.Fatalf("sends = %v, want the spinning park message", got)
	}
	if n := server.RecoveredCount(filepath.Join(h.TicketStore, "proj"), "epic-a"); n != 0 {
		t.Errorf("recovered count = %d, want 0", n)
	}
}

func TestRecoveryNotify_SlowRecoverySendsTheOriginalParkAfterTheHold(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	h, sent := startNotifyRecovery(t, 200*time.Millisecond, func(recovery.Failure, recovery.Verbs) error {
		<-release
		return nil
	})
	if err := h.Server.ParkTicket("proj", "epic-a", "01", events.IterationError, "boom"); err != nil {
		t.Fatal(err)
	}
	var got []string
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline) && len(got) == 0; time.Sleep(50 * time.Millisecond) {
		got = parkMessages(sent())
	}
	if len(got) != 1 || !strings.Contains(got[0], "boom") {
		t.Fatalf("sends = %v, want the original park once the hold expired", got)
	}
}
