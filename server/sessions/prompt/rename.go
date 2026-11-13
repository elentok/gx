package prompt

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
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

// errRenameSkip ends a rename send that must type nothing: the chat is not
// Working or Idle any more, or a newer rename owns the slot.
var errRenameSkip = errors.New("rename not typed")

// sendRename types the rename command into the agent once, under the typing
// lock. It re-reads the status and the slot after taking the lock. A Blocked
// chat keeps the command held in the slot.
func (p namer) sendRename(dir string, seq uint64, command string) {
	id := filepath.Base(dir)
	err := p.typeRename(dir, id, seq, command)
	if errors.Is(err, errRenameSkip) {
		return
	}
	if err == nil {
		p.renames.settle(id, seq)
	}
	finishSend(p.Prompt, dir, command, imageSet{}, err)
}

func (p namer) typeRename(dir, id string, seq uint64, command string) error {
	run, closeRun, err := p.openActionRun(dir, RunOpts{})
	if err != nil {
		return err
	}
	defer closeRun()
	run.gate = func() error {
		if err := run.CatchUp(); err != nil {
			return err
		}
		if !p.renames.owns(id, seq) {
			return errRenameSkip
		}
		switch run.State() {
		case wire.SessionWorking, wire.SessionIdle:
			return nil
		case wire.SessionBlocked:
		default:
			p.renames.settle(id, seq)
		}
		return errRenameSkip
	}
	if err := run.typeStepWith(0, command, imageSet{}); err != nil {
		return err
	}
	return p.checkCodexKnownCommand(dir, run, command)
}

// renameSession sets a plain chat's gx name in any Session status, then sends
// the agent its own /rename when the chat is Working or Idle. A Blocked chat's
// command is held in the chat's pending slot.
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
	command := ""
	switch info.Status.State {
	case wire.SessionWorking, wire.SessionIdle, wire.SessionBlocked:
		command = "/rename " + name
	}
	seq := p.renames.put(req.ID, command)
	if command != "" && info.Status.State != wire.SessionBlocked {
		p.sessions.Watches.Start(func() { p.sendRename(dir, seq, command) })
	}
	return wire.SessionResult{Session: &info}, nil
}
