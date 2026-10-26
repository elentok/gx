package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// Refusal reasons of the designs open route, beside the session ones.
const (
	ReasonDesignNotMapEpic  = "not-map-epic"
	ReasonDesignUnknownEpic = "unknown-epic"
	ReasonDesignEpicExists  = "epic-exists"
	// ReasonDesignBranchClash: an epic named <map>-design would share the
	// Design's branch name.
	ReasonDesignBranchClash = "design-branch-clash"
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

// afterBranch reports whether the map's Design is past the Branch step: its
// design_step is branch or done, or the <map>-impl branch exists. The impl epic
// alone does not count, as step 7 makes it before step 8 needs the worktree.
// A deleted impl branch with design_step branch still counts.
func (s *Server) afterBranch(project, epic string) (bool, error) {
	projectDir, repoDir, err := s.projectOf(project)
	if err != nil {
		return false, err
	}
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		return false, err
	}
	step := tickets.DesignStep(tickets.LoadEpicTicketMD(filepath.Join(projectDir, epic)).DesignStep)
	if step == tickets.StepBranch || step == tickets.StepDone {
		return true, nil
	}
	return tickets.BranchExists(repo.Root, epic+tickets.ImplEpicSuffix), nil
}

// cleanupDesign removes the map's design worktree and branch once the Design is
// past Branch, no design session of the map is live, the worktree has no
// uncommitted edits and every design patch is in <map>-impl. Anything else
// keeps both and logs a warning; the next trigger checks again.
func (s *Server) cleanupDesign(project, epic string) {
	if err := s.tryCleanupDesign(project, epic); err != nil {
		s.log.Warn("design cleanup kept the worktree and branch", "map", project+":"+epic, "err", err)
	}
}

func (s *Server) tryCleanupDesign(project, epic string) error {
	past, err := s.afterBranch(project, epic)
	if err != nil || !past {
		return err
	}
	live, err := s.liveDesignSession(project + ":" + epic)
	if err != nil || live != nil {
		return err
	}
	_, repoDir, err := s.projectOf(project)
	if err != nil {
		return err
	}
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		return err
	}
	branch := designBranch(epic)
	path := filepath.Join(repo.LinkedWorktreeDir(), branch)
	_, statErr := os.Stat(path)
	hasWorktree := statErr == nil
	if !hasWorktree && !tickets.BranchExists(repo.Root, branch) {
		return nil
	}
	if hasWorktree {
		changes, err := git.UncommittedChanges(path)
		if err != nil {
			return err
		}
		if len(changes) > 0 {
			return fmt.Errorf("the worktree has uncommitted edits")
		}
	}
	missing, err := git.UnappliedPatches(repo.Root, epic+tickets.ImplEpicSuffix, branch)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("%d design commit(s) are not in %s", len(missing), epic+tickets.ImplEpicSuffix)
	}
	if hasWorktree {
		if err := git.RemoveWorktree(*repo, path, false); err != nil {
			return err
		}
	}
	return git.DeleteLocalBranch(*repo, branch, true)
}

// scanDesignCleanups runs the design cleanup for every map whose design_step
// is past Branch. A plain Claude session at step 8 reports no exit, and `gx
// tickets set --design-step` writes the file without telling the server, so
// the rescan is what notices. A map that keeps its worktree is checked again
// on every rescan, so a warning is logged only when its reason changes.
func (s *Server) scanDesignCleanups() {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return
	}
	for _, dir := range dirs {
		epics, _ := s.idx.epicsOf(dir)
		for _, e := range epics {
			if !e.IsMap {
				continue
			}
			step := tickets.DesignStep(tickets.LoadEpicTicketMD(e.Path).DesignStep)
			if step != tickets.StepBranch && step != tickets.StepDone {
				continue
			}
			project, epic := tickets.ProjectName(dir), filepath.Base(e.Path)
			key := project + ":" + epic
			if !s.designScans.start(key) {
				continue
			}
			s.goSettle(func() {
				defer s.designScans.end(key)
				err := s.tryCleanupDesign(project, epic)
				text := ""
				if err != nil {
					text = err.Error()
				}
				if prev, _ := s.designWarned.Swap(key, text); text != "" && prev != text {
					s.log.Warn("design cleanup kept the worktree and branch", "map", key, "err", err)
				}
			})
		}
	}
}

// ensureDesignWorktree makes sure the map's design worktree exists and returns
// its path: on a new branch from trunk, or re-added on the existing branch.
func (s *Server) ensureDesignWorktree(project, epic string) (string, error) {
	projectDir, repoDir, err := s.projectOf(project)
	if err != nil {
		return "", err
	}
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		return "", err
	}
	branch := designBranch(epic)
	path := filepath.Join(repo.LinkedWorktreeDir(), branch)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if tickets.BranchExists(repo.Root, branch) {
		return path, git.AddWorktreeOnBranch(*repo, branch, path)
	}
	trunk, err := tickets.Trunk(projectDir, repo.Root)
	if err != nil {
		return "", err
	}
	return path, git.AddWorktree(*repo, branch, path, trunk)
}

// rebaseDesign rebases the design worktree's branch onto trunk, so the agent
// reads current code and docs. A problem never blocks the open: it skips the
// rebase, leaves the branch at its old tip and returns a note saying why.
// excludeID is a session that does not count as another live one.
func (s *Server) rebaseDesign(project, epic, worktree, excludeID string) string {
	live, err := s.liveDesignSessionExcept(project+":"+epic, excludeID)
	if err != nil {
		return "the design branch was not rebased onto trunk: " + err.Error()
	}
	if live != nil {
		return "the design branch was not rebased onto trunk: another design session of this map is live"
	}
	changes, err := git.UncommittedChanges(worktree)
	if err != nil {
		return "the design branch was not rebased onto trunk: " + err.Error()
	}
	if len(changes) > 0 {
		return "the design branch was not rebased onto trunk: the worktree has uncommitted edits"
	}
	projectDir, repoDir, err := s.projectOf(project)
	if err != nil {
		return "the design branch was not rebased onto trunk: " + err.Error()
	}
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		return "the design branch was not rebased onto trunk: " + err.Error()
	}
	trunk, err := tickets.Trunk(projectDir, repo.Root)
	if err != nil {
		return "the design branch was not rebased onto trunk: " + err.Error()
	}
	out, err := git.Rebase(worktree, trunk)
	if err == nil {
		return ""
	}
	if inProgress, _ := git.RebaseInProgress(worktree); inProgress {
		if abortErr := git.AbortRebase(worktree); abortErr != nil {
			return "the design branch was not rebased onto trunk: the rebase conflicted and could not be aborted: " + abortErr.Error()
		}
		return "the design branch was not rebased onto trunk: the rebase conflicted and was aborted"
	}
	return "the design branch was not rebased onto trunk: " + strings.TrimSpace(out+" "+err.Error())
}

// DesignOpenRequest is the body of POST /v1/designs/open. Epic is a map
// epic's address ("project:epic") or one of its tickets ("project:epic/NN").
// New stops the Design's live session and starts a fresh one; State is the
// token of the live session the web saw, required on the web mux for New.
type DesignOpenRequest struct {
	Epic  string `json:"epic"`
	New   bool   `json:"new,omitempty"`
	State string `json:"state,omitempty"`
	// NewDesign starts a Design for Idea that has no map yet: Epic is then the
	// slug the map will take in Project, and no epic is created.
	NewDesign bool   `json:"new_design,omitempty"`
	Idea      string `json:"idea,omitempty"`
	Project   string `json:"project,omitempty"`
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
		var req DesignOpenRequest
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
func (s *Server) openDesign(req DesignOpenRequest, web bool) (SessionResult, error) {
	if req.NewDesign {
		return s.openNewDesign(req)
	}
	epicAddr, ticket, _ := strings.Cut(req.Epic, "/")
	project, epic, ok := strings.Cut(epicAddr, ":")
	if !ok || project == "" || epic == "" || (strings.Contains(req.Epic, "/") && !designTicketRe.MatchString(ticket)) {
		return sessionRefusal(ReasonSessionBadRequest, fmt.Sprintf("%q is not an epic address (want project:epic or project:epic/NN)", req.Epic)), nil
	}
	dir, reason, msg := s.mapEpicDir(project, epic)
	if reason != "" {
		return sessionRefusal(reason, msg), nil
	}

	mu, _ := s.designOpens.LoadOrStore(epicAddr, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	live, err := s.liveDesignSession(epicAddr)
	if err != nil {
		return SessionResult{}, err
	}
	if live != nil && !req.New {
		return SessionResult{Session: live}, nil
	}
	if msg := designClash(dir, project, epic); msg != "" {
		return sessionRefusal(ReasonDesignBranchClash, msg), nil
	}
	if live != nil {
		if web && req.State != SessionStateToken(*live) {
			return sessionRefusal(ReasonStale, "the session changed; review its current state"), nil
		}
		if res, err := s.stopSession(SessionRequest{ID: live.ID}); err != nil || (res.Refused && res.Reason != ReasonSessionNotLive) {
			return res, err
		}
	}

	past, err := s.afterBranch(project, epic)
	if err != nil {
		return SessionResult{}, err
	}
	var worktree, note string
	if past {
		s.cleanupDesign(project, epic)
		if worktree = tickets.TrunkCheckout(s.cfg.TicketStore, project); worktree == "" {
			return SessionResult{}, fmt.Errorf("no trunk checkout for project %s", project)
		}
	} else {
		if worktree, err = s.ensureDesignWorktree(project, epic); err != nil {
			return SessionResult{}, err
		}
		note = s.rebaseDesign(project, epic, worktree, "")
	}
	prompt := "/gx-design " + epicAddr
	if ticket != "" {
		prompt += "/" + ticket
	}
	return s.createSession(CreateSessionRequest{
		Project:      project,
		Dir:          worktree,
		madeWorktree: !past,
		Name:         epic,
		Ticket:       epicAddr,
		Prompt:       prompt,
		Harness:      s.settings.Load().chatHarness(),
		rebaseNote:   note,
	})
}

// mapEpicDir returns the project dir of a map epic, or the refusal reason and
// message when the project or epic is unknown or the epic is not a map.
func (s *Server) mapEpicDir(project, epic string) (dir, reason, msg string) {
	dir, err := s.projectDir(project)
	if err != nil {
		return "", ReasonDesignUnknownEpic, err.Error()
	}
	epicDir := filepath.Join(dir, epic)
	if fi, err := os.Stat(epicDir); err != nil || !fi.IsDir() {
		return "", ReasonDesignUnknownEpic, fmt.Sprintf("no epic %s:%s", project, epic)
	}
	if !tickets.IsMapEpic(epicDir) {
		return "", ReasonDesignNotMapEpic, fmt.Sprintf("%s:%s is not a map epic", project, epic)
	}
	return dir, "", ""
}

// DesignWorktreeRequest is the body of POST /v1/designs/worktree. Epic is a map
// epic's address ("project:epic"). ExcludeSession is the caller's own gx
// session, which does not count as another live design session in the rebase.
type DesignWorktreeRequest struct {
	Epic           string `json:"epic"`
	ExcludeSession string `json:"exclude_session,omitempty"`
}

// DesignWorktreeResult answers designs worktree. Path is the Design's worktree,
// or the trunk checkout once AfterBranch. Note says why the rebase was skipped;
// it is empty when the rebase ran. A refusal is a result, as for designs open.
type DesignWorktreeResult struct {
	Path        string `json:"path,omitempty"`
	Note        string `json:"note,omitempty"`
	AfterBranch bool   `json:"after_branch"`
	Refused     bool   `json:"refused,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Message     string `json:"message,omitempty"`
}

func designWorktreeRefusal(reason, msg string) DesignWorktreeResult {
	return DesignWorktreeResult{Refused: true, Reason: reason, Message: msg}
}

func (s *Server) designWorktreeWrite(w http.ResponseWriter, r *http.Request) {
	var req DesignWorktreeRequest
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
func (s *Server) designWorktree(req DesignWorktreeRequest) (DesignWorktreeResult, error) {
	project, epic, ok := strings.Cut(req.Epic, ":")
	if !ok || project == "" || epic == "" || strings.Contains(epic, "/") {
		return designWorktreeRefusal(ReasonSessionBadRequest, fmt.Sprintf("%q is not an epic address (want project:epic)", req.Epic)), nil
	}
	dir, reason, msg := s.mapEpicDir(project, epic)
	if reason != "" {
		return designWorktreeRefusal(reason, msg), nil
	}

	mu, _ := s.designOpens.LoadOrStore(req.Epic, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	if msg := designClash(dir, project, epic); msg != "" {
		return designWorktreeRefusal(ReasonDesignBranchClash, msg), nil
	}
	past, err := s.afterBranch(project, epic)
	if err != nil {
		return DesignWorktreeResult{}, err
	}
	if past {
		s.cleanupDesign(project, epic)
		path := tickets.TrunkCheckout(s.cfg.TicketStore, project)
		if path == "" {
			return DesignWorktreeResult{}, fmt.Errorf("no trunk checkout for project %s", project)
		}
		return DesignWorktreeResult{Path: path, AfterBranch: true}, nil
	}
	path, err := s.ensureDesignWorktree(project, epic)
	if err != nil {
		return DesignWorktreeResult{}, err
	}
	return DesignWorktreeResult{Path: path, Note: s.rebaseDesign(project, epic, path, req.ExcludeSession)}, nil
}

// openNewDesign starts a design session for an idea with no map yet: Epic is
// the slug the map will take in Project. It creates no epic; Chart writes it.
func (s *Server) openNewDesign(req DesignOpenRequest) (SessionResult, error) {
	slug := req.Epic
	if !designSlugRe.MatchString(slug) {
		return sessionRefusal(ReasonSessionBadRequest, fmt.Sprintf("%q is not a slug (want lowercase words joined by -)", slug)), nil
	}
	idea := strings.TrimSpace(normalizeAnswer(req.Idea))
	switch {
	case idea == "":
		return sessionRefusal(ReasonEmptyPrompt, "an idea is required"), nil
	case len(idea) > maxSessionPromptBytes:
		return sessionRefusal(ReasonPromptTextTooLong, fmt.Sprintf("idea is over %d bytes", maxSessionPromptBytes)), nil
	}
	dir, err := s.projectDir(req.Project)
	if err != nil {
		return sessionRefusal(ReasonSessionBadRequest, err.Error()), nil
	}
	addr := req.Project + ":" + slug

	mu, _ := s.designOpens.LoadOrStore(addr, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	if _, err := os.Stat(filepath.Join(dir, slug)); err == nil {
		return sessionRefusal(ReasonDesignEpicExists, fmt.Sprintf("%s already has an epic %s", req.Project, slug)), nil
	}
	if msg := designClash(dir, req.Project, slug); msg != "" {
		return sessionRefusal(ReasonDesignBranchClash, msg), nil
	}

	if err := createMap(dir, slug, idea); err != nil {
		return SessionResult{}, err
	}
	worktree, err := s.ensureDesignWorktree(req.Project, slug)
	if err != nil {
		return SessionResult{EpicCreated: true, StartError: err.Error()}, nil
	}
	return s.startSession(req.Project, slug, worktree, s.rebaseDesign(req.Project, slug, worktree, "")), nil
}

// createMap writes the uncharted map epic for a New Design, at the Chart step.
func createMap(dir, slug, idea string) error {
	epicPath, err := tickets.CreateEpic(dir, slug, tickets.EpicFields{Kind: tickets.KindMap, Status: string(schema.StatusOpen)}, "## Initial prompt\n\n"+idea+"\n")
	if err != nil {
		return err
	}
	step := string(tickets.StepChart)
	return tickets.UpdateEpicFields(filepath.Join(epicPath, "ticket.md"), tickets.EpicUpdate{DesignStep: &step})
}

// startSession starts the design session for a map that now exists. A failure
// is a result, not a refusal: the map is there, so the caller reports it as
// created with the reason in StartError.
func (s *Server) startSession(project, slug, worktree, rebaseNote string) SessionResult {
	addr := project + ":" + slug
	res, err := s.createSession(CreateSessionRequest{
		Project:      project,
		Dir:          worktree,
		madeWorktree: true,
		Name:         slug,
		Ticket:       addr,
		Prompt:       "/gx-design " + addr,
		Harness:      s.settings.Load().chatHarness(),
		rebaseNote:   rebaseNote,
	})
	switch {
	case err != nil:
		return SessionResult{EpicCreated: true, StartError: err.Error()}
	case res.Refused:
		return SessionResult{EpicCreated: true, StartError: res.Message}
	}
	return res
}

// Refusal reason of the designs delete route, beside the unknown and not-map ones.
const ReasonDesignNotUncharted = "not-uncharted"

// DesignDeleteRequest is the body of POST /v1/designs/delete. Epic is the map
// epic's address ("project:epic").
type DesignDeleteRequest struct {
	Epic string `json:"epic"`
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
	var req DesignDeleteRequest
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
func (s *Server) deleteDesign(req DesignDeleteRequest) (SessionResult, error) {
	project, epic, ok := strings.Cut(req.Epic, ":")
	if !ok || project == "" || epic == "" || strings.Contains(epic, "/") {
		return sessionRefusal(ReasonSessionBadRequest, fmt.Sprintf("%q is not an epic address (want project:epic)", req.Epic)), nil
	}
	dir, err := s.projectDir(project)
	if err != nil {
		return sessionRefusal(ReasonDesignUnknownEpic, err.Error()), nil
	}

	mu, _ := s.designOpens.LoadOrStore(req.Epic, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	epics, err := tickets.LoadResolved(project, dir)
	if err != nil {
		return SessionResult{}, err
	}
	i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return e.Name == epic })
	if i < 0 {
		return sessionRefusal(ReasonDesignUnknownEpic, fmt.Sprintf("no epic %s", req.Epic)), nil
	}
	if !epics[i].IsMap {
		return sessionRefusal(ReasonDesignNotMapEpic, fmt.Sprintf("%s is not a map epic", req.Epic)), nil
	}
	uncharted, err := s.designUncharted(req.Epic, epics[i])
	if err != nil {
		return SessionResult{}, err
	}
	if !uncharted {
		return sessionRefusal(ReasonDesignNotUncharted, fmt.Sprintf("%s is charted or has a live session", req.Epic)), nil
	}
	if err := s.forceRemoveDesign(project, epic); err != nil {
		return SessionResult{}, err
	}
	if err := os.RemoveAll(epics[i].Path); err != nil {
		return SessionResult{}, err
	}
	s.events.publish(EventTicketChanged, req.Epic)
	return SessionResult{}, nil
}

// forceRemoveDesign drops the map's design worktree and branch whatever they
// hold: deleting the Design discards its edits and commits. A map that never
// had either is a no-op.
func (s *Server) forceRemoveDesign(project, epic string) error {
	_, repoDir, err := s.projectOf(project)
	if err != nil {
		return err
	}
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		return err
	}
	branch := designBranch(epic)
	path := filepath.Join(repo.LinkedWorktreeDir(), branch)
	if _, err := os.Stat(path); err == nil {
		if err := git.RemoveWorktree(*repo, path, true); err != nil {
			return err
		}
	}
	if !tickets.BranchExists(repo.Root, branch) {
		return nil
	}
	return git.DeleteLocalBranch(*repo, branch, true)
}

// liveDesignSession is the session of the Design at epicAddr that has not
// exited and has the latest RunStartedAt, or nil. The Design page picks by the
// same key (webui/src/design/view.ts).
func (s *Server) liveDesignSession(epicAddr string) (*SessionInfo, error) {
	return s.liveDesignSessionExcept(epicAddr, "")
}

// liveDesignSessionExcept is liveDesignSession leaving out the session
// excludeID, so a caller never counts itself. Empty excludes none.
func (s *Server) liveDesignSessionExcept(epicAddr, excludeID string) (*SessionInfo, error) {
	list, err := s.listSessions()
	if err != nil {
		return nil, err
	}
	var live *SessionInfo
	for i := range list {
		in := &list[i]
		if in.ID != excludeID && in.Design && in.Status.State != SessionExited && epicAddress(in.Ticket) == epicAddr &&
			(live == nil || !in.RunStartedAt.Before(live.RunStartedAt)) {
			live = in
		}
	}
	return live, nil
}
