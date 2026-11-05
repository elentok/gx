package ralphloop

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/ralphloop/metrics"
	"github.com/elentok/gx/tickets"
)

// stampCommitlessFixture writes a done ticket with the given session_ids and
// returns its path, the worktree cwd the sessions run in and temp homes.
func stampCommitlessFixture(t *testing.T, sessionIDs string) (ticketPath, cwd string, homes metrics.SessionHomes) {
	t.Helper()
	homes = metrics.SessionHomes{Claude: t.TempDir(), Codex: filepath.Join(t.TempDir(), "codex")}
	fixture.SetHomeEnv(t, t.TempDir())
	scratchDir := fixture.WriteEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: done\ntype: implement\nsession_ids: " + sessionIDs + "\n---\n# A\n",
	})
	return filepath.Join(scratchDir, "epic", "issues", "01-a.md"), iterationWorktreePath("/fake/worktrees", "epic", "01"), homes
}

func TestStampCommitlessMetrics_NoSession_StampsZero(t *testing.T) {
	ticketPath, cwd, homes := stampCommitlessFixture(t, "[]")
	p := iterationParams{WorktreeDir: "/fake/worktrees", FeatureBranch: "epic", Agent: agent.AgentClaude,
		Ticket: tickets.Ticket{Identifier: "01", Path: ticketPath}, ScratchDir: filepath.Dir(filepath.Dir(filepath.Dir(ticketPath)))}

	stampCommitlessMetrics(homes, p, cwd, "")

	got := fixture.MustParse(t, ticketPath)
	if got.TotalTokens == nil || *got.TotalTokens != 0 || got.ActualCost != 0 || got.HadCodex {
		t.Errorf("TotalTokens = %v, ActualCost = %v, HadCodex = %v; want a zero stamp", got.TotalTokens, got.ActualCost, got.HadCodex)
	}
}

func TestStampCommitlessMetrics_AllCodex_StampsTokensZeroCostAndMarker(t *testing.T) {
	ticketPath, cwd, homes := stampCommitlessFixture(t, "[sess-codex]")
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
