package prompt

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/ralphloop/runlog"
)

// Sources of a session prompt, as the event's source.
const (
	promptSourceWeb = "web"
	promptSourceCLI = "cli"
)

func (p *Prompt) promptHandler(source string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.SessionPromptRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := p.promptSession(req, source)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		wire.WriteJSON(w, res)
	}
}

// promptSession types a person's text into an Idle Claude session, in the
// background so the nudges outlive the request. Continue is for design
// sessions only: it types /clear and /gx-design. It skips the draft guard:
// gx.follow-ups/07 tracks that.
func (p *Prompt) promptSession(req wire.SessionPromptRequest, source string) (wire.SessionResult, error) {
	p.sessions.Watches.MetaMu.Lock()
	defer p.sessions.Watches.MetaMu.Unlock()
	dir, meta, res, err := p.sessions.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if req.Continue && req.Promote {
		return sessions.Refusal(wire.ReasonSessionBadRequest, "continue and promote cannot be combined"), nil
	}
	if req.Continue && !meta.Design {
		return sessions.Refusal(wire.ReasonSessionNotDesign, fmt.Sprintf("session %s is not a design session", req.ID)), nil
	}
	if req.Promote {
		// A Queue agent never gets here: SessionDir refuses its id as unknown.
		if meta.Design {
			return sessions.Refusal(wire.ReasonSessionNotPlain, fmt.Sprintf("session %s is a design session, not a plain one", req.ID)), nil
		}
		// The server builds the command, so the prefix rule stays closed to the page.
		req.Text = harness.SkillCommand(meta.Harness, "gx-promote-to-design", "")
	}
	if !p.settings.Load().HasChatReader(meta.Harness) {
		return sessions.Refusal(wire.ReasonPromptHarnessNotSupported, fmt.Sprintf("session %s runs a harness with no chat", req.ID)), nil
	}
	text, refusal := wire.ValidatePromptText(req.Text, "text")
	if !req.Continue {
		if refusal != nil {
			return *refusal, nil
		}
		if text == "" && len(req.Images) == 0 {
			return sessions.Refusal(wire.ReasonEmptyPrompt, "text to type is required"), nil
		}
	}
	var imgs imageSet
	if !req.Continue {
		paths, refusal := p.resolveImages(dir, req.Images, false)
		if refusal != nil {
			return *refusal, nil
		}
		imgs = p.imageSetOf(dir, req.Images, paths)
	}
	if source == promptSourceWeb {
		last, err := p.lastChatMessageID(req.ID, dir, meta)
		if err != nil {
			return wire.SessionResult{}, err
		}
		if last != req.LastMessage {
			return sessions.Refusal(wire.ReasonStale, "the conversation moved; review it first"), nil
		}
	}
	if st := meta.Info().Status; !st.Promptable() {
		return sessions.Refusal(wire.ReasonSessionNotIdle, fmt.Sprintf("session %s is %s, not idle", req.ID, st.State)), nil
	}
	ev := runlog.Event{
		Time: time.Now(), Type: string(events.SessionPrompted),
		Session: req.ID, Source: source, Action: "text", Bytes: len(text),
	}
	if req.Promote {
		ev.Action = "promote"
	}
	if req.Continue {
		// The server builds the slash commands, so the prefix rule stays closed
		// to the page.
		action := chat.DesignContinueAction(meta.Ticket, meta.Harness)
		if res, refused := p.resumer.HandoffRefusal(meta, action); refused {
			return res, nil
		}
		p.CancelAutoAdvance(req.ID, "Auto-advance stopped: you continued")
		meta.Actionable = wire.SessionActionable{}
		meta.AutoSends = 0
		if err := sessions.WriteMeta(dir, meta); err != nil {
			return wire.SessionResult{}, err
		}
		p.events.Publish(wire.EventSessionChanged, req.ID)
		p.resumer.StartContinue(dir, meta, "Continue", action)
		ev.Action = "continue"
		for _, a := range action {
			ev.Bytes += len(a)
		}
	} else {
		p.sessions.Watches.Start(func() { p.sendPrompt(dir, text, imgs) })
		if name, ok := renameCommandArg(text); ok && name != meta.Name {
			// The harness renames itself, but a typed name never follows the
			// terminal title, so the Agents tab would keep the old one.
			meta.Name, meta.AutoName = name, false
			if err := sessions.WriteMeta(dir, meta); err != nil {
				return wire.SessionResult{}, err
			}
			p.events.Publish(wire.EventSessionChanged, req.ID)
		}
		if err := p.notePersonTyped(dir, meta, "Auto-advance stopped: you typed"); err != nil {
			return wire.SessionResult{}, err
		}
	}
	if err := runlog.AppendServerEvent(p.cfg.TicketStore, ev); err != nil {
		p.log.Warn("logging session prompt", "session", req.ID, "err", err)
	}
	info := meta.Info()
	return wire.SessionResult{Session: &info}, nil
}

// notePersonTyped records a person's input on a session: it stops a running
// auto-advance countdown, resets the auto-send count and clears a derived
// Continue. The caller holds metaMu.
func (p *Prompt) notePersonTyped(dir string, meta sessions.Meta, why string) error {
	stopped := p.CancelAutoAdvance(meta.ID, why)
	changed := meta.AutoSends != 0
	meta.AutoSends = 0
	if !stopped && !meta.Actionable.GxRaised && meta.Actionable.Reason != "" {
		meta.Actionable = wire.SessionActionable{}
		changed = true
	}
	if !changed {
		return nil
	}
	if err := sessions.WriteMeta(dir, meta); err != nil {
		return err
	}
	p.events.Publish(wire.EventSessionChanged, meta.ID)
	return nil
}

// lastChatMessageID is the id of the newest visible message of the session's
// chat, or "" with none.
func (p *Prompt) lastChatMessageID(id, dir string, meta sessions.Meta) (string, error) {
	path, kind, ok := p.sessionChat.Transcript(id, meta.ChatMeta())
	if !ok {
		return "", nil
	}
	snap, err := p.sessionChat.ReadSnapshot(path, kind, meta.Ticket, dir)
	if os.IsNotExist(err) || len(snap.Msgs) == 0 {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return snap.Msgs[len(snap.Msgs)-1].ID, nil
}

// renameCommandArg is the new name in a "/rename <name>" or "/name <name>"
// prompt. A bare command (which asks the harness to pick a name) has none.
func renameCommandArg(text string) (string, bool) {
	cmd, arg, _ := strings.Cut(strings.TrimSpace(text), " ")
	if cmd != "/rename" && cmd != "/name" {
		return "", false
	}
	arg = strings.TrimSpace(arg)
	return arg, arg != "" && !strings.ContainsAny(arg, "\r\n")
}
