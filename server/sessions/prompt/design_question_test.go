package prompt_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// questionRig is an idle Design session whose server chats to a recording
// Slack hook.
type questionRig struct {
	f       servertest.PromptFixture
	chat    *rigs.SlackHook
	started int
	gate    string
}

func startQuestionRig(t *testing.T, afk bool) *questionRig {
	t.Helper()
	chat := rigs.NewSlackHook(t)
	gate := filepath.Join(t.TempDir(), "gate.json")
	f := servertest.StartPromptSessionWith(t, servertest.FirstPromptGrace, servertest.WithAlerts(true, false), func(c *server.Config) {
		c.Chat = notify.ServerChatConfig{SlackWebhookURL: chat.URL, GateStatePath: gate, FlushInterval: servertest.FlushInterval}
	})
	started := chat.WaitStarted(t)
	if _, err := f.H.Client.Sessions.SetAFK(f.Ctx, &afk); err != nil {
		t.Fatal(err)
	}
	return &questionRig{f: f, chat: chat, started: started, gate: gate}
}

// writeRounds writes n Grilling rounds to the transcript, each but the last
// answered by the person.
func writeRounds(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	base := time.Now().Add(-time.Hour)
	for i := 1; i <= n; i++ {
		stamp := base.Add(time.Duration(i) * time.Minute).UTC().Format(time.RFC3339Nano)
		lines := []map[string]any{{"type": "assistant", "uuid": fmt.Sprintf("round-%d", i), "timestamp": stamp, "message": map[string]any{"role": "assistant", "content": []map[string]any{{"type": "text", "text": fmt.Sprintf("❓ **Q1** - **Store**: Which store for round %d?", i)}}}}}
		if i < n {
			lines = append(lines, map[string]any{"type": "user", "uuid": fmt.Sprintf("answer-%d", i), "timestamp": stamp, "message": map[string]any{"role": "user", "content": "the first"}})
		}
		for _, line := range lines {
			if err := enc.Encode(line); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// askRound ends a turn whose transcript holds n rounds, the last one waiting.
func (r *questionRig) askRound(t *testing.T, n int) {
	t.Helper()
	f := r.f
	writeRounds(t, f.Transcript, n)
	report(t, f.Conv, agentfake.Control{State: "working"})
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	report(t, f.Conv, agentfake.Control{State: "idle"})
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle && s.Actionable.Grilling })
}

// answer clears the raised Grilling the way a page prompt does.
func (r *questionRig) answer(t *testing.T) {
	t.Helper()
	r.f.Prompt(t, "the first")
	waitForSession(r.f.Ctx, t, r.f.H, r.f.Evs, r.f.ID, func(s wire.SessionInfo) bool { return s.Actionable.Reason == "" })
}

// quiet waits out the banner delay and the chat flush, then returns what the
// chat got after the start notice.
func (r *questionRig) quiet() []string {
	waitAlerts(r.f.H, 0) // negative wait: a banner would fire within the delay
	time.Sleep(5 * servertest.FlushInterval)
	return r.chat.PostsFrom(r.started)
}

func countContaining(posts []string, sub string) int {
	n := 0
	for _, p := range posts {
		n += strings.Count(p, sub)
	}
	return n
}

func TestDesignQuestion_OnePerRoundWithEpicStepRoundQuestionAndLink(t *testing.T) {
	t.Parallel()
	r := startQuestionRig(t, false)
	r.askRound(t, 1)
	posts := r.quiet()
	if len(posts) != 1 {
		t.Fatalf("posts = %q, want one Question", posts)
	}
	for _, want := range []string{"❓ Question map", "Design waits at the chart step. Round 1.", "Which store for round 1?", r.f.Base + "/#/d/"} {
		if !strings.Contains(posts[0], want) {
			t.Errorf("post lacks %q:\n%s", want, posts[0])
		}
	}

	r.answer(t)
	r.askRound(t, 2)
	posts = r.quiet()
	if got := countContaining(posts, "Design waits at the chart step."); got != 2 || !strings.Contains(strings.Join(posts, "\n"), "Round 2.") {
		t.Errorf("a new round should send a second Question, posts = %q", posts)
	}
}

func TestDesignQuestion_AFKSendsTheQuestionInsteadOfTheAlert(t *testing.T) {
	t.Parallel()
	r := startQuestionRig(t, true)
	r.askRound(t, 1)
	posts := r.quiet()
	if len(posts) != 1 || !strings.Contains(posts[0], "❓ Question map") || strings.Contains(posts[0], "🔔") {
		t.Errorf("posts = %q, want exactly the Question and no alert", posts)
	}
}

func TestDesignQuestion_NotSentWhileOpenOnScreen(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		afk  bool
		want int
	}{
		{"a TUI open with AFK off", false, 0},
		{"a TUI open with AFK on", true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := startQuestionRig(t, tc.afk)
			servertest.OpenTUI(r.f.Ctx, t, r.f.H, "tui-1")
			servertest.ReportFocus(r.f.Ctx, t, r.f.H, wire.FocusReport{Client: "tui-1", Focused: true, Session: r.f.ID})
			r.askRound(t, 1)
			if got := countContaining(r.quiet(), "❓ Question"); got != tc.want {
				t.Errorf("Questions = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestDesignQuestion_ClosingTheSessionAfterwardsSendsNothing(t *testing.T) {
	t.Parallel()
	r := startQuestionRig(t, false)
	servertest.OpenTUI(r.f.Ctx, t, r.f.H, "tui-1")
	servertest.ReportFocus(r.f.Ctx, t, r.f.H, wire.FocusReport{Client: "tui-1", Focused: true, Session: r.f.ID})
	r.askRound(t, 1)
	// A later status change orders what follows after the detection of the idle ran.
	settled(r.f.Ctx, t, r.f.H, r.f.Evs, r.f.ID, r.f.Conv)
	servertest.ReportFocus(r.f.Ctx, t, r.f.H, wire.FocusReport{Client: "tui-1", Focused: false})
	if got := countContaining(r.quiet(), "❓ Question"); got != 0 {
		t.Errorf("Questions = %d, want none: it is decided when the wait starts", got)
	}
}

func TestDesignQuestion_AMarkedRoundIsNeverResent(t *testing.T) {
	t.Parallel()
	r := startQuestionRig(t, false)
	// What a restart leaves behind: the round's mark already on disk.
	if isNew, err := notify.MarkOnce(r.gate, "question:"+r.f.ID+":1"); err != nil || !isNew {
		t.Fatalf("seed mark = %v, %v", isNew, err)
	}
	r.askRound(t, 1)
	if got := countContaining(r.quiet(), "❓ Question"); got != 0 {
		t.Errorf("Questions = %d, want none for a round already marked", got)
	}
}

func TestDesignQuestion_ABlockedSessionInAFKStillGetsItsAlert(t *testing.T) {
	t.Parallel()
	r := startQuestionRig(t, true)
	report(t, r.f.Conv, agentfake.Control{State: "blocked", Kind: "permission", Message: "run ls?"})
	waitForSession(r.f.Ctx, t, r.f.H, r.f.Evs, r.f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionBlocked })
	posts := r.quiet()
	if len(posts) != 1 || !strings.Contains(posts[0], "🔔 Blocked") || strings.Contains(posts[0], "❓") {
		t.Errorf("posts = %q, want the Blocked alert and no Question", posts)
	}
}
