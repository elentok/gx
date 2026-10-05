package cmd

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/spf13/cobra"
)

// resetResult is the --json success payload of `gx tickets reset`. AtticRef is
// null when the ticket had no iteration branch to set aside.
type resetResult struct {
	Ticket   string  `json:"ticket"`
	Status   string  `json:"status"`
	AtticRef *string `json:"attic_ref"`
	Warning  string  `json:"warning,omitempty"`
}

// resetInput is everything runTicketsReset needs from the command line.
type resetInput struct {
	EpicPath string
	ID       string
	Reason   string
	Force    bool
	// DeleteBranch deletes the iteration branch instead of setting it aside.
	DeleteBranch bool
	JSON         bool
	Cwd          string
	Now          time.Time
	// Subjects lists the commit subjects on branch since its merge-base with
	// base, newest first.
	Subjects func(dir, base, branch string) ([]string, error)
}

// resetLiveRunWarning is shown on every successful reset: a running
// ralph-loop keeps its in-memory launched set, so a reset ticket is not
// re-queued until the run restarts.
const resetLiveRunWarning = "if a ralph-loop run is live, this reset is only half effective: downstream tickets unblock but this ticket is not re-queued until the run restarts"

const resetLongHelp = `Send a stuck ticket back to open instead of landing it.

Requires --reason, which is written into the ticket's Comments framed as
unverified partial work. Accepts claimed, needs-repair and needs-answer
tickets; done only with --force, which silences the already-landed refusal
and never reverts the landing. Refuses a ticket with fork children (reset a
child instead) and a ticket whose agent is still alive on its herdr tab.`

func newTicketsResetCmd(d deps) *cobra.Command {
	in := resetInput{Subjects: gitCommitSubjects}
	cmd := &cobra.Command{
		Use:   "reset <epic> <id>",
		Short: "send a stuck ticket back to open, keeping its commits reachable",
		Long:  resetLongHelp,
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			in.Cwd = cwd
			in.EpicPath = resolveEpicArg(args[0], cwd)
			in.ID = args[1]
			in.Now = time.Now()
			return runTicketsReset(in, ralphloop.DefaultDeps(), c.OutOrStdout(), c.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&in.Reason, "reason", "", "why the ticket is being reset (required)")
	cmd.Flags().BoolVar(&in.Force, "force", false, "allow resetting a done ticket")
	cmd.Flags().BoolVar(&in.DeleteBranch, "delete-branch", false, "delete the iteration branch instead of keeping it under ralph-loop/attic/")
	cmd.Flags().BoolVar(&in.JSON, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func gitCommitSubjects(dir, base, branch string) ([]string, error) {
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

func runTicketsReset(in resetInput, d ralphloop.Deps, stdout, stderr io.Writer) error {
	res, err := resetTicket(in, d)
	text := fmt.Sprintf("%s: reset to open (attic: %s)", in.ID, atticLabel(res.AtticRef))
	if err == nil && !in.JSON {
		fmt.Fprintln(stderr, "warning: "+resetLiveRunWarning)
	}
	return finishRecovery(stdout, stderr, in.JSON, res, text, err)
}

func atticLabel(ref *string) string {
	if ref == nil {
		return "none, no iteration branch"
	}
	return *ref
}

func resetTicket(in resetInput, d ralphloop.Deps) (resetResult, error) {
	if strings.TrimSpace(in.Reason) == "" {
		return resetResult{}, &RefusalError{Reason: ReasonReasonRequired, Message: "--reason is required"}
	}
	epic, t, err := findEpicTicket(in.EpicPath, in.ID)
	if err != nil {
		return resetResult{}, err
	}
	if err := checkResettable(in, epic, t); err != nil {
		return resetResult{}, err
	}
	epicName := filepath.Base(filepath.Clean(in.EpicPath))
	if ralphloop.IterationAgentAlive(d, epicName, t.Identifier) {
		return resetResult{}, &RefusalError{Reason: ReasonLiveAgentOnTab, Message: fmt.Sprintf("an agent is still alive on the herdr tab for ticket %s; stop it first", in.ID)}
	}

	wtDir, err := d.WorktreeDir(in.Cwd)
	if err != nil {
		return resetResult{}, err
	}
	branch := ralphloop.IterationBranch(epicName, t.Identifier)
	attic, err := gatherAttic(in, d, filepath.Join(wtDir, epicName), epicName, branch)
	if err != nil {
		return resetResult{}, err
	}

	if err := clearIteration(in, d, wtDir, epicName, t.Identifier, branch, attic); err != nil {
		return resetResult{}, err
	}

	if err := tickets.Reset(t.Path, in.Now, resetNote(in, attic)); err != nil {
		return resetResult{}, fmt.Errorf("resetting ticket %s: %w", in.ID, err)
	}

	res := resetResult{Ticket: in.ID, Status: string(schema.StatusOpen), AtticRef: attic.Ref, Warning: resetLiveRunWarning}
	ev := ralphloop.Event{Type: ralphloop.EventTicketReset, Ticket: t.Identifier, Outcome: "reset", Reason: in.Reason}
	if attic.Ref != nil {
		ev.AtticRef = *attic.Ref
		ev.SHA = attic.Tip
	}
	if err := ralphloop.AppendEvent(filepath.Dir(filepath.Clean(in.EpicPath)), epicName, ev); err != nil {
		return resetResult{}, fmt.Errorf("logging ticket-reset: %w", err)
	}
	return res, nil
}

// clearIteration removes the iteration worktree and stale tab, then sets the
// branch aside (or deletes it with --delete-branch). The worktree goes first:
// git refuses to rename a branch that is checked out. A missing worktree, tab
// or branch is a normal outcome.
func clearIteration(in resetInput, d ralphloop.Deps, wtDir, epic, id, branch string, attic atticInfo) error {
	path := ralphloop.IterationWorktreePath(wtDir, epic, id)
	if exists, err := d.WorktreeExists(path); err != nil {
		return fmt.Errorf("checking iteration worktree: %w", err)
	} else if exists {
		if err := d.RemoveWorktree(in.Cwd, path, true); err != nil {
			return fmt.Errorf("removing iteration worktree: %w", err)
		}
	}
	if tabID := ralphloop.IterationTabID(d, epic, id); tabID != "" {
		if err := d.TabClose(tabID); err != nil {
			return fmt.Errorf("closing iteration tab: %w", err)
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

// checkResettable applies the status and fork-children rules. Fork children
// are checked first: no flag overrides them.
func checkResettable(in resetInput, epic tickets.Epic, t tickets.Ticket) error {
	for _, other := range epic.Tickets {
		if other.Parent != nil && *other.Parent == t.Identifier {
			return &RefusalError{Reason: ReasonForkChildren, Message: fmt.Sprintf("ticket %s has fork child %s; reset %s instead", in.ID, other.Identifier, other.Identifier)}
		}
	}
	// Raw status, not RenderedStatus: blockers render a claimed ticket
	// "blocked" and dependents render a done one "waiting-for-children".
	switch status := schema.Status(t.Status); status {
	case schema.StatusClaimed, schema.StatusNeedsRepair, schema.StatusNeedsAnswer:
	case schema.StatusDone:
		if !in.Force {
			return &RefusalError{Reason: ReasonStatusRefused, Message: fmt.Sprintf("ticket %s is done and already landed; pass --force to reset it anyway (the landing is not reverted)", in.ID)}
		}
	default:
		return &RefusalError{Reason: ReasonStatusRefused, Message: fmt.Sprintf("ticket %s is %v; reset accepts only claimed, needs-repair or needs-answer (done with --force)", in.ID, status)}
	}
	return nil
}

// atticInfo describes the iteration branch a reset sets aside. Ref is nil when
// the branch does not exist (a normal outcome, Tip is then empty) or when
// --delete-branch discards it.
type atticInfo struct {
	Ref      *string
	Tip      string
	Subjects []string
}

func gatherAttic(in resetInput, d ralphloop.Deps, featurePath, epic, branch string) (atticInfo, error) {
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
// exist yet, so a second reset never overwrites the first's attic.
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

// resetNote frames what the previous attempt left as unverified: nobody
// reviewed the commits, and a fresh agent must not trust them.
func resetNote(in resetInput, a atticInfo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s** — reset by hand. Reason: %s\n\n", in.Now.Format("2006-01-02"), strings.TrimSpace(in.Reason))
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
