package repair

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
)

// Stable machine-readable refusal codes shared by the repair verbs
// (land, verify, reset, unpark). Callers branch on these, never on message text.
const (
	ReasonLandLocked             = "land_locked"
	ReasonIterationBranchMissing = "iteration_branch_missing"
	ReasonLiveAgentOnTab         = "live_agent_on_tab"
	ReasonForkChildren           = "fork_children"
	ReasonNotParked              = "not_parked"
	ReasonStatusRefused          = "status_refused"
	ReasonCommitless             = "commitless"
	ReasonRalphLoopCwd           = "ralph_loop_cwd"
	ReasonLandConflictPending    = "land_conflict_pending"
	ReasonLandBlocked            = "land_blocked"
	ReasonNoPendingLand          = "no_pending_land"
	ReasonLandNotResolved        = "land_not_resolved"
	ReasonReasonRequired         = "reason_required"
	// ReasonError is the fallback for a failure that carries no specific code.
	ReasonError = "error"
)

// RefusalError is returned by a repair verb to refuse with a reason code.
type RefusalError struct {
	Reason  string
	Message string
}

func (e *RefusalError) Error() string { return e.Message }

// FindEpicTicket loads the epic at epicPath and returns it with ticket id.
func FindEpicTicket(epicPath, id string) (tickets.Epic, tickets.Ticket, error) {
	epicPath = filepath.Clean(epicPath)
	epics, err := tickets.Load(filepath.Dir(epicPath))
	if err != nil {
		return tickets.Epic{}, tickets.Ticket{}, fmt.Errorf("loading epics under %s: %w", filepath.Dir(epicPath), err)
	}
	for _, epic := range epics {
		if epic.Name != filepath.Base(epicPath) {
			continue
		}
		for _, t := range epic.Tickets {
			if t.DisplayNumber() == id {
				return epic, t, nil
			}
		}
		return tickets.Epic{}, tickets.Ticket{}, fmt.Errorf("ticket %s not found in epic %s", id, epic.Name)
	}
	return tickets.Epic{}, tickets.Ticket{}, fmt.Errorf("epic not found: %s", epicPath)
}

// EpicTicketLabel is the canonical address of ticket id in the epic at
// epicPath, for callers that know the epic and id but not a ticket path.
func EpicTicketLabel(epicPath, id string) string {
	epicPath = filepath.Clean(epicPath)
	abs, err := filepath.Abs(epicPath)
	if err != nil {
		abs = epicPath
	}
	return tickets.Address{Project: tickets.ProjectName(filepath.Dir(abs)), Epic: filepath.Base(abs), ID: id}.String()
}

// IsRalphLoopBranch reports whether getwd's directory is on a "ralph-loop/*"
// branch: getwd being nil or failing, or the branch not matching, all mean
// "not on a guarded branch" rather than a false positive.
func IsRalphLoopBranch(getwd func() (string, error)) (branch string, ok bool) {
	if getwd == nil {
		return "", false
	}
	cwd, err := getwd()
	if err != nil {
		return "", false
	}
	branch, err = git.CurrentBranch(cwd)
	if err != nil || !strings.HasPrefix(branch, "ralph-loop/") {
		return "", false
	}
	return branch, true
}
