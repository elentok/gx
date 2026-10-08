package ralphloop

import (
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
	if len(g) != 1 || !strings.Contains(g[0].Text, "epic complete") || !strings.Contains(g[0].Text, "[alpha]") || !strings.Contains(g[0].Text, "7 done") {
		t.Errorf("got %+v, want an 'epic complete' message for alpha", g)
	}
}
