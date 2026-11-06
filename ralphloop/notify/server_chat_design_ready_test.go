package notify

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestServerChat_DesignReadySendsTheMessageFromTheDesignEpic(t *testing.T) {
	srv, reqs := fakeSlackServer(t, 200)
	statePath := filepath.Join(t.TempDir(), "gate.json")
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: srv.URL, GateStatePath: statePath})

	c.DesignReady("gx", nil, "fast-queue-startup", "fast-queue-startup-impl", 7, "https://gx.local/#/e/gx:fast-queue-startup-impl")
	c.Close()

	got := reqs()
	if len(got) != 1 {
		t.Fatalf("got %d sends, want 1", len(got))
	}
	text := got[0].Text
	prev := -1
	for _, want := range []string{
		"📦 Ready fast-queue-startup-impl",
		"The design is finished. 7 tickets are ready to queue.",
		"https://gx.local/#/e/gx:fast-queue-startup-impl",
		"\ngx",
	} {
		i := strings.Index(text, want)
		if i <= prev {
			t.Fatalf("%q missing or out of order in:\n%s", want, text)
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
		if e.EventType == "design-ready" && e.Source == "epic:gx/fast-queue-startup" {
			found++
		}
	}
	if found != 1 {
		t.Errorf("events = %+v, want one design-ready from epic:gx/fast-queue-startup", dest.Events)
	}
}

func TestServerChat_DesignReadyLeavesTheLinkOutWhenEmpty(t *testing.T) {
	srv, reqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: srv.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})

	c.DesignReady("gx", nil, "m", "m-impl", 1, "")
	c.Close()

	got := reqs()
	if len(got) != 1 || strings.Contains(got[0].Text, "http") {
		t.Errorf("sends = %+v, want one without a link", got)
	}
}
