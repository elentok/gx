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
	_, live := liveAgent(d, workspaceID, epicName, agentKind, worktreeDir, t)
	return live
}

// liveAgent looks up t's iteration pane in workspaceID by its iteration
// label and reports the live herdr.Agent state found there, alongside
// whether a live, owned pane was actually found (see resumeReattachable's
// doc for what "live" and "owned" mean here — this is that same check,
// factored out so a caller that also needs the agent's current status, not
// just whether it's live, doesn't have to re-derive the lookup).
func liveAgent(d Deps, workspaceID, epicName string, agentKind AgentKind, worktreeDir string, t tickets.Ticket) (agent herdr.Agent, live bool) {
	label := iterLabel(epicName, t.Identifier)
	tabs, err := d.TabList(workspaceID)
	if err != nil {
		return herdr.Agent{}, false
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
		return herdr.Agent{}, false
	}
	agent, err = d.AgentGet(label)
	if err != nil || agent.PaneID == "" || agent.TabID != tab.TabID || agent.WorkspaceID != workspaceID {
		return herdr.Agent{}, false
	}
	if agentKind == AgentCodex {
		cwd := iterationWorktreePath(worktreeDir, epicName, t.Identifier)
		verified, verifyErr := d.VerifyCodexSession(cwd, agent.AgentSession)
		if verifyErr != nil || !verified {
			return herdr.Agent{}, false
		}
	}
	return agent, true
}
