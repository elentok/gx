package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
)

func inputsFixture(t *testing.T) *evolveFixture {
	t.Helper()
	f := newEvolveFixture(t, config.EvolveContributor, false, "")
	issues := filepath.Join(f.epic, "issues")
	write := func(name, fm string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(issues, name), []byte("---\n"+fm+"---\nBody.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// 01 (from the base fixture) has no metrics and is the slow-baseline-free one.
	write("01-a.md", "id: \"01\"\nstatus: done\ntype: implement\nexpected_context_window: 40000\nactual_context_window: 90000\nelapsed_time: 100\nactual_cost: 1.5\nsession_ids: [s1]\n")
	write("02-b.md", "id: \"02\"\nstatus: done\ntype: implement\nexpected_context_window: 40000\nactual_context_window: 30000\nelapsed_time: 100\nactual_cost: 1\nsession_ids: [s2]\n")
	write("03-c.md", "id: \"03\"\nstatus: needs-repair\ntype: implement\nexpected_context_window: 40000\nactual_context_window: 30000\nelapsed_time: 900\nactual_cost: 2\nsession_ids: [s3]\n")
	write("04-d.md", "id: \"04\"\nstatus: done\ntype: implement\nactual_context_window: 1\nelapsed_time: 100\nactual_cost: 1\n")
	write("05-review.md", "id: \"05\"\nstatus: done\ntype: code-review\nactual_context_window: 1\nelapsed_time: 100\nactual_cost: 1\n")
	write("06-fix.md", "id: \"06\"\nstatus: done\ntype: implement\nparent: \"05\"\nactual_context_window: 1\nelapsed_time: 100\nactual_cost: 1\n")
	write("07-nometrics.md", "id: \"07\"\nstatus: done\ntype: implement\n")

	scratch := filepath.Dir(f.epic)
	for _, ev := range []ralphloop.Event{
		{Type: "iteration-started", Ticket: "02", AgentSession: "s2"},
		{Type: "iteration-started", Ticket: "02", AgentSession: "s2b"},
		{Type: "nudged", Ticket: "02"},
		{Type: "conflict-hit", Ticket: "04", AgentSession: "land-1"},
		{Type: "conflict-resolved", Ticket: "04", AgentSession: "res-1"},
		{Type: "recovery-applied", Ticket: "07", Kind: "zero-commit"},
		{Type: "deadlocked", Kind: "all-parked"},
	} {
		if err := ralphloop.AppendEvent(scratch, "epic", ev); err != nil {
			t.Fatal(err)
		}
	}
	f.env.landed = func(string, string) (map[string]bool, error) { return map[string]bool{"01": true}, nil }
	return f
}

func TestEvolveInputs_FlagsAndSeverityOrder(t *testing.T) {
	t.Parallel()
	f := inputsFixture(t)
	var out bytes.Buffer
	if err := runEvolveInputs(f.env, &out, "proj:epic", true); err != nil {
		t.Fatal(err)
	}
	var table InputsTable
	if err := json.Unmarshal(out.Bytes(), &table); err != nil {
		t.Fatal(err)
	}
	rows := map[string]InputsRow{}
	var order []int
	for _, r := range table.Rows {
		rows[r.ID] = r
		order = append(order, r.Severity)
	}
	if len(table.Rows) != 7 {
		t.Fatalf("rows = %d, want 7", len(table.Rows))
	}
	if !slices.IsSortedFunc(order, func(a, b int) int { return b - a }) {
		t.Errorf("severities not descending: %v", order)
	}
	if table.Deadlocks != 1 {
		t.Errorf("deadlocks = %d", table.Deadlocks)
	}

	want := map[string][]string{
		"01": {"context-overrun"},
		"02": {"relaunch", "nudge"},
		"03": {"needs-repair", "slow"},
		"04": {"conflict-hit"},
		"05": {"has-fix-tickets"},
		"06": {},
		"07": {"recovery", "metrics-missing"},
	}
	for id, flags := range want {
		if got := rows[id].Flags; !reflect.DeepEqual(got, flags) {
			t.Errorf("ticket %s flags = %v, want %v", id, got, flags)
		}
	}
	for id, s := range map[string]int{"01": 1, "02": 0, "03": 3, "04": 2, "07": 3} {
		if rows[id].Severity != s {
			t.Errorf("ticket %s severity = %d, want %d", id, rows[id].Severity, s)
		}
	}
	if !rows["01"].Landed || rows["02"].Landed {
		t.Errorf("landed = %v/%v", rows["01"].Landed, rows["02"].Landed)
	}
	if got := rows["04"].ConflictSessions; !reflect.DeepEqual(got, []string{"land-1", "res-1"}) {
		t.Errorf("conflict sessions = %v", got)
	}
	if got := rows["02"].Sessions; !reflect.DeepEqual(got, []string{"s2", "s2b"}) {
		t.Errorf("sessions = %v", got)
	}
	if rows["06"].Parent != "05" || !reflect.DeepEqual(rows["05"].FixTickets, []string{"06"}) {
		t.Errorf("parent/fix edges wrong: %+v %+v", rows["06"], rows["05"])
	}
}

func TestEvolveInputs_TableOutput(t *testing.T) {
	t.Parallel()
	f := inputsFixture(t)
	var out bytes.Buffer
	if err := runEvolveInputs(f.env, &out, "proj:epic", false); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("context-overrun")) || !bytes.HasPrefix(out.Bytes(), []byte("ID")) {
		t.Errorf("unexpected table:\n%s", out.String())
	}
}
