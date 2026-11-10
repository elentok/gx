package serverapi

import (
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
)

// printTargets prints the results of a many-target write and exits 1 when any
// target was refused. With --json the envelope is the only output. One target
// prints what its verb printed before many targets existed (one), and a refusal
// of it is the error; two or more print a line each and a summary.
func printTargets(w io.Writer, jsonOut bool, envelope any, results []wire.TargetResult, one func(io.Writer, wire.TargetResult) error) error {
	refused := 0
	for _, r := range results {
		if r.Refused() {
			refused++
		}
	}
	switch {
	case jsonOut:
		if err := cmdbase.EncodeProvenance(w, envelope, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd)); err != nil {
			return err
		}
	case len(results) == 1:
		if r := results[0]; r.Refused() {
			return fmt.Errorf("refused (%s): %s", r.Reason, r.Message)
		}
		return one(w, results[0])
	default:
		for _, r := range results {
			line := fmt.Sprintf("%s %s", r.Status, r.Target)
			if r.Refused() {
				line += ": " + r.Message
			}
			if _, err := fmt.Fprintln(w, line); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "%d done, %d refused\n", len(results)-refused, refused); err != nil {
			return err
		}
	}
	if refused > 0 {
		return &cmdbase.ExitError{Code: 1}
	}
	return nil
}

// notRunning is the single refusal a write gets when no server answers.
func notRunning(target string) wire.TargetResult {
	return wire.RefusedTarget(target, wire.ReasonServerNotRunning, "no server is running; start it with `gx server start`")
}
