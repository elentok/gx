package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/elentok/gx/repair"
)

// Refusal codes and error type live in the shared repair package.
const (
	ReasonLandLocked             = repair.ReasonLandLocked
	ReasonIterationBranchMissing = repair.ReasonIterationBranchMissing
	ReasonLiveAgentOnTab         = repair.ReasonLiveAgentOnTab
	ReasonForkChildren           = repair.ReasonForkChildren
	ReasonNotParked              = repair.ReasonNotParked
	ReasonStatusRefused          = repair.ReasonStatusRefused
	ReasonCommitless             = repair.ReasonCommitless
	ReasonRalphLoopCwd           = repair.ReasonRalphLoopCwd
	ReasonLandConflictPending    = repair.ReasonLandConflictPending
	ReasonLandBlocked            = repair.ReasonLandBlocked
	ReasonNoPendingLand          = repair.ReasonNoPendingLand
	ReasonLandNotResolved        = repair.ReasonLandNotResolved
	ReasonReasonRequired         = repair.ReasonReasonRequired
	ReasonError                  = repair.ReasonError
)

// RefusalEnvelope is the JSON written on stdout when a recovery command refuses or fails.
type RefusalEnvelope struct {
	Refused bool   `json:"refused"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// RefusalError is returned by a recovery command's run* function to refuse with a reason code.
type RefusalError = repair.RefusalError

// finishRecovery applies the shared recovery-command contract to a run* outcome.
// Success: exit 0; --json prints result, human mode prints humanText.
// Failure: exit 1 (*ExitError); --json prints the refusal envelope on stdout,
// human mode prints the message on stderr. Both JSON shapes carry via/actor.
func finishRecovery(stdout, stderr io.Writer, jsonMode bool, result any, humanText string, runErr error) error {
	if runErr == nil {
		if jsonMode {
			return writeStamped(stdout, result, viaDirect, actorRecovery)
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
		if err := writeStamped(stdout, RefusalEnvelope{Refused: true, Reason: reason, Message: runErr.Error()}, viaDirect, actorRecovery); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(stderr, runErr.Error())
	}
	return &ExitError{Code: 1}
}

func writeStamped(w io.Writer, v any, via, actor string) error {
	stamped, err := stampProvenance(v, via, actor)
	if err != nil {
		return err
	}
	return writeJSON(w, stamped)
}

func writeJSON(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(v)
}
