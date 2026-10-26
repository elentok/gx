package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// RepairRequest is the body of the repair verbs; each uses the fields it needs.
// Cwd and Branch are the caller's context: the server evaluates the caller
// guards on them, so no client can skip a guard by not implementing it.
type RepairRequest struct {
	// Address is the ticket's project:epic/NN; verify also takes project:epic
	// to check the whole epic.
	Address string `json:"address"`
	Cwd     string `json:"cwd,omitempty"`
	Branch  string `json:"branch,omitempty"`

	// merge
	From          string `json:"from,omitempty"`
	To            string `json:"to,omitempty"`
	IgnoreLiveTab bool   `json:"ignore_live_tab,omitempty"`
	Continue      bool   `json:"continue,omitempty"`
	Abort         bool   `json:"abort,omitempty"`

	// start-over
	Reason       string `json:"reason,omitempty"`
	Force        bool   `json:"force,omitempty"`
	DeleteBranch bool   `json:"delete_branch,omitempty"`

	// resume
	Base  string `json:"base,omitempty"`
	State string `json:"state,omitempty"`
	// RequireState is set by the web mux, never by a client: the resume then
	// needs a State token that still matches the pause round.
	RequireState bool `json:"-"`
	// origin is web, tui or cli, set by repairWriteAs from the request.
	origin string
}

// ReasonBadBase refuses an resume whose base is not one of the competing blockers.
const ReasonBadBase = "bad-base"

// zeroCommitPrompt is what a live agent receives when a person resumes its
// zero-commit finish.
const zeroCommitPrompt = "A person reviewed your finish with no commits and wants you to continue. " +
	"Finish the work, or report `--iteration-status finished --commitless true`."

// retryPrompts is what a live agent receives when a person retries one of the
// pauses whose agent is still running. The retry route prompts instead of
// relaunching for exactly these kinds.
var retryPrompts = map[schema.PauseReason]string{
	schema.PauseZeroCommit: zeroCommitPrompt,
	schema.PauseBlockedPane: "A person reviewed your blocked prompt and wants you to continue. " +
		"Carry on with the work.",
	schema.PauseBackgroundTaskGate: "A person reviewed the background task you were waiting on and wants you to continue. " +
		"Finish the work.",
}

// RepairResult is what every repair verb returns: the verb's own result in
// Data, or a refusal carrying the repair package's stable reason code.
type RepairResult struct {
	Data    json.RawMessage `json:"data,omitempty"`
	Refused bool            `json:"refused,omitempty"`
	Reason  string          `json:"reason,omitempty"`
	Message string          `json:"message,omitempty"`

	// Via is "direct" when the CLI ran the verb itself because no server was
	// running; empty when the server served it.
	Via string `json:"via,omitempty"`
}

// ViaDirect marks a repair verb the CLI ran without a server.
const ViaDirect = "direct"

// RunRepairDirect runs a repair verb without a server, for the CLI's
// server-down fallback. The verbs take the merge lock themselves, so a live
// server and a direct run still exclude each other.
func RunRepairDirect(ticketStore, verb string, req RepairRequest) (RepairResult, error) {
	s := &Server{cfg: Config{TicketStore: ticketStore}}
	s.settings.Store(&Settings{})
	var do func(RepairRequest) (RepairResult, error)
	switch verb {
	case "merge":
		do = s.repairMerge
	case "start-over":
		do = s.repairStartOver
	case "resume", "unpark":
		do = s.repairResume
	case "verify":
		do = s.repairVerify
	default:
		return RepairResult{}, fmt.Errorf("unknown repair verb %q", verb)
	}
	res, err := do(req)
	res.Via = ViaDirect
	return res, err
}

func repairRefusal(err error) RepairResult {
	reason := repair.ReasonError
	var r *repair.RefusalError
	if errors.As(err, &r) {
		reason = r.Reason
	}
	return RepairResult{Refused: true, Reason: reason, Message: err.Error()}
}

func repairOK(v any) (RepairResult, error) {
	data, err := json.Marshal(v)
	return RepairResult{Data: data}, err
}

// epicTarget resolves a request's address to the epic's directory and the
// ticket id (empty for an epic-only address) and the directory the verb runs
// in: the caller's, else the project's repo (preferProject flips that order).
func (s *Server) epicTarget(req RepairRequest, needID, preferProject bool) (epicPath, id, cwd string, err error) {
	raw, id, hasID := strings.Cut(req.Address, "/")
	if hasID || needID {
		a, perr := tickets.ParseAddress(req.Address, tickets.AddressContext{})
		if perr != nil {
			return "", "", "", &repair.RefusalError{Reason: ReasonInvalidAddress, Message: perr.Error()}
		}
		raw, id = a.Project+":"+a.Epic, a.ID
	}
	project, epic, ok := strings.Cut(raw, ":")
	if !ok || project == "" || epic == "" {
		return "", "", "", &repair.RefusalError{Reason: ReasonInvalidAddress, Message: fmt.Sprintf("%q is not an epic address (want project:epic)", raw)}
	}
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return "", "", "", err
	}
	for _, d := range dirs {
		if tickets.ProjectName(d) != project {
			continue
		}
		cwd = req.Cwd
		if cwd == "" || preferProject {
			if _, repo, err := s.projectOf(project); err == nil {
				cwd = repo
			}
		}
		return filepath.Join(d, epic), id, cwd, nil
	}
	return "", "", "", &repair.RefusalError{Reason: ReasonUnknownTicket, Message: "no project " + project + " in the ticket store"}
}

func (s *Server) repairMerge(req RepairRequest) (RepairResult, error) {
	if repair.IsGuardedBranch(req.Branch) {
		return repairRefusal(&repair.RefusalError{Reason: repair.ReasonRalphLoopCwd, Message: fmt.Sprintf("refusing to merge from a ralph-loop/* working directory (%s); run from outside the iteration worktree", req.Branch)}), nil
	}
	epicPath, id, cwd, err := s.epicTarget(req, true, true)
	if err != nil {
		return repairRefusal(err), nil
	}
	if res, refused, err := staleTicket(epicPath, id, req); refused || err != nil {
		return res, err
	}
	res, _, err := repair.Merge(repair.MergeInput{
		EpicPath: epicPath, ID: id, From: req.From, To: req.To,
		IgnoreLiveTab: req.IgnoreLiveTab, Continue: req.Continue, Abort: req.Abort, Source: req.origin, Cwd: cwd,
	}, ralphloop.DefaultDeps())
	if err != nil {
		return repairRefusal(err), nil
	}
	return repairOK(res)
}

// mergeInQueue is the server's merge. A ticket of a queued root may be the
// one a merging paused, so once it merges the root completes as if its
// iteration had settled. A direct merge has no queue.
func (s *Server) mergeInQueue(req RepairRequest) (RepairResult, error) {
	addr, err := tickets.ParseAddress(req.Address, tickets.AddressContext{})
	if err != nil {
		return s.repairMerge(req)
	}
	res, err := s.repairMerge(req)
	if err != nil || res.Refused {
		return res, err
	}
	if err := s.cancelInvestigateOf(req.Address, req.origin); err != nil {
		return RepairResult{}, err
	}
	if !s.rootQueued(rootOf(addr)) {
		return res, nil
	}
	if _, t, ok := s.freshTicket(addr); ok && s.merges.begin() {
		defer s.merges.end()
		s.completeRootIfDone(rootOf(addr), ralphloop.OneIteration{Epic: addr.Epic, Ticket: t})
	}
	return res, nil
}

func (s *Server) rootQueued(root rootRef) bool {
	for _, it := range s.queued.list() {
		if a, err := parseQueuedAddress(it.Address); err == nil && rootOf(a) == root {
			return true
		}
	}
	return false
}

// freshTicket loads addr's ticket, and its epic, straight from disk.
func (s *Server) freshTicket(addr tickets.Address) (tickets.Epic, tickets.Ticket, bool) {
	e, ok := s.freshEpic(addr.Project, addr.Epic)
	if !ok {
		return tickets.Epic{}, tickets.Ticket{}, false
	}
	for _, t := range e.Tickets {
		if t.Identifier == addr.ID {
			return e, t, true
		}
	}
	return tickets.Epic{}, tickets.Ticket{}, false
}

// startOverInQueue is the server's start-over: a person's, so it also cancels
// the ticket's open investigate child. A direct start-over has no server to
// stop an agent.
func (s *Server) startOverInQueue(req RepairRequest) (RepairResult, error) {
	res, err := s.repairStartOver(req)
	if err != nil || res.Refused {
		return res, err
	}
	if err := s.cancelInvestigateOf(req.Address, req.origin); err != nil {
		return RepairResult{}, err
	}
	return res, nil
}

func (s *Server) repairStartOver(req RepairRequest) (RepairResult, error) {
	epicPath, id, cwd, err := s.epicTarget(req, true, false)
	if err != nil {
		return repairRefusal(err), nil
	}
	if res, refused, err := staleTicket(epicPath, id, req); refused || err != nil {
		return res, err
	}
	if _, t, ferr := repair.FindEpicTicket(epicPath, id); ferr == nil && t.Type == string(schema.TypeTrunkConflict) {
		addr := tickets.Address{Project: tickets.ProjectName(filepath.Dir(epicPath)), Epic: filepath.Base(epicPath), ID: id}
		return s.startOverTrunkConflict(req, addr, t)
	}
	// The web never deletes the branch, whatever the body says.
	deleteBranch := req.DeleteBranch && !req.RequireState
	res, err := repair.StartOver(repair.StartOverInput{
		EpicPath: epicPath, ID: id, Reason: req.Reason, Force: req.Force,
		DeleteBranch: deleteBranch, Source: req.origin, Cwd: cwd, Now: time.Now(), Subjects: repair.GitCommitSubjects,
	}, s.startOverDeps(req.Address))
	if err != nil {
		return repairRefusal(err), nil
	}
	return repairOK(res)
}

// startOverDeps talks to the runner hosting address's iteration, so start-over sees
// and closes the agent where it actually runs. A direct start-over has no server
// runner and keeps the default.
func (s *Server) startOverDeps(address string) ralphloop.Deps {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if s.cfg.Runner == nil || err != nil {
		return ralphloop.DefaultDeps()
	}
	// An unavailable runner is nil, which reports no session, as for an
	// unreachable host.
	runner, _ := s.iterationRunner(addr)
	return s.depsWith(runner)
}

func (s *Server) repairResume(req RepairRequest) (RepairResult, error) {
	epicPath, id, _, err := s.epicTarget(req, true, false)
	if err != nil {
		return repairRefusal(err), nil
	}
	dir := filepath.Dir(epicPath)
	addr := tickets.Address{Project: tickets.ProjectName(dir), Epic: filepath.Base(epicPath), ID: id}
	_, t, ferr := repair.FindEpicTicket(epicPath, id)
	if ferr != nil {
		// Not a ticket of this epic: the plain resume reports why.
		res, err := repair.Resume(epicPath, id, time.Now(), "")
		if err != nil {
			return repairRefusal(err), nil
		}
		return repairOK(res)
	}
	tk, perr := schema.ParseTicket(t.Path)
	paused := perr == nil && tk.PausedAsking()
	zeroCommit := paused && tk.PauseReason == schema.PauseZeroCommit
	// Every refusal runs before the first state change.
	if perr == nil && tk.PausedClass() != "" && req.RequireState {
		res, refused, err := webStale(t.Path, req)
		if err != nil {
			return RepairResult{}, err
		}
		if refused {
			return res, nil
		}
	}
	if req.Base != "" {
		if blockers := s.ambiguousBlockers(addr); !slices.Contains(blockers, req.Base) {
			msg := fmt.Sprintf("base %q is not one of the competing blockers (%s)", req.Base, strings.Join(blockers, ", "))
			return repairRefusal(&repair.RefusalError{Reason: ReasonBadBase, Message: msg}), nil
		}
	}
	if s.adoptFirst(dir, addr, t.Path) {
		res := repair.ResumeResult{Ticket: id, Status: repair.ResumeAdopted}
		if zeroCommit {
			res.Message = repair.MessageAdopted
		}
		return repairOK(res)
	}
	if res, refused, err := s.resumeSelfReported(epicPath, id, req.origin); refused || err != nil {
		return res, err
	}
	if prompt, ok := retryPrompts[tk.PauseReason]; paused && ok {
		prompted, err := s.promptLiveAgent(dir, addr, prompt)
		if err != nil {
			return repairRefusal(err), nil
		}
		if prompted {
			if err := tickets.PromptOpenRound(t.Path, req.origin); err != nil {
				return RepairResult{}, fmt.Errorf("log prompt of %s: %w", addr, err)
			}
			ev := ralphloop.Event{Type: string(events.PausePrompted), Ticket: id, Address: addr.String(), Text: prompt}
			if err := ralphloop.AppendEvent(dir, addr.Epic, ev); err != nil {
				s.log.Warn("logging pause prompt", "ticket", addr.String(), "err", err)
			}
			s.log.Info("prompted the live agent of a paused ticket", "ticket", addr.String(), "origin", req.origin)
			return repairOK(repair.ResumeResult{Ticket: id, Status: repair.ResumePrompted, Message: repair.MessagePrompted})
		}
	}
	var res repair.ResumeResult
	if paused && req.RequireState && tk.PauseReason != schema.PauseSelfReported {
		var refused *RepairResult
		res, refused, err = resumeAnswered(t.Path, id, req)
		if refused != nil {
			return *refused, nil
		}
		if err != nil {
			return RepairResult{}, fmt.Errorf("resume of %s: %w", addr, err)
		}
	} else {
		res, err = repair.Resume(epicPath, id, time.Now(), req.Base)
		if err != nil {
			return repairRefusal(err), nil
		}
		if err := tickets.EndOpenRound(t.Path, req.origin, tickets.HowResume, ralphloop.CloseResumed); err != nil {
			return RepairResult{}, fmt.Errorf("end pause round of %s: %w", addr, err)
		}
	}
	if zeroCommit {
		res.Message = repair.MessageReopened
	}
	return repairOK(res)
}

// webStale refuses a web verb whose token no longer names the ticket's open
// pause round and class; a pause without an open round has nothing to name.
func webStale(ticketPath string, req RepairRequest) (RepairResult, bool, error) {
	if !req.RequireState {
		return RepairResult{}, false, nil
	}
	stale, err := ticketPauseStale(ticketPath, req.State)
	if err != nil || !stale {
		return RepairResult{}, false, err
	}
	return staleResumeRefusal(), true, nil
}

// staleTicket is webStale for the ticket id of the epic at epicPath. A ticket
// that does not exist is left to the verb to refuse.
func staleTicket(epicPath, id string, req RepairRequest) (RepairResult, bool, error) {
	if !req.RequireState {
		return RepairResult{}, false, nil
	}
	_, t, err := repair.FindEpicTicket(epicPath, id)
	if err != nil {
		return RepairResult{}, false, nil
	}
	return webStale(t.Path, req)
}

func staleResumeRefusal() RepairResult {
	return repairRefusal(&repair.RefusalError{Reason: ReasonStale, Message: "the pause changed; review the current one"})
}

// resumeRefused carries a refusal out of the lock-held append callback.
type resumeRefused struct{ res RepairResult }

func (e resumeRefused) Error() string { return "resume refused" }

// resumeAnswered logs the person's resume as the answer to the open question
// and reopens the ticket under one ticket lock. The state token is checked
// again under the lock, and a refusal or a failed reopen appends nothing, so a
// failure never leaves an answered question on a paused ticket.
func resumeAnswered(ticketPath, id string, req RepairRequest) (repair.ResumeResult, *RepairResult, error) {
	_, err := tickets.AppendLogFrom(ticketPath, func(evs []tickets.LogEvent) (tickets.LogEvent, error) {
		t, err := schema.ParseTicket(ticketPath)
		if err != nil {
			return tickets.LogEvent{}, err
		}
		open := tickets.OpenQuestion(evs)
		if !t.PausedAsking() || open == nil || answerState(*open, t.PausedClass()) != req.State {
			return tickets.LogEvent{}, resumeRefused{staleResumeRefusal()}
		}
		if err := ralphloop.ResumeTicketWithBaseLocked(ticketPath, time.Now(), req.Base); err != nil {
			return tickets.LogEvent{}, resumeRefused{repairRefusal(err)}
		}
		return tickets.LogEvent{
			Type: tickets.EventPauseAnswer, Source: req.origin, How: tickets.HowResume, Question: open.Seq, Choice: "unpark", Base: req.Base,
		}, nil
	})
	var refused resumeRefused
	if errors.As(err, &refused) {
		return repair.ResumeResult{}, &refused.res, nil
	}
	if err != nil {
		return repair.ResumeResult{}, nil, err
	}
	return repair.ResumeResult{Ticket: id, Status: "open"}, nil, nil
}

// ambiguousBlockers lists the blockers a ticket paused on an ambiguous base
// has to choose between, recomputed from the epics; none when it is not ambiguous.
func (s *Server) ambiguousBlockers(addr tickets.Address) []string {
	dir, err := s.projectDir(addr.Project)
	if err != nil {
		return nil
	}
	epics, err := tickets.LoadResolved(addr.Project, dir)
	if err != nil {
		return nil
	}
	for _, e := range epics {
		if filepath.Base(e.Path) != addr.Epic {
			continue
		}
		for _, t := range e.Tickets {
			if t.Identifier != addr.ID {
				continue
			}
			var amb *tickets.AmbiguousBaseError
			if _, err := tickets.DeriveLeafBase(addr.Project, epics, addr.Epic, t); errors.As(err, &amb) {
				return amb.Blockers
			}
			if _, err := tickets.DeriveRootBase(addr.Project, epics, addr.Epic, t, s.mergingOnMerged(addr.Project)); errors.As(err, &amb) {
				return amb.Blockers
			}
		}
	}
	return nil
}

// promptLiveAgent sends text to the iteration's agent when it is still
// running, and reports whether it did. A direct run has no runner and finds none.
func (s *Server) promptLiveAgent(dir string, addr tickets.Address, text string) (bool, error) {
	runner, sess, ok, err := s.iterationSession(dir, addr)
	if err != nil || !ok {
		return false, err
	}
	switch err := runner.Prompt(sess, text); {
	case errors.Is(err, agentrunner.ErrNotFound), errors.Is(err, agentrunner.ErrSessionDied):
		return false, nil
	case errors.Is(err, agentrunner.ErrNotReady):
		return false, &repair.RefusalError{Reason: ReasonAgentNotReady, Message: err.Error()}
	case err != nil:
		return false, fmt.Errorf("prompt %s: %w", addr, err)
	}
	return true, nil
}

// iterationSession finds the iteration's agent session, if it is still running.
func (s *Server) iterationSession(dir string, addr tickets.Address) (agentrunner.Runner, agentrunner.Session, bool, error) {
	if s.registry == nil {
		return nil, agentrunner.Session{}, false, nil
	}
	label, _, _ := ralphloop.IterationIdentity(addr.Epic, addr.ID, s.worktreeDir(addr.Project))
	runner := s.runnerFor(addr.Project)
	if pause, ok := ralphloop.LatestPauseEvent(dir, addr.Epic, addr.ID); ok {
		if pause.Label != "" {
			label = pause.Label
		}
		if r := s.runnerForRun(Run{Address: addr.String(), Runner: pause.Runner}); pause.Runner != "" && r != nil {
			runner = r
		}
	}
	if runner == nil {
		return nil, agentrunner.Session{}, false, nil
	}
	sess, ok, err := runner.Find(label)
	return runner, sess, ok, err
}

// resumeSelfReported applies the rule for a self-reported pause: a live waiting
// agent keeps its question, so resume refuses and names the answer verb; a gone
// agent's question is closed before the status moves on. Other tickets and kinds
// pass through untouched. A direct run has no runner to ask and reads every
// agent as gone, since no server is running to wait on an answer.
func (s *Server) resumeSelfReported(epicPath, id, origin string) (RepairResult, bool, error) {
	_, t, err := repair.FindEpicTicket(epicPath, id)
	if err != nil {
		return RepairResult{}, false, nil
	}
	tk, err := schema.ParseTicket(t.Path)
	if err != nil || !tk.PausedAsking() {
		return RepairResult{}, false, nil
	}
	selfReported := tk.PauseReason == schema.PauseSelfReported
	dir := filepath.Dir(epicPath)
	address := tickets.Address{Project: tickets.ProjectName(dir), Epic: filepath.Base(epicPath), ID: id}.String()
	if !selfReported && !s.waitsOnHeadlessBlock(tk, address) {
		return RepairResult{}, false, nil
	}
	if s.registry != nil {
		gone, err := s.agentGone(address)
		if err != nil {
			return RepairResult{}, false, err
		}
		if !gone {
			msg := fmt.Sprintf("%s has a live agent waiting on its question; reply with `gx tickets answer`", address)
			if !selfReported {
				msg = fmt.Sprintf("%s has a live agent blocked on a request; reply with `gx server agents answer`, or `gx server agents interrupt` if there is nothing to answer", address)
			}
			return repairRefusal(&repair.RefusalError{Reason: ReasonAgentWaiting, Message: msg}), true, nil
		}
		s.registry.delete(address)
	}
	if !selfReported {
		return RepairResult{}, false, nil
	}
	if err := tickets.EndOpenRound(t.Path, origin, tickets.HowResume, ralphloop.CloseResumed); err != nil {
		return RepairResult{}, false, fmt.Errorf("close question of %s: %w", address, err)
	}
	return RepairResult{}, false, nil
}

func (s *Server) repairVerify(req RepairRequest) (RepairResult, error) {
	epicPath, id, cwd, err := s.epicTarget(req, false, false)
	if err != nil {
		return repairRefusal(err), nil
	}
	run, err := repair.DefaultVerifyRun(epicPath, cwd)
	if err != nil {
		return repairRefusal(err), nil
	}
	run.ID = id
	res, err := repair.Verify(run)
	if err != nil {
		return repairRefusal(err), nil
	}
	return repairOK(res)
}

// repairWrite adapts a repair verb to a POST handler. Refusals are results, so
// they answer 200; only a malformed body or a failure to encode is an HTTP error.
func repairWrite(do func(RepairRequest) (RepairResult, error)) http.HandlerFunc {
	return repairWriteAs(false, do)
}

// repairWriteWeb is repairWrite for the web mux: the request needs a state token.
func repairWriteWeb(do func(RepairRequest) (RepairResult, error)) http.HandlerFunc {
	return repairWriteAs(true, do)
}

func repairWriteAs(web bool, do func(RepairRequest) (RepairResult, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RepairRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req.RequireState = web
		req.origin = requestOrigin(r, web)
		if web && req.State == "" {
			writeJSON(w, repairRefusal(&repair.RefusalError{Reason: ReasonStale, Message: "a state token is required; review the current pause"}))
			return
		}
		res, err := do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, res)
	}
}
