package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/server/servertest"
)

// unitedEpics is the united scenario's epics, grouped by section.
func unitedEpics() []epic {
	open := []ticket{{"01", "first", servertest.TicketOpts{}}}
	done := []ticket{{"01", "built", servertest.TicketOpts{Status: "done"}}}
	return []epic{
		// Needs you.
		{project, "paused-epic", []ticket{{"01", "repair", servertest.TicketOpts{PauseReason: "iteration-error"}}}},
		{project, "question-epic", []ticket{{"01", "answer", servertest.TicketOpts{PauseReason: "self-reported"}}}},
		{project, "perm-epic", open},
		// Its epic blocker is unmerged, so the ticket is blocked and nothing can
		// run: the epic is deadlocked. A cycle would not do, since recovery pauses it.
		{project, "deadlock-epic", []ticket{{"01", "first", servertest.TicketOpts{ClaimedAt: "2026-01-01T10:00:00Z"}}}},
		{project, "blocked-merge-epic", done},
		{project, "awaiting-epic", done},
		{project, "dirty-trunk-epic", done},
		{project, "design-question", nil},
		{project, "design-stopped", nil},
		// Implementing and Designing.
		{project, "running-epic", open},
		{project, "queued-epic", open},
		{"auto", "awaiting-auto-epic", done},
		{project, "design-live", nil},
		// Ready to queue.
		{project, "ready-epic", open},
		// Ready to design.
		{project, "follow-ups", []ticket{
			{"01", "a-bug", servertest.TicketOpts{Category: "bug"}},
			{"02", "an-idea", servertest.TicketOpts{Category: "idea"}},
		}},
		{project, "design-new", nil},
		{project, "draft-epic", open},
		// A draft -impl epic: its tickets are still being published.
		{project, "publishing-epic-impl", open},
		{project, "idle-epic", []ticket{{"01", "stuck", servertest.TicketOpts{Status: "claimed"}}}},
		// Finished: more than recentlyFinished, one-offs included.
		{project, "finished-a", done},
		{project, "finished-b", done},
		{"auto", "finished-c", done},
		// One-offs: queued, paused and finished.
		{project, "one-offs", []ticket{
			{"01", "paused", servertest.TicketOpts{PauseReason: "iteration-error"}},
			{"02", "queued", servertest.TicketOpts{}},
			{"03", "done-a", servertest.TicketOpts{Status: "done"}},
			{"04", "done-b", servertest.TicketOpts{Status: "done"}},
			{"05", "done-c", servertest.TicketOpts{Status: "done"}},
		}},
	}
}

// unitedMeta is the epic ticket.md frontmatter the united scenario overrides.
func unitedMeta() map[string]string {
	merged := func(day string) string {
		return "status: open\ncompleted_at: 2026-01-" + day + "T10:00:00Z\nmerged_at: 2026-01-" + day + "T11:00:00Z\n"
	}
	const (
		oldCreated = "created_at: 2026-01-01T09:00:00Z\n"
		doneStamp  = "completed_at: 2026-01-01T10:00:00Z\n"
	)
	meta := map[string]string{
		project + ":blocked-merge-epic":   "status: open\nland_block_reason: needs a rebase\n" + doneStamp,
		project + ":dirty-trunk-epic":     dirtyTrunkMeta("a.txt", "b.txt", "c.txt", "d.txt", "e.txt", "f.txt", "g.txt") + doneStamp,
		project + ":awaiting-epic":        "status: open\n" + doneStamp,
		project + ":deadlock-epic":        "status: open\nblocked_by: [\"ready-epic\"]\n",
		project + ":draft-epic":           "status: draft\n",
		project + ":publishing-epic-impl": "status: draft\n",
		project + ":finished-a":           merged("01"),
		project + ":finished-b":           merged("02"),
		"auto:finished-c":                 merged("03"),
		project + ":one-offs":             "status: draft\n",
		project + ":running-epic":         "status: open\nstarted_at: 2026-01-01T10:00:00Z\n",
		"auto:awaiting-auto-epic":         "status: open\nstarted_at: 2026-01-01T09:30:00Z\n" + doneStamp,
		// The maps keep their kind, which this override replaces. design-live's
		// stamp comes from its opened session; design-stopped has none, so it
		// gets one here.
		project + ":design-question": "status: open\nkind: map\n",
		project + ":design-live":     "status: open\nkind: map\n",
		project + ":design-stopped":  "status: open\nkind: map\ndesign_started_at: 2026-01-01T09:30:00Z\n",
		project + ":design-new":      "status: open\nkind: map\n",
	}
	// Every epic is created; ready-epic minutes ago, relative to fixture start.
	recent := "created_at: " + time.Now().Add(-5*time.Minute).UTC().Format(time.RFC3339) + "\n"
	for _, e := range unitedEpics() {
		addr := e.project + ":" + e.name
		created := oldCreated
		if addr == project+":ready-epic" {
			created = recent
		}
		if _, ok := meta[addr]; !ok {
			meta[addr] = "status: open\n"
		}
		meta[addr] += created
	}
	return meta
}

// dirtyTrunkMeta is the epic frontmatter of a merge paused on a dirty trunk,
// with the full clashing file list stored as the server stores it.
func dirtyTrunkMeta(files ...string) string {
	reason := fmt.Sprintf("uncommitted changes on main clash with %s; commit or discard them, then merge again", strings.Join(files[:min(len(files), 5)], ", "))
	if extra := len(files) - 5; extra > 0 {
		reason += fmt.Sprintf(" (+%d more)", extra)
	}
	return "status: open\nland_block_kind: dirty-trunk\nland_block_reason: " + strconv.Quote(reason) +
		"\nland_block_files: [" + strings.Join(files, ", ") + "]\n"
}

// commitOnBranch creates branch off main with one committed file and returns to main.
func commitOnBranch(f *fixture, repo, branch, file, content string) error {
	if err := f.git(repo, "checkout", "-b", branch); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(repo, file), []byte(content), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"add", file}, {"commit", "-m", branch}, {"checkout", "main"}} {
		if err := f.git(repo, args...); err != nil {
			return err
		}
	}
	return nil
}
