package server_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tokenledger"
)

var pollDay1 = time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)

// tokenFixture runs one live iteration whose cumulative total the test sets.
type tokenFixture struct {
	h       *servertest.Harness
	fake    *runnerfake.Runner
	address string
	epic    string

	mu     sync.Mutex
	tokens int64
	cost   float64
	reads  int
}

func newTokenFixture(t *testing.T, epic string, opts ...func(*server.Config)) *tokenFixture {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", epic, "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", t.TempDir())
	fake := runnerfake.NewRunner()
	all := append([]func(*server.Config){
		servertest.WithRunner(fake),
		func(c *server.Config) { c.TokenPollInterval = time.Hour },
	}, opts...)
	h := servertest.StartWithStore(t, store, all...)
	f := &tokenFixture{h: h, fake: fake, address: "proj:" + epic + "/01", epic: epic}
	f.live(t)
	return f
}

// live makes the ticket a live, metered iteration of the current server: a
// nudged pane of a paused ticket.
func (f *tokenFixture) live(t *testing.T) {
	t.Helper()
	f.h.Server.SetTokensOf(func(ralphloop.SessionRef) (int64, float64, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.reads++
		return f.tokens, f.cost, nil
	})
	if err := f.h.Server.PauseAs(f.address, events.IterationError, "setup"); err != nil {
		t.Fatal(err)
	}
	label, _, _ := ralphloop.IterationIdentity(f.epic, "01", "")
	if _, ok, _ := f.fake.Find(label); !ok {
		if _, err := f.fake.Start(agentrunner.StartOptions{Label: label, Epic: f.epic, Cwd: t.TempDir(), Kind: "claude"}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := f.h.Client.TicketNudge(ctx, f.address, "go"); err != nil || res.Refused {
		t.Fatalf("nudge = %+v, %v", res, err)
	}
}

// grow sets the total and appends to the transcript, as a running agent does.
func (f *tokenFixture) grow(t *testing.T, tokens int64, cost float64) {
	t.Helper()
	f.mu.Lock()
	f.tokens, f.cost = tokens, cost
	f.mu.Unlock()
	path, ok := f.h.Server.TokenTranscriptPath(f.address)
	if !ok {
		t.Fatal("no live session")
	}
	appendLine(t, path)
}

func appendLine(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	fh, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	if _, err := fh.WriteString("{}\n"); err != nil {
		t.Fatal(err)
	}
}

func (f *tokenFixture) readCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads
}

func (f *tokenFixture) day(t *testing.T, now time.Time) []tokenledger.Row {
	t.Helper()
	l, err := tokenledger.Open(f.h.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := l.Day(now.Format(tokenledger.DayLayout))
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func wantRow(t *testing.T, rows []tokenledger.Row, group, harness string, tokens int64, dollars float64) {
	t.Helper()
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want one", rows)
	}
	r := rows[0]
	if r.Project != "proj" || r.Group != group || r.Harness != harness || r.Tokens != tokens || r.Dollars != dollars {
		t.Fatalf("row = %+v, want proj/%s/%s tokens %d dollars %v", r, group, harness, tokens, dollars)
	}
}

func TestTokenPoll_FirstSeenAddsNothingThenDelta(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, "epic-a")
	f.grow(t, 100, 1)
	f.h.Server.PollTokensAt(pollDay1)
	if rows := f.day(t, pollDay1); len(rows) != 0 {
		t.Fatalf("first poll wrote %+v", rows)
	}
	f.grow(t, 160, 1.5)
	f.h.Server.PollTokensAt(pollDay1)
	wantRow(t, f.day(t, pollDay1), tokenledger.GroupQueue, "claude", 60, 0.5)
}

func TestTokenPoll_DeltaGoesToTheDayOfThePoll(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, "epic-a")
	f.grow(t, 100, 1)
	f.h.Server.PollTokensAt(pollDay1)
	f.grow(t, 130, 1.25)
	f.h.Server.PollTokensAt(pollDay1)
	day2 := pollDay1.AddDate(0, 0, 1)
	f.grow(t, 200, 2)
	f.h.Server.PollTokensAt(day2)
	wantRow(t, f.day(t, pollDay1), tokenledger.GroupQueue, "claude", 30, 0.25)
	wantRow(t, f.day(t, day2), tokenledger.GroupQueue, "claude", 70, 0.75)
}

func TestTokenPoll_OneOffGroup(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, "one-offs")
	f.grow(t, 10, 0)
	f.h.Server.PollTokensAt(pollDay1)
	f.grow(t, 25, 0.5)
	f.h.Server.PollTokensAt(pollDay1)
	wantRow(t, f.day(t, pollDay1), tokenledger.GroupOneOff, "claude", 15, 0.5)
}

func TestTokenPoll_RestartDoesNotDoubleCount(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, "epic-a")
	f.grow(t, 100, 1)
	f.h.Server.PollTokensAt(pollDay1)
	f.grow(t, 150, 2)
	f.h.Server.PollTokensAt(pollDay1)

	f.h.Restart(t)
	f.live(t)
	f.h.Server.PollTokensAt(pollDay1) // the same total again
	wantRow(t, f.day(t, pollDay1), tokenledger.GroupQueue, "claude", 50, 1)
	f.grow(t, 170, 2)
	f.h.Server.PollTokensAt(pollDay1)
	wantRow(t, f.day(t, pollDay1), tokenledger.GroupQueue, "claude", 70, 1)
}

func TestTokenPoll_StoppedTicketKeepsItsTokens(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, "epic-a")
	f.grow(t, 100, 1)
	f.h.Server.PollTokensAt(pollDay1)
	f.grow(t, 180, 1)
	f.h.Server.PollTokensAt(pollDay1)

	label, _, _ := ralphloop.IterationIdentity(f.epic, "01", "")
	sess, _, _ := f.fake.Find(label)
	if err := f.fake.Stop(sess); err != nil {
		t.Fatal(err)
	}
	f.h.Server.PollTokensAt(pollDay1)
	wantRow(t, f.day(t, pollDay1), tokenledger.GroupQueue, "claude", 80, 0)
}

func TestTokenPoll_IdlePollReadsNothing(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, "epic-a")
	f.grow(t, 100, 1)
	f.h.Server.PollTokensAt(pollDay1)
	reads := f.readCount()
	if reads == 0 {
		t.Fatal("the first poll did not read")
	}
	for range 3 {
		f.h.Server.PollTokensAt(pollDay1)
	}
	if got := f.readCount(); got != reads {
		t.Fatalf("idle polls read %d times", got-reads)
	}
	f.grow(t, 120, 1)
	f.h.Server.PollTokensAt(pollDay1)
	if f.readCount() == reads {
		t.Fatal("a grown transcript was not read")
	}
}

func TestTokenPoll_CodexAddsTokensWithoutDollars(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, "epic-a")
	label, _, _ := ralphloop.IterationIdentity(f.epic, "01", "")
	dir := filepath.Join(nativerunner.AgentsRoot(f.h.StateDir, "proj"), label)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := nativerunner.WriteMeta(dir, nativerunner.Meta{Runner: "pty", Harness: harness.Codex, Label: label, Epic: f.epic}); err != nil {
		t.Fatal(err)
	}
	const id = "codex-conv-1"
	if err := os.WriteFile(filepath.Join(dir, harness.CodexSessionFile), []byte(`{"session_id":"`+id+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Not polled until the rollout exists.
	f.mu.Lock()
	f.tokens = 500
	f.mu.Unlock()
	f.h.Server.PollTokensAt(pollDay1)
	if f.readCount() != 0 {
		t.Fatal("polled a Codex session without a rollout")
	}
	rollout := filepath.Join(f.h.Env("HOME"), ".codex", "sessions", "2026", "10", "06", "rollout-2026-10-06T00-00-00-"+id+".jsonl")
	appendLine(t, rollout)
	f.h.Server.PollTokensAt(pollDay1) // first seen
	f.mu.Lock()
	f.tokens = 800
	f.mu.Unlock()
	appendLine(t, rollout)
	f.h.Server.PollTokensAt(pollDay1)
	wantRow(t, f.day(t, pollDay1), tokenledger.GroupQueue, "codex", 300, 0)
}

// Seam: nothing is polled before the migrations end, and polling starts after.
func TestTokenPoll_WaitsForMigrations(t *testing.T) {
	t.Parallel()
	hold, entered, release := heldMigration(t)
	f := newTokenFixture(t, "epic-a", hold, func(c *server.Config) { c.TokenPollInterval = 20 * time.Millisecond })
	<-entered
	f.grow(t, 100, 1)
	time.Sleep(200 * time.Millisecond)
	if n := f.readCount(); n != 0 {
		t.Fatalf("polled %d times while a migration was held", n)
	}
	release()
	waitMigrations(t, f.h)
	deadline := time.Now().Add(10 * time.Second)
	for f.readCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no poll after the migrations ended")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
