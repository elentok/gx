package tickets

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/tickets/schema"
)

func TestReset_ParkedTicketReturnsToFreshOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "05-thing.md")
	writeFile(t, path, `---
id: "05"
status: needs-answer
blocked_by: ["03"]
type: task
expected_context_window: 25000
actual_context_window: 90000
elapsed_time: 120
actual_cost: 1.5
compactions: 2
session_ids: [abc, def]
commitless: true
iteration_status: needs-answer
park_kind: self-reported
---

# 05 — Thing

Human prose.

## Needs Answer

Which db?

## Comments

Earlier note.
`)

	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	if err := Reset(path, now); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := schema.ParseTicketFromRaw(string(raw), path)
	if err != nil {
		t.Fatal(err)
	}
	want := schema.Ticket{
		ID:                    "05",
		Status:                schema.StatusOpen,
		BlockedBy:             []schema.TicketID{"03"},
		Type:                  schema.TypeTask,
		ExpectedContextWindow: 25000,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("frontmatter after reset:\n got %+v\nwant %+v", got, want)
	}

	body := schema.ParseBody(string(raw))
	if strings.Contains(body, "\n## Needs Answer") {
		t.Errorf("Needs Answer section not retired:\n%s", body)
	}
	for _, s := range []string{"# 05 — Thing", "Human prose.", "Earlier note.", "retired from `## Needs Answer`", "Which db?"} {
		if !strings.Contains(body, s) {
			t.Errorf("body lost %q:\n%s", s, body)
		}
	}
}

func TestReset_RetiresNeedsRepairSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "05-thing.md")
	writeFile(t, path, "---\nid: \"05\"\nstatus: needs-repair\ntype: task\n---\n\n## Needs Repair\n\nBroke.\n")

	if err := Reset(path, time.Now()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	raw, _ := os.ReadFile(path)
	body := schema.ParseBody(string(raw))
	if strings.Contains(body, "\n## Needs Repair") || !strings.Contains(body, "Broke.") {
		t.Fatalf("unexpected body:\n%s", body)
	}
}
