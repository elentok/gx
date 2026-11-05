package apiclient

import (
	"context"
	"net/url"

	"github.com/elentok/gx/server/wire"
)

// Sessions is the sub-client for interactive sessions, designs and the TUI
// toggles that ride on them. It shares Client's transport.
type Sessions struct {
	t *transport
}

// Create starts an interactive session. A refusal is a result
// (Refused set), not an error.
func (s *Sessions) Create(ctx context.Context, req wire.CreateSessionRequest) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/create", req, &res)
	return res, err
}

// OpenDesign returns the Design's live session or starts one. A refusal is a
// result (Refused set), not an error.
func (s *Sessions) OpenDesign(ctx context.Context, req wire.DesignOpenRequest) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/designs/open", req, &res)
	return res, err
}

// DeleteDesign deletes an uncharted design's epic. A refusal is a result
// (Refused set), not an error.
func (s *Sessions) DeleteDesign(ctx context.Context, req wire.DesignDeleteRequest) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/designs/delete", req, &res)
	return res, err
}

// DesignWorktree returns a map's design worktree, made and rebased if needed,
// or the trunk checkout after Branch. A refusal is a result (Refused set), not
// an error.
func (s *Sessions) DesignWorktree(ctx context.Context, req wire.DesignWorktreeRequest) (wire.DesignWorktreeResult, error) {
	var res wire.DesignWorktreeResult
	err := s.t.post(ctx, "/v1/designs/worktree", req, &res)
	return res, err
}

// FixAgent starts a fix agent for a ticket paused for repair. A refusal is a
// result (Refused set), not an error.
func (s *Sessions) FixAgent(ctx context.Context, req wire.FixAgentRequest) (wire.FixAgentResult, error) {
	var res wire.FixAgentResult
	err := s.t.post(ctx, "/v1/tickets/fix-agent", req, &res)
	return res, err
}

// RetryMigration runs a project's pending migration steps again in the server.
// A refusal is a result (Refused set), not an error.
func (s *Sessions) RetryMigration(ctx context.Context, req wire.RetryMigrationRequest) (wire.RetryMigrationResult, error) {
	var res wire.RetryMigrationResult
	err := s.t.post(ctx, "/v1/projects/retry-migration", req, &res)
	return res, err
}

// Stop ends a live session's holder. A refusal is a result (Refused set), not
// an error.
func (s *Sessions) Stop(ctx context.Context, id string) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/stop", wire.SessionRequest{ID: id}, &res)
	return res, err
}

// Interrupt ends a Working session's current turn and keeps the session. A
// refusal is a result (Refused set), not an error.
func (s *Sessions) Interrupt(ctx context.Context, id string) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/interrupt", wire.SessionRequest{ID: id}, &res)
	return res, err
}

// SetAutoAdvance turns a Design session's auto-advance switch on or off. A
// refusal is a result (Refused set), not an error.
func (s *Sessions) SetAutoAdvance(ctx context.Context, id string, on bool) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/auto-advance", wire.AutoAdvanceRequest{ID: id, Off: !on}, &res)
	return res, err
}

// CancelAutoAdvance stops a session's running auto-advance countdown. A refusal
// is a result (Refused set), not an error.
func (s *Sessions) CancelAutoAdvance(ctx context.Context, id string) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/auto-advance/cancel", wire.SessionRequest{ID: id}, &res)
	return res, err
}

// Remove deletes an Exited session's state dir. A refusal is a result (Refused
// set), not an error.
func (s *Sessions) Remove(ctx context.Context, id string) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/remove", wire.SessionRequest{ID: id}, &res)
	return res, err
}

// MarkRead clears a session's Unread. A refusal is a result (Refused set), not
// an error.
func (s *Sessions) MarkRead(ctx context.Context, id string) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/mark-read", wire.SessionRequest{ID: id}, &res)
	return res, err
}

// Focus asks the most recently focused TUI to show a session; Client names it,
// empty with no TUI connected.
func (s *Sessions) Focus(ctx context.Context, id string) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/show", wire.SessionRequest{ID: id}, &res)
	return res, err
}

// RunAction starts a session's Action; it runs on in the server. A refusal is
// a result (Refused set), not an error.
func (s *Sessions) RunAction(ctx context.Context, id string) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/run-action", wire.SessionRequest{ID: id}, &res)
	return res, err
}

// Resume resumes an Exited session's conversation in a new holder. A refusal
// is a result (Refused set), not an error.
func (s *Sessions) Resume(ctx context.Context, id string) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/resume", wire.SessionRequest{ID: id}, &res)
	return res, err
}

// Prompt types text into an Idle Claude session. A refusal is a result
// (Refused set), not an error.
func (s *Sessions) Prompt(ctx context.Context, req wire.SessionPromptRequest) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/prompt", req, &res)
	return res, err
}

// AddImage saves one Prompt image, as its raw bytes, in a live session's
// state folder. A refusal is a result (Refused set), not an error.
func (s *Sessions) AddImage(ctx context.Context, id string, data []byte) (wire.SessionImageResult, error) {
	var res wire.SessionImageResult
	err := s.t.postBody(ctx, "/v1/sessions/images?id="+url.QueryEscape(id), "application/octet-stream", data, &res)
	return res, err
}

// Image reads one saved Prompt image: its bytes and detected Content-Type.
func (s *Sessions) Image(ctx context.Context, session, image string) ([]byte, string, error) {
	return s.t.getBytes(ctx, "/v1/sessions/images?session="+url.QueryEscape(session)+"&id="+url.QueryEscape(image))
}

// RemoveImage deletes one saved Prompt image that has not been sent. A refusal
// is a result (Refused set), not an error.
func (s *Sessions) RemoveImage(ctx context.Context, req wire.SessionImageRequest) (wire.SessionImageResult, error) {
	var res wire.SessionImageResult
	err := s.t.post(ctx, "/v1/sessions/images/delete", req, &res)
	return res, err
}

// Keys sends a named key or literal text to a live session. A refusal is a
// result (Refused set), not an error.
func (s *Sessions) Keys(ctx context.Context, req wire.SessionKeysRequest) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/keys", req, &res)
	return res, err
}

// MarkActionable raises an agent's Actionable on its session. A refusal is a
// result (Refused set), not an error.
func (s *Sessions) MarkActionable(ctx context.Context, req wire.MarkActionableRequest) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/actionable", req, &res)
	return res, err
}

// Link sets an agent's Session ticket. A refusal is a result (Refused set),
// not an error.
func (s *Sessions) Link(ctx context.Context, req wire.LinkSessionRequest) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/link", req, &res)
	return res, err
}

// ReportStep stores the Design step an agent is running on its session. A
// refusal is a result (Refused set), not an error.
func (s *Sessions) ReportStep(ctx context.Context, req wire.ReportStepRequest) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/step", req, &res)
	return res, err
}

// ReportFocus sends a TUI's focus report. A refusal is a result (Refused set),
// not an error.
func (s *Sessions) ReportFocus(ctx context.Context, req wire.FocusReport) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/focus", req, &res)
	return res, err
}

// List lists the interactive sessions.
func (s *Sessions) List(ctx context.Context) ([]wire.SessionInfo, error) {
	var l []wire.SessionInfo
	err := s.t.get(ctx, "/v1/sessions", &l)
	return l, err
}

// Chat reads the visible messages of a session's newest conversation.
func (s *Sessions) Chat(ctx context.Context, id string) (wire.SessionChatResponse, error) {
	var res wire.SessionChatResponse
	err := s.t.get(ctx, "/v1/sessions/chat?session="+url.QueryEscape(id), &res)
	return res, err
}

// ChatArchive decodes a session's archived rollout and reads its chat.
func (s *Sessions) ChatArchive(ctx context.Context, id string) (wire.SessionChatResponse, error) {
	var res wire.SessionChatResponse
	err := s.t.get(ctx, "/v1/sessions/chat/archive?session="+url.QueryEscape(id), &res)
	return res, err
}

// Screen reads a session's screen as plain text rows.
func (s *Sessions) Screen(ctx context.Context, id string) (wire.SessionScreenResponse, error) {
	var res wire.SessionScreenResponse
	err := s.t.get(ctx, "/v1/sessions/screen?session="+url.QueryEscape(id), &res)
	return res, err
}

// Commands reads the command menu of a session (id set) or of a First prompt
// in a project dir (id empty). kind is "/" or "$"; empty means "/".
func (s *Sessions) Commands(ctx context.Context, id, project, dir, kind string) (wire.SessionCommandsResponse, error) {
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
	var res wire.SessionCommandsResponse
	err := s.t.get(ctx, "/v1/sessions/commands?"+q.Encode(), &res)
	return res, err
}

// Permission reads the dialog a Blocked session waits on.
func (s *Sessions) Permission(ctx context.Context, id string) (wire.SessionPermissionResponse, error) {
	var res wire.SessionPermissionResponse
	err := s.t.get(ctx, "/v1/sessions/permission?session="+url.QueryEscape(id), &res)
	return res, err
}

// Transcript locates a session's conversation file.
func (s *Sessions) Transcript(ctx context.Context, id string) (wire.SessionTranscriptResponse, error) {
	var res wire.SessionTranscriptResponse
	err := s.t.get(ctx, "/v1/sessions/transcript?session="+url.QueryEscape(id), &res)
	return res, err
}

// SetAFK sets AFK mode; a nil on toggles it.
func (s *Sessions) SetAFK(ctx context.Context, on *bool) (wire.AFKResult, error) {
	var res wire.AFKResult
	err := s.t.post(ctx, "/v1/afk", wire.AFKRequest{On: on}, &res)
	return res, err
}

// SetViMode sets the web UI's vi mode; a nil on toggles it.
func (s *Sessions) SetViMode(ctx context.Context, on *bool) (wire.ViModeResult, error) {
	var res wire.ViModeResult
	err := s.t.post(ctx, "/v1/vimode", wire.ViModeRequest{On: on}, &res)
	return res, err
}
