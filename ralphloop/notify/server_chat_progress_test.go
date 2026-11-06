package notify

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/ralphloop/schedule"
)

func progressTestFacts() ProgressFacts {
	return ProgressFacts{
		Counts: schedule.EpicCounts{Done: 3, InProgress: 2, Total: 12},
		Running: []ProgressRunning{
			{Identifier: "06", Title: "Match the header title sizes", ElapsedSeconds: 840},
			{Identifier: "07", Title: "Phone step line", ElapsedSeconds: 180},
		},
		FinishedCount: 2, FinishedElapsedSeconds: 1260, FinishedCost: 3.4,
	}
}

func TestServerChat_ProgressSendsTheMessageAndRecordsItInTheGate(t *testing.T) {
	srv, reqs := fakeSlackServer(t, 200)
	statePath := filepath.Join(t.TempDir(), "gate.json")
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: srv.URL, GateStatePath: statePath})

	c.Progress("gx", nil, "better-epic", progressTestFacts())
	c.Close()

	got := reqs()
	if len(got) != 1 {
		t.Fatalf("got %d sends, want 1", len(got))
	}
	for _, want := range []string{
		"⏳ Progress better-epic · 3 of 12 done",
		"Running: 06 Match the header title sizes (14m), 07 Phone step line (3m)",
		"Finished since the last update: 2 tickets · 21m · $3.40",
		"Left: 7",
		RenderCountsLine(progressTestFacts().Counts),
		"\ngx",
	} {
		if !strings.Contains(got[0].Text, want) {
			t.Errorf("message lacks %q:\n%s", want, got[0].Text)
		}
	}

	state, err := LoadNotificationState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	dest := state.Transports[destinationKey(transportSlack, srv.URL, slackTarget(srv.URL))]
	found := 0
	for _, e := range dest.Events {
		if e.EventType == "progress-update" && e.Source == "epic:gx/better-epic" {
			found++
		}
	}
	if found != 1 {
		t.Errorf("events = %+v, want one progress-update from epic:gx/better-epic", dest.Events)
	}
	if len(dest.Sends) != 1 {
		t.Errorf("sends = %d, want 1", len(dest.Sends))
	}
}

func TestServerChat_ProgressIsNeverPerSourceMuted(t *testing.T) {
	srv, reqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: srv.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})

	for range 8 {
		c.Progress("gx", nil, "e", progressTestFacts())
	}
	c.Close()

	if len(reqs()) == 0 {
		t.Fatal("progress updates were stopped without a global mute")
	}
	for _, r := range reqs() {
		if strings.Contains(r.Text, "Muting") || strings.Contains(r.Text, "muting") {
			t.Errorf("a mute notice was sent for an epic source:\n%s", r.Text)
		}
	}
}

func TestServerChat_ProgressIsStoppedByTheGlobalMute(t *testing.T) {
	srv, reqs := fakeSlackServer(t, 200)
	statePath := filepath.Join(t.TempDir(), "gate.json")
	if err := updateNotificationStateAt(statePath, func(s *NotificationState) {
		s.Transports[destinationKey(transportSlack, srv.URL, slackTarget(srv.URL))] = TransportState{Muted: true, Reason: "auto-trip"}
	}); err != nil {
		t.Fatal(err)
	}
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: srv.URL, GateStatePath: statePath})

	c.Progress("gx", nil, "e", progressTestFacts())
	c.Close()

	if got := reqs(); len(got) != 0 {
		t.Errorf("globally muted destination got %+v", got)
	}
}
