package server

import (
	"net/http"
	"os"
	"time"

	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/chatreader"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
)

// codexArchive finds the zstd rollout of a Codex session whose plain rollout is
// gone. It only looks for the file; nothing is decoded.
func (s *Server) codexArchive(id string, meta sessionMeta) (string, bool) {
	if meta.Harness != string(harness.Codex) || meta.ConversationID == "" {
		return "", false
	}
	home, err := s.projects.CodexHome()
	if err != nil {
		s.log.Warn("codex home", "session", id, "err", err)
		return "", false
	}
	path, ok, err := codexsession.ArchivedRolloutIn(home, meta.ConversationID)
	if err != nil {
		s.log.Warn("find archived rollout", "session", id, "err", err)
		return "", false
	}
	return path, ok
}

// sessionChatArchiveRead decodes a session's archived rollout on demand and
// answers like a plain chat read would.
func (s *Server) sessionChatArchiveRead(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("session")
	_, meta, res, err := s.readableSession(id)
	if res.Refused {
		status := http.StatusBadRequest
		if res.Reason == wire.ReasonSessionUnknown {
			status = http.StatusNotFound
		}
		http.Error(w, res.Message, status)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	path, ok := s.codexArchive(id, meta)
	if !ok {
		http.Error(w, "no archived rollout for this session", http.StatusNotFound)
		return
	}
	read, err := chatreader.ReadZstdFile(harness.Codex, path)
	if err != nil {
		http.Error(w, "cannot decode the archived rollout: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	snap := chat.SnapshotOf(read, harness.Codex, meta.Ticket, s.designFinal(meta.Ticket))
	writeJSON(w, wire.SessionChatResponse{
		ConversationID: meta.ConversationID, Messages: snap.Msgs, Truncated: snap.Truncated,
		Round: snap.Round, Next: snap.Next, Background: []wire.SessionBackgroundTask{},
		Failure:    s.sessionInfo(meta).Failure,
		Unreadable: snap.Unreadable, CodexVersion: snap.CodexVersion,
	})
}

// chatTranscript finds the conversation file of a session with a chat reader,
// and the format kind to read it with.
func (s *Server) chatTranscript(id string, meta sessionMeta) (string, agentrunner.Kind, bool) {
	if meta.Harness == string(harness.Codex) {
		path, ok := s.codexRollout(id, meta)
		return path, harness.Codex, ok
	}
	path, ok := s.claudeTranscript(id, meta)
	return path, harness.Claude, ok
}

// codexRollout finds the rollout of a session's conversation, like
// claudeTranscript does for Claude.
func (s *Server) codexRollout(id string, meta sessionMeta) (string, bool) {
	if meta.ConversationID == "" {
		return "", false
	}
	return s.transcripts.Resolve(meta.ConversationID, meta.Status.State, time.Now(), s.cfg.TranscriptMissTTL, func() (string, bool) {
		home, err := s.projects.CodexHome()
		if err != nil {
			s.log.Warn("codex home", "session", id, "err", err)
			return "", false
		}
		path, ok, err := codexsession.RolloutPathIn(home, meta.ConversationID)
		if err != nil {
			s.log.Warn("find rollout", "session", id, "err", err)
			return "", false
		}
		return path, ok
	})
}

func (s *Server) sessionChatRead(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("session")
	_, meta, res, err := s.readableSession(id)
	if res.Refused {
		status := http.StatusBadRequest
		if res.Reason == wire.ReasonSessionUnknown {
			status = http.StatusNotFound
		}
		http.Error(w, res.Message, status)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := wire.SessionChatResponse{ConversationID: meta.ConversationID, Messages: []wire.SessionChatMessage{}, Background: []wire.SessionBackgroundTask{}, Failure: s.sessionInfo(meta).Failure}
	if !s.settings.Load().HasChatReader(meta.Harness) {
		out.Unsupported = true
		writeJSON(w, out)
		return
	}
	// The page reads only on an event, so a remembered miss from before the
	// transcript existed would hide the message the event announces.
	s.transcripts.ForgetMiss(meta.ConversationID)
	path, kind, found := s.chatTranscript(id, meta)
	if !found {
		_, out.Archived = s.codexArchive(id, meta)
	} else {
		final := s.designFinal(meta.Ticket)
		snap, err := s.chats.Read(path, kind, final, chat.Loader(kind, meta.Ticket, final))
		switch {
		case os.IsNotExist(err): // vanished since the lookup: no chat yet
		case err != nil:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		default:
			out.Messages, out.Truncated = snap.Msgs, snap.Truncated
			out.Round, out.Next, out.Background = snap.Round, snap.Next, snap.Background(time.Now())
			out.Unreadable, out.CodexVersion = snap.Unreadable, snap.CodexVersion
		}
	}
	writeJSON(w, out)
}
