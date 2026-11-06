package notify

import (
	"github.com/elentok/gx/ralphloop/ticketops"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/schedule"
)

func TestServerChat_PausesGoToTheirOwnDestinationAndSharedTargetsShareABatch(t *testing.T) {
	global, globalReqs := fakeSlackServer(t, 200)
	other, otherReqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: global.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})

	own := &ServerChatConfig{SlackWebhookURL: other.URL}
	c.Pause("alpha", nil, "e", "/t/1.md", "01", ticketops.PauseStatus("zero-commit"), "zero-commit", "r", schedule.EpicCounts{})
	c.Pause("beta", own, "e", "/t/2.md", "02", ticketops.PauseStatus("zero-commit"), "zero-commit", "r", schedule.EpicCounts{})
	c.Pause("gamma", own, "e", "/t/3.md", "03", ticketops.PauseStatus("zero-commit"), "zero-commit", "r", schedule.EpicCounts{})
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

func TestServerChat_PausesInOneBatchEndEachWithTheirProjectLine(t *testing.T) {
	srv, reqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: srv.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})

	c.Pause("alpha", nil, "e", "/t/1.md", "A1", ticketops.PauseStatus("zero-commit"), "zero-commit", "r", schedule.EpicCounts{})
	c.Pause("beta", nil, "e", "/t/2.md", "B1", ticketops.PauseStatus("zero-commit"), "zero-commit", "r", schedule.EpicCounts{})
	c.Pause("alpha", nil, "e", "/t/3.md", "A2", ticketops.PauseStatus("zero-commit"), "zero-commit", "r", schedule.EpicCounts{})
	c.Close()

	got := reqs()
	if len(got) != 1 {
		t.Fatalf("got %d sends, want one batch", len(got))
	}
	text := got[0].Text
	if strings.Contains(text, "*alpha*") || strings.Contains(text, "*beta*") {
		t.Errorf("a pause has no bold project header:\n%s", text)
	}
	if n := strings.Count(text, "\nalpha"); n != 2 {
		t.Errorf("alpha project line appears %d times, want 2:\n%s", n, text)
	}
	if n := strings.Count(text, "\nbeta"); n != 1 {
		t.Errorf("beta project line appears %d times, want 1:\n%s", n, text)
	}
}

func TestServerChat_NoticeIsIconPhraseDetailWithNoProjectLine(t *testing.T) {
	msg := serverNoticeMessage(ServerNotice{Kind: "k", Emoji: "🔌", Title: "herdr unavailable", Detail: "gone", HoldsWork: true})
	if got, want := msg.Text(slackStyle.chatStyle).String(), "🔌 Herdr unavailable\ngone"; got != want {
		t.Errorf("Slack notice = %q, want %q", got, want)
	}
	if got, want := msg.Text(telegramStyle.chatStyle).String(), "🔌 Herdr unavailable\ngone"; got != want {
		t.Errorf("Telegram notice = %q, want %q", got, want)
	}
	if msg.urgency != urgencyAttention {
		t.Errorf("urgency = %q, a notice that holds work needs attention", msg.urgency)
	}
	if serverNoticeMessage(ServerNotice{Emoji: "🚀", Title: "server started"}).urgency != "" {
		t.Error("a notice that holds nothing has no urgency")
	}
}

func TestServerChat_LongTicketNoticeNamesItsTicket(t *testing.T) {
	msg := serverNoticeMessage(ServerNotice{Emoji: "⏳", Title: "long ticket e/06 Match the header", Detail: "The agent has run 62 assistant turns."})
	if got, want := msg.Text(slackStyle.chatStyle).String(), "⏳ Long ticket e/06 Match the header\nThe agent has run 62 assistant turns."; got != want {
		t.Errorf("long ticket = %q, want %q", got, want)
	}
}

func TestServerChat_PauseDigestListsOneLinePerPauseWithNoProjectLine(t *testing.T) {
	lines := []string{
		PauseDigestLine("p1", "e", "01", ticketops.PauseStatus("spinning"), "The agent session ended unexpectedly."),
		PauseDigestLine("p2", "e", "02", ticketops.PauseStatus("zero-commit"), "No commits merged."),
	}
	want := "🅿️ 2 tickets paused while herdr was down\n" +
		"p1 · e/01 Repair: The agent session ended unexpectedly.\n" +
		"p2 · e/02 Answer: No commits merged."
	if got := pauseDigestMessage(lines).Text(slackStyle.chatStyle).String(); got != want {
		t.Errorf("digest = %q, want %q", got, want)
	}
}

func TestServerChat_OverrideReplacesGlobalWithoutMerging(t *testing.T) {
	global, globalReqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: global.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})

	// An override naming no destination mutes the project; it must not fall
	// back to the global webhook.
	c.Pause("quiet", &ServerChatConfig{}, "e", "/t/1.md", "01", ticketops.PauseStatus("zero-commit"), "zero-commit", "r", schedule.EpicCounts{})
	c.Close()

	if got := globalReqs(); len(got) != 0 {
		t.Errorf("global got %+v, want nothing", got)
	}
}

func TestServerChat_PauseHasNoCountsLine(t *testing.T) {
	global, globalReqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: global.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})
	c.Pause("alpha", nil, "e", "/t/1.md", "01", ticketops.PauseStatus("zero-commit"), "", "needs rebase", schedule.EpicCounts{Done: 5, Total: 7})
	c.Close()

	g := globalReqs()
	if len(g) != 1 || strings.Contains(g[0].Text, "done") || strings.Contains(g[0].Text, "total") {
		t.Errorf("got %+v, want a pause without a counts line", g)
	}
}

func TestServerChat_EpicCompleteNamesTheProjectAndTheTotals(t *testing.T) {
	global, globalReqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: global.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})
	c.EpicComplete("alpha", nil, "e", schedule.EpicCounts{Done: 7, Total: 7}, 3600, 12.5, true)
	c.Close()

	g := globalReqs()
	if len(g) != 1 || !strings.Contains(g[0].Text, "\U0001f389 Merged e") || !strings.Contains(g[0].Text, "alpha") || !strings.Contains(g[0].Text, "7 done") || !strings.Contains(g[0].Text, "7 tickets merged in 1h 00m · $12.50") {
		t.Errorf("got %+v, want a 'Merged' message for alpha", g)
	}
}

func TestServerChat_EpicCompleteNotMergedSaysSo(t *testing.T) {
	global, globalReqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: global.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})
	c.EpicComplete("alpha", nil, "e", schedule.EpicCounts{Done: 7, Total: 7}, 3600, 12.5, false)
	c.Close()

	g := globalReqs()
	if len(g) != 1 || !strings.Contains(g[0].Text, "✅ Complete e") || !strings.Contains(g[0].Text, "Not merged.") || strings.Contains(g[0].Text, "Merged") {
		t.Errorf("got %+v, want a 'Complete' message saying not merged", g)
	}
}

func TestServerChat_EmptyResultSaysNoneRecorded(t *testing.T) {
	hook, reqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: hook.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "e"), 0o755); err != nil {
		t.Fatal(err)
	}

	ticketPath := filepath.Join(dir, "e", "01-do-thing.md")
	if err := os.WriteFile(ticketPath, []byte("# Do thing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c.Result("alpha", nil, dir, "e", ticketPath, "01", " \n")
	c.Close()

	got := reqs()
	if len(got) != 1 || !strings.Contains(got[0].Text, "✅ Done e/01 Do thing") || !strings.Contains(got[0].Text, "No result recorded.") {
		t.Errorf("sends = %+v, want the Done headline and 'No result recorded.'", got)
	}
}

func TestServerChat_ResultIsTruncatedAndLogsNotificationSent(t *testing.T) {
	hook, reqs := fakeSlackServer(t, 200)
	c := NewServerChat(ServerChatConfig{SlackWebhookURL: hook.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")})
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "e"), 0o755); err != nil {
		t.Fatal(err)
	}

	ticketPath := filepath.Join(dir, "e", "01-do-thing.md")
	if err := os.WriteFile(ticketPath, []byte("# Do thing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c.Result("alpha", nil, dir, "e", ticketPath, "01", strings.Repeat("x", 5000))
	c.Close()

	got := reqs()
	want := strings.Repeat("x", maxResultRunes) + "…"
	if len(got) != 1 || !strings.Contains(got[0].Text, "✅ Done e/01 Do thing") || !strings.Contains(got[0].Text, want) || strings.Contains(got[0].Text, want[:len(want)-3]+"x") {
		t.Fatalf("sends = %+v, want one message with the Done headline and the Result cut at the limit", got)
	}
	evs, _, err := runlog.ReadEvents(dir, "e")
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Type != "notification-sent" || evs[0].NotifyKind != "result" {
		t.Errorf("events = %+v, want one notification-sent with notify_kind=result", evs)
	}
}
