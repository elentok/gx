package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
)

func appendTestLog(t *testing.T, project string) {
	t.Helper()
	path := filepath.Join(project, "epic", "issues", "06-hello.md")
	for _, ev := range []tickets.LogEvent{
		{Type: tickets.EventPauseQuestion, By: "person", Summary: "Which?", Text: "detail"},
		{Type: tickets.EventPauseAnswer, Question: 1, Choice: "a"},
	} {
		if _, err := tickets.AppendLog(path, ev); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExecute_TicketsLog_PrintsRawEventsAndFiltersByType(t *testing.T) {
	repo, project := setupShowProject(t)
	appendTestLog(t, project)

	out, err := runIn(t, repo, "tickets", "log", "epic/06")
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"type":"pause.question"`) || !strings.Contains(lines[1], `"type":"pause.answer"`) {
		t.Errorf("log printed %q", out)
	}

	out, err = runIn(t, repo, "tickets", "log", "epic/06", "--type", "pause.question")
	if err != nil {
		t.Fatalf("log --type: %v", err)
	}
	lines = strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], `"type":"pause.question"`) {
		t.Errorf("log --type printed %q", out)
	}
}

func TestExecute_TicketsLog_NoLogPrintsNothing(t *testing.T) {
	repo, _ := setupShowProject(t)
	out, err := runIn(t, repo, "tickets", "log", "epic/06")
	if err != nil || out != "" {
		t.Errorf("log = %q, %v; want empty", out, err)
	}
}

func TestExecute_TicketsShow_AppendsPauseRoundsNotRawEvents(t *testing.T) {
	repo, project := setupShowProject(t)

	out, err := runIn(t, repo, "tickets", "show", "epic/06")
	if err != nil || strings.Contains(out, "Pause rounds") {
		t.Fatalf("show without log = %q, %v", out, err)
	}

	appendTestLog(t, project)
	out, err = runIn(t, repo, "tickets", "show", "epic/06")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if !strings.Contains(out, "body text\n\nPause rounds\n1. answered: Which?") {
		t.Errorf("show missing rounds block after body: %q", out)
	}
	if strings.Contains(out, `"type"`) || strings.Contains(out, "pause.question") {
		t.Errorf("show leaked raw events: %q", out)
	}
}
