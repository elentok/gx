package ticketops

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/ticketwrite"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/ticketops"
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

// directLaunch is the Launch of a repair run without a server. It has no
// runner, so only the default deps are answered; the queue-bound methods are
// never reached because a direct run uses the verbs without their queue half.
type directLaunch struct {
	Launch
	projects *projects.Registry
}

func (l directLaunch) HostingDeps(tickets.Address) ralphloop.Deps { return l.projects.DefaultDeps() }

// directBack is the projects.RegistryBack of a repair run without a server.
type directBack struct{}

func (directBack) SessionFinished(agentrunner.Session) bool { return false }
func (directBack) BusyAddresses() []string                  { return nil }

// RunRepairDirect runs a repair verb without a server, for the CLI's
// server-down fallback. The verbs take the merge lock themselves, so a live
// server and a direct run still exclude each other.
func RunRepairDirect(ticketStore, verb string, req wire.RepairRequest) (wire.RepairResult, error) {
	env := func() projects.Env { return projects.Env{TicketStore: ticketStore} }
	set := &settings.Holder{}
	set.Store(&settings.Settings{})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := projects.NewRegistry(env, log, nil, set, nil, directBack{})
	o := New(Deps{TicketStore: ticketStore, Projects: reg, Log: log, Launch: directLaunch{projects: reg}})
	var do func(wire.RepairRequest, repairCtx) (wire.RepairResult, error)
	switch verb {
	case "merge":
		do = o.repairMerge
	case "start-over":
		do = o.repairStartOver
	case "resume", "unpark":
		do = o.repairResume
	case "verify":
		do = o.repairVerify
	default:
		return wire.RepairResult{}, fmt.Errorf("unknown repair verb %q", verb)
	}
	res, err := do(req, repairCtx{})
	res.Via = wire.ViaDirect
	return res, err
}

// ResumeAsWeb is the resume the web mux serves: it needs a state token. For tests.
func (o *Ops) ResumeAsWeb(req wire.RepairRequest) (wire.RepairResult, error) {
	return o.repairResume(req, repairCtx{RequireState: true, origin: wire.OriginWeb})
}

// StartOverAsWeb is the start-over the web mux serves. For tests.
func (o *Ops) StartOverAsWeb(req wire.RepairRequest) (wire.RepairResult, error) {
	return o.repairStartOver(req, repairCtx{RequireState: true, origin: wire.OriginWeb})
}

// ResumeAs is the resume a client of the given origin sends over the socket.
// For tests.
func (o *Ops) ResumeAs(req wire.RepairRequest, origin string) (wire.RepairResult, error) {
	return o.repairResume(req, repairCtx{origin: origin})
}

// ResumeHandler is the resume route's handler, for the retry route that
// prompts a live agent instead of relaunching.
func (o *Ops) ResumeHandler(web bool) http.HandlerFunc { return repairWriteAs(web, o.repairResume) }

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
func (o *Ops) epicTarget(req wire.RepairRequest, needID, preferProject bool) (epicPath, id, cwd string, err error) {
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
	dirs, err := tickets.ProjectDirs(o.d.TicketStore)
	if err != nil {
		return "", "", "", err
	}
	for _, d := range dirs {
		if tickets.ProjectName(d) != project {
			continue
		}
		cwd = req.Cwd
		if cwd == "" || preferProject {
			if _, repo, err := o.d.Projects.Of(project); err == nil {
				cwd = repo
			}
		}
		return filepath.Join(d, epic), id, cwd, nil
	}
	return "", "", "", &repair.RefusalError{Reason: wire.ReasonUnknownTicket, Message: "no project " + project + " in the ticket store"}
}

func (o *Ops) repairMerge(req wire.RepairRequest, rc repairCtx) (wire.RepairResult, error) {
	if repair.IsGuardedBranch(req.Branch) {
		return repairRefusal(&repair.RefusalError{Reason: repair.ReasonRalphLoopCwd, Message: fmt.Sprintf("refusing to merge from a ralph-loop/* working directory (%s); run from outside the iteration worktree", req.Branch)}), nil
	}
	epicPath, id, cwd, err := o.epicTarget(req, true, true)
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
	}, o.d.Launch.HostingDeps(epicTicketAddress(epicPath, id)))
	if err != nil {
		return repairRefusal(err), nil
	}
	return repairOK(res)
}

// mergeInQueue is the server's merge. A ticket of a queued root may be the
// one a merging paused, so once it merges the root completes as if its
// iteration had settled. A direct merge has no queue.
func (o *Ops) mergeInQueue(req wire.RepairRequest, rc repairCtx) (wire.RepairResult, error) {
	addr, err := tickets.ParseAddress(req.Address, tickets.AddressContext{})
	if err != nil {
		return o.repairMerge(req, rc)
	}
	res, err := o.repairMerge(req, rc)
	if err != nil || res.Refused {
		return res, err
	}
	if err := o.cancelInvestigateUnlessSelf(req, rc); err != nil {
		return wire.RepairResult{}, err
	}
	o.d.Launch.CompleteMergedRoot(addr)
	return res, nil
}

// startOverInQueue is the server's start-over: a person's, so it also cancels
// the ticket's open investigate child. A direct start-over has no server to
// stop an agent.
func (o *Ops) startOverInQueue(req wire.RepairRequest, rc repairCtx) (wire.RepairResult, error) {
	res, err := o.repairStartOver(req, rc)
	if err != nil || res.Refused {
		return res, err
	}
	if err := o.cancelInvestigateUnlessSelf(req, rc); err != nil {
		return wire.RepairResult{}, err
	}
	return res, nil
}

// cancelInvestigateUnlessSelf cancels the ticket's investigate child, except
// when that investigate agent is the caller.
func (o *Ops) cancelInvestigateUnlessSelf(req wire.RepairRequest, rc repairCtx) error {
	if rc.investigator != "" {
		return nil
	}
	return o.CancelInvestigateOf(req.Address, rc.origin)
}

func (o *Ops) repairStartOver(req wire.RepairRequest, rc repairCtx) (wire.RepairResult, error) {
	// The web has no --reason flag to name; the CLI refusal text stays in repair.
	if rc.RequireState && strings.TrimSpace(req.Reason) == "" {
		return repairRefusal(&repair.RefusalError{Reason: repair.ReasonReasonRequired, Message: "a reason is required to start over"}), nil
	}
	epicPath, id, cwd, err := o.epicTarget(req, true, false)
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
		res, err := o.d.Landing.StartOverTrunkConflict(req, rc.origin, addr, t)
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
	}, o.d.Launch.HostingDeps(epicTicketAddress(epicPath, id)))
	if err != nil {
		return repairRefusal(err), nil
	}
	return repairOK(res)
}

func epicTicketAddress(epicPath, id string) tickets.Address {
	return tickets.Address{Project: tickets.ProjectName(filepath.Dir(epicPath)), Epic: filepath.Base(epicPath), ID: id}
}

func (o *Ops) repairResume(req wire.RepairRequest, rc repairCtx) (wire.RepairResult, error) {
	epicPath, id, _, err := o.epicTarget(req, true, false)
	if err != nil {
		return repairRefusal(err), nil
	}
	if req.Tier != "" && !agent.Tier(req.Tier).Valid() {
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
		if blockers := o.AmbiguousBlockers(addr); !slices.Contains(blockers, req.Base) {
			msg := fmt.Sprintf("base %q is not one of the competing blockers (%s)", req.Base, strings.Join(blockers, ", "))
			return repairRefusal(&repair.RefusalError{Reason: wire.ReasonBadBase, Message: msg}), nil
		}
	}
	if o.d.Rec.AdoptFirst(dir, addr, t.Path) {
		res := repair.ResumeResult{Ticket: id, Status: repair.ResumeAdopted}
		if zeroCommit {
			res.Message = repair.MessageAdopted
		}
		return repairOK(res)
	}
	if res, refused, err := o.resumeSelfReported(epicPath, id, rc.origin); refused || err != nil {
		return res, err
	}
	if prompt, ok := retryPrompts[tk.PauseReason]; paused && ok {
		prompted, err := o.promptLiveAgent(dir, addr, prompt)
		if err != nil {
			return repairRefusal(err), nil
		}
		if prompted {
			if err := tickets.PromptOpenRound(t.Path, rc.origin); err != nil {
				return wire.RepairResult{}, fmt.Errorf("log prompt of %s: %w", addr, err)
			}
			ev := runlog.Event{Type: string(events.PausePrompted), Ticket: id, Address: addr.String(), Text: prompt}
			if err := runlog.AppendEvent(dir, addr.Epic, ev); err != nil {
				o.d.Log.Warn("logging pause prompt", "ticket", addr.String(), "err", err)
			}
			o.d.Log.Info("prompted the live agent of a paused ticket", "ticket", addr.String(), "origin", rc.origin)
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
		if err := tickets.EndOpenRound(t.Path, rc.origin, tickets.HowResume, ticketops.CloseResumed); err != nil {
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

// TakeTierOverride returns the ticket's one-time tier and clears it, so only
// the launch that reads it runs on it. An unreadable ticket has none.
func TakeTierOverride(ticketPath string) agent.Tier {
	var tier string
	err := schema.UpdateTicketWithBody(ticketPath, func(t *schema.Ticket, _ *string) {
		tier, t.TierOverride = t.TierOverride, ""
	})
	if err != nil || !agent.Tier(tier).Valid() {
		return ""
	}
	return agent.Tier(tier)
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
		if err := ticketops.ResumeTicketWithBaseLocked(ticketPath, time.Now(), req.Base); err != nil {
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

// AmbiguousBlockers lists the blockers a ticket paused on an ambiguous base
// has to choose between, recomputed from the epics; none when it is not ambiguous.
func (o *Ops) AmbiguousBlockers(addr tickets.Address) []string {
	dir, err := o.d.Projects.Dir(addr.Project)
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
			if _, err := tickets.DeriveRootBase(addr.Project, epics, addr.Epic, t, o.d.Projects.MergingOnMerged(addr.Project)); errors.As(err, &amb) {
				return amb.Blockers
			}
		}
	}
	return nil
}

// PromptsOnRetry reports whether a retry of address prompts its live agent
// instead of relaunching.
func (o *Ops) PromptsOnRetry(address string) bool {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return false
	}
	_, t, ok := o.d.Writer.FreshTicket(addr)
	if !ok {
		return false
	}
	tk, err := schema.ParseTicket(t.Path)
	if err != nil || !tk.PausedAsking() {
		return false
	}
	if _, ok := retryPrompts[tk.PauseReason]; !ok {
		return false
	}
	dir, err := o.d.Projects.Dir(addr.Project)
	if err != nil {
		return false
	}
	_, _, live, err := o.iterationSession(dir, addr)
	return err == nil && live
}

// promptLiveAgent sends text to the iteration's agent when it is still
// running, and reports whether it did. A direct run has no runner and finds none.
func (o *Ops) promptLiveAgent(dir string, addr tickets.Address, text string) (bool, error) {
	runner, sess, ok, err := o.iterationSession(dir, addr)
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
func (o *Ops) iterationSession(dir string, addr tickets.Address) (agentrunner.Runner, agentrunner.Session, bool, error) {
	if o.d.Registry == nil {
		return nil, agentrunner.Session{}, false, nil
	}
	label, _, _ := ralphloop.IterationIdentity(addr.Epic, addr.ID, o.d.Projects.WorktreeDir(addr.Project))
	runner := o.d.Harness.RunnerFor(addr.Project)
	if pause, ok := runlog.LatestPauseEvent(dir, addr.Epic, addr.ID); ok {
		if pause.Label != "" {
			label = pause.Label
		}
		if r := o.d.Harness.RunnerForRun(runs.Run{Address: addr.String(), Runner: pause.Runner}); pause.Runner != "" && r != nil {
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
func (o *Ops) resumeSelfReported(epicPath, id, origin string) (wire.RepairResult, bool, error) {
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
	if !selfReported && !o.d.Writer.WaitsOnBlockedPause(tk, address) {
		return wire.RepairResult{}, false, nil
	}
	if o.d.Registry != nil {
		gone, err := o.AgentGone(address)
		if err != nil {
			return wire.RepairResult{}, false, err
		}
		if !gone && !selfReported {
			// A PTY or herdr run has no approvable card for a question menu, so
			// Retry is the way out: end the pause, interrupt, and let the caller
			// send the retry prompt.
			interrupted, err := o.d.Writer.RetryBlocked(address)
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
		o.d.Registry.Delete(address)
	}
	if !selfReported {
		return wire.RepairResult{}, false, nil
	}
	if err := tickets.EndOpenRound(t.Path, origin, tickets.HowResume, ticketops.CloseResumed); err != nil {
		return wire.RepairResult{}, false, fmt.Errorf("close question of %s: %w", address, err)
	}
	return wire.RepairResult{}, false, nil
}

func (o *Ops) repairVerify(req wire.RepairRequest, _ repairCtx) (wire.RepairResult, error) {
	epicPath, id, cwd, err := o.epicTarget(req, false, false)
	if err != nil {
		return repairRefusal(err), nil
	}
	run, err := repair.DefaultVerifyRun(epicPath, cwd, o.d.Launch.HostingDeps(epicTicketAddress(epicPath, id)).Runner)
	if err != nil {
		return repairRefusal(err), nil
	}
	if run.MergeLockStateDir, err = o.d.Projects.LockStateDir(); err != nil {
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
			wire.WriteJSON(w, repairRefusal(&repair.RefusalError{Reason: wire.ReasonStale, Message: "a state token is required; review the current pause"}))
			return
		}
		res, err := do(req, rc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		wire.WriteJSON(w, res)
	}
}
