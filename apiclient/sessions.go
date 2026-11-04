package apiclient

import (
	"context"
	"net/url"

	"github.com/elentok/gx/server"
)

// CreateSession starts an interactive session. A refusal is a result
// (Refused set), not an error.
func (c *Client) CreateSession(ctx context.Context, req server.CreateSessionRequest) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/create", req, &res)
	return res, err
}

// OpenDesign returns the Design's live session or starts one. A refusal is a
// result (Refused set), not an error.
func (c *Client) OpenDesign(ctx context.Context, req server.DesignOpenRequest) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/designs/open", req, &res)
	return res, err
}

// DeleteDesign deletes an uncharted design's epic. A refusal is a result
// (Refused set), not an error.
func (c *Client) DeleteDesign(ctx context.Context, req server.DesignDeleteRequest) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/designs/delete", req, &res)
	return res, err
}

// DesignWorktree returns a map's design worktree, made and rebased if needed,
// or the trunk checkout after Branch. A refusal is a result (Refused set), not
// an error.
func (c *Client) DesignWorktree(ctx context.Context, req server.DesignWorktreeRequest) (server.DesignWorktreeResult, error) {
	var res server.DesignWorktreeResult
	err := c.post(ctx, "/v1/designs/worktree", req, &res)
	return res, err
}

// FixAgent starts a fix agent for a ticket paused for repair. A refusal is a
// result (Refused set), not an error.
func (c *Client) FixAgent(ctx context.Context, req server.FixAgentRequest) (server.FixAgentResult, error) {
	var res server.FixAgentResult
	err := c.post(ctx, "/v1/tickets/fix-agent", req, &res)
	return res, err
}

// RetryMigration runs a project's pending migration steps again in the server.
// A refusal is a result (Refused set), not an error.
func (c *Client) RetryMigration(ctx context.Context, req server.RetryMigrationRequest) (server.RetryMigrationResult, error) {
	var res server.RetryMigrationResult
	err := c.post(ctx, "/v1/projects/retry-migration", req, &res)
	return res, err
}

// StopSession ends a live session's holder. A refusal is a result (Refused
// set), not an error.
func (c *Client) StopSession(ctx context.Context, id string) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/stop", server.SessionRequest{ID: id}, &res)
	return res, err
}

// InterruptSession ends a Working session's current turn and keeps the session.
// A refusal is a result (Refused set), not an error.
func (c *Client) InterruptSession(ctx context.Context, id string) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/interrupt", server.SessionRequest{ID: id}, &res)
	return res, err
}

// SetAutoAdvance turns a Design session's auto-advance switch on or off. A
// refusal is a result (Refused set), not an error.
func (c *Client) SetAutoAdvance(ctx context.Context, id string, on bool) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/auto-advance", server.AutoAdvanceRequest{ID: id, Off: !on}, &res)
	return res, err
}

// CancelAutoAdvance stops a session's running auto-advance countdown. A refusal
// is a result (Refused set), not an error.
func (c *Client) CancelAutoAdvance(ctx context.Context, id string) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/auto-advance/cancel", server.SessionRequest{ID: id}, &res)
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

// PromptSession types text into an Idle Claude session. A refusal is a result
// (Refused set), not an error.
func (c *Client) PromptSession(ctx context.Context, req server.SessionPromptRequest) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/prompt", req, &res)
	return res, err
}

// KeysSession sends a named key or literal text to a live session. A refusal
// is a result (Refused set), not an error.
func (c *Client) KeysSession(ctx context.Context, req server.SessionKeysRequest) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/keys", req, &res)
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

// ReportSessionStep stores the Design step an agent is running on its session.
// A refusal is a result (Refused set), not an error.
func (c *Client) ReportSessionStep(ctx context.Context, req server.ReportStepRequest) (server.SessionResult, error) {
	var res server.SessionResult
	err := c.post(ctx, "/v1/sessions/step", req, &res)
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

// SessionChat reads the visible messages of a session's newest conversation.
func (c *Client) SessionChat(ctx context.Context, id string) (server.SessionChatResponse, error) {
	var res server.SessionChatResponse
	err := c.get(ctx, "/v1/sessions/chat?session="+url.QueryEscape(id), &res)
	return res, err
}

// SessionChatArchive decodes a session's archived rollout and reads its chat.
func (c *Client) SessionChatArchive(ctx context.Context, id string) (server.SessionChatResponse, error) {
	var res server.SessionChatResponse
	err := c.get(ctx, "/v1/sessions/chat/archive?session="+url.QueryEscape(id), &res)
	return res, err
}

// SessionScreen reads a session's screen as plain text rows.
func (c *Client) SessionScreen(ctx context.Context, id string) (server.SessionScreenResponse, error) {
	var res server.SessionScreenResponse
	err := c.get(ctx, "/v1/sessions/screen?session="+url.QueryEscape(id), &res)
	return res, err
}

// SessionCommands reads the command menu of a session (id set) or of a First
// prompt in a project dir (id empty). kind is "/" or "$"; empty means "/".
func (c *Client) SessionCommands(ctx context.Context, id, project, dir, kind string) (server.SessionCommandsResponse, error) {
	q := url.Values{}
	if id != "" {
		q.Set("session", id)
	}
	if project != "" {
		q.Set("project", project)
		q.Set("dir", dir)
	}
	if kind != "" {
		q.Set("kind", kind)
	}
	var res server.SessionCommandsResponse
	err := c.get(ctx, "/v1/sessions/commands?"+q.Encode(), &res)
	return res, err
}

// SessionPermission reads the dialog a Blocked session waits on.
func (c *Client) SessionPermission(ctx context.Context, id string) (server.SessionPermissionResponse, error) {
	var res server.SessionPermissionResponse
	err := c.get(ctx, "/v1/sessions/permission?session="+url.QueryEscape(id), &res)
	return res, err
}

// TicketScreen reads a loop ticket agent's screen as plain text rows.
func (c *Client) TicketScreen(ctx context.Context, address string) (server.SessionScreenResponse, error) {
	var res server.SessionScreenResponse
	err := c.get(ctx, "/v1/sessions/screen?ticket="+url.QueryEscape(address), &res)
	return res, err
}
