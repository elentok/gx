package server_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

const investigateChildFile = "01a-investigate.md"

// writeInvestigateChild forks an investigate child off ticket 01 beside
// ticketPath, with the given status.
func writeInvestigateChild(t *testing.T, ticketPath, status string) string {
	t.Helper()
	dir := filepath.Dir(ticketPath)
	testutil.WriteFile(t, dir, investigateChildFile, "---\nid: \"01a\"\nparent: \"01\"\nstatus: "+status+"\ntype: investigate\n---\n\n# 01a\n")
	return filepath.Join(dir, investigateChildFile)
}

func wantChildStatus(t *testing.T, childPath string, want schema.Status) {
	t.Helper()
	tk, err := schema.ParseTicket(childPath)
	if err != nil || tk.Status != want {
		t.Errorf("investigate child = %q, %v; want %s", tk.Status, err, want)
	}
}

func TestPersonVerb_MergeCancelsTheOpenInvestigateChild(t *testing.T) {
	h, ticketPath := pausedIteration(t, string(events.ZeroCommit), true)
	child := writeInvestigateChild(t, ticketPath, "open")
	res, err := h.Client.Repair(context.Background(), "merge", server.RepairRequest{Address: "proj:epic-a/01", Cwd: t.TempDir()})
	if err != nil || res.Refused {
		t.Fatalf("merge = %+v, %v; want merged", res, err)
	}
	wantChildStatus(t, child, schema.StatusCancelled)
}

func TestPersonVerb_StartOverCancelsTheOpenInvestigateChild(t *testing.T) {
	h, ticketPath := pausedIteration(t, string(events.ZeroCommit), true)
	child := writeInvestigateChild(t, ticketPath, "open")
	res, err := h.Client.Repair(context.Background(), "start-over", server.RepairRequest{Address: "proj:epic-a/01", Reason: "retry"})
	if err != nil || res.Refused {
		t.Fatalf("start-over = %+v, %v; want started over", res, err)
	}
	wantChildStatus(t, child, schema.StatusCancelled)
}

func TestInvestigateVerb_MergeLeavesItsOwnTicket(t *testing.T) {
	h, ticketPath := pausedIteration(t, string(events.ZeroCommit), true)
	child := writeInvestigateChild(t, ticketPath, "open")
	res, err := h.Client.AsInvestigator("proj:epic-a/01a").Repair(context.Background(), "merge", server.RepairRequest{Address: "proj:epic-a/01", Cwd: t.TempDir()})
	if err != nil || res.Refused {
		t.Fatalf("merge = %+v, %v; want merged", res, err)
	}
	wantChildStatus(t, child, schema.StatusOpen)
}

func TestInvestigateVerb_StartOverLeavesItsOwnTicket(t *testing.T) {
	h, ticketPath := pausedIteration(t, string(events.ZeroCommit), true)
	child := writeInvestigateChild(t, ticketPath, "open")
	res, err := h.Client.AsInvestigator("proj:epic-a/01a").Repair(context.Background(), "start-over", server.RepairRequest{Address: "proj:epic-a/01", Reason: "retry"})
	if err != nil || res.Refused {
		t.Fatalf("start-over = %+v, %v; want started over", res, err)
	}
	wantChildStatus(t, child, schema.StatusOpen)
}

func TestWebStartOver_EmptyReasonRefusalNamesNoCLIFlag(t *testing.T) {
	h, _ := pausedIteration(t, string(events.ZeroCommit), true)
	res, err := h.Server.StartOverAsWeb(server.RepairRequest{Address: "proj:epic-a/01", State: "tok"})
	if err != nil || !res.Refused || res.Reason != repair.ReasonReasonRequired {
		t.Fatalf("start-over = %+v, %v; want %s refusal", res, err, repair.ReasonReasonRequired)
	}
	if strings.Contains(res.Message, "--reason") {
		t.Errorf("message = %q, must not mention a CLI flag", res.Message)
	}
}

func TestPersonVerb_RefusedMergeLeavesTheInvestigateChild(t *testing.T) {
	h, ticketPath := pausedIteration(t, string(events.SelfReported), true)
	child := writeInvestigateChild(t, ticketPath, "open")
	res, err := h.Client.Repair(context.Background(), "merge", server.RepairRequest{Address: "proj:epic-a/01"})
	if err != nil || !res.Refused {
		t.Fatalf("merge = %+v, %v; want refusal", res, err)
	}
	wantChildStatus(t, child, schema.StatusOpen)
}

func TestPersonVerb_AnswerCancelsTheOpenInvestigateChild(t *testing.T) {
	h, ticketPath, _ := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	child := writeInvestigateChild(t, ticketPath, "open")
	state := pauseWithQuestion(t, ticketPath, optionsBlock)
	res, err := h.Client.TicketAnswer(context.Background(), server.QueueRequest{Address: answerAddr, State: state, Choice: "a"})
	if err != nil || res.Refused {
		t.Fatalf("answer = %+v, %v; want delivered", res, err)
	}
	wantChildStatus(t, child, schema.StatusCancelled)
}

func TestPersonVerb_RetryCancelsTheOpenInvestigateChild(t *testing.T) {
	h := startInvestigateEnv(t, events.IterationError)
	if res := retryAs(t, h, ""); res.Refused {
		t.Fatalf("retry refused: %+v", res)
	}
	wantChildStatus(t, filepath.Join(h.TicketStore, "proj", "epic-a", "issues", "02-probe.md"), schema.StatusCancelled)
}

func TestInvestigateRetry_LeavesItsOwnTicketRunning(t *testing.T) {
	h := startInvestigateEnv(t, events.IterationError)
	if res := retryAs(t, h, investigateAddr); res.Refused {
		t.Fatalf("retry refused: %+v", res)
	}
	wantChildStatus(t, filepath.Join(h.TicketStore, "proj", "epic-a", "issues", "02-probe.md"), schema.StatusOpen)
}

func TestPersonVerb_RetryLeavesADoneInvestigateChild(t *testing.T) {
	h := startInvestigateEnv(t, events.IterationError)
	child := filepath.Join(h.TicketStore, "proj", "epic-a", "issues", "02-probe.md")
	if err := schema.UpdateTicket(child, func(tk *schema.Ticket) { tk.Status = schema.StatusDone }); err != nil {
		t.Fatal(err)
	}
	if res := retryAs(t, h, ""); res.Refused {
		t.Fatalf("retry refused: %+v", res)
	}
	wantChildStatus(t, child, schema.StatusDone)
}
