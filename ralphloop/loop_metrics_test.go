package ralphloop

import (
	"os"
	"path/filepath"
	"strconv"

	"testing"
	"time"

	eventsc "github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// compactedMergingFixture writes a transcript whose occupancy peaks before a
// compaction and ends far lower, and wires d's live readers to report the
// post-compaction (last) value, so a test can tell peak from last.
func compactedMergingFixture(t *testing.T, home, cwd, sessionID string) (d Deps, peak int) {
	t.Helper()
	fixture.WriteFakeTranscript(t, home, cwd, sessionID, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		[3]any{"claude-sonnet-5", 1000, 129000},
		[3]any{"claude-sonnet-5", 1000, 69000},
	)
	d, _, _ = fakeDeps()
	d.SessionHomes = SessionHomes{Claude: home}
	d.ReadOccupancy = func(cwd, sessionID string) (int, bool, error) { return 70000, true, nil }
	d.ReadCompactions = func(cwd, sessionID string) (int, bool, error) { return 1, true, nil }
	return d, 130000
}

// TestMergeAndMarkDone_CompactedSession_StampsPeakNotLast pins
// follow-ups-2026-10-11/04: closing a fresh iteration whose session compacted
// keeps mergeCherryPick's peak actual_context_window instead of overwriting it
// with the post-compaction occupancy.
func TestMergeAndMarkDone_CompactedSession_StampsPeakNotLast(t *testing.T) {
	scratchDir := fixture.WriteEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	ticketPath := filepath.Join(scratchDir, "epic", "issues", "01-a.md")
	sessionID := "sess-merge-01"
	cwd := iterationWorktreePath("/fake/worktrees", "epic", "01")
	d, peak := compactedMergingFixture(t, t.TempDir(), cwd, sessionID)
	p := iterationParams{
		WorktreeDir: "/fake/worktrees", FeatureWorktree: "/fake/feature", FeatureBranch: "epic", Agent: AgentClaude,
		Ticket: tickets.Ticket{Identifier: "01", Path: ticketPath}, ScratchDir: scratchDir, Sink: noopEventSink{},
	}

	if _, err := mergeCherryPick(d, p, "base", "branch", sessionID, "pane"); err != nil {
		t.Fatalf("mergeCherryPick() error = %v", err)
	}
	if err := markDoneStampingCloseMetadata(d, p, cwd, sessionID); err != nil {
		t.Fatalf("markDoneStampingCloseMetadata() error = %v", err)
	}

	got := fixture.MustParse(t, ticketPath)
	if got.Status != schema.StatusDone || got.ActualContextWindow != peak || got.Compactions != 1 {
		t.Errorf("got status=%q actual_context_window=%d compactions=%d, want done/%d/1", got.Status, got.ActualContextWindow, got.Compactions, peak)
	}
}

// TestMergeAndMarkDone_ReattachedCompactedSession_BackfillsPeak covers the
// reattached close: mergeCherryPick has no session to stamp from, so the
// backfill recovers the run log's session and must stamp its peak itself.
func TestMergeAndMarkDone_ReattachedCompactedSession_BackfillsPeak(t *testing.T) {
	scratchDir := fixture.WriteEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	ticketPath := filepath.Join(scratchDir, "epic", "issues", "01-a.md")
	sessionID := "sess-orig-01"
	cwd := "/fake/worktrees/epic-item-01"
	d, peak := compactedMergingFixture(t, t.TempDir(), cwd, sessionID)
	if err := logEvent(scratchDir, "epic", Event{
		Type: string(eventsc.IterationStarted), Ticket: "01", Agent: AgentClaude, AgentSession: sessionID, Cwd: cwd,
	}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	p := iterationParams{
		WorktreeDir: "/fake/worktrees", FeatureWorktree: "/fake/feature", FeatureBranch: "epic", Agent: AgentClaude,
		Ticket: tickets.Ticket{Identifier: "01", Path: ticketPath}, ScratchDir: scratchDir, Sink: noopEventSink{},
	}

	if _, err := mergeCherryPick(d, p, "base", "branch", "", ""); err != nil {
		t.Fatalf("mergeCherryPick() error = %v", err)
	}
	if err := markDoneStampingCloseMetadata(d, p, cwd, ""); err != nil {
		t.Fatalf("markDoneStampingCloseMetadata() error = %v", err)
	}

	got := fixture.MustParse(t, ticketPath)
	if got.Status != schema.StatusDone || got.ActualContextWindow != peak || got.Compactions != 1 {
		t.Errorf("got status=%q actual_context_window=%d compactions=%d, want done/%d/1", got.Status, got.ActualContextWindow, got.Compactions, peak)
	}
}

// TestMergeCherryPick_WritesActualContextWindowAndElapsedTimeToTicketFrontmatter
// verifies ticket 05a: mergeCherryPick reads the merging session's own
// transcript (report.go's sessionStats source, the same one `gx ralph-loop
// report` reads) and writes its peak context occupancy and wall-clock
// duration into the merged ticket's actual_context_window/elapsed_time
// frontmatter fields.
func TestMergeCherryPick_WritesActualContextWindowAndElapsedTimeToTicketFrontmatter(t *testing.T) {
	home := t.TempDir()

	scratchDir := fixture.WriteEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	ticketPath := filepath.Join(scratchDir, "epic", "issues", "01-a.md")

	sessionID := "sess-merge-01"
	cwd := iterationWorktreePath("/fake/worktrees", "epic", "01")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fixture.WriteFakeTranscript(t, home, cwd, sessionID, start,
		[3]any{"claude-sonnet-5", 1000, 0},
		[3]any{"claude-sonnet-5", 2000, 5000},
	)

	d, _, _ := fakeDeps()
	d.SessionHomes = SessionHomes{Claude: home}
	p := iterationParams{
		WorktreeDir:     "/fake/worktrees",
		FeatureWorktree: "/fake/feature",
		FeatureBranch:   "epic",
		Agent:           AgentClaude,
		Ticket:          tickets.Ticket{Identifier: "01", Path: ticketPath},
		ScratchDir:      scratchDir,
		Sink:            noopEventSink{},
	}

	if _, err := mergeCherryPick(d, p, "base", "branch", sessionID, "pane"); err != nil {
		t.Fatalf("mergeCherryPick() error = %v", err)
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

// TestMergeCherryPick_StampsTokensAndElapsedTrailers verifies ticket 05b:
// alongside the existing Ralph-Loop-Ticket trailer, mergeCherryPick stamps
// Ralph-Loop-Tokens/Ralph-Loop-Elapsed trailers whose values match what
// writeMergedMetrics wrote to the ticket's frontmatter, all in the merged
// commit's message via a single Deps.AppendTrailers call.
func TestMergeCherryPick_StampsTokensAndElapsedTrailers(t *testing.T) {
	home := t.TempDir()

	scratchDir := fixture.WriteEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	ticketPath := filepath.Join(scratchDir, "epic", "issues", "01-a.md")

	sessionID := "sess-merge-01"
	cwd := iterationWorktreePath("/fake/worktrees", "epic", "01")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fixture.WriteFakeTranscript(t, home, cwd, sessionID, start,
		[3]any{"claude-sonnet-5", 1000, 0},
		[3]any{"claude-sonnet-5", 2000, 5000},
	)

	repoDir := testutil.TempRepo(t)

	d, _, _ := fakeDeps()
	d.SessionHomes = SessionHomes{Claude: home}
	d.AppendTrailers = history.AppendTrailers
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

	if _, err := mergeCherryPick(d, p, "base", "branch", sessionID, "pane"); err != nil {
		t.Fatalf("mergeCherryPick() error = %v", err)
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
	if found, err := history.TrailerCommitExists(repoDir, "HEAD", tokensTrailerKey, wantTokens); err != nil {
		t.Fatalf("TrailerCommitExists(%s): %v", tokensTrailerKey, err)
	} else if !found {
		t.Errorf("merged commit missing %s: %s trailer", tokensTrailerKey, wantTokens)
	}
	if found, err := history.TrailerCommitExists(repoDir, "HEAD", elapsedTrailerKey, wantElapsed); err != nil {
		t.Fatalf("TrailerCommitExists(%s): %v", elapsedTrailerKey, err)
	} else if !found {
		t.Errorf("merged commit missing %s: %s trailer", elapsedTrailerKey, wantElapsed)
	}
	if found, err := history.TrailerCommitExists(repoDir, "HEAD", ticketTrailerKey, addressTrailerValue(scratchDir, "epic", "01")); err != nil {
		t.Fatalf("TrailerCommitExists(%s): %v", ticketTrailerKey, err)
	} else if !found {
		t.Errorf("merged commit missing %s trailer", ticketTrailerKey)
	}
}

// TestStampCommitlessMetrics_FreshSession_WritesActualContextWindowAndElapsedTime
// verifies ticket 04: the commitless branch of finishIteration reads its own
// finishing session's stats, the same way mergeCherryPick's writeMergedMetrics
// does for a committed finish, and stamps them into the ticket's frontmatter.
func TestStampCommitlessMetrics_FreshSession_WritesActualContextWindowAndElapsedTime(t *testing.T) {
	home := t.TempDir()

	scratchDir := fixture.WriteEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: done\ntype: code-review\ncommitless: true\n---\n# A\n",
	})
	ticketPath := filepath.Join(scratchDir, "epic", "issues", "01-a.md")

	sessionID := "sess-commitless-01"
	cwd := iterationWorktreePath("/fake/worktrees", "epic", "01")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fixture.WriteFakeTranscript(t, home, cwd, sessionID, start,
		[3]any{"claude-sonnet-5", 1000, 0},
		[3]any{"claude-sonnet-5", 2000, 5000},
	)

	p := iterationParams{
		WorktreeDir: "/fake/worktrees",
		Agent:       AgentClaude,
		Ticket:      tickets.Ticket{Identifier: "01", Path: ticketPath},
		ScratchDir:  scratchDir,
	}

	stampCommitlessMetrics(SessionHomes{Claude: home}, p, cwd, sessionID)

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

	scratchDir := fixture.WriteEpic(t, "epic", map[string]string{
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

	stampCommitlessMetrics(SessionHomes{Claude: home}, p, iterationWorktreePath("/fake/worktrees", "epic", "01"), "")

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
