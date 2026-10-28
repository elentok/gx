package cmd

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/tickets"
)

// mapWithConversations creates the map mine:m, records recs against it (their
// Ticket is set by the caller), and returns the project dir and cwd. Recording
// began before the map was created unless preRecording is set.
func mapWithConversations(t *testing.T, preRecording bool, recs ...conversations.Record) (cwd, project string) {
	t.Helper()
	cwd, project = setupEpicAddProject(t)
	data, err := config.DirsFrom(envFor(t)).DataDir()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(data, "conversations")
	started := time.Now().Add(-time.Hour)
	if preRecording {
		started = time.Now().Add(time.Hour)
	}
	if err := conversations.MarkStarted(root, started); err != nil {
		t.Fatal(err)
	}
	if err := runTicketsEpicAdd(config.DirsFrom(envFor(t)), cwd, "mine:m", "map", "open", "", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if err := conversations.Save(root, r); err != nil {
			t.Fatal(err)
		}
	}
	return cwd, project
}

func sumRecs() []conversations.Record {
	return []conversations.Record{
		{ID: "a", Ticket: "mine:m", Totals: &conversations.Totals{TotalTokens: 100, Cost: 0.5}},
		{ID: "b", Ticket: "mine:m/01", Totals: &conversations.Totals{TotalTokens: 20, Cost: 0.25}},
		{ID: "c", Ticket: "mine:m-impl/01", Totals: &conversations.Totals{TotalTokens: 9999, Cost: 9}},
	}
}

func TestRunTicketsEpicAdd_MapStampsConversationTotalsAtImplCreation(t *testing.T) {
	cwd, project := mapWithConversations(t, false, sumRecs()...)
	if err := runTicketsEpicAdd(config.DirsFrom(envFor(t)), cwd, "mine:m-impl", "loop", "open", "", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	m := tickets.LoadEpicTicketMD(filepath.Join(project, "m"))
	if m.TotalTokens == nil || *m.TotalTokens != 120 || m.ActualCost != 0.75 {
		t.Errorf("tokens = %v, cost = %v; want 120 and 0.75", m.TotalTokens, m.ActualCost)
	}
}

func TestRunTicketsEpicAdd_MapMissingTotalsStampsNoTokens(t *testing.T) {
	recs := append(sumRecs(), conversations.Record{ID: "d", Ticket: "mine:m/02"})
	cwd, project := mapWithConversations(t, false, recs...)
	if err := runTicketsEpicAdd(config.DirsFrom(envFor(t)), cwd, "mine:m-impl", "loop", "open", "", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	m := tickets.LoadEpicTicketMD(filepath.Join(project, "m"))
	if m.TotalTokens != nil || m.CompletedAt.IsZero() {
		t.Errorf("tokens = %v, completed_at = %v; want no tokens but a completion", m.TotalTokens, m.CompletedAt)
	}
}

func TestExecute_TicketsSet_MapCompletedAtStampsConversationTotals(t *testing.T) {
	cwd, project := mapWithConversations(t, false, sumRecs()...)
	d := deps{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, getwd: func() (string, error) { return cwd, nil }, env: envFor(t)}
	if err := execute([]string{"tickets", "set", "mine:m", "--completed-at=now"}, d); err != nil {
		t.Fatal(err)
	}
	m := tickets.LoadEpicTicketMD(filepath.Join(project, "m"))
	if m.TotalTokens == nil || *m.TotalTokens != 120 || m.ActualCost != 0.75 {
		t.Errorf("tokens = %v, cost = %v; want 120 and 0.75", m.TotalTokens, m.ActualCost)
	}
}

func TestExecute_TicketsSet_PreRecordingMapCompletedAtStampsNoTokens(t *testing.T) {
	cwd, project := mapWithConversations(t, true, sumRecs()...)
	d := deps{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, getwd: func() (string, error) { return cwd, nil }, env: envFor(t)}
	if err := execute([]string{"tickets", "set", "mine:m", "--completed-at=now"}, d); err != nil {
		t.Fatal(err)
	}
	m := tickets.LoadEpicTicketMD(filepath.Join(project, "m"))
	if m.TotalTokens != nil || m.CompletedAt.IsZero() {
		t.Errorf("tokens = %v, completed_at = %v; want no tokens but a completion", m.TotalTokens, m.CompletedAt)
	}
}
