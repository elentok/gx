package cmdbase

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server/wire"
)

// RunTargets makes a many-target server write and prints it with PrintTargets.
// With no server running every target is refused ReasonServerNotRunning with
// message; any other failure goes through onErr. A nil ctx means Background.
func RunTargets(ctx context.Context, w io.Writer, jsonOut bool, targets []string, message string,
	call func(context.Context) (wire.TargetsResult, error),
	onErr func(error) error,
	one func(io.Writer, wire.TargetResult) error,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := call(ctx)
	switch {
	case apiclient.IsNotRunning(err):
		res = wire.TargetsResult{}
		for _, t := range targets {
			res.Results = append(res.Results, wire.RefusedTarget(t, wire.ReasonServerNotRunning, message))
		}
	case err != nil:
		return onErr(err)
	}
	return PrintTargets(w, jsonOut, res, one)
}

// PrintTargets prints the results of a many-target write and exits 1 when any
// target was refused. With --json the envelope is the only output. One target
// prints what its verb printed before many targets existed (one), and a refusal
// of it is the error; two or more print a line each and a summary.
func PrintTargets(w io.Writer, jsonOut bool, envelope wire.TargetsResult, one func(io.Writer, wire.TargetResult) error) error {
	results := envelope.Results
	refused := 0
	for _, r := range results {
		if r.Refused() {
			refused++
		}
	}
	switch {
	case jsonOut:
		if err := EncodeProvenance(w, envelope, ViaServer, CallerActor(os.Getwd)); err != nil {
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
		return &ExitError{Code: 1}
	}
	return nil
}
