package prompt

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
)

// namer holds the session-name methods, keeping Prompt's own method count down.
type namer struct{ *Prompt }

// validName trims name and reports whether it can be a session name: not
// empty and on one line.
func validName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	return name, name != "" && !strings.ContainsAny(name, "\r\n")
}

// setName pins name as the session's name. A name equal to a pinned one changes
// nothing. An unchanged automatic name is still pinned, so a later terminal
// title cannot replace it. The caller holds MetaMu.
func (p namer) setName(dir string, meta *sessions.Meta, name string) error {
	if name == meta.Name && !meta.AutoName {
		return nil
	}
	meta.Name, meta.AutoName = name, false
	if err := sessions.WriteMeta(dir, *meta); err != nil {
		return err
	}
	p.events.Publish(wire.EventSessionChanged, meta.ID)
	return nil
}

func (p namer) renameHandler(w http.ResponseWriter, r *http.Request) {
	var req wire.SessionRenameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := p.renameSession(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

// renameSession sets a plain chat's gx name in any Session status. It types
// nothing into the agent.
func (p namer) renameSession(req wire.SessionRenameRequest) (wire.SessionResult, error) {
	if strings.HasPrefix(req.ID, queue.AgentIDPrefix) {
		return sessions.Refusal(wire.ReasonSessionQueue, fmt.Sprintf("session %s is a Queue agent", req.ID)), nil
	}
	p.sessions.Watches.MetaMu.Lock()
	defer p.sessions.Watches.MetaMu.Unlock()
	dir, meta, res, err := p.sessions.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if meta.Flow != "" || meta.Ticket != "" || meta.FixOf != "" {
		return sessions.Refusal(wire.ReasonSessionNotPlain, fmt.Sprintf("session %s is not a plain chat", req.ID)), nil
	}
	if len(req.Name) > wire.MaxSessionPromptBytes {
		return sessions.Refusal(wire.ReasonPromptTextTooLong, fmt.Sprintf("name is over %d bytes", wire.MaxSessionPromptBytes)), nil
	}
	name, ok := validName(req.Name)
	if !ok {
		return sessions.Refusal(wire.ReasonSessionBadName, "a name is one non-empty line"), nil
	}
	if err := p.setName(dir, &meta, name); err != nil {
		return wire.SessionResult{}, err
	}
	info := meta.Info()
	return wire.SessionResult{Session: &info}, nil
}
