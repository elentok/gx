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

// DesignDrafts lists the New Design drafts, most recently edited first.
func (s *Sessions) DesignDrafts(ctx context.Context) ([]wire.DesignDraftItem, error) {
	var items []wire.DesignDraftItem
	err := s.t.get(ctx, "/v1/designs/drafts", &items)
	return items, err
}

// DesignDraft reads one draft. A gone draft is a refused result, not an error.
func (s *Sessions) DesignDraft(ctx context.Context, id string) (wire.DesignDraftSaveResult, error) {
	var res wire.DesignDraftSaveResult
	err := s.t.get(ctx, "/v1/designs/drafts/read?id="+url.QueryEscape(id), &res)
	return res, err
}

// SaveDesignDraft saves a draft's text fields against req.Revision. A stale
// revision or a gone draft is a refused result, not an error.
func (s *Sessions) SaveDesignDraft(ctx context.Context, req wire.DesignDraftSaveRequest) (wire.DesignDraftSaveResult, error) {
	var res wire.DesignDraftSaveResult
	err := s.t.post(ctx, "/v1/designs/drafts/save", req, &res)
	return res, err
}

// AddDesignDraftImage saves one image, as its raw bytes, on a draft. A refusal
// is a result (Refused set), not an error.
func (s *Sessions) AddDesignDraftImage(ctx context.Context, id string, data []byte) (wire.SessionImageResult, error) {
	var res wire.SessionImageResult
	err := s.t.postBody(ctx, "/v1/designs/drafts/images?id="+url.QueryEscape(id), "application/octet-stream", data, &res)
	return res, err
}

// DesignDraftImage reads one image of a draft: its bytes and detected Content-Type.
func (s *Sessions) DesignDraftImage(ctx context.Context, id, image string) ([]byte, string, error) {
	return s.t.getBytes(ctx, "/v1/designs/drafts/images?id="+url.QueryEscape(id)+"&image="+url.QueryEscape(image))
}

// DesignImages lists the ids and file names in a map epic's images/ folder.
func (s *Sessions) DesignImages(ctx context.Context, epic string) ([]wire.SessionImageResult, error) {
	var res []wire.SessionImageResult
	err := s.t.get(ctx, "/v1/designs/images?epic="+url.QueryEscape(epic), &res)
	return res, err
}

// DesignImage reads one image of a map epic's images/ folder.
func (s *Sessions) DesignImage(ctx context.Context, epic, id string) ([]byte, string, error) {
	return s.t.getBytes(ctx, "/v1/designs/images/read?epic="+url.QueryEscape(epic)+"&id="+url.QueryEscape(id))
}

// RemoveDesignDraftImage deletes one image of a draft. A refusal is a result.
func (s *Sessions) RemoveDesignDraftImage(ctx context.Context, id, image string) (wire.SessionImageResult, error) {
	var res wire.SessionImageResult
	err := s.t.post(ctx, "/v1/designs/drafts/images/delete", wire.DraftImageRequest{ID: id, Image: image}, &res)
	return res, err
}

// DeleteDesignDraftdeletes a draft. A draft already gone is not an error.
func (s *Sessions) DeleteDesignDraft(ctx context.Context, id string) (wire.DesignDraftSaveResult, error) {
	var res wire.DesignDraftSaveResult
	err := s.t.post(ctx, "/v1/designs/drafts/delete", wire.DesignDraftDeleteRequest{ID: id}, &res)
	return res, err
}

// DesignLastProject reads the project the New Design form last used ("" on a
// fresh server).
func (s *Sessions) DesignLastProject(ctx context.Context) (wire.DesignLastProject, error) {
	var res wire.DesignLastProject
	err := s.t.get(ctx, "/v1/designs/drafts/last-project", &res)
	return res, err
}

// SetDesignLastProject records the last-used project; the newest write wins.
func (s *Sessions) SetDesignLastProject(ctx context.Context, project string) (wire.DesignLastProject, error) {
	var res wire.DesignLastProject
	err := s.t.post(ctx, "/v1/designs/drafts/last-project", wire.DesignLastProject{Project: project}, &res)
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
	res, err := s.StopMany(ctx, []string{id})
	return firstSessionResult(res), err
}

// StopMany stops each session; a refusal is that target's result.
func (s *Sessions) StopMany(ctx context.Context, ids []string) (wire.TargetsResult, error) {
	return s.targets(ctx, "/v1/sessions/stop", ids, false)
}

// RemoveMany removes each session. Yes confirms stopping the live ones.
func (s *Sessions) RemoveMany(ctx context.Context, ids []string, yes bool) (wire.TargetsResult, error) {
	return s.targets(ctx, "/v1/sessions/remove", ids, yes)
}

func (s *Sessions) targets(ctx context.Context, path string, ids []string, yes bool) (wire.TargetsResult, error) {
	req := wire.SessionTargetsRequest{Yes: yes}
	for _, id := range ids {
		req.Targets = append(req.Targets, wire.SessionTarget{ID: id})
	}
	var res wire.TargetsResult
	err := s.t.post(ctx, path, req, &res)
	return res, err
}

// firstSessionResult is the single-session view of a many-target answer.
func firstSessionResult(res wire.TargetsResult) wire.SessionResult {
	if len(res.Results) == 0 {
		return wire.SessionResult{}
	}
	r := res.Results[0]
	return wire.SessionResult{Refused: r.Refused(), Reason: r.Reason, Message: r.Message}
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

// SetAutoQueue turns a Design session's auto-queue switch on or off. A refusal
// is a result (Refused set), not an error.
func (s *Sessions) SetAutoQueue(ctx context.Context, id string, on bool) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/auto-queue", wire.AutoQueueRequest{ID: id, On: on}, &res)
	return res, err
}

// CancelAutoAdvance stops a session's running auto-advance countdown. A refusal
// is a result (Refused set), not an error.
func (s *Sessions) CancelAutoAdvance(ctx context.Context, id string) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/auto-advance/cancel", wire.SessionRequest{ID: id}, &res)
	return res, err
}

// CancelAutoQueue stops a session's running auto-queue countdown. A refusal is
// a result (Refused set), not an error.
func (s *Sessions) CancelAutoQueue(ctx context.Context, id string) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/sessions/auto-queue/cancel", wire.SessionRequest{ID: id}, &res)
	return res, err
}

// Remove deletes an Exited session's state dir; a live session is refused
// needs-yes. A refusal is a result (Refused set), not an error.
func (s *Sessions) Remove(ctx context.Context, id string) (wire.SessionResult, error) {
	res, err := s.RemoveMany(ctx, []string{id}, false)
	return firstSessionResult(res), err
}

// RemoveExited deletes every Exited session in one server-side sweep, or with
// dryRun only lists them.
func (s *Sessions) RemoveExited(ctx context.Context, dryRun bool) (wire.RemoveExitedResult, error) {
	var res wire.RemoveExitedResult
	err := s.t.post(ctx, "/v1/sessions/remove-exited", wire.RemoveExitedRequest{DryRun: dryRun}, &res)
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
