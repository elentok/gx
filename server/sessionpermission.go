package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/codexui"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
)

// sessionPermissionRead serves GET /v1/sessions/permission?session=<id>.
func (s *Server) sessionPermissionRead(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("session")
	res, out, err := s.readSessionPermission(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if res.Refused {
		writeCommandsRefusal(w, res)
		return
	}
	writeJSON(w, out)
}

func (s *Server) readSessionPermission(id string) (wire.SessionResult, wire.SessionPermissionResponse, error) {
	var out wire.SessionPermissionResponse
	if strings.HasPrefix(id, queue.AgentIDPrefix) {
		return sessionRefusal(wire.ReasonSessionReadOnly, fmt.Sprintf("%s is a Queue agent; its loop owns it", id)), out, nil
	}
	dir, meta, res, err := s.SessionDir(id)
	if res.Refused || err != nil {
		return res, out, err
	}
	switch st := meta.Status.State; {
	case !chat.AcceptsKeys(st):
		return sessionRefusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s is %s", id, st)), out, nil
	case st != wire.SessionBlocked:
		return sessionRefusal(wire.ReasonNotBlocked, fmt.Sprintf("session %s is %s, not blocked", id, st)), out, nil
	}
	c, err := nativerunner.DialHolder(dir)
	if err != nil {
		return sessionRefusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s has no live holder", id)), out, nil
	}
	if meta.Harness == string(harness.Codex) {
		// The hook's request is exact and needs no screen.
		if req, hooked, err := nativerunner.ReadPermissionRequest(dir); err != nil {
			return wire.SessionResult{}, out, err
		} else if hooked {
			text := req.Description
			if text == "" {
				text = req.Tool
			}
			return wire.SessionResult{}, wire.SessionPermissionResponse{Kind: wire.PermissionKindPrompt, Text: text, RequestID: req.ID}, nil
		}
	}
	rows, _, err := c.Screen(0)
	if err != nil {
		return wire.SessionResult{}, out, fmt.Errorf("read screen of session %s: %w", id, err)
	}
	return wire.SessionResult{}, permissionFromScreen(meta.Harness, rows), nil
}

// permissionFromScreen reads the dialog on rows. Codex has no plan dialog.
func permissionFromScreen(h string, rows []string) wire.SessionPermissionResponse {
	unreadable := wire.SessionPermissionResponse{Kind: wire.PermissionKindUnreadable}
	if h == string(harness.Codex) {
		d := codexui.ParseDialog(rows)
		if !d.Approvable {
			return unreadable
		}
		return wire.SessionPermissionResponse{Kind: wire.PermissionKindPrompt, Text: joinDialogText(d.Header, d.Body), RequestID: d.ID}
	}
	v := claudeui.ReadDialog(rows)
	switch v.Kind {
	case claudeui.DialogPermission:
		return wire.SessionPermissionResponse{Kind: wire.PermissionKindPrompt, Text: joinDialogText(v.Header, v.Body), RequestID: v.ID}
	case claudeui.DialogPlan:
		return wire.SessionPermissionResponse{Kind: wire.PermissionKindPlan, Text: joinDialogText(v.Header, v.Body), RequestID: v.ID}
	}
	return unreadable
}

func joinDialogText(header, body string) string {
	return strings.TrimSpace(header + "\n" + body)
}
