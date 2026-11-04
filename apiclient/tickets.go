package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/elentok/gx/cpuwatch"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
)

// OneOff creates a top-level ticket from a prompt and queues it. A refusal is a
// result (Refused set), not an error.
func (c *Client) OneOff(ctx context.Context, req wire.OneOffRequest) (wire.OneOffResult, error) {
	var res wire.OneOffResult
	err := c.post(ctx, "/v1/oneoff", req, &res)
	return res, err
}

// CPUSpike reports a TUI's CPU spike; the server decides whether to file a ticket.
func (c *Client) CPUSpike(ctx context.Context, spike cpuwatch.Spike) (server.CPUSpikeResult, error) {
	var res server.CPUSpikeResult
	err := c.post(ctx, "/v1/cpu-spike", spike, &res)
	return res, err
}

// TicketPause pauses a ticket with a required one-line reason.
func (c *Client) TicketPause(ctx context.Context, address, reason string) (wire.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/tickets/pause", wire.QueueRequest{Address: address, PauseReason: reason})
}

// TicketUnmute clears a ticket's mutes and reopens it if it was paused for repair.
func (c *Client) TicketUnmute(ctx context.Context, address string) (wire.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/tickets/unmute", wire.QueueRequest{Address: address})
}

// TicketCancel cancels a ticket and its non-terminal fork descendants; stop
// lets it stop a live pane first.
func (c *Client) TicketCancel(ctx context.Context, address string, stop bool) (wire.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/tickets/cancel", wire.QueueRequest{Address: address, Stop: stop})
}

// TicketApprove runs the ticket's latest recovery proposal.
func (c *Client) TicketApprove(ctx context.Context, address string) (wire.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/tickets/approve", wire.QueueRequest{Address: address})
}

// TicketRetry starts a fresh agent on a ticket's branch, keeping its commits;
// stop lets it stop a working leftover pane first.
func (c *Client) TicketRetry(ctx context.Context, address string, stop bool) (wire.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/tickets/retry", wire.QueueRequest{Address: address, Stop: stop})
}

// TicketRelaunch is the old name of TicketRetry.
func (c *Client) TicketRelaunch(ctx context.Context, address string, stop bool) (wire.QueueResult, error) {
	return c.TicketRetry(ctx, address, stop)
}

// EpicMerge triggers a manual merge of an epic address: a direct land
// (Merged) or a forked merge ticket (MergeTicket).
func (c *Client) EpicMerge(ctx context.Context, address string) (wire.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/epics/merge", wire.QueueRequest{Address: address})
}

// EpicStashMerge is "Stash, merge and restore" for an epic paused on a dirty
// trunk: the target is fast-forwarded and the uncommitted files come back with
// conflict markers, behind a trunk-conflict ticket (TrunkConflict).
func (c *Client) EpicStashMerge(ctx context.Context, address string) (wire.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/epics/stash-merge", wire.QueueRequest{Address: address})
}

// TicketAsk records an agent's question (a "## Question" + "## Handoff" body)
// on a ticket and pauses it asking.
func (c *Client) TicketAsk(ctx context.Context, address, body string) (wire.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/tickets/ask", wire.QueueRequest{Address: address, Text: body})
}

// TicketRecommend stores an investigate agent's diagnosis and recommended verb
// on the paused ticket it investigates.
func (c *Client) TicketRecommend(ctx context.Context, address, verb, diagnosis string) (wire.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/tickets/recommend", wire.QueueRequest{Address: address, Verb: verb, Text: diagnosis})
}

// TicketNudge types text into the live pane of a ticket's iteration.
func (c *Client) TicketNudge(ctx context.Context, address, text string) (wire.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/tickets/nudge", wire.QueueRequest{Address: address, Text: text})
}

// TicketAnswer delivers a person's reply to the live agent that asked. The
// request carries the address, the state token, the choice or free text, a note
// and the source.
func (c *Client) TicketAnswer(ctx context.Context, req wire.QueueRequest) (wire.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/tickets/answer", req)
}

// AgentPrompt sends text to the live agent of a ticket's iteration.
func (c *Client) AgentPrompt(ctx context.Context, address, text string) (wire.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/agents/prompt", wire.QueueRequest{Address: address, Text: text})
}

// AgentInterrupt stops the current turn of a ticket's live agent.
func (c *Client) AgentInterrupt(ctx context.Context, address string) (wire.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/agents/interrupt", wire.QueueRequest{Address: address})
}

// AgentAnswer allows or denies the permission request a ticket's agent waits on.
func (c *Client) AgentAnswer(ctx context.Context, address, decision, text string) (wire.QueueResult, error) {
	return c.AgentAnswerRequest(ctx, wire.QueueRequest{Address: address, Decision: decision, Text: text})
}

// AgentAnswerRequest is AgentAnswer with the full request: the request ID of the
// prompt being answered and the source of the decision.
func (c *Client) AgentAnswerRequest(ctx context.Context, req wire.QueueRequest) (wire.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/agents/answer", req)
}

// TicketChanged pings the server that a direct write changed address's file.
func (c *Client) TicketChanged(ctx context.Context, address string) error {
	if err := c.CheckWrite(); err != nil {
		return err
	}
	body, err := json.Marshal(wire.ChangedRequest{Address: address})
	if err != nil {
		return err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://gx/v1/tickets/changed", bytes.NewReader(body))
	if err != nil {
		return err
	}
	hreq.Header.Set("Content-Type", "application/json")
	c.setOrigin(hreq)
	resp, err := c.http.Do(hreq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("server returned %s for /v1/tickets/changed", resp.Status)
	}
	return nil
}
