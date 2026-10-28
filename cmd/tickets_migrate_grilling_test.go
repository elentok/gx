package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

// grillingEnv is a store project "proj" with a trunk checkout and map epic
// "mapped", under an isolated HOME and data dir.
type grillingEnv struct {
	root, trunk, home, records string
	env                        []string
}

func newGrillingEnv(t *testing.T) grillingEnv {
	t.Helper()
	repoDir, _ := mergeTestRepo(t, nil)
	store := t.TempDir()
	env := tempEnv(t)
	home := envValue(env, "HOME")
	root := filepath.Join(store, "proj")
	if err := os.MkdirAll(filepath.Join(root, "mapped"), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, root, "project.json", `{"repo": "`+repoDir+`", "trunk": "main"}`)
	testutil.WriteFile(t, filepath.Join(root, "mapped"), "ticket.md", "---\nstatus: open\nkind: map\n---\n")
	records, err := conversations.DirIn(config.DirsFrom(env))
	if err != nil {
		t.Fatal(err)
	}
	return grillingEnv{root: root, trunk: tickets.TrunkCheckout(store, "proj"), home: home, records: records, env: env}
}

// writeGrillingTranscript writes a transcript of cwd that opens with the given
// /gx-design epic (none when empty) and holds one Grilling round.
func (e grillingEnv) writeGrillingTranscript(t *testing.T, cwd, id, epic string, at time.Time) {
	t.Helper()
	path := transcript.PathIn(e.home, cwd, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var lines [][]byte
	add := func(typ, text string) {
		message := map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}
		if typ == "assistant" {
			// Non-zero usage, so a backfilled record that joins the sums shows.
			message["model"] = "claude-sonnet-4-5"
			message["usage"] = map[string]any{"input_tokens": 1000, "output_tokens": 500}
		}
		line, _ := json.Marshal(map[string]any{
			"type": typ, "uuid": id + typ, "timestamp": at.Format(time.RFC3339),
			"message": message,
		})
		lines = append(lines, line, []byte("\n"))
	}
	if epic != "" {
		add("user", "<command-name>/gx-design</command-name>\n<command-args>"+epic+"</command-args>")
	}
	add("assistant", "❓ **Q1** - **Alpha**: one two")
	if err := os.WriteFile(path, bytes.Join(lines, nil), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (e grillingEnv) record(t *testing.T, id string) conversations.Record {
	t.Helper()
	r, err := conversations.Read(e.records, id)
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	return r
}

func (e grillingEnv) snapshot(t *testing.T) map[string]string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(e.records, "*"))
	out := map[string]string{}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		out[filepath.Base(f)] = string(b)
	}
	return out
}

func migrateGrilling(t *testing.T, e grillingEnv, args ...string) {
	t.Helper()
	if err := execute(append([]string{"tickets", "migrate", e.root}, args...), deps{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, env: e.env}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func TestTicketsMigrate_GrillingBackfill(t *testing.T) {
	e := newGrillingEnv(t)
	marker := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	before, after := marker.Add(-24*time.Hour), marker.Add(24*time.Hour)
	if err := conversations.MarkStarted(e.records, marker); err != nil {
		t.Fatal(err)
	}
	e.writeGrillingTranscript(t, e.trunk, "before", "alpha", before)
	e.writeGrillingTranscript(t, e.trunk, "after", "alpha", after)
	e.writeGrillingTranscript(t, e.trunk, "ghost", "no-such-epic", before)
	e.writeGrillingTranscript(t, e.trunk, "plain", "", before)
	e.writeGrillingTranscript(t, "/work/nocount", "nocount", "", before)
	for _, r := range []conversations.Record{
		{ID: "nocount", Dir: "/work/nocount", Harness: "claude", Ticket: "proj:mapped/01", Totals: &conversations.Totals{TotalTokens: 10, Cost: 0.5}},
		{ID: "notranscript", Dir: "/work/gone", Harness: "claude", Ticket: "proj:mapped/01", Design: true, Totals: &conversations.Totals{TotalTokens: 1}},
		{ID: "codex", Dir: "/work/nocount", Harness: "codex", Ticket: "proj:mapped/01", Design: true, Totals: &conversations.Totals{TotalTokens: 2}},
	} {
		if err := conversations.Save(e.records, r); err != nil {
			t.Fatal(err)
		}
	}
	e.writeGrillingTranscript(t, "/work/nocount", "codex", "", before)
	createdAt := after.Add(time.Hour)
	sums := func() [3]conversations.Totals {
		var out [3]conversations.Totals
		for i, epic := range []string{"proj:mapped", "proj:alpha", "proj:no-such-epic"} {
			sum, ok, err := conversations.SumEpicTotals(e.records, epic, createdAt)
			if err != nil || !ok {
				t.Fatalf("sum %s = %v, %v", epic, ok, err)
			}
			out[i] = sum
		}
		return out
	}
	sumsBefore, snapBefore := sums(), e.snapshot(t)

	migrateGrilling(t, e, "--dry-run")
	if got := e.snapshot(t); len(got) != len(snapBefore) {
		t.Fatalf("dry run changed the records: %v", got)
	}
	for id, data := range snapBefore {
		if e.snapshot(t)[id] != data {
			t.Errorf("dry run changed %s", id)
		}
	}

	migrateGrilling(t, e)
	for _, id := range []string{"nocount", "before", "ghost"} {
		if got := e.record(t, id).Grilling; len(got) != 1 || got[0].Questions != 1 {
			t.Errorf("%s grilling = %+v, want one round", id, got)
		}
	}
	for _, id := range []string{"notranscript", "codex"} {
		if got := e.record(t, id).Grilling; got != nil {
			t.Errorf("%s grilling = %+v, want uncounted", id, got)
		}
	}
	snap := e.snapshot(t)
	for _, id := range []string{"after", "plain"} {
		if _, ok := snap[id+".json"]; ok {
			t.Errorf("%s got a record", id)
		}
	}
	if r := e.record(t, "ghost"); r.Ticket != "proj:no-such-epic" || !r.Design || r.Harness != "claude" {
		t.Errorf("ghost record = %+v", r)
	}
	if got := sums(); got != sumsBefore {
		t.Errorf("sums = %+v, want %+v", got, sumsBefore)
	}

	migrateGrilling(t, e)
	for id, data := range snap {
		if again := e.snapshot(t)[id]; again != data {
			t.Errorf("%s changed on rerun:\n%s\n%s", id, data, again)
		}
	}
}

func TestTicketsMigrate_GrillingBackfillWithoutMarkerCountsEveryDesignTranscript(t *testing.T) {
	e := newGrillingEnv(t)
	at := time.Now().Add(-time.Hour)
	e.writeGrillingTranscript(t, e.trunk, "one", "alpha", at)
	e.writeGrillingTranscript(t, e.trunk, "two", "beta", at)
	e.writeGrillingTranscript(t, e.trunk, "plain", "", at)

	migrateGrilling(t, e)
	for id, ticket := range map[string]string{"one": "proj:alpha", "two": "proj:beta"} {
		r := e.record(t, id)
		if r.Ticket != ticket || len(r.Grilling) != 1 {
			t.Errorf("%s = %+v, want %s with one round", id, r, ticket)
		}
	}
	if _, err := conversations.Read(e.records, "plain"); err == nil {
		t.Error("plain transcript got a record")
	}
}
