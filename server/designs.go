package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/sessions"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// designBranch is the branch and worktree name of a map's Design.
func designBranch(epic string) string { return epic + "-design" }

// designClash is the refusal message when the project has an epic named like
// the Design's branch, or "".
func designClash(projectDir, project, epic string) string {
	if _, err := os.Stat(filepath.Join(projectDir, designBranch(epic))); err != nil {
		return ""
	}
	return fmt.Sprintf("%s has an epic %s, which would share the branch of the %s Design", project, designBranch(epic), epic)
}

// designCtx is what every design git helper needs about one map: its project
// dir, repo, and the Design's branch and worktree path.
type designCtx struct {
	projectDir string
	repo       *git.Repo
	epic       string
	branch     string
	path       string
}

func (s *Server) designCtxOf(project, epic string) (designCtx, error) {
	projectDir, repoDir, err := s.projects.Of(project)
	if err != nil {
		return designCtx{}, err
	}
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		return designCtx{}, err
	}
	branch := designBranch(epic)
	return designCtx{projectDir, repo, epic, branch, filepath.Join(repo.LinkedWorktreeDir(), branch)}, nil
}

// pastBranchStep reports whether design_step is branch or done.
func pastBranchStep(step tickets.DesignStep) bool {
	return step == tickets.StepBranch || step == tickets.StepDone
}

// afterBranch reports whether the map's Design is past the Branch step: its
// design_step is branch or done, or the <map>-impl branch exists. The impl epic
// alone does not count, as step 7 makes it before step 8 needs the worktree.
// A deleted impl branch with design_step branch still counts.
func (c designCtx) afterBranch() bool {
	step := tickets.DesignStep(tickets.LoadEpicTicketMD(filepath.Join(c.projectDir, c.epic)).DesignStep)
	return pastBranchStep(step) || tickets.BranchExists(c.repo.Root, c.epic+tickets.ImplEpicSuffix)
}

// designDir is where a map's Design session runs: the trunk checkout once past
// Branch (Past), else the design worktree with Note saying why its rebase was
// skipped. Clash is the refusal message when an epic shares the branch name.
type designDir struct {
	Path  string
	Note  string
	Past  bool
	Clash string
}

// resolveDesignDir checks the branch clash, then after Branch cleans up the
// design worktree and returns the trunk checkout; before it ensures the
// worktree and rebases it. excludeID is a session that does not count as
// another live one in the rebase.
func (s *Server) resolveDesignDir(project, epic, excludeID string) (designDir, error) {
	c, err := s.designCtxOf(project, epic)
	if err != nil {
		return designDir{}, err
	}
	if msg := designClash(c.projectDir, project, epic); msg != "" {
		return designDir{Clash: msg}, nil
	}
	if c.afterBranch() {
		s.CleanupDesign(project, epic)
		path := tickets.TrunkCheckout(s.cfg.TicketStore, project)
		if path == "" {
			return designDir{}, fmt.Errorf("no trunk checkout for project %s", project)
		}
		return designDir{Path: path, Past: true}, nil
	}
	path, err := s.ensureDesignWorktree(c)
	if err != nil {
		return designDir{}, err
	}
	return designDir{Path: path, Note: s.rebaseDesign(project, epic, path, excludeID)}, nil
}

// CleanupDesign removes the map's design worktree and branch once the Design is
// past Branch, no design session of the map is live, the worktree has no
// uncommitted edits and every design patch is in <map>-impl. Anything else
// keeps both and logs a warning; the next trigger checks again.
func (s *Server) CleanupDesign(project, epic string) {
	if err := s.tryCleanupDesign(project, epic); err != nil {
		s.log.Warn("design cleanup kept the worktree and branch", "map", project+":"+epic, "err", err)
		return
	}
	s.log.Info("design cleanup checked", "map", project+":"+epic)
}

func (s *Server) tryCleanupDesign(project, epic string) error {
	c, err := s.designCtxOf(project, epic)
	if err != nil || !c.afterBranch() {
		return err
	}
	live, err := s.liveDesignSession(project + ":" + epic)
	if err != nil || live != nil {
		return err
	}
	_, statErr := os.Stat(c.path)
	if statErr != nil && !tickets.BranchExists(c.repo.Root, c.branch) {
		return nil
	}
	if statErr == nil {
		changes, err := git.UncommittedChanges(c.path)
		if err != nil {
			return err
		}
		if len(changes) > 0 {
			return fmt.Errorf("the worktree has uncommitted edits")
		}
	}
	missing, err := git.UnappliedPatches(c.repo.Root, epic+tickets.ImplEpicSuffix, c.branch)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("%d design commit(s) are not in %s", len(missing), epic+tickets.ImplEpicSuffix)
	}
	return c.removeWorktreeAndBranch(false)
}

// removeWorktreeAndBranch drops the Design's worktree and branch, each only if
// it exists. force removes the worktree whatever it holds.
func (c designCtx) removeWorktreeAndBranch(force bool) error {
	if _, err := os.Stat(c.path); err == nil {
		if err := git.RemoveWorktree(*c.repo, c.path, force); err != nil {
			return err
		}
	}
	if !tickets.BranchExists(c.repo.Root, c.branch) {
		return nil
	}
	return git.DeleteLocalBranch(*c.repo, c.branch, true)
}

// scanDesignCleanups runs the design cleanup for every map whose design_step
// moved to Branch or Done since the last rescan. A plain Claude session at step
// 8 reports no exit, and `gx tickets set --design-step` writes the file without
// telling the server, so the rescan is what notices. Only the transition
// triggers the check, so an idle server does no git work for old maps; session
// exit and design open cover a map that keeps its worktree.
func (s *Server) scanDesignCleanups() {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return
	}
	for _, dir := range dirs {
		epics, _ := s.idx.EpicsOf(dir)
		for _, e := range epics {
			if !e.IsMap {
				continue
			}
			step := tickets.DesignStep(tickets.LoadEpicTicketMD(e.Path).DesignStep)
			project, epic := tickets.ProjectName(dir), filepath.Base(e.Path)
			prev, _ := s.designSteps.Swap(project+":"+epic, step)
			if prev == step || !pastBranchStep(step) {
				continue
			}
			s.log.Info("design step moved past branch", "map", project+":"+epic, "step", step)
			s.guards.GoSettle(func() {
				if step == tickets.StepDone {
					s.stopFinishedDesign(project, epic)
				}
				s.CleanupDesign(project, epic)
			})
		}
	}
}

// stopFinishedDesign ends the map's live design session once its design_step is
// done: the agent has nothing left to do and would otherwise stay in the list.
// Branch is not enough, as the agent still runs the last step then.
func (s *Server) stopFinishedDesign(project, epic string) {
	live, err := s.liveDesignSession(project + ":" + epic)
	if err != nil || live == nil {
		return
	}
	if res, err := s.sessions.StopSession(wire.SessionRequest{ID: live.ID}); err != nil || res.Refused {
		s.log.Warn("could not stop the finished design session", "map", project+":"+epic, "session", live.ID, "err", err, "refused", res.Message)
		return
	}
	s.log.Info("stopped the finished design session", "map", project+":"+epic, "session", live.ID)
}

// ensureDesignWorktree makes sure the map's design worktree exists and returns
// its path: on a new branch from trunk, or re-added on the existing branch.
func (s *Server) ensureDesignWorktree(c designCtx) (string, error) {
	if _, err := os.Stat(c.path); err == nil {
		return c.path, nil
	}
	if tickets.BranchExists(c.repo.Root, c.branch) {
		return c.path, git.AddWorktreeOnBranch(*c.repo, c.branch, c.path)
	}
	trunk, err := tickets.Trunk(c.projectDir, c.repo.Root)
	if err != nil {
		return "", err
	}
	return c.path, git.AddWorktree(*c.repo, c.branch, c.path, trunk)
}

// rebaseDesign rebases the design worktree's branch onto trunk, so the agent
// reads current code and docs. A problem never blocks the open: it skips the
// rebase, leaves the branch at its old tip and returns a note saying why.
// excludeID is a session that does not count as another live one.
func (s *Server) rebaseDesign(project, epic, worktree, excludeID string) string {
	if err := s.tryRebase(project, epic, worktree, excludeID); err != nil {
		return "the design branch was not rebased onto trunk: " + err.Error()
	}
	return ""
}

func (s *Server) tryRebase(project, epic, worktree, excludeID string) error {
	live, err := s.liveDesignSessionExcept(project+":"+epic, excludeID)
	if err != nil {
		return err
	}
	if live != nil {
		return errors.New("another design session of this map is live")
	}
	changes, err := git.UncommittedChanges(worktree)
	if err != nil {
		return err
	}
	if len(changes) > 0 {
		return errors.New("the worktree has uncommitted edits")
	}
	c, err := s.designCtxOf(project, epic)
	if err != nil {
		return err
	}
	trunk, err := tickets.Trunk(c.projectDir, c.repo.Root)
	if err != nil {
		return err
	}
	out, err := git.Rebase(worktree, trunk)
	if err == nil {
		return nil
	}
	if inProgress, _ := git.RebaseInProgress(worktree); inProgress {
		if abortErr := git.AbortRebase(worktree); abortErr != nil {
			return fmt.Errorf("the rebase conflicted and could not be aborted: %w", abortErr)
		}
		return errors.New("the rebase conflicted and was aborted")
	}
	return errors.New(strings.TrimSpace(out + " " + err.Error()))
}

var (
	designTicketRe = regexp.MustCompile(`^\d+[[:alpha:]]?\d*$`)
	designSlugRe   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

func (s *Server) designOpenWrite(w http.ResponseWriter, r *http.Request) {
	s.designOpenHandler(false)(w, r)
}

// designOpenWeb is designOpenWrite for the web mux: New needs the state token.
func (s *Server) designOpenWeb() http.HandlerFunc { return s.designOpenHandler(true) }

func (s *Server) designOpenHandler(web bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.DesignOpenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := s.openDesign(req, web)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, res)
	}
}

// openDesign returns the Design's newest live session, or creates one. Calls
// for one epic run one at a time, so a double click starts one session.
func (s *Server) openDesign(req wire.DesignOpenRequest, web bool) (wire.SessionResult, error) {
	if req.NewDesign {
		return s.openNewDesign(req)
	}
	epicAddr, ticket, _ := strings.Cut(req.Epic, "/")
	project, epic, ok := strings.Cut(epicAddr, ":")
	if !ok || project == "" || epic == "" || (strings.Contains(req.Epic, "/") && !designTicketRe.MatchString(ticket)) {
		return sessions.Refusal(wire.ReasonSessionBadRequest, fmt.Sprintf("%q is not an epic address (want project:epic or project:epic/NN)", req.Epic)), nil
	}
	dir, reason, msg := s.mapEpicDir(project, epic)
	if reason != "" {
		return sessions.Refusal(reason, msg), nil
	}

	mu, _ := s.designOpens.LoadOrStore(epicAddr, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	live, err := s.liveDesignSession(epicAddr)
	if err != nil {
		return wire.SessionResult{}, err
	}
	if live != nil && !req.New {
		return wire.SessionResult{Session: live}, nil
	}
	if live != nil {
		if msg := designClash(dir, project, epic); msg != "" {
			return sessions.Refusal(wire.ReasonDesignBranchClash, msg), nil
		}
		if web && req.State != sessions.SessionStateToken(*live) {
			return sessions.Refusal(wire.ReasonStale, "the session changed; review its current state"), nil
		}
		if res, err := s.sessions.StopSession(wire.SessionRequest{ID: live.ID}); err != nil || (res.Refused && res.Reason != wire.ReasonSessionNotLive) {
			return res, err
		}
	}

	d, err := s.resolveDesignDir(project, epic, "")
	if err != nil {
		return wire.SessionResult{}, err
	}
	if d.Clash != "" {
		return sessions.Refusal(wire.ReasonDesignBranchClash, d.Clash), nil
	}
	target := epicAddr
	if ticket != "" {
		target += "/" + ticket
	}
	prompt := harness.SkillCommand(s.projects.LaunchHarness(project), "gx-design", target)
	return s.sessions.CreateSession(wire.CreateSessionRequest{
		Project:      project,
		Dir:          d.Path,
		MadeWorktree: !d.Past,
		Name:         epic,
		Ticket:       epicAddr,
		Prompt:       prompt,
		RebaseNote:   d.Note,
	}, sessions.SessionCtx{LaunchDesign: true})
}

// mapEpicDir returns the project dir of a map epic, or the refusal reason and
// message when the project or epic is unknown or the epic is not a map.
func (s *Server) mapEpicDir(project, epic string) (dir, reason, msg string) {
	dir, err := s.projects.Dir(project)
	if err != nil {
		return "", wire.ReasonDesignUnknownEpic, err.Error()
	}
	epicDir := filepath.Join(dir, epic)
	if fi, err := os.Stat(epicDir); err != nil || !fi.IsDir() {
		return "", wire.ReasonDesignUnknownEpic, fmt.Sprintf("no epic %s:%s", project, epic)
	}
	if !tickets.IsMapEpic(epicDir) {
		return "", wire.ReasonDesignNotMapEpic, fmt.Sprintf("%s:%s is not a map epic", project, epic)
	}
	return dir, "", ""
}

func designWorktreeRefusal(reason, msg string) wire.DesignWorktreeResult {
	return wire.DesignWorktreeResult{Refused: true, Reason: reason, Message: msg}
}

func (s *Server) designWorktreeWrite(w http.ResponseWriter, r *http.Request) {
	var req wire.DesignWorktreeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.designWorktree(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

// designWorktree is designs open without the session: it ensures and rebases
// the map's design worktree under the same rules, or after Branch returns the
// trunk checkout and makes nothing.
func (s *Server) designWorktree(req wire.DesignWorktreeRequest) (wire.DesignWorktreeResult, error) {
	project, epic, ok := strings.Cut(req.Epic, ":")
	if !ok || project == "" || epic == "" || strings.Contains(epic, "/") {
		return designWorktreeRefusal(wire.ReasonSessionBadRequest, fmt.Sprintf("%q is not an epic address (want project:epic)", req.Epic)), nil
	}
	_, reason, msg := s.mapEpicDir(project, epic)
	if reason != "" {
		return designWorktreeRefusal(reason, msg), nil
	}

	mu, _ := s.designOpens.LoadOrStore(req.Epic, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	d, err := s.resolveDesignDir(project, epic, req.ExcludeSession)
	if err != nil {
		return wire.DesignWorktreeResult{}, err
	}
	if d.Clash != "" {
		return designWorktreeRefusal(wire.ReasonDesignBranchClash, d.Clash), nil
	}
	return wire.DesignWorktreeResult{Path: d.Path, Note: d.Note, AfterBranch: d.Past, Harness: s.projects.LaunchHarness(project)}, nil
}

// openNewDesign starts a design session for an idea with no map yet: Epic is
// the slug the map will take in Project. It creates no epic; Chart writes it.
func (s *Server) openNewDesign(req wire.DesignOpenRequest) (wire.SessionResult, error) {
	slug := req.Epic
	if !designSlugRe.MatchString(slug) {
		return sessions.Refusal(wire.ReasonSessionBadRequest, fmt.Sprintf("%q is not a slug (want lowercase words joined by -)", slug)), nil
	}
	idea := strings.TrimSpace(wire.NormalizeAnswer(req.Idea))
	switch {
	case idea == "":
		return sessions.Refusal(wire.ReasonEmptyPrompt, "an idea is required"), nil
	case len(idea) > wire.MaxSessionPromptBytes:
		return sessions.Refusal(wire.ReasonPromptTextTooLong, fmt.Sprintf("idea is over %d bytes", wire.MaxSessionPromptBytes)), nil
	}
	dir, err := s.projects.Dir(req.Project)
	if err != nil {
		return sessions.Refusal(wire.ReasonSessionBadRequest, err.Error()), nil
	}
	addr := req.Project + ":" + slug

	mu, _ := s.designOpens.LoadOrStore(addr, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	if _, err := os.Stat(filepath.Join(dir, slug)); err == nil {
		return sessions.Refusal(wire.ReasonDesignEpicExists, fmt.Sprintf("%s already has an epic %s", req.Project, slug)), nil
	}
	if msg := designClash(dir, req.Project, slug); msg != "" {
		return sessions.Refusal(wire.ReasonDesignBranchClash, msg), nil
	}

	if err := createMap(s.cfg.ConversationsDir, dir, slug, idea); err != nil {
		return wire.SessionResult{}, err
	}
	d, err := s.resolveDesignDir(req.Project, slug, "")
	if err != nil {
		return wire.SessionResult{EpicCreated: true, StartError: err.Error()}, nil
	}
	return s.startSession(req.Project, slug, d.Path, d.Note), nil
}

// createMap writes the uncharted map epic for a New Design, at the Chart step.
func createMap(conversationsDir, dir, slug, idea string) error {
	epicPath, err := tickets.CreateEpic(conversationsDir, dir, slug, tickets.EpicFields{Kind: tickets.KindMap, Status: string(schema.StatusOpen)}, "## Initial prompt\n\n"+idea+"\n")
	if err != nil {
		return err
	}
	step := string(tickets.StepChart)
	return tickets.UpdateEpicFields(filepath.Join(epicPath, "ticket.md"), tickets.EpicUpdate{DesignStep: &step})
}

// startSession starts the design session for a map that now exists. A failure
// is a result, not a refusal: the map is there, so the caller reports it as
// created with the reason in StartError.
func (s *Server) startSession(project, slug, worktree, rebaseNote string) wire.SessionResult {
	addr := project + ":" + slug
	res, err := s.sessions.CreateSession(wire.CreateSessionRequest{
		Project:      project,
		Dir:          worktree,
		MadeWorktree: true,
		Name:         slug,
		Ticket:       addr,
		Prompt:       harness.SkillCommand(s.projects.LaunchHarness(project), "gx-design", addr),
		RebaseNote:   rebaseNote,
	}, sessions.SessionCtx{LaunchDesign: true})
	switch {
	case err != nil:
		return wire.SessionResult{EpicCreated: true, StartError: err.Error()}
	case res.Refused:
		return wire.SessionResult{EpicCreated: true, StartError: res.Message}
	}
	return res
}

// designUncharted is the one rule behind EpicDetail.Deletable and the delete
// route: a map with no Destination, no tickets of any status and no live
// design session.
func (s *Server) designUncharted(epicAddr string, e tickets.Epic) (bool, error) {
	if !e.IsMap || schema.Section(e.Body, "Destination") != "" || len(e.Tickets) > 0 {
		return false, nil
	}
	live, err := s.liveDesignSession(epicAddr)
	return live == nil, err
}

func (s *Server) designDeleteWrite(w http.ResponseWriter, r *http.Request) {
	var req wire.DesignDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.deleteDesign(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

// deleteDesign removes an uncharted design's epic directory. It re-checks the
// rule from disk under the lock designs open takes, so a session or a chart
// that lands first wins. Exited session records stay.
func (s *Server) deleteDesign(req wire.DesignDeleteRequest) (wire.SessionResult, error) {
	project, epic, ok := strings.Cut(req.Epic, ":")
	if !ok || project == "" || epic == "" || strings.Contains(epic, "/") {
		return sessions.Refusal(wire.ReasonSessionBadRequest, fmt.Sprintf("%q is not an epic address (want project:epic)", req.Epic)), nil
	}
	dir, err := s.projects.Dir(project)
	if err != nil {
		return sessions.Refusal(wire.ReasonDesignUnknownEpic, err.Error()), nil
	}

	mu, _ := s.designOpens.LoadOrStore(req.Epic, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	epics, err := tickets.LoadResolved(project, dir)
	if err != nil {
		return wire.SessionResult{}, err
	}
	i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return e.Name == epic })
	if i < 0 {
		return sessions.Refusal(wire.ReasonDesignUnknownEpic, fmt.Sprintf("no epic %s", req.Epic)), nil
	}
	if !epics[i].IsMap {
		return sessions.Refusal(wire.ReasonDesignNotMapEpic, fmt.Sprintf("%s is not a map epic", req.Epic)), nil
	}
	uncharted, err := s.designUncharted(req.Epic, epics[i])
	if err != nil {
		return wire.SessionResult{}, err
	}
	if !uncharted {
		return sessions.Refusal(wire.ReasonDesignNotUncharted, fmt.Sprintf("%s is charted or has a live session", req.Epic)), nil
	}
	if err := s.forceRemoveDesign(project, epic); err != nil {
		return wire.SessionResult{}, err
	}
	if err := os.RemoveAll(epics[i].Path); err != nil {
		return wire.SessionResult{}, err
	}
	s.events.Publish(wire.EventTicketChanged, req.Epic)
	return wire.SessionResult{}, nil
}

// forceRemoveDesign drops the map's design worktree and branch whatever they
// hold: deleting the Design discards its edits and commits. A map that never
// had either is a no-op.
func (s *Server) forceRemoveDesign(project, epic string) error {
	c, err := s.designCtxOf(project, epic)
	if err != nil {
		return err
	}
	return c.removeWorktreeAndBranch(true)
}

// liveDesignSession is the session of the Design at epicAddr that has not
// exited and has the latest RunStartedAt, or nil. The Design page picks by the
// same key (webui/src/design/view.ts).
func (s *Server) liveDesignSession(epicAddr string) (*wire.SessionInfo, error) {
	return s.liveDesignSessionExcept(epicAddr, "")
}

// liveDesignSessionExcept is liveDesignSession leaving out the session
// excludeID, so a caller never counts itself. Empty excludes none.
func (s *Server) liveDesignSessionExcept(epicAddr, excludeID string) (*wire.SessionInfo, error) {
	list, err := s.sessions.ListSessions()
	if err != nil {
		return nil, err
	}
	var live *wire.SessionInfo
	for i := range list {
		in := &list[i]
		if in.ID != excludeID && in.Design && in.Status.State != wire.SessionExited && queue.EpicAddress(in.Ticket) == epicAddr &&
			(live == nil || !in.RunStartedAt.Before(live.RunStartedAt)) {
			live = in
		}
	}
	return live, nil
}
