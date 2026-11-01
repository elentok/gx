package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/codexui"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
)

// Kinds of SessionPermissionResponse.
const (
	PermissionKindPrompt     = "prompt"
	PermissionKindPlan       = "plan"
	PermissionKindUnreadable = "unreadable"
)

// SessionPermissionResponse is the dialog a Blocked session waits on, read by
// the server so the page never parses a screen. RequestID is what a permission
// answer sends back; it is empty when Kind is unreadable.
type SessionPermissionResponse struct {
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	RequestID string `json:"request_id"`
}

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

func (s *Server) readSessionPermission(id string) (SessionResult, SessionPermissionResponse, error) {
	var out SessionPermissionResponse
	if strings.HasPrefix(id, queueAgentIDPrefix) {
		return sessionRefusal(ReasonSessionReadOnly, fmt.Sprintf("%s is a Queue agent; its loop owns it", id)), out, nil
	}
	dir, meta, res, err := s.sessionDir(id)
	if res.Refused || err != nil {
		return res, out, err
	}
	switch st := meta.Status.State; {
	case !acceptsKeys(st):
		return sessionRefusal(ReasonSessionNotLive, fmt.Sprintf("session %s is %s", id, st)), out, nil
	case st != SessionBlocked:
		return sessionRefusal(ReasonNotBlocked, fmt.Sprintf("session %s is %s, not blocked", id, st)), out, nil
	}
	c, err := nativerunner.DialHolder(dir)
	if err != nil {
		return sessionRefusal(ReasonSessionNotLive, fmt.Sprintf("session %s has no live holder", id)), out, nil
	}
	if meta.Harness == string(harness.Codex) {
		// The hook's request is exact and needs no screen.
		if req, hooked, err := nativerunner.ReadPermissionRequest(dir); err != nil {
			return SessionResult{}, out, err
		} else if hooked {
			text := req.Description
			if text == "" {
				text = req.Tool
			}
			return SessionResult{}, SessionPermissionResponse{Kind: PermissionKindPrompt, Text: text, RequestID: req.ID}, nil
		}
	}
	rows, _, err := c.Screen(0)
	if err != nil {
		return SessionResult{}, out, fmt.Errorf("read screen of session %s: %w", id, err)
	}
	return SessionResult{}, permissionFromScreen(meta.Harness, rows), nil
}

// permissionFromScreen reads the dialog on rows. Codex has no plan dialog.
func permissionFromScreen(h string, rows []string) SessionPermissionResponse {
	unreadable := SessionPermissionResponse{Kind: PermissionKindUnreadable}
	if h == string(harness.Codex) {
		d := codexui.ParseDialog(rows)
		if !d.Approvable {
			return unreadable
		}
		return SessionPermissionResponse{Kind: PermissionKindPrompt, Text: joinDialogText(d.Header, d.Body), RequestID: d.ID}
	}
	v := claudeui.ReadDialog(rows)
	switch v.Kind {
	case claudeui.DialogPermission:
		return SessionPermissionResponse{Kind: PermissionKindPrompt, Text: joinDialogText(v.Header, v.Body), RequestID: v.ID}
	case claudeui.DialogPlan:
		return SessionPermissionResponse{Kind: PermissionKindPlan, Text: joinDialogText(v.Header, v.Body), RequestID: v.ID}
	}
	return unreadable
}

func joinDialogText(header, body string) string {
	return strings.TrimSpace(header + "\n" + body)
}
