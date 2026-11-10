package prflow

import (
	"sort"
	"strings"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// entryWords is how each activity kind reads on the page and which dot it gets.
var entryWords = map[Kind]struct {
	text string
	kind wire.PRActivityKind
}{
	KindPROpened:        {"Draft PR opened", wire.PRActivityInfo},
	KindPRAdopted:       {"Existing PR adopted", wire.PRActivityInfo},
	KindMarkedReady:     {"Marked ready for review", wire.PRActivityGood},
	KindReranCheck:      {"Check re-run", wire.PRActivityInfo},
	KindCIFailed:        {"CI failed", wire.PRActivityBad},
	KindFlaky:           {"Marked flaky", wire.PRActivityWarning},
	KindPausedCIFailing: {"Paused: CI failing", wire.PRActivityBad},
	KindTryAgainReset:   {"Try again", wire.PRActivityInfo},
	KindPRMerged:        {"PR merged", wire.PRActivityGood},
	KindTrunkUpdated:    {"Local trunk updated", wire.PRActivityGood},
	KindPRClosed:        {"PR closed without merging", wire.PRActivityWarning},
	KindGitHubDown:      {"GitHub unreachable", wire.PRActivityWarning},
	KindGitHubUp:        {"GitHub reachable again", wire.PRActivityInfo},
	KindMarkedMerged:    {"Marked as merged", wire.PRActivityGood},
}

// List is the epic's PR activity, newest first: the entries of pr-activity.jsonl
// and the entries read from the Resolutions of its pr-text and ci-fix tickets.
// project names the ticket addresses the fix entries link to.
func List(project string, e tickets.Epic) []wire.PRActivityEntry {
	var out []wire.PRActivityEntry
	entries, _ := Read(e.Path)
	for _, en := range entries {
		w, ok := entryWords[en.Kind]
		if !ok {
			w.text, w.kind = string(en.Kind), wire.PRActivityInfo
		}
		if en.Title != "" {
			w.text = en.Title
		}
		detail := en.Detail
		if detail == "" && len(en.Checks) > 0 {
			detail = strings.Join(en.Checks, ", ")
		}
		out = append(out, wire.PRActivityEntry{Time: en.Time, Kind: w.kind, Text: w.text, Detail: detail})
	}
	for _, t := range e.Tickets {
		if t.Status != string(schema.StatusDone) {
			continue
		}
		addr := tickets.Address{Project: project, Epic: e.Name, ID: t.DisplayNumber()}.String()
		res := schema.Section(t.Body, "Resolution")
		switch t.Type {
		case string(schema.TypeCIFix):
			if line(res, "Outcome") != OutcomeFixed {
				continue
			}
			detail := "Cause: " + line(res, "Cause")
			if c := line(res, "Commit"); c != "" {
				detail += " · Commit: " + c
			}
			out = append(out, wire.PRActivityEntry{Time: t.DoneAt, Kind: wire.PRActivityGood, Text: "Fix pushed", Detail: detail, Ticket: addr})
		case string(schema.TypePRText):
			out = append(out, wire.PRActivityEntry{Time: t.DoneAt, Kind: wire.PRActivityInfo, Text: "PR text written", Detail: line(res, "Title"), Ticket: addr})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out
}

// line is the value of a `Key:` line of a Resolution, "" without one.
func line(res, key string) string {
	for l := range strings.SplitSeq(res, "\n") {
		l = strings.TrimLeft(strings.TrimSpace(l), "-* ")
		if rest, ok := strings.CutPrefix(l, key+":"); ok {
			return strings.Trim(strings.TrimSpace(rest), "`* ")
		}
	}
	return ""
}
