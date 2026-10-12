package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/subscription"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// chatServer is a server wired to a fake Slack webhook that records each body.
// It keeps the real 6s batch tick, so notices sent before s.chat.Close() share
// one batch; use chatServerFlushing to wait on the tick instead.
func chatServer(t *testing.T, soft, hard float64) (*Server, func(n int) []string) {
	t.Helper()
	return chatServerFlushing(t, soft, hard, 0)
}

// chatServerFlushing is chatServer with the batch tick set to flush (zero
// means the real one).
func chatServerFlushing(t *testing.T, soft, hard float64, flush time.Duration) (*Server, func(n int) []string) {
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
	// New probes herdr; answer it so a runner without herdr doesn't leak a
	// "herdr unavailable" notice into the recorded sends.
	fake := herdrfake.NewState(t)
	fake.Register("workspace", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspaces": []any{}}, herdrfake.Identities{}, nil
	})
	herdrfake.StartState(t, fake)
	stateDir, err := os.MkdirTemp("", "gx") // short path: unix socket limit
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	s, err := New(Config{StateDir: stateDir, TicketStore: t.TempDir(), Settings: Settings{BudgetSoftLimit: soft, BudgetHardLimit: hard,
		Chat: ralphloop.ServerChatConfig{SlackWebhookURL: hook.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: flush}},
		// Not the real ~/.claude.json, which could add an extra-usage notice.
		ExtraUsageCheck: func() subscription.State { return subscription.StateDisabled }})
	if err != nil {
		t.Fatal(err)
	}
	// wait returns what was sent once n messages arrived, plus a grace period
	// for an extra one to show. n < 0 returns at once: the caller has already
	// called s.chat.Close(), which sends what is queued and waits for it.
	wait := func(n int) []string {
		if n >= 0 {
			for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
				mu.Lock()
				got := len(bodies)
				mu.Unlock()
				if got >= n {
					break
				}
			}
			time.Sleep(500 * time.Millisecond) // anything extra would show up now
		}
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), bodies...)
	}
	return s, wait
}

func TestServerNotices_RestartSendsOneServerLevelMessage(t *testing.T) {
	s, wait := chatServerFlushing(t, 0, 0, 100*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()

	got := wait(1)
	cancel()
	<-done
	if len(got) != 1 || !strings.Contains(got[0], "server started") {
		t.Fatalf("sends = %v, want one 'server started'", got)
	}
}

func TestServerNotices_BudgetLimitLatchesOncePerDay(t *testing.T) {
	s, wait := chatServer(t, 10, 20)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	s.seedBudgetNotes(now)

	s.ledger.record("", "a", 11, now)
	s.notifyBudget(now)
	s.ledger.record("", "a", 12, now.Add(time.Minute))
	s.notifyBudget(now.Add(time.Minute)) // still over the soft limit: no repeat
	s.ledger.record("", "a", 21, now.Add(2*time.Minute))
	s.notifyBudget(now.Add(2 * time.Minute))

	s.chat.Close()
	got := wait(-1)
	if len(got) != 1 {
		t.Fatalf("sends = %d, want the soft and hard notices batched in 1: %v", len(got), got)
	}
	if !strings.Contains(got[0], "soft limit") || !strings.Contains(got[0], "hard limit") {
		t.Errorf("message lacks a limit notice: %s", got[0])
	}
}

func TestServerNotices_BudgetAlreadyOverAtStartIsNotAnnounced(t *testing.T) {
	s, wait := chatServer(t, 10, 0)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	s.ledger.record("", "a", 11, now)
	s.seedBudgetNotes(now)
	s.notifyBudget(now)

	s.chat.Close()
	if got := wait(-1); len(got) != 0 {
		t.Fatalf("sends = %v, want none", got)
	}
}

func TestServerNotices_BudgetLimitsCountSpendBeyondOverride(t *testing.T) {
	s, wait := chatServer(t, 20, 50)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	s.seedBudgetNotes(now)

	s.ledger.record("", "a", 30, now)
	s.notifyBudget(now)
	if _, err := s.budgetOverride(now); err != nil {
		t.Fatal(err)
	}
	s.ledger.record("", "a", 45, now.Add(time.Minute))
	s.notifyBudget(now.Add(time.Minute)) // 15 beyond the override: nothing
	s.ledger.record("", "a", 55, now.Add(2*time.Minute))
	s.notifyBudget(now.Add(2 * time.Minute)) // 25 beyond: soft again, not hard
	s.ledger.record("", "a", 85, now.Add(3*time.Minute))
	s.notifyBudget(now.Add(3 * time.Minute)) // 55 beyond: hard

	s.chat.Close()
	all := strings.Join(wait(-1), "\n")
	if n := strings.Count(all, "soft limit reached"); n != 2 {
		t.Errorf("soft notices = %d, want 2: %s", n, all)
	}
	if n := strings.Count(all, "hard limit reached"); n != 1 {
		t.Errorf("hard notices = %d, want 1: %s", n, all)
	}
	for _, want := range []string{"$25.00 of $20.00", "$55.00 of $50.00"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing post-override detail %q: %s", want, all)
		}
	}
}

func TestServerNotices_RestartAfterOverrideSeedsFromSpendBeyondIt(t *testing.T) {
	s, wait := chatServer(t, 20, 0)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	s.ledger.record("", "a", 30, now)
	if _, err := s.budgetOverride(now); err != nil {
		t.Fatal(err)
	}
	s.ledger.record("", "a", 45, now.Add(time.Minute))
	s.seedBudgetNotes(now.Add(time.Minute)) // restart: 15 beyond, not yet crossed
	s.notifyBudget(now.Add(time.Minute))
	s.ledger.record("", "a", 55, now.Add(2*time.Minute))
	s.notifyBudget(now.Add(2 * time.Minute))

	s.chat.Close()
	all := strings.Join(wait(-1), "\n")
	if n := strings.Count(all, "soft limit reached"); n != 1 {
		t.Errorf("soft notices = %d, want 1: %s", n, all)
	}
}

func TestServerNotices_IncreaseRearmsAgainstSpendBeyondOverride(t *testing.T) {
	s, wait := chatServer(t, 20, 0)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	s.seedBudgetNotes(now)
	s.ledger.record("", "a", 30, now)
	if _, err := s.budgetOverride(now); err != nil {
		t.Fatal(err)
	}
	s.ledger.record("", "a", 55, now.Add(time.Minute))
	s.notifyBudget(now.Add(time.Minute)) // 25 beyond: soft
	soft := 30.0
	if _, err := s.budgetIncrease(now.Add(time.Minute), BudgetIncreaseRequest{Soft: &soft}); err != nil {
		t.Fatal(err)
	}
	s.ledger.record("", "a", 62, now.Add(2*time.Minute))
	s.notifyBudget(now.Add(2 * time.Minute)) // 32 beyond the raised 30: soft again

	s.chat.Close()
	all := strings.Join(wait(-1), "\n")
	if n := strings.Count(all, "soft limit reached"); n != 2 {
		t.Errorf("soft notices = %d, want 2: %s", n, all)
	}
}

func TestServerNotices_DayRolloverSendsTheSummary(t *testing.T) {
	s, wait := chatServer(t, 0, 0)
	now := time.Date(2026, 10, 6, 23, 59, 0, 0, time.Local)
	s.seedBudgetNotes(now)
	s.ledger.record("", "a", 4, now)
	s.notifyBudget(now) // same day: nothing

	s.notifyBudget(now.Add(2 * time.Minute))
	s.chat.Close()
	got := wait(-1)
	if len(got) != 1 || !strings.Contains(got[0], "daily summary") || !strings.Contains(got[0], "2026-10-06") {
		t.Fatalf("sends = %v, want one daily summary for 2026-10-06", got)
	}
}

func TestNotifyPark_ShowsRealStatusKindAndReason(t *testing.T) {
	s, wait := chatServer(t, 0, 0)
	addr := tickets.Address{Project: "p", Epic: "epic", ID: "01"}

	s.notifyPark(addr, "", events.BudgetKilled, "daily budget hard limit reached")
	s.notifyPark(tickets.Address{Project: "p", Epic: "epic", ID: "02"}, "", events.AmbiguousBase, "choose a base")

	s.chat.Close()
	all := strings.Join(wait(-1), "\n")
	for _, want := range []string{"needs repair", string(events.BudgetKilled), "daily budget hard limit reached", "needs answer", string(events.AmbiguousBase), "choose a base"} {
		if !strings.Contains(all, want) {
			t.Errorf("chat lacks %q: %s", want, all)
		}
	}
	if strings.Contains(all, "iteration ended without landing") {
		t.Errorf("hardcoded reason leaked: %s", all)
	}
}

func TestParkFold_HerdrOutageParksBecomeOneDigest(t *testing.T) {
	s, wait := chatServer(t, 0, 0)
	s.herdr.unavailable = true
	park := func(project, id string, kind events.Kind) {
		s.notifyPark(tickets.Address{Project: project, Epic: "epic", ID: id}, "", kind, "why-"+project)
	}

	park("p1", "01", events.AgentNameTaken)
	park("p2", "02", events.HandleMismatch)
	park("p3", "03", events.AgentPaneBusy)
	park("p4", "04", events.ZeroCommit) // not herdr-caused: at once
	s.herdr.unavailable = false
	s.flushParkFold()

	s.chat.Close()
	all := strings.Join(wait(-1), "\n") // the batcher may merge both into one send
	if n := strings.Count(all, "parked while herdr was down"); n != 1 {
		t.Fatalf("digests = %d, want 1: %s", n, all)
	}
	for _, p := range []string{"p1", "p2", "p3", "p4"} {
		// Digest lines carry "[p]"; a live park sits under a bold "*p*" header.
		if strings.Count(all, "["+p+"]")+strings.Count(all, "*"+p+"*") != 1 {
			t.Errorf("%s not sent exactly once: %s", p, all)
		}
	}
}

func TestNotifyResult_OnlyWhenTheSubmitAskedForIt(t *testing.T) {
	s, wait := chatServer(t, 0, 0)
	dir := filepath.Join(s.cfg.TicketStore, "scratch", "e", "issues")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, filepath.Dir(dir))
	write := func(name string, notify bool) string {
		tk := schema.Ticket{ID: "01", Status: schema.StatusDone, Type: schema.TypePrompt, Notify: notify}
		out, err := schema.MarshalTicket(tk, "\nprompt\n\n## Result\n\nthe answer is 42\n")
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, out, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	addr := tickets.Address{Project: "scratch", Epic: "e", ID: "01"}

	s.notifyResult(addr, write("01-quiet.md", false))
	s.chat.Close()
	if got := wait(-1); len(got) != 0 {
		t.Fatalf("a ticket without --notify sent %v", got)
	}

	s, wait = chatServer(t, 0, 0)
	s.notifyResult(addr, write("01-loud.md", true))
	s.chat.Close()
	got := wait(1)
	if len(got) != 1 || !strings.Contains(got[0], "the answer is 42") {
		t.Fatalf("sends = %v, want the Result", got)
	}
}

func TestNotifyDone_EpicTicketSendsOneOffStaysSilent(t *testing.T) {
	s, wait := chatServer(t, 0, 0)
	proj := filepath.Join(s.cfg.TicketStore, "p")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "project.json"), []byte(`{"repo":"/x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	write := func(epic, name string, tk schema.Ticket) string {
		dir := filepath.Join(proj, epic, "issues")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		testutil.EnsureEpicTicketMD(t, filepath.Dir(dir))
		out, err := schema.MarshalTicket(tk, "\nbody\n")
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	landed := write("epic", "01-landed-ticket.md", schema.Ticket{ID: "01", Status: schema.StatusDone, Type: schema.TypeImplement})
	write("epic", "02-next.md", schema.Ticket{ID: "02", Status: schema.StatusOpen, Type: schema.TypeImplement})
	oneOff := write("cron", oneOffFile, schema.Ticket{ID: "01", Status: schema.StatusDone, Type: schema.TypePrompt})

	s.notifyDone(tickets.Address{Project: "p", Epic: "cron", ID: "01"}, oneOff)
	s.notifyDone(tickets.Address{Project: "p", Epic: "epic", ID: "01"}, landed)
	s.chat.Close()
	got := wait(1)
	if len(got) != 1 {
		t.Fatalf("sends = %v, want only the epic ticket's", got)
	}
	for _, want := range []string{"*p*", "Landed ticket", "1 done · 2 total", "p/epic/01"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("done message lacks %q: %s", want, got[0])
		}
	}
}

// crossEpicFixture is project "p" with epic "epic" whose two done tickets are
// blocked_by ref, a qualified ticket address in a second epic of the project
// whose only ticket, other/01, is done.
func crossEpicFixture(t *testing.T, s *Server, ref string) {
	t.Helper()
	proj := filepath.Join(s.cfg.TicketStore, "p")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "project.json"), []byte(`{"repo":"/x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	write := func(epic, name string, tk schema.Ticket) {
		dir := filepath.Join(proj, epic, "issues")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		testutil.EnsureEpicTicketMD(t, filepath.Dir(dir))
		out, err := schema.MarshalTicket(tk, "\nbody\n")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	blocked := []schema.TicketID{schema.TicketID(ref)}
	write("epic", "01-a.md", schema.Ticket{ID: "01", Status: schema.StatusDone, Type: schema.TypeImplement, BlockedBy: blocked})
	write("epic", "02-b.md", schema.Ticket{ID: "02", Status: schema.StatusDone, Type: schema.TypeImplement, BlockedBy: blocked})
	write("other", "01-c.md", schema.Ticket{ID: "01", Status: schema.StatusDone, Type: schema.TypeImplement})
}

func TestCompleteRootIfDone_ResolvedCrossEpicBlockersCompleteTheEpic(t *testing.T) {
	s, wait := chatServer(t, 0, 0)
	crossEpicFixture(t, s, "other/01")

	s.completeRootIfDone(rootRef{Project: "p", Epic: "epic"}, ralphloop.OneIteration{Epic: "epic"})
	s.chat.Close()
	got := wait(1)
	if len(got) != 1 || !strings.Contains(got[0], "epic complete") || !strings.Contains(got[0], "2 done · 2 total") {
		t.Fatalf("sends = %v, want one epic-complete message with 2 done · 2 total", got)
	}
}

func TestNotifyDone_CrossEpicBlockersDoNotUndercountDoneTickets(t *testing.T) {
	s, wait := chatServer(t, 0, 0)
	crossEpicFixture(t, s, "other/01")

	s.notifyDone(tickets.Address{Project: "p", Epic: "epic", ID: "02"}, "")
	s.chat.Close()
	got := wait(1)
	if len(got) != 1 || !strings.Contains(got[0], "2 done · 2 total") {
		t.Fatalf("sends = %v, want counts 2 done · 2 total", got)
	}
}

// A ref that names no ticket stays an error, which is never terminal.
func TestCompleteRootIfDone_UnresolvedCrossEpicBlockerKeepsTheEpicOpen(t *testing.T) {
	s, wait := chatServer(t, 0, 0)
	crossEpicFixture(t, s, "other/99")

	s.completeRootIfDone(rootRef{Project: "p", Epic: "epic"}, ralphloop.OneIteration{Epic: "epic"})
	s.chat.Close()
	if got := wait(-1); len(got) != 0 {
		t.Fatalf("sends = %v, want none", got)
	}
}

// The read path (index scan, then projectEpics) resolves a cross-epic blocker:
// a done ticket stays done instead of failing closed to error.
func TestProjectEpics_CrossEpicBlockerReadsAsDone(t *testing.T) {
	s, _ := chatServer(t, 0, 0)
	crossEpicFixture(t, s, "other/01")
	if err := s.idx.refresh(s.cfg.TicketStore); err != nil {
		t.Fatal(err)
	}

	epics, err := s.projectEpics("p")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range epics {
		for _, tk := range e.Tickets {
			if got := e.RenderedStatus(tk); got != tickets.StatusDone {
				t.Errorf("%s/%s = %v, want done", e.Name, tk.Identifier, got)
			}
		}
	}
}
