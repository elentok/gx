package prompt

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/nativerunner/holder"
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
// nothing and reports false. An unchanged automatic name is still pinned, so a
// later terminal title cannot replace it. The caller holds MetaMu.
func (p namer) setName(dir string, meta *sessions.Meta, name string) (changed bool, err error) {
	if name == meta.Name && !meta.AutoName {
		return false, nil
	}
	meta.Name, meta.AutoName = name, false
	if err := sessions.WriteMeta(dir, *meta); err != nil {
		return false, err
	}
	p.events.Publish(wire.EventSessionChanged, meta.ID)
	return true, nil
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

// errRenameBlocked is a skip that keeps the command held.
var errRenameBlocked = fmt.Errorf("%w: chat is blocked", errRenameSkip)

// sendRename types the rename command once, under the typing lock.
func (p namer) sendRename(dir string, seq uint64, command string) {
	id := filepath.Base(dir)
	p.finishRename(dir, id, seq, command, p.typeRename(dir, id, seq, command))
}

// finishRename settles a send. A chat that turned Blocked keeps its command held.
func (p namer) finishRename(dir, id string, seq uint64, command string, err error) {
	if errors.Is(err, errRenameSkip) {
		if errors.Is(err, errRenameBlocked) && p.renames.claimWatch(id) {
			p.sessions.Watches.Start(func() { p.holdRename(dir) })
		}
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
			return errRenameBlocked
		default:
			p.renames.settle(id, seq)
		}
		return errRenameSkip
	}
	if err := run.typeStepWith(0, command, imageSet{}); err != nil {
		return err
	}
	return checkCodexKnownCommand(dir, run, command)
}

// holdRename follows a Blocked chat's status stream and types the held rename
// when Blocked closes into Working or Idle. It drops the command when the chat
// goes Starting, Rate-limited or Exited, or the stream ends. One watcher
// serves the chat (renameSlots.claimWatch); it ends on release, on drop and
// when the server stops. A new popup (Blocked to Blocked) keeps the hold.
// SendAction is not reused: it stops at Blocked and only types from Idle.
func (p namer) holdRename(dir string) {
	id := filepath.Base(dir)
	run, closeRun, err := p.openActionRun(dir, RunOpts{})
	if err != nil {
		p.dropHeldRename(id)
		return
	}
	defer closeRun()
	for {
		select {
		case <-p.sessions.Watches.Stop:
			return
		case msg, ok := <-run.msgs:
			err := run.apply(msg, ok)
			if err == nil && msg.Type != holder.StreamStatus {
				continue
			}
			switch state := run.State(); {
			case err != nil:
				p.dropHeldRename(id)
				return
			case state == wire.SessionBlocked:
			case state == wire.SessionWorking, state == wire.SessionIdle:
				if p.releaseHeldRename(dir, id) {
					return
				}
			default:
				p.dropHeldRename(id)
				return
			}
		}
	}
}

// dropHeldRename empties the slot and ends the watcher, serving a newer
// request that arrives in between by dropping it too.
func (p namer) dropHeldRename(id string) {
	for {
		seq, _ := p.renames.current(id)
		if p.renames.endWatch(id, seq) {
			return
		}
	}
}

// releaseHeldRename types the held command and reports whether the watcher is
// done. typeRename re-reads the status first: Blocked again keeps holding,
// anything else but Working or Idle drops.
func (p namer) releaseHeldRename(dir, id string) bool {
	for {
		seq, command := p.renames.current(id)
		if command == "" {
			return p.renames.endWatch(id, seq)
		}
		err := p.typeRename(dir, id, seq, command)
		if !errors.Is(err, errRenameSkip) {
			finishSend(p.Prompt, dir, command, imageSet{}, err)
		} else if _, left := p.renames.current(id); left != "" {
			return false
		}
		if p.renames.endWatch(id, seq) {
			return true
		}
	}
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
	changed, err := p.setName(dir, &meta, name)
	if err != nil {
		return wire.SessionResult{}, err
	}
	info := meta.Info()
	command := "/rename " + name
	switch state := info.Status.State; {
	case !changed:
	case state == wire.SessionWorking, state == wire.SessionIdle:
		seq := p.renames.put(req.ID, command)
		p.sessions.Watches.Start(func() { p.sendRename(dir, seq, command) })
	case state == wire.SessionBlocked:
		p.renames.put(req.ID, command)
		if p.renames.claimWatch(req.ID) {
			p.sessions.Watches.Start(func() { p.holdRename(dir) })
		}
	default:
		p.renames.put(req.ID, "") // supersedes an older held command
	}
	return wire.SessionResult{Session: &info}, nil
}
