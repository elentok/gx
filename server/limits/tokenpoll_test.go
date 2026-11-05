package limits_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/metrics"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tokenledger"
	"github.com/elentok/gx/transcript"
)

var pollDay1 = time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)

// tokenFixture runs one live iteration whose cumulative total the test sets.
type tokenFixture struct {
	h       *servertest.Harness
	fake    *runnerfake.Runner
	address string
	epic    string
	store   string

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
	f := &tokenFixture{h: h, fake: fake, address: "proj:" + epic + "/01", epic: epic, store: store}
	f.live(t)
	return f
}

// live makes the ticket a live, metered iteration of the current server: a
// nudged pane of a paused ticket.
func (f *tokenFixture) live(t *testing.T) {
	t.Helper()
	f.h.Server.Limits().SetTokensOf(func(metrics.SessionRef) (int64, float64, error) {
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
	if res, err := f.h.Client.Tickets.Nudge(ctx, f.address, "go"); err != nil || res.Refused {
		t.Fatalf("nudge = %+v, %v", res, err)
	}
}

// grow sets the total and appends to the transcript, as a running agent does.
func (f *tokenFixture) grow(t *testing.T, tokens int64, cost float64) {
	t.Helper()
	f.mu.Lock()
	f.tokens, f.cost = tokens, cost
	f.mu.Unlock()
	path, ok := f.h.Server.Limits().TokenTranscriptPath(f.address)
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
	f.h.Server.Limits().PollTokensAt(pollDay1)
	if rows := f.day(t, pollDay1); len(rows) != 0 {
		t.Fatalf("first poll wrote %+v", rows)
	}
	f.grow(t, 160, 1.5)
	f.h.Server.Limits().PollTokensAt(pollDay1)
	wantRow(t, f.day(t, pollDay1), tokenledger.GroupQueue, "claude", 60, 0.5)
}

func TestTokenPoll_DeltaGoesToTheDayOfThePoll(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, "epic-a")
	f.grow(t, 100, 1)
	f.h.Server.Limits().PollTokensAt(pollDay1)
	f.grow(t, 130, 1.25)
	f.h.Server.Limits().PollTokensAt(pollDay1)
	day2 := pollDay1.AddDate(0, 0, 1)
	f.grow(t, 200, 2)
	f.h.Server.Limits().PollTokensAt(day2)
	wantRow(t, f.day(t, pollDay1), tokenledger.GroupQueue, "claude", 30, 0.25)
	wantRow(t, f.day(t, day2), tokenledger.GroupQueue, "claude", 70, 0.75)
}

func TestTokenPoll_OneOffGroup(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, "one-offs")
	f.grow(t, 10, 0)
	f.h.Server.Limits().PollTokensAt(pollDay1)
	f.grow(t, 25, 0.5)
	f.h.Server.Limits().PollTokensAt(pollDay1)
	wantRow(t, f.day(t, pollDay1), tokenledger.GroupOneOff, "claude", 15, 0.5)
}

func TestTokenPoll_RestartDoesNotDoubleCount(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, "epic-a")
	f.grow(t, 100, 1)
	f.h.Server.Limits().PollTokensAt(pollDay1)
	f.grow(t, 150, 2)
	f.h.Server.Limits().PollTokensAt(pollDay1)

	f.h.Restart(t)
	f.live(t)
	f.h.Server.Limits().PollTokensAt(pollDay1) // the same total again
	wantRow(t, f.day(t, pollDay1), tokenledger.GroupQueue, "claude", 50, 1)
	f.grow(t, 170, 2)
	f.h.Server.Limits().PollTokensAt(pollDay1)
	wantRow(t, f.day(t, pollDay1), tokenledger.GroupQueue, "claude", 70, 1)
}

func TestTokenPoll_StoppedTicketKeepsItsTokens(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, "epic-a")
	f.grow(t, 100, 1)
	f.h.Server.Limits().PollTokensAt(pollDay1)
	f.grow(t, 180, 1)
	f.h.Server.Limits().PollTokensAt(pollDay1)

	label, _, _ := ralphloop.IterationIdentity(f.epic, "01", "")
	sess, _, _ := f.fake.Find(label)
	if err := f.fake.Stop(sess); err != nil {
		t.Fatal(err)
	}
	f.h.Server.Limits().PollTokensAt(pollDay1)
	wantRow(t, f.day(t, pollDay1), tokenledger.GroupQueue, "claude", 80, 0)
}

func TestTokenPoll_CancelledTicketKeepsItsTokens(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, "epic-a")
	f.grow(t, 100, 1)
	f.h.Server.Limits().PollTokensAt(pollDay1)
	f.grow(t, 180, 1)
	f.h.Server.Limits().PollTokensAt(pollDay1)

	servertest.WriteTicketWith(t, f.store, "proj", f.epic, "01", "first", servertest.TicketOpts{Status: "cancelled", CancelledAt: "2026-10-06T13:00:00Z"})
	f.h.Server.Limits().PollTokensAt(pollDay1)
	wantRow(t, f.day(t, pollDay1), tokenledger.GroupQueue, "claude", 80, 0)
}

func TestTokenPoll_StartedOverTicketKeepsItsTokens(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, "epic-a")
	f.grow(t, 100, 1)
	f.h.Server.Limits().PollTokensAt(pollDay1)
	f.grow(t, 180, 1)
	f.h.Server.Limits().PollTokensAt(pollDay1)

	// A start-over resets the ticket to open and drops its session.
	servertest.WriteTicketWith(t, f.store, "proj", f.epic, "01", "first", servertest.TicketOpts{})
	f.h.Server.Limits().PollTokensAt(pollDay1)
	wantRow(t, f.day(t, pollDay1), tokenledger.GroupQueue, "claude", 80, 0)
}

func storeFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestTokenPoll_LedgerLivesInStateDirNotTicketStore(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, "epic-a")
	before := storeFiles(t, f.store)
	f.grow(t, 100, 1)
	f.h.Server.Limits().PollTokensAt(pollDay1)
	f.grow(t, 160, 1.5)
	f.h.Server.Limits().PollTokensAt(pollDay1)
	wantRow(t, f.day(t, pollDay1), tokenledger.GroupQueue, "claude", 60, 0.5)
	if after := storeFiles(t, f.store); !slices.Equal(before, after) {
		t.Errorf("ticket store files = %v, want %v", after, before)
	}
}

func TestTokenPoll_IdlePollReadsNothing(t *testing.T) {
	t.Parallel()
	f := newTokenFixture(t, "epic-a")
	f.grow(t, 100, 1)
	f.h.Server.Limits().PollTokensAt(pollDay1)
	reads := f.readCount()
	if reads == 0 {
		t.Fatal("the first poll did not read")
	}
	for range 3 {
		f.h.Server.Limits().PollTokensAt(pollDay1)
	}
	if got := f.readCount(); got != reads {
		t.Fatalf("idle polls read %d times", got-reads)
	}
	f.grow(t, 120, 1)
	f.h.Server.Limits().PollTokensAt(pollDay1)
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
	f.h.Server.Limits().PollTokensAt(pollDay1)
	if f.readCount() != 0 {
		t.Fatal("polled a Codex session without a rollout")
	}
	rollout := filepath.Join(f.h.Env("HOME"), ".codex", "sessions", "2026", "10", "06", "rollout-2026-10-06T00-00-00-"+id+".jsonl")
	appendLine(t, rollout)
	f.h.Server.Limits().PollTokensAt(pollDay1) // first seen
	f.mu.Lock()
	f.tokens = 800
	f.mu.Unlock()
	appendLine(t, rollout)
	f.h.Server.Limits().PollTokensAt(pollDay1)
	wantRow(t, f.day(t, pollDay1), tokenledger.GroupQueue, "codex", 300, 0)
}

// sessionFixture meters interactive sessions written straight to the sessions dir.
type sessionFixture struct {
	h      *servertest.Harness
	mu     sync.Mutex
	totals map[string]int64 // conversation id -> cumulative tokens
}

func newSessionFixture(t *testing.T) *sessionFixture {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	if err := os.MkdirAll(filepath.Join(store, "proj", "design", "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "proj", "design", "ticket.md"), []byte("---\nkind: map\nstatus: open\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.TokenPollInterval = time.Hour })
	// The socket answers only after the startup restore of sessions, which would
	// end the holderless sessions written below.
	if _, err := h.Client.Snapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	f := &sessionFixture{h: h, totals: map[string]int64{}}
	h.Server.Limits().SetTokensOf(func(ref metrics.SessionRef) (int64, float64, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.totals[ref.SessionID], 0, nil
	})
	return f
}

// add writes a session's meta; its transcript starts empty.
func (f *sessionFixture) add(t *testing.T, id, harnessName, project, ticket string, extra map[string]any) {
	t.Helper()
	dir := t.TempDir()
	meta := map[string]any{
		"id": id, "harness": harnessName, "project": project, "dir": dir, "ticket": ticket,
		"conversation_id": "conv-" + id, "created_at": pollDay1,
	}
	for k, v := range extra {
		meta[k] = v
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	sdir := filepath.Join(f.h.SessionsDir, id)
	if err := os.MkdirAll(sdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdir, "meta.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if harnessName == "codex" {
		appendLine(t, filepath.Join(f.h.Env("HOME"), ".codex", "sessions", "2026", "10", "06", "rollout-2026-10-06T00-00-00-conv-"+id+".jsonl"))
		return
	}
	appendLine(t, transcript.PathIn(f.h.Env("HOME"), dir, "conv-"+id))
}

// grow sets the session's total and touches its transcript.
func (f *sessionFixture) grow(t *testing.T, id, harnessName string, tokens int64) {
	t.Helper()
	f.mu.Lock()
	f.totals["conv-"+id] = tokens
	f.mu.Unlock()
	meta := map[string]any{}
	data, err := os.ReadFile(filepath.Join(f.h.SessionsDir, id, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if harnessName == "codex" {
		appendLine(t, filepath.Join(f.h.Env("HOME"), ".codex", "sessions", "2026", "10", "06", "rollout-2026-10-06T00-00-00-conv-"+id+".jsonl"))
		return
	}
	appendLine(t, transcript.PathIn(f.h.Env("HOME"), meta["dir"].(string), "conv-"+id))
}

func (f *sessionFixture) rows(t *testing.T) map[string]tokenledger.Row {
	t.Helper()
	l, err := tokenledger.Open(f.h.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := l.Day(pollDay1.Format(tokenledger.DayLayout))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]tokenledger.Row{}
	for _, r := range rows {
		out[r.Project+"/"+r.Group+"/"+r.Harness] = r
	}
	return out
}

func (f *sessionFixture) poll(t *testing.T, id, harnessName string, first, second int64) {
	t.Helper()
	f.grow(t, id, harnessName, first)
	f.h.Server.Limits().PollTokensAt(pollDay1)
	f.grow(t, id, harnessName, second)
	f.h.Server.Limits().PollTokensAt(pollDay1)
}

func TestTokenPoll_InteractiveSessionGroups(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.add(t, "d", "claude", "proj", "proj:design", nil)
	f.add(t, "dt", "claude", "proj", "proj:design/01", nil)
	f.add(t, "o", "claude", "proj", "proj:epic-a/01", nil)
	f.add(t, "n", "claude", "proj", "", nil)
	f.add(t, "np", "claude", "", "", nil)
	f.add(t, "cx", "codex", "proj", "", nil)
	for _, id := range []string{"d", "dt", "o", "n", "np"} {
		f.grow(t, id, "claude", 100)
	}
	f.grow(t, "cx", "codex", 100)
	f.h.Server.Limits().PollTokensAt(pollDay1)
	for _, id := range []string{"d", "dt", "o", "n", "np"} {
		f.grow(t, id, "claude", 130)
	}
	f.grow(t, "cx", "codex", 150)
	f.h.Server.Limits().PollTokensAt(pollDay1)

	rows := f.rows(t)
	want := map[string]int64{
		"proj/Design/claude": 60,
		"proj/Other/claude":  60,
		"/Other/claude":      30,
		"proj/Other/codex":   50,
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v", rows)
	}
	for k, tokens := range want {
		if r, ok := rows[k]; !ok || r.Tokens != tokens || r.Dollars != 0 {
			t.Errorf("%s = %+v (found %v), want %d tokens, $0", k, r, ok, tokens)
		}
	}
}

func TestTokenPoll_InteractiveGroupIsFixedAtFirstSight(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.add(t, "s", "claude", "proj", "", nil)
	f.poll(t, "s", "claude", 100, 150)
	// The person links the session to a map ticket later.
	f.add(t, "s", "claude", "proj", "proj:design/01", map[string]any{"conversation_id": "conv-s"})
	f.grow(t, "s", "claude", 200)
	f.h.Server.Limits().PollTokensAt(pollDay1)
	rows := f.rows(t)
	if r := rows["proj/Other/claude"]; r.Tokens != 100 || len(rows) != 1 {
		t.Fatalf("rows = %+v, want 100 Other tokens only", rows)
	}
}

func TestTokenPoll_ExitedSessionsAreNotPolled(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.add(t, "x", "claude", "proj", "", map[string]any{"exit": "exited"})
	f.add(t, "st", "claude", "proj", "", map[string]any{"stopped": true})
	f.poll(t, "x", "claude", 100, 200)
	f.poll(t, "st", "claude", 100, 200)
	if rows := f.rows(t); len(rows) != 0 {
		t.Fatalf("rows = %+v, want none", rows)
	}
}

// Seam: nothing is polled before the migrations end, and polling starts after.
func TestTokenPoll_WaitsForMigrations(t *testing.T) {
	t.Parallel()
	// The live iteration is set up first: a held migration holds the project
	// lock, which PauseAs needs. The held step joins the registry for the restart.
	reg := &migrations.Registry{}
	f := newTokenFixture(t, "epic-a",
		func(c *server.Config) { c.Migrations = reg },
		func(c *server.Config) { c.TokenPollInterval = 20 * time.Millisecond })
	entered := make(chan struct{})
	gate := make(chan struct{})
	var once, relOnce sync.Once
	release := func() { relOnce.Do(func() { close(gate) }) }
	t.Cleanup(release)
	reg.Register(migrations.Step{Name: "hold", Run: func(migrations.Env) ([]tickets.MigrationChange, error) {
		once.Do(func() { close(entered) })
		<-gate
		return nil, nil
	}})
	f.h.Restart(t)
	f.h.Server.Limits().SetTokensOf(func(metrics.SessionRef) (int64, float64, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.reads++
		return f.tokens, f.cost, nil
	})
	<-entered
	// The ticket is still paused on disk, so the nudge makes it live again.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := f.h.Client.Tickets.Nudge(ctx, f.address, "go"); err != nil || res.Refused {
		t.Fatalf("nudge = %+v, %v", res, err)
	}
	f.grow(t, 100, 1)
	time.Sleep(200 * time.Millisecond)
	if n := f.readCount(); n != 0 {
		t.Fatalf("polled %d times while a migration was held", n)
	}
	release()
	servertest.WaitMigrations(t, f.h)
	deadline := time.Now().Add(10 * time.Second)
	for f.readCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no poll after the migrations ended")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
