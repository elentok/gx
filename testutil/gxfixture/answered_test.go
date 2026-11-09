package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
)

func TestAnswerFinishedScenario_AnsweringAFinishedAgentIsAdopted(t *testing.T) {
	t.Parallel()
	r := start(t, "answer-finished")
	const addr = "demo:finished-answer-epic/01"
	r.waitStatus(t, addr, askedStatus)
	d := r.detail(t, addr)
	if err := os.Remove(r.HoldFile); err != nil {
		t.Fatal(err)
	}
	// The late finish is adopted on its own soon after; the answer must meet it first.
	// The agent's second finish lands a moment after the release; until then the
	// answer is refused, and once the adoption ends the ticket is done. Poll
	// for the answer that meets the late finish in between.
	var data string
	web := r.web(t)
	if !testutil.Poll(60*time.Second, 50*time.Millisecond, func() bool {
		data, _ = web.try(t, "/v1/tickets/answer", map[string]any{"address": addr, "state": d.State, "text": "go on"})
		return !strings.Contains(data, "not-answerable")
	}) {
		t.Fatalf("answer never left not-answerable: %s", data)
	}
	if !strings.Contains(data, `"outcome":"adopted"`) {
		t.Fatalf("answer = %s, want outcome adopted", data)
	}
	r.waitStatus(t, addr, "done")
	r.stop(t)
}

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
