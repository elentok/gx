package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server/files"
	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketgit"
)

// hostedFiles serves the /v1/hosted-files/ routes. It is its own type so the
// handlers stay off Server.
type hostedFiles struct{ s *Server }

func (h hostedFiles) routes() []wire.Route {
	return []wire.Route{
		{Pattern: "POST /v1/hosted-files/resolve", Handler: h.resolve, Web: wire.WebWrite},
		{Pattern: "GET /v1/hosted-files/roots", Handler: h.listRoots, Web: wire.WebRead},
		{Pattern: "POST /v1/hosted-files/roots/add", Handler: h.addRoot, Web: wire.WebWrite},
		{Pattern: "POST /v1/hosted-files/roots/remove", Handler: h.removeRoot, Web: wire.WebWrite},
		{Pattern: "GET " + webauth.FileTokenPrefix, Web: wire.WebReadPrefix, WebHandler: h.token},
		{Pattern: "GET /v1/files/", Web: wire.WebReadPrefix, WebHandler: h.raw},
	}
}

// checker snapshots the roots once, so a request with 200 candidates lists the
// worktrees once instead of per candidate.
func (h hostedFiles) checker() *files.Checker {
	cfg := h.s.cfg
	builtin := append(h.worktreePaths(), cfg.TicketStore)
	denied := []string{cfg.StateDir, cfg.SessionsDir, cfg.ConversationsDir}
	if cfg.ConfigPath != "" {
		denied = append(denied, filepath.Dir(cfg.ConfigPath))
	}
	uid := uint32(os.Getuid())
	if cfg.HostedFilesUID != nil {
		uid = *cfg.HostedFilesUID
	}
	c := &files.Checker{
		BuiltinRoots: func() []string { return builtin },
		ExtraRoots:   h.extraRoots,
		TempRoots:    files.DefaultTempRoots,
		DeniedDirs:   func() []string { return denied },
		TicketsStore: func() string { return cfg.TicketStore },
		UID:          uid,
	}
	if data, err := config.DirsFrom(cfg.Env).DataDir(); err == nil {
		c.DataDir = func() string { return data }
	}
	return c
}

// worktreePaths lists every checked-out worktree of every project repo. A bare
// repo's own folder is left out: only its worktrees are hostable.
func (h hostedFiles) worktreePaths() []string {
	store := h.s.cfg.TicketStore
	dirs, err := tickets.ProjectDirs(store)
	if err != nil {
		return nil
	}
	var out []string
	for _, dir := range dirs {
		pf, err := config.ReadProjectFile(dir)
		if err != nil || pf.Repo == nil {
			continue
		}
		repo, err := git.FindRepo(*pf.Repo)
		if err != nil {
			continue
		}
		wts, _ := git.ListWorktrees(*repo)
		for _, wt := range wts {
			out = append(out, wt.Path)
		}
	}
	return out
}

func (h hostedFiles) resolve(w http.ResponseWriter, r *http.Request) {
	var req ops.HostedResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	var res ops.HostedResolveResult
	if len(req.Candidates) > ops.MaxHostedCandidates {
		res = res.Refuse(ops.ReasonTooManyCandidates, "too many paths: at most 200 per call")
		wire.WriteJSON(w, res)
		return
	}
	n := 0
	for _, b := range []string{req.Session, req.Project, req.Dir} {
		if b != "" {
			n++
		}
	}
	if n > 1 {
		res = res.Refuse(ops.ReasonBadBase, "give at most one of a session, a project and a folder as the base")
		wire.WriteJSON(w, res)
		return
	}
	check := h.checker()
	base := h.base(check, req)
	home, _ := config.DirsFrom(h.s.cfg.Env).Home()
	res.Files = make([]ops.HostedResolved, 0, len(req.Candidates))
	for _, cand := range req.Candidates {
		out := ops.HostedResolved{Candidate: cand, Verdict: ops.HostedMissing}
		if abs, ok := joinCandidate(cand, base, home); ok {
			if hosted, ok := check.Check(abs); ok {
				out.Verdict = ops.HostedFound
				out.Path = hosted.Path
				out.Size = hosted.Size
				out.Kind = files.KindOf(hosted.Path)
				out.Modified = hosted.ModTime
			}
		}
		res.Files = append(res.Files, out)
	}
	wire.WriteJSON(w, res)
}

// base is the folder relative candidates join onto, or "" when the request has
// none or its base cannot be resolved. An unresolvable base is not a refusal, so
// the verb cannot be used to probe which ids, projects and folders exist.
func (h hostedFiles) base(check *files.Checker, req ops.HostedResolveRequest) string {
	switch {
	case req.Session != "":
		_, meta, res, err := h.s.sessions.SessionDir(req.Session)
		if err != nil || res.Refused {
			return ""
		}
		return meta.Dir
	case req.Project != "":
		return ticketgit.TrunkCheckout(h.s.cfg.TicketStore, req.Project)
	case req.Dir != "":
		if !filepath.IsAbs(req.Dir) || !check.InRoots(req.Dir) {
			return ""
		}
		return req.Dir
	}
	return ""
}

// joinCandidate turns one candidate into an absolute, cleaned path. A relative
// candidate with no base, and an empty one, cannot be joined.
func joinCandidate(cand, base, home string) (string, bool) {
	switch {
	case cand == "":
		return "", false
	case filepath.IsAbs(cand):
		return filepath.Clean(cand), true
	case cand == "~" || strings.HasPrefix(cand, "~/"):
		if home == "" {
			return "", false
		}
		return filepath.Join(home, cand[1:]), true
	case base == "":
		return "", false
	}
	return filepath.Join(base, cand), true
}
