package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/tickets"
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
// server-down fallback. The verbs take the land lock themselves, so a live
// server and a direct run still exclude each other.
func RunRepairDirect(ticketStore, verb string, req RepairRequest) (RepairResult, error) {
	s := &Server{cfg: Config{TicketStore: ticketStore}}
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
// in: the caller's, else the project's repo.
func (s *Server) epicTarget(req RepairRequest, needID bool) (epicPath, id, cwd string, err error) {
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
		if cwd == "" {
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
	epicPath, id, cwd, err := s.epicTarget(req, true)
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

func (s *Server) repairReset(req RepairRequest) (RepairResult, error) {
	epicPath, id, cwd, err := s.epicTarget(req, true)
	if err != nil {
		return repairRefusal(err), nil
	}
	res, err := repair.Reset(repair.ResetInput{
		EpicPath: epicPath, ID: id, Reason: req.Reason, Force: req.Force,
		DeleteBranch: req.DeleteBranch, Cwd: cwd, Now: time.Now(), Subjects: repair.GitCommitSubjects,
	}, ralphloop.DefaultDeps())
	if err != nil {
		return repairRefusal(err), nil
	}
	return repairOK(res)
}

func (s *Server) repairUnpark(req RepairRequest) (RepairResult, error) {
	epicPath, id, _, err := s.epicTarget(req, true)
	if err != nil {
		return repairRefusal(err), nil
	}
	res, err := repair.Unpark(epicPath, id, time.Now())
	if err != nil {
		return repairRefusal(err), nil
	}
	return repairOK(res)
}

func (s *Server) repairVerify(req RepairRequest) (RepairResult, error) {
	epicPath, id, cwd, err := s.epicTarget(req, false)
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
	return func(w http.ResponseWriter, r *http.Request) {
		var req RepairRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
