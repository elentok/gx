package tickets

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestUpdateEpicFields_CompletedAtWritesWithOtherFieldsAndKeepsFirst(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	path := writeEpic(t, t.TempDir(), "m", "kind: map\nstatus: open\n", "# m\n")
	first := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	draft := "draft"

	if err := UpdateEpicFields(path, EpicUpdate{Status: &draft, CompletedAt: &first}); err != nil {
		t.Fatal(err)
	}
	if err := UpdateEpicFields(path, EpicUpdate{CompletedAt: &[]time.Time{first.AddDate(1, 0, 0)}[0]}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{"status: draft", "completed_at: 2026-01-02T03:04:05Z"} {
		if !strings.Contains(got, want) {
			t.Errorf("ticket.md missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "2027") {
		t.Errorf("second completion overwrote the first:\n%s", got)
	}
}

func TestUpdateEpicFields_DesignStepChecksTheKindTheCallLeavesBehind(t *testing.T) {
	step := string(DesignSteps[0])
	mapKind, loopKind := KindMap, "loop"

	loop := writeEpic(t, t.TempDir(), "l", "kind: loop\nstatus: open\n", "# l\n")
	if err := UpdateEpicFields(loop, EpicUpdate{Kind: &mapKind, DesignStep: &step}); err != nil {
		t.Errorf("loop epic turned into a map in the same call: %v", err)
	}

	m := writeEpic(t, t.TempDir(), "m", "kind: map\nstatus: open\n", "# m\n")
	before, _ := os.ReadFile(m)
	if err := UpdateEpicFields(m, EpicUpdate{Kind: &loopKind, DesignStep: &step}); err == nil {
		t.Error("want an error for a map epic turned into a loop in the same call")
	}
	if after, _ := os.ReadFile(m); string(after) != string(before) {
		t.Errorf("a refused update changed the file:\n%s", after)
	}
}

func TestUpdateEpicFields_CompletedAtRefusesLoopEpicBeforeWriting(t *testing.T) {
	path := writeEpic(t, t.TempDir(), "l", "kind: loop\nstatus: open\n", "# l\n")
	before, _ := os.ReadFile(path)
	at := time.Now().UTC()
	draft := "draft"

	if err := UpdateEpicFields(path, EpicUpdate{Status: &draft, CompletedAt: &at}); err == nil {
		t.Fatal("want an error for a loop epic")
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Errorf("a refused update changed the file:\n%s", after)
	}
}
