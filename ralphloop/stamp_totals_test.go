package ralphloop

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/tickets"
)

// stampTotalsFixture writes a done ticket with the given session_ids and
// returns its path, the worktree cwd every session runs in and the temp homes
// the sessions live under.
func stampTotalsFixture(t *testing.T, sessionIDs string) (ticketPath, cwd string, homes SessionHomes) {
	t.Helper()
	homes = SessionHomes{Claude: t.TempDir(), Codex: filepath.Join(t.TempDir(), "codex")}
	// recordedConversationTotals still reads the conversations dir from the
	// process HOME.
	fixture.SetHomeEnv(t, t.TempDir())
	scratchDir := fixture.WriteEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: done\ntype: implement\nsession_ids: " + sessionIDs + "\n---\n# A\n",
	})
	return filepath.Join(scratchDir, "epic", "issues", "01-a.md"), iterationWorktreePath("/fake/worktrees", "epic", "01"), homes
}

func TestWriteMergedMetrics_StampsTokensAndCostOverEverySession(t *testing.T) {
	ticketPath, cwd, homes := stampTotalsFixture(t, "[sess-1, sess-2]")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fixture.WriteFakeTranscript(t, homes.Claude, cwd, "sess-1", start, [3]any{"claude-sonnet-5", 1000, 0})
	fixture.WriteFakeTranscript(t, homes.Claude, cwd, "sess-2", start, [3]any{"claude-sonnet-5", 2000, 5000})
	wantTokens, wantCost, err := SessionsTotals(homes, []SessionRef{
		{Agent: agent.AgentClaude, Cwd: cwd, SessionID: "sess-1"},
		{Agent: agent.AgentClaude, Cwd: cwd, SessionID: "sess-2"},
	})
	if err != nil {
		t.Fatalf("SessionsTotals: %v", err)
	}

	m, ok, err := writeMergedMetrics(homes, agent.AgentClaude, cwd, "sess-2", ticketPath)
	if err != nil || !ok {
		t.Fatalf("writeMergedMetrics = %v, %v", ok, err)
	}

	got := fixture.MustParse(t, ticketPath)
	if got.TotalTokens == nil || *got.TotalTokens != wantTokens {
		t.Errorf("TotalTokens = %v, want %d", got.TotalTokens, wantTokens)
	}
	if got.ActualCost != wantCost || m.cost != wantCost || !m.costStamped {
		t.Errorf("ActualCost = %v, returned %v (stamped %v), want %v", got.ActualCost, m.cost, m.costStamped, wantCost)
	}
	if got.HadCodex {
		t.Error("HadCodex = true for an all-Claude ticket")
	}
}

func TestWriteMergedMetrics_PrefixTokens(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		sessionIDs string
		turns      [][3]any
		want       int
	}{
		{"first call, not the peak", "[sess-1]", [][3]any{{"claude-sonnet-5", 40000, 0}, {"claude-sonnet-5", 90000, 0}}, 40000},
		{"empty without a first call", "[sess-1]", nil, 0},
		{"empty for a resumed session", "[sess-1, sess-1]", [][3]any{{"claude-sonnet-5", 40000, 0}, {"claude-sonnet-5", 90000, 0}}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ticketPath, cwd, homes := stampTotalsFixture(t, c.sessionIDs)
			turns := c.turns
			if turns == nil {
				// A transcript with no assistant usage line still has to exist.
				turns = [][3]any{{"claude-sonnet-5", 0, 0}}
			}
			fixture.WriteFakeTranscript(t, homes.Claude, cwd, "sess-1", start, turns...)
			if _, _, err := writeMergedMetrics(homes, agent.AgentClaude, cwd, "sess-1", ticketPath); err != nil {
				t.Fatal(err)
			}
			if got := fixture.MustParse(t, ticketPath).PrefixTokens; got != c.want {
				t.Errorf("PrefixTokens = %d, want %d", got, c.want)
			}
		})
	}
}

func TestWriteMergedMetrics_PrefixTokens_FromThePeakSession(t *testing.T) {
	ticketPath, cwd, homes := stampTotalsFixture(t, "[sess-1, sess-2]")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fixture.WriteFakeTranscript(t, homes.Claude, cwd, "sess-1", start, [3]any{"claude-sonnet-5", 10000, 0})
	fixture.WriteFakeTranscript(t, homes.Claude, cwd, "sess-2", start, [3]any{"claude-sonnet-5", 45000, 0}, [3]any{"claude-sonnet-5", 90000, 0})

	if _, _, err := writeMergedMetrics(homes, agent.AgentClaude, cwd, "sess-2", ticketPath); err != nil {
		t.Fatal(err)
	}
	if got := fixture.MustParse(t, ticketPath).PrefixTokens; got != 45000 {
		t.Errorf("PrefixTokens = %d, want the merging session's 45000", got)
	}
}

func TestWriteMergedMetrics_CodexStampsPrefixTokensFromFirstTokenCount(t *testing.T) {
	ticketPath, cwd, homes := stampTotalsFixture(t, "[sess-codex]")
	rollout := filepath.Join(homes.Codex, "sessions", "rollout-sess-codex.jsonl")
	if err := fixture.WriteCodexSessionMeta(rollout, 0, "sess-codex", cwd); err != nil {
		t.Fatal(err)
	}
	if err := fixture.WriteCodexUsageLine(rollout, 1, 500, 7000); err != nil {
		t.Fatal(err)
	}
	if _, _, err := writeMergedMetrics(homes, agent.AgentCodex, cwd, "sess-codex", ticketPath); err != nil {
		t.Fatal(err)
	}
	if got := fixture.MustParse(t, ticketPath).PrefixTokens; got != 500 {
		t.Errorf("PrefixTokens = %d, want 500", got)
	}
}

func TestWriteMergedMetrics_UnreadableSession_LeavesTicketUnstamped(t *testing.T) {
	ticketPath, cwd, homes := stampTotalsFixture(t, "[sess-1, sess-missing, sess-2]")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fixture.WriteFakeTranscript(t, homes.Claude, cwd, "sess-1", start, [3]any{"claude-sonnet-5", 1000, 0})
	fixture.WriteFakeTranscript(t, homes.Claude, cwd, "sess-2", start, [3]any{"claude-sonnet-5", 2000, 5000})

	m, ok, err := writeMergedMetrics(homes, agent.AgentClaude, cwd, "sess-2", ticketPath)
	if err != nil || !ok {
		t.Fatalf("writeMergedMetrics = %v, %v", ok, err)
	}

	got := fixture.MustParse(t, ticketPath)
	if got.TotalTokens != nil || got.ActualCost != 0 || m.costStamped {
		t.Errorf("TotalTokens = %v, ActualCost = %v, costStamped = %v; want none stamped", got.TotalTokens, got.ActualCost, m.costStamped)
	}
	if got.ActualContextWindow == 0 {
		t.Error("ActualContextWindow = 0, want the merging session's peak still stamped")
	}
}

func recordConversation(t *testing.T, ticketPath string, totals *conversations.Totals) {
	t.Helper()
	addr, ok := tickets.AddressOfPath(ticketPath, "01")
	if !ok {
		t.Fatalf("no address for %s", ticketPath)
	}
	root, err := conversations.DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := conversations.Save(root, conversations.Record{ID: "conv-1", Ticket: addr.String(), Totals: totals}); err != nil {
		t.Fatal(err)
	}
}

func TestWriteMergedMetrics_RecordedConversation_AddsToStamp(t *testing.T) {
	ticketPath, cwd, homes := stampTotalsFixture(t, "[sess-1]")
	fixture.WriteFakeTranscript(t, homes.Claude, cwd, "sess-1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), [3]any{"claude-sonnet-5", 1000, 0})
	sessTokens, sessCost, err := SessionsTotals(homes, []SessionRef{{Agent: agent.AgentClaude, Cwd: cwd, SessionID: "sess-1"}})
	if err != nil {
		t.Fatalf("SessionsTotals: %v", err)
	}
	recordConversation(t, ticketPath, &conversations.Totals{TotalTokens: 300, Cost: 0.5})

	if _, _, err := writeMergedMetrics(homes, agent.AgentClaude, cwd, "sess-1", ticketPath); err != nil {
		t.Fatalf("writeMergedMetrics: %v", err)
	}

	got := fixture.MustParse(t, ticketPath)
	if got.TotalTokens == nil || *got.TotalTokens != sessTokens+300 || got.ActualCost != sessCost+0.5 {
		t.Errorf("TotalTokens = %v, ActualCost = %v; want %d, %v", got.TotalTokens, got.ActualCost, sessTokens+300, sessCost+0.5)
	}
}

func TestWriteMergedMetrics_RecordedConversationWithoutTotals_LeavesTicketUnstamped(t *testing.T) {
	ticketPath, cwd, homes := stampTotalsFixture(t, "[sess-1]")
	fixture.WriteFakeTranscript(t, homes.Claude, cwd, "sess-1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), [3]any{"claude-sonnet-5", 1000, 0})
	recordConversation(t, ticketPath, nil)

	m, _, err := writeMergedMetrics(homes, agent.AgentClaude, cwd, "sess-1", ticketPath)
	if err != nil {
		t.Fatalf("writeMergedMetrics: %v", err)
	}

	got := fixture.MustParse(t, ticketPath)
	if got.TotalTokens != nil || got.ActualCost != 0 || m.costStamped {
		t.Errorf("TotalTokens = %v, ActualCost = %v, costStamped = %v; want none stamped", got.TotalTokens, got.ActualCost, m.costStamped)
	}
}

func TestWriteMergedMetrics_MixedCodexTicket_StampsClaudeCostAndMarker(t *testing.T) {
	ticketPath, cwd, homes := stampTotalsFixture(t, "[sess-claude, sess-codex]")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fixture.WriteFakeTranscript(t, homes.Claude, cwd, "sess-claude", start, [3]any{"claude-sonnet-5", 1000, 0})
	claudeTokens, claudeCost, err := SessionsTotals(homes, []SessionRef{{Agent: agent.AgentClaude, Cwd: cwd, SessionID: "sess-claude"}})
	if err != nil {
		t.Fatalf("SessionsTotals: %v", err)
	}
	rollout := filepath.Join(homes.Codex, "sessions", "rollout-sess-codex.jsonl")
	if err := fixture.WriteCodexSessionMeta(rollout, 0, "sess-codex", cwd); err != nil {
		t.Fatal(err)
	}
	if err := fixture.WriteCodexUsageLine(rollout, 1, 500, 7000); err != nil {
		t.Fatal(err)
	}

	if _, _, err := writeMergedMetrics(homes, agent.AgentCodex, cwd, "sess-codex", ticketPath); err != nil {
		t.Fatalf("writeMergedMetrics: %v", err)
	}

	got := fixture.MustParse(t, ticketPath)
	if got.TotalTokens == nil || *got.TotalTokens != claudeTokens+7000 {
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
	ticketPath, cwd, homes := stampTotalsFixture(t, "[]")
	p := iterationParams{WorktreeDir: "/fake/worktrees", FeatureBranch: "epic", Agent: agent.AgentClaude,
		Ticket: tickets.Ticket{Identifier: "01", Path: ticketPath}, ScratchDir: filepath.Dir(filepath.Dir(filepath.Dir(ticketPath)))}

	stampCommitlessMetrics(homes, p, cwd, "")

	got := fixture.MustParse(t, ticketPath)
	if got.TotalTokens == nil || *got.TotalTokens != 0 || got.ActualCost != 0 || got.HadCodex {
		t.Errorf("TotalTokens = %v, ActualCost = %v, HadCodex = %v; want a zero stamp", got.TotalTokens, got.ActualCost, got.HadCodex)
	}
}

func TestStampCommitlessMetrics_AllCodex_StampsTokensZeroCostAndMarker(t *testing.T) {
	ticketPath, cwd, homes := stampTotalsFixture(t, "[sess-codex]")
	rollout := filepath.Join(homes.Codex, "sessions", "rollout-sess-codex.jsonl")
	if err := fixture.WriteCodexSessionMeta(rollout, 0, "sess-codex", cwd); err != nil {
		t.Fatal(err)
	}
	if err := fixture.WriteCodexUsageLine(rollout, 1, 500, 7000); err != nil {
		t.Fatal(err)
	}
	p := iterationParams{WorktreeDir: "/fake/worktrees", Agent: agent.AgentCodex,
		Ticket: tickets.Ticket{Identifier: "01", Path: ticketPath}}

	stampCommitlessMetrics(homes, p, cwd, "sess-codex")

	got := fixture.MustParse(t, ticketPath)
	if got.TotalTokens == nil || *got.TotalTokens != 7000 || got.ActualCost != 0 || !got.HadCodex {
		t.Errorf("TotalTokens = %v, ActualCost = %v, HadCodex = %v; want 7000, 0, true", got.TotalTokens, got.ActualCost, got.HadCodex)
	}
}
