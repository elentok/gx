package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/fsutil"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// Scopes of a Default harness write.
const (
	DefaultHarnessGlobal  = "global"
	DefaultHarnessProject = "project"
)

// Refusal reasons of a Default harness write.
const (
	ReasonBadScope           = "bad-scope"
	ReasonBadHarness         = "bad-harness"
	ReasonHarnessNotOnPath   = "harness-not-on-path"
	ReasonNoGlobalHarnessSet = "global-needs-harness"
)

const defaultHarnessKey = "default-harness"

// ProjectDefaultHarness is one project's override; Harness is empty when the
// project follows the global default.
type ProjectDefaultHarness struct {
	Project string `json:"project"`
	Harness string `json:"harness"`
}

// DefaultHarnessStatus is the Default harness the Settings page shows.
type DefaultHarnessStatus struct {
	Default  string                  `json:"default"`
	Runner   string                  `json:"runner"`
	Projects []ProjectDefaultHarness `json:"projects"`
}

// SetDefaultHarnessRequest writes the Default harness. An empty Harness with
// scope project removes the override.
type SetDefaultHarnessRequest struct {
	Scope   string `json:"scope"`
	Project string `json:"project,omitempty"`
	Harness string `json:"harness"`
}

// SetDefaultHarnessResult says what a write did. A refusal wrote nothing.
type SetDefaultHarnessResult struct {
	Scope   string `json:"scope,omitempty"`
	Project string `json:"project,omitempty"`
	// Harness is the Default harness in effect for the scope after the write.
	Harness string `json:"harness,omitempty"`
	Changed bool   `json:"changed"`
	// Waiting and Running are the notice's counts; Notice is its text, empty
	// when nothing changed.
	Waiting int    `json:"waiting"`
	Running int    `json:"running"`
	Notice  string `json:"notice,omitempty"`
	Path    string `json:"path,omitempty"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

func refuseDefaultHarness(reason, msg string) (SetDefaultHarnessResult, error) {
	return SetDefaultHarnessResult{Refused: true, Reason: reason, Message: msg}, nil
}

func (s *Server) defaultHarnessRead(w http.ResponseWriter, _ *http.Request) {
	st, err := s.defaultHarnessStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, st)
}

func (s *Server) defaultHarnessStatus() (DefaultHarnessStatus, error) {
	projects, err := s.listProjects()
	if err != nil {
		return DefaultHarnessStatus{}, err
	}
	st := DefaultHarnessStatus{
		Default:  string(defaultOrClaude(s.settings.Load().AgentsDefaultHarness)),
		Runner:   s.cfg.AgentRunner,
		Projects: []ProjectDefaultHarness{},
	}
	for _, p := range projects {
		st.Projects = append(st.Projects, ProjectDefaultHarness{Project: p.Name, Harness: s.projectOverride(p.Name)})
	}
	return st, nil
}

// projectOverride is project's own agents.default-harness, "" when it follows
// the global default.
func (s *Server) projectOverride(project string) string {
	dir, err := s.projectDir(project)
	if err != nil {
		return ""
	}
	pf, err := config.ReadProjectFile(dir)
	if err != nil || pf.Agents == nil {
		return ""
	}
	return pf.Agents.DefaultHarness
}

func (s *Server) defaultHarnessWrite(w http.ResponseWriter, r *http.Request) {
	var req SetDefaultHarnessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.setDefaultHarness(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

func (s *Server) setDefaultHarness(req SetDefaultHarnessRequest) (SetDefaultHarnessResult, error) {
	if req.Harness != "" && req.Harness != config.HarnessClaude && req.Harness != config.HarnessCodex {
		return refuseDefaultHarness(ReasonBadHarness, "harness must be "+config.HarnessClaude+" or "+config.HarnessCodex)
	}
	var path, project string
	switch req.Scope {
	case DefaultHarnessGlobal:
		if req.Harness == "" {
			return refuseDefaultHarness(ReasonNoGlobalHarnessSet, "the global default needs a harness")
		}
		if s.cfg.ConfigPath == "" {
			return refuseDefaultHarness(ReasonReloadUnavailable, "this server has no config file to write")
		}
		path = s.cfg.ConfigPath
	case DefaultHarnessProject:
		dir, err := s.projectDir(req.Project)
		if err != nil {
			return refuseDefaultHarness(ReasonUnknownProject, "no project named "+req.Project)
		}
		path, project = filepath.Join(dir, config.ProjectFileName), req.Project
	default:
		return refuseDefaultHarness(ReasonBadScope, "scope must be "+DefaultHarnessGlobal+" or "+DefaultHarnessProject)
	}
	if req.Harness != "" {
		if program, missing := s.missingProgram(req.Harness); missing {
			return refuseDefaultHarness(ReasonHarnessNotOnPath, program+" is not on PATH")
		}
	}

	before := s.effectiveDefault(project)
	if err := setAgentsKey(path, defaultHarnessKey, req.Harness); err != nil {
		return SetDefaultHarnessResult{}, err
	}
	if req.Scope == DefaultHarnessGlobal {
		if rr := s.reloadConfig(reloadTriggerCommand); rr.Refused {
			return refuseDefaultHarness(rr.Reason, rr.Message)
		}
	}
	after := s.effectiveDefault(project)

	res := SetDefaultHarnessResult{Scope: req.Scope, Project: project, Harness: string(after), Path: path}
	if before == after {
		return res, nil
	}
	res.Changed = true
	res.Waiting, res.Running = s.switchCounts(project, after)
	res.Notice = defaultHarnessNotice(project, after, res.Waiting, res.Running)
	return res, nil
}

// effectiveDefault is the Default harness in effect: project's, or the global
// one when project is empty.
func (s *Server) effectiveDefault(project string) ralphloop.AgentKind {
	if project == "" {
		return defaultOrClaude(s.settings.Load().AgentsDefaultHarness)
	}
	return s.defaultHarness(project)
}

// setAgentsKey sets agents.<key> in the JSON file at path, or removes it (and
// the agents object once empty) when value is empty. It edits raw JSON so every
// other key survives, and writes through a symlink to its target. A missing
// file is created.
func setAgentsKey(path, key, value string) error {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	top := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if value == "" {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(data, &top); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	agents := map[string]json.RawMessage{}
	if raw, ok := top["agents"]; ok {
		if err := json.Unmarshal(raw, &agents); err != nil {
			return fmt.Errorf("%s: agents: %w", path, err)
		}
	}
	if value == "" {
		delete(agents, key)
	} else {
		agents[key], _ = json.Marshal(value)
	}
	if len(agents) == 0 {
		delete(top, "agents")
	} else {
		top["agents"], _ = json.Marshal(agents)
	}
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, append(out, '\n'))
}

// switchCounts counts, in scope, the waiting tickets that will use the new
// Default and the running agents that keep another harness. An empty project is
// the global scope: only projects that inherit count.
func (s *Server) switchCounts(project string, now ralphloop.AgentKind) (waiting, running int) {
	inScope := func(p string) bool {
		if project != "" {
			return p == project
		}
		return s.projectOverride(p) == ""
	}
	seen := map[rootRef]bool{}
	for _, item := range s.queued.list() {
		addr, err := parseQueuedAddress(item.Address)
		if err != nil || !inScope(addr.Project) || seen[rootOf(addr)] {
			continue
		}
		seen[rootOf(addr)] = true
		waiting += s.waitingOnDefault(addr)
	}
	for _, t := range s.registry.list() {
		if a, err := tickets.ParseAddress(t.Address, tickets.AddressContext{}); err == nil && inScope(a.Project) && t.Agent != string(now) {
			running++
		}
	}
	sessions, _ := s.listSessions()
	for _, si := range sessions {
		if inScope(si.Project) && si.Status.State != SessionExited && si.Harness != string(now) {
			running++
		}
	}
	return waiting, running
}

// waitingOnDefault counts the open or blocked tickets of epic's root whose
// harness comes from the Default: no pin and never ran.
func (s *Server) waitingOnDefault(epic tickets.Address) int {
	epics, err := s.projectEpics(epic.Project)
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
			if s.resolveTicket(addr, t).Source == HarnessFromDefault {
				n++
			}
		}
	}
	return n
}

// defaultHarnessNotice is the one sentence web, TUI and CLI show after a switch.
func defaultHarnessNotice(project string, now ralphloop.AgentKind, waiting, running int) string {
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
		other := ralphloop.AgentClaude
		if now == ralphloop.AgentClaude {
			other = ralphloop.AgentCodex
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
