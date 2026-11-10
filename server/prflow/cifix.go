package prflow

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/elentok/gx/git/pr"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const ciFixSlug = "ci-fix"

// Outcomes a ci-fix Resolution reports on its `Outcome:` line; the spelling is
// the one skills/gx-ci-fix/SKILL.md uses.
const (
	OutcomeFixed           = "fixed"
	OutcomeUnfixable       = "unfixable"
	OutcomeForeignConflict = "foreign-conflict"
)

// CIFixResult is what ForkCIFix did.
type CIFixResult int

// ForkCIFix results.
const (
	// CIFixStarted: a ticket was forked.
	CIFixStarted CIFixResult = iota
	// CIFixBusy: a ci-fix ticket is already open or claimed.
	CIFixBusy
	// CIFixCapReached: the per-check or per-PR attempts are used up.
	CIFixCapReached
)

// CIFixJob is one failed run to fix.
type CIFixJob struct {
	Project string
	Epic    string
	Number  int
	URL     string
	HeadSHA string
	Check   pr.Check
	Skills  []string
	// PerCheck and PerPR are the attempt caps since the last reset.
	PerCheck, PerPR int
}

// CheckKeyString is the key a "CI failed" entry names a check by.
func CheckKeyString(k pr.CheckKey) string {
	if k.WorkflowID == "" {
		return k.Name
	}
	return k.WorkflowID + "/" + k.Name
}

// RunsInFeature reports whether tickets of typ run in the epic's feature
// worktree, on the epic branch, like a merge ticket (its commits stay there).
func RunsInFeature(typ schema.TicketType) bool { return typ == schema.TypeCIFix }

func openCIFix(t tickets.Ticket) bool {
	return t.Type == string(schema.TypeCIFix) && (t.Status == string(schema.StatusOpen) || t.Status == string(schema.StatusClaimed))
}

// ForkCIFix forks one ci-fix ticket into the epic at epicPath for job.Check and
// logs a "CI failed" entry. It forks none while another is open or claimed, or
// when the caps count the attempts already used; counts come from the activity
// log, never from the tickets.
func ForkCIFix(epicPath string, job CIFixJob) (id string, res CIFixResult, err error) {
	epic := tickets.LoadEpicAt(epicPath)
	if slices.ContainsFunc(epic.Tickets, openCIFix) {
		return "", CIFixBusy, nil
	}
	byCheck, err := CIFailedByCheck(epicPath)
	if err != nil {
		return "", 0, err
	}
	total, err := CIFailedAttempts(epicPath)
	if err != nil {
		return "", 0, err
	}
	key := CheckKeyString(job.Check.Key())
	if byCheck[key] >= job.PerCheck || total >= job.PerPR {
		return "", CIFixCapReached, nil
	}
	earlier := earlierResolutions(epic)
	_, id, err = ticketwrite.WriteNewTicket(epicPath, "", ciFixSlug, func(id string) (schema.Ticket, string) {
		return schema.Ticket{ID: schema.TicketID(id), Status: schema.StatusOpen, Type: schema.TypeCIFix},
			job.body(id, byCheck[key]+1, earlier)
	})
	if err != nil {
		return "", 0, err
	}
	if err := Append(epicPath, Entry{Kind: KindCIFailed, Checks: []string{key}, HeadSHA: job.HeadSHA, TicketID: id}); err != nil {
		return id, CIFixStarted, err
	}
	return id, CIFixStarted, nil
}

type earlierFix struct{ id, resolution string }

// earlierResolutions is the Resolution of every earlier ci-fix ticket, oldest first.
func earlierResolutions(e tickets.Epic) []earlierFix {
	var out []earlierFix
	for _, t := range e.Tickets {
		if t.Type != string(schema.TypeCIFix) {
			continue
		}
		if res, err := resolutionOf(t.Path); err == nil && strings.TrimSpace(res) != "" {
			out = append(out, earlierFix{id: t.Identifier, resolution: res})
		}
	}
	slices.SortFunc(out, func(a, b earlierFix) int { return strings.Compare(a.id, b.id) })
	return out
}

func (j CIFixJob) body(id string, attempt int, earlier []earlierFix) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n# %s — Fix CI check %s on %s\n\n## What to build\n\n", id, j.Check.Name, j.Epic)
	b.WriteString("Fix this failed CI run on the epic's pull request branch, as gx-ci-fix describes.\n\n")
	fmt.Fprintf(&b, "%s %s:%s\nPR: #%d %s\nCheck: %s\n", epicLabel, j.Project, j.Epic, j.Number, j.URL, j.Check.Name)
	if j.Check.RunID != "" {
		fmt.Fprintf(&b, "Workflow run id: %s\nJob id: %s\n", j.Check.RunID, j.Check.JobID)
	} else {
		fmt.Fprintf(&b, "No workflow run: read what this URL points to, and report unfixable when it cannot be read.\nURL: %s\n", j.Check.URL)
	}
	fmt.Fprintf(&b, "Head SHA: %s\nAttempt %d of %d\n", j.HeadSHA, attempt, j.PerCheck)
	if len(j.Skills) > 0 {
		b.WriteString("\n## Skills\n\nRead and follow each of these skills (skills.ci-fix):\n\n")
		for _, s := range j.Skills {
			fmt.Fprintf(&b, "- %s\n", s)
		}
	}
	if len(earlier) > 0 {
		b.WriteString("\n## Earlier attempts\n\nDo not repeat a change that already failed.\n")
		for _, e := range earlier {
			fmt.Fprintf(&b, "\n### Resolution of ticket %s\n\n%s\n", e.id, e.resolution)
		}
	}
	return b.String()
}

// OutcomeOf is the `Outcome:` line of a ci-fix ticket's Resolution, "" when it
// has none.
func OutcomeOf(ticketPath string) (string, error) {
	res, err := resolutionOf(ticketPath)
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(res, "\n") {
		line = strings.TrimLeft(strings.TrimSpace(line), "-* ")
		if rest, ok := strings.CutPrefix(line, "Outcome:"); ok {
			return strings.Trim(strings.TrimSpace(rest), "`* "), nil
		}
	}
	return "", nil
}

// startCIFix forks a ci-fix ticket for the first failed check of the open PR's
// head that has none yet, and queues the epic so it is claimed. Pending CI,
// cancelled, stale and action-required checks, and a head the failure is not
// on, start nothing: the view's failed set is already per head.
func (w *Watcher) startCIFix(r pr.Runner, repo string, t Target, v View) error {
	if v.State != "OPEN" {
		return nil
	}
	if stop, err := w.ciGate(t, v); stop || err != nil {
		return err
	}
	entries, err := sinceReset(t.Dir)
	if err != nil {
		return err
	}
	if err := w.settleReruns(t, v, entries); err != nil {
		return err
	}
	if v.CI != pr.ChecksFailed {
		return nil
	}
	perCheck, perPR := w.host.CIFixCaps()
	for _, c := range v.FailedChecks {
		key := CheckKeyString(c.Key())
		if hasEntry(entries, KindCIFailed, v.HeadSHA, key) {
			continue
		}
		if c.RunID != "" && !hasEntry(entries, KindReranCheck, v.HeadSHA, key) {
			// A failed job is rerun once before a ticket starts, and only
			// when its whole workflow run has finished.
			if !runCompleted(v.HeadChecks, c.RunID) {
				continue
			}
			return w.rerun(r, repo, t, v, c, key)
		}
		id, res, err := ForkCIFix(t.Dir, CIFixJob{
			Project: t.Project, Epic: t.Epic, Number: t.Number, URL: t.URL, HeadSHA: v.HeadSHA, Check: c,
			Skills: w.host.CIFixSkills(t.Project), PerCheck: perCheck, PerPR: perPR,
		})
		if res == CIFixStarted {
			// Queue even when the activity entry failed (err set): an unqueued
			// ticket would be orphaned, since it already blocks a second fork.
			qerr := w.host.QueueRoot(queue.RootRef{Project: t.Project, Epic: t.Epic}, id)
			return errors.Join(err, qerr)
		}
		if err != nil {
			return err
		}
		if res == CIFixBusy {
			return nil
		}
		if res == CIFixCapReached {
			return w.giveUp(t, capReason(t.Dir, c, perCheck))
		}
	}
	return nil
}
