package chatdrafts

import (
	"errors"
	"net/http"

	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

// gone is the answer for a draft that no longer exists: draft-gone, with the
// chat it became when a Start record is still alive.
func (c *ChatDrafts) gone(id string) ops.ChatDraftResult {
	return ops.ChatDraftResult{Refused: true, Reason: ops.ReasonDraftGone, Started: c.store.Started(id)}
}

// Start turns draft id into a running chat. A failure keeps the draft and
// saves the message on it as start_error. The per-id lock makes a double click
// start one chat: the second Start finds the draft gone.
func (c *ChatDrafts) Start(id string) (ops.ChatDraftResult, error) {
	return c.StartWithEdit(id, nil)
}

// StartWithEdit is Start with an optional edit applied first, under the same
// lock, with the save route's validation and no revision check. A draft that
// is gone or already started ignores the edit. A written edit stays on the
// draft when the start fails. A written settings edit publishes once: on
// success finishStart does it, on a failure this does.
func (c *ChatDrafts) StartWithEdit(id string, edit *ops.ChatDraftEdit) (out ops.ChatDraftResult, err error) {
	unlock := c.store.lock(id)
	defer unlock()
	d, err := c.store.Get(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return c.gone(id), nil
		}
		return ops.ChatDraftResult{}, err
	}
	if session := c.store.Started(id); session != "" {
		return c.finishStart(id, session), nil
	}
	if edit != nil {
		refusal, err := c.store.applyEdit(d, *edit, c.checkSave)
		if err != nil {
			return ops.ChatDraftResult{}, err
		}
		if refusal != nil {
			return *refusal, nil
		}
		if edit.Settings() {
			defer func() {
				if out.Started == "" {
					c.publish()
				}
			}()
		}
	}
	if d.Brief == "" {
		return refuse(wire.ReasonEmptyPrompt, "a brief is required"), nil
	}
	// Saved before any launch step, so every failure keeps the entry; a repeat
	// Start is skipped by the store. A failed save is logged and Start goes on
	// (an explicit delete refuses instead, see deleteDraft).
	if err := c.saveHistory(d); err != nil {
		c.Log.Warn("save chat draft to history", "draft", id, "err", err)
	}
	dir := d.Dir
	if d.Folder == ops.ChatDraftFolderWorktree {
		if d.WorktreePath == "" {
			path, refusal := c.AddWorktree(d.Project, d.Worktree)
			if refusal != "" {
				return c.startFailed(d, wire.ReasonSessionBadRequest, refusal)
			}
			// Saved before the chat exists, so a retry reuses the folder.
			d.WorktreePath = path
			if err := c.store.write(d); err != nil {
				return ops.ChatDraftResult{}, err
			}
		}
		dir = d.WorktreePath
		if HookArmed(c.ForkHookDir, HookStartFail) {
			return c.startFailed(d, ops.ReasonChatDraftStartFailed, ErrHookStartFail.Error())
		}
	}
	ws := ""
	if c.Workspaces != nil {
		ws = c.Workspaces.Workspace(MemberKey(id))
	}
	// The draft goes once the chat runs, so its images are copied out first.
	images, err := c.store.CopyImages(d)
	if err != nil {
		return c.startFailed(d, ops.ReasonChatDraftStartFailed, err.Error())
	}
	res, err := c.CreateChat(sessions.WebCreateSessionRequest{
		Project: d.Project, Dir: dir, Name: d.Name, Prompt: d.Brief, ImagePaths: images,
		Harness: d.Harness, Model: d.Model, Effort: d.Effort, Workspace: ws,
	})
	switch {
	case err != nil:
		return c.startFailed(d, ops.ReasonChatDraftStartFailed, err.Error())
	case res.Refused || res.Session == nil:
		return c.startFailed(d, res.Reason, res.Message)
	}
	if err := c.store.RecordStart(id, res.Session.ID); err != nil {
		c.Log.Warn("record chat draft start", "draft", id, "err", err)
	}
	return c.finishStart(id, res.Session.ID), nil
}

// finishStart drops the draft of a chat that is running. A failure is logged,
// not returned: the chat exists, and the Start record makes a later Start of
// the leftover draft retry this instead of opening a second chat.
func (c *ChatDrafts) finishStart(id, session string) ops.ChatDraftResult {
	if _, err := c.store.remove(id); err != nil {
		c.Log.Warn("remove started chat draft", "draft", id, "err", err)
	}
	if c.Workspaces != nil {
		if err := c.Workspaces.Remove(MemberKey(id)); err != nil {
			c.Log.Warn("drop chat draft membership", "draft", id, "err", err)
		}
	}
	c.publish()
	return ops.ChatDraftResult{Started: session}
}

// startFailed keeps d, saves msg on it as start_error and returns the refusal.
func (c *ChatDrafts) startFailed(d *ops.ChatDraft, reason, msg string) (ops.ChatDraftResult, error) {
	d.StartError = msg
	if err := c.store.write(d); err != nil {
		return ops.ChatDraftResult{}, err
	}
	return refuse(reason, msg), nil
}

func (c *ChatDrafts) start(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[ops.ChatDraftStartRequest](w, r)
	if !ok {
		return
	}
	res, err := c.StartWithEdit(req.ID, req.Edit)
	if err != nil {
		serverError(w, err)
		return
	}
	wire.WriteJSON(w, res)
}
