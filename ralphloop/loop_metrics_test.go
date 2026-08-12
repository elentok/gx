package ralphloop

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// setHomeEnv points $HOME at dir for the rest of the test, restored on
// cleanup via a manual os.Setenv/os.Unsetenv pair rather than t.Setenv, which
// panics once a test has called t.Parallel(). Production code in this
// package (transcript.Path and friends) still resolves $HOME directly, so
// this remains a process-wide mutation — a test using it must stay
// non-parallel with any other test that also touches $HOME.
func setHomeEnv(t *testing.T, dir string) {
	t.Helper()
	prev, had := os.LookupEnv("HOME")
	if err := os.Setenv("HOME", dir); err != nil {
		t.Fatalf("Setenv HOME: %v", err)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv("HOME", prev)
		} else {
			os.Unsetenv("HOME")
		}
	})
}

// TestRun_FreshIteration_StampsContextWindowOnDone verifies ticket 06: a
// ticket marked done from a fresh (non-reattached) iteration, whose context
// occupancy is known this run, gets it written into its frontmatter's
// actual_context_window field alongside status: done.
func TestRun_FreshIteration_StampsContextWindowOnDone(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-fresh-01"}, nil
	}
	d.ReadOccupancy = func(cwd, sessionID string) (int, bool, error) {
		return 12345, true, nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := mustParse(t, filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if got.Status != schema.StatusDone {
		t.Errorf("Status = %q, want done", got.Status)
	}
	if got.ActualContextWindow != 12345 {
		t.Errorf("ActualContextWindow = %d, want 12345", got.ActualContextWindow)
	}
}

// TestRun_FreshIteration_FrontmatterTicket_EndsWithValidFrontmatter verifies
// ticket 07: a frontmatter-format ticket landed via the real finishIteration
// path (landCherryPick's writeLandedMetrics, then
// markDoneStampingCloseMetadata's MarkDoneWithMetadata, both writing into the
// same ticket file in one run) ends up with valid, gx-tickets-validate
// -passing frontmatter — status: done, and actual_context_window set from
// the closing occupancy — rather than a corrupted YAML block from the old
// line-splicing writer.
func TestRun_FreshIteration_FrontmatterTicket_EndsWithValidFrontmatter(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-fresh-01"}, nil
	}
	d.ReadOccupancy = func(cwd, sessionID string) (int, bool, error) {
		return 12345, true, nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	ticketPath := filepath.Join(scratchDir, "epic", "issues", "01-a.md")
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	ticket, err := schema.ParseTicketFromRaw(string(raw), ticketPath)
	if err != nil {
		t.Fatalf("ParseTicketFromRaw: %v (raw=%q)", err, raw)
	}
	if err := schema.Validate(ticket); err != nil {
		t.Errorf("Validate: %v (raw=%q)", err, raw)
	}
	if ticket.Status != schema.StatusDone {
		t.Errorf("Status = %q, want done", ticket.Status)
	}
	if ticket.ActualContextWindow != 12345 {
		t.Errorf("ActualContextWindow = %d, want 12345", ticket.ActualContextWindow)
	}
}

// TestRun_FreshIteration_OmitsContextWindowWhenOccupancyUnavailable verifies
// that a fresh iteration whose occupancy can't be read (ReadOccupancy's
// default fake behavior in fakeDeps) still marks the ticket done, without
// writing a wrong/placeholder actual_context_window.
func TestRun_FreshIteration_OmitsContextWindowWhenOccupancyUnavailable(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-fresh-01"}, nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := mustParse(t, filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if got.Status != schema.StatusDone {
		t.Errorf("Status = %q, want done", got.Status)
	}
	if got.ActualContextWindow != 0 {
		t.Errorf("ActualContextWindow = %d, want 0 (unavailable)", got.ActualContextWindow)
	}
}

// TestLandCherryPick_WritesActualContextWindowAndElapsedTimeToTicketFrontmatter
// verifies ticket 05a: landCherryPick reads the landing session's own
// transcript (report.go's sessionStats source, the same one `gx ralph-loop
// report` reads) and writes its peak context occupancy and wall-clock
// duration into the landed ticket's actual_context_window/elapsed_time
// frontmatter fields.
func TestLandCherryPick_WritesActualContextWindowAndElapsedTimeToTicketFrontmatter(t *testing.T) {
	home := t.TempDir()
	setHomeEnv(t, home)

	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	ticketPath := filepath.Join(scratchDir, "epic", "issues", "01-a.md")

	sessionID := "sess-land-01"
	cwd := iterationWorktreePath("/fake/worktrees", "epic", "01")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeFakeTranscript(t, "", cwd, sessionID, start,
		[3]any{"claude-sonnet-5", 1000, 0},
		[3]any{"claude-sonnet-5", 2000, 5000},
	)

	d, _, _ := fakeDeps()
	p := iterationParams{
		WorktreeDir:     "/fake/worktrees",
		FeatureWorktree: "/fake/feature",
		FeatureBranch:   "epic",
		Agent:           AgentClaude,
		Ticket:          tickets.Ticket{Identifier: "01", Path: ticketPath},
		ScratchDir:      scratchDir,
		Sink:            noopEventSink{},
	}

	if _, err := landCherryPick(d, p, "base", "branch", sessionID, "pane", "tab"); err != nil {
		t.Fatalf("landCherryPick() error = %v", err)
	}

	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	ticket, err := schema.ParseTicketFromRaw(string(raw), ticketPath)
	if err != nil {
		t.Fatalf("ParseTicketFromRaw: %v", err)
	}
	if ticket.ActualContextWindow == 0 {
		t.Errorf("ActualContextWindow = 0, want non-zero:\n%s", raw)
	}
	if ticket.ElapsedTime == 0 {
		t.Errorf("ElapsedTime = 0, want non-zero:\n%s", raw)
	}
}

// TestLandCherryPick_StampsTokensAndElapsedTrailers verifies ticket 05b:
// alongside the existing Ralph-Loop-Ticket trailer, landCherryPick stamps
// Ralph-Loop-Tokens/Ralph-Loop-Elapsed trailers whose values match what
// writeLandedMetrics wrote to the ticket's frontmatter, all in the landed
// commit's message via a single Deps.AppendTrailers call.
func TestLandCherryPick_StampsTokensAndElapsedTrailers(t *testing.T) {
	home := t.TempDir()
	setHomeEnv(t, home)

	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	ticketPath := filepath.Join(scratchDir, "epic", "issues", "01-a.md")

	sessionID := "sess-land-01"
	cwd := iterationWorktreePath("/fake/worktrees", "epic", "01")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeFakeTranscript(t, "", cwd, sessionID, start,
		[3]any{"claude-sonnet-5", 1000, 0},
		[3]any{"claude-sonnet-5", 2000, 5000},
	)

	repoDir := testutil.TempRepo(t)

	d, _, _ := fakeDeps()
	d.AppendTrailers = git.AppendTrailers
	d.RevParse = git.RevParse

	p := iterationParams{
		WorktreeDir:     "/fake/worktrees",
		FeatureWorktree: repoDir,
		FeatureBranch:   "epic",
		Agent:           AgentClaude,
		Ticket:          tickets.Ticket{Identifier: "01", Path: ticketPath},
		ScratchDir:      scratchDir,
		Sink:            noopEventSink{},
	}

	if _, err := landCherryPick(d, p, "base", "branch", sessionID, "pane", "tab"); err != nil {
		t.Fatalf("landCherryPick() error = %v", err)
	}

	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	ticket, err := schema.ParseTicketFromRaw(string(raw), ticketPath)
	if err != nil {
		t.Fatalf("ParseTicketFromRaw: %v", err)
	}

	wantTokens := strconv.Itoa(ticket.ActualContextWindow)
	wantElapsed := strconv.Itoa(ticket.ElapsedTime) + "s"
	if found, err := git.TrailerCommitExists(repoDir, "HEAD", tokensTrailerKey, wantTokens); err != nil {
		t.Fatalf("TrailerCommitExists(%s): %v", tokensTrailerKey, err)
	} else if !found {
		t.Errorf("landed commit missing %s: %s trailer", tokensTrailerKey, wantTokens)
	}
	if found, err := git.TrailerCommitExists(repoDir, "HEAD", elapsedTrailerKey, wantElapsed); err != nil {
		t.Fatalf("TrailerCommitExists(%s): %v", elapsedTrailerKey, err)
	} else if !found {
		t.Errorf("landed commit missing %s: %s trailer", elapsedTrailerKey, wantElapsed)
	}
	if found, err := git.TrailerCommitExists(repoDir, "HEAD", ticketTrailerKey, ticketTrailerValue("epic", "01")); err != nil {
		t.Fatalf("TrailerCommitExists(%s): %v", ticketTrailerKey, err)
	} else if !found {
		t.Errorf("landed commit missing %s trailer", ticketTrailerKey)
	}
}

// TestRun_IterationFinishedAndEpicComplete_ReceiveRealMetrics exercises
// ticket 03: IterationFinished's InProgress/Completed/Total counts track the
// scheduling loop's actual state (not the fixed zero placeholder ticket 02
// wired up), the landed ticket's ElapsedSeconds/PeakContextTokens come from
// its own frontmatter (written by landCherryPick's writeLandedMetrics before
// this event fires), and EpicComplete's elapsedSeconds is the run's real
// wall-clock duration rather than a hardcoded 0. Ticket 01 is deliberately
// held back from finishing (a fake AgentWait blocks on a channel) until
// ticket 02's own IterationFinished has already been recorded, so ticket 02's
// stats are captured while ticket 01 is still genuinely in progress.
func TestRun_IterationFinishedAndEpicComplete_ReceiveRealMetrics(t *testing.T) {
	home := t.TempDir()
	setHomeEnv(t, home)

	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# B\n",
	})

	d, _, _ := fakeDeps()
	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-" + opts.Pane}, nil
	}
	release01 := make(chan struct{})
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		if strings.Contains(opts.Target, iterLabel("epic", "01")) {
			<-release01
		}
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeFakeTranscript(t, "", iterationWorktreePath("/fake/worktrees", "epic", "01"), "sess-pane-"+iterLabel("epic", "01"), start,
		[3]any{"claude-sonnet-5", 1000, 0},
		[3]any{"claude-sonnet-5", 2000, 5000},
	)
	writeFakeTranscript(t, "", iterationWorktreePath("/fake/worktrees", "epic", "02"), "sess-pane-"+iterLabel("epic", "02"), start.Add(20*time.Second),
		[3]any{"claude-sonnet-5", 3000, 0},
		[3]any{"claude-sonnet-5", 4000, 9000},
	)

	clockTimes := []time.Time{start, start.Add(90 * time.Second)}
	var clockMu sync.Mutex
	call := 0
	d.Now = func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		got := clockTimes[call]
		if call < len(clockTimes)-1 {
			call++
		}
		return got
	}

	sink := &recordingSink{}
	var releaseOnce sync.Once
	sink.onIterationFinished = func(ticket tickets.Ticket) {
		if ticket.Identifier == "02" {
			releaseOnce.Do(func() { close(release01) })
		}
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	firstStats, ok := sink.iterationStatsByTicket["02"]
	if !ok {
		t.Fatalf("no IterationFinished recorded for ticket 02")
	}
	if firstStats.InProgress != 1 {
		t.Errorf("ticket 02 stats.InProgress = %d, want 1 (ticket 01 still running)", firstStats.InProgress)
	}
	if firstStats.Completed != 1 {
		t.Errorf("ticket 02 stats.Completed = %d, want 1", firstStats.Completed)
	}
	if firstStats.Total != 2 {
		t.Errorf("ticket 02 stats.Total = %d, want 2", firstStats.Total)
	}
	if firstStats.ElapsedSeconds == 0 {
		t.Errorf("ticket 02 stats.ElapsedSeconds = 0, want non-zero")
	}
	if firstStats.PeakContextTokens == 0 {
		t.Errorf("ticket 02 stats.PeakContextTokens = 0, want non-zero")
	}

	secondStats, ok := sink.iterationStatsByTicket["01"]
	if !ok {
		t.Fatalf("no IterationFinished recorded for ticket 01")
	}
	if secondStats.InProgress != 0 {
		t.Errorf("ticket 01 stats.InProgress = %d, want 0", secondStats.InProgress)
	}
	if secondStats.Completed != 2 {
		t.Errorf("ticket 01 stats.Completed = %d, want 2", secondStats.Completed)
	}
	if secondStats.Total != 2 {
		t.Errorf("ticket 01 stats.Total = %d, want 2", secondStats.Total)
	}
	if secondStats.ElapsedSeconds == 0 {
		t.Errorf("ticket 01 stats.ElapsedSeconds = 0, want non-zero")
	}
	if secondStats.PeakContextTokens == 0 {
		t.Errorf("ticket 01 stats.PeakContextTokens = 0, want non-zero")
	}

	if sink.lastEpicElapsedSeconds != 90 {
		t.Errorf("EpicComplete elapsedSeconds = %d, want 90 (scripted wall-clock duration)", sink.lastEpicElapsedSeconds)
	}
}

func TestRun_LogsDepsInstalledEventWithCommand(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	d.InstallDeps = func(path string) (string, error) {
		return "npm ci", nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	var found *Event
	for i, ev := range events {
		if ev.Type == eventDepsInstalled {
			found = &events[i]
		}
	}
	if found == nil {
		t.Fatalf("events = %+v, want a deps-installed event", events)
	}
	if found.Reason != "npm ci" {
		t.Errorf("deps-installed event Reason = %q, want the command run", found.Reason)
	}
}

// TestStampCommitlessMetrics_FreshSession_WritesActualContextWindowAndElapsedTime
// verifies ticket 04: the commitless branch of finishIteration reads its own
// finishing session's stats, the same way landCherryPick's writeLandedMetrics
// does for a committed finish, and stamps them into the ticket's frontmatter.
func TestStampCommitlessMetrics_FreshSession_WritesActualContextWindowAndElapsedTime(t *testing.T) {
	home := t.TempDir()
	setHomeEnv(t, home)

	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: done\ntype: code-review\ncommitless: true\n---\n# A\n",
	})
	ticketPath := filepath.Join(scratchDir, "epic", "issues", "01-a.md")

	sessionID := "sess-commitless-01"
	cwd := iterationWorktreePath("/fake/worktrees", "epic", "01")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeFakeTranscript(t, "", cwd, sessionID, start,
		[3]any{"claude-sonnet-5", 1000, 0},
		[3]any{"claude-sonnet-5", 2000, 5000},
	)

	p := iterationParams{
		WorktreeDir: "/fake/worktrees",
		Agent:       AgentClaude,
		Ticket:      tickets.Ticket{Identifier: "01", Path: ticketPath},
		ScratchDir:  scratchDir,
	}

	stampCommitlessMetrics(p, cwd, sessionID)

	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	ticket, err := schema.ParseTicketFromRaw(string(raw), ticketPath)
	if err != nil {
		t.Fatalf("ParseTicketFromRaw: %v", err)
	}
	if ticket.ActualContextWindow == 0 {
		t.Errorf("ActualContextWindow = 0, want non-zero:\n%s", raw)
	}
	if ticket.ElapsedTime == 0 {
		t.Errorf("ElapsedTime = 0, want non-zero:\n%s", raw)
	}
}

// TestStampCommitlessMetrics_NoDiscoverableSession_LeavesFieldsZeroWithoutError
// verifies ticket 04's fallback: a commitless finish with no session to read
// (no sessionID and no prior iteration-started event to backfill from)
// leaves actual_context_window/elapsed_time at 0 rather than erroring.
func TestStampCommitlessMetrics_NoDiscoverableSession_LeavesFieldsZeroWithoutError(t *testing.T) {
	home := t.TempDir()
	setHomeEnv(t, home)

	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: done\ntype: code-review\ncommitless: true\n---\n# A\n",
	})
	ticketPath := filepath.Join(scratchDir, "epic", "issues", "01-a.md")

	p := iterationParams{
		WorktreeDir:   "/fake/worktrees",
		FeatureBranch: "epic",
		Agent:         AgentClaude,
		Ticket:        tickets.Ticket{Identifier: "01", Path: ticketPath},
		ScratchDir:    scratchDir,
	}

	stampCommitlessMetrics(p, iterationWorktreePath("/fake/worktrees", "epic", "01"), "")

	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	ticket, err := schema.ParseTicketFromRaw(string(raw), ticketPath)
	if err != nil {
		t.Fatalf("ParseTicketFromRaw: %v", err)
	}
	if ticket.ActualContextWindow != 0 {
		t.Errorf("ActualContextWindow = %d, want 0 (no discoverable session)", ticket.ActualContextWindow)
	}
	if ticket.ElapsedTime != 0 {
		t.Errorf("ElapsedTime = %d, want 0 (no discoverable session)", ticket.ElapsedTime)
	}
	if ticket.Status != schema.StatusDone {
		t.Errorf("Status = %q, want done (finish still completes cleanly)", ticket.Status)
	}
}
