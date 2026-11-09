package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
)

// Serial on purpose: it holds a fixture slot for the two launches it waits on, and run alongside the
// parallel start-and-stop smoke tests it starves the last of them past their time limit.
func TestAnswerFinishedScenario_AnsweringAFinishedAgentIsAdopted(t *testing.T) {
	r := start(t, "answer-finished")
	const addr = "demo:finished-answer-epic/01"
	r.waitStatus(t, addr, askedStatus)
	// The token of the first ask, as an open Answer form holds it.
	d := r.detail(t, addr)
	r.gx(t, "server", "tickets", "resume", addr)
	pauses := func() int { return strings.Count(r.gx(t, "server", "tickets", "history", addr), "\tpaused\t") }
	if !testutil.Poll(90*time.Second, 100*time.Millisecond, func() bool { return pauses() == 2 }) {
		t.Fatalf("the relaunched agent never paused the ticket again (%d pauses)", pauses())
	}
	if err := os.Remove(r.HoldFile); err != nil {
		t.Fatal(err)
	}
	// The agent's second finish lands a moment after the release; until then the
	// answer is refused, and once the adoption ends the ticket is done. Poll
	// for the answer that meets the late finish in between.
	var data string
	web := r.web(t)
	if !testutil.Poll(60*time.Second, 50*time.Millisecond, func() bool {
		data, _ = web.try(t, "/v1/tickets/answer", map[string]any{"address": addr, "state": d.State, "text": "go on"})
		return !strings.Contains(data, "not-answerable") && !strings.Contains(data, `"stale"`)
	}) {
		t.Fatalf("answer never met the late finish: %s", data)
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
