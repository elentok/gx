package evolve

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/conversations"
)

// mapFixture is a map whose tickets each carry one condition.
func mapFixture(t *testing.T) *evolveFixture {
	t.Helper()
	f := newEvolveFixture(t, config.EvolveContributor, false, "kind: map\n")
	issues := filepath.Join(f.epic, "issues")
	write := func(name, status, body string) {
		t.Helper()
		fm := "---\nid: \"" + name[:2] + "\"\nstatus: " + status + "\ntype: research\n---\n" + body
		if err := os.WriteFile(filepath.Join(issues, name), []byte(fm), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("01-a.md", "done", "Body.\n")
	write("02-spec-round-1.md", "done", "Body.\n")
	write("03-spec-round-2.md", "done", "Body.\n\n## Re-review findings\n\n- fix it (stories/scope changed)\n")
	write("04-many.md", "done", "Body.\n")
	write("05-huh.md", "done", "Body.\n")
	write("06-quiet.md", "done", "Body.\n")
	return f
}

func saveConv(t *testing.T, dir, id, ticket string, grilling []conversations.GrillingItem, huh ...string) {
	t.Helper()
	rec := conversations.Record{ID: id, Ticket: ticket, Grilling: grilling}
	if err := conversations.Save(dir, rec); err != nil {
		t.Fatal(err)
	}
	var items []conversations.HuhItem
	for i, q := range huh {
		items = append(items, conversations.HuhItem{Question: q, MessageID: id + string(rune('a'+i)), At: time.Now()})
	}
	if len(items) > 0 {
		if err := conversations.AppendHuh(dir, id, items); err != nil {
			t.Fatal(err)
		}
	}
}

func rounds(n int) []conversations.GrillingItem {
	var items []conversations.GrillingItem
	for i := range n {
		items = append(items, conversations.GrillingItem{MessageID: "m" + string(rune('a'+i)), Questions: 2, Words: 10})
	}
	return items
}

func runMapInputs(t *testing.T, f *evolveFixture) InputsTable {
	t.Helper()
	var out bytes.Buffer
	if err := runEvolveInputs(f.env, &out, "proj:epic", true); err != nil {
		t.Fatal(err)
	}
	var table InputsTable
	if err := json.Unmarshal(out.Bytes(), &table); err != nil {
		t.Fatal(err)
	}
	return table
}

func TestEvolveInputs_MapRowsAndFlags(t *testing.T) {
	t.Parallel()
	f := mapFixture(t)
	f.env.convDir = t.TempDir()
	saveConv(t, f.env.convDir, "c4", "proj:epic/04", rounds(3))
	saveConv(t, f.env.convDir, "c5", "proj:epic/05", rounds(1), "q1?", "q2?")
	saveConv(t, f.env.convDir, "c6", "proj:epic/06", rounds(2), "q3?")

	table := runMapInputs(t, f)
	if len(table.Rows) != 6 {
		t.Fatalf("rows = %d, want 6", len(table.Rows))
	}
	rows := map[string]InputsRow{}
	for _, r := range table.Rows {
		rows[r.ID] = r
	}
	want := map[string][]string{
		"01": {},
		"02": {"spec-round-cap"},
		"03": {"spec-round-cap", "re-review", "scope-changed"},
		"04": {"many-rounds"},
		"05": {"huh-cluster"},
		"06": {"huh-cluster"}, // below the per-ticket count, but 3 across the map
	}
	for id, flags := range want {
		if got := rows[id].Flags; !reflect.DeepEqual(got, flags) {
			t.Errorf("ticket %s flags = %v, want %v", id, got, flags)
		}
	}
	if r := rows["04"]; r.GrillingRounds != 3 || r.Words != 30 {
		t.Errorf("ticket 04 rounds/words = %d/%d, want 3/30", r.GrillingRounds, r.Words)
	}
	if len(table.HuhClusters) != 2 {
		t.Fatalf("huh clusters = %+v, want 2", table.HuhClusters)
	}
	if c := table.HuhClusters[0]; c.Ticket != "05" || len(c.Questions) != 2 || c.Questions[0].Question != "q1?" {
		t.Errorf("first cluster = %+v", c)
	}
}

func TestEvolveInputs_MapBelowThresholdsNoFlags(t *testing.T) {
	t.Parallel()
	f := mapFixture(t)
	for _, name := range []string{"02-spec-round-1.md", "03-spec-round-2.md"} {
		if err := os.Remove(filepath.Join(f.epic, "issues", name)); err != nil {
			t.Fatal(err)
		}
	}
	f.env.convDir = t.TempDir()
	saveConv(t, f.env.convDir, "c4", "proj:epic/04", rounds(2), "only one?")
	saveConv(t, f.env.convDir, "c5", "proj:epic/05", rounds(1), "another?")

	for _, r := range runMapInputs(t, f).Rows {
		if len(r.Flags) != 0 {
			t.Errorf("ticket %s flags = %v, want none", r.ID, r.Flags)
		}
	}
}

func TestEvolveInputs_MapHasNoQueueFlags(t *testing.T) {
	t.Parallel()
	f := mapFixture(t) // no run log, no metrics: done tickets must not read metrics-missing
	for _, r := range runMapInputs(t, f).Rows {
		for _, fl := range r.Flags {
			if fl == "metrics-missing" {
				t.Errorf("ticket %s carries a queue flag: %v", r.ID, r.Flags)
			}
		}
	}
}
