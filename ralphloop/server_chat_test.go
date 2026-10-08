package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerChat_ParksGoToTheirOwnDestinationAndSharedTargetsShareABatch(t *testing.T) {
	global, globalReqs := fakeSlackServer(t, 200)
	other, otherReqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: global.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})

	own := &ServerChatConfig{SlackWebhookURL: other.URL}
	c.Park("alpha", nil, "e", "/t/1.md", "01", "needs-answer", "r", EpicCounts{})
	c.Park("beta", own, "e", "/t/2.md", "02", "needs-answer", "r", EpicCounts{})
	c.Park("gamma", own, "e", "/t/3.md", "03", "needs-answer", "r", EpicCounts{})
	c.Close()

	g := globalReqs()
	if len(g) != 1 || !strings.Contains(g[0].Text, "alpha") || strings.Contains(g[0].Text, "beta") {
		t.Errorf("global got %+v", g)
	}
	o := otherReqs()
	if len(o) != 1 || !strings.Contains(o[0].Text, "beta") || !strings.Contains(o[0].Text, "gamma") {
		t.Errorf("shared destination got %+v, want one batch with beta and gamma", o)
	}
}

func TestServerChat_MultiProjectBatchGroupsUnderOneHeaderPerProject(t *testing.T) {
	srv, reqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: srv.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})

	c.Park("alpha", nil, "e", "/t/1.md", "A1", "needs-answer", "r", EpicCounts{})
	c.Park("beta", nil, "e", "/t/2.md", "B1", "needs-answer", "r", EpicCounts{})
	c.Park("alpha", nil, "e", "/t/3.md", "A2", "needs-answer", "r", EpicCounts{})
	c.Close()

	got := reqs()
	if len(got) != 1 {
		t.Fatalf("got %d sends, want one batch", len(got))
	}
	text := got[0].Text
	if n := strings.Count(text, "*alpha*"); n != 1 {
		t.Errorf("alpha header appears %d times, want 1:\n%s", n, text)
	}
	if n := strings.Count(text, "*beta*"); n != 1 {
		t.Errorf("beta header appears %d times, want 1:\n%s", n, text)
	}
	if !(strings.Index(text, "A1") < strings.Index(text, "A2") && strings.Index(text, "A2") < strings.Index(text, "B1")) {
		t.Errorf("alpha's parks should sit together before beta's:\n%s", text)
	}
}

func TestServerChat_OverrideReplacesGlobalWithoutMerging(t *testing.T) {
	global, globalReqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: global.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})

	// An override naming no destination mutes the project; it must not fall
	// back to the global webhook.
	c.Park("quiet", &ServerChatConfig{}, "e", "/t/1.md", "01", "needs-answer", "r", EpicCounts{})
	c.Close()

	if got := globalReqs(); len(got) != 0 {
		t.Errorf("global got %+v, want nothing", got)
	}
}

func TestServerChat_ParkShowsTheEpicCounts(t *testing.T) {
	global, globalReqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: global.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})
	c.Park("alpha", nil, "e", "/t/1.md", "01", "needs-answer", "needs rebase", EpicCounts{Done: 5, Total: 7})
	c.Close()

	g := globalReqs()
	if len(g) != 1 || !strings.Contains(g[0].Text, "5 done") || !strings.Contains(g[0].Text, "7 total") {
		t.Errorf("got %+v, want the epic's counts and not '0 done · 0 total'", g)
	}
}

func TestServerChat_EpicCompleteNamesTheProjectAndTheTotals(t *testing.T) {
	global, globalReqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: global.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})
	c.EpicComplete("alpha", nil, "e", EpicCounts{Done: 7, Total: 7}, 3600, 12.5)
	c.Close()

	g := globalReqs()
	if len(g) != 1 || !strings.Contains(g[0].Text, "epic complete") || !strings.Contains(g[0].Text, "alpha") || !strings.Contains(g[0].Text, "7 done") {
		t.Errorf("got %+v, want an 'epic complete' message for alpha", g)
	}
}

func TestServerChat_ResultIsTruncatedAndLogsNotificationSent(t *testing.T) {
	hook, reqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: hook.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "e"), 0o755); err != nil {
		t.Fatal(err)
	}

	c.Result("alpha", nil, dir, "e", "alpha:e/01", strings.Repeat("x", 5000))
	c.Close()

	got := reqs()
	if len(got) != 1 || !strings.Contains(got[0].Text, "alpha:e/01") || len(got[0].Text) > 3000 {
		t.Fatalf("sends = %+v, want one truncated message naming the address", got)
	}
	evs, _, err := ReadEvents(dir, "e")
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Type != "notification-sent" || evs[0].NotifyKind != "result" {
		t.Errorf("events = %+v, want one notification-sent with notify_kind=result", evs)
	}
}
