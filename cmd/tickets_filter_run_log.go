package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/elentok/gx/ralphloop"
)

// runTicketsFilterRunLog reads epicPath's run-log.jsonl (via
// ralphloop.ReadEvents, the same decoding the scheduler itself uses) and
// prints only the events matching ticketFilter/eventFilter as pretty-printed
// JSON blocks, in file order. Both filters are optional and AND together;
// with neither, every event is printed. epicPath must already be a resolved
// epic directory (see resolveEpicArg) — a missing epic directory is an
// error, but a missing run-log.jsonl (an epic that hasn't started yet) is
// not: it prints a short message and returns nil so callers see exit 0.
func runTicketsFilterRunLog(epicPath, ticketFilter string, eventFilter []string, w io.Writer) error {
	epicPath = filepath.Clean(epicPath)

	info, err := os.Stat(epicPath)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("epic not found: %s", epicPath)
	}

	scratchDir := filepath.Dir(epicPath)
	epicName := filepath.Base(epicPath)

	events, ok, err := ralphloop.ReadEvents(scratchDir, epicName)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintf(w, "%s: no run-log.jsonl yet\n", epicPath)
		return nil
	}

	wantEvents := make(map[string]bool, len(eventFilter))
	for _, t := range eventFilter {
		wantEvents[t] = true
	}

	for _, ev := range events {
		if ticketFilter != "" && ev.Ticket != ticketFilter {
			continue
		}
		if len(wantEvents) > 0 && !wantEvents[ev.Type] {
			continue
		}
		data, err := json.MarshalIndent(ev, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(w, string(data))
	}
	return nil
}
