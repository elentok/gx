package harness

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/settingsapi"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func refuseDefaultHarness(reason, msg string) (wire.SetDefaultHarnessResult, error) {
	return wire.SetDefaultHarnessResult{Refused: true, Reason: reason, Message: msg}, nil
}

// Status is the Default harness, the configured runner and every project's own
// override.
func (h *Harness) Status() (wire.DefaultHarnessStatus, error) {
	projects, err := h.idx.ListProjects()
	if err != nil {
		return wire.DefaultHarnessStatus{}, err
	}
	st := wire.DefaultHarnessStatus{
		Default:  string(DefaultOrClaude(h.settings.Load().AgentsDefaultHarness)),
		Runner:   h.cfg.AgentRunner,
		Projects: []wire.ProjectDefaultHarness{},
	}
	for _, p := range projects {
		st.Projects = append(st.Projects, wire.ProjectDefaultHarness{Project: p.Name, Harness: h.projectOverride(p.Name)})
	}
	return st, nil
}

// projectOverride is project's own agents.default-harness, "" when it follows
// the global default.
func (h *Harness) projectOverride(project string) string {
	dir, err := h.projects.Dir(project)
	if err != nil {
		return ""
	}
	pf, err := config.ReadProjectFile(dir)
	if err != nil || pf.Agents == nil {
		return ""
	}
	return pf.Agents.DefaultHarness
}

// SetDefault writes the Default harness of a scope and reports what the switch
// changes.
func (h *Harness) SetDefault(req wire.SetDefaultHarnessRequest) (wire.SetDefaultHarnessResult, error) {
	if req.Harness != "" && req.Harness != config.HarnessClaude && req.Harness != config.HarnessCodex {
		return refuseDefaultHarness(wire.ReasonBadHarness, "harness must be "+config.HarnessClaude+" or "+config.HarnessCodex)
	}
	var path, project string
	switch req.Scope {
	case wire.DefaultHarnessGlobal:
		if req.Harness == "" {
			return refuseDefaultHarness(wire.ReasonNoGlobalHarnessSet, "the global default needs a harness")
		}
		if h.cfg.ConfigPath == "" {
			return refuseDefaultHarness(wire.ReasonReloadUnavailable, "this server has no config file to write")
		}
		path = h.cfg.ConfigPath
	case wire.DefaultHarnessProject:
		dir, err := h.projects.Dir(req.Project)
		if err != nil {
			return refuseDefaultHarness(wire.ReasonUnknownProject, "no project named "+req.Project)
		}
		path, project = filepath.Join(dir, config.ProjectFileName), req.Project
	default:
		return refuseDefaultHarness(wire.ReasonBadScope, "scope must be "+wire.DefaultHarnessGlobal+" or "+wire.DefaultHarnessProject)
	}

	change := settingsapi.Change{Key: defaultHarnessPath, Remove: req.Harness == ""}
	if req.Harness != "" {
		change.Value, _ = json.Marshal(req.Harness)
	}
	if _, err := h.writer.Write(settingsapi.Target{Project: project}, []settingsapi.Change{change}); err != nil {
		if ref, ok := errors.AsType[*settingsapi.Refusal](err); ok {
			return refuseDefaultHarness(wire.ReasonHarnessNotOnPath, ref.Reason)
		}
		return wire.SetDefaultHarnessResult{}, err
	}
	out := h.hook.outcome()
	if out.reloadRefused {
		return refuseDefaultHarness(out.reloadReason, out.reloadMsg)
	}

	res := wire.SetDefaultHarnessResult{Scope: req.Scope, Project: project, Harness: string(out.after), Path: path}
	if out.before == out.after {
		return res, nil
	}
	res.Changed, res.Waiting, res.Running, res.Notice = true, out.waiting, out.running, out.notice
	return res, nil
}

// effectiveDefault is the Default harness in effect: project's, or the global
// one when project is empty.
func (h *Harness) effectiveDefault(project string) agent.AgentKind {
	if project == "" {
		return DefaultOrClaude(h.settings.Load().AgentsDefaultHarness)
	}
	return h.DefaultHarness(project)
}

// switchCounts counts, in scope, the waiting tickets that will use the new
// Default and the running agents that keep another harness. An empty project is
// the global scope: only projects that inherit count.
func (h *Harness) switchCounts(project string, now agent.AgentKind) (waiting, running int) {
	inScope := func(p string) bool {
		if project != "" {
			return p == project
		}
		return h.projectOverride(p) == ""
	}
	seen := map[queue.RootRef]bool{}
	for _, item := range h.queue.List() {
		addr, err := tickets.ParseQueuedAddress(item.Address)
		if err != nil || !inScope(addr.Project) || seen[queue.RootOf(addr)] {
			continue
		}
		seen[queue.RootOf(addr)] = true
		waiting += h.waitingOnDefault(addr)
	}
	for _, t := range h.registry.List() {
		if a, err := tickets.ParseAddress(t.Address, tickets.AddressContext{}); err == nil && inScope(a.Project) && t.Agent != string(now) {
			running++
		}
	}
	sessions, _ := h.host.ListSessions()
	for _, si := range sessions {
		if inScope(si.Project) && si.Status.State != wire.SessionExited && si.Harness != string(now) {
			running++
		}
	}
	return waiting, running
}

// waitingOnDefault counts the open or blocked tickets of epic's root whose
// harness comes from the Default: no pin and never ran.
func (h *Harness) waitingOnDefault(epic tickets.Address) int {
	epics, err := h.projects.Epics(epic.Project)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range epics {
		if filepath.Base(e.Path) != epic.Epic {
			continue
		}
		for _, t := range e.Tickets {
			// Blocked is a rendered state of an open ticket, not a stored status.
			if t.Status != string(schema.StatusOpen) {
				continue
			}
			addr := tickets.Address{Project: epic.Project, Epic: epic.Epic, ID: t.Identifier}
			if h.ResolveTicket(addr, t).Source == HarnessFromDefault {
				n++
			}
		}
	}
	return n
}

// defaultHarnessNotice is the one sentence web, TUI and CLI show after a switch.
func defaultHarnessNotice(project string, now agent.AgentKind, waiting, running int) string {
	head := "Default is now " + harnessDisplayName(now) + "."
	if project != "" {
		head = "Default for " + project + " is now " + harnessDisplayName(now) + "."
	}
	use := "The next design step will use it."
	if waiting > 0 {
		use = fmt.Sprintf("%d waiting %s and the next design step will use it.", waiting, plural(waiting, "ticket", "tickets"))
	}
	notice := head + " " + use
	if running > 0 {
		other := agent.AgentClaude
		if now == agent.AgentClaude {
			other = agent.AgentCodex
		}
		notice += fmt.Sprintf(" %d running %s %s %s.", running, plural(running, "agent", "agents"), plural(running, "keeps", "keep"), harnessDisplayName(other))
	}
	return notice
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
