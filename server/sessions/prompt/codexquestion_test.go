package prompt_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/agentfake/codex"
)

const codexQuestionCall = "call_q1"

// codexQuestions is three questions of three options each: a, b and c.
func codexQuestions() []agentfake.QuestionSpec {
	var qs []agentfake.QuestionSpec
	for _, id := range []string{"a", "b", "c"} {
		q := agentfake.QuestionSpec{ID: id, Header: "Header " + id, Question: "Question " + id + "?"}
		for _, l := range []string{"One", "Two", "Three"} {
			q.Options = append(q.Options, agentfake.QuestionOption{Label: l + " " + id, Description: "Pick " + l})
		}
		qs = append(qs, q)
	}
	return qs
}

func startCodexQuestionSession(t *testing.T, extraEnv ...string) rigs.PromptFixture {
	t.Helper()
	return rigs.StartCodexPromptSessionWith(t, func(c *server.Config) {
		c.PromptGrace = rigs.FirstPromptGrace
		c.CodexQuestionWait = time.Second
	}, extraEnv...)
}

func openCodexQuestion(t *testing.T, f rigs.PromptFixture, call agentfake.QuestionCall) {
	t.Helper()
	call.CallID = codexQuestionCall
	if call.Questions == nil {
		call.Questions = codexQuestions()
	}
	if err := agentfake.SendProgramControl(f.Conv, agentfake.Control{Action: agentfake.ActionQuestion, Question: &call}); err != nil {
		t.Fatal(err)
	}
	if call.Quiet {
		waitForCodexCard(t, f)
		return
	}
	servertest.WaitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool {
		return s.Status.State == wire.SessionBlocked && s.Status.Kind == "question"
	})
	servertest.WaitForScreen(t, servertest.WaitForHolder(t, filepath.Join(f.H.SessionsDir, f.ID)), "Question "+strconv.Itoa(call.StartAt+1)+"/")
}

func waitForCodexCard(t *testing.T, f rigs.PromptFixture) wire.QuestionCard {
	t.Helper()
	var card wire.QuestionCard
	servertest.WaitFor(t, 5*time.Second, "the codex-question card", func() bool {
		chat, err := f.H.Client.Sessions.Chat(f.Ctx, f.ID)
		if err != nil {
			return false
		}
		for _, c := range chat.Questions {
			if c.Source == wire.CardSourceCodex {
				card = c
				return true
			}
		}
		return false
	})
	return card
}

func answerCodex(t *testing.T, f rigs.PromptFixture, cardID string, answers ...wire.SessionAnswer) wire.SessionResult {
	t.Helper()
	res, err := f.H.Client.Sessions.Answer(f.Ctx, wire.SessionAnswerRequest{
		ID: f.ID, Source: wire.CardSourceCodex, CardID: cardID, Answers: answers,
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func codexKeys(t *testing.T, f rigs.PromptFixture) []string {
	t.Helper()
	keys, err := codex.ReadQuestionKeys(f.CodexHome, f.Conv)
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

// codexOutput is the answers of the call's function_call_output, by question id.
func codexOutput(t *testing.T, f rigs.PromptFixture) map[string][]string {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(f.CodexHome, "sessions", "*", "*", "*", "rollout-*"+f.Conv+".jsonl"))
	if len(paths) != 1 {
		t.Fatalf("rollouts = %v", paths)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	var out string
	for _, line := range splitLines(data) {
		var l struct {
			Payload struct {
				Type   string `json:"type"`
				CallID string `json:"call_id"`
				Output string `json:"output"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &l) == nil && l.Payload.Type == "function_call_output" && l.Payload.CallID == codexQuestionCall {
			out = l.Payload.Output
		}
	}
	var parsed struct {
		Answers map[string]struct{ Answers []string }
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output %q: %v", out, err)
	}
	got := map[string][]string{}
	for id, a := range parsed.Answers {
		got[id] = a.Answers
	}
	return got
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	return lines
}

func wantKeys(t *testing.T, f rigs.PromptFixture, want ...string) {
	t.Helper()
	if got := codexKeys(t, f); !slices.Equal(got, want) {
		t.Fatalf("keys = %q, want %q", got, want)
	}
}

func TestSessionAnswer_CodexQuestionDigitsSubmitOnTheLastAndMatchByID(t *testing.T) {
	t.Parallel()
	f := startCodexQuestionSession(t)
	openCodexQuestion(t, f, agentfake.QuestionCall{})
	card := waitForCodexCard(t, f)
	res := answerCodex(t, f, card.ID, wire.SessionAnswer{Choice: "1"}, wire.SessionAnswer{Choice: "2"}, wire.SessionAnswer{Choice: "3"})
	if res.Refused {
		t.Fatalf("answer = %+v", res)
	}
	wantKeys(t, f, "1", "2", "3")
	got := codexOutput(t, f)
	for id, want := range map[string]string{"a": "One a", "b": "Two b", "c": "Three c"} {
		if !slices.Equal(got[id], []string{want}) {
			t.Errorf("answer %s = %q, want %q", id, got[id], want)
		}
	}
}

func TestSessionAnswer_CodexQuestionNoteAndFreeTextUseTheirKeys(t *testing.T) {
	t.Parallel()
	f := startCodexQuestionSession(t)
	openCodexQuestion(t, f, agentfake.QuestionCall{})
	card := waitForCodexCard(t, f)
	res := answerCodex(t, f, card.ID,
		wire.SessionAnswer{Choice: "2", Note: "because"},
		wire.SessionAnswer{Text: "my own"},
		wire.SessionAnswer{Choice: "3"})
	if res.Refused {
		t.Fatalf("answer = %+v", res)
	}
	wantKeys(t, f,
		"Down", "Tab", "text:because", "Enter",
		"Down", "Down", "Down", "Enter", "text:my own", "Enter",
		"3")
	got := codexOutput(t, f)
	if !slices.Equal(got["a"], []string{"Two a", "user_note: because"}) ||
		!slices.Equal(got["b"], []string{"None of the above", "user_note: my own"}) ||
		!slices.Equal(got["c"], []string{"Three c"}) {
		t.Errorf("output = %q", got)
	}
}

func TestSessionAnswer_CodexQuestionFreeTextRowIDWithTextUsesNoneOfTheAbove(t *testing.T) {
	t.Parallel()
	f := startCodexQuestionSession(t)
	openCodexQuestion(t, f, agentfake.QuestionCall{})
	card := waitForCodexCard(t, f)
	res := answerCodex(t, f, card.ID,
		wire.SessionAnswer{Choice: "1"},
		wire.SessionAnswer{Choice: "other", Text: "my own"},
		wire.SessionAnswer{Choice: "3"})
	if res.Refused {
		t.Fatalf("answer = %+v", res)
	}
	wantKeys(t, f, "1",
		"Down", "Down", "Down", "Enter", "text:my own", "Enter",
		"3")
	if got := codexOutput(t, f); !slices.Equal(got["b"], []string{"None of the above", "user_note: my own"}) {
		t.Errorf("output = %q", got)
	}
}

func TestSessionAnswer_CodexQuestionNotReadySendsNothingThenWorks(t *testing.T) {
	t.Parallel()
	f := startCodexQuestionSession(t)
	openCodexQuestion(t, f, agentfake.QuestionCall{Quiet: true})
	card := waitForCodexCard(t, f)
	answers := []wire.SessionAnswer{{Choice: "1"}, {Choice: "1"}, {Choice: "1"}}

	// The call is on disk but the session is not Blocked on a question.
	if res := answerCodex(t, f, card.ID, answers...); !res.Refused || res.Reason != wire.ReasonCodexNotReady {
		t.Fatalf("idle answer = %+v, want %s", res, wire.ReasonCodexNotReady)
	}
	// Blocked on a question, but the screen shows no overlay.
	f.Block(t, agentfake.Control{Action: agentfake.ActionReport, State: "blocked", Kind: "question"})
	if res := answerCodex(t, f, card.ID, answers...); !res.Refused || res.Reason != wire.ReasonCodexNotReady {
		t.Fatalf("blocked answer without overlay = %+v, want %s", res, wire.ReasonCodexNotReady)
	}
	wantKeys(t, f)

	if err := agentfake.SendProgramControl(f.Conv, agentfake.Control{Action: agentfake.ActionQuestionShow}); err != nil {
		t.Fatal(err)
	}
	servertest.WaitForScreen(t, servertest.WaitForHolder(t, filepath.Join(f.H.SessionsDir, f.ID)), "Question 1/")
	if res := answerCodex(t, f, card.ID, answers...); res.Refused {
		t.Fatalf("answer once ready = %+v", res)
	}
	wantKeys(t, f, "1", "1", "1")
}

func TestSessionAnswer_CodexQuestionLateRedrawIsNotAMismatch(t *testing.T) {
	t.Parallel()
	f := startCodexQuestionSession(t, codex.CodexQuestionRedrawEnv+"=300")
	openCodexQuestion(t, f, agentfake.QuestionCall{})
	card := waitForCodexCard(t, f)
	if res := answerCodex(t, f, card.ID, wire.SessionAnswer{Choice: "1"}, wire.SessionAnswer{Choice: "1"}, wire.SessionAnswer{Choice: "1"}); res.Refused {
		t.Fatalf("answer = %+v", res)
	}
	wantKeys(t, f, "1", "1", "1")
}

// shown copies the questions with mutate applied to the question at index.
func shownWith(index int, mutate func(*agentfake.QuestionSpec)) []agentfake.QuestionSpec {
	qs := codexQuestions()
	mutate(&qs[index])
	return qs
}

func TestSessionAnswer_CodexQuestionMismatchStopsWithTheRightOutcome(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		shown    []agentfake.QuestionSpec
		start    int // overlay opens on this 0-based question
		cursor   int // overlay opens with its cursor on this row
		reason   string
		wantKeys []string
	}{
		"header on the screen names another question": {
			start:  1,
			reason: wire.ReasonCodexMismatch,
		},
		"cursor not on the first row": {
			cursor: 2,
			reason: wire.ReasonCodexMismatch,
		},
		"question text on the first": {
			shown:  shownWith(0, func(q *agentfake.QuestionSpec) { q.Question = "Another?" }),
			reason: wire.ReasonCodexMismatch,
		},
		"row label on the first": {
			shown:  shownWith(0, func(q *agentfake.QuestionSpec) { q.Options[1].Label = "Changed" }),
			reason: wire.ReasonCodexMismatch,
		},
		"row count on the first": {
			shown:  shownWith(0, func(q *agentfake.QuestionSpec) { q.Options = q.Options[:2] }),
			reason: wire.ReasonCodexMismatch,
		},
		"question count": {
			shown:  codexQuestions()[:2],
			reason: wire.ReasonCodexMismatch,
		},
		"row count on the second": {
			shown:    shownWith(1, func(q *agentfake.QuestionSpec) { q.Options = q.Options[:2] }),
			reason:   wire.ReasonCodexStopped,
			wantKeys: []string{"1"},
		},
		"row label on the third": {
			shown:    shownWith(2, func(q *agentfake.QuestionSpec) { q.Options[0].Label = "Changed" }),
			reason:   wire.ReasonCodexStopped,
			wantKeys: []string{"1", "1"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := startCodexQuestionSession(t)
			openCodexQuestion(t, f, agentfake.QuestionCall{Shown: tc.shown, StartAt: tc.start, StartCursor: tc.cursor})
			card := waitForCodexCard(t, f)
			res := answerCodex(t, f, card.ID, wire.SessionAnswer{Choice: "1"}, wire.SessionAnswer{Choice: "1"}, wire.SessionAnswer{Choice: "1"})
			if !res.Refused || res.Reason != tc.reason {
				t.Fatalf("answer = %+v, want %s", res, tc.reason)
			}
			wantKeys(t, f, tc.wantKeys...)
		})
	}
}

// A question the screen wraps onto two lines is the same question.
func TestSessionAnswer_CodexQuestionWrappedQuestionIsNotAMismatch(t *testing.T) {
	t.Parallel()
	f := startCodexQuestionSession(t)
	openCodexQuestion(t, f, agentfake.QuestionCall{Shown: shownWith(0, func(q *agentfake.QuestionSpec) { q.Question = "Question\r\n  a?" })})
	card := waitForCodexCard(t, f)
	if res := answerCodex(t, f, card.ID, wire.SessionAnswer{Choice: "1"}, wire.SessionAnswer{Choice: "1"}, wire.SessionAnswer{Choice: "1"}); res.Refused {
		t.Fatalf("answer = %+v", res)
	}
	wantKeys(t, f, "1", "1", "1")
}

// A Fix agent is not a plain chat: an open question call is no codex card and
// sends no Question push.
func TestCodexQuestion_FixSessionListsNoCodexCard(t *testing.T) {
	t.Parallel()
	chatHook := rigs.NewSlackHook(t)
	f := rigs.StartCodexPromptSessionAs(t, func(c *server.Config) {
		c.PromptGrace = rigs.FirstPromptGrace
		c.CodexQuestionWait = time.Second
		c.Chat = notify.ServerChatConfig{SlackWebhookURL: chatHook.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: servertest.FlushInterval}
	}, wire.CreateSessionRequest{Ticket: "proj:plain/01", FixOf: "proj:plain/01"})
	started := chatHook.WaitStarted(t)
	openCodexQuestion(t, f, agentfake.QuestionCall{})
	time.Sleep(10 * servertest.FlushInterval) // negative wait: no event marks the chat flush
	for _, p := range chatHook.PostsFrom(started) {
		if strings.Contains(p, "❓ Question") {
			t.Errorf("chat post %q, want no Question push", p)
		}
	}
	chat, err := f.H.Client.Sessions.Chat(f.Ctx, f.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range chat.Questions {
		if c.Source == wire.CardSourceCodex {
			t.Errorf("questions = %+v, want no codex card", chat.Questions)
		}
	}
}

func TestSessionAnswer_CodexQuestionRefusesAStaleCallAndAnUnansweredQuestion(t *testing.T) {
	t.Parallel()
	f := startCodexQuestionSession(t)
	openCodexQuestion(t, f, agentfake.QuestionCall{})
	card := waitForCodexCard(t, f)
	full := []wire.SessionAnswer{{Choice: "1"}, {Choice: "1"}, {Choice: "1"}}
	if res := answerCodex(t, f, "call_old", full...); !res.Refused || res.Reason != wire.ReasonStale {
		t.Fatalf("stale answer = %+v, want %s", res, wire.ReasonStale)
	}
	// No answer for the last question, and no dismiss.
	if res := answerCodex(t, f, card.ID, full[:2]...); !res.Refused || res.Reason != wire.ReasonSessionBadRequest {
		t.Fatalf("partial answer = %+v, want %s", res, wire.ReasonSessionBadRequest)
	}
	if res := answerCodex(t, f, card.ID, wire.SessionAnswer{Choice: strconv.Itoa(9)}, full[1], full[2]); !res.Refused {
		t.Fatalf("answer with a choice that is no option = %+v", res)
	}
	wantKeys(t, f)
}
