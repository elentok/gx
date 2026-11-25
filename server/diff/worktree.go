package diff

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/rangediff"
	"github.com/elentok/gx/ralphloop/worktree"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketgit"
)

// SessionRef is what the Worktree route needs of a chat.
type SessionRef struct {
	Dir string
	// Name is the chat's name; may be empty.
	Name string
	// Stepped is true for a session that runs a Flow (a Design or Triage
	// session): its entity gets no Show diff.
	Stepped bool
	// RunTicket is the address of the ticket a Queue agent is running; empty
	// for every other chat.
	RunTicket string
}

// Lookups are what the root gives the Diff after the sessions and Designs
// exist. Either may be nil, which makes that target kind answer the empty state.
type Lookups struct {
	// Session resolves a chat id; ok is false for an unknown one.
	Session func(id string) (ref SessionRef, ok bool)
	// DesignWorktree is where a map's Design worktree lives, existing or not.
	DesignWorktree func(project, epic string) (string, error)
}

// GiveLookups sets the session and Design lookups.
func (d *Diff) GiveLookups(l Lookups) { d.lookups = l }

// Worktree serves GET /v1/diff/worktree?project=&target=<kind>:<id>.
// A target with nothing to show is a 200 with a Reason, never an error.
func (d *Diff) Worktree(w http.ResponseWriter, r *http.Request) {
	project, repo, ok := d.repoDir(w, r)
	if !ok {
		return
	}
	target := r.URL.Query().Get("target")
	kind, id, found := strings.Cut(target, ":")
	if !found || id == "" {
		http.Error(w, "target must be <kind>:<id>", http.StatusBadRequest)
		return
	}
	targetLabel := d.targetLabel(kind, id)
	tg, reason := d.resolveTarget(project, repo, kind, id)
	if reason == "" && !sameRepo(repo, tg.dir) {
		reason = wire.DiffNoWorktree
	}
	if reason != "" {
		wire.WriteJSON(w, wire.DiffWorktree{Reason: reason, TargetLabel: targetLabel})
		return
	}
	out, err := d.snapshot(project, repo, tg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out.TargetLabel = targetLabel
	wire.WriteJSON(w, out)
}

// targetLabel is the name the page shows for a target: a chat's name, else the id.
// A Design's id is its map name and a ticket's id is its address.
func (d *Diff) targetLabel(kind, id string) string {
	if kind == "chat" && d.lookups.Session != nil {
		if ref, ok := d.lookups.Session(id); ok {
			// A Queue agent is named by the ticket it works on.
			if ref.RunTicket != "" {
				return ref.RunTicket
			}
			if ref.Name != "" {
				return ref.Name
			}
		}
	}
	return id
}

// target is a resolved worktree. An empty base means the trunk merge-base.
type target struct {
	dir, base, label string
}

// resolveTarget is the worktree of a target, or the reason it has none.
func (d *Diff) resolveTarget(project, repo, kind, id string) (target, string) {
	switch kind {
	case "chat":
		if d.lookups.Session == nil {
			return target{}, wire.DiffNoWorktree
		}
		ref, ok := d.lookups.Session(id)
		switch {
		case !ok:
			return target{}, wire.DiffNoWorktree
		case ref.RunTicket != "":
			return d.ticketTarget(project, repo, ref.RunTicket, true)
		case ref.Dir == "":
			return target{}, wire.DiffNoWorktree
		case ref.Stepped:
			return target{}, wire.DiffNoDiff
		}
		return target{dir: ref.Dir}, ""
	case "design":
		if d.lookups.DesignWorktree == nil {
			return target{}, wire.DiffNoWorktree
		}
		p, err := d.lookups.DesignWorktree(project, id)
		if err != nil || p == "" {
			return target{}, wire.DiffNoWorktree
		}
		return target{dir: p}, ""
	case "ticket":
		return d.ticketTarget(project, repo, id, false)
	}
	return target{}, wire.DiffNoDiff
}

// ticketTarget is the iteration worktree of a claimed or paused ticket. A
// tracked run supplies the base it branched from. Without one, a paused ticket
// derives it (a Queue agent, which needs a run, does not).
func (d *Diff) ticketTarget(project, repo, address string, needRun bool) (target, string) {
	none := target{}
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{Project: project})
	if err != nil || addr.Project != project || d.Epics == nil || d.Run == nil || d.WorktreeDir == nil {
		return none, wire.DiffNoWorktree
	}
	epics, err := d.Epics(project)
	if err != nil {
		return none, wire.DiffNoWorktree
	}
	var epic tickets.Epic
	var tk tickets.Ticket
	for _, e := range epics {
		if e.Name != addr.Epic {
			continue
		}
		epic = e
		for _, c := range e.Tickets {
			if c.Identifier == addr.ID {
				tk = c
			}
		}
	}
	if tk.Path == "" || (tk.Status != "claimed" && tk.Status != "paused") {
		return none, wire.DiffNoWorktree
	}
	wtDir := d.WorktreeDir(project)
	if wtDir == "" {
		return none, wire.DiffNoWorktree
	}
	dir := worktree.IterationWorktreePath(wtDir, addr.Epic, tk.Identifier)
	// A stacked ticket is labelled by its blocker, an unstacked one by the feature branch.
	name, blockerBranch := addr.Epic, ""
	if leaf, _ := tickets.DeriveLeafBase(project, epics, addr.Epic, tk); leaf != "" {
		for _, c := range epic.Tickets {
			if c.Identifier == leaf {
				name = tickets.Address{Project: project, Epic: addr.Epic, ID: c.Identifier}.String()
				blockerBranch = worktree.IterationBranch(addr.Epic, leaf)
			}
		}
	}
	if base, ok := d.Run(addr.String()); ok && base != "" {
		return target{dir: dir, base: base, label: name + "@" + short(base)}, ""
	}
	if needRun || !sameRepo(repo, dir) {
		return none, wire.DiffNoWorktree
	}
	if blockerBranch != "" {
		if base, err := git.MergeBase(dir, "HEAD", blockerBranch); err == nil {
			return target{dir: dir, base: base, label: name + "@" + short(base)}, ""
		}
	}
	base, err := git.MergeBase(dir, "HEAD", addr.Epic)
	if err != nil {
		return none, wire.DiffNoWorktree
	}
	return target{dir: dir, base: base, label: addr.Epic + "@" + short(base)}, ""
}

func short(sha string) string { return sha[:min(7, len(sha))] }

// sameRepo reports whether dir is a live checkout sharing repo's object store,
// where the snapshot objects would be found.
func sameRepo(repo, dir string) bool {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return false
	}
	_, want, err := git.GitDirs(repo)
	if err != nil {
		return false
	}
	_, got, err := git.GitDirs(dir)
	if err != nil {
		return false
	}
	return realPath(want) == realPath(got)
}

func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

func (d *Diff) snapshot(project, repo string, tg target) (wire.DiffWorktree, error) {
	dir, base, label := tg.dir, tg.base, tg.label
	if base == "" {
		projectDir, err := tickets.ProjectDirByName(d.TicketStore, project)
		if err != nil {
			return wire.DiffWorktree{}, err
		}
		trunk, err := ticketgit.Trunk(projectDir, repo)
		if err != nil {
			return wire.DiffWorktree{}, err
		}
		if base, err = git.MergeBase(dir, "HEAD", trunk); err != nil {
			return wire.DiffWorktree{}, err
		}
		label = trunk + "@" + short(base)
	}
	head, err := git.RevParse(dir, "HEAD")
	if err != nil {
		return wire.DiffWorktree{}, err
	}
	snap, err := rangediff.SnapshotWorktree(dir)
	if err != nil {
		return wire.DiffWorktree{}, err
	}
	return wire.DiffWorktree{
		BaseSHA: base, BaseLabel: label, HeadSHA: head,
		SnapshotSHA: snap.SHA, Uncommitted: snap.Dirty, LeftOut: snap.LeftOut,
	}, nil
}
