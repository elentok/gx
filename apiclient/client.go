// Package apiclient is the CLI's (and test harness's) only way to call the
// orchestrator server.
package apiclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/elentok/gx/cpuwatch"
	"github.com/elentok/gx/server"
)

type Client struct {
	http     *http.Client
	readOnly bool
}

// ErrReadOnly is returned by CheckWrite after Negotiate found an API version
// mismatch: the wire contract can't be trusted for writes.
var ErrReadOnly = errors.New("server API version differs from this client; run `gx server restart`")

// Negotiation is the outcome of comparing client and server versions.
type Negotiation struct {
	server.Handshake
	ReadOnly bool
	// Hint is a user-facing remedy; empty when versions agree.
	Hint string
}

// Negotiate handshakes and compares versions. An API mismatch makes the client
// read-only; a build-only mismatch just yields a hint.
func (c *Client) Negotiate(ctx context.Context, build string) (Negotiation, error) {
	h, err := c.Handshake(ctx)
	if err != nil {
		return Negotiation{}, err
	}
	n := Negotiation{Handshake: h}
	switch {
	case h.APIVersion != server.APIVersion:
		n.ReadOnly = true
		n.Hint = fmt.Sprintf("read-only: server API v%d, client API v%d; run `gx server restart`", h.APIVersion, server.APIVersion)
	case h.Build != build:
		n.Hint = fmt.Sprintf("server is an older build (%s, client %s); run `gx server restart` to upgrade", h.Build, build)
	}
	c.readOnly = n.ReadOnly
	return n, nil
}

// CheckWrite must be called before any request that mutates server state.
func (c *Client) CheckWrite() error {
	if c.readOnly {
		return ErrReadOnly
	}
	return nil
}

// IsNotRunning reports whether err means nothing listens on the socket.
func IsNotRunning(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ENOENT)
}

// New returns a client for the server listening on the unix socket.
func New(socketPath string) *Client {
	return &Client{http: &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}}}
}

// Handshake returns the server's API version and build id.
func (c *Client) Handshake(ctx context.Context) (server.Handshake, error) {
	var h server.Handshake
	err := c.get(ctx, "/v1/handshake", &h)
	return h, err
}

// Snapshot returns every indexed ticket plus the server-wide sequence number.
func (c *Client) Snapshot(ctx context.Context) (server.Snapshot, error) {
	var s server.Snapshot
	err := c.get(ctx, "/v1/snapshot", &s)
	return s, err
}

// Events subscribes to the event stream from seq since (use Snapshot's Seq).
// The channel closes when the stream ends: the server dropped this client for
// lagging, stopped, or ctx was cancelled. Re-snapshot and resubscribe then.
func (c *Client) Events(ctx context.Context, since uint64) (<-chan server.Event, error) {
	return c.EventsAs(ctx, since, "")
}

// EventsAs is Events for a client that reports focus: its reports count
// while this stream is open.
func (c *Client) EventsAs(ctx context.Context, since uint64, client string) (<-chan server.Event, error) {
	path := fmt.Sprintf("/v1/events?since=%d", since)
	if client != "" {
		path += "&client=" + url.QueryEscape(client)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://gx"+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("server returned %s for %s", resp.Status, path)
	}
	out := make(chan server.Event)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			data, ok := strings.CutPrefix(sc.Text(), "data: ")
			if !ok {
				continue
			}
			var ev server.Event
			if json.Unmarshal([]byte(data), &ev) != nil {
				return
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// Projects returns every project in the server's ticket store with its state.
func (c *Client) Projects(ctx context.Context) ([]server.ProjectInfo, error) {
	var p []server.ProjectInfo
	err := c.get(ctx, "/v1/projects", &p)
	return p, err
}

// AddProject registers a project. A refusal is a result (Refused set), not an
// error.
func (c *Client) AddProject(ctx context.Context, req server.AddProjectRequest) (server.AddProjectResult, error) {
	var res server.AddProjectResult
	err := c.post(ctx, "/v1/projects/add", req, &res)
	return res, err
}

// RemoveProject unregisters a project, keeping its tickets. A refusal is a
// result (Refused set), not an error.
func (c *Client) RemoveProject(ctx context.Context, req server.ProjectRequest) (server.ProjectResult, error) {
	var res server.ProjectResult
	err := c.post(ctx, "/v1/projects/remove", req, &res)
	return res, err
}

// SetProjectPath points a project at another repo. A refusal is a result.
func (c *Client) SetProjectPath(ctx context.Context, req server.ProjectRequest) (server.ProjectResult, error) {
	var res server.ProjectResult
	err := c.post(ctx, "/v1/projects/set-path", req, &res)
	return res, err
}

// Locks returns every held lock with its owner.
func (c *Client) Locks(ctx context.Context) ([]server.LockInfo, error) {
	var l []server.LockInfo
	err := c.get(ctx, "/v1/locks", &l)
	return l, err
}

// Budget returns today's spend and the configured limits.
func (c *Client) Budget(ctx context.Context) (server.BudgetStatus, error) {
	var b server.BudgetStatus
	err := c.get(ctx, "/v1/budget", &b)
	return b, err
}

// History returns the events logged for one ticket, by its full address.
func (c *Client) History(ctx context.Context, address string) (server.History, error) {
	var h server.History
	err := c.get(ctx, "/v1/tickets/history?address="+url.QueryEscape(address), &h)
	return h, err
}

// Explain returns the scheduler's verdict on one ticket, by its full address.
func (c *Client) Explain(ctx context.Context, address string) (server.Explanation, error) {
	var e server.Explanation
	err := c.get(ctx, "/v1/tickets/explain?address="+url.QueryEscape(address), &e)
	return e, err
}

// Detail returns one ticket's park detail, by its full address.
func (c *Client) Detail(ctx context.Context, address string) (server.TicketDetail, error) {
	var d server.TicketDetail
	err := c.get(ctx, "/v1/tickets/detail?address="+url.QueryEscape(address), &d)
	return d, err
}

// Epics returns every epic the Epics page lists.
func (c *Client) Epics(ctx context.Context) (server.EpicList, error) {
	var l server.EpicList
	err := c.get(ctx, "/v1/epics", &l)
	return l, err
}

// EpicDetail returns an epic's state, progress and ticket rows.
func (c *Client) EpicDetail(ctx context.Context, address string) (server.EpicDetail, error) {
	var d server.EpicDetail
	err := c.get(ctx, "/v1/epics/detail?address="+url.QueryEscape(address), &d)
	return d, err
}

// Iterations returns the live iterations.
func (c *Client) Iterations(ctx context.Context) ([]server.IterationInfo, error) {
	var l []server.IterationInfo
	err := c.get(ctx, "/v1/iterations", &l)
	return l, err
}

// Queue returns every ticket with the scheduler's verdict on it.
func (c *Client) Queue(ctx context.Context) ([]server.QueueEntry, error) {
	var l []server.QueueEntry
	err := c.get(ctx, "/v1/queue", &l)
	return l, err
}

// QueueItems returns the server-wide queue in order.
func (c *Client) QueueItems(ctx context.Context) ([]server.QueueItem, error) {
	var l []server.QueueItem
	err := c.get(ctx, "/v1/queue/items", &l)
	return l, err
}

// QueueAdd appends a ticket to the queue; agent "" takes the default.
func (c *Client) QueueAdd(ctx context.Context, address, agent string) (server.QueueResult, error) {
	return c.queueWrite(ctx, "add", server.QueueRequest{Address: address, Agent: agent})
}

// OneOff creates a top-level ticket from a prompt and queues it. A refusal is a
// result (Refused set), not an error.
func (c *Client) OneOff(ctx context.Context, req server.OneOffRequest) (server.OneOffResult, error) {
	var res server.OneOffResult
	err := c.post(ctx, "/v1/oneoff", req, &res)
	return res, err
}

// CPUSpike reports a TUI's CPU spike; the server decides whether to file a ticket.
func (c *Client) CPUSpike(ctx context.Context, spike cpuwatch.Spike) (server.CPUSpikeResult, error) {
	var res server.CPUSpikeResult
	err := c.post(ctx, "/v1/cpu-spike", spike, &res)
	return res, err
}

// QueueRemove drops a ticket from the queue.
func (c *Client) QueueRemove(ctx context.Context, address string) (server.QueueResult, error) {
	return c.queueWrite(ctx, "remove", server.QueueRequest{Address: address})
}

// QueueReplace swaps one project's pending entries for items.
func (c *Client) QueueReplace(ctx context.Context, project string, items []server.QueueItem) (server.QueueResult, error) {
	return c.queueWrite(ctx, "replace", server.QueueRequest{Project: project, Items: items})
}

// QueueMove puts a queued ticket at the 1-based position.
func (c *Client) QueueMove(ctx context.Context, address string, position int) (server.QueueResult, error) {
	return c.queueWrite(ctx, "move", server.QueueRequest{Address: address, Position: position})
}

// TicketPause pauses a ticket with a required one-line reason.
func (c *Client) TicketPause(ctx context.Context, address, reason string) (server.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/tickets/pause", server.QueueRequest{Address: address, ParkReason: reason})
}

// TicketUnmute clears a ticket's mutes and reopens it if it was parked needs-repair.
func (c *Client) TicketUnmute(ctx context.Context, address string) (server.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/tickets/unmute", server.QueueRequest{Address: address})
}

// TicketCancel cancels a ticket and its non-terminal fork descendants; stop
// lets it stop a live pane first.
func (c *Client) TicketCancel(ctx context.Context, address string, stop bool) (server.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/tickets/cancel", server.QueueRequest{Address: address, Stop: stop})
}

// TicketApprove runs the ticket's latest recovery proposal.
func (c *Client) TicketApprove(ctx context.Context, address string) (server.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/tickets/approve", server.QueueRequest{Address: address})
}

// TicketRelaunch starts a fresh iteration of a ticket; stop lets it stop a
// working leftover pane first.
func (c *Client) TicketRelaunch(ctx context.Context, address string, stop bool) (server.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/tickets/relaunch", server.QueueRequest{Address: address, Stop: stop})
}

// EpicMerge triggers a manual merge of an epic address: a direct land
// (Merged) or a forked merge ticket (MergeTicket).
func (c *Client) EpicMerge(ctx context.Context, address string) (server.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/epics/merge", server.QueueRequest{Address: address})
}

// TicketAsk records an agent's question (a "## Question" + "## Handoff" body)
// on a ticket and flags it needs-answer.
func (c *Client) TicketAsk(ctx context.Context, address, body string) (server.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/tickets/ask", server.QueueRequest{Address: address, Text: body})
}

// TicketNudge types text into the live pane of a ticket's iteration.
func (c *Client) TicketNudge(ctx context.Context, address, text string) (server.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/tickets/nudge", server.QueueRequest{Address: address, Text: text})
}

// TicketAnswer delivers a person's reply to the live agent that asked. The
// request carries the address, the state token, the choice or free text, a note
// and the source.
func (c *Client) TicketAnswer(ctx context.Context, req server.QueueRequest) (server.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/tickets/answer", req)
}

// AgentPrompt sends text to the live agent of a ticket's iteration.
func (c *Client) AgentPrompt(ctx context.Context, address, text string) (server.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/agents/prompt", server.QueueRequest{Address: address, Text: text})
}

// AgentInterrupt stops the current turn of a ticket's live agent.
func (c *Client) AgentInterrupt(ctx context.Context, address string) (server.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/agents/interrupt", server.QueueRequest{Address: address})
}

// AgentAnswer allows or denies the permission request a ticket's agent waits on.
func (c *Client) AgentAnswer(ctx context.Context, address, decision, text string) (server.QueueResult, error) {
	return c.AgentAnswerRequest(ctx, server.QueueRequest{Address: address, Decision: decision, Text: text})
}

// AgentAnswerRequest is AgentAnswer with the full request: the request ID of the
// prompt being answered and the source of the decision.
func (c *Client) AgentAnswerRequest(ctx context.Context, req server.QueueRequest) (server.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/agents/answer", req)
}

// TicketChanged pings the server that a direct write changed address's file.
func (c *Client) TicketChanged(ctx context.Context, address string) error {
	if err := c.CheckWrite(); err != nil {
		return err
	}
	body, err := json.Marshal(server.ChangedRequest{Address: address})
	if err != nil {
		return err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://gx/v1/tickets/changed", bytes.NewReader(body))
	if err != nil {
		return err
	}
	hreq.Header.Set("Content-Type", "application/json")
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

// QueuePause is Finish and pause: new roots stop starting and live runs finish.
// The pause is persisted across restarts.
func (c *Client) QueuePause(ctx context.Context) (server.QueueResult, error) {
	return c.queueWrite(ctx, "pause", server.QueueRequest{})
}

// QueueResume lifts a pause.
func (c *Client) QueueResume(ctx context.Context) (server.QueueResult, error) {
	return c.queueWrite(ctx, "resume", server.QueueRequest{})
}

// QueueDrain is an alias of QueuePause, kept for one release.
func (c *Client) QueueDrain(ctx context.Context) (server.QueueResult, error) {
	return c.queueWrite(ctx, "drain", server.QueueRequest{})
}

// queueWrite posts one queue write. A refusal is a result (Refused set), not an error.
func (c *Client) queueWrite(ctx context.Context, verb string, req server.QueueRequest) (server.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/queue/"+verb, req)
}

func (c *Client) queueWriteTo(ctx context.Context, path string, req server.QueueRequest) (server.QueueResult, error) {
	var res server.QueueResult
	err := c.post(ctx, path, req, &res)
	return res, err
}

// CreateSession starts an interactive session. A refusal is a result
// (Refused set), not an error.
func (c *Client) CreateSession(ctx context.Context, req server.CreateSessionRequest) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/create", req, &res)
	return res, err
}

// StopSession ends a live session's holder. A refusal is a result (Refused
// set), not an error.
func (c *Client) StopSession(ctx context.Context, id string) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/stop", server.SessionRequest{ID: id}, &res)
	return res, err
}

// RemoveSession deletes an Exited session's state dir. A refusal is a result
// (Refused set), not an error.
func (c *Client) RemoveSession(ctx context.Context, id string) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/remove", server.SessionRequest{ID: id}, &res)
	return res, err
}

// MarkSessionRead clears a session's Unread. A refusal is a result (Refused
// set), not an error.
func (c *Client) MarkSessionRead(ctx context.Context, id string) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/mark-read", server.SessionRequest{ID: id}, &res)
	return res, err
}

// FocusSession asks the most recently focused TUI to show a session; Client
// names it, empty with no TUI connected.
func (c *Client) FocusSession(ctx context.Context, id string) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/show", server.SessionRequest{ID: id}, &res)
	return res, err
}

// RunSessionAction starts a session's Action; it runs on in the server. A
// refusal is a result (Refused set), not an error.
func (c *Client) RunSessionAction(ctx context.Context, id string) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/run-action", server.SessionRequest{ID: id}, &res)
	return res, err
}

// ResumeSession resumes an Exited session's conversation in a new holder. A
// refusal is a result (Refused set), not an error.
func (c *Client) ResumeSession(ctx context.Context, id string) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/resume", server.SessionRequest{ID: id}, &res)
	return res, err
}

// MarkSessionActionable raises an agent's Actionable on its session. A
// refusal is a result (Refused set), not an error.
func (c *Client) MarkSessionActionable(ctx context.Context, req server.MarkActionableRequest) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/actionable", req, &res)
	return res, err
}

// LinkSession sets an agent's Session ticket. A refusal is a result (Refused
// set), not an error.
func (c *Client) LinkSession(ctx context.Context, req server.LinkSessionRequest) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/link", req, &res)
	return res, err
}

// ReportFocus sends a TUI's focus report. A refusal is a result (Refused
// set), not an error.
func (c *Client) ReportFocus(ctx context.Context, req server.FocusReport) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/focus", req, &res)
	return res, err
}

// Sessions lists the interactive sessions.
func (c *Client) Sessions(ctx context.Context) ([]server.SessionInfo, error) {
	var l []server.SessionInfo
	err := c.get(ctx, "/v1/sessions", &l)
	return l, err
}

// SetAFK sets AFK mode; a nil on toggles it.
func (c *Client) SetAFK(ctx context.Context, on *bool) (server.AFKResult, error) {
	var res server.AFKResult
	err := c.post(ctx, "/v1/afk", server.AFKRequest{On: on}, &res)
	return res, err
}

// BudgetOverride lifts the budget latches. A refusal is a result (Refused set),
// not an error.
func (c *Client) BudgetOverride(ctx context.Context) (server.BudgetResult, error) {
	var res server.BudgetResult
	err := c.post(ctx, "/v1/budget/override", struct{}{}, &res)
	return res, err
}

// ReloadConfig reloads the global config file into the server. A refusal is
// a result (Refused set), not an error.
func (c *Client) ReloadConfig(ctx context.Context) (server.ReloadResult, error) {
	var res server.ReloadResult
	err := c.post(ctx, "/v1/config/reload", struct{}{}, &res)
	return res, err
}

// ConfigStatus is the last reload result, the startup-only keys waiting for a
// restart and the hard-limit grace countdown.
func (c *Client) ConfigStatus(ctx context.Context) (server.ConfigStatus, error) {
	var st server.ConfigStatus
	err := c.get(ctx, "/v1/config/status", &st)
	return st, err
}

// BudgetIncrease raises the live budget limits until the server restarts. A
// refusal is a result (Refused set), not an error.
func (c *Client) BudgetIncrease(ctx context.Context, req server.BudgetIncreaseRequest) (server.BudgetResult, error) {
	var res server.BudgetResult
	err := c.post(ctx, "/v1/budget/increase", req, &res)
	return res, err
}

// Repair runs a repair verb ("merge", "start-over", "resume" or "verify"). A refusal
// is a result (Refused set), not an error.
func (c *Client) Repair(ctx context.Context, verb string, req server.RepairRequest) (server.RepairResult, error) {
	var res server.RepairResult
	err := c.post(ctx, "/v1/tickets/"+verb, req, &res)
	return res, err
}

func (c *Client) post(ctx context.Context, path string, req, res any) error {
	if err := c.CheckWrite(); err != nil {
		return err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://gx"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	hreq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(hreq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("server returned %s for %s: %s", resp.Status, path, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(resp.Body).Decode(res)
}

// Follow keeps a consumer in sync: it takes a snapshot, then streams events
// from that snapshot's seq. Any stream end or seq gap (and any error while
// reconnecting) triggers a fresh snapshot, so the consumer never has to
// reason about missed events. Exactly one of the callback's arguments is
// non-nil. It returns only when ctx is done.
func (c *Client) Follow(ctx context.Context, fn func(*server.Snapshot, *server.Event)) error {
	for {
		if err := c.followOnce(ctx, fn); err != nil && ctx.Err() == nil {
			select {
			case <-time.After(followRetry):
			case <-ctx.Done():
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

const followRetry = time.Second

// followOnce runs one snapshot+stream round and returns when it must restart.
func (c *Client) followOnce(ctx context.Context, fn func(*server.Snapshot, *server.Event)) error {
	snap, err := c.Snapshot(ctx)
	if err != nil {
		return err
	}
	fn(&snap, nil)
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	evs, err := c.Events(sctx, snap.Seq)
	if err != nil {
		return err
	}
	next := snap.Seq + 1
	for ev := range evs {
		if ev.Seq != next {
			return nil
		}
		next++
		fn(nil, &ev)
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	// The host is ignored by the unix dialer; it only has to parse.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://gx"+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("server returned %s for %s: %s", resp.Status, path, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
