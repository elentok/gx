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

	// land
	From          string `json:"from,omitempty"`
	To            string `json:"to,omitempty"`
	IgnoreLiveTab bool   `json:"ignore_live_tab,omitempty"`
	Continue      bool   `json:"continue,omitempty"`
	Abort         bool   `json:"abort,omitempty"`

	// reset
	Reason       string `json:"reason,omitempty"`
	Force        bool   `json:"force,omitempty"`
	DeleteBranch bool   `json:"delete_branch,omitempty"`

	// unpark
	Base  string `json:"base,omitempty"`
	State string `json:"state,omitempty"`
	// RequireState is set by the web mux, never by a client: the unpark then
	// needs a State token that still matches the park round.
	RequireState bool `json:"-"`
}

// ReasonBadBase refuses an unpark whose base is not one of the competing blockers.
const ReasonBadBase = "bad-base"

// zeroCommitPrompt is what a live agent receives when a person unparks its
// zero-commit finish.
const zeroCommitPrompt = "A person reviewed your finish with no commits and wants you to continue. " +
	"Finish the work, or report `--iteration-status finished --commitless true`."

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
// server-down fallback. The verbs take the land lock themselves, so a live
// server and a direct run still exclude each other.
func RunRepairDirect(ticketStore, verb string, req RepairRequest) (RepairResult, error) {
	s := &Server{cfg: Config{TicketStore: ticketStore}}
	s.settings.Store(&Settings{})
	var do func(RepairRequest) (RepairResult, error)
	switch verb {
	case "land":
		do = s.repairLand
	case "reset":
		do = s.repairReset
	case "unpark":
		do = s.repairUnpark
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

func (s *Server) repairLand(req RepairRequest) (RepairResult, error) {
	if repair.IsGuardedBranch(req.Branch) {
		return repairRefusal(&repair.RefusalError{Reason: repair.ReasonRalphLoopCwd, Message: fmt.Sprintf("refusing to land from a ralph-loop/* working directory (%s); run from outside the iteration worktree", req.Branch)}), nil
	}
	epicPath, id, cwd, err := s.epicTarget(req, true, true)
	if err != nil {
		return repairRefusal(err), nil
	}
	res, _, err := repair.Land(repair.LandInput{
		EpicPath: epicPath, ID: id, From: req.From, To: req.To,
		IgnoreLiveTab: req.IgnoreLiveTab, Continue: req.Continue, Abort: req.Abort, Cwd: cwd,
	}, ralphloop.DefaultDeps())
	if err != nil {
		return repairRefusal(err), nil
	}
	return repairOK(res)
}

// landInQueue is the server's land. A ticket of a queued root may be the
// one a landing parked, so once it lands the root completes as if its
// iteration had settled. A direct land has no queue.
func (s *Server) landInQueue(req RepairRequest) (RepairResult, error) {
	addr, err := tickets.ParseAddress(req.Address, tickets.AddressContext{})
	if err != nil || !s.rootQueued(rootOf(addr)) {
		return s.repairLand(req)
	}
	res, err := s.repairLand(req)
	if err != nil || res.Refused {
		return res, err
	}
	if _, t, ok := s.freshTicket(addr); ok && s.lands.begin() {
		defer s.lands.end()
		s.completeRootIfDone(rootOf(addr), ralphloop.OneIteration{Epic: addr.Epic, Ticket: t})
	}
	return res, nil
}

func (s *Server) rootQueued(root rootRef) bool {
	for _, it := range s.queued.list() {
		if a, err := tickets.ParseAddress(it.Address, tickets.AddressContext{}); err == nil && rootOf(a) == root {
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

func (s *Server) repairReset(req RepairRequest) (RepairResult, error) {
	epicPath, id, cwd, err := s.epicTarget(req, true, false)
	if err != nil {
		return repairRefusal(err), nil
	}
	res, err := repair.Reset(repair.ResetInput{
		EpicPath: epicPath, ID: id, Reason: req.Reason, Force: req.Force,
		DeleteBranch: req.DeleteBranch, Cwd: cwd, Now: time.Now(), Subjects: repair.GitCommitSubjects,
	}, s.resetDeps(req.Address))
	if err != nil {
		return repairRefusal(err), nil
	}
	return repairOK(res)
}

// resetDeps talks to the runner hosting address's iteration, so reset sees
// and closes the agent where it actually runs. A direct reset has no server
// runner and keeps the default.
func (s *Server) resetDeps(address string) ralphloop.Deps {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if s.cfg.Runner == nil || err != nil {
		return ralphloop.DefaultDeps()
	}
	// An unavailable runner is nil, which reports no session, as for an
	// unreachable host.
	runner, _ := s.iterationRunner(addr)
	return s.depsWith(runner)
}

func (s *Server) repairUnpark(req RepairRequest) (RepairResult, error) {
	epicPath, id, _, err := s.epicTarget(req, true, false)
	if err != nil {
		return repairRefusal(err), nil
	}
	dir := filepath.Dir(epicPath)
	addr := tickets.Address{Project: tickets.ProjectName(dir), Epic: filepath.Base(epicPath), ID: id}
	_, t, ferr := repair.FindEpicTicket(epicPath, id)
	if ferr != nil {
		// Not a ticket of this epic: the plain unpark reports why.
		res, err := repair.Unpark(epicPath, id, time.Now(), "")
		if err != nil {
			return repairRefusal(err), nil
		}
		return repairOK(res)
	}
	tk, perr := schema.ParseTicket(t.Path)
	parked := perr == nil && tk.Status == schema.StatusNeedsAnswer
	zeroCommit := parked && tk.ParkKind == schema.ParkKindZeroCommit
	// Every refusal runs before the first state change.
	if parked && req.RequireState {
		if res, refused := unparkStale(t.Path, tk, req.State); refused {
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
		res := repair.UnparkResult{Ticket: id, Status: repair.UnparkAdopted}
		if zeroCommit {
			res.Message = repair.MessageAdopted
		}
		return repairOK(res)
	}
	if res, refused, err := s.unparkSelfReported(epicPath, id); refused || err != nil {
		return res, err
	}
	if zeroCommit {
		prompted, err := s.promptLiveAgent(dir, addr, zeroCommitPrompt)
		if err != nil {
			return repairRefusal(err), nil
		}
		if prompted {
			return repairOK(repair.UnparkResult{Ticket: id, Status: repair.UnparkPrompted, Message: repair.MessagePrompted})
		}
	}
	if parked && req.RequireState && tk.ParkKind != schema.ParkKindSelfReported {
		if err := logUnparkAnswer(t.Path, req.Base); err != nil {
			return RepairResult{}, fmt.Errorf("log unpark of %s: %w", addr, err)
		}
	}
	res, err := repair.Unpark(epicPath, id, time.Now(), req.Base)
	if err != nil {
		return repairRefusal(err), nil
	}
	if zeroCommit {
		res.Message = repair.MessageReopened
	}
	return repairOK(res)
}

// unparkStale refuses an unpark whose token no longer names the ticket's open
// question and status; a park without an open question has no round to name.
func unparkStale(ticketPath string, t schema.Ticket, state string) (RepairResult, bool) {
	evs, err := tickets.ReadLog(ticketPath)
	if err == nil {
		if open := tickets.OpenQuestion(evs); open != nil && answerState(*open, t.Status) == state {
			return RepairResult{}, false
		}
	}
	return repairRefusal(&repair.RefusalError{Reason: ReasonStale, Message: "the park changed; review the current one"}), true
}

// logUnparkAnswer records the person's unpark as the answer to the open
// question. A ticket without an open question logs nothing.
func logUnparkAnswer(ticketPath, base string) error {
	evs, err := tickets.ReadLog(ticketPath)
	if err != nil {
		return err
	}
	open := tickets.OpenQuestion(evs)
	if open == nil {
		return nil
	}
	_, err = tickets.AppendLogIf(ticketPath, tickets.LogEvent{
		Type: tickets.EventParkAnswer, Source: "web", Question: open.Seq, Choice: "unpark", Base: base,
	}, func(evs []tickets.LogEvent) error {
		if cur := tickets.OpenQuestion(evs); cur == nil || cur.Seq != open.Seq {
			return errUnparkQuestionGone
		}
		return nil
	})
	if errors.Is(err, errUnparkQuestionGone) {
		return nil
	}
	return err
}

var errUnparkQuestionGone = errors.New("question no longer open")

// ambiguousBlockers lists the blockers a ticket parked on an ambiguous base
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
			if _, err := tickets.DeriveRootBase(addr.Project, epics, addr.Epic, t, s.landingOnLanded(addr.Project)); errors.As(err, &amb) {
				return amb.Blockers
			}
		}
	}
	return nil
}

// promptLiveAgent sends text to the iteration's agent when it is still
// running, and reports whether it did. A direct run has no runner and finds none.
func (s *Server) promptLiveAgent(dir string, addr tickets.Address, text string) (bool, error) {
	if s.registry == nil {
		return false, nil
	}
	label, _, _ := ralphloop.IterationIdentity(addr.Epic, addr.ID, s.worktreeDir(addr.Project))
	runner := s.runnerFor(addr.Project)
	if park, ok := ralphloop.LatestParkEvent(dir, addr.Epic, addr.ID); ok {
		if park.Label != "" {
			label = park.Label
		}
		if r := s.runnerForRun(Run{Address: addr.String(), Runner: park.Runner}); park.Runner != "" && r != nil {
			runner = r
		}
	}
	if runner == nil {
		return false, nil
	}
	sess, ok, err := runner.Find(label)
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

// unparkSelfReported applies the rule for a self-reported park: a live waiting
// agent keeps its question, so unpark refuses and names the answer verb; a gone
// agent's question is closed before the status moves on. Other tickets and kinds
// pass through untouched. A direct run has no runner to ask and reads every
// agent as gone, since no server is running to wait on an answer.
func (s *Server) unparkSelfReported(epicPath, id string) (RepairResult, bool, error) {
	_, t, err := repair.FindEpicTicket(epicPath, id)
	if err != nil {
		return RepairResult{}, false, nil
	}
	tk, err := schema.ParseTicket(t.Path)
	if err != nil || tk.Status != schema.StatusNeedsAnswer || tk.ParkKind != schema.ParkKindSelfReported {
		return RepairResult{}, false, nil
	}
	dir := filepath.Dir(epicPath)
	address := tickets.Address{Project: tickets.ProjectName(dir), Epic: filepath.Base(epicPath), ID: id}.String()
	if s.registry != nil {
		gone, err := s.agentGone(address)
		if err != nil {
			return RepairResult{}, false, err
		}
		if !gone {
			msg := fmt.Sprintf("%s has a live agent waiting on its question; reply with `gx tickets answer`", address)
			return repairRefusal(&repair.RefusalError{Reason: ReasonAgentWaiting, Message: msg}), true, nil
		}
		s.registry.delete(address)
	}
	if err := ralphloop.CloseOpenQuestion(t.Path, ralphloop.CloseUnparked); err != nil {
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
		if web && req.State == "" {
			writeJSON(w, repairRefusal(&repair.RefusalError{Reason: ReasonStale, Message: "a state token is required; review the current park"}))
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
