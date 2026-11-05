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

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/ticketwrite"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// repairCtx is what the server knows about a repair write that the body does
// not carry. It travels beside the wire.RepairRequest, never inside it.
type repairCtx struct {
	// RequireState is set by the web mux, never by a client: the resume then
	// needs a State token that still matches the pause round.
	RequireState bool
	// origin is web, tui or cli, set by repairWriteAs from the request.
	origin string
	// investigator is the investigate ticket whose agent made the call, from
	// InvestigateHeader. Its merge or start-over must not cancel its own ticket.
	investigator string
}

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

// RunRepairDirect runs a repair verb without a server, for the CLI's
// server-down fallback. The verbs take the merge lock themselves, so a live
// server and a direct run still exclude each other.
func RunRepairDirect(ticketStore, verb string, req wire.RepairRequest) (wire.RepairResult, error) {
	s := &Server{cfg: Config{TicketStore: ticketStore}}
	s.settings.Store(&Settings{})
	s.initProjects()
	var do func(wire.RepairRequest, repairCtx) (wire.RepairResult, error)
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
		return wire.RepairResult{}, fmt.Errorf("unknown repair verb %q", verb)
	}
	res, err := do(req, repairCtx{})
	res.Via = wire.ViaDirect
	return res, err
}

func repairRefusal(err error) wire.RepairResult {
	reason := repair.ReasonError
	var r *repair.RefusalError
	if errors.As(err, &r) {
		reason = r.Reason
	}
	return wire.RepairResult{Refused: true, Reason: reason, Message: err.Error()}
}

func repairOK(v any) (wire.RepairResult, error) {
	data, err := json.Marshal(v)
	return wire.RepairResult{Data: data}, err
}

// epicTarget resolves a request's address to the epic's directory and the
// ticket id (empty for an epic-only address) and the directory the verb runs
// in: the caller's, else the project's repo (preferProject flips that order).
func (s *Server) epicTarget(req wire.RepairRequest, needID, preferProject bool) (epicPath, id, cwd string, err error) {
	raw, id, hasID := strings.Cut(req.Address, "/")
	if hasID || needID {
		a, perr := tickets.ParseAddress(req.Address, tickets.AddressContext{})
		if perr != nil {
			return "", "", "", &repair.RefusalError{Reason: wire.ReasonInvalidAddress, Message: perr.Error()}
		}
		raw, id = a.Project+":"+a.Epic, a.ID
	}
	project, epic, ok := strings.Cut(raw, ":")
	if !ok || project == "" || epic == "" {
		return "", "", "", &repair.RefusalError{Reason: wire.ReasonInvalidAddress, Message: fmt.Sprintf("%q is not an epic address (want project:epic)", raw)}
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
			if _, repo, err := s.projects.Of(project); err == nil {
				cwd = repo
			}
		}
		return filepath.Join(d, epic), id, cwd, nil
	}
	return "", "", "", &repair.RefusalError{Reason: wire.ReasonUnknownTicket, Message: "no project " + project + " in the ticket store"}
}

func (s *Server) repairMerge(req wire.RepairRequest, rc repairCtx) (wire.RepairResult, error) {
	if repair.IsGuardedBranch(req.Branch) {
		return repairRefusal(&repair.RefusalError{Reason: repair.ReasonRalphLoopCwd, Message: fmt.Sprintf("refusing to merge from a ralph-loop/* working directory (%s); run from outside the iteration worktree", req.Branch)}), nil
	}
	epicPath, id, cwd, err := s.epicTarget(req, true, true)
	if err != nil {
		return repairRefusal(err), nil
	}
	if rc.RequireState {
		if res, refused, err := staleTicket(epicPath, id, req); refused || err != nil {
			return res, err
		}
	}
	res, _, err := repair.Merge(repair.MergeInput{
		EpicPath: epicPath, ID: id, From: req.From, To: req.To,
		IgnoreLiveTab: req.IgnoreLiveTab, Continue: req.Continue, Abort: req.Abort, Source: rc.origin, Cwd: cwd,
	}, s.hostingDeps(epicTicketAddress(epicPath, id)))
	if err != nil {
		return repairRefusal(err), nil
	}
	return repairOK(res)
}

// mergeInQueue is the server's merge. A ticket of a queued root may be the
// one a merging paused, so once it merges the root completes as if its
// iteration had settled. A direct merge has no queue.
func (s *Server) mergeInQueue(req wire.RepairRequest, rc repairCtx) (wire.RepairResult, error) {
	addr, err := tickets.ParseAddress(req.Address, tickets.AddressContext{})
	if err != nil {
		return s.repairMerge(req, rc)
	}
	res, err := s.repairMerge(req, rc)
	if err != nil || res.Refused {
		return res, err
	}
	if err := s.cancelInvestigateUnlessSelf(req, rc); err != nil {
		return wire.RepairResult{}, err
	}
	if !s.queue.RootQueued(queue.RootOf(addr)) {
		return res, nil
	}
	if _, t, ok := s.FreshTicket(addr); ok && s.guards.Merges.Begin() {
		defer s.guards.Merges.End()
		s.merger.CompleteRootIfDone(queue.RootOf(addr), ralphloop.OneIteration{Epic: addr.Epic, Ticket: t})
	}
	return res, nil
}

// FreshTicket loads addr's ticket, and its epic, straight from disk.
// It is the root's harness.Host method.
func (s *Server) FreshTicket(addr tickets.Address) (tickets.Epic, tickets.Ticket, bool) {
	return s.writer.FreshTicket(addr)
}

// startOverInQueue is the server's start-over: a person's, so it also cancels
// the ticket's open investigate child. A direct start-over has no server to
// stop an agent.
func (s *Server) startOverInQueue(req wire.RepairRequest, rc repairCtx) (wire.RepairResult, error) {
	res, err := s.repairStartOver(req, rc)
	if err != nil || res.Refused {
		return res, err
	}
	if err := s.cancelInvestigateUnlessSelf(req, rc); err != nil {
		return wire.RepairResult{}, err
	}
	return res, nil
}

// cancelInvestigateUnlessSelf cancels the ticket's investigate child, except
// when that investigate agent is the caller.
func (s *Server) cancelInvestigateUnlessSelf(req wire.RepairRequest, rc repairCtx) error {
	if rc.investigator != "" {
		return nil
	}
	return s.cancelInvestigateOf(req.Address, rc.origin)
}

func (s *Server) repairStartOver(req wire.RepairRequest, rc repairCtx) (wire.RepairResult, error) {
	// The web has no --reason flag to name; the CLI refusal text stays in repair.
	if rc.RequireState && strings.TrimSpace(req.Reason) == "" {
		return repairRefusal(&repair.RefusalError{Reason: repair.ReasonReasonRequired, Message: "a reason is required to start over"}), nil
	}
	epicPath, id, cwd, err := s.epicTarget(req, true, false)
	if err != nil {
		return repairRefusal(err), nil
	}
	if rc.RequireState {
		if res, refused, err := staleTicket(epicPath, id, req); refused || err != nil {
			return res, err
		}
	}
	if _, t, ferr := repair.FindEpicTicket(epicPath, id); ferr == nil && t.Type == string(schema.TypeTrunkConflict) {
		addr := tickets.Address{Project: tickets.ProjectName(filepath.Dir(epicPath)), Epic: filepath.Base(epicPath), ID: id}
		res, err := s.landing.StartOverTrunkConflict(req, rc.origin, addr, t)
		var refusal *repair.RefusalError
		if errors.As(err, &refusal) {
			return repairRefusal(refusal), nil
		}
		if err != nil {
			return wire.RepairResult{}, err
		}
		return repairOK(res)
	}
	// The web never deletes the branch, whatever the body says.
	deleteBranch := req.DeleteBranch && !rc.RequireState
	res, err := repair.StartOver(repair.StartOverInput{
		EpicPath: epicPath, ID: id, Reason: req.Reason, Force: req.Force,
		DeleteBranch: deleteBranch, Source: rc.origin, Cwd: cwd, Now: time.Now(), Subjects: repair.GitCommitSubjects,
	}, s.hostingDeps(epicTicketAddress(epicPath, id)))
	if err != nil {
		return repairRefusal(err), nil
	}
	return repairOK(res)
}

// hostingDeps talks to the runner hosting addr's iteration, so start-over and
// merge see (and close) the agent where it actually runs. A direct repair has
// no server runner, so there is no agent to ask and the defaults stand.
func (s *Server) hostingDeps(addr tickets.Address) ralphloop.Deps {
	if s.cfg.Runner == nil {
		return s.projects.DefaultDeps()
	}
	// An unavailable runner is nil, which reports no session, as for an
	// unreachable host.
	runner, _ := s.harness.IterationRunner(addr)
	return s.projects.DepsWith(runner)
}

func epicTicketAddress(epicPath, id string) tickets.Address {
	return tickets.Address{Project: tickets.ProjectName(filepath.Dir(epicPath)), Epic: filepath.Base(epicPath), ID: id}
}

func (s *Server) repairResume(req wire.RepairRequest, rc repairCtx) (wire.RepairResult, error) {
	epicPath, id, _, err := s.epicTarget(req, true, false)
	if err != nil {
		return repairRefusal(err), nil
	}
	if req.Tier != "" && !ralphloop.Tier(req.Tier).Valid() {
		msg := fmt.Sprintf("tier %q is not one of light, worker, strongest", req.Tier)
		return repairRefusal(&repair.RefusalError{Reason: wire.ReasonBadTier, Message: msg}), nil
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
	if perr == nil && tk.PausedClass() != "" && rc.RequireState {
		res, refused, err := webStale(t.Path, req)
		if err != nil {
			return wire.RepairResult{}, err
		}
		if refused {
			return res, nil
		}
	}
	if req.Base != "" {
		if blockers := s.ambiguousBlockers(addr); !slices.Contains(blockers, req.Base) {
			msg := fmt.Sprintf("base %q is not one of the competing blockers (%s)", req.Base, strings.Join(blockers, ", "))
			return repairRefusal(&repair.RefusalError{Reason: wire.ReasonBadBase, Message: msg}), nil
		}
	}
	if s.rec.AdoptFirst(dir, addr, t.Path) {
		res := repair.ResumeResult{Ticket: id, Status: repair.ResumeAdopted}
		if zeroCommit {
			res.Message = repair.MessageAdopted
		}
		return repairOK(res)
	}
	if res, refused, err := s.resumeSelfReported(epicPath, id, rc.origin); refused || err != nil {
		return res, err
	}
	if prompt, ok := retryPrompts[tk.PauseReason]; paused && ok {
		prompted, err := s.promptLiveAgent(dir, addr, prompt)
		if err != nil {
			return repairRefusal(err), nil
		}
		if prompted {
			if err := tickets.PromptOpenRound(t.Path, rc.origin); err != nil {
				return wire.RepairResult{}, fmt.Errorf("log prompt of %s: %w", addr, err)
			}
			ev := ralphloop.Event{Type: string(events.PausePrompted), Ticket: id, Address: addr.String(), Text: prompt}
			if err := ralphloop.AppendEvent(dir, addr.Epic, ev); err != nil {
				s.log.Warn("logging pause prompt", "ticket", addr.String(), "err", err)
			}
			s.log.Info("prompted the live agent of a paused ticket", "ticket", addr.String(), "origin", rc.origin)
			return repairOK(repair.ResumeResult{Ticket: id, Status: repair.ResumePrompted, Message: repair.MessagePrompted})
		}
	}
	// The override lands before the reopen so a scheduler claim cannot beat it;
	// a failed resume takes it back.
	if req.Tier != "" {
		if err := setTierOverride(t.Path, req.Tier); err != nil {
			return wire.RepairResult{}, fmt.Errorf("set tier of %s: %w", addr, err)
		}
	}
	var res repair.ResumeResult
	if paused && rc.RequireState && tk.PauseReason != schema.PauseSelfReported {
		var refused *wire.RepairResult
		res, refused, err = resumeAnswered(t.Path, id, req, rc.origin)
		if refused != nil || err != nil {
			clearTierOverride(t.Path, req.Tier)
		}
		if refused != nil {
			return *refused, nil
		}
		if err != nil {
			return wire.RepairResult{}, fmt.Errorf("resume of %s: %w", addr, err)
		}
	} else {
		res, err = repair.Resume(epicPath, id, time.Now(), req.Base)
		if err != nil {
			clearTierOverride(t.Path, req.Tier)
			return repairRefusal(err), nil
		}
		if err := tickets.EndOpenRound(t.Path, rc.origin, tickets.HowResume, ralphloop.CloseResumed); err != nil {
			return wire.RepairResult{}, fmt.Errorf("end pause round of %s: %w", addr, err)
		}
	}
	if zeroCommit {
		res.Message = repair.MessageReopened
	}
	return repairOK(res)
}

func setTierOverride(ticketPath, tier string) error {
	return schema.UpdateTicketWithBody(ticketPath, func(t *schema.Ticket, _ *string) { t.TierOverride = tier })
}

// clearTierOverride undoes setTierOverride for a resume that did not happen.
// An empty tier means none was set.
func clearTierOverride(ticketPath, tier string) {
	if tier != "" {
		_ = setTierOverride(ticketPath, "")
	}
}

// takeTierOverride returns the ticket's one-time tier and clears it, so only
// the launch that reads it runs on it. An unreadable ticket has none.
func takeTierOverride(ticketPath string) ralphloop.Tier {
	var tier string
	err := schema.UpdateTicketWithBody(ticketPath, func(t *schema.Ticket, _ *string) {
		tier, t.TierOverride = t.TierOverride, ""
	})
	if err != nil || !ralphloop.Tier(tier).Valid() {
		return ""
	}
	return ralphloop.Tier(tier)
}

// webStale refuses a web verb whose token no longer names the ticket's open
// pause round and class; a pause without an open round has nothing to name.
// Callers check repairCtx.RequireState first.
func webStale(ticketPath string, req wire.RepairRequest) (wire.RepairResult, bool, error) {
	stale, err := ticketwrite.PauseStale(ticketPath, req.State)
	if err != nil || !stale {
		return wire.RepairResult{}, false, err
	}
	return staleResumeRefusal(), true, nil
}

// staleTicket is webStale for the ticket id of the epic at epicPath. A ticket
// that does not exist is left to the verb to refuse.
func staleTicket(epicPath, id string, req wire.RepairRequest) (wire.RepairResult, bool, error) {
	_, t, err := repair.FindEpicTicket(epicPath, id)
	if err != nil {
		return wire.RepairResult{}, false, nil
	}
	return webStale(t.Path, req)
}

func staleResumeRefusal() wire.RepairResult {
	return repairRefusal(&repair.RefusalError{Reason: wire.ReasonStale, Message: "the pause changed; review the current one"})
}

// resumeRefused carries a refusal out of the lock-held append callback.
type resumeRefused struct{ res wire.RepairResult }

func (e resumeRefused) Error() string { return "resume refused" }

// resumeAnswered logs the person's resume as the answer to the open question
// and reopens the ticket under one ticket lock. The state token is checked
// again under the lock, and a refusal or a failed reopen appends nothing, so a
// failure never leaves an answered question on a paused ticket.
func resumeAnswered(ticketPath, id string, req wire.RepairRequest, origin string) (repair.ResumeResult, *wire.RepairResult, error) {
	_, err := tickets.AppendLogFrom(ticketPath, func(evs []tickets.LogEvent) (tickets.LogEvent, error) {
		t, err := schema.ParseTicket(ticketPath)
		if err != nil {
			return tickets.LogEvent{}, err
		}
		open := tickets.OpenQuestion(evs)
		if !t.PausedAsking() || open == nil || queue.AnswerState(*open, t.PausedClass()) != req.State {
			return tickets.LogEvent{}, resumeRefused{staleResumeRefusal()}
		}
		if err := ralphloop.ResumeTicketWithBaseLocked(ticketPath, time.Now(), req.Base); err != nil {
			return tickets.LogEvent{}, resumeRefused{repairRefusal(err)}
		}
		return tickets.LogEvent{
			Type: tickets.EventPauseAnswer, Source: origin, How: tickets.HowResume, Question: open.Seq, Choice: "unpark", Base: req.Base,
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
	dir, err := s.projects.Dir(addr.Project)
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
		return false, &repair.RefusalError{Reason: wire.ReasonAgentNotReady, Message: err.Error()}
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
	label, _, _ := ralphloop.IterationIdentity(addr.Epic, addr.ID, s.projects.WorktreeDir(addr.Project))
	runner := s.harness.RunnerFor(addr.Project)
	if pause, ok := ralphloop.LatestPauseEvent(dir, addr.Epic, addr.ID); ok {
		if pause.Label != "" {
			label = pause.Label
		}
		if r := s.harness.RunnerForRun(runs.Run{Address: addr.String(), Runner: pause.Runner}); pause.Runner != "" && r != nil {
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
func (s *Server) resumeSelfReported(epicPath, id, origin string) (wire.RepairResult, bool, error) {
	_, t, err := repair.FindEpicTicket(epicPath, id)
	if err != nil {
		return wire.RepairResult{}, false, nil
	}
	tk, err := schema.ParseTicket(t.Path)
	if err != nil || !tk.PausedAsking() {
		return wire.RepairResult{}, false, nil
	}
	selfReported := tk.PauseReason == schema.PauseSelfReported
	dir := filepath.Dir(epicPath)
	address := tickets.Address{Project: tickets.ProjectName(dir), Epic: filepath.Base(epicPath), ID: id}.String()
	if !selfReported && !s.writer.WaitsOnBlockedPause(tk, address) {
		return wire.RepairResult{}, false, nil
	}
	if s.registry != nil {
		gone, err := s.agentGone(address)
		if err != nil {
			return wire.RepairResult{}, false, err
		}
		if !gone && !selfReported {
			// A PTY or herdr run has no approvable card for a question menu, so
			// Retry is the way out: end the pause, interrupt, and let the caller
			// send the retry prompt.
			interrupted, err := s.writer.RetryBlocked(address)
			if errors.Is(err, agentrunner.ErrNotReady) {
				return repairRefusal(&repair.RefusalError{Reason: wire.ReasonAgentNotReady, Message: err.Error()}), true, nil
			}
			if err != nil || interrupted {
				return wire.RepairResult{}, false, err
			}
		}
		if !gone {
			msg := fmt.Sprintf("%s has a live agent waiting on its question; reply with `gx tickets answer`", address)
			if !selfReported {
				msg = fmt.Sprintf("%s has a live agent blocked on a request; reply with `gx server agents answer`, or `gx server agents interrupt` if there is nothing to answer", address)
			}
			return repairRefusal(&repair.RefusalError{Reason: wire.ReasonAgentWaiting, Message: msg}), true, nil
		}
		s.registry.Delete(address)
	}
	if !selfReported {
		return wire.RepairResult{}, false, nil
	}
	if err := tickets.EndOpenRound(t.Path, origin, tickets.HowResume, ralphloop.CloseResumed); err != nil {
		return wire.RepairResult{}, false, fmt.Errorf("close question of %s: %w", address, err)
	}
	return wire.RepairResult{}, false, nil
}

func (s *Server) repairVerify(req wire.RepairRequest, _ repairCtx) (wire.RepairResult, error) {
	epicPath, id, cwd, err := s.epicTarget(req, false, false)
	if err != nil {
		return repairRefusal(err), nil
	}
	run, err := repair.DefaultVerifyRun(epicPath, cwd, s.hostingDeps(epicTicketAddress(epicPath, id)).Runner)
	if err != nil {
		return repairRefusal(err), nil
	}
	if run.MergeLockStateDir, err = s.projects.LockStateDir(); err != nil {
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
func repairWrite(do func(wire.RepairRequest, repairCtx) (wire.RepairResult, error)) http.HandlerFunc {
	return repairWriteAs(false, do)
}

// repairWriteWeb is repairWrite for the web mux: the request needs a state token.
func repairWriteWeb(do func(wire.RepairRequest, repairCtx) (wire.RepairResult, error)) http.HandlerFunc {
	return repairWriteAs(true, do)
}

func repairWriteAs(web bool, do func(wire.RepairRequest, repairCtx) (wire.RepairResult, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.RepairRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rc := repairCtx{RequireState: web, origin: wire.RequestOrigin(r, web)}
		if !web {
			rc.investigator = r.Header.Get(wire.InvestigateHeader)
		}
		if web && req.State == "" {
			writeJSON(w, repairRefusal(&repair.RefusalError{Reason: wire.ReasonStale, Message: "a state token is required; review the current pause"}))
			return
		}
		res, err := do(req, rc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, res)
	}
}
