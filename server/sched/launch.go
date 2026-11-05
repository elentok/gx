package sched

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/budget"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/merge"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/queue"
	srvrecovery "github.com/elentok/gx/server/recovery"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketlog"
)

// LaunchHost is what the launcher needs from the server: the base derivation
// and the delay, which the claim loop shares.
type LaunchHost interface {
	// ResolveBase derives where t starts from; see the root's base rules.
	ResolveBase(addr tickets.Address, t tickets.Ticket, repo string, epics []tickets.Epic) (ref, stamp, leaf string, err error)
	// CommitlessRef is the ref a commitless ticket reads at.
	CommitlessRef(addr tickets.Address, t tickets.Ticket, repo string, epics []tickets.Epic) (string, error)
	// MergeTargetTip is the tip of the branch addr merges into.
	MergeTargetTip(addr tickets.Address, repo string) (string, error)
	// StalledNudgeDelay is how long an idle agent with no report waits for its nudge.
	StalledNudgeDelay() time.Duration
}

// LauncherDeps is what a Launcher is built from.
type LauncherDeps struct {
	Log      *slog.Logger
	Host     LaunchHost
	Projects *projects.Registry
	Recovery *srvrecovery.Recovery
	Sessions *sessions.Manager
	Settings *settings.Holder
	Registry *runs.Registry
	Guards   *runs.Guards
	Events   *broker.Broker
	Index    *index.Index
	Budget   *budget.Budget
	Writer   *ticketwrite.Writer
	Harness  *harnesschoice.Harness
	Landing  *merge.Landing
	Merger   *merge.Merger
	OneOffs  *queue.OneOffs
	Chat     *ralphloop.ServerChat
	// DropAskedAgent closes an agent that asked instead of keeping it alive.
	DropAskedAgent bool
}

// Launcher claims a ticket, starts its agent and settles the run when the
// agent ends.
type Launcher struct {
	LauncherDeps
}

// NewLauncher returns a launcher over d.
func NewLauncher(d LauncherDeps) *Launcher { return &Launcher{d} }

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
	return srvrecovery.InvestigatePrompt(t)
}

// evolveDirs are the dirs a retro's `gx tickets` verbs write to outside the
// ticket's own epic: the follow-ups epic and the gx project's ticket dir.
// A dir that does not resolve is left out; the verb then fails with its own error.
func (l *Launcher) evolveDirs(ticket tickets.Address) []string {
	var dirs []string
	if d, err := l.Recovery.FollowUpsEpicPath(ticket.Project); err == nil {
		dirs = append(dirs, d)
	}
	if d, err := l.Projects.Dir("gx"); err == nil {
		dirs = append(dirs, d)
	}
	return dirs
}

// agentEnv is the tab env, plus the run's agent (see AgentEnv) and, for an
// investigate agent, its ticket's address so its CLI calls identify it (see
// InvestigateEnv).
func agentEnv(tabEnv []string, t tickets.Ticket, addr tickets.Address, agentKind agent.AgentKind) []string {
	env := append(slices.Clone(tabEnv), projects.AgentEnv+"="+string(agentKind))
	if t.Type == string(schema.TypeInvestigate) {
		env = append(env, wire.InvestigateEnv+"="+addr.String())
	}
	return env
}

// launch starts one's agent in cwd. Its Codex sandbox gets one.ScratchDir, the
// project dir where the epic's tickets live.
func (l *Launcher) launch(deps ralphloop.Deps, ticket tickets.Address, one ralphloop.OneIteration, cwd string) (runs.Run, error) {
	t, agentKind := one.Ticket, one.Agent
	var extraDirs []string
	if t.Type == string(schema.TypeEvolve) {
		extraDirs = l.evolveDirs(ticket)
	}
	// herdr rejects a ticket address as an agent name; every lookup uses the iteration label.
	label, _, _ := ralphloop.IterationIdentity(ticket.Epic, ticket.ID, "")
	resumeID := one.ResumeSessionID
	if agentKind == agent.AgentCodex && !l.CodexRolloutExists(cwd, resumeID) {
		resumeID = ""
	}
	// A server stop ends the launch prompt's wait, which Serve would otherwise
	// sit out before it can finish.
	ctx, release := l.Sessions.Watches.Context()
	defer release()
	start := func() (agentrunner.Session, error) {
		return ralphloop.StartAndPrompt(ctx, deps.Runner, agentrunner.StartOptions{
			Label:           label,
			Epic:            ticket.Epic,
			Cwd:             cwd,
			Kind:            agentrunner.Kind(agentKind),
			Args:            agent.AgentArgs(agentKind, cwd, one.ScratchDir, ticket.Epic, one.Model, one.Effort, extraDirs...),
			Env:             agentEnv(l.Settings.Load().TabEnv, t, ticket, agentKind),
			ResumeSessionID: resumeID,
		}, srvrecovery.LaunchPrompt(agentKind, srvrecovery.LaunchSkill(t), launchNote(t), ticket.String()), func(attempt int, decision string) {
			l.LogEvent(ticket, ralphloop.Event{Type: string(events.PromptNudged), Label: label, Attempt: attempt, Reason: decision})
		})
	}
	session, err := start()
	if err != nil && resumeID != "" && !errors.Is(err, agentrunner.ErrLabelTaken) {
		// The conversation may be gone or unreadable: the agent starts over
		// from the ticket's latest round and handoff. A session that came up
		// before the error is stopped so only one of the two attempts lives.
		l.Log.Warn("resume failed, cold-starting", "ticket", ticket.String(), "session", resumeID, "err", err)
		if session.Label != "" {
			_ = deps.Runner.Stop(session)
		}
		resumeID = ""
		session, err = start()
	}
	if errors.Is(err, agentrunner.ErrLabelTaken) {
		if h := l.labelHolder(deps.Runner, ticket, label, err); h.stale {
			if serr := deps.Runner.Stop(h.Session); serr != nil {
				l.Log.Warn("close stale holder", "ticket", ticket.String(), "pane", h.ID, "err", serr)
			} else {
				l.LogEvent(ticket, ralphloop.Event{Type: string(events.StalePaneClosed), Label: label, Pane: h.ID, Cwd: h.cwd, Reason: string(h.State)})
				session, err = start()
			}
		}
	}
	if errors.Is(err, agentrunner.ErrLabelTaken) {
		taken := &nameTakenError{reason: l.labelHolder(deps.Runner, ticket, label, err).describe(), err: err}
		l.LogEvent(ticket, ralphloop.Event{Type: string(events.LaunchFailed), Kind: string(events.AgentNameTaken), Label: label, Reason: taken.Error()})
		return runs.Run{}, taken
	}
	if err != nil {
		return runs.Run{}, err
	}
	path := "cold-start"
	if resumeID != "" {
		path = "resumed"
	}
	l.LogEvent(ticket, ralphloop.Event{
		Type: string(events.IterationStarted), Agent: agentKind, Label: label, Cwd: cwd,
		AgentSession: session.SessionID, Path: path, TargetTip: one.TargetTip,
	})
	run := runs.Run{Address: ticket.String(), Agent: string(agentKind), Runner: deps.Runner.Name(), Session: session, Model: one.Model, Effort: one.Effort}
	return run, nil
}

// CodexRolloutExists reports whether Codex has a rollout for session id under
// cwd. An empty id has none.
func (l *Launcher) CodexRolloutExists(cwd, id string) bool {
	if id == "" {
		return false
	}
	home, err := l.Projects.CodexHome()
	if err != nil {
		return false
	}
	ok, err := codexsession.VerifyIdentityIn(home, cwd, id)
	return err == nil && ok
}

// AnsweredSession is the last agent session of a ticket whose latest question
// has an answer, "" for any other ticket. Call it before the claim: claiming
// clears the answered state.
func AnsweredSession(t tickets.Ticket) string {
	st, err := schema.ParseTicket(t.Path)
	if err != nil || len(st.SessionIDs) == 0 {
		return ""
	}
	evs, err := tickets.ReadLog(t.Path)
	if err != nil || !ticketlog.Answered(st.Status, evs) {
		return ""
	}
	return st.SessionIDs[len(st.SessionIDs)-1]
}

// LogEvent appends ev to ticket's run log, best effort.
func (l *Launcher) LogEvent(ticket tickets.Address, ev ralphloop.Event) {
	dir, err := l.Projects.Dir(ticket.Project)
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
func (l *Launcher) labelHolder(r agentrunner.Runner, ticket tickets.Address, label string, takenErr error) labelHolder {
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
	tracked := slices.ContainsFunc(l.Registry.List(), func(t runs.TrackedRun) bool {
		return t.Address != ticket.String() && (t.Session.Label == sess.Label || (sess.ID != "" && t.Session.ID == sess.ID))
	})
	h.stale = !tracked && h.Waiting()
	return h
}
