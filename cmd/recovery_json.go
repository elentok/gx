package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Stable machine-readable refusal codes shared by the recovery commands
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

// RefusalEnvelope is the JSON written on stdout when a recovery command refuses or fails.
type RefusalEnvelope struct {
	Refused bool   `json:"refused"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// RefusalError is returned by a recovery command's run* function to refuse with a reason code.
type RefusalError struct {
	Reason  string
	Message string
}

func (e *RefusalError) Error() string { return e.Message }

// finishRecovery applies the shared recovery-command contract to a run* outcome.
// Success: exit 0; --json prints result, human mode prints humanText.
// Failure: exit 1 (*ExitError); --json prints the refusal envelope on stdout,
// human mode prints the message on stderr.
func finishRecovery(stdout, stderr io.Writer, jsonMode bool, result any, humanText string, runErr error) error {
	if runErr == nil {
		if jsonMode {
			return writeJSON(stdout, result)
		}
		if humanText != "" {
			fmt.Fprintln(stdout, humanText)
		}
		return nil
	}

	reason := ReasonError
	var refusal *RefusalError
	if errors.As(runErr, &refusal) {
		reason = refusal.Reason
	}
	if jsonMode {
		if err := writeJSON(stdout, RefusalEnvelope{Refused: true, Reason: reason, Message: runErr.Error()}); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(stderr, runErr.Error())
	}
	return &ExitError{Code: 1}
}

func writeJSON(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(v)
}
