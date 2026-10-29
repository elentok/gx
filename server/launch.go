package server

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// launchSkill is the skill an agent starts under: code-review tickets get their
// own, since gx-implement would ask them for commits, prompt tickets (the
// one-offs) get one that keeps the TDD rules out of the way, and investigate
// and diagnose tickets run gx-investigate's unattended mode.
func launchSkill(t tickets.Ticket) string {
	switch {
	case t.IsCodeReview():
		return codeReviewSkill
	case t.IsVerify():
		return verifySkill
	case t.Type == string(schema.TypePrompt), t.Type == string(schema.TypeTrunkConflict):
		return oneOffSkill
	case t.Type == string(schema.TypeEvolve):
		return evolveSkill
	case t.Type == string(schema.TypeInvestigate), t.Type == string(schema.TypeDiagnose):
		return investigateSkill
	case t.Type == string(schema.TypeMerge):
		return mergeSkill
	}
	return implementSkill
}

// trunkConflictNote tells a trunk-conflict agent what the one-off skill does
// not: it works in the target's own checkout, where a commit would be the
// person's to make.
const trunkConflictNote = "This is a trunk-conflict ticket. You are in the checkout of the merge target. " +
	"Resolve the conflict markers in the files the ticket lists, keeping both sides' intent. " +
	"Leave the files uncommitted: do not commit, stash, reset or switch branches."

// launchNote is what the agent is told on top of the skill command.
func launchNote(t tickets.Ticket) string {
	if t.Type == string(schema.TypeTrunkConflict) {
		return trunkConflictNote
	}
	return investigatePrompt(t)
}

func launchPrompt(agent ralphloop.AgentKind, skill, note, address string) string {
	prompt := ralphloop.SkillPrompt(agent, skill, address)
	if note != "" {
		prompt += "\n\n" + note
	}
	return prompt
}

// evolveDirs are the dirs a retro's `gx tickets` verbs write to outside the
// ticket's own epic: the follow-ups epic and the gx project's ticket dir.
// A dir that does not resolve is left out; the verb then fails with its own error.
func (s *Server) evolveDirs(ticket tickets.Address) []string {
	var dirs []string
	if d, err := s.followUpsEpicPath(ticket.Project); err == nil {
		dirs = append(dirs, d)
	}
	if d, err := s.projectDir("gx"); err == nil {
		dirs = append(dirs, d)
	}
	return dirs
}

// agentsConfig is config agents.* for project, with the project's overrides on
// top.
func (s *Server) agentsConfig(project string) config.AgentsConfig {
	set := s.settings.Load()
	cfg := config.Config{Agents: config.AgentsConfig{Claude: set.AgentsClaude, Codex: set.AgentsCodex}}
	if dir, err := s.projectDir(project); err == nil {
		if pf, err := config.ReadProjectFile(dir); err == nil {
			cfg = cfg.WithProject(pf)
		}
	}
	return cfg.Agents
}

// runAgentConfig is the model and effort run's agent launched under, else
// the project's agents.* config. Every launch, finish and adoption resolves
// them here.
func (s *Server) runAgentConfig(project string, agent ralphloop.AgentKind, run Run) (model, effort string) {
	model, effort = ralphloop.ResolveTier(s.agentsConfig(project), agent, ralphloop.TierWorker)
	return cmp.Or(run.Model, model), cmp.Or(run.Effort, effort)
}

// agentEnv is the tab env, plus the run's agent (see AgentEnv) and, for an
// investigate agent, its ticket's address so its CLI calls identify it (see
// InvestigateEnv).
func agentEnv(tabEnv []string, t tickets.Ticket, addr tickets.Address, agent ralphloop.AgentKind) []string {
	env := append(slices.Clone(tabEnv), AgentEnv+"="+string(agent))
	if t.Type == string(schema.TypeInvestigate) {
		env = append(env, InvestigateEnv+"="+addr.String())
	}
	return env
}

// launch starts one's agent in cwd. Its Codex sandbox gets one.ScratchDir, the
// project dir where the epic's tickets live.
func (s *Server) launch(deps ralphloop.Deps, ticket tickets.Address, one ralphloop.OneIteration, cwd string) (Run, error) {
	t, agent := one.Ticket, one.Agent
	var extraDirs []string
	if t.Type == string(schema.TypeEvolve) {
		extraDirs = s.evolveDirs(ticket)
	}
	// herdr rejects a ticket address as an agent name; every lookup uses the iteration label.
	label, _, _ := ralphloop.IterationIdentity(ticket.Epic, ticket.ID, "")
	resumeID := one.ResumeSessionID
	start := func() (agentrunner.Session, error) {
		return ralphloop.StartAndPrompt(deps.Runner, agentrunner.StartOptions{
			Label:           label,
			Epic:            ticket.Epic,
			Cwd:             cwd,
			Kind:            agentrunner.Kind(agent),
			Args:            ralphloop.AgentArgs(agent, one.ScratchDir, ticket.Epic, one.Model, one.Effort, extraDirs...),
			Env:             agentEnv(s.settings.Load().TabEnv, t, ticket, agent),
			ResumeSessionID: resumeID,
		}, launchPrompt(agent, launchSkill(t), launchNote(t), ticket.String()), func(attempt int, decision string) {
			s.logEvent(ticket, ralphloop.Event{Type: string(events.PromptNudged), Label: label, Attempt: attempt, Reason: decision})
		})
	}
	session, err := start()
	if err != nil && resumeID != "" && !errors.Is(err, agentrunner.ErrLabelTaken) {
		// The conversation may be gone or unreadable: the agent starts over
		// from the ticket's latest round and handoff. A session that came up
		// before the error is stopped so only one of the two attempts lives.
		s.log.Warn("resume failed, cold-starting", "ticket", ticket.String(), "session", resumeID, "err", err)
		if session.Label != "" {
			_ = deps.Runner.Stop(session)
		}
		resumeID = ""
		session, err = start()
	}
	if errors.Is(err, agentrunner.ErrLabelTaken) {
		if h := s.labelHolder(deps.Runner, ticket, label, err); h.stale {
			if serr := deps.Runner.Stop(h.Session); serr != nil {
				s.log.Warn("close stale holder", "ticket", ticket.String(), "pane", h.ID, "err", serr)
			} else {
				s.logEvent(ticket, ralphloop.Event{Type: string(events.StalePaneClosed), Label: label, Pane: h.ID, Cwd: h.cwd, Reason: string(h.State)})
				session, err = start()
			}
		}
	}
	if errors.Is(err, agentrunner.ErrLabelTaken) {
		taken := &nameTakenError{reason: s.labelHolder(deps.Runner, ticket, label, err).describe(), err: err}
		s.logEvent(ticket, ralphloop.Event{Type: string(events.LaunchFailed), Kind: string(events.AgentNameTaken), Label: label, Reason: taken.Error()})
		return Run{}, taken
	}
	if err != nil {
		return Run{}, err
	}
	path := "cold-start"
	if resumeID != "" {
		path = "resumed"
	}
	s.logEvent(ticket, ralphloop.Event{
		Type: string(events.IterationStarted), Agent: agent, Label: label, Cwd: cwd,
		AgentSession: session.SessionID, Path: path, TargetTip: one.TargetTip,
	})
	run := Run{Address: ticket.String(), Agent: string(agent), Runner: deps.Runner.Name(), Session: session, Model: one.Model, Effort: one.Effort}
	return run, nil
}

// answeredSession is the last agent session of a ticket whose latest question
// has an answer, "" for any other ticket. Call it before the claim: claiming
// clears the answered state.
func answeredSession(t tickets.Ticket) string {
	st, err := schema.ParseTicket(t.Path)
	if err != nil || len(st.SessionIDs) == 0 {
		return ""
	}
	evs, err := tickets.ReadLog(t.Path)
	if err != nil || !tickets.Answered(st.Status, evs) {
		return ""
	}
	return st.SessionIDs[len(st.SessionIDs)-1]
}

// logEvent appends ev to ticket's run log, best effort.
func (s *Server) logEvent(ticket tickets.Address, ev ralphloop.Event) {
	dir, err := s.projectDir(ticket.Project)
	if err != nil {
		return
	}
	ev.Ticket, ev.Address = ticket.ID, ticket.String()
	_ = ralphloop.AppendEvent(dir, ticket.Epic, ev)
}

// nameTakenError is a launch whose label a holder gx may not close still
// has; reason names that holder.
type nameTakenError struct {
	reason string
	err    error
}

func (e *nameTakenError) Error() string { return e.err.Error() + ": " + e.reason }
func (e *nameTakenError) Unwrap() error { return e.err }

// labelHolder is the live session holding a launch's label.
type labelHolder struct {
	agentrunner.Session
	agentrunner.Status
	cwd   string
	found bool
	// stale: no other run tracks it and it is not mid-turn, so nothing is
	// lost by closing it.
	stale bool
}

func (h labelHolder) describe() string {
	if !h.found {
		return "holder not found"
	}
	return fmt.Sprintf("held by pane %s (cwd %s, status %s)", h.ID, cmp.Or(h.cwd, "unknown"), h.State)
}

// labelHolder finds who holds label after Start failed with takenErr. Only
// herdr's error says the holder's cwd. The launching ticket's own registry
// entry has no session yet, so it never counts as tracking the holder.
func (s *Server) labelHolder(r agentrunner.Runner, ticket tickets.Address, label string, takenErr error) labelHolder {
	var h labelHolder
	var taken *agentrunner.LabelTakenError
	if errors.As(takenErr, &taken) {
		h.cwd = taken.HolderCwd
	}
	sess, ok, err := r.Find(label)
	if err != nil || !ok {
		return h
	}
	h.Session, h.found = sess, true
	if h.Status, err = r.Status(sess); err != nil {
		return h
	}
	tracked := slices.ContainsFunc(s.registry.list(), func(t trackedRun) bool {
		return t.Address != ticket.String() && (t.Session.Label == sess.Label || (sess.ID != "" && t.Session.ID == sess.ID))
	})
	h.stale = !tracked && h.Waiting()
	return h
}
