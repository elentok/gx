package webflows

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/gxfixture/chatcapture"
	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
)

// captured is every line of the capture file so far.
func captured(path string) []chatcapture.Line {
	data, _ := os.ReadFile(path)
	var got []chatcapture.Line
	for l := range strings.SplitSeq(string(data), "\n") {
		var line chatcapture.Line
		if json.Unmarshal([]byte(l), &line) == nil {
			got = append(got, line)
		}
	}
	return got
}

// waitForParts waits until, for every part, a Slack line holds all its needles.
// Needles are the ticket address and the kind wording, not the layout.
// It polls only for what is left of the test's budget (the fixture start and
// the launch-slot wait came out of it first), so it gives up and prints before
// the hang limit kills the run.
func waitForParts(t *testing.T, r *running, want map[string][]string) {
	t.Helper()
	if msg := pollParts(r.ChatCapture, want, r.BudgetLeft(), r.StartUsed()); msg != "" {
		t.Fatal(msg)
	}
}

// pollParts is waitForParts without a test: it returns "" once every part is
// there, or the failure message when within runs out.
func pollParts(capture string, want map[string][]string, within, startUsed time.Duration) string {
	var got []chatcapture.Line
	missing := func() []string {
		got = captured(capture)
		var out []string
		for part, needles := range want {
			found := false
			for _, l := range got {
				if l.Transport != "slack" {
					continue
				}
				all := true
				for _, n := range needles {
					all = all && strings.Contains(strings.ToLower(l.Text), strings.ToLower(n))
				}
				found = found || all
			}
			if !found {
				out = append(out, part)
			}
		}
		return out
	}
	if !testutil.Poll(within, 200*time.Millisecond, func() bool { return len(missing()) == 0 }) {
		return fmt.Sprintf("no capture line for %v after polling %s (the fixture start used %s of the %s test budget); got %+v",
			missing(), within, startUsed, fixturetest.TestBudget, got)
	}
	return ""
}

func TestWaitForParts_StopsWhenTheBudgetIsSpent(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "capture.jsonl")
	line, _ := json.Marshal(chatcapture.Line{Transport: "slack", Text: "hello there"})
	if err := os.WriteFile(path, append(line, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	began := time.Now()
	msg := pollParts(path, map[string][]string{"missing-part": {"nope"}}, 300*time.Millisecond, 45*time.Second)
	if took := time.Since(began); took > 5*time.Second {
		t.Fatalf("poll ran %s, want it to stop with the spent budget", took)
	}
	for _, want := range []string{"missing-part", "hello there", "45s", "50s"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q lacks %q", msg, want)
		}
	}
}

func TestChatTicketsScenario_EveryProducibleKindLandsInTheCaptureFile(t *testing.T) {
	t.Parallel()
	r := start(t, "chat-tickets")
	waitForParts(t, r, map[string][]string{
		"answer-question": {"ask-epic/01", "Answer", "Which option should the fake worker take?"},
		"repair-blocked":  {"menu-epic/01", "permission prompt"},
		"repair-fallback": {"manual-epic/01", "Repair", "the operator set this ticket aside"},
	})
}

func TestChatEpicsScenario_EachEpicOutcomeLandsInTheCaptureFile(t *testing.T) {
	t.Parallel()
	r := start(t, "chat-epics")
	waitForParts(t, r, map[string][]string{
		"epic-merged":     {"🎉 Merged merged-chat-epic", "tickets merged in"},
		"epic-unmerged":   {"✅ Complete unmerged-chat-epic", "Not merged."},
		"epic-escalation": {"Escalated stuck-chat-epic"},
	})
}

func TestChatSessionsScenario_EachSessionStateAlertLandsInTheCaptureFile(t *testing.T) {
	t.Parallel()
	r := start(t, "chat-sessions")
	waitForParts(t, r, map[string][]string{
		"done":         {"🔔 Done", "The agent finished and waits for you."},
		"blocked":      {"🔔 Blocked", "permission"},
		"rate-limited": {"🔔 Rate-limited", "Claude hit its usage limit."},
		"exited":       {"🔔 Exited", "The session ended."},
	})
}

func TestChatTicketsRepairScenario_StallPauseLandsInTheCaptureFile(t *testing.T) {
	t.Parallel()
	r := start(t, "chat-tickets-stall")
	// Recovery gives up on the stalled ticket and files an investigate ticket
	// in the same epic, so the Repair line names the epic, not ticket 01.
	waitForParts(t, r, map[string][]string{
		"repair-stall": {"stall-chat-epic/", "Repair", "did not accept its prompt"},
	})
}

func slackPosts(path string) int {
	n := 0
	for _, l := range captured(path) {
		if l.Transport == "slack" {
			n++
		}
	}
	return n
}

func TestChatBatchScenario_MessagesQueuedInOneTickArriveInOnePost(t *testing.T) {
	t.Parallel()
	r := start(t, "chat-batch")
	// The worker's question is the last startup message, so once its post is in
	// the capture the next tick is a few seconds away.
	waitForParts(t, r, map[string][]string{"answer": {"ask-epic/01", "Answer"}})
	before := slackPosts(r.ChatCapture)
	r.Gx(t, "server", "tickets", "pause", "demo:manual-epic/01", "--reason", "set aside for the batch check")
	r.SetDisk(t, "free", "5G")
	waitForParts(t, r, map[string][]string{"batch": {"manual-epic/01", "Repair", "low disk space"}})
	if got := slackPosts(r.ChatCapture) - before; got != 1 {
		t.Fatalf("%d posts after the first, want 1", got)
	}
}
