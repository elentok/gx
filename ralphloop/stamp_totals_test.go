package ralphloop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/tickets"
)

// stampTotalsFixture writes a done ticket with the given session_ids and
// returns its path and the worktree cwd every session runs in.
func stampTotalsFixture(t *testing.T, sessionIDs string) (ticketPath, cwd string) {
	t.Helper()
	// not parallel-safe: setHomeEnv mutates the process-wide $HOME env var.
	setHomeEnv(t, t.TempDir())
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "codex"))
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: done\ntype: implement\nsession_ids: " + sessionIDs + "\n---\n# A\n",
	})
	return filepath.Join(scratchDir, "epic", "issues", "01-a.md"), iterationWorktreePath("/fake/worktrees", "epic", "01")
}

func TestWriteMergedMetrics_StampsTokensAndCostOverEverySession(t *testing.T) {
	ticketPath, cwd := stampTotalsFixture(t, "[sess-1, sess-2]")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeFakeTranscript(t, "", cwd, "sess-1", start, [3]any{"claude-sonnet-5", 1000, 0})
	writeFakeTranscript(t, "", cwd, "sess-2", start, [3]any{"claude-sonnet-5", 2000, 5000})
	wantTokens, wantCost, err := SessionsTotals([]SessionRef{
		{Agent: AgentClaude, Cwd: cwd, SessionID: "sess-1"},
		{Agent: AgentClaude, Cwd: cwd, SessionID: "sess-2"},
	})
	if err != nil {
		t.Fatalf("SessionsTotals: %v", err)
	}

	m, ok, err := writeMergedMetrics(AgentClaude, cwd, "sess-2", ticketPath)
	if err != nil || !ok {
		t.Fatalf("writeMergedMetrics = %v, %v", ok, err)
	}

	got := mustParse(t, ticketPath)
	if got.TotalTokens == nil || int(*got.TotalTokens) != wantTokens {
		t.Errorf("TotalTokens = %v, want %d", got.TotalTokens, wantTokens)
	}
	if got.ActualCost != wantCost || m.cost != wantCost || !m.costStamped {
		t.Errorf("ActualCost = %v, returned %v (stamped %v), want %v", got.ActualCost, m.cost, m.costStamped, wantCost)
	}
	if got.HadCodex {
		t.Error("HadCodex = true for an all-Claude ticket")
	}
}

func TestWriteMergedMetrics_UnreadableSession_LeavesTicketUnstamped(t *testing.T) {
	ticketPath, cwd := stampTotalsFixture(t, "[sess-1, sess-missing, sess-2]")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeFakeTranscript(t, "", cwd, "sess-1", start, [3]any{"claude-sonnet-5", 1000, 0})
	writeFakeTranscript(t, "", cwd, "sess-2", start, [3]any{"claude-sonnet-5", 2000, 5000})

	m, ok, err := writeMergedMetrics(AgentClaude, cwd, "sess-2", ticketPath)
	if err != nil || !ok {
		t.Fatalf("writeMergedMetrics = %v, %v", ok, err)
	}

	got := mustParse(t, ticketPath)
	if got.TotalTokens != nil || got.ActualCost != 0 || m.costStamped {
		t.Errorf("TotalTokens = %v, ActualCost = %v, costStamped = %v; want none stamped", got.TotalTokens, got.ActualCost, m.costStamped)
	}
	if got.ActualContextWindow == 0 {
		t.Error("ActualContextWindow = 0, want the merging session's peak still stamped")
	}
}

func TestWriteMergedMetrics_MixedCodexTicket_StampsClaudeCostAndMarker(t *testing.T) {
	ticketPath, cwd := stampTotalsFixture(t, "[sess-claude, sess-codex]")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeFakeTranscript(t, "", cwd, "sess-claude", start, [3]any{"claude-sonnet-5", 1000, 0})
	claudeTokens, claudeCost, err := SessionsTotals([]SessionRef{{Agent: AgentClaude, Cwd: cwd, SessionID: "sess-claude"}})
	if err != nil {
		t.Fatalf("SessionsTotals: %v", err)
	}
	rollout := filepath.Join(os.Getenv("CODEX_HOME"), "sessions", "rollout-sess-codex.jsonl")
	if err := writeCodexSessionMeta(rollout, 0, "sess-codex", cwd); err != nil {
		t.Fatal(err)
	}
	if err := writeCodexUsageLine(rollout, 1, 500, 7000); err != nil {
		t.Fatal(err)
	}

	if _, _, err := writeMergedMetrics(AgentCodex, cwd, "sess-codex", ticketPath); err != nil {
		t.Fatalf("writeMergedMetrics: %v", err)
	}

	got := mustParse(t, ticketPath)
	if got.TotalTokens == nil || int(*got.TotalTokens) != claudeTokens+7000 {
		t.Errorf("TotalTokens = %v, want %d", got.TotalTokens, claudeTokens+7000)
	}
	if got.ActualCost != claudeCost {
		t.Errorf("ActualCost = %v, want only the Claude cost %v", got.ActualCost, claudeCost)
	}
	if !got.HadCodex {
		t.Error("HadCodex = false, want the Codex marker")
	}
}

func TestStampCommitlessMetrics_NoSession_StampsZero(t *testing.T) {
	ticketPath, cwd := stampTotalsFixture(t, "[]")
	p := iterationParams{WorktreeDir: "/fake/worktrees", FeatureBranch: "epic", Agent: AgentClaude,
		Ticket: tickets.Ticket{Identifier: "01", Path: ticketPath}, ScratchDir: filepath.Dir(filepath.Dir(filepath.Dir(ticketPath)))}

	stampCommitlessMetrics(p, cwd, "")

	got := mustParse(t, ticketPath)
	if got.TotalTokens == nil || *got.TotalTokens != 0 || got.ActualCost != 0 || got.HadCodex {
		t.Errorf("TotalTokens = %v, ActualCost = %v, HadCodex = %v; want a zero stamp", got.TotalTokens, got.ActualCost, got.HadCodex)
	}
}

func TestStampCommitlessMetrics_AllCodex_StampsTokensZeroCostAndMarker(t *testing.T) {
	ticketPath, cwd := stampTotalsFixture(t, "[sess-codex]")
	rollout := filepath.Join(os.Getenv("CODEX_HOME"), "sessions", "rollout-sess-codex.jsonl")
	if err := writeCodexSessionMeta(rollout, 0, "sess-codex", cwd); err != nil {
		t.Fatal(err)
	}
	if err := writeCodexUsageLine(rollout, 1, 500, 7000); err != nil {
		t.Fatal(err)
	}
	p := iterationParams{WorktreeDir: "/fake/worktrees", Agent: AgentCodex,
		Ticket: tickets.Ticket{Identifier: "01", Path: ticketPath}}

	stampCommitlessMetrics(p, cwd, "sess-codex")

	got := mustParse(t, ticketPath)
	if got.TotalTokens == nil || *got.TotalTokens != 7000 || got.ActualCost != 0 || !got.HadCodex {
		t.Errorf("TotalTokens = %v, ActualCost = %v, HadCodex = %v; want 7000, 0, true", got.TotalTokens, got.ActualCost, got.HadCodex)
	}
}
