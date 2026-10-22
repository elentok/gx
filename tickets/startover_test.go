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

func TestStartOver_PausedTicketReturnsToFreshOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "05-thing.md")
	writeFile(t, path, `---
id: "05"
status: paused
blocked_by: ["03"]
type: implement
expected_context_window: 25000
actual_context_window: 90000
elapsed_time: 120
actual_cost: 1.5
compactions: 2
session_ids: [abc, def]
commitless: true
iteration_status: asking
pause_reason: self-reported
---

# 05 — Thing

Human prose.

`+LegacyAnswerHeading+`

Which db?

## Comments

Earlier note.
`)

	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	if err := StartOver(path, now, ""); err != nil {
		t.Fatalf("StartOver: %v", err)
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
		Type:                  schema.TypeImplement,
		ExpectedContextWindow: 25000,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("frontmatter after start over:\n got %+v\nwant %+v", got, want)
	}

	body := schema.ParseBody(string(raw))
	if strings.Contains(body, "\n"+LegacyAnswerHeading) {
		t.Errorf("legacy answer section not retired:\n%s", body)
	}
	for _, s := range []string{"# 05 — Thing", "Human prose.", "Earlier note.", "retired from `" + LegacyAnswerHeading + "`", "Which db?"} {
		if !strings.Contains(body, s) {
			t.Errorf("body lost %q:\n%s", s, body)
		}
	}
}

func TestStartOver_RetiresOldRepairSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "05-thing.md")
	writeFile(t, path, "---\nid: \"05\"\nstatus: paused\npause_reason: merge-conflict\ntype: implement\n---\n\n"+LegacyRepairHeading+"\n\nBroke.\n")

	if err := StartOver(path, time.Now(), ""); err != nil {
		t.Fatalf("StartOver: %v", err)
	}

	raw, _ := os.ReadFile(path)
	body := schema.ParseBody(string(raw))
	if strings.Contains(body, "\n"+LegacyRepairHeading) || !strings.Contains(body, "Broke.") {
		t.Fatalf("unexpected body:\n%s", body)
	}
}

func TestStartOver_NoteIsWrittenInSameWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "05-thing.md")
	writeFile(t, path, "---\nid: \"05\"\nstatus: claimed\ntype: implement\n---\n\n# 05 — Thing\n")

	if err := StartOver(path, time.Now(), "restart note text\n"); err != nil {
		t.Fatalf("StartOver: %v", err)
	}

	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "status: open") || !strings.Contains(string(raw), "restart note text") {
		t.Fatalf("restart and note not both present:\n%s", raw)
	}
}

func TestStartOver_ClosesOpenQuestion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "05-thing.md")
	writeFile(t, path, "---\nid: \"05\"\nstatus: paused\npause_reason: self-reported\ntype: implement\n---\n\n# 05 — Thing\n")
	q, err := AppendLog(path, LogEvent{Type: EventPauseQuestion, By: "gx", Summary: "Which?", Text: "Which?"})
	if err != nil {
		t.Fatal(err)
	}

	if err := StartOver(path, time.Now(), ""); err != nil {
		t.Fatalf("StartOver: %v", err)
	}

	evs, _ := ReadLog(path)
	if len(evs) != 2 || evs[1].Type != EventPauseClosed || evs[1].Question != q.Seq || evs[1].Reason != "start-over" {
		t.Fatalf("log = %+v, want pause.closed{question: %d, reason: start-over}", evs, q.Seq)
	}
	// A second start over has no open question: no write, no error.
	if err := StartOver(path, time.Now(), ""); err != nil {
		t.Fatalf("second StartOver: %v", err)
	}
	if evs, _ = ReadLog(path); len(evs) != 2 {
		t.Fatalf("log grew without an open question: %+v", evs)
	}
}

func TestStartOver_NoQuestionWritesNoLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "05-thing.md")
	writeFile(t, path, "---\nid: \"05\"\nstatus: claimed\ntype: implement\n---\n\n# 05 — Thing\n")
	if err := StartOver(path, time.Now(), ""); err != nil {
		t.Fatalf("StartOver: %v", err)
	}
	if _, err := os.Stat(LogPath(path)); !os.IsNotExist(err) {
		t.Fatalf("log exists after start over with no question: %v", err)
	}
}
