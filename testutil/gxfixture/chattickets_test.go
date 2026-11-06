package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/gxfixture/chatcapture"
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
func waitForParts(t *testing.T, r *running, want map[string][]string) {
	t.Helper()
	var got []chatcapture.Line
	missing := func() []string {
		got = captured(r.ChatCapture)
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
	if !testutil.Poll(60*time.Second, 200*time.Millisecond, func() bool { return len(missing()) == 0 }) {
		t.Fatalf("no capture line for %v; got %+v", missing(), got)
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
		"epic-merged":     {"merged-chat-epic"},
		"epic-unmerged":   {"unmerged-chat-epic", "Not merged"},
		"epic-escalation": {"stuck-chat-epic", "recovery escalated"},
	})
}

func TestChatTicketsRepairScenario_StallPauseLandsInTheCaptureFile(t *testing.T) {
	t.Parallel()
	r := start(t, "chat-tickets-stall")
	waitForParts(t, r, map[string][]string{
		"repair-stall": {"stall-chat-epic/01", "Repair", "prompt not delivered"},
	})
}
