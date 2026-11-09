package main

import (
	"strings"
	"testing"
)

func TestAnswerQueuedScenario_AnsweringTheGoneAgentQueuesTheTicket(t *testing.T) {
	t.Parallel()
	r := start(t, "answer-queued")
	const addr = "demo:queued-answer-epic/01"
	r.waitStatus(t, addr, askedStatus)
	d := r.detail(t, addr)
	data, ok := r.web(t).try(t, "/v1/tickets/answer", map[string]any{"address": addr, "state": d.State, "text": "go on"})
	if !ok {
		t.Fatalf("answer refused: %s", data)
	}
	if !strings.Contains(data, `"outcome":"queued"`) {
		t.Fatalf("answer = %s, want outcome queued", data)
	}
	r.waitStatus(t, addr, "done")
	r.stop(t)
}
