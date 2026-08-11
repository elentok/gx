package ralphloop

import (
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

// resumeReattachable reports whether t's iteration still has a live, owned
// herdr tab/agent in workspaceID — the same live-ownership test reconcile's
// startup reattach applies to a claimed/needs-repair ticket (see
// reconcile's reattach closure), reused here so a cleared ticket the
// scheduler is about to reclaim is judged by iteration ownership, not by its
// now-ambiguous "open" status: an "open" ticket the scheduler last saw
// launched looks identical whether its prior iteration is still running or
// long gone.
func resumeReattachable(d Deps, workspaceID, epicName string, agentKind AgentKind, worktreeDir string, t tickets.Ticket) bool {
	label := iterLabel(epicName, t.Identifier)
	tabs, err := d.TabList(workspaceID)
	if err != nil {
		return false
	}
	var tab herdr.Tab
	found := false
	for _, candidate := range tabs {
		if candidate.Label == label {
			tab = candidate
			found = true
			break
		}
	}
	if !found {
		return false
	}
	agent, err := d.AgentGet(label)
	if err != nil || agent.PaneID == "" || agent.TabID != tab.TabID || agent.WorkspaceID != workspaceID {
		return false
	}
	if agentKind == AgentCodex {
		cwd := iterationWorktreePath(worktreeDir, epicName, t.Identifier)
		verified, verifyErr := d.VerifyCodexSession(cwd, agent.AgentSession)
		if verifyErr != nil || !verified {
			return false
		}
	}
	return true
}
