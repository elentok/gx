package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/grillround"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

// designCommand opens the command message of a Design session. The migration
// matches Claude transcripts only.
var designCommand = harness.SkillCommand(string(harness.Claude), "gx-design", "")

// backfillGrilling counts the Grilling rounds of the ticket-store project at
// root (see grillround.Recount): the Design conversation records with no counts
// yet, and the record-less /gx-design transcripts of the project's trunk
// checkout that started before the conversations started marker (all of them
// without a marker). The latter get a record against "project:epic", whatever
// state the epic is in. With dryRun it only reports. A root that is not a store
// project has no conversations to count.
func backfillGrilling(dirs config.Dirs, root string, dryRun bool) ([]tickets.MigrationChange, error) {
	if _, err := config.ReadProjectFile(root); err != nil {
		return nil, nil
	}
	dir, err := conversations.DirIn(dirs)
	if err != nil {
		return nil, err
	}
	home, err := dirs.Home()
	if err != nil {
		return nil, err
	}
	recs, err := conversations.List(dir)
	if err != nil {
		return nil, err
	}
	project := tickets.ProjectName(root)
	var changes []tickets.MigrationChange
	known := map[string]bool{}
	for _, r := range recs {
		known[r.ID] = true
		if r.Grilling != nil || r.Harness != string(harness.Claude) || !designRecord(root, project, r) {
			continue
		}
		if _, err := os.Stat(transcript.PathIn(home, r.Dir, r.ID)); err != nil {
			continue
		}
		if !dryRun {
			if err := grillround.Recount(dir, grillround.Homes{Claude: home}, harness.Claude, r.ID, r.Dir); err != nil {
				return changes, err
			}
		}
		changes = append(changes, grillingChange(dir, r.ID))
	}

	store := filepath.Dir(root)
	trunkDir := tickets.TrunkCheckout(store, project)
	if trunkDir == "" {
		return changes, nil
	}
	started, hasMarker, err := conversations.Started(dir)
	if err != nil {
		return changes, err
	}
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(transcript.PathIn(home, trunkDir, "x")), "*.jsonl"))
	if err != nil {
		return changes, err
	}
	for _, path := range paths {
		id := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		if known[id] {
			continue
		}
		at, ok, err := transcript.FirstLineTimestamp(path)
		if err != nil || !ok || (hasMarker && !at.Before(started)) {
			continue
		}
		epic, err := designEpic(path)
		if err != nil || epic == "" {
			continue
		}
		if !dryRun {
			if err := saveBackfilledDesign(dir, home, trunkDir, id, project+":"+epic, at); err != nil {
				return changes, err
			}
		}
		changes = append(changes, grillingChange(dir, id))
	}
	return changes, nil
}

// designRecord reports whether r is a Design conversation of project: flagged
// so, or (for a record saved before the flag) linked to a ticket of a map epic.
func designRecord(root, project string, r conversations.Record) bool {
	p, rest, ok := strings.Cut(r.Ticket, ":")
	if !ok || p != project {
		return false
	}
	epic, _, _ := strings.Cut(rest, "/")
	return r.Design || (epic != "" && tickets.IsMapEpic(filepath.Join(root, epic)))
}

// designEpic is the epic named by the first /gx-design command of the
// transcript at path, or "" when it has none.
func designEpic(path string) (string, error) {
	msgs, _, err := transcript.ReadChat(path)
	for _, m := range msgs {
		if m.Kind != transcript.ChatKindCommand {
			continue
		}
		if f := strings.Fields(m.Text); len(f) >= 2 && f[0] == designCommand {
			return f[1], err
		}
	}
	return "", err
}

// saveBackfilledDesign writes the record of an old Design transcript and counts
// it. RecordedAt is the transcript's start, so a rerun writes the same bytes.
// Its totals are zero, not the transcript's: the transcript predates the
// started marker, so its spend must stay out of the token and cost sums of
// epics created after the marker. Nil totals would make those sums unknown.
func saveBackfilledDesign(dir, home, cwd, id, ticket string, at time.Time) error {
	rec := conversations.Record{ID: id, Dir: cwd, Harness: string(harness.Claude), Ticket: ticket, Design: true, RecordedAt: at, Totals: &conversations.Totals{}}
	if err := conversations.Save(dir, rec); err != nil {
		return err
	}
	return grillround.Recount(dir, grillround.Homes{Claude: home}, harness.Claude, rec.ID, rec.Dir)
}

func grillingChange(dir, id string) tickets.MigrationChange {
	return tickets.MigrationChange{Path: filepath.Join(dir, id+".json"), Notes: []string{"grilling counted"}}
}
