package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/elentok/gx/server/wire"
)

// Tickets is the sub-client for ticket, epic and live-agent writes. It shares
// Client's transport.
type Tickets struct {
	t *transport
}

// Pause pauses a ticket with a required one-line reason.
func (s *Tickets) Pause(ctx context.Context, address, reason string) (wire.QueueResult, error) {
	return s.t.queueWriteTo(ctx, "/v1/tickets/pause", wire.QueueRequest{Address: address, PauseReason: reason})
}

// Unmute clears a ticket's mutes and reopens it if it was paused for repair.
func (s *Tickets) Unmute(ctx context.Context, address string) (wire.QueueResult, error) {
	return s.t.queueWriteTo(ctx, "/v1/tickets/unmute", wire.QueueRequest{Address: address})
}

// Cancel cancels a ticket and its non-terminal fork descendants; stop
// lets it stop a live pane first.
func (s *Tickets) Cancel(ctx context.Context, address string, stop bool) (wire.QueueResult, error) {
	return s.t.queueWriteTo(ctx, "/v1/tickets/cancel", wire.QueueRequest{Address: address, Stop: stop})
}

// Approve runs the ticket's latest recovery proposal.
func (s *Tickets) Approve(ctx context.Context, address string) (wire.QueueResult, error) {
	return s.t.queueWriteTo(ctx, "/v1/tickets/approve", wire.QueueRequest{Address: address})
}

// Retry starts a fresh agent on a ticket's branch, keeping its commits;
// stop lets it stop a working leftover pane first.
func (s *Tickets) Retry(ctx context.Context, address string, stop bool) (wire.QueueResult, error) {
	return s.t.queueWriteTo(ctx, "/v1/tickets/retry", wire.QueueRequest{Address: address, Stop: stop})
}

// Relaunch is the old name of Retry.
func (s *Tickets) Relaunch(ctx context.Context, address string, stop bool) (wire.QueueResult, error) {
	return s.Retry(ctx, address, stop)
}

// EpicMerge triggers a manual merge of an epic address: a direct land
// (Merged) or a forked merge ticket (MergeTicket).
func (s *Tickets) EpicMerge(ctx context.Context, address string) (wire.QueueResult, error) {
	return s.t.queueWriteTo(ctx, "/v1/epics/merge", wire.QueueRequest{Address: address})
}

// EpicStashMerge is "Stash, merge and restore" for an epic paused on a dirty
// trunk: the target is fast-forwarded and the uncommitted files come back with
// conflict markers, behind a trunk-conflict ticket (TrunkConflict).
func (s *Tickets) EpicStashMerge(ctx context.Context, address string) (wire.QueueResult, error) {
	return s.t.queueWriteTo(ctx, "/v1/epics/stash-merge", wire.QueueRequest{Address: address})
}

// Ask records an agent's question (a "## Question" + "## Handoff" body)
// on a ticket and pauses it asking.
func (s *Tickets) Ask(ctx context.Context, address, body string) (wire.QueueResult, error) {
	return s.t.queueWriteTo(ctx, "/v1/tickets/ask", wire.QueueRequest{Address: address, Text: body})
}

// Recommend stores an investigate agent's diagnosis and recommended verb
// on the paused ticket it investigates.
func (s *Tickets) Recommend(ctx context.Context, address, verb, diagnosis string) (wire.QueueResult, error) {
	return s.t.queueWriteTo(ctx, "/v1/tickets/recommend", wire.QueueRequest{Address: address, Verb: verb, Text: diagnosis})
}

// Nudge types text into the live pane of a ticket's iteration.
func (s *Tickets) Nudge(ctx context.Context, address, text string) (wire.QueueResult, error) {
	return s.t.queueWriteTo(ctx, "/v1/tickets/nudge", wire.QueueRequest{Address: address, Text: text})
}

// Answer delivers a person's reply to the live agent that asked. The
// request carries the address, the state token, the choice or free text, a note
// and the source.
func (s *Tickets) Answer(ctx context.Context, req wire.QueueRequest) (wire.QueueResult, error) {
	return s.t.queueWriteTo(ctx, "/v1/tickets/answer", req)
}

// AgentPrompt sends text to the live agent of a ticket's iteration.
func (s *Tickets) AgentPrompt(ctx context.Context, address, text string) (wire.QueueResult, error) {
	return s.t.queueWriteTo(ctx, "/v1/agents/prompt", wire.QueueRequest{Address: address, Text: text})
}

// AgentInterrupt stops the current turn of a ticket's live agent.
func (s *Tickets) AgentInterrupt(ctx context.Context, address string) (wire.QueueResult, error) {
	return s.t.queueWriteTo(ctx, "/v1/agents/interrupt", wire.QueueRequest{Address: address})
}

// AgentAnswer allows or denies the permission request a ticket's agent waits on.
func (s *Tickets) AgentAnswer(ctx context.Context, address, decision, text string) (wire.QueueResult, error) {
	return s.AgentAnswerRequest(ctx, wire.QueueRequest{Address: address, Decision: decision, Text: text})
}

// AgentAnswerRequest is AgentAnswer with the full request: the request ID of the
// prompt being answered and the source of the decision.
func (s *Tickets) AgentAnswerRequest(ctx context.Context, req wire.QueueRequest) (wire.QueueResult, error) {
	return s.t.queueWriteTo(ctx, "/v1/agents/answer", req)
}

// Screen reads a loop ticket agent's screen as plain text rows.
func (s *Tickets) Screen(ctx context.Context, address string) (wire.SessionScreenResponse, error) {
	var res wire.SessionScreenResponse
	err := s.t.get(ctx, "/v1/sessions/screen?ticket="+url.QueryEscape(address), &res)
	return res, err
}

// Changed pings the server that a direct write changed address's file.
func (s *Tickets) Changed(ctx context.Context, address string) error {
	t := s.t
	if err := t.checkWrite(); err != nil {
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
	t.setOrigin(hreq)
	resp, err := t.http.Do(hreq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("server returned %s for /v1/tickets/changed", resp.Status)
	}
	return nil
}
