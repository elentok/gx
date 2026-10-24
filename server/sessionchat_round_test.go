package server_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/agentfake"
)

func assistantText(uuid, text string) string {
	b, _ := json.Marshal(text)
	return fmt.Sprintf(`{"type":"assistant","uuid":%q,"timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"text","text":%s}]}}`, uuid, b)
}

func personPrompt(uuid, text string) string {
	b, _ := json.Marshal(text)
	return fmt.Sprintf(`{"type":"user","uuid":%q,"timestamp":"2026-01-01T10:00:01Z","message":{"content":%s}}`, uuid, b)
}

func chatOf(t *testing.T, lines ...string) server.SessionChatResponse {
	t.Helper()
	h, ctx, _, id, _, path := startChatSession(t, string(agentfake.HarnessKind))
	writeTranscript(t, path, lines...)
	got, err := h.Client.SessionChat(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

const goodRound = "Intro\n\n" +
	"❓ **Q1** - **Which store?**: SQLite or files?\n\n➡️ SQLite\n\n```gx-question\ntype: options\nrecommended: a\noptions:\n  - id: a\n    label: SQLite\n  - id: b\n    label: Files\n```\n\n---\n\n" +
	"❓ **Q2** - **Keep going?**: Ok?\n\n➡️ Yes\n\n```gx-question\ntype: yes-no\nrecommended: yes\n```"

func TestSessionChat_RoundParsesCardsAndStripsFences(t *testing.T) {
	got := chatOf(t, assistantText("a1", goodRound))
	if len(got.Round) != 2 {
		t.Fatalf("round = %+v, want 2 cards", got.Round)
	}
	c1, c2 := got.Round[0], got.Round[1]
	if c1.Label != "1" || c1.Title != "Which store?" || c1.Kind != "options" || c1.Recommended != "a" || len(c1.Options) != 2 || c1.Note != "" {
		t.Errorf("card 1 = %+v", c1)
	}
	if c2.Label != "2" || c2.Title != "Keep going?" || c2.Kind != "yes-no" || c2.Recommended != "yes" {
		t.Errorf("card 2 = %+v", c2)
	}
	if strings.Contains(c1.Body, "---") || strings.Contains(c1.Body, "gx-question") || !strings.Contains(c1.Body, "➡️ SQLite") {
		t.Errorf("card 1 body = %q, want the text up to the rule, without the fence", c1.Body)
	}
	if text := got.Messages[0].Text; strings.Contains(text, "gx-question") || !strings.Contains(text, "Intro") {
		t.Errorf("message text = %q, want fences stripped", text)
	}
}

func TestSessionChat_RoundReadsDriftedHeadersLeniently(t *testing.T) {
	yesNo := "\n\n```gx-question\ntype: yes-no\n```"
	got := chatOf(t, assistantText("a1",
		"❓ **Q1 - Plain title**: body"+yesNo+"\n\n"+
			"❓ **Q2** Third thing? More"+yesNo+"\n\n"+
			"❓ **Q3** - **Bold title** body"+yesNo))
	var titles []string
	for _, c := range got.Round {
		titles = append(titles, fmt.Sprintf("%s=%s", c.Label, c.Title))
	}
	if want := "1=Plain title,2=Third thing?,3=Bold title"; strings.Join(titles, ",") != want {
		t.Errorf("cards = %v, want %s", titles, want)
	}
}

func TestSessionChat_RoundBadOrMissingBlockGivesFreeTextCard(t *testing.T) {
	good := "\n\n```gx-question\ntype: yes-no\nrecommended: yes\n```"
	got := chatOf(t, assistantText("a1",
		"❓ **Q1** - **Bad**: x\n\n```gx-question\ntype: options\n```\n\n---\n\n"+
			"❓ **Q2** - **Missing**: y\n\n---\n\n"+
			"❓ **Q3** - **Twice**: z"+good+good+"\n\n---\n\n"+
			"❓ **Q4** - **Fine**: w"+good))
	if len(got.Round) != 4 {
		t.Fatalf("round = %+v, want 4 cards", got.Round)
	}
	for _, c := range got.Round[:3] {
		if c.Kind != "text" || c.Note == "" || len(c.Options) != 0 || c.Recommended != "" {
			t.Errorf("card %s = %+v, want a free-text card with a note", c.Label, c)
		}
	}
	if c := got.Round[3]; c.Kind != "yes-no" || c.Recommended != "yes" || c.Note != "" {
		t.Errorf("card 4 = %+v, want it answerable", c)
	}
}

func TestSessionChat_RoundIsNullOnceThePersonAnswers(t *testing.T) {
	got := chatOf(t, assistantText("a1", goodRound), personPrompt("u1", "yes to both"))
	if got.Round != nil {
		t.Errorf("round = %+v, want null after a person prompt", got.Round)
	}
	got = chatOf(t, assistantText("a1", goodRound),
		`{"type":"assistant","uuid":"a2","timestamp":"2026-01-01T10:00:02Z","message":{"content":[{"type":"tool_use","id":"t","name":"Bash"}]}}`)
	if len(got.Round) != 2 {
		t.Errorf("round = %+v, want it kept when a tool call follows", got.Round)
	}
}

func TestSessionChat_NextReadsTheRunEndLine(t *testing.T) {
	for _, next := range []string{"map — go on", "anything the agent picks", "implement"} {
		got := chatOf(t, assistantText("a1", fmt.Sprintf("Done.\n\n✅ Design: all good. Next: %s", next)))
		if got.Next == nil || got.Next.Action.Reason == "" || len(got.Next.Action.Action) == 0 {
			t.Errorf("Next: %s => %+v, want a Continue action", next, got.Next)
		}
	}
	if got := chatOf(t, assistantText("a1", "no run end here")); got.Next != nil {
		t.Errorf("next = %+v, want null without a run-end line", got.Next)
	}
}

func TestSessionChat_BackgroundListsLiveTasksAndDropsAgedOutOnes(t *testing.T) {
	now := time.Now().UTC()
	ts := func(ago time.Duration) string { return now.Add(-ago).Format(time.RFC3339Nano) }
	launch := func(use, input, result, ago string) []string {
		return []string{
			fmt.Sprintf(`{"type":"assistant","uuid":"a-%s","timestamp":%q,"message":{"content":[{"type":"tool_use","id":%q,"name":"X","input":%s}]}}`, use, ts(time.Minute), use, input),
			fmt.Sprintf(`{"type":"user","uuid":"r-%s","timestamp":%q,"toolUseResult":%s,"message":{"content":[{"type":"tool_result","tool_use_id":%q,"content":"ok"}]}}`, use, ts(time.Minute), result, use),
		}
	}
	long := strings.Repeat("x", 80)
	var lines []string
	lines = append(lines, launch("t1", `{"command":"`+long+`","description":""}`, `{"backgroundTaskId":"b1"}`, "")...)
	lines = append(lines, launch("t2", `{"description":"watch the logs","command":"tail -f"}`, `{"taskId":"m1","timeoutMs":3600000}`, "")...)
	lines = append(lines, launch("t3", `{"description":"survey code"}`, `{"status":"async_launched","agentId":"g1"}`, "")...)
	old := launch("t4", `{"description":"stale"}`, `{"backgroundTaskId":"b4"}`, "")
	for i := range old {
		old[i] = strings.ReplaceAll(old[i], ts(time.Minute), ts(3*time.Hour))
	}
	lines = append(lines, old...)

	got := chatOf(t, lines...)
	var seen []string
	for _, b := range got.Background {
		seen = append(seen, b.Kind+":"+b.Label)
	}
	want := "shell:" + long[:60] + ",monitor:watch the logs,agent:survey code"
	if strings.Join(seen, ",") != want {
		t.Errorf("background = %v, want %s", seen, want)
	}
	if got.Background[0].StartedAt == "" {
		t.Error("background task has no started_at")
	}
}
