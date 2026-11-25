package diff

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/rangediff"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketgit"
)

// SessionRef is what the Worktree route needs of a chat.
type SessionRef struct {
	Dir string
	// Stepped is true for a session that runs a Flow (a Design or Triage
	// session): its entity gets no Show diff.
	Stepped bool
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
	dir, reason := d.targetDir(project, kind, id)
	if reason == "" && !sameRepo(repo, dir) {
		reason = wire.DiffNoWorktree
	}
	if reason != "" {
		wire.WriteJSON(w, wire.DiffWorktree{Reason: reason})
		return
	}
	out, err := d.snapshot(project, repo, dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, out)
}

// targetDir is the worktree dir of a target, or the reason it has none.
func (d *Diff) targetDir(project, kind, id string) (dir, reason string) {
	switch kind {
	case "chat":
		if d.lookups.Session == nil {
			return "", wire.DiffNoWorktree
		}
		ref, ok := d.lookups.Session(id)
		switch {
		case !ok || ref.Dir == "":
			return "", wire.DiffNoWorktree
		case ref.Stepped:
			return "", wire.DiffNoDiff
		}
		return ref.Dir, ""
	case "design":
		if d.lookups.DesignWorktree == nil {
			return "", wire.DiffNoWorktree
		}
		p, err := d.lookups.DesignWorktree(project, id)
		if err != nil || p == "" {
			return "", wire.DiffNoWorktree
		}
		return p, ""
	}
	// ticket targets, and any other kind, are not resolved yet.
	return "", wire.DiffNoDiff
}

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

func (d *Diff) snapshot(project, repo, dir string) (wire.DiffWorktree, error) {
	projectDir, err := tickets.ProjectDirByName(d.TicketStore, project)
	if err != nil {
		return wire.DiffWorktree{}, err
	}
	trunk, err := ticketgit.Trunk(projectDir, repo)
	if err != nil {
		return wire.DiffWorktree{}, err
	}
	head, err := git.RevParse(dir, "HEAD")
	if err != nil {
		return wire.DiffWorktree{}, err
	}
	base, err := git.MergeBase(dir, "HEAD", trunk)
	if err != nil {
		return wire.DiffWorktree{}, err
	}
	snap, err := rangediff.SnapshotWorktree(dir)
	if err != nil {
		return wire.DiffWorktree{}, err
	}
	return wire.DiffWorktree{
		BaseSHA: base, BaseLabel: trunk + "@" + base[:min(7, len(base))], HeadSHA: head,
		SnapshotSHA: snap.SHA, Uncommitted: snap.Dirty, LeftOut: snap.LeftOut,
	}, nil
}
