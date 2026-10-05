package cmd

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/spf13/cobra"
)

// landResult is the --json success payload of `gx tickets land`.
type landResult struct {
	Outcome        string `json:"outcome"`
	SHA            string `json:"sha"`
	TrailerValue   string `json:"trailer_value"`
	MetricsStamped bool   `json:"metrics_stamped"`
}

// landInput is everything runTicketsLand needs from the command line.
type landInput struct {
	EpicPath      string
	ID            string
	From, To      string
	IgnoreLiveTab bool
	Continue      bool
	Abort         bool
	JSON          bool
	Cwd           string
	Getwd         func() (string, error)
}

const landLongHelp = `Land a stuck ticket's commits onto the epic's feature branch, the way the
orchestrator would, and leave the ticket at status: done.

Accepts done, claimed and needs-repair tickets. Refuses draft, open and
needs-answer tickets, and tickets explicitly flagged commitless.

The commit range comes from the ticket's iteration branch; --from/--to land an
explicit range instead (--from is exclusive). Already-landed work is detected
and only the status is repaired. A conflict exits 0, leaves the cherry-pick in
progress and writes a land marker; nothing is written to the ticket.

After resolving a conflict, run land --continue: it refuses while the
cherry-pick is still in progress or the branch has not moved, then stamps the
commit, marks the ticket done and clears the marker and lock. land --abort
aborts the cherry-pick and clears the marker and lock; no ticket is touched.

Refuses to run from a ralph-loop/* working directory. This is a guard rail,
not a boundary: changing directory evades it. It also refuses while the
iteration's herdr tab exists (--ignore-live-tab overrides, since a tab can
outlive its agent).

Metrics are recovered from the run log. Recovered elapsed time is the least
trustworthy of the three metrics. Worktree, branch and tab leftovers are not
cleaned; a later live run sweeps them.`

func newTicketsLandCmd(d deps) *cobra.Command {
	in := landInput{Getwd: d.getwd}
	cmd := &cobra.Command{
		Use:   "land <epic> <id>",
		Short: "land a stuck ticket's commits onto the feature branch and mark it done",
		Long:  landLongHelp,
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			in.Cwd = cwd
			in.EpicPath = resolveEpicArg(args[0], cwd)
			in.ID = args[1]
			return runTicketsLand(in, ralphloop.DefaultDeps(), c.OutOrStdout(), c.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&in.From, "from", "", "exclusive base of an explicit commit range (requires --to)")
	cmd.Flags().StringVar(&in.To, "to", "", "tip of an explicit commit range (requires --from)")
	cmd.Flags().BoolVar(&in.IgnoreLiveTab, "ignore-live-tab", false, "land even though the iteration's herdr tab exists")
	cmd.Flags().BoolVar(&in.Continue, "continue", false, "finish a conflicted land after resolving it: stamp, mark done, clear the marker and lock")
	cmd.Flags().BoolVar(&in.Abort, "abort", false, "abandon a conflicted land: abort the cherry-pick, clear the marker and lock")
	cmd.Flags().BoolVar(&in.JSON, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func runTicketsLand(in landInput, d ralphloop.Deps, stdout, stderr io.Writer) error {
	res, text, err := landStuckTicket(in, d)
	return finishRecovery(stdout, stderr, in.JSON, res, text, err)
}

func landStuckTicket(in landInput, d ralphloop.Deps) (landResult, string, error) {
	if branch, ok := isRalphLoopBranch(in.Getwd); ok {
		return landResult{}, "", &RefusalError{Reason: ReasonRalphLoopCwd, Message: fmt.Sprintf("refusing to land from a ralph-loop/* working directory (%s); run from outside the iteration worktree", branch)}
	}
	if in.Continue && in.Abort {
		return landResult{}, "", errors.New("--continue and --abort are mutually exclusive")
	}
	if in.Continue || in.Abort {
		return resolveLand(in, d)
	}
	if (in.From == "") != (in.To == "") {
		return landResult{}, "", errors.New("--from and --to must be given together")
	}

	_, t, err := findEpicTicket(in.EpicPath, in.ID)
	if err != nil {
		return landResult{}, "", err
	}
	parsed, err := schema.ParseTicket(t.Path)
	if err != nil {
		return landResult{}, "", err
	}
	if err := checkLandable(in.ID, parsed); err != nil {
		return landResult{}, "", err
	}

	epic := filepath.Base(filepath.Clean(in.EpicPath))
	if !in.IgnoreLiveTab && ralphloop.IterationTabLive(d, epic, t.Identifier) {
		return landResult{}, "", &RefusalError{Reason: ReasonLiveAgentOnTab, Message: fmt.Sprintf("the herdr tab for ticket %s exists; close it or pass --ignore-live-tab", in.ID)}
	}

	wtDir, err := d.WorktreeDir(in.Cwd)
	if err != nil {
		return landResult{}, "", err
	}
	featurePath := filepath.Join(wtDir, epic)
	rng, err := landSourceRange(in, d, featurePath, epic, t.Identifier)
	if err != nil {
		return landResult{}, "", err
	}

	lockDir := filepath.Clean(in.EpicPath)
	if err := preflightLand(lockDir, epic, in.ID, featurePath, d); err != nil {
		return landResult{}, "", err
	}
	if err := ralphloop.AcquireLandLock(lockDir); err != nil {
		if errors.Is(err, ralphloop.ErrLandLocked) {
			return landResult{}, "", &RefusalError{Reason: ReasonLandLocked, Message: "another land is in progress (land lock is held)"}
		}
		return landResult{}, "", err
	}
	conflicted := false
	defer func() {
		// A conflict keeps the lock: it must outlive this process until a
		// --continue or --abort clears it.
		if !conflicted {
			_ = ralphloop.ReleaseLandLock(lockDir)
		}
	}()

	prePickHead, err := d.RevParse(featurePath, "HEAD")
	if err != nil {
		return landResult{}, "", fmt.Errorf("resolving %s HEAD: %w", epic, err)
	}
	session, recorded := ralphloop.RecoverLandSession(filepath.Dir(lockDir), epic, t.Identifier)
	res, err := ralphloop.LandTicket(ralphloop.LandDepsOf(d), ralphloop.LandParams{
		FeatureWorktree: featurePath,
		FeatureBranch:   epic,
		TicketID:        t.Identifier,
		TicketPath:      t.Path,
		SourceRange:     rng,
		RecordedSHA:     recorded,
		Session:         session,
	})
	if err != nil {
		return landResult{}, "", err
	}

	out := landResult{Outcome: jsonOutcome(res.Outcome), SHA: res.SHA, TrailerValue: res.TrailerValue, MetricsStamped: res.MetricsStamped}
	if res.Outcome == ralphloop.Conflicted {
		conflicted = true
		marker := ralphloop.LandMarker{Epic: epic, Ticket: in.ID, SourceRange: rng.Base + ".." + rng.Tip, PrePickHead: prePickHead}
		if err := ralphloop.WriteLandMarker(lockDir, marker); err != nil {
			return landResult{}, "", fmt.Errorf("writing land marker: %w", err)
		}
		return out, fmt.Sprintf("%s: conflict landing %s; resolve it in %s, then --continue or --abort", in.ID, marker.SourceRange, featurePath), nil
	}

	if err := recordLanded(lockDir, epic, t, parsed.Status, res, session); err != nil {
		return landResult{}, "", err
	}
	return out, fmt.Sprintf("%s: %s (%s)", in.ID, jsonOutcome(res.Outcome), res.SHA), nil
}

// recordLanded writes status: done (unless already) and the manual-land event.
func recordLanded(lockDir, epic string, t tickets.Ticket, status schema.Status, res ralphloop.LandResult, session ralphloop.LandSession) error {
	if status != schema.StatusDone {
		if err := ralphloop.MarkDone(t.Path); err != nil {
			return fmt.Errorf("marking ticket %s done: %w", t.Identifier, err)
		}
	}
	ev := ralphloop.Event{
		Type:         ralphloop.EventManualLand,
		Ticket:       t.Identifier,
		Outcome:      string(res.Outcome),
		SHA:          res.SHA,
		TrailerValue: res.TrailerValue,
		AgentSession: session.ID,
		Agent:        session.Agent,
	}
	if session.ID == "" {
		ev.Reason = "no recoverable agent session; metrics not stamped"
	}
	if err := ralphloop.AppendEvent(filepath.Dir(lockDir), epic, ev); err != nil {
		return fmt.Errorf("logging manual-land: %w", err)
	}
	return nil
}

// resolveLand finishes (--continue) or abandons (--abort) the conflict a
// previous land left pending for in.ID.
func resolveLand(in landInput, d ralphloop.Deps) (landResult, string, error) {
	if in.From != "" || in.To != "" {
		return landResult{}, "", errors.New("--from/--to cannot be combined with --continue or --abort")
	}
	_, t, err := findEpicTicket(in.EpicPath, in.ID)
	if err != nil {
		return landResult{}, "", err
	}
	epic := filepath.Base(filepath.Clean(in.EpicPath))
	lockDir := filepath.Clean(in.EpicPath)
	marker, err := ralphloop.ReadLandMarker(lockDir)
	if err != nil {
		return landResult{}, "", err
	}
	if marker == nil || marker.Ticket != in.ID {
		return landResult{}, "", &RefusalError{Reason: ReasonNoPendingLand, Message: fmt.Sprintf("no conflict landing ticket %s is pending", in.ID)}
	}
	wtDir, err := d.WorktreeDir(in.Cwd)
	if err != nil {
		return landResult{}, "", err
	}
	featurePath := filepath.Join(wtDir, epic)
	inProgress, err := d.CherryPickInProgress(featurePath)
	if err != nil {
		return landResult{}, "", fmt.Errorf("checking cherry-pick state: %w", err)
	}

	if in.Abort {
		if inProgress {
			if err := d.AbortCherryPick(featurePath); err != nil {
				return landResult{}, "", fmt.Errorf("aborting cherry-pick: %w", err)
			}
		}
		if err := ralphloop.ClearLand(lockDir); err != nil {
			return landResult{}, "", fmt.Errorf("clearing land marker: %w", err)
		}
		return landResult{Outcome: "aborted"}, fmt.Sprintf("%s: land aborted", in.ID), nil
	}

	if inProgress {
		return landResult{}, "", &RefusalError{Reason: ReasonLandConflictPending, Message: fmt.Sprintf("the cherry-pick for ticket %s is still in progress; resolve it and run `git cherry-pick --continue` first", in.ID)}
	}
	head, err := d.RevParse(featurePath, "HEAD")
	if err != nil {
		return landResult{}, "", fmt.Errorf("resolving %s HEAD: %w", epic, err)
	}
	if head == marker.PrePickHead {
		return landResult{}, "", &RefusalError{Reason: ReasonLandNotResolved, Message: fmt.Sprintf("%s has not moved since the conflict; nothing was committed (use --abort to give up)", epic)}
	}

	parsed, err := schema.ParseTicket(t.Path)
	if err != nil {
		return landResult{}, "", err
	}
	session, _ := ralphloop.RecoverLandSession(filepath.Dir(lockDir), epic, t.Identifier)
	res, err := ralphloop.StampLanded(ralphloop.LandDepsOf(d), ralphloop.LandParams{
		FeatureWorktree: featurePath,
		FeatureBranch:   epic,
		TicketID:        t.Identifier,
		TicketPath:      t.Path,
		Session:         session,
	})
	if err != nil {
		return landResult{}, "", err
	}
	if err := recordLanded(lockDir, epic, t, parsed.Status, res, session); err != nil {
		return landResult{}, "", err
	}
	if err := ralphloop.ClearLand(lockDir); err != nil {
		return landResult{}, "", fmt.Errorf("clearing land marker: %w", err)
	}
	out := landResult{Outcome: jsonOutcome(res.Outcome), SHA: res.SHA, TrailerValue: res.TrailerValue, MetricsStamped: res.MetricsStamped}
	return out, fmt.Sprintf("%s: %s (%s)", in.ID, out.Outcome, res.SHA), nil
}

// checkLandable applies the status and commitless rules. Commitless is the
// explicit flag only, never the type-derived notion.
func checkLandable(id string, t schema.Ticket) error {
	switch t.Status {
	case schema.StatusDone, schema.StatusClaimed, schema.StatusNeedsRepair:
	default:
		return &RefusalError{Reason: ReasonStatusRefused, Message: fmt.Sprintf("ticket %s is %s; land accepts only done, claimed or needs-repair", id, t.Status)}
	}
	if t.Commitless {
		return &RefusalError{Reason: ReasonCommitless, Message: fmt.Sprintf("ticket %s is flagged commitless; there is nothing to land", id)}
	}
	return nil
}

// landSourceRange is --from/--to when given, else the iteration branch's
// commits since its merge-base with the feature branch.
func landSourceRange(in landInput, d ralphloop.Deps, featurePath, epic, identifier string) (ralphloop.SourceRange, error) {
	if in.From != "" {
		return ralphloop.SourceRange{Base: in.From, Tip: in.To}, nil
	}
	branch := ralphloop.IterationBranch(epic, identifier)
	if _, err := d.RevParse(featurePath, branch); err != nil {
		return ralphloop.SourceRange{}, &RefusalError{Reason: ReasonIterationBranchMissing, Message: fmt.Sprintf("no iteration branch %s; commits not recoverable (--from/--to can still land an explicit range)", branch)}
	}
	base, err := d.MergeBase(featurePath, branch, epic)
	if err != nil {
		return ralphloop.SourceRange{}, fmt.Errorf("resolving merge-base of %s and %s: %w", branch, epic, err)
	}
	return ralphloop.SourceRange{Base: base, Tip: branch}, nil
}

// preflightLand is the three-way marker/cherry-pick check.
func preflightLand(lockDir, epic, id, featurePath string, d ralphloop.Deps) error {
	inProgress, err := d.CherryPickInProgress(featurePath)
	if err != nil {
		return fmt.Errorf("checking cherry-pick state: %w", err)
	}
	verdict, err := ralphloop.LandPreflight(lockDir, epic, id, inProgress)
	if err != nil {
		return err
	}
	switch verdict {
	case ralphloop.LandConflictPending:
		return &RefusalError{Reason: ReasonLandConflictPending, Message: fmt.Sprintf("a conflict landing ticket %s is pending; resolve it, then --continue or --abort", id)}
	case ralphloop.LandOtherTicket:
		return &RefusalError{Reason: ReasonLandBlocked, Message: "a conflict landing a different ticket is pending; finish or abort it first"}
	case ralphloop.LandUnknownPick:
		return &RefusalError{Reason: ReasonLandBlocked, Message: "a cherry-pick is in progress on the feature branch with no land marker; refusing to trample it"}
	}
	return nil
}

// jsonOutcome maps the Go outcome to the spec's kebab-case JSON vocabulary.
func jsonOutcome(o ralphloop.LandOutcome) string {
	if o == ralphloop.AlreadyApplied {
		return "already-applied"
	}
	return string(o)
}
