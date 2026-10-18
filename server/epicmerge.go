package server

import (
	"cmp"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// epicMerge is the manual merge trigger for an epic address: it lands the
// epic's branch when it already fast-forwards, and forks a manual merge ticket
// otherwise. A manual ticket resets the merge-ticket cap and lands the epic
// whatever auto-ff-merge says.
func (s *Server) epicMerge(req QueueRequest) (QueueResult, error) {
	project, epic, ok := tickets.ParseEpicAddress(req.Address)
	if !ok {
		return refusal(ReasonInvalidAddress, req.Address+" is not an epic address (want project:epic)"), nil
	}
	a := tickets.Address{Project: project, Epic: epic}
	root := rootRef{Project: a.Project, Epic: a.Epic}
	unknown := refusal(ReasonUnknownTicket, "no epic "+root.String())
	dir, repo, err := s.projectOf(a.Project)
	if err != nil {
		return unknown, nil
	}
	epics, err := tickets.LoadResolved(a.Project, dir)
	if err != nil {
		return QueueResult{}, err
	}
	i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return filepath.Base(e.Path) == a.Epic })
	if i < 0 || len(epics[i].Tickets) == 0 {
		return unknown, nil
	}
	e := epics[i]
	if e.IsMap {
		return refusal(ReasonMapEpic, root.String()+" is a map epic; its tickets are resolved by hand"), nil
	}
	first := lowestTicket(e)
	one := ralphloop.OneIteration{Epic: a.Epic, Ticket: first}
	_, target, err := s.rootRefs(a.Project, epics, a.Epic, first, repo)
	if err != nil {
		return QueueResult{}, err
	}
	defer s.trunkLocks.lock(repo, target)()
	if r := epicMergeRefusal(root, e, repo); r != nil {
		return *r, nil
	}
	branch := "refs/heads/" + a.Epic
	if err := s.ensureFeatureWorktree(a.Project, a.Epic, repo); err != nil {
		return QueueResult{}, err
	}
	// Queued first, so a park of the land below or of the merge ticket's shows.
	if err := s.ensureRootQueued(root, first); err != nil {
		return QueueResult{}, err
	}
	if ff, err := git.IsAncestor(repo, target, branch); err != nil {
		return QueueResult{}, err
	} else if ff {
		return s.landEpic(root, one, dir, repo, epics, e, target, true), nil
	}
	id := s.forkMergeTicket(root, e, target, true)
	if id == "" {
		return QueueResult{}, errors.New("fork manual merge ticket for " + root.String())
	}
	return QueueResult{MergeTicket: tickets.Address{Project: a.Project, Epic: a.Epic, ID: id}.String()}, nil
}

// lowestTicket is the ticket an epic's root refs are worked out from.
func lowestTicket(e tickets.Epic) tickets.Ticket {
	return slices.MinFunc(e.Tickets, func(x, y tickets.Ticket) int {
		return cmp.Or(cmp.Compare(x.Number, y.Number), cmp.Compare(x.Identifier, y.Identifier))
	})
}

// fillEpicMergeTargets sets each ticket's EpicMergeTarget to the branch
// epicMerge would land its epic on. rootRefs runs git, so once per epic.
func (s *Server) fillEpicMergeTargets(list []TicketInfo) {
	targets := map[tickets.Address]string{}
	for i, info := range list {
		a, err := tickets.ParseAddress(info.Address, tickets.AddressContext{})
		if err != nil {
			continue
		}
		key := tickets.Address{Project: a.Project, Epic: a.Epic}
		target, ok := targets[key]
		if !ok {
			target = s.epicMergeTarget(key)
			targets[key] = target
		}
		list[i].EpicMergeTarget = target
	}
}

// epicMergeTarget is "" when the target can't be worked out: the menu then
// just says "Merge". It is also "" for an epic whose Merge is dimmed (tickets
// not done, already merged): every snapshot runs this per epic, and only a
// mergeable epic is worth its git calls.
func (s *Server) epicMergeTarget(a tickets.Address) string {
	dir, repo, err := s.projectOf(a.Project)
	if err != nil {
		return ""
	}
	epics, _ := s.idx.epicsOf(dir)
	i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return filepath.Base(e.Path) == a.Epic })
	if i < 0 || epics[i].IsMap || !epics[i].AllDone() || !epics[i].MergedAt.IsZero() {
		return ""
	}
	_, target, err := s.rootRefs(a.Project, epics, a.Epic, lowestTicket(epics[i]), repo)
	if err != nil {
		return ""
	}
	return target
}

// epicMergeRefusal is the refusal for a manual merge of e, nil when it may go.
// A pending merge ticket is checked before completeness: it is the one
// non-terminal ticket a person retrying the trigger is likely to meet.
func epicMergeRefusal(root rootRef, e tickets.Epic, repo string) *QueueResult {
	var r QueueResult
	switch {
	case !tickets.LoadEpicTicketMD(e.Path).MergedAt.IsZero():
		r = refusal(ReasonAlreadyMerged, root.String()+" is already merged")
	case slices.ContainsFunc(e.Tickets, func(t tickets.Ticket) bool { return pendingMerge(e, t) }):
		r = refusal(ReasonMergePending, root.String()+" already has a merge ticket pending")
	case !ralphloop.AllDone(e):
		r = refusal(ReasonNotComplete, root.String()+" has tickets that are not done")
	case !git.RefExists(repo, "refs/heads/"+root.Epic):
		r = refusal(ReasonBranchMissing, "no branch "+root.Epic+" in "+repo)
	default:
		return nil
	}
	return &r
}

func pendingMerge(e tickets.Epic, t tickets.Ticket) bool {
	if t.Type != string(schema.TypeMerge) {
		return false
	}
	switch schema.Status(t.Status) {
	case schema.StatusOpen, schema.StatusClaimed, schema.StatusNeedsAnswer, schema.StatusNeedsRepair:
		return true
	}
	return false
}

// ensureFeatureWorktree recreates the epic's feature worktree from its branch
// when a person removed it. The server never removes it again.
func (s *Server) ensureFeatureWorktree(project, epic, repo string) error {
	path := filepath.Join(s.worktreeDir(project), epic)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	info, err := git.IdentifyDir(repo)
	if err != nil {
		return err
	}
	return git.AddWorktreeOnBranch(info.Repo, epic, path)
}

// ensureRootQueued queues the root's first ticket unless the root is queued.
func (s *Server) ensureRootQueued(root rootRef, first tickets.Ticket) error {
	addr := tickets.Address{Project: root.Project, Epic: root.Epic, ID: first.Identifier}.String()
	_, err := s.writeQueue(addr, func(items []QueueItem) ([]QueueItem, *QueueResult) {
		for _, it := range items {
			if a, err := tickets.ParseAddress(it.Address, tickets.AddressContext{}); err == nil && rootOf(a) == root {
				r := QueueResult{}
				return nil, &r
			}
		}
		return append(items, QueueItem{Address: addr, Agent: string(ralphloop.AgentClaude)}), nil
	})
	return err
}
