package prompt

import (
	"log/slog"
	"strings"

	"github.com/elentok/gx/server/drafthistory"
	"github.com/elentok/gx/server/sessions"
)

// sentEntry is the Sent messages entry of a web send, or false for a session
// that saves none. Only a Design and a plain chat (no flow, ticket or fix-of)
// save; the web UI mirrors this rule from the session info.
func sentEntry(meta sessions.Meta, text string) (drafthistory.Entry, bool) {
	e := drafthistory.Entry{Text: text, Project: meta.Project}
	switch {
	case meta.IsDesign():
		_, e.Slug, _ = strings.Cut(meta.Ticket, ":")
		e.Composer = drafthistory.ComposerDesign
	case meta.Flow == "" && meta.Ticket == "" && meta.FixOf == "":
		e.Name, e.Harness, e.Model, e.Effort = meta.Name, meta.Harness, meta.Model, meta.Effort
		e.Composer = drafthistory.ComposerChat
	default:
		return e, false
	}
	return e, true
}

// saveSent saves a web send to the Sent messages list. A failed save is logged
// and the send goes on: history never blocks a send.
func saveSent(h *drafthistory.Store, log *slog.Logger, meta sessions.Meta, text string, imagePaths []string) {
	e, ok := sentEntry(meta, text)
	if !ok {
		return
	}
	if _, err := h.Save(drafthistory.Sent, e, imagePaths); err != nil {
		log.Warn("saving sent message", "session", meta.ID, "err", err)
	}
}
