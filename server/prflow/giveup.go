package prflow

import (
	"fmt"

	"github.com/elentok/gx/git/pr"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// ciGate runs before a poll starts any CI fix. stop is true while "CI failing"
// is stored and the head is still red, and when a finished ci-fix ticket just
// reported the failure unfixable; either way no ticket starts. A fully green
// head clears a stored "CI failing" by itself: the person fixed it by hand.
func (w *Watcher) ciGate(t Target, v View) (stop bool, err error) {
	if tickets.LoadEpicAt(t.Dir).MergeBlockKind == string(schema.PauseCIFailing) {
		if v.CI != pr.ChecksGreen {
			return true, nil
		}
		return false, ResetBlock(t.Dir, "CI green")
	}
	reason, err := unfixableReason(t.Dir)
	if err != nil || reason == "" {
		return false, err
	}
	return true, w.giveUp(t, giveUpNote{Reason: reason, Detail: reason})
}

// giveUpNote is the words of one give-up: Reason is the epic's pause notice,
// Title and Detail the activity entry (an empty Title keeps the kind's words).
type giveUpNote struct{ Reason, Title, Detail string }

// giveUp stores "CI failing" and sends its one notice, then logs it. Later polls
// see the stored block and do not come back here.
func (w *Watcher) giveUp(t Target, n giveUpNote) error {
	if err := w.host.PauseCIFailing(t, n.Reason); err != nil {
		return err
	}
	return Append(t.Dir, Entry{Kind: KindPausedCIFailing, Title: n.Title, Detail: n.Detail})
}

// ResetBlock clears the epic's stored pause and logs a Try again reset, so the
// attempt counts start from it. The detail says why it was reset.
func ResetBlock(epicDir, detail string) error {
	if err := Append(epicDir, Entry{Kind: KindTryAgainReset, Detail: detail}); err != nil {
		return err
	}
	return tickets.ClearEpicFields(epicDir, blockKindKey, blockReasonKey)
}

// capReason says which cap gave out: the check's own, else the PR's.
func capReason(epicDir string, c pr.Check, perCheck, perPR int) giveUpNote {
	key := CheckKeyString(c.Key())
	what, limit, unit, n := "", perCheck, "check", perCheck
	if byCheck, err := CIFailedByCheck(epicDir); err == nil && byCheck[key] >= perCheck {
		what = c.Name + " failed"
	} else {
		total, _ := CIFailedAttempts(epicDir)
		what, limit, unit, n = "CI failed", perPR, "pull request", total
	}
	return giveUpNote{
		Reason: fmt.Sprintf("CI failing: %s %d times (limit %d per %s). Fix it by hand and push, or press Try again to give the agent %d more attempts.", what, n, limit, unit, limit),
		Title:  fmt.Sprintf("Paused: %s %d times", what, n),
		Detail: fmt.Sprintf("Limit reached for this %s. Try again resets the count.", unit),
	}
}

// unfixableReason is why gx gives up at once: the newest ci-fix ticket finished
// since the last reset reported `Outcome: unfixable`. "" when it did not.
func unfixableReason(epicDir string) (string, error) {
	since, err := ResetTime(epicDir)
	if err != nil {
		return "", err
	}
	var newest tickets.Ticket
	for _, t := range tickets.LoadEpicAt(epicDir).Tickets {
		if t.Type != string(schema.TypeCIFix) || t.Status != string(schema.StatusDone) || t.Number < newest.Number {
			continue
		}
		finished := t.CreatedAt
		if st, err := schema.ParseTicket(t.Path); err == nil && !st.IterationFinishedAt.IsZero() {
			finished = st.IterationFinishedAt
		}
		if finished.After(since) {
			newest = t
		}
	}
	if newest.Path == "" {
		return "", nil
	}
	out, err := OutcomeOf(newest.Path)
	if err != nil || out != OutcomeUnfixable {
		return "", err
	}
	return fmt.Sprintf("The agent could not fix this failure (ticket %s).", newest.Identifier), nil
}
