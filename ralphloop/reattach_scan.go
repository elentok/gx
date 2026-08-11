package ralphloop

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

// ReattachSignal is a detect-only report that ticket's iteration tab is
// still alive in epicName's herdr workspace after a restart — the same
// live-tab test reconcile's reattach path uses, without reconcile's
// claim/reattach side effects (AgentGet verification, sink events, orphaned-
// claim reversion). Surfacing it is the caller's job (e.g. a UI indicator);
// ScanForReattachable itself never mutates ticket or process state.
type ReattachSignal struct {
	EpicName string
	Ticket   tickets.Ticket
}

// ScanForReattachable reports a ReattachSignal for every claimed or
// needs-repair ticket, across epics, whose epic still has a live herdr
// tab for that ticket's iteration — the lightweight half of reconcile's
// per-ticket live-tab check, factored out so a restart-recovery scan can run
// it across every epic without invoking reconcile's own claim/reattach
// logic (which requires a single epic's already-resolved workspaceID and
// mutates ticket files, AgentGet state, and Sink events). An epic with no
// live herdr workspace (findWorkspace returns "") contributes nothing rather
// than erroring, since "never started" and "already finished and torn down"
// are both ordinary, not failures.
func ScanForReattachable(
	findWorkspace func(label string) (string, error),
	tabList func(workspaceID string) ([]herdr.Tab, error),
	epics []tickets.Epic,
) ([]ReattachSignal, error) {
	var signals []ReattachSignal
	for _, epic := range epics {
		var (
			live    map[string]bool
			checked bool
		)
		for _, t := range epic.Tickets {
			status := strings.ToLower(strings.TrimSpace(t.Status))
			if status != "claimed" && status != "needs-repair" {
				continue
			}
			if !checked {
				checked = true
				workspaceID, err := findWorkspace(epic.Name)
				if err != nil {
					return nil, fmt.Errorf("finding workspace for epic %q: %w", epic.Name, err)
				}
				if workspaceID == "" {
					continue
				}
				tabs, err := tabList(workspaceID)
				if err != nil {
					return nil, fmt.Errorf("listing tabs for epic %q: %w", epic.Name, err)
				}
				live = make(map[string]bool, len(tabs))
				for _, tab := range tabs {
					live[iterationKey(epic.Name, tab.Label)] = true
				}
			}
			if live[iterationKey(epic.Name, iterLabel(epic.Name, t.Identifier))] {
				signals = append(signals, ReattachSignal{EpicName: epic.Name, Ticket: t})
			}
		}
	}
	return signals, nil
}
