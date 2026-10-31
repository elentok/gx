package server

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// epicMerge is the manual merge trigger for an epic address: it merges the
// epic's branch when it already fast-forwards, and forks a manual merge ticket
// otherwise. A manual ticket resets the merge-ticket cap and merges the epic
// whatever auto-ff-merge says.
func (s *Server) epicMerge(req QueueRequest) (QueueResult, error) {
	m, err := s.loadMergeEpic(req)
	if err != nil {
		return mergeVerbError(err)
	}
	first, _, target, err := s.epicRefs(m.root.Project, m.epics, m.root.Epic, m.repo)
	if err != nil {
		return QueueResult{}, err
	}
	one := ralphloop.OneIteration{Epic: m.root.Epic, Ticket: first}
	unlock := s.trunkLocks.lock(m.repo, target)
	d, err := s.epicMergeLocked(m.root, one, m.repo, m.epics, m.epic, target)
	unlock()
	if err != nil {
		return QueueResult{}, err
	}
	return s.actOnMerge(m.root, one, m.dir, m.epic, d), nil
}

// mergeEpicRef is the epic a manual merge verb acts on, loaded and checked.
type mergeEpicRef struct {
	root      rootRef
	dir, repo string
	epics     []tickets.Epic
	epic      tickets.Epic
}

// mergeRefused is the error loadMergeEpic returns when the verb must stop with
// a refusal rather than fail.
type mergeRefused struct{ res QueueResult }

func (e mergeRefused) Error() string { return "merge refused: " + e.res.Reason }

func refuseMerge(reason, msg string) error {
	return mergeRefused{res: refusal(reason, msg)}
}

// mergeVerbError turns loadMergeEpic's error into the verb's return: a refusal
// is a result, anything else an error.
func mergeVerbError(err error) (QueueResult, error) {
	var refused mergeRefused
	if errors.As(err, &refused) {
		return refused.res, nil
	}
	return QueueResult{}, err
}

// shownEpicState is the state a merge request's token names, in the Epic
// page's vocabulary. An Epics card shows the list state, which calls a
// merge-blocked epic "merge-blocked" where the Epic page says paused.
func shownEpicState(shown EpicState, e tickets.Epic) EpicState {
	if shown == EpicState(EpicListMergeBlocked) && epicMergeBlocked(e) {
		return EpicStatePaused
	}
	return shown
}

// loadMergeEpic resolves req's epic address for a manual merge verb. A
// mergeRefused error means the verb stops there; req's state token, when it has
// one, must match the epic's current state.
func (s *Server) loadMergeEpic(req QueueRequest) (mergeEpicRef, error) {
	project, epic, ok := tickets.ParseEpicAddress(req.Address)
	if !ok {
		return mergeEpicRef{}, refuseMerge(ReasonInvalidAddress, req.Address+" is not an epic address (want project:epic)")
	}
	root := rootRef{Project: project, Epic: epic}
	dir, repo, err := s.projectOf(project)
	if err != nil {
		return mergeEpicRef{}, refuseMerge(ReasonUnknownTicket, "no epic "+root.String())
	}
	epics, err := tickets.LoadResolved(project, dir)
	if err != nil {
		return mergeEpicRef{}, err
	}
	i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return filepath.Base(e.Path) == epic })
	if i < 0 || len(epics[i].Tickets) == 0 {
		return mergeEpicRef{}, refuseMerge(ReasonUnknownTicket, "no epic "+root.String())
	}
	e := epics[i]
	if e.IsMap {
		return mergeEpicRef{}, refuseMerge(ReasonMapEpic, root.String()+" is a map epic; its tickets are resolved by hand")
	}
	if !isMergeableEpic(root) {
		return mergeEpicRef{}, refuseMerge(ReasonOneOffsEpic, root.String()+" is the one-offs inbox; it never merges")
	}
	// The web page sends the state it shows; the socket route sends none.
	if shown := EpicState(req.State); shown != "" {
		now := epicStateFor(root, e, s.registry.startedAtOf)
		if shown = shownEpicState(shown, e); shown != now {
			return mergeEpicRef{}, refuseMerge(ReasonStale, fmt.Sprintf("%s is %s now, the page showed %s", root, now, shown))
		}
	}
	return mergeEpicRef{root: root, dir: dir, repo: repo, epics: epics, epic: e}, nil
}

// epicMergeLocked is epicMerge's work under the trunk lock; the caller acts on
// the decision after unlock.
func (s *Server) epicMergeLocked(root rootRef, one ralphloop.OneIteration, repo string, epics []tickets.Epic, e tickets.Epic, target string) (mergeDecision, error) {
	if r := epicMergeRefusal(root, e, repo); r != nil {
		return mergeDecision{result: *r}, nil
	}
	// No branch (a commitless epic): nothing to merge, so mergeEpic only stamps it.
	if !git.RefExists(repo, "refs/heads/"+root.Epic) {
		return s.mergeEpic(root, one, repo, epics, e, target, true), nil
	}
	if err := s.ensureFeatureWorktree(root.Project, root.Epic, repo); err != nil {
		return mergeDecision{}, err
	}
	// Queued first, so a pause of the merge below or of the merge ticket's shows.
	if err := s.ensureRootQueued(root, one.Ticket); err != nil {
		return mergeDecision{}, err
	}
	if ff, err := git.IsAncestor(repo, target, "refs/heads/"+root.Epic); err != nil {
		return mergeDecision{}, err
	} else if ff {
		return s.mergeEpic(root, one, repo, epics, e, target, true), nil
	}
	id := s.forkMergeTicket(root, e, target, true)
	if id == "" {
		return mergeDecision{}, errors.New("fork manual merge ticket for " + root.String())
	}
	// mergeEpic clears the block for the merge paths; a fork that failed keeps it.
	s.clearMergeBlock(root, e.Path)
	return mergeDecision{result: QueueResult{MergeTicket: tickets.Address{Project: root.Project, Epic: root.Epic, ID: id}.String()}, outcome: outcomeMergeTicket}, nil
}

// lowestTicket is the epic's Root ticket: the lowest-numbered one, whatever
// its status, cancelled included.
func lowestTicket(e tickets.Epic) tickets.Ticket {
	return slices.MinFunc(e.Tickets, func(x, y tickets.Ticket) int {
		return cmp.Or(cmp.Compare(x.Number, y.Number), cmp.Compare(x.Identifier, y.Identifier))
	})
}

// fillEpicMergeTargets sets each ticket's EpicMergeTarget to the branch
// epicMerge would merge its epic on. rootRefs runs git, so once per epic.
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
	// The cheap epic checks come from the index and run before projectOf, which
	// reads project.json: most epics are not mergeable.
	dir, epics, ok := s.idx.projectEpics(a.Project)
	if !ok {
		return ""
	}
	i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return filepath.Base(e.Path) == a.Epic })
	if i < 0 || epics[i].IsMap || !epics[i].AllDone() || !epics[i].MergedAt.IsZero() {
		return ""
	}
	repoPtr := s.projectRepos.repoOf(dir)
	if repoPtr == nil {
		return ""
	}
	repo := *repoPtr
	// The target is a function of the loaded epics (a new scan is a new slice),
	// project.json and the repo's refs, so those make the cache stamp.
	stamp, cacheable := tickets.RefsStamp(repo, filepath.Join(dir, config.ProjectFileName))
	stamp += fmt.Sprintf("%p", &epics[0])
	if cacheable {
		if target, hit := s.mergeTargets.get(a, stamp); hit {
			return target
		}
	}
	_, _, target, err := s.epicRefs(a.Project, epics, a.Epic, repo)
	if err != nil {
		return ""
	}
	if cacheable {
		s.mergeTargets.put(a, stamp, target)
	}
	return target
}

// gitIsAncestor is swapped in tests to count git calls.
var gitIsAncestor = git.IsAncestor

// mergeTargetCache holds each epic's last merge target. Errors aren't stored,
// so a failing resolve retries on the next snapshot.
type mergeTargetCache struct {
	mu sync.Mutex
	m  map[tickets.Address]mergeTargetEntry
}

type mergeTargetEntry struct{ stamp, target string }

// projectRepoCache holds each project's parsed repo.
type projectRepoCache struct {
	mu    sync.Mutex
	repos map[string]projectRepoEntry
}

// projectRepoEntry is a project's parsed repo, valid while project.json keeps
// its size and mtime. repo is nil when the file is unreadable or sets none.
type projectRepoEntry struct {
	size, mtime int64
	repo        *string
}

// readProjectFile is swapped in tests to count reads.
var readProjectFile = config.ReadProjectFile

// repoOf is the repo project.json in dir names, nil when there is none. It
// reads the file only when its size or mtime moved, since every snapshot asks.
// Errors aren't stored, so a failing read retries on the next snapshot.
func (c *projectRepoCache) repoOf(dir string) *string {
	fi, err := os.Stat(filepath.Join(dir, config.ProjectFileName))
	if err != nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.repos[dir]; ok && e.size == fi.Size() && e.mtime == fi.ModTime().UnixNano() {
		return e.repo
	}
	pf, err := readProjectFile(dir)
	if err != nil {
		return nil
	}
	if c.repos == nil {
		c.repos = map[string]projectRepoEntry{}
	}
	c.repos[dir] = projectRepoEntry{size: fi.Size(), mtime: fi.ModTime().UnixNano(), repo: pf.Repo}
	return pf.Repo
}

func (c *mergeTargetCache) get(a tickets.Address, stamp string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[a]
	return e.target, ok && e.stamp == stamp
}

func (c *mergeTargetCache) put(a tickets.Address, stamp, target string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[tickets.Address]mergeTargetEntry{}
	}
	c.m[a] = mergeTargetEntry{stamp: stamp, target: target}
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
	// A commitless epic never makes a branch: mergeEpic stamps it merged.
	case !allCommitless(e) && !git.RefExists(repo, "refs/heads/"+root.Epic):
		r = refusal(ReasonBranchMissing, "no branch "+root.Epic+" in "+repo)
	default:
		return nil
	}
	return &r
}

func allCommitless(e tickets.Epic) bool {
	return slices.IndexFunc(e.Tickets, func(t tickets.Ticket) bool { return !t.Commitless }) < 0
}

func pendingMerge(e tickets.Epic, t tickets.Ticket) bool {
	if t.Type != string(schema.TypeMerge) {
		return false
	}
	switch schema.Status(t.Status) {
	case schema.StatusOpen, schema.StatusClaimed, schema.StatusPaused, schema.StatusLegacyAnswer, schema.StatusLegacyRepair:
		return true
	}
	return false
}

// ensureFeatureWorktree recreates the epic's feature worktree from its branch
// when a person removed it. The post-merge removal (removeMergedWorktree) is the
// one place the server removes it again.
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
		return append(items, newQueueItem(addr, "")), nil
	})
	return err
}
