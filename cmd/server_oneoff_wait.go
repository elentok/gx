package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets/schema"
)

// waitExitCodes maps the statuses a --wait stops on to their exit code.
var waitExitCodes = map[string]int{
	string(schema.StatusDone):        0,
	string(schema.StatusNeedsAnswer): 3,
	string(schema.StatusNeedsRepair): 4,
	string(schema.StatusCancelled):   5,
}

// exitWaitTimeout is the exit code when --timeout expires before the ticket ends.
const exitWaitTimeout = 6

var waitRetry = time.Second

// eventSource is the slice of apiclient.Client a wait needs.
type eventSource interface {
	Snapshot(ctx context.Context) (server.Snapshot, error)
	Events(ctx context.Context, since uint64) (<-chan server.Event, error)
}

// waitForTicket blocks until address is done, parked or cancelled. A dropped
// stream or a dead server is not an end: it re-snapshots after a pause, so a
// restart mid-wait only delays the answer. Events carry no status, so any
// event for the ticket just triggers a fresh snapshot.
func waitForTicket(ctx context.Context, src eventSource, address string) (server.TicketInfo, error) {
	for {
		if t, ok, err := waitOnce(ctx, src, address); err != nil && ctx.Err() == nil {
			select {
			case <-time.After(waitRetry):
			case <-ctx.Done():
			}
		} else if ok {
			return t, nil
		}
		if ctx.Err() != nil {
			return server.TicketInfo{}, ctx.Err()
		}
	}
}

// waitOnce takes one snapshot and streams from it until the ticket changes.
func waitOnce(ctx context.Context, src eventSource, address string) (server.TicketInfo, bool, error) {
	snap, err := src.Snapshot(ctx)
	if err != nil {
		return server.TicketInfo{}, false, err
	}
	for _, t := range snap.Tickets {
		if t.Address != address {
			continue
		}
		if _, stop := waitExitCodes[t.Status]; stop {
			return t, true, nil
		}
	}
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	evs, err := src.Events(sctx, snap.Seq)
	if err != nil {
		return server.TicketInfo{}, false, err
	}
	for ev := range evs {
		if ev.Address == address {
			return server.TicketInfo{}, false, nil
		}
	}
	return server.TicketInfo{}, false, nil
}

// runOneOffWait waits for address, prints its ## Result, and maps its status to
// the exit code. A parked ticket has no result worth printing. A positive
// timeout only stops this waiter: the server is never told, so the ticket runs on.
func runOneOffWait(ctx context.Context, src eventSource, w, errW io.Writer, address string, timeout time.Duration) error {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	t, err := waitForTicket(ctx, src, address)
	if errors.Is(err, context.DeadlineExceeded) {
		fmt.Fprintf(errW, "%s: still running after %s; it keeps running\n", address, timeout)
		return &ExitError{Code: exitWaitTimeout}
	}
	if err != nil {
		return err
	}
	code := waitExitCodes[t.Status]
	if code == 0 {
		if res := readResultSection(t.File); res != "" {
			fmt.Fprintln(w, res)
		}
		return nil
	}
	fmt.Fprintf(errW, "%s: %s\n", address, t.Status)
	return &ExitError{Code: code}
}

// readResultSection returns the trimmed content of the ticket's "## Result"
// section, or "" when the file or section is missing.
func readResultSection(file string) string {
	raw, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	var out []string
	in, fence := false, false
	for _, line := range strings.Split(schema.ParseBody(string(raw)), "\n") {
		if strings.HasPrefix(line, "```") {
			fence = !fence
		}
		if !fence && strings.HasPrefix(line, "## ") {
			in = strings.TrimSpace(line[3:]) == "Result"
			continue
		}
		if in {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
