package tickets

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/tickets/schema"
)

// PauseStep is one stored needs-answer / needs-repair ticket and the paused
// form it migrates to.
type PauseStep struct {
	Path   string
	Reason schema.PauseReason
	// Mismatch says the recorded reason disagreed with the stored status and
	// was replaced by the status class's default. Empty when they agreed.
	Mismatch string
}

// Note is the step as a MigrationChange note.
func (s PauseStep) Note() string {
	note := fmt.Sprintf("status -> paused (pause_reason: %s)", s.Reason)
	if s.Mismatch != "" {
		note += "; " + s.Mismatch
	}
	return note
}

// PlanPauseMigration chooses the pause reason of every needs-answer and
// needs-repair ticket under root, without writing. Map epics are skipped, and
// so is a file that does not parse (Migrate reports it).
//
// Plan before Migrate and the park-section migration run: both rewrite through
// the read shim, which replaces a repair ticket's missing reason with the
// default and so loses the one the event log holds.
func PlanPauseMigration(root string) ([]PauseStep, error) {
	epics, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("reading tracker root %s: %w", root, err)
	}
	var steps []PauseStep
	for _, epic := range epics {
		epicPath := filepath.Join(root, epic.Name())
		if !epic.IsDir() || strings.HasPrefix(epic.Name(), ".") || IsMapEpic(epicPath) {
			continue
		}
		issuesDir := filepath.Join(epicPath, "issues")
		issues, err := os.ReadDir(issuesDir)
		if err != nil {
			continue
		}
		for _, issue := range issues {
			if issue.IsDir() {
				continue
			}
			if _, _, _, ok := parseTicketFilename(issue.Name()); !ok {
				continue
			}
			path := filepath.Join(issuesDir, issue.Name())
			step, ok, err := planPauseStep(path)
			if err != nil {
				return nil, err
			}
			if ok {
				steps = append(steps, step)
			}
		}
	}
	return steps, nil
}

func planPauseStep(path string) (PauseStep, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return PauseStep{}, false, fmt.Errorf("reading %s: %w", path, err)
	}
	parsed, err := schema.ParseTicketRaw(string(raw), path)
	if err != nil {
		return PauseStep{}, false, nil
	}
	t := parsed.Ticket
	var class schema.PauseClass
	switch migratedStatusOf(t.Status) {
	case schema.StatusNeedsAnswer:
		class = schema.PauseClassAnswer
	case schema.StatusNeedsRepair:
		class = schema.PauseClassRepair
	default:
		return PauseStep{}, false, nil
	}

	step := PauseStep{Path: path}
	recorded := t.PauseReason
	if recorded == "" {
		recorded = schema.ParsePauseReason(string(t.ParkKind))
	}
	switch {
	case recorded == "" && class == schema.PauseClassRepair:
		step.Reason, err = latestLoggedReason(path, class)
		if err != nil {
			return PauseStep{}, false, err
		}
	case recorded == "" || recorded.Class() == class:
		step.Reason = schema.PausedReason(string(recorded), class)
	default:
		step.Reason = class.DefaultReason()
		step.Mismatch = fmt.Sprintf("recorded reason %q does not fit status %s, used %s", recorded, t.Status, step.Reason)
	}
	return step, true, nil
}

// latestLoggedReason is the reason of the latest pause event in the ticket's
// log, or class's default when the log has none that fits.
func latestLoggedReason(path string, class schema.PauseClass) (schema.PauseReason, error) {
	events, err := ReadLog(path)
	if err != nil {
		return "", fmt.Errorf("reading log of %s: %w", path, err)
	}
	for _, ev := range slices.Backward(events) {
		if ev.Type == EventParkQuestion {
			return schema.PausedReason(ev.ParkKind, class), nil
		}
	}
	return class.DefaultReason(), nil
}

// ApplyPauseMigration writes each planned step: status paused, the chosen
// reason, no park_kind, and the old body headings renamed to "## Paused". Run
// it after MigrateParkSections, which still looks for "## Needs Answer".
func ApplyPauseMigration(steps []PauseStep) ([]MigrationChange, error) {
	var changes []MigrationChange
	for _, s := range steps {
		err := schema.UpdateTicketWithBody(s.Path, func(t *schema.Ticket, body *string) {
			t.Status = schema.StatusPaused
			t.PauseReason = s.Reason
			t.ParkKind = ""
			*body = renamePauseHeadings(*body)
		})
		if err != nil {
			return changes, fmt.Errorf("%s: %w", s.Path, err)
		}
		changes = append(changes, MigrationChange{Path: s.Path, Notes: []string{s.Note()}})
	}
	return changes, nil
}

func renamePauseHeadings(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if line == NeedsAnswerHeading || line == NeedsRepairHeading {
			lines[i] = schema.PausedHeading
		}
	}
	return strings.Join(lines, "\n")
}
