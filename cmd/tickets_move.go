package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
)

// runTicketsMove moves one flat, never-run ticket of the cwd-resolved source
// to another epic of the same project, written directly like `add`. Both epic
// locks are held, in path order, from the checks through the write; the new
// file lands before the old one is deleted so a crash never loses the ticket.
func runTicketsMove(d deps, addr, target, slug string, dryRun, jsonOut bool, w io.Writer) error {
	cwd, err := d.getwd()
	if err != nil {
		return err
	}
	a, src, err := findTicket(cwd, addr)
	if err != nil {
		return err
	}
	srcEpicPath := filepath.Dir(filepath.Dir(src.Path))
	root := filepath.Dir(srcEpicPath)

	dstProject, dstEpic := a.Project, target
	if m := projectEpicRe.FindStringSubmatch(target); m != nil {
		dstProject, dstEpic = m[1], m[2]
	}
	if dstProject != a.Project {
		if _, err := projectRoot(cwd, dstProject); err != nil {
			return err
		}
		return tickets.MoveRefusal(tickets.CodeMoveCrossProject, "target epic %q is in project %q, not %q", dstEpic, dstProject, a.Project)
	}
	if dstEpic == a.Epic {
		return tickets.MoveRefusal(tickets.CodeMoveSameEpic, "ticket %s is already in epic %q", a.ID, a.Epic)
	}
	dstEpicPath := filepath.Join(root, dstEpic)
	if info, err := os.Stat(dstEpicPath); err != nil || !info.IsDir() {
		return tickets.MoveRefusal(tickets.CodeUnknownEpic, "no epic %q in project %q", dstEpic, a.Project)
	}
	if tickets.IsMapEpic(srcEpicPath) {
		return tickets.MoveRefusal(tickets.CodeMoveMapSource, "epic %q is a map; its tickets cannot be moved out", a.Epic)
	}

	unlock, err := tickets.LockEpics(srcEpicPath, dstEpicPath)
	if err != nil {
		return err
	}
	defer unlock()

	if err := refuseIfActive(d.dial(), a); err != nil {
		return err
	}

	plan, err := tickets.PlanMove(root, a, src, dstEpic, slug, tickets.MoveOpts{})
	if err != nil {
		return err
	}
	if dryRun {
		return printMove(w, plan, jsonOut, "dry-run")
	}

	if err := tickets.ApplyMove(plan); err != nil {
		return err
	}
	// The old file is gone, so both sides are pinged by address, not by path.
	pingLabels(d.dial(), plan.From.String(), plan.To.String())
	return printMove(w, plan, jsonOut, "")
}

// refuseIfActive refuses a ticket that is queued or has a live iteration. A
// reachable server answers; otherwise the persisted queue and run files do, and
// if those can't be read either the move is refused rather than guessed.
func refuseIfActive(dial func() (*apiclient.Client, error), a tickets.Address) error {
	active, err := activeAddresses(dial)
	if err != nil {
		return tickets.MoveRefusal(tickets.CodeMoveActive, "cannot tell whether %s is queued or live: %v", a, err)
	}
	if slices.Contains(active, a.String()) {
		return tickets.MoveRefusal(tickets.CodeMoveActive, "ticket %s is queued or live on the server", a)
	}
	return nil
}

func activeAddresses(dial func() (*apiclient.Client, error)) ([]string, error) {
	if cl, err := dial(); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		defer cancel()
		items, qerr := cl.QueueItems(ctx)
		its, ierr := cl.Iterations(ctx)
		if qerr == nil && ierr == nil {
			var out []string
			for _, i := range items {
				out = append(out, i.Address)
			}
			for _, i := range its {
				out = append(out, i.Address)
			}
			return out, nil
		}
	}
	stateDir, err := config.StateDir()
	if err != nil {
		return nil, err
	}
	return server.PersistedActive(stateDir)
}

func printMove(w io.Writer, p tickets.MovePlan, jsonOut bool, mode string) error {
	if jsonOut {
		rewritten := p.Rewritten
		if rewritten == nil {
			rewritten = []string{}
		}
		warnings := p.Warnings
		if warnings == nil {
			warnings = []string{}
		}
		return writeStamped(w, struct {
			From      string   `json:"from"`
			To        string   `json:"to"`
			Path      string   `json:"path"`
			Rewritten []string `json:"rewritten"`
			Warnings  []string `json:"warnings"`
			DryRun    bool     `json:"dry_run,omitempty"`
		}{p.From.String(), p.To.String(), p.NewPath, rewritten, warnings, mode == "dry-run"}, viaDirect, callerActor(os.Getwd))
	}
	if mode != "dry-run" {
		if _, err := fmt.Fprintln(w, p.To.String()); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w, "%s -> %s (%s)\n", p.From, p.To, p.NewPath); err != nil {
			return err
		}
		for _, r := range p.Rewritten {
			if _, err := fmt.Fprintln(w, "  "+r); err != nil {
				return err
			}
		}
	}
	for _, path := range p.Warnings {
		if _, err := fmt.Fprintf(w, "warning: %s still mentions %s\n", path, p.From); err != nil {
			return err
		}
	}
	return nil
}
