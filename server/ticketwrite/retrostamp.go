package ticketwrite

import (
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// RetroKeyPrefix is how RetroSubmit's dedupe key names the epic a retro covers.
const RetroKeyPrefix = "evolve:"

// RetroSubmit is the one-off a retro of epic runs as: a commitless evolve ticket
// in the epic's own project. The dedupe key refuses a second live retro of the
// same epic; the timestamped name keeps a --force re-run from hitting a taken one.
func RetroSubmit(project, epic string, now time.Time) wire.OneOffRequest {
	return wire.OneOffRequest{
		Prompt:  "Retro of epic " + project + ":" + epic,
		Project: project,
		Name:    "evolve-" + epic + "-" + now.Format("20060102-1504"),
		Type:    string(schema.TypeEvolve),
		Unique:  RetroKeyPrefix + project + "/" + epic,
		Notify:  true,
	}
}

// StampFinished is the one hook every done and cancel path calls once the
// ticket's status is written.
func (w *Writer) StampFinished(addr tickets.Address, ticketPath string, cancelled bool) {
	w.stampFriction(addr, ticketPath)
	if !cancelled {
		w.stampLaunch(addr, ticketPath)
	}
	w.stampRetro(addr, ticketPath, cancelled)
}

// stampLaunch appends the launch of the in-memory run record to the ticket's
// launch stamps, so a reclaimed ticket carries one entry per launch. A run with
// no finished launch timing (not tracked, found finished at reattach, launch
// still open) adds nothing. Finishing the same claim twice replaces its entry.
func (w *Writer) stampLaunch(addr tickets.Address, ticketPath string) {
	run, ok := w.d.Registry.Tracked(addr.String())
	if !ok || run.LaunchStep != "" || run.StartedAt.IsZero() || run.LaunchTimings.Total() == 0 {
		return
	}
	lt := run.LaunchTimings
	stamp := schema.LaunchStamp{
		ClaimedAt:  run.StartedAt.UTC(),
		TotalMs:    lt.Total().Milliseconds(),
		WorktreeMs: lt.Worktree.Milliseconds(),
		DepsMs:     lt.Deps.Milliseconds(),
		AgentMs:    lt.Agent.Milliseconds(),
		PromptMs:   lt.Prompt.Milliseconds(),
	}
	err := schema.UpdateTicket(ticketPath, func(t *schema.Ticket) {
		t.Launch = slices.DeleteFunc(t.Launch, func(l schema.LaunchStamp) bool { return l.ClaimedAt.Equal(stamp.ClaimedAt) })
		t.Launch = append(t.Launch, stamp)
	})
	if err != nil {
		w.d.Log.Warn("stamp launch", "ticket", addr.String(), "err", err)
	}
}

func (w *Writer) stampFriction(addr tickets.Address, ticketPath string) {
	dir, err := w.d.Projects.Dir(addr.Project)
	if err == nil {
		err = ticketops.StampFriction(dir, addr.Epic, addr.ID, ticketPath)
	}
	if err != nil {
		w.d.Log.Warn("stamp friction", "ticket", addr.String(), "err", err)
	}
}

// stampRetro records on the retro'd epic what became of its evolve one-off: the
// server stamps, never the skill. A done retro (zero findings included) sets
// evolve_ticket, overwriting a --force re-run's predecessor; a cancelled one
// sets evolve_skipped so `gx evolve all` stops offering the epic. Both set
// evolved_at. Tickets that are not retros are ignored.
func (w *Writer) stampRetro(addr tickets.Address, ticketPath string, cancelled bool) {
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil || tk.Type != schema.TypeEvolve {
		return
	}
	target, ok := strings.CutPrefix(tk.Unique, RetroKeyPrefix)
	if !ok {
		return
	}
	project, epic, ok := strings.Cut(target, "/")
	if !ok {
		return
	}
	dir, err := w.d.Projects.Dir(project)
	if err != nil {
		w.d.Log.Warn("stamp retro", "ticket", addr.String(), "err", err)
		return
	}
	epicPath := filepath.Join(dir, epic)
	now := time.Now().UTC()
	if cancelled {
		// A cancel after a finished retro keeps the stamp that retro wrote.
		err = tickets.StampEpicFields(epicPath, map[string]any{"evolved_at": now, "evolve_skipped": true})
	} else {
		err = tickets.OverwriteEpicFields(epicPath, map[string]any{"evolved_at": now, "evolve_ticket": addr.String()})
	}
	if err != nil {
		w.d.Log.Warn("stamp retro", "ticket", addr.String(), "err", err)
	}
}
