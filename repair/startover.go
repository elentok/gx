package repair

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// StartOverResult is the --json success payload of a start-over. AtticRef is nil when
// the ticket had no iteration branch to set aside.
type StartOverResult struct {
	Ticket   string  `json:"ticket"`
	Status   string  `json:"status"`
	AtticRef *string `json:"attic_ref"`
	Warning  string  `json:"warning,omitempty"`
}

// StartOverInput is everything StartOver needs from its caller.
type StartOverInput struct {
	EpicPath string
	ID       string
	Reason   string
	Force    bool
	// DeleteBranch deletes the iteration branch instead of setting it aside.
	DeleteBranch bool
	// Source is who asked, a tickets.Source* value; it ends the ticket's open Pause round.
	Source string
	JSON   bool
	Cwd    string
	Now    time.Time
	// Subjects lists the commit subjects on branch since its merge-base with
	// base, newest first.
	Subjects func(dir, base, branch string) ([]string, error)
}

// StartOverLiveRunWarning is shown on every successful start-over: a running
// ralph-loop keeps its in-memory launched set, so a started-over ticket is not
// re-queued until the run restarts.
const StartOverLiveRunWarning = "if a ralph-loop run is live, this start-over is only half effective: downstream tickets unblock but this ticket is not re-queued until the run restarts"

// GitCommitSubjects is the production StartOverInput.Subjects.
func GitCommitSubjects(dir, base, branch string) ([]string, error) {
	commits, err := git.CommitsBetween(git.Repo{Root: dir}, base, branch)
	if err != nil {
		return nil, err
	}
	subjects := make([]string, len(commits))
	for i, c := range commits {
		subjects[i] = c.Subject
	}
	return subjects, nil
}

// StartOver sends a stuck ticket back to open, keeping its commits reachable
// under an attic ref.
func StartOver(in StartOverInput, d ralphloop.Deps) (StartOverResult, error) {
	if strings.TrimSpace(in.Reason) == "" {
		return StartOverResult{}, &RefusalError{Reason: ReasonReasonRequired, Message: "--reason is required"}
	}
	epic, t, err := FindEpicTicket(in.EpicPath, in.ID)
	if err != nil {
		return StartOverResult{}, err
	}
	if err := checkStartOverable(in, epic, t); err != nil {
		return StartOverResult{}, err
	}
	epicName := filepath.Base(filepath.Clean(in.EpicPath))
	if sess, alive := ralphloop.FindIterationSession(d, epicName, t.Identifier); alive {
		if err := closeIdleSession(d.Runner, sess, in.ID); err != nil {
			return StartOverResult{}, err
		}
	}

	wtDir, err := d.WorktreeDir(in.Cwd)
	if err != nil {
		return StartOverResult{}, err
	}
	branch := ralphloop.IterationBranch(epicName, t.Identifier)
	attic, err := gatherAttic(in, d, filepath.Join(wtDir, epicName), epicName, branch)
	if err != nil {
		return StartOverResult{}, err
	}

	if err := clearIteration(in, d, wtDir, epicName, t.Identifier, branch, attic); err != nil {
		return StartOverResult{}, err
	}

	if err := tickets.StartOver(t.Path, in.Now, startOverNote(in, attic), in.Source); err != nil {
		return StartOverResult{}, fmt.Errorf("starting ticket %s over: %w", in.ID, err)
	}

	res := StartOverResult{Ticket: in.ID, Status: string(schema.StatusOpen), AtticRef: attic.Ref, Warning: StartOverLiveRunWarning}
	ev := ralphloop.Event{Type: string(events.TicketStartedOver), Ticket: t.Identifier, Outcome: "started-over", Reason: in.Reason}
	if attic.Ref != nil {
		ev.AtticRef = *attic.Ref
		ev.SHA = attic.Tip
	}
	if err := ralphloop.AppendEvent(filepath.Dir(filepath.Clean(in.EpicPath)), epicName, ev); err != nil {
		return StartOverResult{}, fmt.Errorf("logging ticket-started-over: %w", err)
	}
	return res, nil
}

// closeIdleSession stops an idle or done agent: it is a leftover pane, and
// leaving it would make the next launch hit its label. A working agent, or
// one whose state is unknown, refuses.
func closeIdleSession(r agentrunner.Runner, s agentrunner.Session, id string) error {
	st, err := r.Status(s)
	if err != nil || (st.State != agentrunner.StateIdle && st.State != agentrunner.StateDone) {
		return &RefusalError{Reason: ReasonLiveAgentOnTab, Message: fmt.Sprintf("an agent is still alive for ticket %s; stop it first", id)}
	}
	if err := r.Stop(s); err != nil {
		return fmt.Errorf("closing the %s agent of ticket %s: %w", st.State, id, err)
	}
	return nil
}

// clearIteration removes the iteration worktree, then sets the branch aside
// (or deletes it with --delete-branch). The worktree goes first: git refuses
// to rename a branch that is checked out. A missing worktree or branch is a
// normal outcome.
func clearIteration(in StartOverInput, d ralphloop.Deps, wtDir, epic, id, branch string, attic atticInfo) error {
	path := ralphloop.IterationWorktreePath(wtDir, epic, id)
	if exists, err := d.WorktreeExists(path); err != nil {
		return fmt.Errorf("checking iteration worktree: %w", err)
	} else if exists {
		if err := d.RemoveWorktree(in.Cwd, path, true); err != nil {
			return fmt.Errorf("removing iteration worktree: %w", err)
		}
	}
	if attic.Tip == "" {
		return nil
	}
	if in.DeleteBranch {
		if err := d.DeleteBranch(in.Cwd, branch); err != nil {
			return fmt.Errorf("deleting %s: %w", branch, err)
		}
		return nil
	}
	if err := d.RenameBranch(in.Cwd, branch, *attic.Ref); err != nil {
		return fmt.Errorf("moving %s to %s: %w", branch, *attic.Ref, err)
	}
	return nil
}

// checkStartOverable applies the status and fork-children rules. Fork children
// are checked first: no flag overrides them.
func checkStartOverable(in StartOverInput, epic tickets.Epic, t tickets.Ticket) error {
	for _, other := range epic.Tickets {
		if other.Parent != nil && *other.Parent == t.Identifier &&
			other.Status != string(schema.StatusDone) && other.Type != string(schema.TypeInvestigate) {
			return &RefusalError{Reason: ReasonForkChildren, Message: fmt.Sprintf("ticket %s has fork child %s; start over %s instead", in.ID, other.Identifier, other.Identifier)}
		}
	}
	// Raw status, not RenderedStatus: blockers render a claimed ticket
	// "blocked" and dependents render a done one "waiting-for-children".
	switch status := schema.Status(t.Status); status {
	case schema.StatusClaimed, schema.StatusPaused:
	case schema.StatusDone:
		if !in.Force {
			return &RefusalError{Reason: ReasonStatusRefused, Message: fmt.Sprintf("ticket %s is done and already merged; pass --force to start it over anyway (the merge is not reverted)", in.ID)}
		}
	default:
		return &RefusalError{Reason: ReasonStatusRefused, Message: fmt.Sprintf("ticket %s is %v; start-over accepts only claimed or paused (done with --force)", in.ID, status)}
	}
	return nil
}

// atticInfo describes the iteration branch a start-over sets aside. Ref is nil when
// the branch does not exist (a normal outcome, Tip is then empty) or when
// --delete-branch discards it.
type atticInfo struct {
	Ref      *string
	Tip      string
	Subjects []string
}

func gatherAttic(in StartOverInput, d ralphloop.Deps, featurePath, epic, branch string) (atticInfo, error) {
	tip, err := d.RevParse(featurePath, branch)
	if errors.Is(err, git.ErrRefNotFound) {
		return atticInfo{}, nil
	}
	if err != nil {
		return atticInfo{}, fmt.Errorf("resolving %s: %w", branch, err)
	}
	if in.DeleteBranch {
		return atticInfo{Tip: tip}, nil
	}
	ref, err := nextAtticRef(d, featurePath, epic, in.ID)
	if err != nil {
		return atticInfo{}, err
	}
	subjects, err := in.Subjects(featurePath, epic, branch)
	if err != nil {
		return atticInfo{}, fmt.Errorf("listing commits on %s: %w", branch, err)
	}
	return atticInfo{Ref: &ref, Tip: tip, Subjects: subjects}, nil
}

// nextAtticRef is the first ralph-loop/attic/<epic>/<id>-<n> that does not
// exist yet, so a second start-over never overwrites the first's attic.
func nextAtticRef(d ralphloop.Deps, dir, epic, id string) (string, error) {
	for n := 1; n < 1000; n++ {
		ref := fmt.Sprintf("ralph-loop/attic/%s/%s-%d", epic, id, n)
		_, err := d.RevParse(dir, ref)
		if errors.Is(err, git.ErrRefNotFound) {
			return ref, nil
		}
		if err != nil {
			return "", fmt.Errorf("resolving %s: %w", ref, err)
		}
	}
	return "", fmt.Errorf("no free attic ref for ticket %s", id)
}

// startOverNote frames what the previous attempt left as unverified: nobody
// reviewed the commits, and a fresh agent must not trust them.
func startOverNote(in StartOverInput, a atticInfo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s** — started over by hand. Reason: %s\n\n", in.Now.Format("2006-01-02"), strings.TrimSpace(in.Reason))
	if a.Ref == nil && a.Tip != "" {
		fmt.Fprintf(&b, "The iteration branch (tip `%s`) was deleted with --delete-branch; no earlier work was kept.\n", a.Tip)
		return b.String()
	}
	if a.Ref == nil {
		b.WriteString("No iteration branch existed, so no earlier work was kept.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "Earlier partial work is unverified: it was never reviewed or landed, and may be incomplete or wrong. Treat it as a hint, not as done.\n\n- Attic ref: `%s` (tip `%s`)\n- Commits: %d\n", *a.Ref, a.Tip, len(a.Subjects))
	for _, s := range a.Subjects {
		fmt.Fprintf(&b, "  - %s\n", s)
	}
	return b.String()
}
