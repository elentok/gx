package chatdrafts

import (
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
	unlock := c.store.lock(id)
	defer unlock()
	d, err := c.store.Get(id)
	if err != nil {
		if err == ErrNotFound {
			return c.gone(id), nil
		}
		return ops.ChatDraftResult{}, err
	}
	if d.Brief == "" {
		return refuse(wire.ReasonEmptyPrompt, "a brief is required"), nil
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
	}
	ws := ""
	if c.Workspaces != nil {
		ws = c.Workspaces.Workspace(MemberKey(id))
	}
	res, err := c.CreateChat(sessions.WebCreateSessionRequest{
		Project: d.Project, Dir: dir, Name: d.Name, Prompt: d.Brief,
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
	if _, err := c.store.remove(id); err != nil {
		return ops.ChatDraftResult{}, err
	}
	if c.Workspaces != nil {
		if err := c.Workspaces.Remove(MemberKey(id)); err != nil {
			c.Log.Warn("drop chat draft membership", "draft", id, "err", err)
		}
	}
	c.publish()
	return ops.ChatDraftResult{Started: res.Session.ID}, nil
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
	req, ok := decode[ops.ChatDraftReadRequest](w, r)
	if !ok {
		return
	}
	res, err := c.Start(req.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	wire.WriteJSON(w, res)
}
