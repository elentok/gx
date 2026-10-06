package ralphloop

import "github.com/elentok/gx/tickets"

// EpicQueue reads epic's queue without running it: every ticket with the
// scheduler's own verdict (TicketVerdict), whole-epic scope, a ticket already
// claimed reported as "claimed".
func EpicQueue(epic tickets.Epic) []ScanDecision {
	scope := RunScope{wholeEpic: true}
	frontier := scope.Frontier(epic)
	inFrontier := make(map[string]bool, len(frontier))
	for _, t := range frontier {
		inFrontier[t.Path] = true
	}
	out := make([]ScanDecision, 0, len(epic.Tickets))
	for _, t := range epic.Tickets {
		claimed := epic.RenderedStatus(t) == tickets.StatusClaimed
		out = append(out, TicketVerdict(epic, scope, t, claimed, inFrontier[t.Path]))
	}
	return out
}

// IterationIdentity is where a ticket's iteration lives: the herdr agent
// label, the git branch and the worktree path under worktreeDir.
func IterationIdentity(epicName, identifier, worktreeDir string) (label, branch, worktree string) {
	return iterLabel(epicName, identifier), iterBranch(epicName, identifier), iterationWorktreePath(worktreeDir, epicName, identifier)
}
