package notify

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestServerChat_DesignQuestionSendsTheMessageAndRecordsItInTheGate(t *testing.T) {
	srv, reqs := fakeSlackServer(t, 200)
	statePath := filepath.Join(t.TempDir(), "gate.json")
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: srv.URL, GateStatePath: statePath})

	c.DesignQuestion("gx", nil, "fast-queue-startup", "Grill", 2, "Should startup block on the queue?", "http://localhost:8080/design/x")
	c.Close()

	got := reqs()
	if len(got) != 1 {
		t.Fatalf("got %d sends, want 1", len(got))
	}
	text := got[0].Text
	ordered := []string{
		"❓ Question fast-queue-startup",
		"Design waits at the Grill step. Round 2.",
		"Should startup block on the queue?",
		"http://localhost:8080/design/x",
		"\ngx",
	}
	prev := -1
	for _, want := range ordered {
		i := strings.Index(text, want)
		if i < 0 {
			t.Fatalf("message lacks %q:\n%s", want, text)
		}
		if i <= prev {
			t.Errorf("%q is out of order in:\n%s", want, text)
		}
		prev = i
	}

	state, err := LoadNotificationState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	dest := state.Transports[destinationKey(transportSlack, srv.URL, slackTarget(srv.URL))]
	found := 0
	for _, e := range dest.Events {
		if e.EventType == "design-question" && e.Source == "epic:gx/fast-queue-startup" {
			found++
		}
	}
	if found != 1 {
		t.Errorf("events = %+v, want one design-question from epic:gx/fast-queue-startup", dest.Events)
	}
	if len(dest.Sends) != 1 {
		t.Errorf("sends = %d, want 1", len(dest.Sends))
	}
}

func TestServerChat_DesignQuestionLeavesTheLinkOutWhenEmpty(t *testing.T) {
	srv, reqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: srv.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})

	c.DesignQuestion("gx", nil, "e", "Grill", 1, "Why?", "")
	c.Close()

	got := reqs()
	if len(got) != 1 {
		t.Fatalf("got %d sends, want 1", len(got))
	}
	if strings.Contains(got[0].Text, "http") {
		t.Errorf("link not omitted:\n%s", got[0].Text)
	}
	if !strings.Contains(got[0].Text, "Why?\ngx") {
		t.Errorf("project line should follow the question directly:\n%s", got[0].Text)
	}
}

func TestServerChat_DesignQuestionIsNeverPerSourceMuted(t *testing.T) {
	srv, reqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: srv.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})

	for range 8 {
		c.DesignQuestion("gx", nil, "e", "Grill", 1, "Why?", "")
	}
	c.Close()

	if len(reqs()) == 0 {
		t.Fatal("questions were stopped without a global mute")
	}
	for _, r := range reqs() {
		if strings.Contains(strings.ToLower(r.Text), "muting") {
			t.Errorf("a mute notice was sent for an epic source:\n%s", r.Text)
		}
	}
}

func TestServerChat_DesignQuestionIsStoppedByTheGlobalMute(t *testing.T) {
	srv, reqs := fakeSlackServer(t, 200)
	statePath := filepath.Join(t.TempDir(), "gate.json")
	if err := updateNotificationStateAt(statePath, func(s *NotificationState) {
		s.Transports[destinationKey(transportSlack, srv.URL, slackTarget(srv.URL))] = TransportState{Muted: true, Reason: "auto-trip"}
	}); err != nil {
		t.Fatal(err)
	}
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: srv.URL, GateStatePath: statePath})

	c.DesignQuestion("gx", nil, "e", "Grill", 1, "Why?", "")
	c.Close()

	if got := reqs(); len(got) != 0 {
		t.Errorf("globally muted destination got %+v", got)
	}
}
