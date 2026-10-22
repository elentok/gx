package tickets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/elentok/gx/tickets/schema"
)

const (
	handAnsweredNote = "answered by hand before the ticket log existed"
	legacyAnswerName = "Needs Answer"
)

// MigrateQuestionSections turns each live "## Needs Answer" section of a ticket
// under root, with its "## Handoff", into one pause.question in the ticket's
// log, and retires both sections into "## Comments". A ticket paused for an
// answer gets just the question; an open one was answered by hand and also
// gets a pause.answer, so it shows no open question. Tickets in any other
// status are left alone.
//
// Run after Migrate, which maps the legacy pause statuses. Retiring the
// sections is what makes a second run a no-op.
func MigrateQuestionSections(root string, now time.Time) ([]MigrationChange, error) {
	epics, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("reading tracker root %s: %w", root, err)
	}
	var changes []MigrationChange
	for _, epic := range epics {
		if !epic.IsDir() || strings.HasPrefix(epic.Name(), ".") {
			continue
		}
		issuesDir := filepath.Join(root, epic.Name(), "issues")
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
			migrated, err := migrateQuestionSection(path, now)
			if err != nil {
				return changes, fmt.Errorf("%s: %w", path, err)
			}
			if migrated {
				changes = append(changes, MigrationChange{Path: path, Notes: []string{"Needs Answer section moved to the ticket log"}})
			}
		}
	}
	return changes, nil
}

func migrateQuestionSection(path string, now time.Time) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	t, err := schema.ParseTicketFromRaw(string(raw), path)
	if err != nil {
		return false, err
	}
	body := schema.ParseBody(string(raw))
	text := schema.Section(body, legacyAnswerName)
	if text == "" || (t.PausedClass() != schema.PauseClassAnswer && t.Status != schema.StatusOpen) {
		return false, nil
	}

	// The read shim clears the old kind field; the old spelling comes from the reason.
	kind := schema.PauseKind(t.PauseReason.LegacyKind())
	if kind == "" {
		kind = schema.PauseKindZeroCommit
	}
	handoff := schema.Section(body, "Handoff")
	by := "gx"
	if handoff != "" {
		by = "agent"
	}
	summary, _, _ := strings.Cut(text, "\n")
	q, err := AppendLog(path, LogEvent{
		Type:      EventPauseQuestion,
		By:        by,
		PauseKind: string(kind),
		Summary:   clipRunes(strings.TrimSpace(summary), MaxLogSummaryChars),
		Text:      clipRunes(text, MaxLogTextChars),
		Handoff:   clipRunes(handoff, MaxLogTextChars),
	})
	if err != nil {
		return false, err
	}
	if t.Status == schema.StatusOpen {
		_, err = AppendLog(path, LogEvent{Type: EventPauseAnswer, Question: q.Seq, Source: "cli", Note: handAnsweredNote})
		if err != nil {
			return false, err
		}
	}

	err = schema.UpdateTicketWithBody(path, func(_ *schema.Ticket, body *string) {
		*body = schema.DemoteSection(*body, LegacyAnswerHeading, now)
		*body = schema.DemoteSection(*body, "## Handoff", now)
	})
	return err == nil, err
}

// clipRunes cuts s to at most n runes.
func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
